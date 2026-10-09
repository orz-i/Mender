package daytona

import (
	"context"
	"io"
	"time"

	sandboxdomain "github.com/orz-i/Mender/backend/internal/sandbox/domain"
)

const (
	labelSandboxID  = "io.mender.sandbox.id"
	labelGeneration = "io.mender.sandbox.generation"
	labelBackend    = "io.mender.sandbox.backend"
	maxRemoteCalls  = 3000
)

// RemoteSnapshot contains only reviewed remote state needed for admission.
// It deliberately excludes the SDK's preview tokens, URLs and environment.
type RemoteSnapshot struct {
	ID                      string
	State                   string
	Labels                  map[string]string
	CPU, MemoryGiB, DiskGiB int
	Target                  string
	NetworkBlocked          bool
	Public                  bool
	TTLActive               bool
}
type RemoteFile struct {
	Path        string
	Name        string
	Size        int64
	Mode        string
	IsDirectory bool
}
type RemoteExec struct {
	ExitCode int
	Output   string
}

// RemoteSandbox exposes ONLY the subset needed by Workspace + lifecycle.
type RemoteSandbox interface {
	Snapshot(context.Context) (RemoteSnapshot, error)
	WorkingDir(context.Context) (string, error)
	Exec(context.Context, string, string, time.Duration) (RemoteExec, error)
	FileInfo(context.Context, string) (RemoteFile, error)
	ListFiles(context.Context, string, int) ([]RemoteFile, error)
	ReadStream(context.Context, string) (io.ReadCloser, error)
	UploadStream(context.Context, io.Reader, string) error
	Move(context.Context, string, string) error
	DeleteFile(context.Context, string) error
	Start(context.Context) error
	Stop(context.Context) error
	Destroy(context.Context) error
	SetLabels(context.Context, map[string]string) error
}
type RemoteClient interface {
	Create(context.Context, Template, string, map[string]string) (RemoteSandbox, error)
	Get(context.Context, string) (RemoteSandbox, error)
	FindByLabel(context.Context, map[string]string) ([]RemoteSandbox, error)
	Close(context.Context) error
}
type Factory interface {
	Connect(context.Context, Template) (RemoteClient, error)
}
type TemplateLookup interface {
	GetDaytonaTemplate(context.Context, sandboxdomain.ID) (Template, error)
}
