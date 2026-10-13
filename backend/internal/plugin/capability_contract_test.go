package plugin

import (
	"encoding/json"
	"errors"
	"slices"
	"testing"
)

func TestVersionedCapabilityContractsRejectMalformedAndOverclaimedOffers(t *testing.T) {
	t.Parallel()
	base := CapabilityContract{ID: "job.submit", Version: CapabilityVersion{Major: 1}, Mode: ExecutionDetached,
		Guarantees: []ExecutionGuarantee{GuaranteeIdempotentSubmit, GuaranteeReconnectObservable, GuaranteePersistedResult}}
	if err := base.Validate(); err != nil {
		t.Fatal(err)
	}
	for name, change := range map[string]func(*CapabilityContract){
		"empty ID":        func(c *CapabilityContract) { c.ID = "" },
		"bad namespace":   func(c *CapabilityContract) { c.ID = "job" },
		"uppercase ID":    func(c *CapabilityContract) { c.ID = "Job.submit" },
		"invalid syntax":  func(c *CapabilityContract) { c.ID = "job..submit" },
		"invalid version": func(c *CapabilityContract) { c.Version.Major = 0 },
		"future version":  func(c *CapabilityContract) { c.Version.Major = 65536 },
		"unknown mode":    func(c *CapabilityContract) { c.Mode = "continuous" },
		"unknown guarantee": func(c *CapabilityContract) {
			c.Guarantees = append(c.Guarantees, "survive_datacenter_loss")
		},
		"duplicate guarantee": func(c *CapabilityContract) {
			c.Guarantees = append(c.Guarantees, GuaranteePersistedResult)
		},
		"inline detached claim": func(c *CapabilityContract) { c.Mode = ExecutionInline },
	} {
		t.Run(name, func(t *testing.T) {
			value := base.Clone()
			change(&value)
			if err := value.Validate(); !errors.Is(err, ErrInvalidCapabilityContract) {
				t.Fatalf("invalid contract accepted: %+v %v", value, err)
			}
		})
	}
	if err := ValidateContracts([]CapabilityContract{base, base}); !errors.Is(err, ErrInvalidCapabilityContract) {
		t.Fatalf("duplicate job operation accepted: %v", err)
	}
}

func TestVersionedNegotiationHasNoFallbackOrImpliedDurability(t *testing.T) {
	t.Parallel()
	required := []CapabilityContract{{ID: "job.submit", Version: CapabilityVersion{Major: 1, Minor: 2}, Mode: ExecutionDetached,
		Guarantees: []ExecutionGuarantee{GuaranteeIdempotentSubmit, GuaranteeReconnectObservable, GuaranteePersistedResult}}}
	offered := []CapabilityContract{{ID: "job.submit", Version: CapabilityVersion{Major: 1, Minor: 3}, Mode: ExecutionDetached,
		Guarantees: []ExecutionGuarantee{GuaranteePersistedResult, GuaranteeIdempotentSubmit, GuaranteeReconnectObservable, GuaranteeLogCursor}}}
	if err := NegotiateCapabilities(required, offered); err != nil {
		t.Fatal(err)
	}
	for name, change := range map[string]func(*CapabilityContract){
		"older minor":         func(c *CapabilityContract) { c.Version.Minor = 1 },
		"different major":     func(c *CapabilityContract) { c.Version.Major = 2 },
		"not detached":        func(c *CapabilityContract) { c.Mode = ExecutionInline; c.Guarantees = nil },
		"missing guarantee":   func(c *CapabilityContract) { c.Guarantees = []ExecutionGuarantee{GuaranteeLogCursor} },
		"different operation": func(c *CapabilityContract) { c.ID = "process.shell" },
	} {
		t.Run(name, func(t *testing.T) {
			value := offered[0].Clone()
			change(&value)
			if err := NegotiateCapabilities(required, []CapabilityContract{value}); !errors.Is(err, ErrCapabilityIncompatible) {
				t.Fatalf("incompatible offer accepted: %+v %v", value, err)
			}
		})
	}
	if err := NegotiateCapabilities(required, nil); !errors.Is(err, ErrCapabilityIncompatible) {
		t.Fatalf("missing operation accepted: %v", err)
	}
	// A detached task handle with none of the required guarantees is not
	// durable, even if its operation ID and version match.
	noGuarantees := []CapabilityContract{{ID: "job.submit", Version: CapabilityVersion{Major: 1, Minor: 4}, Mode: ExecutionDetached}}
	if err := NegotiateCapabilities(required, noGuarantees); !errors.Is(err, ErrCapabilityIncompatible) {
		t.Fatalf("detached alone granted durable semantics: %v", err)
	}
}

func TestPluginDescriptorContractDeclarationsAreInertAndCloned(t *testing.T) {
	t.Parallel()
	descriptor := validDescriptor(SandboxBackend)
	descriptor.Capabilities = []string{"job.submit"}
	descriptor.CapabilityContracts = []CapabilityContract{{ID: "job.submit", Version: CapabilityVersion{Major: 1}, Mode: ExecutionDetached,
		Guarantees: []ExecutionGuarantee{GuaranteeIdempotentSubmit}}}
	if err := descriptor.Validate(); err != nil {
		t.Fatal(err)
	}
	clone := descriptor.Clone()
	clone.CapabilityContracts[0].Guarantees[0] = GuaranteeCheckpointRestore
	if descriptor.CapabilityContracts[0].Guarantees[0] != GuaranteeIdempotentSubmit {
		t.Fatal("descriptor exposes mutable capability guarantees")
	}
	r := NewRegistry()
	if err := r.Register(descriptor); err != nil {
		t.Fatal(err)
	}
	returned, err := r.Lookup(descriptor.PluginID, descriptor.Extension.ID, descriptor.Extension.Kind)
	if err != nil {
		t.Fatal(err)
	}
	returned.CapabilityContracts[0].Guarantees[0] = GuaranteeCheckpointRestore
	if r.Descriptors()[0].CapabilityContracts[0].Guarantees[0] != GuaranteeIdempotentSubmit {
		t.Fatal("registry exposes mutable versioned metadata")
	}
	raw, err := json.Marshal(descriptor)
	if err != nil {
		t.Fatal(err)
	}
	var decoded Descriptor
	if err := json.Unmarshal(raw, &decoded); err != nil || decoded.Validate() != nil {
		t.Fatalf("contract snapshot roundtrip: %s %v", raw, err)
	}
	missing := descriptor.Clone()
	missing.Capabilities = []string{"file.read"}
	if err := missing.Validate(); err == nil {
		t.Fatal("missing matching descriptor capability accepted")
	}
	duped := descriptor.Clone()
	duped.CapabilityContracts = append(duped.CapabilityContracts, duped.CapabilityContracts[0])
	if err := duped.Validate(); err == nil {
		t.Fatal("duplicate capability contract accepted")
	}
	// Legacy descriptors have no extra contracts: no implicit job claim.
	prior := validDescriptor(ProviderTools)
	if prior.Validate() != nil || !slices.Equal(prior.Capabilities, []string{"execute"}) || len(prior.CapabilityContracts) != 0 {
		t.Fatal("existing Provider metadata contract changed")
	}
}
