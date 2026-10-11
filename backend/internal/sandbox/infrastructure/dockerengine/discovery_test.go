package dockerengine

import (
	"path/filepath"
	"reflect"
	"testing"
)

func TestKnownLocalEndpointsAreFixedAndOrdered(t *testing.T) {
	t.Parallel()
	home := filepath.Join(string(filepath.Separator), "Users", "example")
	tests := []struct {
		name     string
		platform string
		uid      int
		want     []string
	}{
		{"macOS", "darwin", 501, []string{
			"unix:///Users/example/.docker/run/docker.sock",
			"unix:///Users/example/.colima/default/docker.sock",
			"unix:///Users/example/.orbstack/run/docker.sock",
		}},
		{"Linux", "linux", 1001, []string{
			"unix:///Users/example/.docker/desktop/docker.sock",
			"unix:///run/user/1001/docker.sock",
			"unix:///run/docker.sock",
		}},
		{"Linux without user id", "linux", -1, []string{
			"unix:///Users/example/.docker/desktop/docker.sock",
			"unix:///run/docker.sock",
		}},
		{"unsupported", "windows", 501, nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := KnownLocalEndpoints(home, tc.platform, tc.uid)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("endpoints = %#v, want %#v", got, tc.want)
			}
			for _, raw := range got {
				if _, err := ParseEngineEndpoint(raw); err != nil {
					t.Fatalf("invalid trusted endpoint %q: %v", raw, err)
				}
			}
		})
	}
}

func TestKnownLocalEndpointsRejectsUntrustedHomes(t *testing.T) {
	t.Parallel()
	for _, home := range []string{"", "relative/home", "../home", "/Users/foo/../bar", "/Users/foo//bar"} {
		if got := KnownLocalEndpoints(home, "darwin", 501); len(got) != 0 {
			t.Fatalf("accepted unclean home %q: %#v", home, got)
		}
	}
}
