package bootstrap

import (
	"context"
	"errors"
	"testing"

	sandboxapp "github.com/orz-i/Mender/backend/internal/sandbox/application"
	sandboxdomain "github.com/orz-i/Mender/backend/internal/sandbox/domain"
	sandboxdocker "github.com/orz-i/Mender/backend/internal/sandbox/infrastructure/dockerengine"
	sandboxpluginregistry "github.com/orz-i/Mender/backend/internal/sandbox/infrastructure/pluginregistry"
	workspaceapp "github.com/orz-i/Mender/backend/internal/workspace/application"
	workspace "github.com/orz-i/Mender/backend/internal/workspace/domain"
)

type dockerGateFixtureBackend struct {
	opened  int
	session *dockerGateFixtureSession
}

func (*dockerGateFixtureBackend) Kind() sandboxdomain.BackendKind { return sandboxdomain.BackendDocker }
func (*dockerGateFixtureBackend) Capabilities() sandboxdomain.Capabilities {
	return sandboxdomain.Capabilities{Read: true, Search: true, Edit: true, Shell: true}
}
func (b *dockerGateFixtureBackend) Open(context.Context, sandboxdomain.Sandbox, sandboxdomain.Run) (sandboxapp.Session, error) {
	b.opened++
	return b.session, nil
}

type dockerGateFixtureSession struct{ executed, closed int }

func (s *dockerGateFixtureSession) Close() error { s.closed++; return nil }
func (s *dockerGateFixtureSession) Read(context.Context, workspace.ReadRequest) (workspace.ReadResult, error) {
	s.executed++
	return workspace.ReadResult{}, nil
}
func (s *dockerGateFixtureSession) Search(context.Context, workspace.SearchRequest) (workspace.SearchResult, error) {
	s.executed++
	return workspace.SearchResult{}, nil
}
func (s *dockerGateFixtureSession) Edit(context.Context, workspace.EditRequest) (workspace.EditResult, error) {
	s.executed++
	return workspace.EditResult{}, nil
}
func (s *dockerGateFixtureSession) Shell(context.Context, workspace.ShellRequest) (workspace.ShellResult, error) {
	s.executed++
	return workspace.ShellResult{}, nil
}

func TestRegisteredDockerBackendFailsClosedOnLossAndGatesAlreadyOpenSession(t *testing.T) {
	t.Parallel()
	controller := &sandboxdocker.Provisioner{}
	s := &Sandboxes{
		docker: controller, dockerTemplates: &sandboxdocker.TemplateStore{}, dockerReady: true,
		backendPlugins: sandboxpluginregistry.NewRegistry(),
	}
	session := &dockerGateFixtureSession{}
	concrete := &dockerGateFixtureBackend{session: session}
	backend := dockerBackendGate{owner: s, backend: concrete}
	descriptor, err := sandboxpluginregistry.BuiltinDescriptor(backend)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.backendPlugins.Register(backend, descriptor); err != nil {
		t.Fatal(err)
	}
	item, err := sandboxdomain.NewSandbox(sandboxdomain.Spec{
		ID: "sbx_0123456789abcdef0123456789abcdef", Name: "Bound Docker Sandbox",
		Backend:   sandboxdomain.BackendDocker,
		Binding:   sandboxdomain.BuiltinBinding(sandboxdomain.BackendDocker),
		Isolation: sandboxdomain.IsolationContainer, OwnerKind: "local_user", OwnerRef: "fixture",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !s.BackendAvailable(item) {
		t.Fatal("healthy registered Docker backend unavailable")
	}
	resolved, err := s.backendPlugins.ResolveBackend(sandboxdomain.BackendDocker, item.Spec().Binding, sandboxdomain.CapabilityRead)
	if err != nil {
		t.Fatal(err)
	}
	ws, err := resolved.Open(context.Background(), item, sandboxdomain.Run{})
	if err != nil {
		t.Fatal(err)
	}
	work, ok := ws.(workspaceapp.Session)
	if !ok {
		t.Fatal("Docker execution session was not gated")
	}
	operations := map[string]func() error{}
	operations["read"] = func() error {
		_, err := work.Read(context.Background(), workspaceapp.NormalizeRead(workspace.ReadRequest{}))
		return err
	}
	operations["search"] = func() error {
		_, err := work.Search(context.Background(), workspaceapp.NormalizeSearch(workspace.SearchRequest{}))
		return err
	}
	operations["edit"] = func() error {
		_, err := work.Edit(context.Background(), workspaceapp.NormalizeEdit(workspace.EditRequest{}))
		return err
	}
	operations["shell"] = func() error {
		_, err := work.Shell(context.Background(), workspaceapp.NormalizeShell(workspace.ShellRequest{}))
		return err
	}
	for name, operation := range operations {
		if err := operation(); err != nil {
			t.Fatalf("ready Engine %s execution rejected: %v", name, err)
		}
	}
	s.updateDockerHealth(controller, errors.New("offline"))
	if s.BackendAvailable(item) {
		t.Fatal("registered backend leaked health as availability")
	}
	if _, err := resolved.Open(context.Background(), item, sandboxdomain.Run{}); !errors.Is(err, sandboxdomain.ErrBackendUnavailable) {
		t.Fatalf("offline Engine still acquired: %v", err)
	}
	for name, operation := range operations {
		if err := operation(); !errors.Is(err, sandboxdomain.ErrBackendUnavailable) {
			t.Fatalf("offline Engine still executed existing %s session: %v", name, err)
		}
	}
	if concrete.opened != 1 || session.executed != len(operations) {
		t.Fatalf("unhealthy backend executed: opened %d, executed %d", concrete.opened, session.executed)
	}
	s.updateDockerHealth(controller, nil)
	if !s.BackendAvailable(item) {
		t.Fatal("same-Engine recovery not reflected in bound Sandbox")
	}
	for name, operation := range operations {
		if err := operation(); err != nil {
			t.Fatalf("recovered Engine %s execution rejected: %v", name, err)
		}
	}
	if session.executed != 2*len(operations) {
		t.Fatalf("recovered Engine execution count %d", session.executed)
	}
	if err := ws.Close(); err != nil || session.closed != 1 {
		t.Fatalf("session cleanup lost: %v, count %d", err, session.closed)
	}
}
