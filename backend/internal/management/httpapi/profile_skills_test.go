package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/orz-i/Mender/backend/internal/bootstrap"
)

func TestProfileSkillsManagementHTTPUsesExactSelectionWithoutDeploying(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	runtime, err := bootstrap.NewRuntime(ctx, bootstrap.RuntimeOptions{
		DatabasePath: filepath.Join(root, "mender.db"), SkillRoot: filepath.Join(root, "managed"),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runtime.Close() })

	server := httptest.NewServer(New(runtime))
	t.Cleanup(server.Close)
	url := server.URL + "/api/v1/profiles/delivery/skills"
	profile := requestJSON(t, server.Client(), http.MethodPost, server.URL+"/api/v1/profiles", map[string]any{
		"slug": "delivery", "displayName": "Delivery",
	}, http.StatusCreated)
	if ids, ok := profile["skills"].([]any); !ok || len(ids) != 0 {
		t.Fatalf("new Profile Skill selection = %#v", profile["skills"])
	}
	items, err := runtime.Skills.List(ctx)
	if err != nil || len(items) == 0 {
		t.Fatalf("Skill inventory = %v, %v", items, err)
	}
	builtInID := items[0].InstallationID.String()

	requestJSON(t, server.Client(), http.MethodPut, url, map[string]any{}, http.StatusBadRequest)
	requestJSON(t, server.Client(), http.MethodPut, url, map[string]any{
		"installationIds": []string{"invalid"},
	}, http.StatusBadRequest)
	requestJSON(t, server.Client(), http.MethodPut, url, map[string]any{
		"installationIds": []string{builtInID, builtInID},
	}, http.StatusBadRequest)
	profile = requestJSON(t, server.Client(), http.MethodPut, url, map[string]any{
		"installationIds": []string{builtInID},
	}, http.StatusOK)
	ids, ok := profile["skills"].([]any)
	if !ok || len(ids) != 1 || ids[0] != builtInID || len(profile["tools"].([]any)) != 0 {
		t.Fatalf("Profile Skill binding = %#v", profile)
	}
	profile = requestJSON(t, server.Client(), http.MethodGet, server.URL+"/api/v1/profiles/delivery", nil, http.StatusOK)
	ids, ok = profile["skills"].([]any)
	if !ok || len(ids) != 1 || ids[0] != builtInID {
		t.Fatalf("GET Profile lost Skill selection = %#v", profile)
	}
	deployments, err := runtime.Skills.ListDeployments(ctx)
	if err != nil || len(deployments) != 0 {
		t.Fatalf("selecting a Skill deployed files: %v, %v", deployments, err)
	}
	requestJSON(t, server.Client(), http.MethodPut, url, map[string]any{"installationIds": []string{}}, http.StatusOK)
}
