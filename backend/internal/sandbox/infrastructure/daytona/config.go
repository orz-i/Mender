// Package daytona is the first explicitly governed remote Sandbox driver.
// SDK clients and credentials are never created by tool discovery or placement.
package daytona

import (
	"fmt"
	"regexp"
	"strings"

	credentialdomain "github.com/orz-i/Mender/backend/internal/credential/domain"
	sandboxdomain "github.com/orz-i/Mender/backend/internal/sandbox/domain"
)

const (
	APIURL                = "https://app.daytona.io/api"
	TargetUS              = "us"
	WorkingRoot           = "/workspace"
	MaxActivePerPrincipal = 2
)

var digestPattern = regexp.MustCompile(`^[a-z0-9]+(?:[.-][a-z0-9]+)+(?:/[a-z0-9]+(?:[._-][a-z0-9]+)*){2,}@sha256:[0-9a-f]{64}$`)

// Template is an immutable bounded and non-secret remote contract. TTL is a
// provider-enforced wall-clock lifetime, NOT a monetary spend cap.
type Template struct {
	CredentialRef credentialdomain.Reference
	ImageDigest   string
	Target        string
	CPU           int
	MemoryGiB     int
	DiskGiB       int
	TTLMinutes    int
}

func (t Template) Validate() error {
	if _, err := credentialdomain.ParseReference(t.CredentialRef.String()); err != nil {
		return fmt.Errorf("daytona: invalid Mender Credential Reference: %w", err)
	}
	if len(t.ImageDigest) > 320 || !digestPattern.MatchString(t.ImageDigest) {
		return fmt.Errorf("daytona: require immutable fully qualified lowercase image digest")
	}
	if t.Target != TargetUS {
		return fmt.Errorf("daytona: only reviewed US target supported")
	}
	if t.CPU < 1 || t.CPU > 2 || t.MemoryGiB < 1 || t.MemoryGiB > 4 || t.DiskGiB < 3 || t.DiskGiB > 10 {
		return fmt.Errorf("daytona: CPU/memory/disk quota exceeds reviewed bounds")
	}
	if t.TTLMinutes < 15 || t.TTLMinutes > 120 {
		return fmt.Errorf("daytona: wall-clock TTL must be 15..120 minutes")
	}
	return nil
}
func (t Template) EstimatedMaximumResourceHours() float64 {
	// This is intentionally a conservative COMPUTE-TIME allocation estimate.
	// Actual provider charges can include disk/storage/network/platform fees.
	if t.Validate() != nil {
		return 0
	}
	return float64(t.CPU) * float64(t.TTLMinutes) / 60
}
func ValidateExternalID(raw string) error {
	if raw == "" || raw != strings.TrimSpace(raw) || len(raw) > 256 || strings.ContainsAny(raw, "/?#\x00\\") {
		return fmt.Errorf("daytona: remote instance identity must be bounded and opaque")
	}
	return nil
}
func ValidateSandbox(item sandboxdomain.Sandbox) error {
	if err := item.Validate(); err != nil {
		return err
	}
	if item.Backend() != sandboxdomain.BackendDaytona || item.Isolation() != sandboxdomain.IsolationRemote {
		return sandboxdomain.ErrBackendUnavailable
	}
	return nil
}
