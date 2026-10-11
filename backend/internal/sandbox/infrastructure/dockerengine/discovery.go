package dockerengine

import (
	"path/filepath"
	"strconv"
)

// KnownLocalEndpoints returns only fixed host-owned UNIX socket locations.
// It must not inspect Docker contexts, DOCKER_HOST or user-selected daemon
// addresses. PreflightSocket and Probe still decide whether an endpoint can
// actually be trusted; discovery never relaxes those checks.
func KnownLocalEndpoints(home, platform string, uid int) []string {
	if home == "" || !filepath.IsAbs(home) || filepath.Clean(home) != home {
		return nil
	}
	var paths []string
	switch platform {
	case "darwin":
		paths = []string{
			filepath.Join(home, ".docker", "run", "docker.sock"),
			filepath.Join(home, ".colima", "default", "docker.sock"),
			filepath.Join(home, ".orbstack", "run", "docker.sock"),
		}
	case "linux":
		paths = []string{filepath.Join(home, ".docker", "desktop", "docker.sock")}
		if uid >= 0 {
			paths = append(paths, filepath.Join("/run", "user", strconv.Itoa(uid), "docker.sock"))
		}
		paths = append(paths, "/run/docker.sock")
	default:
		return nil
	}
	result := make([]string, 0, len(paths))
	for _, path := range paths {
		result = append(result, "unix://"+path)
	}
	return result
}
