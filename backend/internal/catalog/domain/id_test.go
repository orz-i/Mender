package domain

import "testing"

func TestParseProviderID(t *testing.T) {
	t.Parallel()

	valid := "prov_0123456789abcdef0123456789abcdef"
	id, err := ParseProviderID(valid)
	if err != nil {
		t.Fatalf("ParseProviderID() error = %v", err)
	}
	if got := id.String(); got != valid {
		t.Fatalf("ProviderID.String() = %q, want %q", got, valid)
	}

	invalid := []string{
		"",
		"tool_0123456789abcdef0123456789abcdef",
		"prov_0123",
		"prov_0123456789ABCDEF0123456789ABCDEF",
		"prov_zzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzz",
	}
	for _, value := range invalid {
		if _, err := ParseProviderID(value); err == nil {
			t.Errorf("ParseProviderID(%q) expected error", value)
		}
	}
}

func TestParseToolID(t *testing.T) {
	t.Parallel()

	valid := "tool_fedcba9876543210fedcba9876543210"
	if _, err := ParseToolID(valid); err != nil {
		t.Fatalf("ParseToolID() error = %v", err)
	}
	if _, err := ParseToolID("prov_fedcba9876543210fedcba9876543210"); err == nil {
		t.Fatal("ParseToolID() expected prefix error")
	}
}
