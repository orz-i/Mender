package domain

import "testing"

func TestParseReference(t *testing.T) {
	t.Parallel()

	valid := []string{
		"credential://github/work",
		"credential://slack/team-a",
	}
	for _, value := range valid {
		ref, err := ParseReference(value)
		if err != nil {
			t.Errorf("ParseReference(%q) error = %v", value, err)
			continue
		}
		if ref.String() != value {
			t.Errorf("Reference = %q, want %q", ref, value)
		}
	}

	invalid := []string{
		"",
		"https://github/work",
		"credential://GitHub/work",
		"credential://github/",
		"credential://github/work/extra",
		"credential://github/work?token=secret",
	}
	for _, value := range invalid {
		if _, err := ParseReference(value); err == nil {
			t.Errorf("ParseReference(%q) expected error", value)
		}
	}
}
