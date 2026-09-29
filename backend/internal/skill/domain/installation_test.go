package domain

import (
	"strings"
	"testing"
	"time"
)

func TestInstallationTrustAndEnablement(t *testing.T) {
	t.Parallel()

	item := newTestInstallation(t, false, false)
	if item.Trusted() || item.Enabled() {
		t.Fatalf("initial state trusted=%v enabled=%v", item.Trusted(), item.Enabled())
	}
	if _, err := item.WithEnabled(true); err == nil {
		t.Fatal("WithEnabled(true) without trust error = nil")
	}

	item = item.WithTrusted(true)
	enabled, err := item.WithEnabled(true)
	if err != nil {
		t.Fatalf("WithEnabled(true) error = %v", err)
	}
	if !enabled.Trusted() || !enabled.Enabled() {
		t.Fatalf("enabled state trusted=%v enabled=%v", enabled.Trusted(), enabled.Enabled())
	}

	untrusted := enabled.WithTrusted(false)
	if untrusted.Trusted() || untrusted.Enabled() {
		t.Fatalf("untrusted state trusted=%v enabled=%v", untrusted.Trusted(), untrusted.Enabled())
	}
}

func TestInstallationValidation(t *testing.T) {
	t.Parallel()

	base := newTestInstallation(t, false, false)
	if err := base.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}

	_, err := NewInstallation(
		base.ID(),
		base.SkillID(),
		base.Name(),
		base.Description(),
		base.SourceKind(),
		base.SourceReference(),
		base.Digest(),
		base.VersionHint(),
		false,
		true,
		base.InstalledAt(),
	)
	if err == nil || !strings.Contains(err.Error(), "must be trusted") {
		t.Fatalf("enabled untrusted error = %v", err)
	}
}

func newTestInstallation(t *testing.T, trusted, enabled bool) Installation {
	t.Helper()
	id, err := ParseInstallationID("sinst_0123456789abcdef0123456789abcdef")
	if err != nil {
		t.Fatal(err)
	}
	skillID, err := ParseSkillID("skill_0123456789abcdef0123456789abcdef")
	if err != nil {
		t.Fatal(err)
	}
	item, err := NewInstallation(
		id,
		skillID,
		"example-skill",
		"An example Skill.",
		SourceLocalDirectory,
		"/tmp/example-skill",
		"sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		"1.0.0",
		trusted,
		enabled,
		time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatal(err)
	}
	return item
}
