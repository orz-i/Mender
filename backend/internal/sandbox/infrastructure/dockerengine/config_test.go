package dockerengine

import (
	"fmt"
	"strings"
	"testing"
)

const immutableDigest = "docker.io/library/alpine@sha256:" + "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func TestEngineEndpointAcceptsOnlyCanonicalExplicitLocalUnixSocket(t *testing.T) {
	ok, err := ParseEngineEndpoint("unix:///Users/runner/.docker/run/docker.sock")
	if err != nil {
		t.Fatal(err)
	}
	if ok.URL() != "unix:///Users/runner/.docker/run/docker.sock" || ok.SocketPath() != "/Users/runner/.docker/run/docker.sock" {
		t.Fatalf("parsed %+v", ok)
	}
	for _, raw := range []string{
		"", "unix://", "unix://localhost/tmp/docker.sock", "tcp://127.0.0.1:2375", "ssh://alice@example.com/run/docker.sock",
		"http://127.0.0.1:2375", "https://docker.example.com", "npipe:////./pipe/docker_engine", "desktop-linux",
		"unix:///tmp/../var/docker.sock", "unix:///tmp/./docker.sock", "unix:///tmp//docker.sock", "unix:///tmp/%64ocker.sock",
		"unix:///tmp/docker.sock?x=y", "unix:///tmp/docker.sock#frag", "unix:///tmp/docker.sock ", "unix:/relative/docker.sock",
		"unix:///tmp/docker.sock\x00",
	} {
		t.Run(fmt.Sprintf("reject_%q", raw), func(t *testing.T) {
			if _, err := ParseEngineEndpoint(raw); err == nil {
				t.Fatalf("accepted dangerous Engine endpoint %q", raw)
			}
		})
	}
	if err := (*new(EngineEndpoint)).PreflightSocket(); err == nil {
		t.Fatal("zero-value EngineEndpoint accepted")
	}
}

func TestDockerTemplateRequiresDigestNonRootAndBoundedBudgets(t *testing.T) {
	good := Template{ImageDigest: immutableDigest, UserID: 10001, GroupID: 10001, CPUmilli: 1000, MemoryMiB: 1024, WorkspaceMiB: 256, PIDs: 64}
	if err := good.Validate(); err != nil {
		t.Fatalf("valid immutable template: %v", err)
	}
	requirements, err := good.SecurityRequirements()
	if err != nil {
		t.Fatal(err)
	}
	if !requirements.ReadonlyRootfs || requirements.NetworkMode != "none" || !requirements.DropAllCapabilities || !requirements.NoNewPrivileges || requirements.Privileged || requirements.HostMountsAllowed || requirements.PortPublishingAllowed || requirements.WorkspaceRoot != "/workspace" {
		t.Fatalf("unsafe security requirements %+v", requirements)
	}
	tests := []struct {
		name   string
		modify func(*Template)
	}{
		{"mutable tag", func(t *Template) { t.ImageDigest = "docker.io/library/alpine:latest" }},
		{"repo not qualified", func(t *Template) { t.ImageDigest = "library/alpine@sha256:" + strings.Repeat("a", 64) }},
		{"image uppercase", func(t *Template) { t.ImageDigest = "DOCKER.io/library/alpine@sha256:" + strings.Repeat("a", 64) }},
		{"image uppercase hex", func(t *Template) { t.ImageDigest = "docker.io/library/alpine@sha256:" + strings.Repeat("F", 64) }},
		{"image short digest", func(t *Template) { t.ImageDigest = "docker.io/library/alpine@sha256:abcd" }},
		{"image tag plus digest", func(t *Template) { t.ImageDigest = "docker.io/library/alpine:3@sha256:" + strings.Repeat("a", 64) }},
		{"pull flags", func(t *Template) {
			t.ImageDigest = "docker.io/library/alpine@sha256:" + strings.Repeat("a", 64) + " --pull=always"
		}},
		{"root UID", func(t *Template) { t.UserID = 0 }},
		{"system UID", func(t *Template) { t.UserID = 10 }},
		{"root GID", func(t *Template) { t.GroupID = 0 }},
		{"zero CPU", func(t *Template) { t.CPUmilli = 0 }},
		{"unbounded CPU", func(t *Template) { t.CPUmilli = 100000 }},
		{"tiny memory", func(t *Template) { t.MemoryMiB = 64 }},
		{"unbounded memory", func(t *Template) { t.MemoryMiB = 65535 }},
		{"empty scratch", func(t *Template) { t.WorkspaceMiB = 0 }},
		{"scratch larger than memory", func(t *Template) { t.WorkspaceMiB = 2048; t.MemoryMiB = 512 }},
		{"no PIDs", func(t *Template) { t.PIDs = 0 }},
		{"excessive PIDs", func(t *Template) { t.PIDs = 2048 }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			v := good
			test.modify(&v)
			if err := v.Validate(); err == nil {
				t.Fatalf("accepted unsafe template %+v", v)
			}
			if _, err := v.SecurityRequirements(); err == nil {
				t.Fatal("unsafe template produced security requirements")
			}
		})
	}
}
