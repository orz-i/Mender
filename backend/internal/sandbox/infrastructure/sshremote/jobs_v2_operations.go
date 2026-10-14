package sshremote

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
)

// CheckV2 is a capability gate executed before Core enrolls this Sandbox in
// a task obligation. No v2 attempt may fall back to v1.
func (j *Jobs) CheckV2(ctx context.Context) error {
	if j == nil || j.store == nil || !j.store.v2FileSyncAvailable() {
		return ErrJobUnavailable
	}
	host, ok := j.runner.(v2HostOperations)
	if !ok {
		return ErrJobUnavailable
	}
	return host.VerifyV2(ctx, j.store.basePath())
}

// Submit is the sole, v2-only detached execution entry. Core must hold
// the exact Sandbox/job.submit lease before it is called.
// Existing key/tombstone is always observed, never recreated or launched.
func (j *Jobs) Submit(ctx context.Context, request JobRequest, generation uint64) (JobSnapshot, error) {
	if j == nil || j.store == nil || j.runner == nil || request.Validate() != nil || generation == 0 {
		return JobSnapshot{}, ErrJobInvalid
	}
	if err := ctx.Err(); err != nil {
		return JobSnapshot{}, err
	}
	if err := j.CheckV2(ctx); err != nil {
		return JobSnapshot{}, err
	}
	host := j.runner.(v2HostOperations)
	id := j.identity(request.IdempotencyKey)
	wanted := sha256.Sum256([]byte(request.Command))
	created, err := j.store.reserve(ctx, id)
	if err != nil {
		return JobSnapshot{TaskID: id, State: JobUnknown}, err
	}
	if !created {
		m, _, e := j.store.readV2Manifest(ctx, id)
		if e != nil {
			return JobSnapshot{TaskID: id, State: JobUnknown}, ErrJobUnknown
		}
		if m.CommandDigest != hex.EncodeToString(wanted[:]) || m.RunGeneration != generation {
			return JobSnapshot{TaskID: id, State: JobUnknown}, ErrJobConflict
		}
		return j.Status(ctx, id)
	}
	// Once the task directory exists, all interruptions preserve the idempotency
	// tombstone as UNKNOWN. No write is ever replayed.
	// The task directory may have been bind-mounted independently from its
	// parent. Validate the exact execution filesystem before any script or
	// launch-intent material can be created.
	if err = host.VerifyV2(ctx, j.store.taskPath(id)); err != nil {
		return JobSnapshot{TaskID: id, State: JobUnknown}, ErrJobUnknown
	}
	if err = host.PersistV2Directory(ctx, j.store.basePath()); err != nil {
		return JobSnapshot{TaskID: id, State: JobUnknown}, ErrJobUnknown
	}
	plan, e := j.planV2(request)
	if e != nil {
		return JobSnapshot{TaskID: id, State: JobUnknown}, ErrJobUnknown
	}
	plan.manifest.RunGeneration = generation
	raw, e := json.Marshal(plan.manifest)
	if e != nil {
		return JobSnapshot{TaskID: id, State: JobUnknown}, ErrJobUnknown
	}
	if err = j.store.writeV2Exclusive(ctx, id, "manifest.json", raw, host); err != nil {
		return JobSnapshot{TaskID: id, State: JobUnknown}, ErrJobUnknown
	}
	if err = j.store.writeV2Exclusive(ctx, id, "run.sh", []byte(plan.script), host); err != nil {
		return JobSnapshot{TaskID: id, State: JobUnknown}, ErrJobUnknown
	}
	launch := "v2:" + id + ":" + plan.manifest.AttemptID + ":" + plan.manifest.CommandDigest + "\n"
	if err = j.store.writeV2Exclusive(ctx, id, "launch.intent", []byte(launch), host); err != nil {
		return JobSnapshot{TaskID: id, State: JobUnknown}, ErrJobUnknown
	}
	if err = j.runner.Start(ctx, plan.manifest.Unit, j.store.scriptPath(id)); err != nil {
		return JobSnapshot{TaskID: id, State: JobUnknown, Unit: plan.manifest.Unit}, ErrJobUnknown
	}
	return JobSnapshot{TaskID: id, State: JobRunning, Unit: plan.manifest.Unit}, nil
}

func (j *Jobs) statusV2(ctx context.Context, id string) (JobSnapshot, error) {
	ev, err := j.store.readV2Evidence(ctx, id)
	out := JobSnapshot{TaskID: id, State: JobUnknown}
	if err != nil {
		return out, ErrJobUnknown
	}
	out.Unit = ev.manifest.Unit
	out.CancelRequested = ev.cancel
	if ev.terminal {
		out.ExitCode = ev.exitCode
		switch {
		case *ev.exitCode == 0:
			out.State = JobCompleted
		case ev.cancel && *ev.exitCode == 143:
			out.State = JobCancelled
		default:
			out.State = JobFailed
		}
		return out, nil
	}
	if ev.resolved {
		return out, nil
	} // Operational verdict distinct from exit code.
	if ev.started {
		status, e := j.runner.Observe(ctx, ev.manifest.Unit)
		if e != nil {
			return out, ErrJobUnknown
		}
		if status == JobRunning {
			out.State = JobRunning
		}
	}
	// No valid start marker is NOT completion, even after failed systemd ACK.
	return out, nil
}

func (j *Jobs) inspectV2(ctx context.Context, id string) (RecoveryEvidence, error) {
	ev, err := j.store.readV2Evidence(ctx, id)
	report := RecoveryEvidence{TaskID: id, State: JobUnknown, Phase: "unverified", Activity: "uncertain"}
	if err != nil {
		// Partial/corrupt v2 evidence stays visible as unrecoverable UNKNOWN.
		// It must not prevent inspection of every other task in the Sandbox.
		fingerprint := sha256.Sum256([]byte("mender-ssh-v2-unverified:" + id))
		report.Fingerprint = hex.EncodeToString(fingerprint[:])
		return report, nil
	}
	report.Fingerprint = ev.fingerprint
	switch {
	case ev.resolved:
		// A marker can be visible after a lost directory-sync ACK without
		// being durable. Confirm fsync of the existing marker+directory under
		// the launch lock before advertising any quiescence proof.
		if j.proveV2Resolution(ctx, id, ev) == nil {
			report.Phase = "resolved_before_launch"
			report.Activity = "quiescent_proved"
		}
	case ev.terminal:
		report.Phase = "terminal"
		report.Activity = "quiescent_proved"
		if ev.exitCode != nil {
			if *ev.exitCode == 0 {
				report.State = JobCompleted
			} else if ev.cancel && *ev.exitCode == 143 {
				report.State = JobCancelled
			} else {
				report.State = JobFailed
			}
		}
	case ev.started:
		report.Phase = "dispatch_uncertain"
		status, e := j.runner.Observe(ctx, ev.manifest.Unit)
		if e == nil && status == JobRunning {
			report.Phase = "active"
			report.Activity = "active"
			report.State = JobRunning
		}
	case ev.startPartial:
		report.Phase = "unverified"
	default:
		report.Phase = "prelaunch_candidate"
		// A candidate is not a no-execution proof. Resolution requires Core owner
		// authorization and the backend's exact remote lock at execution point.
		report.Recoverable = true
	}
	return report, nil
}

type V2ResolveRequest struct {
	TaskID      string
	Fingerprint string
	Generation  uint64
	Actor       string
	Reason      string
}

func (r V2ResolveRequest) Validate() error {
	if validateJobID(r.TaskID) != nil || !hex64(r.Fingerprint) || r.Generation == 0 ||
		len(r.Actor) < 3 || len(r.Actor) > 256 || strings.ContainsAny(r.Actor, "\r\n\x00") ||
		len(strings.TrimSpace(r.Reason)) < 5 || len(r.Reason) > 512 || strings.ContainsRune(r.Reason, 0) {
		return ErrJobInvalid
	}
	return nil
}

// ResolveV2UnderFence must only run while Core holds an exact paused
// submission/recovery lease. It asks the backend to acquire the SAME remote
// exclusive launch lock as the gate; no successful response is possible on an
// unproved v1, running, corrupt, partial, or stale-evidence Task.
func (j *Jobs) ResolveV2UnderFence(ctx context.Context, req V2ResolveRequest) (RecoveryEvidence, error) {
	if j == nil || j.store == nil || req.Validate() != nil {
		return RecoveryEvidence{}, ErrJobInvalid
	}
	host, ok := j.runner.(v2HostOperations)
	if !ok || !j.store.v2FileSyncAvailable() {
		return RecoveryEvidence{}, ErrJobUnavailable
	}
	if err := host.VerifyV2(ctx, j.store.basePath()); err != nil {
		return RecoveryEvidence{}, err
	}
	evidence, err := j.store.readV2Evidence(ctx, req.TaskID)
	if err != nil {
		return RecoveryEvidence{}, ErrJobUnknown
	}
	if evidence.manifest.RunGeneration != req.Generation || evidence.fingerprint != req.Fingerprint ||
		evidence.started || evidence.startPartial || evidence.terminal || evidence.resolved {
		return RecoveryEvidence{}, ErrJobConflict
	}
	reasonHash := sha256.Sum256([]byte(req.Reason))
	marker := v2ResolutionMarker{Version: 2, TaskID: req.TaskID, Disposition: "aborted_before_launch",
		ManifestHash: evidence.manifestHash, ScriptHash: evidence.scriptHash,
		AttemptID: evidence.manifest.AttemptID, EvidenceFingerprint: req.Fingerprint,
		Actor: req.Actor, ReasonDigest: hex.EncodeToString(reasonHash[:])}
	payload, err := json.Marshal(marker)
	if err != nil {
		return RecoveryEvidence{}, fmt.Errorf("encode v2 resolution: %w", err)
	}
	r := v2RemoteResolution{TaskDir: j.store.taskPath(req.TaskID),
		ManifestHash: evidence.manifestHash, ScriptHash: evidence.scriptHash,
		LaunchHash: evidence.launchHash, LaunchPresent: evidence.launchPresent, Marker: payload}
	if err = host.ResolveV2UnderLock(ctx, r); err != nil {
		return RecoveryEvidence{}, ErrJobUnknown
	}
	// Lost or ambiguous readback is UNKNOWN, not an invitation to append again.
	after, err := j.store.readV2Evidence(ctx, req.TaskID)
	if err != nil || !after.resolved || after.resolution != marker {
		return RecoveryEvidence{}, ErrJobUnknown
	}
	verified, err := j.inspectV2(ctx, req.TaskID)
	if err != nil || verified.Activity != "quiescent_proved" {
		return RecoveryEvidence{}, ErrJobUnknown
	}
	return verified, nil
}

// v2Quiescent is allowed to recognize only actual terminal artifact or a
// verified immutable resolution, never an absent systemd Unit.
func (j *Jobs) v2Quiescent(ctx context.Context, id string) error {
	e, err := j.store.readV2Evidence(ctx, id)
	if err != nil || (!e.terminal && !e.resolved) {
		return ErrJobUnknown
	}
	if e.resolved {
		return j.proveV2Resolution(ctx, id, e)
	}
	return nil
}

// proveV2Resolution obtains a fresh, positive remote durability certificate
// under the same execution-point lock. Reading a valid marker is insufficient
// if a prior SSH ACK was lost before the directory fsync completed.
func (j *Jobs) proveV2Resolution(ctx context.Context, id string, evidence v2Evidence) error {
	if j == nil || j.runner == nil || j.store == nil || !evidence.resolved ||
		!j.store.v2FileSyncAvailable() {
		return ErrJobUnknown
	}
	host, ok := j.runner.(v2HostOperations)
	if !ok || host.VerifyV2(ctx, j.store.taskPath(id)) != nil {
		return ErrJobUnknown
	}
	raw, err := j.store.readFile(ctx, id, "resolution.json", 2048)
	if err != nil {
		return ErrJobUnknown
	}
	digest := sha256.Sum256(raw)
	if err = host.ProveV2ResolutionDurable(ctx, j.store.taskPath(id), hex.EncodeToString(digest[:])); err != nil {
		return ErrJobUnknown
	}
	// A concurrent remote change between inspection and the proof must not
	// turn uncertain evidence into a lifecycle-release acknowledgement.
	after, err := j.store.readV2Evidence(ctx, id)
	if err != nil || !after.resolved || after.fingerprint != evidence.fingerprint {
		return ErrJobUnknown
	}
	return nil
}
