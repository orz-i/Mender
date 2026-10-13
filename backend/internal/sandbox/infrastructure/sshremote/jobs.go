package sshremote

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"path"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	sandboxdomain "github.com/orz-i/Mender/backend/internal/sandbox/domain"
	"github.com/pkg/sftp"
)

const (
	maxJobCommandBytes = 64 << 10
	maxJobRecords      = 64
	maxJobLogChunk     = 64 << 10
)

var (
	ErrJobInvalid     = errors.New("SSH job request invalid")
	ErrJobConflict    = errors.New("SSH job idempotency conflict")
	ErrJobUnknown     = errors.New("SSH job state unknown")
	ErrJobUnavailable = errors.New("SSH job capability unavailable")
	jobKeyPattern     = regexp.MustCompile(`^[a-f0-9]{32}$`)
	jobIDPattern      = regexp.MustCompile(`^sjob_[a-f0-9]{32}$`)
)

type JobState string

const (
	JobRunning   JobState = "running"
	JobCompleted JobState = "completed"
	JobFailed    JobState = "failed"
	JobCancelled JobState = "cancelled"
	JobUnknown   JobState = "unknown"
)

func (s JobState) Terminal() bool { return s == JobCompleted || s == JobFailed || s == JobCancelled }

type JobRequest struct {
	// The trusted caller must generate a random 128-bit hex key before an
	// attempt and reuse it for retries. No model-controlled fallback is used.
	IdempotencyKey string
	Command        string
}

func (r JobRequest) Validate() error {
	if !jobKeyPattern.MatchString(r.IdempotencyKey) || len(r.Command) == 0 || len(r.Command) > maxJobCommandBytes ||
		!utf8.ValidString(r.Command) || strings.TrimSpace(r.Command) == "" || strings.ContainsRune(r.Command, 0) {
		return ErrJobInvalid
	}
	return nil
}

type JobSnapshot struct {
	TaskID          string
	State           JobState
	Unit            string
	CancelRequested bool
	ExitCode        *int
}

type JobLogs struct {
	TaskID    string
	Stream    string
	Content   string
	Cursor    int64
	Truncated bool
}

// JobRunner owns only the remote transient service, never Mender Sandbox
// permissions or the job's persistent TaskID / authoritative result.
type JobRunner interface {
	Start(context.Context, string, string) error
	Observe(context.Context, string) (JobState, error)
	Stop(context.Context, string) error
}

// Jobs is the built-in SSH backend's PRIVATE task engine. A caller must first
// acquire an exact, governed Sandbox session. Stage B provides no Tool or
// external HTTP/CLI dispatch path and does not grant Profile job privileges.
type Jobs struct {
	sandboxID sandboxdomain.ID
	root      string
	store     *jobStore
	runner    JobRunner
}

func (s *Session) Jobs() (*Jobs, error) {
	if s == nil || s.conn == nil || s.conn.SFTP == nil || s.conn.SSH == nil || s.sandboxID == "" {
		return nil, ErrJobUnavailable
	}
	return newJobs(s.sandboxID, s.root, s.conn.SFTP, systemdJobRunner{client: s.conn.SSH, user: s.remoteUser})
}
func newJobs(id sandboxdomain.ID, root string, client *sftp.Client, runner JobRunner) (*Jobs, error) {
	if _, err := sandboxdomain.ParseID(id.String()); err != nil || !path.IsAbs(root) || path.Clean(root) != root || runner == nil || client == nil {
		return nil, ErrJobInvalid
	}
	return &Jobs{sandboxID: id, root: root, store: &jobStore{sftp: client, root: root, sandboxID: id}, runner: runner}, nil
}
func (j *Jobs) identity(key string) string {
	sum := sha256.Sum256([]byte("mender:ssh-job:v1:" + j.sandboxID.String() + ":" + key))
	return "sjob_" + hex.EncodeToString(sum[:16])
}
func (j *Jobs) unit(taskID string) string { return "mender-" + taskID + ".service" }
func validateJobID(raw string) error {
	if !jobIDPattern.MatchString(raw) {
		return ErrJobInvalid
	}
	return nil
}

// Submit never invokes Start twice for one idempotency key, including after
// an SSH ACK is lost. A partial reservation is durable UNKNOWN, never replayed.
func (j *Jobs) Submit(ctx context.Context, request JobRequest) (JobSnapshot, error) {
	if j == nil || j.store == nil || j.runner == nil || request.Validate() != nil {
		return JobSnapshot{}, ErrJobInvalid
	}
	if err := ctx.Err(); err != nil {
		return JobSnapshot{}, err
	}
	id := j.identity(request.IdempotencyKey)
	digest := sha256.Sum256([]byte(request.Command))
	manifest := jobManifest{Version: 1, TaskID: id, SandboxID: j.sandboxID.String(), CommandDigest: hex.EncodeToString(digest[:]), Unit: j.unit(id)}
	created, err := j.store.reserve(ctx, id)
	if err != nil {
		return JobSnapshot{TaskID: id, State: JobUnknown}, err
	}
	if !created {
		saved, e := j.store.manifest(ctx, id)
		if e != nil {
			return JobSnapshot{TaskID: id, State: JobUnknown}, ErrJobUnknown
		}
		if saved != manifest {
			return JobSnapshot{TaskID: id, State: JobUnknown}, ErrJobConflict
		}
		return j.Status(ctx, id)
	}
	if err = j.store.writeManifest(ctx, id, manifest); err != nil {
		return JobSnapshot{TaskID: id, State: JobUnknown}, ErrJobUnknown
	}
	if err = j.store.writeScript(ctx, id, jobScript(j.root, j.store.taskPath(id), request.Command)); err != nil {
		return JobSnapshot{TaskID: id, State: JobUnknown}, ErrJobUnknown
	}
	if err = j.runner.Start(ctx, manifest.Unit, j.store.scriptPath(id)); err != nil {
		// The remote manager may have accepted the unit before the SSH ACK was
		// lost. Do not retry a side-effecting command from this branch.
		return JobSnapshot{TaskID: id, State: JobUnknown, Unit: manifest.Unit}, ErrJobUnknown
	}
	return JobSnapshot{TaskID: id, State: JobRunning, Unit: manifest.Unit}, nil
}

func (j *Jobs) Status(ctx context.Context, taskID string) (JobSnapshot, error) {
	if j == nil || j.store == nil || j.runner == nil || validateJobID(taskID) != nil {
		return JobSnapshot{}, ErrJobInvalid
	}
	snapshot := JobSnapshot{TaskID: taskID, State: JobUnknown}
	record, err := j.store.manifest(ctx, taskID)
	if err != nil {
		return snapshot, ErrJobUnknown
	}
	if record.TaskID != taskID || record.SandboxID != j.sandboxID.String() || record.Unit != j.unit(taskID) {
		return snapshot, ErrJobUnknown
	}
	snapshot.Unit = record.Unit
	snapshot.CancelRequested, err = j.store.cancelRequested(ctx, taskID)
	if err != nil {
		return snapshot, ErrJobUnknown
	}
	if exit, exists, e := j.store.exitCode(ctx, taskID); e != nil {
		return snapshot, ErrJobUnknown
	} else if exists {
		snapshot.ExitCode = &exit
		switch {
		case exit == 0:
			snapshot.State = JobCompleted
		case snapshot.CancelRequested && exit == 143:
			snapshot.State = JobCancelled
		default:
			snapshot.State = JobFailed
		}
		return snapshot, nil
	}
	state, err := j.runner.Observe(ctx, record.Unit)
	if err != nil {
		return snapshot, ErrJobUnknown
	}
	if state == JobRunning {
		snapshot.State = JobRunning
	}
	// A collected or failed systemd unit without a durable result file proves
	// neither success nor cancellation: preserve unknown until reviewed.
	return snapshot, nil
}

func (j *Jobs) Logs(ctx context.Context, id, stream string, cursor int64, limit int) (JobLogs, error) {
	if j == nil || j.store == nil || validateJobID(id) != nil || (stream != "stdout" && stream != "stderr") || cursor < 0 || limit < 1 || limit > maxJobLogChunk {
		return JobLogs{}, ErrJobInvalid
	}
	if _, err := j.store.manifest(ctx, id); err != nil {
		return JobLogs{}, ErrJobUnknown
	}
	content, next, more, err := j.store.readLog(ctx, id, stream, cursor, limit)
	if err != nil {
		return JobLogs{}, err
	}
	return JobLogs{TaskID: id, Stream: stream, Content: content, Cursor: next, Truncated: more}, nil
}
func (j *Jobs) Result(ctx context.Context, id string) (JobSnapshot, error) { return j.Status(ctx, id) }

// Cancel records intent before signalling systemd. Request acknowledgment is
// never mistaken for a terminal cancellation. A second explicit cancel is
// safe, but Status never automatically sends another signal.
func (j *Jobs) Cancel(ctx context.Context, id string) (JobSnapshot, error) {
	snapshot, err := j.Status(ctx, id)
	if err != nil {
		return snapshot, err
	}
	if snapshot.State.Terminal() {
		return snapshot, nil
	}
	if snapshot.State != JobRunning {
		return snapshot, ErrJobUnknown
	}
	if err = j.store.markCancel(ctx, id); err != nil {
		return snapshot, err
	}
	snapshot.CancelRequested = true
	if err = j.runner.Stop(ctx, snapshot.Unit); err != nil {
		snapshot.State = JobUnknown
		return snapshot, ErrJobUnknown
	}
	return j.Status(ctx, id)
}

// Quiescent is a backend-owned absence proof. Unknown, partially reserved,
// cancelled-but-unconfirmed, and running tasks all block Sandbox lifecycle.
// It performs no cleanup or shell execution.
func (j *Jobs) Quiescent(ctx context.Context) error {
	if j == nil || j.store == nil {
		return ErrJobUnavailable
	}
	ids, err := j.store.list(ctx)
	if err != nil {
		return ErrJobUnknown
	}
	for _, id := range ids {
		if err := ctx.Err(); err != nil {
			return err
		}
		state, e := j.Status(ctx, id)
		if e != nil || !state.State.Terminal() {
			return ErrJobUnknown
		}
	}
	return nil
}

// PruneOutput discards old logs and the executable script only after a
// confirmed terminal result. It keeps the manifest, result and immutable
// idempotency tombstone, so a later identical Submit can never rerun.
func (j *Jobs) PruneOutput(ctx context.Context, id string) error {
	snapshot, err := j.Status(ctx, id)
	if err != nil {
		return err
	}
	if !snapshot.State.Terminal() {
		return ErrJobUnknown
	}
	return j.store.pruneOutput(ctx, id)
}
func jobScript(root, taskPath, command string) string {
	result := path.Join(taskPath, "exit")
	temporary := path.Join(taskPath, "exit.tmp")
	stdout := path.Join(taskPath, "stdout")
	stderr := path.Join(taskPath, "stderr")
	// The command is shell-quoted as one argument to sh -c. Executing arbitrary
	// code as the selected remote account is the explicit SSH Sandbox privilege.
	return "#!/bin/sh\n" +
		"umask 077\n" +
		"cd " + quoted(root) + " || exit 125\n" +
		"finish() { rc=$?; printf '%s\\n' \"$rc\" > " + quoted(temporary) + "; mv -f " + quoted(temporary) + " " + quoted(result) + "; }\n" +
		"trap finish EXIT\ntrap 'exit 143' TERM\n" +
		"(ulimit -f 8192; exec /bin/sh -c " + quoted(command) + ") > " + quoted(stdout) + " 2> " + quoted(stderr) + "\n"
}
func parseExit(raw string) (int, error) {
	if len(raw) > 8 || strings.TrimSpace(raw) != raw {
		return 0, ErrJobUnknown
	}
	exit, err := strconv.Atoi(raw)
	if err != nil || exit < 0 || exit > 255 {
		return 0, ErrJobUnknown
	}
	return exit, nil
}
