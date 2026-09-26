package domain

import "testing"

func TestNewNamespace(t *testing.T) {
	t.Parallel()

	valid := []string{"github", "local-1", "a"}
	for _, value := range valid {
		namespace, err := NewNamespace(value)
		if err != nil {
			t.Errorf("NewNamespace(%q) error = %v", value, err)
			continue
		}
		if namespace.String() != value {
			t.Errorf("Namespace.String() = %q, want %q", namespace.String(), value)
		}
	}

	invalid := []string{"", "GitHub", "-github", "github-", "git_hub", "git hub"}
	for _, value := range invalid {
		if _, err := NewNamespace(value); err == nil {
			t.Errorf("NewNamespace(%q) expected error", value)
		}
	}
}
