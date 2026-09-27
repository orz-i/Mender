package domain

import (
	"testing"
	"time"
)

func TestCredentialMetadataInvariants(t *testing.T) {
	t.Parallel()

	ref, err := NewReference("github", "work")
	if err != nil {
		t.Fatal(err)
	}
	if ref.String() != "credential://github/work" ||
		ref.Namespace() != "github" ||
		ref.Name() != "work" {
		t.Fatalf("Reference = %q namespace=%q name=%q", ref, ref.Namespace(), ref.Name())
	}

	now := time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC)
	native, err := NewCredential(ref, "GitHub Work", SourceNative, "", now)
	if err != nil {
		t.Fatal(err)
	}
	if native.Source() != SourceNative || native.EnvironmentVariable() != "" {
		t.Fatalf("native Credential = %#v", native)
	}

	env, err := NewCredential(
		ref,
		"GitHub Work",
		SourceEnvironment,
		"GITHUB_TOKEN",
		now,
	)
	if err != nil {
		t.Fatal(err)
	}
	if env.EnvironmentVariable() != "GITHUB_TOKEN" {
		t.Fatalf("EnvironmentVariable() = %q", env.EnvironmentVariable())
	}

	if _, err := NewCredential(ref, "", SourceNative, "", now); err == nil {
		t.Fatal("accepted empty display name")
	}
	if _, err := NewCredential(ref, "x", SourceNative, "TOKEN", now); err == nil {
		t.Fatal("native source accepted environment variable")
	}
	if _, err := NewCredential(ref, "x", SourceEnvironment, "bad-name", now); err == nil {
		t.Fatal("environment source accepted invalid variable")
	}
	if _, err := NewCredential(ref, "x", Source("unknown"), "", now); err == nil {
		t.Fatal("accepted unknown source")
	}
}
