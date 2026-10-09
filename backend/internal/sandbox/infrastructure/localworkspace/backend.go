// Package localworkspace implements one local Sandbox backend. Structured file
// tools and the shell share a canonical root. Restricted host applies an OS
// policy to child processes; it is not a container or a virtual machine.
package localworkspace

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"unicode/utf8"

	invocationdomain "github.com/orz-i/Mender/backend/internal/invocation/domain"
	sandboxapplication "github.com/orz-i/Mender/backend/internal/sandbox/application"
	sandboxdomain "github.com/orz-i/Mender/backend/internal/sandbox/domain"
)

// Backend is a host-owned local adapter. The ShellLauncher must enforce the
// caller-selected isolation mode and must never silently downgrade it to host.
type Backend struct{ launcher ShellLauncher }

func New(launcher ShellLauncher) (*Backend, error) {
	if launcher == nil {
		return nil, fmt.Errorf("local workspace: governed launcher required")
	}
	return &Backend{launcher: launcher}, nil
}
func (*Backend) Kind() sandboxdomain.BackendKind { return sandboxdomain.BackendLocalWorkspace }
func (*Backend) Capabilities() sandboxdomain.Capabilities {
	return sandboxdomain.Capabilities{Read: true, Search: true, Edit: true, Shell: true}
}
func (b *Backend) Open(ctx context.Context, sandbox sandboxdomain.Sandbox, run sandboxdomain.Run) (sandboxapplication.Session, error) {
	if b == nil || b.launcher == nil {
		return nil, sandboxdomain.ErrBackendUnavailable
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := sandbox.Validate(); err != nil {
		return nil, err
	}
	if sandbox.Backend() != sandboxdomain.BackendLocalWorkspace {
		return nil, sandboxdomain.ErrBackendUnavailable
	}
	if run.SandboxID != sandbox.ID() || run.State != sandboxdomain.StateReady || run.Generation == 0 || run.ExternalID == "" {
		return nil, sandboxdomain.ErrConflict
	}
	if sandbox.Isolation() != sandboxdomain.IsolationHost && sandbox.Isolation() != sandboxdomain.IsolationRestrictedHost {
		return nil, sandboxdomain.ErrBackendUnavailable
	}
	if err := b.launcher.CheckIsolation(sandbox.Isolation()); err != nil {
		return nil, fmt.Errorf("open local workspace: required process isolation unavailable: %w", err)
	}
	root := sandbox.Spec().LocalRoot
	resolved, err := filepath.EvalSymlinks(root)
	if err != nil {
		return nil, fmt.Errorf("open local workspace: canonical root: %w", err)
	}
	if filepath.Clean(resolved) != root {
		return nil, fmt.Errorf("open local workspace: root is not canonical")
	}
	info, err := os.Stat(root)
	if err != nil {
		return nil, fmt.Errorf("open local workspace: root: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("open local workspace: root is not a directory")
	}
	resolver, err := newWorkspacePathResolver(root)
	if err != nil {
		return nil, err
	}
	fsRoot, err := os.OpenRoot(root)
	if err != nil {
		return nil, fmt.Errorf("open local workspace root: %w", err)
	}
	return &Instance{root: root, fsRoot: fsRoot, resolver: resolver, shellLauncher: b.launcher, mode: sandbox.Isolation()}, nil
}

// Instance owns one sandbox filesystem handle, and can only be acquired by the
// outer Execution Router. It does not contain Policy or Principal authority.
type Instance struct {
	root          string
	fsRoot        *os.Root
	resolver      workspacePathResolver
	shellLauncher ShellLauncher
	mode          sandboxdomain.IsolationClass
}

func (i *Instance) ExecuteWorkspace(ctx context.Context, capability sandboxdomain.Capability, arguments invocationdomain.JSONValue) (invocationdomain.Outcome, error) {
	if i == nil || i.fsRoot == nil {
		return invocationdomain.Outcome{}, sandboxdomain.ErrBackendUnavailable
	}
	if err := ctx.Err(); err != nil {
		return invocationdomain.Outcome{}, err
	}
	switch capability {
	case sandboxdomain.CapabilityRead:
		return i.executeRead(ctx, arguments)
	case sandboxdomain.CapabilitySearch:
		return i.executeSearch(ctx, arguments)
	case sandboxdomain.CapabilityEdit:
		return i.executeEdit(ctx, arguments)
	case sandboxdomain.CapabilityShell:
		return i.executeShell(ctx, arguments)
	default:
		return invocationdomain.Outcome{}, sandboxdomain.ErrCapabilityUnavailable
	}
}
func (i *Instance) Close() error {
	if i == nil || i.fsRoot == nil {
		return nil
	}
	root := i.fsRoot
	i.fsRoot = nil
	return root.Close()
}

func completeJSON(value any) (invocationdomain.Outcome, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return invocationdomain.Outcome{}, fmt.Errorf("encode Workspace result: %w", err)
	}
	content, err := invocationdomain.NewJSONValue(raw)
	if err != nil {
		return invocationdomain.Outcome{}, fmt.Errorf("normalize Workspace result: %w", err)
	}
	return invocationdomain.Complete(content)
}
func toolFailure(err error) (invocationdomain.Outcome, error) {
	if err == nil {
		err = errors.New("workspace tool failed")
	}
	message := err.Error()
	const maxToolFailureRunes = 1800
	if utf8.RuneCountInString(message) > maxToolFailureRunes {
		runes := []rune(message)
		message = string(runes[:maxToolFailureRunes]) + "..."
	}
	return invocationdomain.Failed(invocationdomain.FailureSourceTool, invocationdomain.FailureToolError, message)
}
func decodeStrict(raw []byte, target any, label string) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("%s: %w", label, err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return fmt.Errorf("%s: multiple JSON values", label)
		}
		return fmt.Errorf("%s: trailing data: %w", label, err)
	}
	return nil
}

var _ sandboxapplication.Backend = (*Backend)(nil)
var _ sandboxapplication.WorkspaceSession = (*Instance)(nil)
