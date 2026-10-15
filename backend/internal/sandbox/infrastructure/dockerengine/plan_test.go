package dockerengine

import (
	"maps"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/mount"
	sandboxdomain "github.com/orz-i/Mender/backend/internal/sandbox/domain"
)

func safePlan(t *testing.T) ContainerPlan {
	t.Helper()
	id := sandboxdomain.ID("sbx_0123456789abcdef0123456789abcdef")
	item, err := sandboxdomain.NewSandbox(sandboxdomain.Spec{ID: id, Name: "Container sandbox", Backend: sandboxdomain.BackendDocker, Binding: sandboxdomain.BuiltinBinding(sandboxdomain.BackendDocker), Isolation: sandboxdomain.IsolationContainer, OwnerKind: "local_user", OwnerRef: "operator"})
	if err != nil {
		t.Fatal(err)
	}
	run, err := sandboxdomain.NewRun(id, 1, sandboxdomain.StateProvisioning, "")
	if err != nil {
		t.Fatal(err)
	}
	template := Template{ImageDigest: immutableDigest, UserID: 10001, GroupID: 10001, CPUmilli: 750, MemoryMiB: 1024, WorkspaceMiB: 256, PIDs: 64}
	image := LocalImage{ImageID: "sha256:" + strings.Repeat("a", 64), Digest: immutableDigest, OSType: "linux", Architecture: "arm64"}
	plan, err := NewContainerPlan(item, run, template, image)
	if err != nil {
		t.Fatal(err)
	}
	return plan
}

func inspectedPlan(plan ContainerPlan) container.InspectResponse {
	config := *plan.Options.Config
	config.Env = slices.Clone(config.Env)
	config.Labels = maps.Clone(config.Labels)
	config.Cmd = slices.Clone(config.Cmd)
	config.Entrypoint = slices.Clone(config.Entrypoint)
	originalHealthcheck := *config.Healthcheck
	originalHealthcheck.Test = slices.Clone(originalHealthcheck.Test)
	config.Healthcheck = &originalHealthcheck
	host := *plan.Options.HostConfig
	host.CapAdd = slices.Clone(host.CapAdd)
	host.CapDrop = slices.Clone(host.CapDrop)
	host.SecurityOpt = slices.Clone(host.SecurityOpt)
	host.Tmpfs = maps.Clone(host.Tmpfs)
	pids := *host.PidsLimit
	host.PidsLimit = &pids
	return container.InspectResponse{
		ID: strings.Repeat("f", 64), Name: "/" + plan.Name, Image: plan.Image.ImageID, Config: &config, HostConfig: &host,
		State: &container.State{Status: container.StateCreated},
	}
}

func TestContainerPlanPinsAllPrivilegeAndResourceControls(t *testing.T) {
	p := safePlan(t)
	o := p.Options
	if o.Name != p.Name || o.Config.Image != immutableDigest || o.HostConfig == nil || o.Config == nil {
		t.Fatal("plan incomplete")
	}
	if o.Config.User != "10001:10001" || o.Config.WorkingDir != "/workspace" || o.Config.Healthcheck == nil || !slices.Equal(o.Config.Healthcheck.Test, []string{"NONE"}) {
		t.Fatalf("user/root/healthcheck %v", o.Config)
	}
	if len(o.Config.Env) != 3 || !o.Config.NetworkDisabled || len(o.Config.ExposedPorts) != 0 || len(o.Config.Volumes) != 0 {
		t.Fatal("environment/network/exposures not constrained")
	}
	h := o.HostConfig
	if h.Privileged || !h.ReadonlyRootfs || h.NetworkMode != "none" || len(h.Binds) != 0 || len(h.Mounts) != 0 || len(h.PortBindings) != 0 || h.PublishAllPorts || h.LogConfig.Type != "none" {
		t.Fatalf("unsafe host configuration %+v", h)
	}
	if !slices.Equal(h.CapDrop, []string{"ALL"}) || !slices.Equal(h.SecurityOpt, []string{"no-new-privileges"}) {
		t.Fatal("privilege controls relaxed")
	}
	if h.Memory != 1<<30 || h.MemorySwap != 1<<30 || h.NanoCPUs != 750*1_000_000 || h.PidsLimit == nil || *h.PidsLimit != 64 {
		t.Fatalf("limits %+v", h.Resources)
	}
	if len(h.Tmpfs) != 2 || !strings.Contains(h.Tmpfs["/workspace"], "size=256m") || !strings.Contains(h.Tmpfs["/workspace"], "uid=10001") || !strings.Contains(h.Tmpfs["/tmp"], "size=64m") {
		t.Fatalf("tmpfs %+v", h.Tmpfs)
	}
	if o.Config.Labels[labelSandboxID] != p.SandboxID.String() || o.Config.Labels[labelGeneration] != "1" || o.Config.Labels[labelBackend] != "docker" {
		t.Fatalf("labels %+v", o.Config.Labels)
	}
	if !strings.Contains(p.Name, "-g1") || !strings.HasPrefix(p.Name, "mender-sbx-") {
		t.Fatalf("name %q", p.Name)
	}
	for _, state := range []container.ContainerState{container.StateCreated, container.StateRunning, container.StateExited} {
		snapshot := inspectedPlan(p)
		snapshot.State.Status = state
		if err := p.VerifyInspect(snapshot); err != nil {
			t.Fatalf("valid %s: %v", state, err)
		}
	}
}

func TestInspectRejectsEffectiveSecurityWeakening(t *testing.T) {
	plan := safePlan(t)
	cases := []struct {
		name   string
		mutate func(*container.InspectResponse)
	}{
		{"wrong identity", func(v *container.InspectResponse) { v.ID = "evil" }},
		{"wrong container name", func(v *container.InspectResponse) { v.Name = "/foreign" }},
		{"wrong image", func(v *container.InspectResponse) { v.Image = "sha256:" + strings.Repeat("b", 64) }},
		{"cross sandbox labels", func(v *container.InspectResponse) {
			v.Config.Labels[labelSandboxID] = "sbx_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
		}},
		{"stale generation", func(v *container.InspectResponse) { v.Config.Labels[labelGeneration] = "2" }},
		{"root user", func(v *container.InspectResponse) { v.Config.User = "0:0" }},
		{"unexpected entrypoint", func(v *container.InspectResponse) { v.Config.Entrypoint = []string{"/bin/sh", "-l"} }},
		{"additional environment", func(v *container.InspectResponse) { v.Config.Env = append(v.Config.Env, "HOST_TOKEN=private") }},
		{"image healthcheck inherited", func(v *container.InspectResponse) { v.Config.Healthcheck.Test = []string{"CMD", "curl", "example.com"} }},
		{"network allowed", func(v *container.InspectResponse) { v.HostConfig.NetworkMode = "bridge" }},
		{"container network enabled", func(v *container.InspectResponse) { v.Config.NetworkDisabled = false }},
		{"exposed ports", func(v *container.InspectResponse) { v.HostConfig.PublishAllPorts = true }},
		{"host bind", func(v *container.InspectResponse) { v.HostConfig.Binds = []string{"/host:/workspace"} }},
		{"daemon socket mount", func(v *container.InspectResponse) {
			v.HostConfig.Mounts = []mount.Mount{{Type: mount.TypeBind, Source: "/var/run/docker.sock", Target: "/var/run/docker.sock"}}
		}},
		{"privileged", func(v *container.InspectResponse) { v.HostConfig.Privileged = true }},
		{"writable root", func(v *container.InspectResponse) { v.HostConfig.ReadonlyRootfs = false }},
		{"host pid namespace", func(v *container.InspectResponse) { v.HostConfig.PidMode = "host" }},
		{"host cgroup namespace", func(v *container.InspectResponse) { v.HostConfig.CgroupnsMode = "host" }},
		{"host IPC namespace", func(v *container.InspectResponse) { v.HostConfig.IpcMode = "host" }},
		{"added caps", func(v *container.InspectResponse) { v.HostConfig.CapAdd = []string{"SYS_ADMIN"} }},
		{"caps retained", func(v *container.InspectResponse) { v.HostConfig.CapDrop = nil }},
		{"no new privileges disabled", func(v *container.InspectResponse) { v.HostConfig.SecurityOpt = []string{"no-new-privileges:false"} }},
		{"CPU unbounded", func(v *container.InspectResponse) { v.HostConfig.NanoCPUs = 0 }},
		{"memory raised", func(v *container.InspectResponse) { v.HostConfig.Memory *= 2 }},
		{"swap unbounded", func(v *container.InspectResponse) { v.HostConfig.MemorySwap = -1 }},
		{"PIDs unrestricted", func(v *container.InspectResponse) { v.HostConfig.PidsLimit = nil }},
		{"scratch unlimited", func(v *container.InspectResponse) { delete(v.HostConfig.Tmpfs, "/workspace") }},
		{"temp filesystem enlarged", func(v *container.InspectResponse) { v.HostConfig.Tmpfs["/tmp"] = "size=4096m" }},
		{"extra mount", func(v *container.InspectResponse) {
			v.Mounts = []container.MountPoint{{Type: "bind", Destination: "/host"}}
		}},
		{"exec state unknown", func(v *container.InspectResponse) { v.State.Status = container.StateDead }},
		{"daemon logging enabled", func(v *container.InspectResponse) { v.HostConfig.LogConfig.Type = "json-file" }},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			inspected := inspectedPlan(plan)
			test.mutate(&inspected)
			if err := plan.VerifyInspect(inspected); err == nil {
				t.Fatal("verified weakened container")
			}
		})
	}
}

func TestNewPlanRefusesUntrustedProviderAndRun(t *testing.T) {
	plan := safePlan(t)
	id := plan.SandboxID
	template := plan.Template
	image := plan.Image
	spec := sandboxdomain.Spec{ID: id, Name: "Sandbox", Backend: sandboxdomain.BackendDocker, Binding: sandboxdomain.BuiltinBinding(sandboxdomain.BackendDocker), Isolation: sandboxdomain.IsolationContainer, OwnerKind: "local_user", OwnerRef: "operator"}
	good, err := sandboxdomain.NewSandbox(spec)
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name         string
		modifySpec   func(*sandboxdomain.Spec)
		state        sandboxdomain.State
		externalID   string
		imageImageID string
	}{
		{"host masquerading as docker", func(s *sandboxdomain.Spec) {
			s.Backend = sandboxdomain.BackendLocalWorkspace
			s.Binding = sandboxdomain.BuiltinBinding(sandboxdomain.BackendLocalWorkspace)
			s.Isolation = sandboxdomain.IsolationHost
			s.LocalRoot = filepath.Clean(t.TempDir())
		}, sandboxdomain.StateProvisioning, "", ""},
		{"already ready", nil, sandboxdomain.StateReady, "container-id", ""},
		{"unknown run", nil, sandboxdomain.StateUnknown, "", ""},
		{"externally bound", nil, sandboxdomain.StateProvisioning, "unexpected", ""},
		{"wrong image id", nil, sandboxdomain.StateProvisioning, "", "not-sha256"},
	} {
		t.Run(test.name, func(t *testing.T) {
			changed := spec
			if test.modifySpec != nil {
				test.modifySpec(&changed)
			}
			item, err := sandboxdomain.NewSandbox(changed)
			if err != nil {
				t.Fatal(err)
			}
			run, err := sandboxdomain.NewRun(id, 1, test.state, test.externalID)
			if err != nil {
				t.Fatal(err)
			}
			changedImage := image
			if test.imageImageID != "" {
				changedImage.ImageID = test.imageImageID
			}
			if _, err := NewContainerPlan(item, run, template, changedImage); err == nil {
				t.Fatal("unsafe plan built")
			}
		})
	}
	run, err := sandboxdomain.NewRun(id, 1, sandboxdomain.StateProvisioning, "")
	if err != nil {
		t.Fatal(err)
	}
	swapped := image
	swapped.Digest = "docker.io/library/alpine@sha256:" + strings.Repeat("b", 64)
	if _, err := NewContainerPlan(good, run, template, swapped); err == nil {
		t.Fatal("plan accepted mismatched manifest")
	}
}
