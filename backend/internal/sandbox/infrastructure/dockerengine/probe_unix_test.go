//go:build !windows

package dockerengine

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	apiimage "github.com/moby/moby/api/types/image"
	apisystem "github.com/moby/moby/api/types/system"
	mobyclient "github.com/moby/moby/client"
	sandboxdomain "github.com/orz-i/Mender/backend/internal/sandbox/domain"
)

type probeFake struct {
	ping                       mobyclient.PingResult
	info                       mobyclient.SystemInfoResult
	image                      mobyclient.ImageInspectResult
	pingErr, infoErr, imageErr error
	imageCalls                 int
	imageList                  mobyclient.ImageListResult
	imageListErr               error
	imageListCalls             int
}

func (f *probeFake) Ping(context.Context, mobyclient.PingOptions) (mobyclient.PingResult, error) {
	return f.ping, f.pingErr
}
func (f *probeFake) Info(context.Context, mobyclient.InfoOptions) (mobyclient.SystemInfoResult, error) {
	return f.info, f.infoErr
}
func (f *probeFake) ImageInspect(_ context.Context, _ string, _ ...mobyclient.ImageInspectOption) (mobyclient.ImageInspectResult, error) {
	f.imageCalls++
	return f.image, f.imageErr
}
func (f *probeFake) ImageList(_ context.Context, _ mobyclient.ImageListOptions) (mobyclient.ImageListResult, error) {
	f.imageListCalls++
	return f.imageList, f.imageListErr
}

func newProbeFixture(t *testing.T) (*Inspector, *probeFake) {
	t.Helper()
	endpoint, _ := localSocketFixture(t)
	f := &probeFake{
		ping: mobyclient.PingResult{APIVersion: "1.56", OSType: "linux"},
		info: mobyclient.SystemInfoResult{Info: apisystem.Info{
			ServerVersion: "29.8.0", OSType: "linux", CgroupVersion: "2", CPUCfsQuota: true, CPUCfsPeriod: true, MemoryLimit: true, PidsLimit: true, SecurityOptions: []string{"name=seccomp,profile=builtin"},
		}},
	}
	return &Inspector{endpoint: endpoint, api: f}, f
}

func TestDockerEngineReadOnlyProbeAndImmutableLocalImage(t *testing.T) {
	inspector, f := newProbeFixture(t)
	result, err := inspector.Probe(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if result.EngineVersion != "29.8.0" || result.APIVersion != "1.56" || !result.SeccompBuiltin || !result.CPULimits || !result.MemoryLimits || !result.PIDsLimit {
		t.Fatalf("invalid Engine probe %+v", result)
	}
	plan := Template{ImageDigest: immutableDigest, UserID: 10001, GroupID: 10001, CPUmilli: 1000, MemoryMiB: 1024, WorkspaceMiB: 256, PIDs: 32}
	f.image = mobyclient.ImageInspectResult{InspectResponse: apiimage.InspectResponse{
		ID: "sha256:" + strings.Repeat("0", 64), RepoDigests: []string{immutableDigest}, Os: "linux", Architecture: "arm64",
	}}
	image, err := inspector.InspectLocalImage(context.Background(), plan)
	if err != nil {
		t.Fatal(err)
	}
	if image.Digest != immutableDigest || image.OSType != "linux" || image.Architecture != "arm64" || f.imageCalls != 1 {
		t.Fatalf("wrong local image %+v", image)
	}
}

func TestDockerEngineProbeFailsClosedOnMissingHardGuarantees(t *testing.T) {
	mutations := []struct {
		name   string
		change func(*probeFake)
	}{
		{"unavailable daemon", func(f *probeFake) { f.pingErr = errors.New("internal docker daemon detail /socket/file") }},
		{"unsupported old API", func(f *probeFake) { f.ping.APIVersion = "1.50" }},
		{"invalid API", func(f *probeFake) { f.ping.APIVersion = "unexpected" }},
		{"not linux", func(f *probeFake) { f.info.Info.OSType = "windows" }},
		{"no cgroups v2", func(f *probeFake) { f.info.Info.CgroupVersion = "1" }},
		{"seccomp disabled", func(f *probeFake) { f.info.Info.SecurityOptions = nil }},
		{"seccomp unconfined", func(f *probeFake) { f.info.Info.SecurityOptions = []string{"name=seccomp,profile=unconfined"} }},
		{"CPU not enforced", func(f *probeFake) { f.info.Info.CPUCfsQuota = false }},
		{"memory not enforced", func(f *probeFake) { f.info.Info.MemoryLimit = false }},
		{"PID not enforced", func(f *probeFake) { f.info.Info.PidsLimit = false }},
		{"missing server version", func(f *probeFake) { f.info.Info.ServerVersion = "" }},
	}
	for _, tc := range mutations {
		t.Run(tc.name, func(t *testing.T) {
			i, f := newProbeFixture(t)
			tc.change(f)
			_, err := i.Probe(context.Background())
			if !errors.Is(err, sandboxdomain.ErrBackendUnavailable) {
				t.Fatalf("probe failed open: %v", err)
			}
			if strings.Contains(err.Error(), "/socket/file") {
				t.Fatal("daemon error leaked socket path")
			}
		})
	}
}

func TestDockerEngineRejectsUnverifiedImage(t *testing.T) {
	variations := []struct {
		name   string
		change func(*probeFake)
	}{
		{"repo digest mismatch", func(f *probeFake) {
			f.image.RepoDigests = []string{"docker.io/library/alpine@sha256:" + strings.Repeat("f", 64)}
		}},
		{"missing ID", func(f *probeFake) { f.image.ID = "" }},
		{"wrong OS", func(f *probeFake) { f.image.Os = "windows" }},
		{"missing architecture", func(f *probeFake) { f.image.Architecture = "" }},
		{"not locally present", func(f *probeFake) { f.imageErr = errors.New("not found") }},
	}
	plan := Template{ImageDigest: immutableDigest, UserID: 10001, GroupID: 10001, CPUmilli: 1000, MemoryMiB: 1024, WorkspaceMiB: 256, PIDs: 32}
	for _, tc := range variations {
		t.Run(tc.name, func(t *testing.T) {
			i, f := newProbeFixture(t)
			f.image = mobyclient.ImageInspectResult{InspectResponse: apiimage.InspectResponse{ID: "sha256:" + strings.Repeat("0", 64), RepoDigests: []string{immutableDigest}, Os: "linux", Architecture: "amd64"}}
			tc.change(f)
			if _, err := i.InspectLocalImage(context.Background(), plan); !errors.Is(err, sandboxdomain.ErrBackendUnavailable) {
				t.Fatalf("untrusted image accepted: %v", err)
			}
		})
	}
}

// Real daemon smoke is opt-in and READ-ONLY: only Ping/Info are invoked, and
// no image is pulled or container is created.
func TestDockerEngineReadOnlyLiveSmoke(t *testing.T) {
	raw := os.Getenv("MENDER_DOCKER_SMOKE_SOCKET")
	if raw == "" {
		t.Skip("set MENDER_DOCKER_SMOKE_SOCKET=unix:///explicit/socket to run read-only Engine probe")
	}
	endpoint, err := ParseEngineEndpoint(raw)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("DOCKER_HOST", "tcp://127.0.0.1:2375") // API must ignore untrusted host override.
	inspector, err := NewInspector(endpoint)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = inspector.Close() }()
	result, err := inspector.Probe(context.Background())
	if err != nil {
		t.Fatalf("local read-only Engine probe: %v", err)
	}
	t.Logf("read-only Engine=%s API=%s OS=%s cgroup=%s seccompBuiltin=%t", result.EngineVersion, result.APIVersion, result.OSType, result.CgroupVersion, result.SeccompBuiltin)
}

func TestDockerHubDigestAliasRequiresIdenticalManifest(t *testing.T) {
	manifest := "sha256:" + strings.Repeat("a", 64)
	if !locallyVerifiedDigest([]string{"alpine@" + manifest}, "docker.io/library/alpine@"+manifest) {
		t.Fatal("normalized official Docker Hub digest was rejected")
	}
	if locallyVerifiedDigest([]string{"alpine@" + manifest}, "example.com/library/alpine@"+manifest) {
		t.Fatal("alias crossed registry boundary")
	}
	if locallyVerifiedDigest([]string{"alpine@" + manifest}, "docker.io/library/alpine@sha256:"+strings.Repeat("b", 64)) {
		t.Fatal("different manifest accepted")
	}
}
