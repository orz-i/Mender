package cli

import (
	"errors"
	"fmt"
	"path/filepath"
	"testing"

	mcpdelivery "github.com/orz-i/Mender/backend/internal/mcp/application/delivery"
)

func TestProfileMCPDeliveryCommandsSeparateSelectionAndRevisionedConfiguration(t *testing.T) {
	db := filepath.Join(t.TempDir(), "delivery.db")
	execute := func(args ...string) map[string]any {
		t.Helper()
		result, err := executeSkillJSON(db, args...)
		if err != nil {
			t.Fatalf("%v: %v", args, err)
		}
		return result
	}
	selected := execute("profile", "create", "--slug", "delivery", "--name", "Delivery")
	if _, ok := selected["mode"]; ok {
		t.Fatal("Profile selection retained MCP mode")
	}
	profileRevision := fmt.Sprint(selected["revision"])
	if _, err := executeSkillJSON(db, "profile", "mcp-delivery", "show", "delivery"); !errors.Is(err, mcpdelivery.ErrNotFound) || exitCode(err) != ExitConfig {
		t.Fatalf("unconfigured delivery silently acquired defaults: %v", err)
	}
	if _, err := executeSkillJSON(db, "profile", "mcp-delivery", "configure", "delivery", "--mode", "dynamic"); err == nil {
		t.Fatal("MCP configuration accepted an omitted Profile revision")
	}
	configured := execute("profile", "mcp-delivery", "configure", "delivery", "--mode", "dynamic", "--profile-revision", profileRevision)
	if configured["mode"] != "dynamic" || configured["profileId"] != selected["profileId"] || fmt.Sprint(configured["profileRevision"]) != profileRevision {
		t.Fatalf("separate delivery identity changed: %+v", configured)
	}
	firstRevision := fmt.Sprint(configured["revision"])
	if _, err := executeSkillJSON(db, "profile", "mcp-delivery", "configure", "delivery", "--mode", "direct", "--profile-revision", profileRevision); !errors.Is(err, mcpdelivery.ErrRevisionConflict) || exitCode(err) != ExitConfig {
		t.Fatalf("omitted CAS revision replaced existing delivery: %v", err)
	}
	updated := execute("profile", "mcp-delivery", "configure", "delivery", "--mode", "direct", "--profile-revision", profileRevision, "--revision", firstRevision)
	currentRevision := fmt.Sprint(updated["revision"])
	if updated["mode"] != "direct" || currentRevision == firstRevision {
		t.Fatalf("CAS update did not advance delivery: %+v", updated)
	}
	if _, err := executeSkillJSON(db, "profile", "mcp-delivery", "configure", "delivery", "--mode", "dynamic", "--profile-revision", profileRevision, "--revision", firstRevision); !errors.Is(err, mcpdelivery.ErrRevisionConflict) {
		t.Fatalf("stale MCP revision accepted: %v", err)
	}
	if _, err := executeSkillJSON(db, "profile", "mcp-delivery", "configure", "delivery", "--mode", "dynamic", "--profile-revision", "999999", "--revision", currentRevision); !errors.Is(err, mcpdelivery.ErrProfileRevisionConflict) {
		t.Fatalf("stale Profile revision accepted: %v", err)
	}
	if _, err := executeSkillJSON(db, "profile", "mcp-delivery", "configure", "delivery", "--mode", "hybrid", "--profile-revision", profileRevision, "--revision", currentRevision, "--direct-tool", "tool_00000000000000000000000000000009"); !errors.Is(err, mcpdelivery.ErrToolOutsideProfile) {
		t.Fatalf("direct Tool broadened Profile membership: %v", err)
	}
	loaded := execute("profile", "mcp-delivery", "show", "delivery")
	if loaded["mode"] != "direct" || fmt.Sprint(loaded["revision"]) != currentRevision {
		t.Fatalf("failed configuration mutated delivery: %+v", loaded)
	}
	unchanged := execute("profile", "show", "delivery")
	if fmt.Sprint(unchanged["revision"]) != profileRevision {
		t.Fatalf("MCP configuration mutated Profile revision: %+v", unchanged)
	}
}

func TestProfileCommandSurfaceDoesNotRetainOldDeliveryAuthority(t *testing.T) {
	root := NewRootCommand(Options{})
	create, _, err := root.Find([]string{"profile", "create"})
	if err != nil {
		t.Fatal(err)
	}
	if create.Flags().Lookup("mode") != nil {
		t.Fatal("Profile creation still accepts transport mode")
	}
	add, _, err := root.Find([]string{"profile", "add-tool"})
	if err != nil {
		t.Fatal(err)
	}
	if add.Flags().Lookup("direct") != nil {
		t.Fatal("Tool selection still accepts MCP direct exposure")
	}
	for _, path := range [][]string{{"profile", "mcp-delivery", "show"}, {"profile", "mcp-delivery", "configure"}} {
		if _, remaining, err := root.Find(path); err != nil || len(remaining) != 0 {
			t.Fatalf("delivery command missing: %v %v", path, err)
		}
	}
	for _, err := range []error{mcpdelivery.ErrNotFound, mcpdelivery.ErrRevisionConflict, mcpdelivery.ErrProfileRevisionConflict, mcpdelivery.ErrToolOutsideProfile, mcpdelivery.ErrInvalidConfiguration} {
		if exitCode(err) != ExitConfig {
			t.Fatalf("delivery error classified as execution failure: %v", err)
		}
	}
}
