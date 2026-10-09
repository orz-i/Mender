package dockerengine

import (
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/moby/moby/api/types/container"
	mobyclient "github.com/moby/moby/client"
	sandboxdomain "github.com/orz-i/Mender/backend/internal/sandbox/domain"
)

const (
	labelSandboxID        = "io.mender.sandbox.id"
	labelGeneration       = "io.mender.sandbox.generation"
	labelBackend          = "io.mender.sandbox.backend"
	maxContainerNameBytes = 125
	runnerExecutable      = "/bin/sh"
	runnerCommand         = "exec sleep 2147483647"
)

var contentAddressPattern = regexp.MustCompile(`^sha256:[a-f0-9]{64}$`)
var containerIDPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)

// ContainerPlan binds one admitted, previously recorded provisioning Run to a
// verified local immutable image. The plan is host-owned and unavailable to
// arbitrary Tool arguments. It does not create/start a container.
type ContainerPlan struct {
	SandboxID  sandboxdomain.ID
	Generation uint64
	Name       string
	Image      LocalImage
	Template   Template
	Options    mobyclient.ContainerCreateOptions
}

func NewContainerPlan(sandbox sandboxdomain.Sandbox, run sandboxdomain.Run, t Template, image LocalImage) (ContainerPlan, error) {
	if err := sandbox.Validate(); err != nil {
		return ContainerPlan{}, err
	}
	if sandbox.Backend() != sandboxdomain.BackendDocker || sandbox.Isolation() != sandboxdomain.IsolationContainer {
		return ContainerPlan{}, sandboxdomain.ErrBackendUnavailable
	}
	if _, err := sandboxdomain.NewRun(run.SandboxID, run.Generation, run.State, run.ExternalID); err != nil {
		return ContainerPlan{}, err
	}
	if run.SandboxID != sandbox.ID() || run.State != sandboxdomain.StateProvisioning || run.ExternalID != "" {
		return ContainerPlan{}, sandboxdomain.ErrConflict
	}
	if err := t.Validate(); err != nil {
		return ContainerPlan{}, err
	}
	if image.Digest != t.ImageDigest || image.OSType != "linux" || image.Architecture == "" || !contentAddressPattern.MatchString(image.ImageID) {
		return ContainerPlan{}, fmt.Errorf("docker sandbox: immutable local image identity mismatch")
	}
	expectedName := "mender-sbx-" + strings.TrimPrefix(run.SandboxID.String(), "sbx_") + "-g" + strconv.FormatUint(run.Generation, 10)
	if len(expectedName) > maxContainerNameBytes {
		return ContainerPlan{}, fmt.Errorf("docker sandbox: computed container name too long")
	}
	user := fmt.Sprintf("%d:%d", t.UserID, t.GroupID)
	labels := map[string]string{labelSandboxID: sandbox.ID().String(), labelGeneration: strconv.FormatUint(run.Generation, 10), labelBackend: string(sandboxdomain.BackendDocker)}
	pids := int64(t.PIDs)
	maxBytes := int64(t.MemoryMiB) * 1024 * 1024
	tmpMiB := t.WorkspaceMiB / 4
	if tmpMiB > 128 {
		tmpMiB = 128
	}
	if tmpMiB < 16 {
		tmpMiB = 16
	}
	workspaceTmpfs := fmt.Sprintf("rw,nosuid,nodev,mode=0700,uid=%d,gid=%d,size=%dm", t.UserID, t.GroupID, t.WorkspaceMiB)
	tempTmpfs := fmt.Sprintf("rw,nosuid,nodev,mode=1777,size=%dm", tmpMiB)
	config := &container.Config{
		Image: t.ImageDigest, User: user, WorkingDir: workspacePath,
		Entrypoint: []string{runnerExecutable, "-c"}, Cmd: []string{runnerCommand},
		Healthcheck: &container.HealthConfig{Test: []string{"NONE"}},
		Env:         []string{"HOME=/workspace", "PATH=/usr/bin:/bin", "TMPDIR=/tmp"},
		Labels:      labels, NetworkDisabled: true,
	}
	host := &container.HostConfig{
		ReadonlyRootfs: true, NetworkMode: container.NetworkMode("none"),
		CapDrop: []string{"ALL"}, SecurityOpt: []string{"no-new-privileges"},
		CgroupnsMode: container.CgroupnsModePrivate, IpcMode: container.IPCModePrivate,
		Privileged: false, AutoRemove: false, LogConfig: container.LogConfig{Type: "none"},
		Tmpfs:     map[string]string{workspacePath: workspaceTmpfs, "/tmp": tempTmpfs},
		Resources: container.Resources{NanoCPUs: int64(t.CPUmilli) * 1_000_000, Memory: maxBytes, MemorySwap: maxBytes, PidsLimit: &pids},
	}
	return ContainerPlan{SandboxID: sandbox.ID(), Generation: run.Generation, Name: expectedName, Image: image, Template: t,
		Options: mobyclient.ContainerCreateOptions{Name: expectedName, Config: config, HostConfig: host}}, nil
}

// VerifyInspect rejects any observed resource whose identity, constraints or
// effective privilege level diverges from the *original* trusted plan.
// Docker may normalize 'ALL' to 'CAP_ALL' and no-new-privileges to ':true'.
func (p ContainerPlan) VerifyInspect(observed container.InspectResponse) error {
	if p.Name == "" || p.Options.Config == nil || p.Options.HostConfig == nil || p.Image.ImageID == "" {
		return fmt.Errorf("verify Docker container: missing reviewed plan")
	}
	if !containerIDPattern.MatchString(observed.ID) || observed.Name != "/"+p.Name || observed.Image != p.Image.ImageID || observed.Config == nil || observed.HostConfig == nil || observed.State == nil {
		return fmt.Errorf("verify Docker container: resource identity mismatch")
	}
	expected := p.Options.Config
	c := observed.Config
	if c.Image != expected.Image || c.User != expected.User || c.WorkingDir != workspacePath || !slices.Equal(c.Entrypoint, expected.Entrypoint) || !slices.Equal(c.Cmd, expected.Cmd) || !slices.Equal(c.Env, expected.Env) || c.Healthcheck == nil || !slices.Equal(c.Healthcheck.Test, []string{"NONE"}) || !c.NetworkDisabled || len(c.ExposedPorts) != 0 || len(c.Volumes) != 0 {
		return fmt.Errorf("verify Docker container: execution config changed")
	}
	for k, v := range expected.Labels {
		if c.Labels[k] != v {
			return fmt.Errorf("verify Docker container: management label mismatch")
		}
	}
	h := observed.HostConfig
	expectedH := p.Options.HostConfig
	if !h.ReadonlyRootfs || string(h.NetworkMode) != "none" || h.Privileged || h.AutoRemove || h.PublishAllPorts || len(h.PortBindings) != 0 || len(h.Binds) != 0 || len(h.VolumesFrom) != 0 || len(h.Mounts) != 0 || len(h.Devices) != 0 || len(h.DeviceRequests) != 0 || len(h.CapAdd) != 0 || len(h.GroupAdd) != 0 || len(h.ExtraHosts) != 0 || len(h.DNS) != 0 || len(h.DNSOptions) != 0 || len(h.DNSSearch) != 0 {
		return fmt.Errorf("verify Docker container: elevated privileges or host/network mounts")
	}
	if h.CgroupnsMode != container.CgroupnsModePrivate || h.IpcMode != container.IPCModePrivate || string(h.PidMode) == "host" || string(h.UsernsMode) == "host" || string(h.UTSMode) == "host" || h.LogConfig.Type != "none" {
		return fmt.Errorf("verify Docker container: untrusted namespace or logging")
	}
	if len(h.CapDrop) != 1 || (h.CapDrop[0] != "ALL" && h.CapDrop[0] != "CAP_ALL") || len(h.SecurityOpt) != 1 || (h.SecurityOpt[0] != "no-new-privileges" && h.SecurityOpt[0] != "no-new-privileges:true") {
		return fmt.Errorf("verify Docker container: capability or no-new-privileges weakened")
	}
	if h.Memory != expectedH.Memory || h.MemorySwap != expectedH.MemorySwap || h.NanoCPUs != expectedH.NanoCPUs || h.PidsLimit == nil || expectedH.PidsLimit == nil || *h.PidsLimit != *expectedH.PidsLimit || len(h.Tmpfs) != len(expectedH.Tmpfs) {
		return fmt.Errorf("verify Docker container: resource constraints changed")
	}
	for k, v := range expectedH.Tmpfs {
		if h.Tmpfs[k] != v {
			return fmt.Errorf("verify Docker container: workspace filesystem constraints changed")
		}
	}
	for _, mount := range observed.Mounts {
		if mount.Type != "tmpfs" || (mount.Destination != workspacePath && mount.Destination != "/tmp") {
			return fmt.Errorf("verify Docker container: unexpected mount")
		}
	}
	switch string(observed.State.Status) {
	case "created", "running", "exited":
		return nil
	default:
		return fmt.Errorf("verify Docker container: unknown or unavailable state")
	}
}
