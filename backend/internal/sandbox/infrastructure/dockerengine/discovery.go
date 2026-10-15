package dockerengine

import (
	"path"
	"path/filepath"
	"strconv"
	"strings"
)

// KnownLocalEndpoints returns only fixed host-owned UNIX socket locations.
// It must not inspect Docker contexts, DOCKER_HOST or user-selected daemon
// addresses. PreflightSocket and Probe still decide whether an endpoint can
// actually be trusted; discovery never relaxes those checks.
func KnownLocalEndpoints(home, platform string, uid int) []string {
	home = filepath.ToSlash(home)
	if home == "" || !strings.HasPrefix(home, "/") || path.Clean(home) != home {
		return nil
	}
	var paths []string
	switch platform {
	case "darwin":
		paths = []string{
			path.Join(home, ".docker", "run", "docker.sock"),
			path.Join(home, ".colima", "default", "docker.sock"),
			path.Join(home, ".orbstack", "run", "docker.sock"),
		}
	case "linux":
		paths = []string{path.Join(home, ".docker", "desktop", "docker.sock")}
		if uid >= 0 {
			paths = append(paths, path.Join("/run", "user", strconv.Itoa(uid), "docker.sock"))
		}
		paths = append(paths, "/run/docker.sock")
	default:
		return nil
	}
	result := make([]string, 0, len(paths))
	for _, p := range paths {
		result = append(result, "unix://"+p)
	}
	return result
}
