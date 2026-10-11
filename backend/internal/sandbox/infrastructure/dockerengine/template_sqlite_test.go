package dockerengine

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"

	sandboxdomain "github.com/orz-i/Mender/backend/internal/sandbox/domain"
	sandboxsqlite "github.com/orz-i/Mender/backend/internal/sandbox/infrastructure/sqlite"
)

func dockerTemplateFixture(t *testing.T) (context.Context, string, *sandboxsqlite.Store, *TemplateStore, sandboxdomain.ID, Template) {
	t.Helper()
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "mender-docker-template.db")
	resources, err := sandboxsqlite.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = resources.Close() })
	id := sandboxdomain.ID("sbx_0123456789abcdef0123456789abcdef")
	item, err := sandboxdomain.NewSandbox(sandboxdomain.Spec{ID: id, Name: "Docker resource", Backend: sandboxdomain.BackendDocker, Binding: sandboxdomain.BuiltinBinding(sandboxdomain.BackendDocker), Isolation: sandboxdomain.IsolationContainer, OwnerKind: "local_user", OwnerRef: "operator"})
	if err != nil {
		t.Fatal(err)
	}
	if err := resources.Create(ctx, item); err != nil {
		t.Fatal(err)
	}
	run, err := sandboxdomain.NewRun(id, 1, sandboxdomain.StateDefined, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := resources.CreateRun(ctx, run); err != nil {
		t.Fatal(err)
	}
	templates, err := OpenTemplateStore(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = templates.Close() })
	return ctx, path, resources, templates, id, safePlan(t).Template
}
func TestTrustedDockerTemplateImmutableAndDurableAcrossRestart(t *testing.T) {
	ctx, path, resources, templates, id, template := dockerTemplateFixture(t)
	if err := templates.CreateDockerTemplate(ctx, id, template); err != nil {
		t.Fatal(err)
	}
	if err := templates.CreateDockerTemplate(ctx, id, template); !errors.Is(err, sandboxdomain.ErrConflict) {
		t.Fatalf("overwrote pinned Docker settings: %v", err)
	}
	loaded, err := templates.GetDockerTemplate(ctx, id)
	if err != nil || loaded != template {
		t.Fatalf("template %+v %v", loaded, err)
	}
	pending, err := resources.GetRun(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := resources.TransitionRun(ctx, pending, sandboxdomain.StateProvisioning, ""); err != nil {
		t.Fatal(err)
	}
	updated := template
	updated.ImageDigest = "docker.io/team/other@sha256:" + "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	if err := templates.CreateDockerTemplate(ctx, id, updated); !errors.Is(err, sandboxdomain.ErrConflict) {
		t.Fatalf("mutable after provision: %v", err)
	}
	reopened, err := OpenTemplateStore(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = reopened.Close() }()
	persisted, err := reopened.GetDockerTemplate(ctx, id)
	if err != nil || persisted != template {
		t.Fatalf("persisted template %+v %v", persisted, err)
	}
	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = raw.Close() }()
	if _, err := raw.ExecContext(ctx, `UPDATE sandbox_docker_templates SET memory_mib=16384 WHERE sandbox_id=?`, id.String()); err == nil {
		t.Fatal("SQLite allowed direct Docker template mutation")
	}
	if _, err := raw.ExecContext(ctx, `DELETE FROM sandbox_docker_templates WHERE sandbox_id=?`, id.String()); err == nil {
		t.Fatal("SQLite allowed immutable template deletion")
	}
}
func TestTrustedDockerTemplateRejectsNonDockerAndInvalidInputs(t *testing.T) {
	ctx, _, resources, templates, id, good := dockerTemplateFixture(t)
	unknown := sandboxdomain.ID("sbx_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	if err := templates.CreateDockerTemplate(ctx, unknown, good); !errors.Is(err, sandboxdomain.ErrConflict) {
		t.Fatalf("allowed template on absent Sandbox: %v", err)
	}
	broken := good
	broken.ImageDigest = "alpine:latest"
	if err := templates.CreateDockerTemplate(ctx, id, broken); err == nil {
		t.Fatal("accepted mutable image ref")
	}
	otherID := sandboxdomain.ID("sbx_bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")
	local, err := sandboxdomain.NewSandbox(sandboxdomain.Spec{ID: otherID, Name: "host local", Backend: sandboxdomain.BackendLocalWorkspace, Binding: sandboxdomain.BuiltinBinding(sandboxdomain.BackendLocalWorkspace), Isolation: sandboxdomain.IsolationHost, OwnerKind: "local_user", OwnerRef: "operator", LocalRoot: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if err := resources.Create(ctx, local); err != nil {
		t.Fatal(err)
	}
	run, err := sandboxdomain.NewRun(otherID, 1, sandboxdomain.StateDefined, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := resources.CreateRun(ctx, run); err != nil {
		t.Fatal(err)
	}
	if err := templates.CreateDockerTemplate(ctx, otherID, good); !errors.Is(err, sandboxdomain.ErrConflict) {
		t.Fatalf("bound Docker template to Local Workspace: %v", err)
	}
	if _, err := templates.GetDockerTemplate(ctx, id); !errors.Is(err, sandboxdomain.ErrNotFound) {
		t.Fatalf("missing Docker template appeared: %v", err)
	}
}
