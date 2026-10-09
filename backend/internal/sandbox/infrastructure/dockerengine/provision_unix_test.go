//go:build !windows

package dockerengine

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/moby/moby/api/types/container"
	apiimage "github.com/moby/moby/api/types/image"
	mobyclient "github.com/moby/moby/client"
	governancedomain "github.com/orz-i/Mender/backend/internal/governance/domain"
	sandboxdomain "github.com/orz-i/Mender/backend/internal/sandbox/domain"
	sandboxsqlite "github.com/orz-i/Mender/backend/internal/sandbox/infrastructure/sqlite"
)

type fakeTemplateLookup struct {
	id       sandboxdomain.ID
	template Template
	calls    int
	err      error
}

func (t *fakeTemplateLookup) GetDockerTemplate(_ context.Context, id sandboxdomain.ID) (Template, error) {
	t.calls++
	if id != t.id {
		return Template{}, sandboxdomain.ErrNotFound
	}
	if t.err != nil {
		return Template{}, t.err
	}
	return t.template, nil
}

type fakeActionAdmitter struct {
	calls  int
	denied bool
}

func (a *fakeActionAdmitter) AdmitAction(_ context.Context, caller governancedomain.Context, sandbox sandboxdomain.Sandbox, action governancedomain.Action) error {
	a.calls++
	if err := caller.Validate(); err != nil {
		return err
	}
	if (action != governancedomain.ActionSandboxProvision && action != governancedomain.ActionSandboxStart && action != governancedomain.ActionSandboxStop && action != governancedomain.ActionSandboxDestroy) || sandbox.Backend() != sandboxdomain.BackendDocker {
		return sandboxdomain.ErrNotAuthorized
	}
	if a.denied {
		return sandboxdomain.ErrNotAuthorized
	}
	return nil
}

type fakeMutatingDaemon struct {
	*probeFake
	plan                                  ContainerPlan
	created                               bool
	started                               bool
	createCalls, startCalls, inspectCalls int
	createErr, startErr, inspectErr       error
	tamper                                bool
}

func (d *fakeMutatingDaemon) ContainerCreate(_ context.Context, opts mobyclient.ContainerCreateOptions) (mobyclient.ContainerCreateResult, error) {
	d.createCalls++
	if opts.Name != d.plan.Name || opts.Config.Image != d.plan.Template.ImageDigest || opts.HostConfig.Privileged {
		return mobyclient.ContainerCreateResult{}, fmt.Errorf("unsafe create options")
	}
	d.created = true
	if d.createErr != nil {
		return mobyclient.ContainerCreateResult{}, d.createErr
	}
	return mobyclient.ContainerCreateResult{ID: strings.Repeat("f", 64)}, nil
}
func (d *fakeMutatingDaemon) ContainerStart(_ context.Context, id string, _ mobyclient.ContainerStartOptions) (mobyclient.ContainerStartResult, error) {
	d.startCalls++
	if id != strings.Repeat("f", 64) || !d.created {
		return mobyclient.ContainerStartResult{}, fmt.Errorf("missing created container")
	}
	// Even an error can arrive AFTER Docker actually started the process.
	d.started = true
	return mobyclient.ContainerStartResult{}, d.startErr
}
func (d *fakeMutatingDaemon) ContainerInspect(_ context.Context, id string, _ mobyclient.ContainerInspectOptions) (mobyclient.ContainerInspectResult, error) {
	d.inspectCalls++
	if d.inspectErr != nil {
		return mobyclient.ContainerInspectResult{}, d.inspectErr
	}
	if !d.created || id != d.plan.Name && id != strings.Repeat("f", 64) {
		return mobyclient.ContainerInspectResult{}, fmt.Errorf("not found")
	}
	snapshot := inspectedPlan(d.plan)
	if d.started {
		snapshot.State.Status = container.StateRunning
		snapshot.State.Running = true
	}
	if d.tamper {
		snapshot.HostConfig.NetworkMode = "host"
	}
	return mobyclient.ContainerInspectResult{Container: snapshot}, nil
}
func dockerProvisionFixture(t *testing.T) (*Provisioner, *sandboxsqlite.Store, *fakeMutatingDaemon, *fakeTemplateLookup, *fakeActionAdmitter, governancedomain.Context, sandboxdomain.ID) {
	t.Helper()
	ctx := context.Background()
	db := filepath.Join(t.TempDir(), "docker-staging.db")
	store, err := sandboxsqlite.Open(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	p, err := governancedomain.NewPrincipal(governancedomain.PrincipalLocalUser, "developer")
	if err != nil {
		t.Fatal(err)
	}
	caller, err := governancedomain.NewContext(governancedomain.OriginCLI, p, governancedomain.CatalogScope())
	if err != nil {
		t.Fatal(err)
	}
	id := sandboxdomain.ID("sbx_0123456789abcdef0123456789abcdef")
	item, err := sandboxdomain.NewSandbox(sandboxdomain.Spec{ID: id, Name: "Docker pending", Backend: sandboxdomain.BackendDocker, Isolation: sandboxdomain.IsolationContainer, OwnerKind: string(p.Kind()), OwnerRef: p.Reference()})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Create(ctx, item); err != nil {
		t.Fatal(err)
	}
	run, err := sandboxdomain.NewRun(id, 1, sandboxdomain.StateDefined, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.CreateRun(ctx, run); err != nil {
		t.Fatal(err)
	}
	inspector, probe := newProbeFixture(t)
	template := safePlan(t).Template
	probe.image = mobyclient.ImageInspectResult{InspectResponse: apiimage.InspectResponse{ID: "sha256:" + strings.Repeat("a", 64), RepoDigests: []string{immutableDigest}, Os: "linux", Architecture: "arm64"}}
	planned, err := sandboxdomain.NewRun(id, 1, sandboxdomain.StateProvisioning, "")
	if err != nil {
		t.Fatal(err)
	}
	plan, err := NewContainerPlan(item, planned, template, LocalImage{ImageID: probe.image.ID, Digest: immutableDigest, OSType: "linux", Architecture: "arm64"})
	if err != nil {
		t.Fatal(err)
	}
	daemon := &fakeMutatingDaemon{probeFake: probe, plan: plan}
	inspector.api = daemon
	lookup := &fakeTemplateLookup{id: id, template: template}
	gate := &fakeActionAdmitter{}
	controller := &Provisioner{observer: inspector, daemon: daemon, store: store, templates: lookup, admit: gate}
	return controller, store, daemon, lookup, gate, caller, id
}

func TestDockerProvisionerAdmitsStagesCreatesAndVerifiesOnce(t *testing.T) {
	provisioner, store, daemon, lookup, gate, caller, id := dockerProvisionFixture(t)
	done, err := provisioner.Provision(context.Background(), caller, id)
	if err != nil {
		t.Fatal(err)
	}
	if done.State != sandboxdomain.StateReady || done.Generation != 1 || done.ExternalID != strings.Repeat("f", 64) {
		t.Fatalf("provision %+v", done)
	}
	if gate.calls != 1 || lookup.calls != 1 || daemon.createCalls != 1 || daemon.startCalls != 1 || daemon.inspectCalls != 1 {
		t.Fatalf("unexpected calls gate=%d template=%d create=%d start=%d inspect=%d", gate.calls, lookup.calls, daemon.createCalls, daemon.startCalls, daemon.inspectCalls)
	}
	stored, err := store.GetRun(context.Background(), id)
	if err != nil || stored != done {
		t.Fatalf("durable run %+v %v", stored, err)
	}
	if _, err := provisioner.Provision(context.Background(), caller, id); !errors.Is(err, sandboxdomain.ErrConflict) {
		t.Fatalf("provision automatically retried: %v", err)
	}
	if daemon.createCalls != 1 {
		t.Fatal("duplicated container create")
	}
}
func TestDockerProvisionerDeniedBeforeEngineOrTemplate(t *testing.T) {
	provisioner, store, daemon, lookup, gate, caller, id := dockerProvisionFixture(t)
	gate.denied = true
	if _, err := provisioner.Provision(context.Background(), caller, id); !errors.Is(err, sandboxdomain.ErrNotAuthorized) {
		t.Fatalf("denial bypass: %v", err)
	}
	if lookup.calls != 0 || daemon.imageCalls != 0 || daemon.createCalls != 0 {
		t.Fatal("denied provisioning observed template or Engine")
	}
	run, err := store.GetRun(context.Background(), id)
	if err != nil || run.State != sandboxdomain.StateDefined {
		t.Fatalf("denied changed state %+v %v", run, err)
	}
}
func TestDockerAmbiguousCreateNeverRetriesOrSilentlyRecreates(t *testing.T) {
	controller, store, daemon, _, _, caller, id := dockerProvisionFixture(t)
	daemon.createErr = errors.New("POST /containers/create socket: connection reset with possible side effects")
	_, err := controller.Provision(context.Background(), caller, id)
	if !errors.Is(err, sandboxdomain.ErrBackendUnavailable) || strings.Contains(err.Error(), "socket: connection reset") {
		t.Fatalf("unsafe/unsanitized result: %v", err)
	}
	after, err := store.GetRun(context.Background(), id)
	if err != nil || after.State != sandboxdomain.StateUnknown {
		t.Fatalf("ambiguous state %+v %v", after, err)
	}
	if _, err := controller.Provision(context.Background(), caller, id); !errors.Is(err, sandboxdomain.ErrConflict) {
		t.Fatalf("recreated uncertain container: %v", err)
	}
	if daemon.createCalls != 1 {
		t.Fatal("duplicated create after transport error")
	}
	if _, err := controller.Reconcile(context.Background(), caller, id); !errors.Is(err, sandboxdomain.ErrBackendUnavailable) {
		t.Fatalf("created-only instance marked ready: %v", err)
	}
	if daemon.createCalls != 1 || daemon.startCalls != 0 {
		t.Fatal("reconciliation modified a container")
	}
}
func TestDockerReconcileRecoversOnlyIdenticalRunningContainer(t *testing.T) {
	controller, store, daemon, _, _, caller, id := dockerProvisionFixture(t)
	daemon.startErr = errors.New("start response lost after daemon already started")
	_, err := controller.Provision(context.Background(), caller, id)
	if !errors.Is(err, sandboxdomain.ErrBackendUnavailable) {
		t.Fatalf("expected unknown: %v", err)
	}
	uncertain, err := store.GetRun(context.Background(), id)
	if err != nil || uncertain.State != sandboxdomain.StateUnknown || uncertain.ExternalID != strings.Repeat("f", 64) {
		t.Fatalf("unknown %+v %v", uncertain, err)
	}
	// Another Runtime process can reconcile from the persisted Run; it does not
	// create/start an additional container or re-use the old generation.
	done, err := controller.Reconcile(context.Background(), caller, id)
	if err != nil || done.State != sandboxdomain.StateReady || done.ExternalID != strings.Repeat("f", 64) {
		t.Fatalf("read-only reconcile %+v %v", done, err)
	}
	if daemon.createCalls != 1 || daemon.startCalls != 1 {
		t.Fatal("reconciler performed mutating calls")
	}
}
func TestDockerInspectSecurityMismatchRetainsUnknownState(t *testing.T) {
	controller, store, daemon, _, _, caller, id := dockerProvisionFixture(t)
	daemon.tamper = true
	_, err := controller.Provision(context.Background(), caller, id)
	if !errors.Is(err, sandboxdomain.ErrBackendUnavailable) {
		t.Fatalf("unsafe Engine settings accepted: %v", err)
	}
	after, err := store.GetRun(context.Background(), id)
	if err != nil || after.State != sandboxdomain.StateUnknown {
		t.Fatalf("unsafe state %+v %v", after, err)
	}
	if _, err := controller.Reconcile(context.Background(), caller, id); !errors.Is(err, sandboxdomain.ErrBackendUnavailable) {
		t.Fatalf("tampered container recovered as ready: %v", err)
	}
	if daemon.createCalls != 1 || daemon.startCalls != 1 {
		t.Fatal("tampered container auto recreated")
	}
}
func TestDockerUnverifiedImageFailsWithoutRunStaging(t *testing.T) {
	controller, store, daemon, _, _, caller, id := dockerProvisionFixture(t)
	daemon.image.RepoDigests = []string{"unrelated@sha256:" + strings.Repeat("b", 64)}
	if _, err := controller.Provision(context.Background(), caller, id); !errors.Is(err, sandboxdomain.ErrBackendUnavailable) {
		t.Fatalf("unverified image: %v", err)
	}
	after, err := store.GetRun(context.Background(), id)
	if err != nil || after.State != sandboxdomain.StateDefined {
		t.Fatalf("unverified image changed Run %+v %v", after, err)
	}
	if daemon.createCalls != 0 {
		t.Fatal("image failure still created a container")
	}
}
func TestDockerControllerRequiresAllDependencies(t *testing.T) {
	_, err := NewProvisioner(EngineEndpoint{}, nil, nil, nil)
	if err == nil {
		t.Fatal("missing governance accepted")
	}
	// Verify no implicit Docker daemon socket defaults can be inferred from env.
	t.Setenv("DOCKER_HOST", "tcp://127.0.0.1:2375")
	if _, err := NewProvisioner(EngineEndpoint{}, &noopRunStore{}, fakeEmptyTemplate{}, &fakeActionAdmitter{}); err == nil {
		t.Fatal("zero socket silently chose environment Docker host")
	}
}

type noopRunStore struct{}

func (*noopRunStore) Get(context.Context, sandboxdomain.ID) (sandboxdomain.Sandbox, error) {
	return sandboxdomain.Sandbox{}, sandboxdomain.ErrNotFound
}
func (*noopRunStore) GetRun(context.Context, sandboxdomain.ID) (sandboxdomain.Run, error) {
	return sandboxdomain.Run{}, sandboxdomain.ErrNotFound
}
func (*noopRunStore) TransitionRun(context.Context, sandboxdomain.Run, sandboxdomain.State, string) (sandboxdomain.Run, error) {
	return sandboxdomain.Run{}, sandboxdomain.ErrConflict
}

type fakeEmptyTemplate struct{}

func (fakeEmptyTemplate) GetDockerTemplate(context.Context, sandboxdomain.ID) (Template, error) {
	return Template{}, sandboxdomain.ErrNotFound
}

func TestDockerUnknownRunReconcilesAfterRestartUsingPersistedTemplate(t *testing.T) {
	ctx, path, resources, templates, id, template := dockerTemplateFixture(t)
	if err := templates.CreateDockerTemplate(ctx, id, template); err != nil {
		t.Fatal(err)
	}
	inspector, fake := newProbeFixture(t)
	fake.image = mobyclient.ImageInspectResult{InspectResponse: apiimage.InspectResponse{
		ID: "sha256:" + strings.Repeat("a", 64), RepoDigests: []string{immutableDigest}, Os: "linux", Architecture: "arm64",
	}}
	item, err := resources.Get(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	staged, err := sandboxdomain.NewRun(id, 1, sandboxdomain.StateProvisioning, "")
	if err != nil {
		t.Fatal(err)
	}
	plan, err := NewContainerPlan(item, staged, template, LocalImage{ImageID: fake.image.ID, Digest: immutableDigest, OSType: "linux", Architecture: "arm64"})
	if err != nil {
		t.Fatal(err)
	}
	daemon := &fakeMutatingDaemon{probeFake: fake, plan: plan, startErr: errors.New("uncertain start result")}
	inspector.api = daemon
	gate := &fakeActionAdmitter{}
	owner, err := governancedomain.NewPrincipal(governancedomain.PrincipalLocalUser, "operator")
	if err != nil {
		t.Fatal(err)
	}
	caller, err := governancedomain.NewContext(governancedomain.OriginCLI, owner, governancedomain.CatalogScope())
	if err != nil {
		t.Fatal(err)
	}
	before := &Provisioner{observer: inspector, daemon: daemon, store: resources, templates: templates, admit: gate}
	if _, err := before.Provision(ctx, caller, id); !errors.Is(err, sandboxdomain.ErrBackendUnavailable) {
		t.Fatalf("expected ambiguous result: %v", err)
	}
	unknown, err := resources.GetRun(ctx, id)
	if err != nil || unknown.State != sandboxdomain.StateUnknown {
		t.Fatalf("unreconciled run %+v %v", unknown, err)
	}
	// Open independent repositories, mimicking a fresh Mender Runtime process.
	restartedRuns, err := sandboxsqlite.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = restartedRuns.Close() }()
	restartedTemplates, err := OpenTemplateStore(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = restartedTemplates.Close() }()
	after := &Provisioner{observer: inspector, daemon: daemon, store: restartedRuns, templates: restartedTemplates, admit: gate}
	ready, err := after.Reconcile(ctx, caller, id)
	if err != nil || ready.State != sandboxdomain.StateReady || ready.ExternalID != strings.Repeat("f", 64) {
		t.Fatalf("restart reconciliation %+v %v", ready, err)
	}
	if daemon.createCalls != 1 || daemon.startCalls != 1 || daemon.inspectCalls != 1 {
		t.Fatalf("reconciliation replayed side effects create=%d start=%d inspect=%d", daemon.createCalls, daemon.startCalls, daemon.inspectCalls)
	}
	reloadedTemplate, err := restartedTemplates.GetDockerTemplate(ctx, id)
	if err != nil || reloadedTemplate != template {
		t.Fatalf("restarted template %+v %v", reloadedTemplate, err)
	}
}
func TestDockerReconcileRejectsPreviouslyRecordedContainerIdentityMismatch(t *testing.T) {
	controller, store, daemon, _, _, caller, id := dockerProvisionFixture(t)
	previously, err := store.GetRun(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	staged, err := store.TransitionRun(context.Background(), previously, sandboxdomain.StateProvisioning, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.TransitionRun(context.Background(), staged, sandboxdomain.StateUnknown, strings.Repeat("b", 64)); err != nil {
		t.Fatal(err)
	}
	daemon.created = true
	daemon.started = true
	if _, err := controller.Reconcile(context.Background(), caller, id); !errors.Is(err, sandboxdomain.ErrConflict) {
		t.Fatalf("foreign container recovered: %v", err)
	}
	if daemon.createCalls != 0 || daemon.startCalls != 0 {
		t.Fatal("unsafe reconcile caused side effects")
	}
}

func (d *fakeMutatingDaemon) ContainerStop(_ context.Context, id string, _ mobyclient.ContainerStopOptions) (mobyclient.ContainerStopResult, error) {
	if id != strings.Repeat("f", 64) || !d.created {
		return mobyclient.ContainerStopResult{}, fmt.Errorf("foreign Docker ID")
	}
	d.started = false
	return mobyclient.ContainerStopResult{}, nil
}
func (d *fakeMutatingDaemon) ContainerRemove(_ context.Context, id string, _ mobyclient.ContainerRemoveOptions) (mobyclient.ContainerRemoveResult, error) {
	if id != strings.Repeat("f", 64) || !d.created {
		return mobyclient.ContainerRemoveResult{}, fmt.Errorf("foreign Docker ID")
	}
	d.created = false
	return mobyclient.ContainerRemoveResult{}, nil
}
