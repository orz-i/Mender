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
	TaskID          string   `json:"taskId"`
	State           JobState `json:"state"`
	Unit            string   `json:"unit,omitempty"`
	CancelRequested bool     `json:"cancelRequested"`
	ExitCode        *int     `json:"exitCode,omitempty"`
}

type JobLogs struct {
	TaskID    string `json:"taskId"`
	Stream    string `json:"stream"`
	Content   string `json:"content"`
	Cursor    int64  `json:"cursor"`
	Truncated bool   `json:"truncated"`
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
	// B supports the system-wide transient service manager only. A regular
	// SSH account cannot safely assume systemd --user, linger or authority
	// to create system units. Reject before reserving a durable TaskID.
	if s.remoteUser != "root" {
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

// Prepare checks the remote private storage boundary before host enrolment.
// It creates no task reservation or unit, so a concurrent Sandbox Stop can
// still safely complete before the later atomic submission lease.
func (j *Jobs) Prepare(ctx context.Context) error {
	if j == nil || j.store == nil {
		return ErrJobUnavailable
	}
	return j.store.ensureBase(ctx)
}

// The opaque ID salt is stable across the protocol hard cut so retired
// TaskID tombstones still reject reuse of a previously admitted key.
// It is an identifier format, not a v1 journal compatibility reader.
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

// Status accepts only the durable v2 journal. There is no legacy
// parsing, replay, protocol dispatch or fallback.
func (j *Jobs) Status(ctx context.Context, taskID string) (JobSnapshot, error) {
	if j == nil || j.store == nil || j.runner == nil || validateJobID(taskID) != nil {
		return JobSnapshot{}, ErrJobInvalid
	}
	return j.statusV2(ctx, taskID)
}

func (j *Jobs) Logs(ctx context.Context, id, stream string, cursor int64, limit int) (JobLogs, error) {
	if j == nil || j.store == nil || validateJobID(id) != nil || (stream != "stdout" && stream != "stderr") || cursor < 0 || limit < 1 || limit > maxJobLogChunk {
		return JobLogs{}, ErrJobInvalid
	}
	if _, _, err := j.store.readV2Manifest(ctx, id); err != nil {
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

// Quiescent only trusts v2 terminal or independently fsynced resolution
// evidence. An unknown or partial v2 record blocks resource lifecycle.
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
		if err := j.v2Quiescent(ctx, id); err != nil {
			return ErrJobUnknown
		}
	}
	return nil
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
