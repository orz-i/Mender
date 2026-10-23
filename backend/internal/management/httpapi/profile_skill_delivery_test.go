package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/orz-i/Mender/backend/internal/bootstrap"
	profileapp "github.com/orz-i/Mender/backend/internal/profile/application"
	profiledomain "github.com/orz-i/Mender/backend/internal/profile/domain"
	skilldomain "github.com/orz-i/Mender/backend/internal/skill/domain"
)

func TestHTTPProfileSkillDeliveryReadOnlyAndOwnerScoped(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	r, err := bootstrap.NewRuntime(ctx, bootstrap.RuntimeOptions{
		DatabasePath: filepath.Join(root, "mender.db"),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = r.Close() })
	skillList, err := r.Skills.List(ctx)
	if err != nil || len(skillList) < 1 {
		t.Fatalf("skills %v %v", skillList, err)
	}
	id := skillList[0].InstallationID
	profile, err := r.Profiles.Create.Execute(ctx, profileapp.CreateProfileRequest{
		Slug: "one", DisplayName: "One", Mode: profiledomain.ExposureHybrid,
	})
	if err != nil {
		t.Fatal(err)
	}
	other, err := r.Profiles.Create.Execute(ctx, profileapp.CreateProfileRequest{
		Slug: "two", DisplayName: "Two", Mode: profiledomain.ExposureHybrid,
	})
	if err != nil {
		t.Fatal(err)
	}
	profile, err = r.Profiles.ReplaceSkills.Execute(ctx, profileapp.ReplaceSkillsRequest{
		ProfileSlug: profile.Slug(), InstallationIDs: []skilldomain.InstallationID{id},
	})
	if err != nil {
		t.Fatal(err)
	}
	ownedRoot := filepath.Join(root, "owned")
	legacyRoot := filepath.Join(root, "legacy")
	for _, dir := range []string{ownedRoot, legacyRoot} {
		if err := os.Mkdir(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	legacy, err := r.Skills.Deploy(ctx, id, skilldomain.DeploymentAgentsProject, legacyRoot)
	if err != nil {
		t.Fatal(err)
	}
	_, err = r.DeployProfileSkills(ctx, profile.Slug(), skilldomain.DeploymentAgentsProject, ownedRoot)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(New(r))
	t.Cleanup(server.Close)
	url := server.URL + "/api/v1/profiles/one/skill-deployments"
	found := requestJSON(t, server.Client(), http.MethodGet, url, nil, http.StatusOK)
	list, ok := found["deployments"].([]any)
	if !ok || len(list) != 1 {
		t.Fatalf("owned HTTP projection: %#v", found)
	}
	row, _ := list[0].(map[string]any)
	if row["profileId"] != profile.ID().String() || row["profileRevision"] != float64(profile.Revision()) {
		t.Fatalf("wrong Profile attribution: %#v", row)
	}
	otherList := requestJSON(t, server.Client(), http.MethodGet,
		server.URL+"/api/v1/profiles/"+other.Slug().String()+"/skill-deployments",
		nil, http.StatusOK)
	entries, ok := otherList["deployments"].([]any)
	if !ok || len(entries) != 0 {
		t.Fatalf("cross Profile records exposed: %#v", otherList)
	}
	requestStatus(t, server.Client(), http.MethodPost, url, []byte(`{}`),
		http.StatusMethodNotAllowed)
	ownedID, _ := row["deploymentId"].(string)
	// An owner-scoped record cannot be removed via a legacy global operation
	// or a different Profile's endpoint.
	requestJSON(t, server.Client(), http.MethodDelete,
		server.URL+"/api/v1/skill-deployments/"+ownedID, nil, http.StatusConflict)
	requestJSON(t, server.Client(), http.MethodDelete,
		server.URL+"/api/v1/profiles/"+other.Slug().String()+"/skill-deployments/"+ownedID,
		nil, http.StatusNotFound)
	requestJSON(t, server.Client(), http.MethodGet,
		server.URL+"/api/v1/profiles/not-here/skill-deployments",
		nil, http.StatusNotFound)
	after, err := r.Skills.ListDeployments(ctx)
	if err != nil || len(after) != 2 {
		t.Fatalf("HTTP read mutated deployment inventory: %v %v", after, err)
	}
	requestNoContent(t, server.Client(), http.MethodDelete,
		url+"/"+ownedID, http.StatusNoContent)
	remaining, err := r.Skills.ListDeployments(ctx)
	if err != nil || len(remaining) != 1 || remaining[0].Deployment.ID() != legacy.ID() {
		t.Fatalf("profile-scoped removal touched legacy: %v %v", remaining, err)
	}
	requestNoContent(t, server.Client(), http.MethodDelete,
		server.URL+"/api/v1/skill-deployments/"+legacy.ID().String(), http.StatusNoContent)
}
