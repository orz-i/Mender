package domain

import "testing"

func TestProfileReplaceToolsNormalizesByMode(t *testing.T) {
	t.Parallel()

	id, _ := ParseProfileID("prof_00000000000000000000000000000001")
	slug, _ := NewSlug("default")
	first := mustToolID(t, "tool_00000000000000000000000000000001")
	second := mustToolID(t, "tool_00000000000000000000000000000002")

	profile, err := NewProfileWithBindings(id, slug, "Default", ExposureDirect, nil)
	if err != nil {
		t.Fatal(err)
	}
	replaced, err := profile.ReplaceBindings([]ToolBinding{
		mustBinding(t, first, false),
		mustBinding(t, second, false),
	})
	if err != nil {
		t.Fatalf("ReplaceTools() error = %v", err)
	}
	if len(replaced.Bindings()) != 2 || len(replaced.DirectToolIDs()) != 2 {
		t.Fatalf("replaced selections = %+v", replaced.Bindings())
	}
}

func TestProfileAddAndRemoveTool(t *testing.T) {
	t.Parallel()

	id, _ := ParseProfileID("prof_00000000000000000000000000000001")
	slug, _ := NewSlug("default")
	first := mustToolID(t, "tool_00000000000000000000000000000001")
	second := mustToolID(t, "tool_00000000000000000000000000000002")

	profile, err := NewProfileWithBindings(id, slug, "Default", ExposureHybrid, []ToolBinding{
		mustBinding(t, first, true),
	})
	if err != nil {
		t.Fatal(err)
	}

	added, err := profile.AddBinding(mustBinding(t, second, false))
	if err != nil {
		t.Fatalf("AddTool() error = %v", err)
	}
	if !added.Contains(first) || !added.Contains(second) {
		t.Fatalf("added profile selections = %+v", added.Bindings())
	}
	if profile.Contains(second) {
		t.Fatal("AddTool() mutated original Profile")
	}

	removed, err := added.RemoveTool(first)
	if err != nil {
		t.Fatalf("RemoveTool() error = %v", err)
	}
	if removed.Contains(first) || !removed.Contains(second) {
		t.Fatalf("removed profile selections = %+v", removed.Bindings())
	}
}
