package sshremote

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io/fs"
	"os"
	"sort"
	"strings"
	"time"
)

// RecoveryEvidence is sanitized, plugin-owned evidence, not a claim that an
// unknown Task finished. No command, remote paths, Credential, output, or
// full manifest data may cross this boundary.
type RecoveryEvidence struct {
	TaskID      string   `json:"taskId"`
	Phase       string   `json:"phase"` // reservation_only, manifest_only, dispatch_uncertain, active, terminal, unverified
	State       JobState `json:"state"`
	Activity    string   `json:"activity"`    // active, quiescent_proved, uncertain
	Fingerprint string   `json:"fingerprint"` // bounded observation fingerprint; never remote fencing proof
	Recoverable bool     `json:"recoverable"` // false for legacy v1: no execution-point fence
}

type RecoveryReport struct {
	Records     []RecoveryEvidence `json:"records"`
	InspectedAt time.Time          `json:"inspectedAt"`
	Verdict     string             `json:"verdict"` // quiescent_proved, active, uncertain
}

// InspectRecovery reads the private legacy-v1 remote inventory without creating
// task files, calling Start/Stop, or resolving any state. Even an apparently
// manifest-only record remains uncertain: an old SSH writer might resume.
// This inspector deliberately cannot resolve or clear task obligations.
func (j *Jobs) InspectRecovery(ctx context.Context) (RecoveryReport, error) {
	if j == nil || j.store == nil || j.runner == nil {
		return RecoveryReport{}, ErrJobUnavailable
	}
	ids, err := j.store.list(ctx)
	if err != nil {
		return RecoveryReport{}, ErrJobUnknown
	}
	sort.Strings(ids)
	report := RecoveryReport{Records: make([]RecoveryEvidence, 0, len(ids)), InspectedAt: time.Now().UTC(), Verdict: "quiescent_proved"}
	for _, id := range ids {
		if err := ctx.Err(); err != nil {
			return RecoveryReport{}, err
		}
		ev := RecoveryEvidence{TaskID: id, Phase: "reservation_only", State: JobUnknown, Activity: "uncertain"}
		version, versionErr := j.store.manifestVersion(ctx, id)
		if versionErr == nil && version == 2 {
			ev, err = j.inspectV2(ctx, id)
			if err != nil {
				return RecoveryReport{}, ErrJobUnknown
			}
			if ev.Activity == "uncertain" {
				report.Verdict = "uncertain"
			} else if ev.Activity == "active" && report.Verdict != "uncertain" {
				report.Verdict = "active"
			}
			report.Records = append(report.Records, ev)
			continue
		}
		manifest, manifestErr := j.store.manifest(ctx, id)
		if manifestErr == nil {
			ev.Phase = "manifest_only"
			// Stat not reading the script: command bytes must remain private.
			script, scriptErr := j.store.scriptPresent(ctx, id)
			if scriptErr != nil {
				ev.Phase = "unverified"
			} else {
				snapshot, statusErr := j.Status(ctx, id)
				if statusErr == nil && snapshot.State.Terminal() {
					ev.Phase, ev.State, ev.Activity = "terminal", snapshot.State, "quiescent_proved"
				} else if script && statusErr == nil && snapshot.State == JobRunning {
					ev.Phase, ev.State, ev.Activity = "active", JobRunning, "active"
				} else if script {
					ev.Phase = "dispatch_uncertain"
				}
			}
		} else if present, probeErr := j.store.filePresence(ctx, id, "manifest.json"); present || probeErr != nil {
			ev.Phase = "unverified"
		}
		// Bind the observation to the exact identity and stable manifest
		// digest without returning raw command hashes, scripts or paths.
		// This fingerprint is NOT permission to resolve v1 ambiguity.
		digest := ""
		if manifestErr == nil {
			digest = manifest.CommandDigest
		}
		hash := sha256.Sum256([]byte(strings.Join([]string{"recovery-evidence:v1", id, ev.Phase, string(ev.State), ev.Activity, digest}, ":")))
		ev.Fingerprint = hex.EncodeToString(hash[:])
		if ev.Activity == "uncertain" {
			report.Verdict = "uncertain"
		} else if ev.Activity == "active" && report.Verdict != "uncertain" {
			report.Verdict = "active"
		}
		report.Records = append(report.Records, ev)
	}
	return report, nil
}

// filePresence treats a symlink or unsafe mode as unverified, not absent. It
// never follows links and never creates any directory or file.
func (s *jobStore) filePresence(ctx context.Context, id, filename string) (bool, error) {
	if err := s.checkTask(ctx, id); err != nil {
		return false, err
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if filename != "manifest.json" && filename != "run.sh" {
		return false, ErrJobInvalid
	}
	info, err := s.sftp.Lstat(s.taskPath(id) + "/" + filename)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil || !info.Mode().IsRegular() || info.Mode()&fs.ModeSymlink != 0 || info.Mode().Perm()&0077 != 0 {
		return false, ErrJobUnknown
	}
	// Large script/manifest files are untrusted, even if they were writable
	// only by the managed SSH login account.
	limit := int64(1024)
	if filename == "run.sh" {
		limit = 1 << 20
	}
	if info.Size() > limit || info.Size() < 0 {
		return false, ErrJobUnknown
	}
	return true, nil
}

func (s *jobStore) scriptPresent(ctx context.Context, id string) (bool, error) {
	return s.filePresence(ctx, id, "run.sh")
}
