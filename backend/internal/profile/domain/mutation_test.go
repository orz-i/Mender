package domain

import "testing"

func TestProfileReplaceToolsNormalizesByMode(t *testing.T) {
	t.Parallel()

	id, _ := ParseProfileID("prof_00000000000000000000000000000001")
	slug, _ := NewSlug("default")
	first := mustToolID(t, "tool_00000000000000000000000000000001")
	second := mustToolID(t, "tool_00000000000000000000000000000002")

	profile, err := NewProfile(id, slug, "Default", ExposureDirect, nil)
	if err != nil {
		t.Fatal(err)
	}
	replaced, err := profile.ReplaceTools([]ToolSelection{
		{ToolID: first, Direct: false},
		{ToolID: second, Direct: false},
	})
	if err != nil {
		t.Fatalf("ReplaceTools() error = %v", err)
	}
	if len(replaced.Selections()) != 2 || len(replaced.DirectToolIDs()) != 2 {
		t.Fatalf("replaced selections = %+v", replaced.Selections())
	}
}

func TestProfileAddAndRemoveTool(t *testing.T) {
	t.Parallel()

	id, _ := ParseProfileID("prof_00000000000000000000000000000001")
	slug, _ := NewSlug("default")
	first := mustToolID(t, "tool_00000000000000000000000000000001")
	second := mustToolID(t, "tool_00000000000000000000000000000002")

	profile, err := NewProfile(id, slug, "Default", ExposureHybrid, []ToolSelection{
		{ToolID: first, Direct: true},
	})
	if err != nil {
		t.Fatal(err)
	}

	added, err := profile.AddTool(second, false)
	if err != nil {
		t.Fatalf("AddTool() error = %v", err)
	}
	if !added.Contains(first) || !added.Contains(second) {
		t.Fatalf("added profile selections = %+v", added.Selections())
	}
	if profile.Contains(second) {
		t.Fatal("AddTool() mutated original Profile")
	}

	removed, err := added.RemoveTool(first)
	if err != nil {
		t.Fatalf("RemoveTool() error = %v", err)
	}
	if removed.Contains(first) || !removed.Contains(second) {
		t.Fatalf("removed profile selections = %+v", removed.Selections())
	}
}
