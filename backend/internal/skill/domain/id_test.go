package domain

import "testing"

func TestSkillIDs(t *testing.T) {
	t.Parallel()

	if got, err := ParseSkillID("skill_0123456789abcdef0123456789abcdef"); err != nil ||
		got.String() != "skill_0123456789abcdef0123456789abcdef" {
		t.Fatalf("ParseSkillID() = %q, %v", got, err)
	}
	if _, err := ParseSkillID("skill_BAD"); err == nil {
		t.Fatal("ParseSkillID(invalid) error = nil")
	}

	if got, err := ParseInstallationID("sinst_0123456789abcdef0123456789abcdef"); err != nil ||
		got.String() != "sinst_0123456789abcdef0123456789abcdef" {
		t.Fatalf("ParseInstallationID() = %q, %v", got, err)
	}
	if _, err := ParseInstallationID("sinst_short"); err == nil {
		t.Fatal("ParseInstallationID(invalid) error = nil")
	}
}
