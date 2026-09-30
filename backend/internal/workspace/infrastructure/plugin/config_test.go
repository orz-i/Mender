package plugin

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestWorkspaceDescriptorAndConfig(t *testing.T) {
	t.Parallel()

	descriptor := Descriptor()
	if descriptor.ID != ID || descriptor.ConfigVersion != ConfigVersion {
		t.Fatalf("Descriptor() = %#v", descriptor)
	}
	if err := descriptor.Validate(); err != nil {
		t.Fatalf("Descriptor().Validate() error = %v", err)
	}
	if len(descriptor.Capabilities) != 3 {
		t.Fatalf("Capabilities = %v, want 3 file capabilities", descriptor.Capabilities)
	}

	root := t.TempDir()
	raw, err := json.Marshal(Config{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateConfig(context.Background(), raw); err != nil {
		t.Fatalf("ValidateConfig(valid) error = %v", err)
	}
	metadata, err := InspectConfig(context.Background(), raw)
	if err != nil {
		t.Fatalf("InspectConfig(valid) error = %v", err)
	}
	canonical, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	if metadata.ConnectionSummary != canonical {
		t.Fatalf("ConnectionSummary = %q, want %q", metadata.ConnectionSummary, canonical)
	}
	if len(metadata.CredentialRefs) != 0 {
		t.Fatalf("CredentialRefs = %v, want none", metadata.CredentialRefs)
	}
}

func TestWorkspaceConfigRejectsUnsafeOrInvalidRoots(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	if err := ValidateConfig(ctx, json.RawMessage("{\"root\":\"relative\"}")); err == nil {
		t.Fatal("ValidateConfig() accepted relative root")
	}
	if err := ValidateConfig(ctx, json.RawMessage("{\"root\":\"/tmp\",\"extra\":true}")); err == nil {
		t.Fatal("ValidateConfig() accepted unknown field")
	}

	file := filepath.Join(t.TempDir(), "file.txt")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(Config{Root: file})
	if err := ValidateConfig(ctx, raw); err == nil {
		t.Fatal("ValidateConfig() accepted regular file root")
	}
}
