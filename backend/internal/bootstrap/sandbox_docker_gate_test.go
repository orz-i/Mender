package bootstrap

import (
	"context"
	"errors"
	"testing"

	invocationdomain "github.com/orz-i/Mender/backend/internal/invocation/domain"
	sandboxapp "github.com/orz-i/Mender/backend/internal/sandbox/application"
	sandboxdomain "github.com/orz-i/Mender/backend/internal/sandbox/domain"
	sandboxdocker "github.com/orz-i/Mender/backend/internal/sandbox/infrastructure/dockerengine"
	sandboxpluginregistry "github.com/orz-i/Mender/backend/internal/sandbox/infrastructure/pluginregistry"
)

type dockerGateFixtureBackend struct {
	opened  int
	session *dockerGateFixtureSession
}

func (*dockerGateFixtureBackend) Kind() sandboxdomain.BackendKind { return sandboxdomain.BackendDocker }
func (*dockerGateFixtureBackend) Capabilities() sandboxdomain.Capabilities {
	return sandboxdomain.Capabilities{Read: true}
}
func (b *dockerGateFixtureBackend) Open(context.Context, sandboxdomain.Sandbox, sandboxdomain.Run) (sandboxapp.Session, error) {
	b.opened++
	return b.session, nil
}

type dockerGateFixtureSession struct{ executed, closed int }

func (s *dockerGateFixtureSession) Close() error { s.closed++; return nil }
func (s *dockerGateFixtureSession) ExecuteWorkspace(context.Context, sandboxdomain.Capability, invocationdomain.JSONValue) (invocationdomain.Outcome, error) {
	s.executed++
	return invocationdomain.Outcome{}, nil
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
	work, ok := ws.(sandboxapp.WorkspaceSession)
	if !ok {
		t.Fatal("Docker execution session was not gated")
	}
	if _, err := work.ExecuteWorkspace(context.Background(), sandboxdomain.CapabilityRead, invocationdomain.JSONValue{}); err != nil {
		t.Fatalf("ready Engine execution rejected: %v", err)
	}
	s.updateDockerHealth(controller, errors.New("offline"))
	if s.BackendAvailable(item) {
		t.Fatal("registered backend leaked health as availability")
	}
	if _, err := resolved.Open(context.Background(), item, sandboxdomain.Run{}); !errors.Is(err, sandboxdomain.ErrBackendUnavailable) {
		t.Fatalf("offline Engine still acquired: %v", err)
	}
	if _, err := work.ExecuteWorkspace(context.Background(), sandboxdomain.CapabilityRead, invocationdomain.JSONValue{}); !errors.Is(err, sandboxdomain.ErrBackendUnavailable) {
		t.Fatalf("offline Engine still executed existing session: %v", err)
	}
	if concrete.opened != 1 || session.executed != 1 {
		t.Fatalf("unhealthy backend executed: opened %d, executed %d", concrete.opened, session.executed)
	}
	s.updateDockerHealth(controller, nil)
	if !s.BackendAvailable(item) {
		t.Fatal("same-Engine recovery not reflected in bound Sandbox")
	}
	if _, err := work.ExecuteWorkspace(context.Background(), sandboxdomain.CapabilityRead, invocationdomain.JSONValue{}); err != nil {
		t.Fatalf("recovered Engine execution rejected: %v", err)
	}
	if err := ws.Close(); err != nil || session.closed != 1 {
		t.Fatalf("session cleanup lost: %v, count %d", err, session.closed)
	}
}
