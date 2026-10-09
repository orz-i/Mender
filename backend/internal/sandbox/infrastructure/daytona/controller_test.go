package daytona

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	gov "github.com/orz-i/Mender/backend/internal/governance/domain"
	providerconfigs "github.com/orz-i/Mender/backend/internal/providerplugin/configstore"
	sand "github.com/orz-i/Mender/backend/internal/sandbox/domain"
	sandsqlite "github.com/orz-i/Mender/backend/internal/sandbox/infrastructure/sqlite"
)

type fakeRemote struct {
	info                               RemoteSnapshot
	root                               string
	files                              map[string][]byte
	execCalls, stops, starts, destroys int
	failStop                           bool
}

func (s *fakeRemote) Snapshot(context.Context) (RemoteSnapshot, error) { return s.info, nil }
func (s *fakeRemote) WorkingDir(context.Context) (string, error)       { return s.root, nil }
func (s *fakeRemote) Exec(_ context.Context, command, cwd string, _ time.Duration) (RemoteExec, error) {
	s.execCalls++
	if strings.Contains(command, "/outside") || strings.Contains(command, "/escape") {
		return RemoteExec{ExitCode: 1}, nil
	}
	// Model only fixed, non-Tool-authored file commit commands. No local shell.
	parts := regexp.MustCompile(`'([^']+)'`).FindAllStringSubmatch(command, -1)
	quoted := make([]string, 0, len(parts))
	for _, part := range parts {
		quoted = append(quoted, part[1])
	}
	if strings.Contains(command, " && ln ") && len(quoted) >= 4 {
		stage, target := quoted[len(quoted)-2], quoted[len(quoted)-1]
		if _, exists := s.files[target]; exists {
			return RemoteExec{ExitCode: 1}, nil
		}
		s.files[target] = append([]byte(nil), s.files[stage]...)
	}
	if strings.Contains(command, " && mv -f -- ") && len(quoted) >= 4 {
		stage, target := quoted[len(quoted)-2], quoted[len(quoted)-1]
		s.files[target] = append([]byte(nil), s.files[stage]...)
	}
	if strings.Contains(command, " && rm -- ") && len(quoted) >= 1 {
		delete(s.files, quoted[len(quoted)-1])
	}
	if strings.Contains(command, "rm -f -- ") && len(quoted) >= 1 {
		delete(s.files, quoted[len(quoted)-1])
	}
	return RemoteExec{ExitCode: 0, Output: "approved remote execution"}, nil
}
func (s *fakeRemote) FileInfo(_ context.Context, path string) (RemoteFile, error) {
	data, ok := s.files[path]
	if !ok {
		return RemoteFile{}, sand.ErrNotFound
	}
	return RemoteFile{Path: path, Size: int64(len(data))}, nil
}
func (s *fakeRemote) ListFiles(_ context.Context, p string, _ int) ([]RemoteFile, error) {
	out := make([]RemoteFile, 0)
	for k, v := range s.files {
		if strings.HasPrefix(k, p+"/") {
			out = append(out, RemoteFile{Path: k, Size: int64(len(v))})
		}
	}
	return out, nil
}
func (s *fakeRemote) ReadStream(_ context.Context, path string) (io.ReadCloser, error) {
	content, ok := s.files[path]
	if !ok {
		return nil, sand.ErrNotFound
	}
	return io.NopCloser(strings.NewReader(string(content))), nil
}
func (s *fakeRemote) UploadStream(_ context.Context, source io.Reader, target string) error {
	data, err := io.ReadAll(io.LimitReader(source, 4*1024*1024+1))
	if err != nil {
		return err
	}
	s.files[target] = data
	return nil
}
func (s *fakeRemote) Move(_ context.Context, from, to string) error {
	value, ok := s.files[from]
	if !ok {
		return sand.ErrNotFound
	}
	s.files[to] = value
	delete(s.files, from)
	return nil
}
func (s *fakeRemote) DeleteFile(_ context.Context, p string) error { delete(s.files, p); return nil }
func (s *fakeRemote) Start(context.Context) error                  { s.starts++; s.info.State = "started"; return nil }
func (s *fakeRemote) Stop(context.Context) error {
	s.stops++
	if s.failStop {
		return fmt.Errorf("provider socket failed with potentially applied operation")
	}
	s.info.State = "stopped"
	return nil
}
func (s *fakeRemote) Destroy(context.Context) error {
	s.destroys++
	s.info.State = "destroyed"
	return nil
}
func (s *fakeRemote) SetLabels(_ context.Context, labels map[string]string) error {
	s.info.Labels = labels
	return nil
}

type fakeRemoteClient struct {
	lookupName                                   string
	hideLabelList                                bool
	remote                                       *fakeRemote
	createCalls, getCalls, findCalls, closeCalls int
	failCreate                                   bool
}

func (c *fakeRemoteClient) Create(_ context.Context, t Template, name string, labels map[string]string) (RemoteSandbox, error) {
	c.createCalls++
	if c.failCreate {
		return nil, fmt.Errorf("possible remote side effect after socket failure")
	}
	c.remote.info = RemoteSnapshot{ID: "remote-sandbox-123", State: "started", Labels: labels, CPU: t.CPU, MemoryGiB: t.MemoryGiB, DiskGiB: t.DiskGiB, Target: t.Target, NetworkBlocked: true, TTLActive: true}
	return c.remote, nil
}
func (c *fakeRemoteClient) Get(_ context.Context, id string) (RemoteSandbox, error) {
	c.getCalls++
	if c.remote.info.ID != id && id != c.lookupName {
		return nil, sand.ErrNotFound
	}
	return c.remote, nil
}
func (c *fakeRemoteClient) FindByLabel(_ context.Context, labels map[string]string) ([]RemoteSandbox, error) {
	c.findCalls++
	if c.hideLabelList {
		return nil, nil
	}
	if c.remote.info.ID == "" {
		return nil, nil
	}
	for k, v := range labels {
		if c.remote.info.Labels[k] != v {
			return nil, nil
		}
	}
	return []RemoteSandbox{c.remote}, nil
}
func (c *fakeRemoteClient) Close(context.Context) error { c.closeCalls++; return nil }

type fakeRemoteFactory struct {
	client       *fakeRemoteClient
	connectCalls int
	fail         bool
}

func (f *fakeRemoteFactory) Connect(context.Context, Template) (RemoteClient, error) {
	f.connectCalls++
	if f.fail {
		return nil, sand.ErrNotAuthorized
	}
	return f.client, nil
}

type fakeActionGate struct {
	calls   int
	deny    bool
	actions []gov.Action
}

func (g *fakeActionGate) AdmitAction(_ context.Context, caller gov.Context, item sand.Sandbox, action gov.Action) error {
	g.calls++
	g.actions = append(g.actions, action)
	if err := caller.Validate(); err != nil {
		return err
	}
	if item.Backend() != sand.BackendDaytona || g.deny {
		return sand.ErrNotAuthorized
	}
	return nil
}
func remoteTestFixture(t *testing.T) (context.Context, *Controller, *sandsqlite.Store, *TemplateStore, *fakeRemoteFactory, *fakeActionGate, gov.Context, sand.ID) {
	t.Helper()
	ctx := context.Background()
	db := filepath.Join(t.TempDir(), "daytona-runtime.db")
	runs, err := sandsqlite.Open(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runs.Close() })
	// Lifecycle CAS needs the Provider config table in the shared DB.
	providers, err := providerconfigs.Open(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = providers.Close() })
	templates, err := OpenTemplateStore(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = templates.Close() })
	id := sand.ID("sbx_11111111111111111111111111111111")
	if err := templates.CreateDefinedSandbox(ctx, testDaytonaSandbox(t, id, "owner"), validTemplate()); err != nil {
		t.Fatal(err)
	}
	subject, err := gov.NewPrincipal(gov.PrincipalLocalUser, "owner")
	if err != nil {
		t.Fatal(err)
	}
	caller, err := gov.NewContext(gov.OriginCLI, subject, gov.CatalogScope())
	if err != nil {
		t.Fatal(err)
	}
	client := &fakeRemoteClient{remote: &fakeRemote{root: "/home/remote", files: map[string][]byte{"/home/remote/notes.txt": []byte("hello daytona\n")}}}
	factory := &fakeRemoteFactory{client: client}
	gate := &fakeActionGate{}
	controller, err := NewController(runs, templates, factory, gate)
	if err != nil {
		t.Fatal(err)
	}
	return ctx, controller, runs, templates, factory, gate, caller, id
}
func TestDaytonaProvisionPolicyBeforeCredentialsAndRemoteCalls(t *testing.T) {
	ctx, controller, runs, _, factory, gate, caller, id := remoteTestFixture(t)
	gate.deny = true
	if _, err := controller.Provision(ctx, caller, id); !errors.Is(err, sand.ErrNotAuthorized) {
		t.Fatalf("denied: %v", err)
	}
	if factory.connectCalls != 0 || factory.client.createCalls != 0 {
		t.Fatal("denied provisioning reached Mender Credential or Daytona")
	}
	run, err := runs.GetRun(ctx, id)
	if err != nil || run.State != sand.StateDefined {
		t.Fatalf("denied changed Run %+v %v", run, err)
	}
	gate.deny = false
	started, err := controller.Provision(ctx, caller, id)
	if err != nil || started.State != sand.StateReady || started.ExternalID != "remote-sandbox-123" {
		t.Fatalf("provision %+v %v", started, err)
	}
	if gate.calls != 2 || factory.connectCalls != 1 || factory.client.createCalls != 1 {
		t.Fatal("unexpected repeated call counts")
	}
	if _, err := controller.Provision(ctx, caller, id); !errors.Is(err, sand.ErrConflict) {
		t.Fatalf("ready Run retried: %v", err)
	}
	if factory.client.createCalls != 1 {
		t.Fatal("second remote creation")
	}
}
func TestDaytonaUnknownCreateDoesNotRetryButReconcilesReadOnly(t *testing.T) {
	ctx, controller, runs, _, factory, _, caller, id := remoteTestFixture(t)
	factory.client.failCreate = true
	if _, err := controller.Provision(ctx, caller, id); err == nil {
		t.Fatal("accepted ambiguous create")
	}
	run, err := runs.GetRun(ctx, id)
	if err != nil || run.State != sand.StateUnknown {
		t.Fatalf("unknown not persisted %+v %v", run, err)
	}
	if _, err := controller.Provision(ctx, caller, id); !errors.Is(err, sand.ErrConflict) {
		t.Fatalf("ambiguous create replayed: %v", err)
	}
	if factory.client.createCalls != 1 {
		t.Fatal("duplicate billable Create")
	}
	// Model a remotely created instance whose reply was lost.
	tplt := validTemplate()
	factory.client.failCreate = false
	factory.client.remote.info = RemoteSnapshot{ID: "remote-sandbox-123", State: "started", Labels: remoteLabels(id, 1), CPU: tplt.CPU, MemoryGiB: tplt.MemoryGiB, DiskGiB: tplt.DiskGiB, Target: tplt.Target, NetworkBlocked: true, TTLActive: true}
	recovered, err := controller.Reconcile(ctx, caller, id)
	if err != nil || recovered.State != sand.StateReady {
		t.Fatalf("read-only recover %+v %v", recovered, err)
	}
	if factory.client.createCalls != 1 || factory.client.findCalls != 1 {
		t.Fatal("reconcile modified a resource")
	}
}
func TestDaytonaLifecycleFencedAuditedWithRemoteLabels(t *testing.T) {
	ctx, controller, runs, _, factory, gate, caller, id := remoteTestFixture(t)
	created, err := controller.Provision(ctx, caller, id)
	if err != nil {
		t.Fatal(err)
	}
	stopped, err := controller.ChangeLifecycle(ctx, caller, id, gov.ActionSandboxStop)
	if err != nil || stopped.State != sand.StateStopped {
		t.Fatalf("remote stop %+v %v", stopped, err)
	}
	started, err := controller.ChangeLifecycle(ctx, caller, id, gov.ActionSandboxStart)
	if err != nil || started.State != sand.StateReady || started.Generation != 2 {
		t.Fatalf("remote restart %+v %v", started, err)
	}
	if started.ExternalID != created.ExternalID || factory.client.remote.info.Labels[labelGeneration] != "2" {
		t.Fatal("stale remote generation label")
	}
	destroyed, err := controller.ChangeLifecycle(ctx, caller, id, gov.ActionSandboxDestroy)
	if err != nil || destroyed.State != sand.StateDestroyed {
		t.Fatalf("remote destroyed %+v %v", destroyed, err)
	}
	if factory.client.remote.destroys != 1 || factory.client.remote.stops != 1 || factory.client.remote.starts != 1 {
		t.Fatal("lifecycle side effects repeated")
	}
	if len(gate.actions) != 4 {
		t.Fatalf("policy admission count %v", gate.actions)
	}
	events, err := runs.ListLifecycleEvents(ctx, id, 5)
	if err != nil || len(events) != 3 {
		t.Fatalf("missing lifecycle audit %+v %v", events, err)
	}
	if _, err := controller.ChangeLifecycle(ctx, caller, id, gov.ActionSandboxStart); !errors.Is(err, sand.ErrConflict) {
		t.Fatalf("tombstone restarted: %v", err)
	}
}
func TestDaytonaNetworkOrTTLDriftFailsClosed(t *testing.T) {
	ctx, controller, runs, _, factory, _, caller, id := remoteTestFixture(t)
	factory.client.remote.info = RemoteSnapshot{ID: "remote-sandbox-123", State: "started", Labels: remoteLabels(id, 1), CPU: 1, MemoryGiB: 2, DiskGiB: 3, Target: TargetUS, NetworkBlocked: false, TTLActive: true}
	previous, err := runs.GetRun(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	unknown, err := runs.TransitionRun(ctx, previous, sand.StateProvisioning, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runs.TransitionRun(ctx, unknown, sand.StateUnknown, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := controller.Reconcile(ctx, caller, id); !errors.Is(err, sand.ErrBackendUnavailable) {
		t.Fatalf("networkless claim accepted %v", err)
	}
	factory.client.remote.info.NetworkBlocked = true
	factory.client.remote.info.TTLActive = false
	if _, err := controller.Reconcile(ctx, caller, id); !errors.Is(err, sand.ErrBackendUnavailable) {
		t.Fatalf("missing TTL accepted %v", err)
	}
}

// When the vendor's label-search index lags a successful remote Create,
// recovery also checks Mender's deterministic name. The name is only a
// locator; verified labels and pinned constraints remain mandatory.
func TestDaytonaReconcileUnknownFallsBackToExactNameWithoutMutation(t *testing.T) {
	ctx, controller, runs, _, factory, _, caller, id := remoteTestFixture(t)
	old, err := runs.GetRun(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	stage, err := runs.TransitionRun(ctx, old, sand.StateProvisioning, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runs.TransitionRun(ctx, stage, sand.StateUnknown, ""); err != nil {
		t.Fatal(err)
	}
	template := validTemplate()
	factory.client.remote.info = RemoteSnapshot{
		ID: "remote-sandbox-123", State: "started", Labels: remoteLabels(id, 1),
		CPU: template.CPU, MemoryGiB: template.MemoryGiB, DiskGiB: template.DiskGiB,
		Target: template.Target, NetworkBlocked: true, TTLActive: true,
	}
	factory.client.lookupName = remoteName(id, 1)
	factory.client.hideLabelList = true
	result, err := controller.Reconcile(ctx, caller, id)
	if err != nil || result.State != sand.StateReady {
		t.Fatalf("name-based recovery failed: %v, %+v", err, result)
	}
	if factory.client.findCalls != 1 || factory.client.getCalls != 1 || factory.client.createCalls != 0 {
		t.Fatalf("read-only recovery used incorrect operations find=%d get=%d create=%d", factory.client.findCalls, factory.client.getCalls, factory.client.createCalls)
	}
}
func TestDaytonaNameReconcileRejectsWrongOwnershipLabels(t *testing.T) {
	ctx, controller, runs, _, factory, _, caller, id := remoteTestFixture(t)
	old, err := runs.GetRun(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	stage, err := runs.TransitionRun(ctx, old, sand.StateProvisioning, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runs.TransitionRun(ctx, stage, sand.StateUnknown, ""); err != nil {
		t.Fatal(err)
	}
	template := validTemplate()
	labels := remoteLabels(id, 1)
	labels[labelSandboxID] = "sbx_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	factory.client.remote.info = RemoteSnapshot{ID: "foreign-remote", State: "started", Labels: labels,
		CPU: template.CPU, MemoryGiB: template.MemoryGiB, DiskGiB: template.DiskGiB,
		Target: template.Target, NetworkBlocked: true, TTLActive: true,
	}
	factory.client.lookupName = remoteName(id, 1)
	factory.client.hideLabelList = true
	if _, err := controller.Reconcile(ctx, caller, id); !errors.Is(err, sand.ErrBackendUnavailable) {
		t.Fatalf("foreign ownership verified from name only: %v", err)
	}
	after, err := runs.GetRun(ctx, id)
	if err != nil || after.State != sand.StateUnknown {
		t.Fatalf("foreign resource marked ready: %+v %v", after, err)
	}
}
