package daytona

import (
	"strings"
	"testing"

	credentialdomain "github.com/orz-i/Mender/backend/internal/credential/domain"
)

const testedDigest = "docker.io/library/python@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func validTemplate() Template {
	return Template{CredentialRef: credentialdomain.Reference("credential://daytona/api-key"), ImageDigest: testedDigest, Target: TargetUS, CPU: 1, MemoryGiB: 2, DiskGiB: 3, TTLMinutes: 30}
}
func TestDaytonaImmutableTemplateAndMandatoryProviderTTL(t *testing.T) {
	good := validTemplate()
	if err := good.Validate(); err != nil {
		t.Fatal(err)
	}
	if h := good.EstimatedMaximumResourceHours(); h != 0.5 {
		t.Fatalf("unexpected resource-hours estimate: %v", h)
	}
	tests := []struct {
		name   string
		change func(*Template)
	}{
		{"empty credential", func(t *Template) { t.CredentialRef = "" }},
		{"secret supplied as ref", func(t *Template) { t.CredentialRef = "API_SECRET_VALUE" }},
		{"untrusted target", func(t *Template) { t.Target = "eu" }},
		{"image tag", func(t *Template) { t.ImageDigest = "python:latest" }},
		{"image alias", func(t *Template) { t.ImageDigest = "library/python@sha256:" + strings.Repeat("0", 64) }},
		{"image uppercase", func(t *Template) { t.ImageDigest = "docker.io/library/python@sha256:" + strings.Repeat("F", 64) }},
		{"zero ttl", func(t *Template) { t.TTLMinutes = 0 }},
		{"unbounded ttl", func(t *Template) { t.TTLMinutes = 10000 }},
		{"zero cpu", func(t *Template) { t.CPU = 0 }},
		{"excess cpu", func(t *Template) { t.CPU = 3 }},
		{"excess memory", func(t *Template) { t.MemoryGiB = 9 }},
		{"excess disk", func(t *Template) { t.DiskGiB = 12 }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			v := good
			test.change(&v)
			if err := v.Validate(); err == nil {
				t.Fatalf("unsafe template accepted %+v", v)
			}
			if v.EstimatedMaximumResourceHours() != 0 {
				t.Fatal("invalid plan estimated as admissible")
			}
		})
	}
}
func TestDaytonaExternalIDMustRemainOpaque(t *testing.T) {
	for _, s := range []string{"", " ", "id/path", "https://metadata.internal/", "with?query", "id#fragment", strings.Repeat("a", 260)} {
		if err := ValidateExternalID(s); err == nil {
			t.Fatalf("accepted %q", s)
		}
	}
	if err := ValidateExternalID("daytona-remote-123"); err != nil {
		t.Fatal(err)
	}
}
