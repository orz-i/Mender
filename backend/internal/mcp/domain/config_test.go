package domain

import (
	"testing"

	catalog "github.com/orz-i/Mender/backend/internal/catalog/domain"
	profile "github.com/orz-i/Mender/backend/internal/profile/domain"
)

func TestConfigOwnsOnlyDeliveryAndCopiesDirectSelection(t *testing.T) {
	id := profile.ProfileID("prof_00000000000000000000000000000001")
	first := catalog.ToolID("tool_00000000000000000000000000000001")
	second := catalog.ToolID("tool_00000000000000000000000000000002")
	input := []catalog.ToolID{second, first}
	cfg, err := NewConfig(id, ModeHybrid, "", input)
	if err != nil {
		t.Fatal(err)
	}
	input[0] = first
	got := cfg.DirectToolIDs()
	if got[0] != first || got[1] != second {
		t.Fatal("selection was not canonicalized/copied")
	}
	got[0] = second
	if cfg.DirectToolIDs()[0] != first {
		t.Fatal("getter exposed mutable state")
	}
	for _, mode := range []Mode{ModeDirect, ModeDynamic} {
		c, err := NewConfig(id, mode, "", []catalog.ToolID{first})
		if err != nil || len(c.DirectToolIDs()) != 0 {
			t.Fatalf("mode derives direct scope from Profile: %+v %v", c, err)
		}
	}
	if _, err := NewConfig(id, ModeHybrid, "", []catalog.ToolID{first, first}); err == nil {
		t.Fatal("duplicate direct Tool accepted")
	}
	if _, err := NewConfig(id, "unknown", "", nil); err == nil {
		t.Fatal("unknown mode accepted")
	}
}
