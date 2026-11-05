package binding

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/orz-i/Mender/backend/internal/bootstrap"
	mcpdelivery "github.com/orz-i/Mender/backend/internal/mcp/application/delivery"
	profiledomain "github.com/orz-i/Mender/backend/internal/profile/domain"
)

func TestWailsMCPDeliveryHasIndependentRevisionAndNeverStartsAListener(t *testing.T) {
	runtime, err := bootstrap.NewRuntime(context.Background(), bootstrap.RuntimeOptions{DatabasePath: filepath.Join(t.TempDir(), "delivery.db")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runtime.Close() })
	service := NewService(context.Background(), runtime)
	selected, err := service.CreateProfile("delivery", "Delivery")
	if err != nil {
		t.Fatal(err)
	}
	if missing, err := service.GetProfileMCPDelivery(selected.Slug); err != nil || missing != nil {
		t.Fatalf("unconfigured delivery must be null: %+v %v", missing, err)
	}
	if _, err := runtime.GetProfileMCPDelivery(context.Background(), profiledomain.Slug(selected.Slug)); !errors.Is(err, mcpdelivery.ErrNotFound) {
		t.Fatalf("unconfigured query wrote delivery configuration: %v", err)
	}
	if servers := service.ListMCPServers(); len(servers.Servers) != 0 {
		t.Fatalf("unconfigured query started listeners: %+v", servers)
	}
	if missing, err := service.GetProfileMCPDelivery("missing-profile"); err == nil || missing != nil {
		t.Fatalf("unknown Profile was hidden as unconfigured delivery: %+v %v", missing, err)
	}
	first, err := service.ConfigureProfileMCPDelivery(selected.Slug, ProfileMCPDeliveryInput{Mode: "dynamic", ExpectedProfileRevision: selected.Revision})
	if err != nil || first.Mode != "dynamic" || first.ProfileID != selected.ProfileID || first.ProfileRevision != selected.Revision || first.DirectToolIDs == nil {
		t.Fatalf("configure projection: %+v %v", first, err)
	}
	loaded, err := service.GetProfileMCPDelivery(selected.Slug)
	if err != nil || loaded == nil || !reflect.DeepEqual(first, *loaded) {
		t.Fatalf("delivery round trip: %+v %v", loaded, err)
	}
	if _, err := service.ConfigureProfileMCPDelivery(selected.Slug, ProfileMCPDeliveryInput{Mode: "direct", ExpectedProfileRevision: selected.Revision}); !errors.Is(err, mcpdelivery.ErrRevisionConflict) {
		t.Fatalf("create-only update replaced delivery: %v", err)
	}
	firstRevision := first.Revision
	updated, err := service.ConfigureProfileMCPDelivery(selected.Slug, ProfileMCPDeliveryInput{Mode: "direct", ExpectedProfileRevision: selected.Revision, ExpectedRevision: &firstRevision})
	if err != nil || updated.Revision == first.Revision || updated.Mode != "direct" {
		t.Fatalf("CAS update changed: %+v %v", updated, err)
	}
	currentRevision := updated.Revision
	if _, err := service.ConfigureProfileMCPDelivery(selected.Slug, ProfileMCPDeliveryInput{Mode: "dynamic", ExpectedProfileRevision: selected.Revision, ExpectedRevision: &firstRevision}); !errors.Is(err, mcpdelivery.ErrRevisionConflict) {
		t.Fatalf("stale delivery revision accepted: %v", err)
	}
	if _, err := service.ConfigureProfileMCPDelivery(selected.Slug, ProfileMCPDeliveryInput{Mode: "hybrid", DirectToolIDs: []string{"tool_00000000000000000000000000000009"}, ExpectedProfileRevision: selected.Revision, ExpectedRevision: &currentRevision}); !errors.Is(err, mcpdelivery.ErrToolOutsideProfile) {
		t.Fatalf("MCP delivery broadened selected Tools: %v", err)
	}
	unchanged, err := service.GetProfile(selected.Slug)
	if err != nil || unchanged.Revision != selected.Revision {
		t.Fatalf("delivery mutated Profile: %+v %v", unchanged, err)
	}
	inventory, err := runtime.Skills.List(context.Background())
	if err != nil || len(inventory) == 0 {
		t.Fatalf("Skill fixture missing: %v", err)
	}
	changed, err := service.ReplaceProfileSkills(selected.Slug, []string{inventory[0].InstallationID.String()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.ConfigureProfileMCPDelivery(selected.Slug, ProfileMCPDeliveryInput{Mode: "dynamic", ExpectedProfileRevision: selected.Revision, ExpectedRevision: &currentRevision}); !errors.Is(err, mcpdelivery.ErrProfileRevisionConflict) {
		t.Fatalf("stale selection revision accepted: %v", err)
	}
	loaded, err = service.GetProfileMCPDelivery(selected.Slug)
	if err != nil || loaded == nil || loaded.Revision != updated.Revision || loaded.ProfileRevision != changed.Revision || loaded.Mode != "direct" {
		t.Fatalf("separate revisions changed: %+v %v", loaded, err)
	}
	if servers := service.ListMCPServers(); len(servers.Servers) != 0 {
		t.Fatalf("desired configuration started listeners: %+v", servers)
	}
	payload, err := json.Marshal(changed)
	if err != nil {
		t.Fatal(err)
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(payload, &object); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"mode", "tunnelId"} {
		if _, exists := object[key]; exists {
			t.Fatalf("Profile response retained %s delivery authority", key)
		}
	}
	toolPayload, err := json.Marshal(ProfileTool{ToolID: "tool_00000000000000000000000000000009"})
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(toolPayload, &object); err != nil {
		t.Fatal(err)
	}
	if _, exists := object["direct"]; exists {
		t.Fatal("Profile Tool response retained MCP direct authority")
	}
}
