package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/orz-i/Mender/backend/internal/bootstrap"
	governancedomain "github.com/orz-i/Mender/backend/internal/governance/domain"
	policyapplication "github.com/orz-i/Mender/backend/internal/policy/application"
	policydomain "github.com/orz-i/Mender/backend/internal/policy/domain"
)

func TestSandboxManagementListCreateDetailAndPolicyDenied(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	runtime, err := bootstrap.NewRuntime(ctx, bootstrap.RuntimeOptions{DatabasePath: filepath.Join(t.TempDir(), "sandbox-management.db")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runtime.Close() })
	server := httptest.NewServer(New(runtime))
	t.Cleanup(server.Close)
	empty := requestJSON(t, server.Client(), http.MethodGet, server.URL+"/api/v1/sandboxes", nil, http.StatusOK)
	if arr, ok := empty["sandboxes"].([]any); !ok || len(arr) != 0 {
		t.Fatalf("initial sandboxes %+v", empty)
	}
	root := t.TempDir()
	created := requestJSON(t, server.Client(), http.MethodPost, server.URL+"/api/v1/sandboxes/local", map[string]any{"name": "Safe local", "root": root, "isolation": "host"}, http.StatusCreated)
	id, ok := created["sandboxId"].(string)
	if !ok || id == "" {
		t.Fatalf("created %+v", created)
	}
	if created["backend"] != "local_workspace" || created["state"] != "ready" || created["isolation"] != "host" {
		t.Fatalf("unexpected sandbox %+v", created)
	}
	detail := requestJSON(t, server.Client(), http.MethodGet, server.URL+"/api/v1/sandboxes/"+id, nil, http.StatusOK)
	if detail["sandboxId"] != id {
		t.Fatalf("detail %+v", detail)
	}
	list := requestJSON(t, server.Client(), http.MethodGet, server.URL+"/api/v1/sandboxes", nil, http.StatusOK)
	if arr, ok := list["sandboxes"].([]any); !ok || len(arr) != 1 {
		t.Fatalf("listing %+v", list)
	}
	requestJSON(t, server.Client(), http.MethodGet, server.URL+"/api/v1/sandboxes/invalid", nil, http.StatusBadRequest)
	requestJSON(t, server.Client(), http.MethodGet, server.URL+"/api/v1/sandboxes/sbx_00000000000000000000000000000000", nil, http.StatusNotFound)
	requestJSON(t, server.Client(), http.MethodPost, server.URL+"/api/v1/sandboxes/local", map[string]any{"name": "Bad", "root": "relative", "isolation": "host"}, http.StatusBadRequest)
	if _, err := runtime.Policy.CreateRule.Execute(ctx, policyapplication.RuleInput{DisplayName: "Deny new sandbox", Enabled: true, Decision: policydomain.DecisionDeny, Match: policydomain.MatchSpec{Actions: []governancedomain.Action{governancedomain.ActionSandboxProvision}}}); err != nil {
		t.Fatal(err)
	}
	requestJSON(t, server.Client(), http.MethodPost, server.URL+"/api/v1/sandboxes/local", map[string]any{"name": "Denied", "root": filepath.Join(t.TempDir(), "absent"), "isolation": "host"}, http.StatusForbidden)
	final := requestJSON(t, server.Client(), http.MethodGet, server.URL+"/api/v1/sandboxes", nil, http.StatusOK)
	if arr, ok := final["sandboxes"].([]any); !ok || len(arr) != 1 {
		t.Fatalf("denied provision persisted: %+v", final)
	}
}

func TestSandboxManagementLifecycleLocalAndPolicyDeny(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	runtime, err := bootstrap.NewRuntime(ctx, bootstrap.RuntimeOptions{DatabasePath: filepath.Join(t.TempDir(), "lifecycle-api.db")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runtime.Close() })
	server := httptest.NewServer(New(runtime))
	t.Cleanup(server.Close)
	root := t.TempDir()
	created := requestJSON(t, server.Client(), http.MethodPost, server.URL+"/api/v1/sandboxes/local", map[string]any{"name": "Lifecycle", "root": root, "isolation": "host"}, http.StatusCreated)
	id := created["sandboxId"].(string)
	base := server.URL + "/api/v1/sandboxes/" + id
	if created["activeLeases"] != float64(0) {
		t.Fatalf("unexpected initial leases %+v", created)
	}
	if _, err := runtime.Policy.CreateRule.Execute(ctx, policyapplication.RuleInput{DisplayName: "deny Sandbox stop", Enabled: true, Decision: policydomain.DecisionDeny, Match: policydomain.MatchSpec{Actions: []governancedomain.Action{governancedomain.ActionSandboxStop}}}); err != nil {
		t.Fatal(err)
	}
	requestJSON(t, server.Client(), http.MethodPost, base+"/stop", nil, http.StatusForbidden)
	ready := requestJSON(t, server.Client(), http.MethodGet, base, nil, http.StatusOK)
	if ready["state"] != "ready" {
		t.Fatalf("denial changed state %+v", ready)
	}
	rules, err := runtime.Policy.Get.Execute(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, rule := range rules.Rules() {
		if err := runtime.Policy.DeleteRule.Execute(ctx, rule.ID()); err != nil {
			t.Fatal(err)
		}
	}
	stopped := requestJSON(t, server.Client(), http.MethodPost, base+"/stop", nil, http.StatusOK)
	if stopped["state"] != "stopped" {
		t.Fatalf("stop %+v", stopped)
	}
	requestJSON(t, server.Client(), http.MethodPost, base+"/stop", nil, http.StatusConflict)
	started := requestJSON(t, server.Client(), http.MethodPost, base+"/start", nil, http.StatusOK)
	if started["state"] != "ready" {
		t.Fatalf("start %+v", started)
	}
	destroyed := requestJSON(t, server.Client(), http.MethodDelete, base, nil, http.StatusOK)
	if destroyed["state"] != "destroyed" {
		t.Fatalf("destroy %+v", destroyed)
	}
	requestJSON(t, server.Client(), http.MethodPost, base+"/start", nil, http.StatusConflict)
	requestJSON(t, server.Client(), http.MethodPost, base+"/stop", nil, http.StatusConflict)
	requestJSON(t, server.Client(), http.MethodDelete, base, nil, http.StatusConflict)
	list := requestJSON(t, server.Client(), http.MethodGet, server.URL+"/api/v1/sandboxes", nil, http.StatusOK)
	rows := list["sandboxes"].([]any)
	if len(rows) != 1 || rows[0].(map[string]any)["state"] != "destroyed" {
		t.Fatalf("tombstone disappeared %+v", list)
	}
}

func TestSandboxManagementDockerFailsClosedWithoutTrustedEngine(t *testing.T) {
	t.Setenv(bootstrap.DockerSocketEnvironment, "")
	ctx := context.Background()
	runtime, err := bootstrap.NewRuntime(ctx, bootstrap.RuntimeOptions{DatabasePath: filepath.Join(t.TempDir(), "docker-unavailable.db")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runtime.Close() })
	server := httptest.NewServer(New(runtime))
	t.Cleanup(server.Close)
	status := requestJSON(t, server.Client(), http.MethodGet, server.URL+"/api/v1/sandboxes/backends", nil, http.StatusOK)
	if status["dockerAvailable"] != false {
		t.Fatalf("Docker unexpectedly enabled %+v", status)
	}
	requestJSON(t, server.Client(), http.MethodPost, server.URL+"/api/v1/sandboxes/docker", map[string]any{
		"name": "Denied Docker", "imageDigest": "docker.io/library/alpine@sha256:" + strings.Repeat("a", 64),
		"userId": 10001, "groupId": 10001, "cpuMilli": 1000, "memoryMiB": 512, "workspaceMiB": 64, "pids": 64,
	}, http.StatusServiceUnavailable)
	list := requestJSON(t, server.Client(), http.MethodGet, server.URL+"/api/v1/sandboxes", nil, http.StatusOK)
	if arr, ok := list["sandboxes"].([]any); !ok || len(arr) != 0 {
		t.Fatalf("disabled Docker created resources: %+v", list)
	}
}
