// Package dockerengine contains strict Docker Sandbox adapter configuration.
// Nothing in this package provisions containers or connects to a Docker daemon.
package dockerengine

import (
	"fmt"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
)

const (
	minCPUUnits     uint32 = 100
	maxCPUUnits     uint32 = 8000
	minMemoryMiB    uint32 = 256
	maxMemoryMiB    uint32 = 16384
	minWorkspaceMiB uint32 = 64
	maxWorkspaceMiB uint32 = 2048
	minPIDs         uint32 = 16
	maxPIDs         uint32 = 512
	workspacePath          = "/workspace"
)

var imageDigestPattern = regexp.MustCompile(`^[a-z0-9]+(?:[.-][a-z0-9]+)+(?:/[a-z0-9]+(?:[._-][a-z0-9]+)*){2,}@sha256:[0-9a-f]{64}$`)

// EngineEndpoint is an explicit local daemon endpoint, never a tool argument.
// Network/TCP/SSH endpoints, Docker context name resolution and environment
// defaults are intentionally unsupported.
type EngineEndpoint struct{ socketPath string }

func ParseEngineEndpoint(raw string) (EngineEndpoint, error) {
	if raw == "" || raw != strings.TrimSpace(raw) || len(raw) > 4096 {
		return EngineEndpoint{}, fmt.Errorf("docker Engine: endpoint must be non-empty and bounded")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return EngineEndpoint{}, fmt.Errorf("docker Engine: invalid endpoint: %w", err)
	}
	if u.Scheme != "unix" || u.Host != "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Opaque != "" || u.RawPath != "" {
		return EngineEndpoint{}, fmt.Errorf("docker Engine: only explicit local unix:///absolute/socket endpoints are allowed")
	}
	p := u.Path
	if !strings.HasPrefix(p, "/") || p != path.Clean(p) || len(p) > 4096 || strings.ContainsRune(p, '\x00') || raw != "unix://"+p {
		return EngineEndpoint{}, fmt.Errorf("docker Engine: socket must be a canonical absolute UNIX path without escapes")
	}
	return EngineEndpoint{socketPath: p}, nil
}
func (e EngineEndpoint) URL() string {
	if e.socketPath == "" {
		return ""
	}
	return "unix://" + e.socketPath
}
func (e EngineEndpoint) SocketPath() string { return e.socketPath }

// PreflightSocket checks only the local path shape and its filesystem type.
// D1b must still bind using a guarded Engine connection and verify daemon/API
// identity; this check alone does not prevent subsequent path replacement.
func (e EngineEndpoint) PreflightSocket() error {
	if _, err := ParseEngineEndpoint(e.URL()); err != nil {
		return err
	}
	path := string(filepath.Separator)
	for _, component := range strings.Split(strings.TrimPrefix(e.socketPath, path), path) {
		if component == "" {
			return fmt.Errorf("docker Engine: empty socket path component")
		}
		path = filepath.Join(path, component)
		info, err := os.Lstat(path)
		if err != nil {
			return fmt.Errorf("docker Engine: inspect socket path: %w", err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("docker Engine: socket path must not contain symlinks")
		}
		if path != e.socketPath {
			if !info.IsDir() {
				return fmt.Errorf("docker Engine: socket ancestor is not a directory")
			}
			if info.Mode().Perm()&0o022 != 0 {
				return fmt.Errorf("docker Engine: socket ancestor must not be group/world writable")
			}
		} else {
			if info.Mode()&os.ModeSocket == 0 {
				return fmt.Errorf("docker Engine: endpoint is not a UNIX socket")
			}
			if info.Mode().Perm()&0o022 != 0 {
				return fmt.Errorf("docker Engine: socket is group/world writable")
			}
		}
	}
	return nil
}

// Template captures a small trusted management configuration for one Sandbox.
// It never exposes unrestricted HostConfig, socket mounts, image build/pull,
// network controls or Docker daemon flags to model-controlled Tool inputs.
type Template struct {
	ImageDigest  string
	UserID       uint32
	GroupID      uint32
	CPUmilli     uint32
	MemoryMiB    uint32
	WorkspaceMiB uint32
	PIDs         uint32
}

func (t Template) Validate() error {
	if !imageDigestPattern.MatchString(t.ImageDigest) || len(t.ImageDigest) > 320 {
		return fmt.Errorf("docker Sandbox: require immutable fully qualified lowercase repository@sha256 digest")
	}
	if t.UserID < 1000 || t.UserID > 65534 || t.GroupID < 1000 || t.GroupID > 65534 {
		return fmt.Errorf("docker Sandbox: numeric non-root UID/GID must be in the 1000..65534 range")
	}
	if t.CPUmilli < minCPUUnits || t.CPUmilli > maxCPUUnits {
		return fmt.Errorf("docker Sandbox: CPU millicores outside enforced bounds")
	}
	if t.MemoryMiB < minMemoryMiB || t.MemoryMiB > maxMemoryMiB {
		return fmt.Errorf("docker Sandbox: memory outside enforced bounds")
	}
	if t.WorkspaceMiB < minWorkspaceMiB || t.WorkspaceMiB > maxWorkspaceMiB || t.WorkspaceMiB > t.MemoryMiB {
		return fmt.Errorf("docker Sandbox: workspace scratch limit must fit within memory budget")
	}
	if t.PIDs < minPIDs || t.PIDs > maxPIDs {
		return fmt.Errorf("docker Sandbox: PIDs outside enforced bounds")
	}
	return nil
}

// SecurityRequirements is fixed by Mender and must be verified against the
// *observed* Docker Engine HostConfig before the backend is enabled. Fields are
// intentionally not mutable via Template, Provider config, or Tool arguments.
type SecurityRequirements struct {
	ReadonlyRootfs        bool
	NetworkMode           string
	DropAllCapabilities   bool
	NoNewPrivileges       bool
	Privileged            bool
	HostMountsAllowed     bool
	PortPublishingAllowed bool
	WorkspaceRoot         string
}

func (t Template) SecurityRequirements() (SecurityRequirements, error) {
	if err := t.Validate(); err != nil {
		return SecurityRequirements{}, err
	}
	return SecurityRequirements{
		ReadonlyRootfs:        true,
		NetworkMode:           "none",
		DropAllCapabilities:   true,
		NoNewPrivileges:       true,
		Privileged:            false,
		HostMountsAllowed:     false,
		PortPublishingAllowed: false,
		WorkspaceRoot:         workspacePath,
	}, nil
}
