package provider

import (
	"slices"
	"testing"
)

func TestCommandEnvironmentFiltersAmbientSecrets(t *testing.T) {
	t.Parallel()

	got, err := commandEnvironment(
		[]string{
			"PATH=/usr/bin",
			"HOME=/home/mender",
			"LANG=en_US.UTF-8",
			"GITHUB_TOKEN=secret",
			"AWS_SECRET_ACCESS_KEY=secret",
			"SSH_AUTH_SOCK=/tmp/agent.sock",
			"MENDER_PRIVATE=secret",
		},
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}

	for _, want := range []string{
		"HOME=/home/mender",
		"LANG=en_US.UTF-8",
		"PATH=/usr/bin",
	} {
		if !slices.Contains(got, want) {
			t.Fatalf("environment %v does not contain %q", got, want)
		}
	}
	for _, forbidden := range []string{
		"GITHUB_TOKEN=secret",
		"AWS_SECRET_ACCESS_KEY=secret",
		"SSH_AUTH_SOCK=/tmp/agent.sock",
		"MENDER_PRIVATE=secret",
	} {
		if slices.Contains(got, forbidden) {
			t.Fatalf("environment unexpectedly contains %q: %v", forbidden, got)
		}
	}
}

func TestCommandEnvironmentAllowsExplicitInjection(t *testing.T) {
	t.Parallel()

	got, err := commandEnvironment(
		[]string{"PATH=/usr/bin", "LANG=en_US.UTF-8"},
		map[string]string{
			"LANG":                  "C",
			"MENDER_EXPLICIT_TOKEN": "scoped",
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	if !slices.Contains(got, "LANG=C") {
		t.Fatalf("environment = %v, want explicit LANG override", got)
	}
	if !slices.Contains(got, "MENDER_EXPLICIT_TOKEN=scoped") {
		t.Fatalf("environment = %v, want explicit injection", got)
	}
	if slices.Contains(got, "LANG=en_US.UTF-8") {
		t.Fatalf("environment retains shadowed ambient value: %v", got)
	}
}
