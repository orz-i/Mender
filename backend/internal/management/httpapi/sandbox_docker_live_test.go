package httpapi

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	mobyclient "github.com/moby/moby/client"
	"github.com/orz-i/Mender/backend/internal/bootstrap"
)

// Opt-in real Docker HTTP smoke. It tests the same typed management transport
// used by Web and Desktop; it never pulls images or touches unrelated containers.
func TestHTTPDockerSandboxRealLifecycle(t *testing.T) {
	socket, image := os.Getenv("MENDER_DOCKER_TEST_SOCKET"), os.Getenv("MENDER_DOCKER_TEST_IMAGE")
	if socket == "" || image == "" {
		t.Skip("requires explicit pinned MENDER_DOCKER_TEST_SOCKET and MENDER_DOCKER_TEST_IMAGE")
	}
	t.Setenv(bootstrap.DockerSocketEnvironment, socket)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	runtime, err := bootstrap.NewRuntime(ctx, bootstrap.RuntimeOptions{DatabasePath: filepath.Join(t.TempDir(), "docker-http.db")})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = runtime.Close() }()
	server := httptest.NewServer(New(runtime))
	defer server.Close()
	backends := requestJSON(t, server.Client(), http.MethodGet, server.URL+"/api/v1/sandboxes/backends", nil, http.StatusOK)
	if backends["dockerAvailable"] != true {
		t.Fatalf("Docker backend not enabled: %+v", backends)
	}
	cached := requestJSON(t, server.Client(), http.MethodGet, server.URL+"/api/v1/sandboxes/docker/images", nil, http.StatusOK)
	available, ok := cached["images"].([]any)
	if !ok {
		t.Fatalf("Docker cached images response invalid: %+v", cached)
	}
	found := false
	for _, entry := range available {
		imageEntry, ok := entry.(map[string]any)
		if ok && imageEntry["imageDigest"] == image && imageEntry["displayName"] != "" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected verified local image %s in Docker HTTP inventory: %+v", image, cached)
	}
	created := requestJSON(t, server.Client(), http.MethodPost, server.URL+"/api/v1/sandboxes/docker", map[string]any{
		"name": "HTTP Docker smoke", "imageDigest": image, "userId": 10001, "groupId": 10001,
		"cpuMilli": 1000, "memoryMiB": 512, "workspaceMiB": 64, "pids": 64,
	}, http.StatusCreated)
	id, ok := created["sandboxId"].(string)
	if !ok || id == "" {
		t.Fatalf("missing generated sandbox identity: %+v", created)
	}
	defer func() {
		// Delete only the unique Mender-labeled test resource if an assertion fails.
		endpoint := "unix://" + strings.TrimPrefix(socket, "unix://")
		cli, err := mobyclient.New(mobyclient.WithHost(endpoint))
		if err != nil {
			t.Error(err)
			return
		}
		defer func() { _ = cli.Close() }()
		cleanup, done := context.WithTimeout(context.Background(), 15*time.Second)
		defer done()
		for generation := 1; generation <= 2; generation++ {
			name := fmt.Sprintf("mender-sbx-%s-g%d", strings.TrimPrefix(id, "sbx_"), generation)
			inspected, err := cli.ContainerInspect(cleanup, name, mobyclient.ContainerInspectOptions{})
			if err != nil || inspected.Container.Config == nil || inspected.Container.Config.Labels["io.mender.sandbox.id"] != id {
				continue
			}
			if _, err := cli.ContainerRemove(cleanup, inspected.Container.ID, mobyclient.ContainerRemoveOptions{Force: true}); err != nil {
				t.Errorf("cleanup test Docker container: %v", err)
			}
		}
	}()
	if created["state"] != "ready" || created["backend"] != "docker" || created["backendAvailable"] != true {
		t.Fatalf("created container state: %+v", created)
	}
	path := server.URL + "/api/v1/sandboxes/" + id
	stopped := requestJSON(t, server.Client(), http.MethodPost, path+"/stop", nil, http.StatusOK)
	if stopped["state"] != "stopped" {
		t.Fatalf("HTTP stop %+v", stopped)
	}
	started := requestJSON(t, server.Client(), http.MethodPost, path+"/start", nil, http.StatusOK)
	if started["state"] != "ready" {
		t.Fatalf("HTTP start %+v", started)
	}
	destroyed := requestJSON(t, server.Client(), http.MethodDelete, path, nil, http.StatusOK)
	if destroyed["state"] != "destroyed" {
		t.Fatalf("HTTP destroy %+v", destroyed)
	}
	detail := requestJSON(t, server.Client(), http.MethodGet, path, nil, http.StatusOK)
	if detail["state"] != "destroyed" {
		t.Fatalf("tombstone missing %+v", detail)
	}
}
