package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	mobyclient "github.com/moby/moby/client"
	"github.com/orz-i/Mender/backend/internal/bootstrap"
)

func TestCLILocalSandboxLifecycleAndDestroyConfirmation(t *testing.T) {
	t.Parallel()
	db := filepath.Join(t.TempDir(), "mender.db")
	root := t.TempDir()
	created := executeJSONCommand(t, db, "sandbox", "add", "local", "--name", "CLI Local", "--root", root, "--isolation", "host")
	item, ok := created["sandbox"].(map[string]any)
	if !ok {
		t.Fatalf("sandbox create %+v", created)
	}
	id := item["sandboxId"].(string)
	stopped := executeJSONCommand(t, db, "sandbox", "stop", id)
	if stopped["state"] != "stopped" {
		t.Fatalf("stop %+v", stopped)
	}
	started := executeJSONCommand(t, db, "sandbox", "start", id)
	if started["state"] != "ready" || started["generation"] != float64(2) {
		t.Fatalf("start %+v", started)
	}
	run := NewRootCommand(Options{Stdin: nil, Stdout: io.Discard, Stderr: io.Discard})
	run.SetArgs([]string{"--db", db, "sandbox", "destroy", id})
	if err := run.ExecuteContext(context.Background()); err == nil {
		t.Fatal("destroy without --confirm allowed")
	}
	destroyed := executeJSONCommand(t, db, "sandbox", "destroy", id, "--confirm")
	if destroyed["state"] != "destroyed" {
		t.Fatalf("destroy %+v", destroyed)
	}
	listed := executeJSONCommand(t, db, "sandbox", "list")
	if values, ok := listed["sandboxes"].([]any); !ok || len(values) != 1 {
		t.Fatalf("tombstoned resource lost %+v", listed)
	}
}

// Opt-in integration: every CLI lifecycle action composes a fresh Runtime,
// sharing the persisted Docker Sandbox identity via the same SQLite DB.
func TestCLIDockerRealLifecycle(t *testing.T) {
	socket, image := os.Getenv("MENDER_DOCKER_TEST_SOCKET"), os.Getenv("MENDER_DOCKER_TEST_IMAGE")
	if socket == "" || image == "" {
		t.Skip("requires explicit cached image and local Docker Engine socket")
	}
	t.Setenv(bootstrap.DockerSocketEnvironment, socket)
	db := filepath.Join(t.TempDir(), "docker-cli.db")
	created := executeJSONCommand(t, db, "sandbox", "add", "docker", "--name", "CLI Docker", "--image-digest", image)
	item, ok := created["sandbox"].(map[string]any)
	if !ok {
		t.Fatalf("created missing resource %+v", created)
	}
	id, ok := item["sandboxId"].(string)
	if !ok || id == "" {
		t.Fatalf("created SandboxID missing %+v", item)
	}
	defer func() {
		cli, err := mobyclient.New(mobyclient.WithHost(socket))
		if err != nil {
			t.Error(err)
			return
		}
		defer func() { _ = cli.Close() }()
		ctx, done := context.WithTimeout(context.Background(), 15*time.Second)
		defer done()
		for generation := 1; generation <= 2; generation++ {
			name := fmt.Sprintf("mender-sbx-%s-g%d", strings.TrimPrefix(id, "sbx_"), generation)
			inspected, err := cli.ContainerInspect(ctx, name, mobyclient.ContainerInspectOptions{})
			if err != nil || inspected.Container.Config == nil || inspected.Container.Config.Labels["io.mender.sandbox.id"] != id {
				continue
			}
			if _, err := cli.ContainerRemove(ctx, inspected.Container.ID, mobyclient.ContainerRemoveOptions{Force: true}); err != nil {
				t.Errorf("CLI test cleanup: %v", err)
			}
		}
	}()
	stopped := executeJSONCommand(t, db, "sandbox", "stop", id)
	if stopped["state"] != "stopped" {
		t.Fatalf("CLI stop %+v", stopped)
	}
	started := executeJSONCommand(t, db, "sandbox", "start", id)
	if started["state"] != "ready" || started["generation"] != float64(2) {
		t.Fatalf("CLI start %+v", started)
	}
	destroyed := executeJSONCommand(t, db, "sandbox", "destroy", id, "--confirm")
	if destroyed["state"] != "destroyed" {
		t.Fatalf("CLI destroy %+v", destroyed)
	}
	items := executeJSONCommand(t, db, "sandbox", "list")
	if listed, ok := items["sandboxes"].([]any); !ok || len(listed) != 1 {
		t.Fatalf("CLI tombstone not retained %+v", items)
	}
}
