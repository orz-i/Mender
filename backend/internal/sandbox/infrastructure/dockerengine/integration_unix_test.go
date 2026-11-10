//go:build !windows

package dockerengine

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	mobyclient "github.com/moby/moby/client"
	gov "github.com/orz-i/Mender/backend/internal/governance/domain"
	providerconfigstore "github.com/orz-i/Mender/backend/internal/providerplugin/configstore"
	sandboxdomain "github.com/orz-i/Mender/backend/internal/sandbox/domain"
	v3fixture "github.com/orz-i/Mender/backend/internal/testfixtures/v3"
	workspaceapplication "github.com/orz-i/Mender/backend/internal/workspace/application"
	workspacedomain "github.com/orz-i/Mender/backend/internal/workspace/domain"
)

func TestDockerLiveWorkspaceParityAndIsolation(t *testing.T) {
	socket := os.Getenv("MENDER_DOCKER_TEST_SOCKET")
	image := os.Getenv("MENDER_DOCKER_TEST_IMAGE")
	if socket == "" || image == "" {
		t.Skip("explicit MENDER_DOCKER_TEST_SOCKET and MENDER_DOCKER_TEST_IMAGE digest required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Second)
	defer cancel()
	endpoint, err := ParseEngineEndpoint(socket)
	if err != nil {
		t.Fatal(err)
	}
	template := Template{ImageDigest: image, UserID: 10001, GroupID: 10001, CPUmilli: 1000, MemoryMiB: 512, WorkspaceMiB: 64, PIDs: 64}
	if err := template.Validate(); err != nil {
		t.Fatal(err)
	}
	db := filepath.Join(t.TempDir(), "docker-live.db")
	v3fixture.Prepare(t, db)
	providers, err := providerconfigstore.OpenTyped(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = providers.Close() }()
	store, err := openSandboxV3(t, ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()
	templates, err := OpenTemplateStore(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = templates.Close() }()
	idBytes := [16]byte{}
	if _, err := rand.Read(idBytes[:]); err != nil {
		t.Fatal(err)
	}
	id := sandboxdomain.ID("sbx_" + hex.EncodeToString(idBytes[:]))
	owner, err := gov.NewPrincipal(gov.PrincipalLocalUser, "integration-owner")
	if err != nil {
		t.Fatal(err)
	}
	caller, err := gov.NewContext(gov.OriginCLI, owner, gov.CatalogScope())
	if err != nil {
		t.Fatal(err)
	}
	item, err := sandboxdomain.NewSandbox(sandboxdomain.Spec{ID: id, Name: "Docker integration", Backend: sandboxdomain.BackendDocker, Binding: sandboxdomain.BuiltinBinding(sandboxdomain.BackendDocker), Isolation: sandboxdomain.IsolationContainer, OwnerKind: string(owner.Kind()), OwnerRef: owner.Reference()})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Create(ctx, item); err != nil {
		t.Fatal(err)
	}
	defined, err := sandboxdomain.NewRun(id, 1, sandboxdomain.StateDefined, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.CreateRun(ctx, defined); err != nil {
		t.Fatal(err)
	}
	if err := templates.CreateDockerTemplate(ctx, id, template); err != nil {
		t.Fatal(err)
	}
	provisioner, err := NewProvisioner(endpoint, store, templates, &fakeActionAdmitter{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = provisioner.Close() }()
	cli := provisioner.daemon.(*mobyclient.Client)
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		// Test cleanup only removes a uniquely named container created under
		// this newly generated SandboxID; no arbitrary daemon resources touched.
		for _, generation := range []int{1, 2, 3} {
			name := fmt.Sprintf("mender-sbx-%s-g%d", strings.TrimPrefix(id.String(), "sbx_"), generation)
			inspected, inspectErr := cli.ContainerInspect(cleanup, name, mobyclient.ContainerInspectOptions{})
			if inspectErr == nil && inspected.Container.Config != nil && inspected.Container.Config.Labels[labelSandboxID] == id.String() {
				_, removeErr := cli.ContainerRemove(cleanup, inspected.Container.ID, mobyclient.ContainerRemoveOptions{Force: true})
				if removeErr != nil {
					t.Errorf("cleanup test Sandbox: %v", removeErr)
				}
			}
		}
	}()
	ready, err := provisioner.Provision(ctx, caller, id)
	if err != nil {
		t.Fatalf("real Docker provision: %v", err)
	}
	if ready.State != sandboxdomain.StateReady {
		t.Fatalf("run %+v", ready)
	}
	backend, err := NewBackend(provisioner.observer, templates)
	if err != nil {
		t.Fatal(err)
	}
	session, err := backend.Open(ctx, item, ready)
	if err != nil {
		t.Fatalf("Open Docker Sandbox: %v", err)
	}
	workspace := session.(*workspaceSession)
	defer func() {
		if err := session.Close(); err != nil {
			t.Errorf("Docker session uncertain: %v", err)
		}
	}()
	call := func(result any, err error) map[string]any {
		t.Helper()
		if err != nil {
			t.Fatalf("Workspace execution: %v", err)
		}
		raw, err := json.Marshal(result)
		if err != nil {
			t.Fatal(err)
		}
		var content map[string]any
		if err := json.Unmarshal(raw, &content); err != nil {
			t.Fatal(err)
		}
		return content
	}
	shell := call(workspace.Shell(ctx, workspaceapplication.NormalizeShell(workspacedomain.ShellRequest{Command: "pwd; id; ls -ld /workspace; test ! -e /var/run/docker.sock; cat /proc/self/status | grep -E '^NoNewPrivs:'"})))
	t.Logf("Docker shell initial: %+v", shell)
	if shell["exitCode"] != float64(0) || !strings.Contains(fmt.Sprint(shell["stdout"]), "/workspace") {
		t.Fatal("shell did not execute inside container")
	}
	call(workspace.Edit(ctx, workspaceapplication.NormalizeEdit(workspacedomain.EditRequest{Path: "draft.txt", Operation: "create", Content: "hello world\n"})))
	read := call(workspace.Read(ctx, workspaceapplication.NormalizeRead(workspacedomain.ReadRequest{Path: "draft.txt"})))
	if read["content"] != "hello world" {
		t.Fatalf("read after create %+v", read)
	}
	search := call(workspace.Search(ctx, workspaceapplication.NormalizeSearch(workspacedomain.SearchRequest{Mode: "content", Query: "hello"})))
	if len(search["matches"].([]any)) != 1 {
		t.Fatalf("search %+v", search)
	}
	changed := call(workspace.Edit(ctx, workspaceapplication.NormalizeEdit(workspacedomain.EditRequest{Path: "draft.txt", Operation: "replace", Edits: []workspacedomain.TextEdit{{OldText: "hello", NewText: "goodbye"}}})))
	if changed["replacements"] != float64(1) {
		t.Fatalf("replace %+v", changed)
	}
	after := call(workspace.Shell(ctx, workspaceapplication.NormalizeShell(workspacedomain.ShellRequest{Command: "cat draft.txt"})))
	if !strings.Contains(fmt.Sprint(after["stdout"]), "goodbye world") {
		t.Fatalf("shell and file APIs differ %+v", after)
	}
	call(workspace.Edit(ctx, workspaceapplication.NormalizeEdit(workspacedomain.EditRequest{Path: "draft.txt", Operation: "delete"})))
	if _, err := workspace.readFile(ctx, "draft.txt", maxDockerReadBytes); err == nil {
		t.Fatal("deleted file still exists")
	}
	if err := session.Close(); err != nil {
		t.Fatal(err)
	}
	stopped, err := provisioner.ChangeLifecycle(ctx, caller, id, gov.ActionSandboxStop)
	if err != nil || stopped.State != sandboxdomain.StateStopped {
		t.Fatalf("real Docker Stop %+v %v", stopped, err)
	}
	started, err := provisioner.ChangeLifecycle(ctx, caller, id, gov.ActionSandboxStart)
	if err != nil || started.State != sandboxdomain.StateReady || started.Generation != 2 {
		t.Fatalf("real Docker Start %+v %v", started, err)
	}
	newSession, err := backend.Open(ctx, item, started)
	if err != nil {
		t.Fatalf("restart Backend.Open: %v", err)
	}
	newWorkspace := newSession.(*workspaceSession)
	if oldName := started.ExternalID; oldName == ready.ExternalID {
		t.Fatal("Start reused stale Docker container identity")
	}
	if err := newSession.Close(); err != nil {
		t.Fatal(err)
	}
	_ = newWorkspace
	destroyed, err := provisioner.ChangeLifecycle(ctx, caller, id, gov.ActionSandboxDestroy)
	if err != nil || destroyed.State != sandboxdomain.StateDestroyed {
		t.Fatalf("real Docker Destroy %+v %v", destroyed, err)
	}
}
