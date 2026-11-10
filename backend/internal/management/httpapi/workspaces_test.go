package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/orz-i/Mender/backend/internal/bootstrap"
)

func TestWorkspaceHTTPAdmissionRequiresIndependentToolAndSandboxGrants(t *testing.T) {
	ctx := context.Background()
	r, e := bootstrap.NewRuntime(ctx, bootstrap.RuntimeOptions{DatabasePath: filepath.Join(t.TempDir(), "api-v3.sqlite")})
	if e != nil {
		t.Fatal(e)
	}
	defer func() { _ = r.Close() }()
	server := httptest.NewServer(New(r))
	defer server.Close()
	base := server.URL + "/api/v1"
	root, e := filepath.EvalSymlinks(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	sandbox := requestJSON(t, server.Client(), http.MethodPost, base+"/sandboxes/local", map[string]any{"name": "API Sandbox", "root": root, "isolation": "host"}, http.StatusCreated)
	sid := sandbox["sandboxId"].(string)
	profile := requestJSON(t, server.Client(), http.MethodPost, base+"/profiles", map[string]any{"slug": "api-workspace", "displayName": "API Workspace Profile"}, http.StatusCreated)
	pid := profile["profileId"].(string)
	tools := requestJSON(t, server.Client(), http.MethodGet, base+"/tools/browse?sourceId=native:workspace&limit=100", nil, http.StatusOK)["tools"].([]any)
	var readTool string
	for _, raw := range tools {
		v := raw.(map[string]any)
		if v["name"] == "read" {
			readTool = v["toolId"].(string)
		}
	}
	if readTool == "" {
		t.Fatal("canonical workspace.read not discovered")
	}
	native := requestJSON(t, server.Client(), http.MethodPost, base+"/workspaces", map[string]any{"displayName": "Registered work", "verifiedRoot": root}, http.StatusCreated)
	wid := native["workspaceId"].(string)
	if wid == "" {
		t.Fatal("workspace ID missing")
	}
	current := requestJSON(t, server.Client(), http.MethodGet, base+"/workspaces", nil, http.StatusOK)
	if len(current["workspaces"].([]any)) != 1 {
		t.Fatalf("Workspace not listed: %+v", current)
	}
	grants := requestJSON(t, server.Client(), http.MethodGet, base+"/workspaces/"+wid+"/profile-grants", nil, http.StatusOK)
	if len(grants["profileIds"].([]any)) != 0 {
		t.Fatalf("Workspace registered with implicit grant: %+v", grants)
	}
	path := base + "/workspaces/" + wid + "/placements/" + sid
	requestJSON(t, server.Client(), http.MethodPut, path, nil, http.StatusNoContent)
	targetBase := base + "/profiles/api-workspace/tool-targets"
	target := map[string]any{"toolId": readTool, "sandboxId": sid, "workspaceId": wid}
	requestJSON(t, server.Client(), http.MethodPost, targetBase, target, http.StatusConflict)
	requestJSON(t, server.Client(), http.MethodPut, base+"/workspaces/"+wid+"/profile-grants/"+pid, nil, http.StatusNoContent)
	requestJSON(t, server.Client(), http.MethodPost, targetBase, target, http.StatusConflict)
	requestJSON(t, server.Client(), http.MethodPut, base+"/sandboxes/"+sid+"/profile-grants/"+pid, map[string]any{"capabilities": []string{"file.read"}}, http.StatusOK)
	requestJSON(t, server.Client(), http.MethodPost, targetBase, target, http.StatusConflict) // still no ToolBinding
	requestJSON(t, server.Client(), http.MethodPost, base+"/profiles/api-workspace/tools", map[string]any{"toolId": readTool, "policyConstraint": "inherit", "concurrencyConstraint": "inherit"}, http.StatusOK)
	configured := requestJSON(t, server.Client(), http.MethodPost, targetBase, target, http.StatusCreated)
	if configured["workspaceId"] != wid || configured["sandboxId"] != sid {
		t.Fatalf("wrong scoped target: %+v", configured)
	}
	scopes := requestJSON(t, server.Client(), http.MethodGet, targetBase, nil, http.StatusOK)
	if len(scopes["targets"].([]any)) != 1 {
		t.Fatalf("scopes not visible: %+v", scopes)
	}
	requestJSON(t, server.Client(), http.MethodDelete, base+"/workspaces/"+wid+"/profile-grants/"+pid, nil, http.StatusNoContent)
	scopes = requestJSON(t, server.Client(), http.MethodGet, targetBase, nil, http.StatusOK)
	if len(scopes["targets"].([]any)) != 0 {
		t.Fatal("revoked grant retained a resurrectable target")
	}
	requestJSON(t, server.Client(), http.MethodPut, base+"/workspaces/"+wid+"/profile-grants/"+pid, nil, http.StatusNoContent)
	scopes = requestJSON(t, server.Client(), http.MethodGet, targetBase, nil, http.StatusOK)
	if len(scopes["targets"].([]any)) != 0 {
		t.Fatal("new grant silently reactivated retired target")
	}
	requestJSON(t, server.Client(), http.MethodDelete, path, nil, http.StatusNoContent)
	requestJSON(t, server.Client(), http.MethodDelete, base+"/workspaces/"+wid, nil, http.StatusConflict) // still granted to Profile
	requestJSON(t, server.Client(), http.MethodDelete, base+"/workspaces/"+wid+"/profile-grants/"+pid, nil, http.StatusNoContent)
	requestJSON(t, server.Client(), http.MethodDelete, base+"/workspaces/"+wid, nil, http.StatusNoContent)
}
