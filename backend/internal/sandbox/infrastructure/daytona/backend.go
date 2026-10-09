package daytona

import (
	"context"
	"fmt"
	"path"
	"strings"
	"sync/atomic"
	"time"

	invocationdomain "github.com/orz-i/Mender/backend/internal/invocation/domain"
	sandboxapplication "github.com/orz-i/Mender/backend/internal/sandbox/application"
	sandboxdomain "github.com/orz-i/Mender/backend/internal/sandbox/domain"
)

type Backend struct {
	factory   Factory
	templates TemplateLookup
}

func NewBackend(factory Factory, templates TemplateLookup) (*Backend, error) {
	if factory == nil || templates == nil {
		return nil, fmt.Errorf("daytona backend requires trusted credential factory and template lookup")
	}
	return &Backend{factory: factory, templates: templates}, nil
}
func (*Backend) Kind() sandboxdomain.BackendKind { return sandboxdomain.BackendDaytona }
func (*Backend) Capabilities() sandboxdomain.Capabilities {
	return sandboxdomain.Capabilities{Read: true, Search: true, Edit: true, Shell: true}
}

func (b *Backend) Open(ctx context.Context, item sandboxdomain.Sandbox, run sandboxdomain.Run) (sandboxapplication.Session, error) {
	if b == nil || b.factory == nil || b.templates == nil {
		return nil, sandboxdomain.ErrBackendUnavailable
	}
	if err := ValidateSandbox(item); err != nil {
		return nil, err
	}
	if run.SandboxID != item.ID() || run.State != sandboxdomain.StateReady || run.Generation < 1 {
		return nil, sandboxdomain.ErrConflict
	}
	if err := ValidateExternalID(run.ExternalID); err != nil {
		return nil, sandboxdomain.ErrConflict
	}
	t, err := b.templates.GetDaytonaTemplate(ctx, item.ID())
	if err != nil {
		return nil, sandboxdomain.ErrBackendUnavailable
	}
	if err := t.Validate(); err != nil {
		return nil, err
	}
	client, err := b.factory.Connect(ctx, t)
	if err != nil {
		return nil, err
	}
	cleanup := func() {
		closing, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = client.Close(closing)
	}
	remote, err := client.Get(ctx, run.ExternalID)
	if err != nil {
		cleanup()
		return nil, err
	}
	if err := verifySnapshot(ctx, remote, item.ID(), run.Generation, t, "started"); err != nil {
		cleanup()
		return nil, err
	}
	root, err := remote.WorkingDir(ctx)
	if err != nil {
		cleanup()
		return nil, sandboxdomain.ErrBackendUnavailable
	}
	if err := validateRemoteRoot(root); err != nil {
		cleanup()
		return nil, err
	}
	return &session{client: client, remote: remote, template: t, root: root}, nil
}

// Verify a service-observed identity, network block, TTL and reserved quotas.
// No claimed security property is inferred solely from the user-supplied plan.
func verifySnapshot(ctx context.Context, remote RemoteSandbox, id sandboxdomain.ID, generation uint64, t Template, expectedState string) error {
	if remote == nil {
		return sandboxdomain.ErrBackendUnavailable
	}
	snapshot, err := remote.Snapshot(ctx)
	if err != nil {
		return err
	}
	if err := ValidateExternalID(snapshot.ID); err != nil {
		return sandboxdomain.ErrConflict
	}
	if snapshot.State != expectedState || snapshot.Labels[labelSandboxID] != id.String() || snapshot.Labels[labelGeneration] != fmt.Sprintf("%d", generation) || snapshot.Labels[labelBackend] != "daytona" || snapshot.Target != t.Target || !snapshot.NetworkBlocked || snapshot.Public || !snapshot.TTLActive || snapshot.CPU != t.CPU || snapshot.MemoryGiB != t.MemoryGiB || snapshot.DiskGiB != t.DiskGiB {
		return fmt.Errorf("daytona remote identity, TTL, network or resource constraints mismatch: %w", sandboxdomain.ErrBackendUnavailable)
	}
	return nil
}
func validateRemoteRoot(root string) error {
	if !path.IsAbs(root) || root == "/" || len(root) > 512 || strings.ContainsAny(root, "\x00\\") || path.Clean(root) != root {
		return fmt.Errorf("daytona service working directory is not a safe absolute root: %w", sandboxdomain.ErrBackendUnavailable)
	}
	return nil
}

type session struct {
	client    RemoteClient
	remote    RemoteSandbox
	template  Template
	root      string
	uncertain atomic.Bool
}

func (s *session) Close() error {
	if s == nil {
		return nil
	}
	if s.client == nil {
		return sandboxdomain.ErrBackendUnavailable
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := s.client.Close(ctx); err != nil {
		return sandboxdomain.ErrBackendUnavailable
	}
	if s.uncertain.Load() {
		return fmt.Errorf("daytona operation outcome uncertain; durable lease retained: %w", sandboxdomain.ErrConflict)
	}
	return nil
}
func (s *session) ExecuteWorkspace(ctx context.Context, cap sandboxdomain.Capability, args invocationdomain.JSONValue) (invocationdomain.Outcome, error) {
	if s == nil || s.remote == nil || s.client == nil {
		return invocationdomain.Outcome{}, sandboxdomain.ErrBackendUnavailable
	}
	if err := ctx.Err(); err != nil {
		return invocationdomain.Outcome{}, err
	}
	switch cap {
	case sandboxdomain.CapabilityRead:
		return s.read(ctx, args)
	case sandboxdomain.CapabilitySearch:
		return s.search(ctx, args)
	case sandboxdomain.CapabilityEdit:
		return s.edit(ctx, args)
	case sandboxdomain.CapabilityShell:
		return s.shell(ctx, args)
	default:
		return invocationdomain.Outcome{}, sandboxdomain.ErrCapabilityUnavailable
	}
}
func (s *session) executeFixed(ctx context.Context, command string, timeout time.Duration) (RemoteExec, error) {
	if timeout < time.Second || timeout > 30*time.Second {
		return RemoteExec{}, sandboxdomain.ErrCapabilityUnavailable
	}
	limited, cancel := context.WithTimeout(ctx, timeout+2*time.Second)
	defer cancel()
	result, err := s.remote.Exec(limited, command, s.root, timeout)
	if err != nil {
		if limited.Err() != nil {
			s.uncertain.Store(true)
		}
		return RemoteExec{}, safeRemoteError(limited, err)
	}
	if len(result.Output) > 128*1024 {
		return RemoteExec{}, sandboxdomain.ErrCapabilityUnavailable
	}
	return result, nil
}

var _ sandboxapplication.Backend = (*Backend)(nil)
var _ sandboxapplication.WorkspaceSession = (*session)(nil)
