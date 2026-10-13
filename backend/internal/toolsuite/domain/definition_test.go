package domain

import (
	"errors"
	"testing"

	platformplugin "github.com/orz-i/Mender/backend/internal/plugin"
)

// A future Jobs suite may be described without being registered or executable.
// Real job-specific ports, grants, lifecycle fencing and Provider tools remain
// out of scope until their independent implementation and verification.
func TestUnregisteredVersionedJobsDefinitionHasNoRunnableTools(t *testing.T) {
	t.Parallel()
	required := platformplugin.CapabilityContract{ID: "job.submit", Version: platformplugin.CapabilityVersion{Major: 1},
		Mode: platformplugin.ExecutionDetached,
		Guarantees: []platformplugin.ExecutionGuarantee{platformplugin.GuaranteeIdempotentSubmit,
			platformplugin.GuaranteeReconnectObservable, platformplugin.GuaranteePersistedResult}}
	definition := Definition{SuiteID: "sandbox.jobs", DisplayName: "Jobs", Description: "Detached execution",
		ResourceKind: SandboxResource, RequiredCapabilities: []Capability{"job.submit"},
		RequiredContracts: []platformplugin.CapabilityContract{required}, Status: "unavailable"}
	if err := definition.Validate(); err != nil {
		t.Fatal(err)
	}
	if err := RequireContracts(definition, SandboxResource, []platformplugin.CapabilityContract{required}); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("inert Jobs definition activated: %v", err)
	}
	definition.ToolSourceKeys = []string{"job.submit"}
	if err := definition.Validate(); !errors.Is(err, ErrInvalid) {
		t.Fatalf("unavailable Jobs exposed tools: %v", err)
	}
	definition.ToolSourceKeys = nil
	definition.RequiredContracts[0].ID = "job.status"
	if err := definition.Validate(); !errors.Is(err, ErrInvalid) {
		t.Fatalf("mismatched requirement permitted: %v", err)
	}
}
