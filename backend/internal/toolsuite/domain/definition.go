package domain

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	platformplugin "github.com/orz-i/Mender/backend/internal/plugin"
)

var ErrIncompatible = errors.New("tool suite capabilities unavailable")
var ErrUnavailable = errors.New("tool suite unavailable")
var ErrInvalid = errors.New("invalid tool suite")

type ID string

const (
	AgentDelegation  ID = "agent.delegation"
	SandboxWorkspace ID = "sandbox.workspace"
	SandboxJobs      ID = "sandbox.jobs"
	SandboxBrowser   ID = "sandbox.browser"
)

type ResourceKind string

const (
	AgentResource   ResourceKind = "agent"
	SandboxResource ResourceKind = "sandbox"
)

// Capability is the version-independent operation ID exposed in the existing
// management API. Runtime eligibility uses RequiredContracts instead.
type Capability = platformplugin.CapabilityID

const (
	FileRead       Capability = "file.read"
	FileSearch     Capability = "file.search"
	FileEdit       Capability = "file.edit"
	ProcessShell   Capability = "process.shell"
	AgentTask      Capability = "agent.task"
	BrowserSession Capability = "browser.session"
	JobSubmit      Capability = "job.submit"
	JobObserve     Capability = "job.observe"
	JobCancel      Capability = "job.cancel"
)

// Definition is inert, reviewed host metadata. RequiredCapabilities preserves
// the public management projection. RequiredContracts is the exact host-only
// eligibility contract and MUST contain one matching versioned requirement
// for each public operation. The curated registry, not a plugin or a model,
// is the only source of executable definitions.
type Definition struct {
	SuiteID              ID                                  `json:"suiteId"`
	DisplayName          string                              `json:"displayName"`
	Description          string                              `json:"description"`
	ResourceKind         ResourceKind                        `json:"resourceKind"`
	RequiredCapabilities []Capability                        `json:"requiredCapabilities"`
	RequiredContracts    []platformplugin.CapabilityContract `json:"-"`
	ToolSourceKeys       []string                            `json:"toolSourceKeys"`
	Status               string                              `json:"status"` // builtin, unavailable
	Issue                string                              `json:"issue,omitempty"`
}

func (d Definition) Validate() error {
	if err := platformplugin.CapabilityID(d.SuiteID).Validate(); err != nil {
		return fmt.Errorf("%w: suite ID: %w", ErrInvalid, err)
	}
	if (d.ResourceKind != AgentResource && d.ResourceKind != SandboxResource) ||
		!strings.HasPrefix(string(d.SuiteID), string(d.ResourceKind)+".") ||
		d.DisplayName == "" || d.Description == "" || len(d.RequiredCapabilities) == 0 ||
		(d.Status != "builtin" && d.Status != "unavailable") {
		return ErrInvalid
	}
	if d.Status == "unavailable" && len(d.ToolSourceKeys) != 0 {
		return fmt.Errorf("%w: unavailable tool suite cannot advertise executable Tools", ErrInvalid)
	}
	if !unique(d.RequiredCapabilities) || !unique(d.ToolSourceKeys) ||
		len(d.RequiredContracts) != len(d.RequiredCapabilities) {
		return fmt.Errorf("%w: duplicate or mismatched declarations", ErrInvalid)
	}
	if err := platformplugin.ValidateContracts(d.RequiredContracts); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalid, err)
	}
	for index, c := range d.RequiredCapabilities {
		if err := c.Validate(); err != nil {
			return fmt.Errorf("%w: %w", ErrInvalid, err)
		}
		if d.RequiredContracts[index].ID != c {
			return fmt.Errorf("%w: operation contract does not match %s", ErrInvalid, c)
		}
	}
	return nil
}

func unique[T comparable](items []T) bool {
	seen := map[T]bool{}
	for _, value := range items {
		if seen[value] {
			return false
		}
		seen[value] = true
	}
	return true
}

func (d Definition) Clone() Definition {
	d.RequiredCapabilities = slices.Clone(d.RequiredCapabilities)
	d.RequiredContracts = platformplugin.CloneContracts(d.RequiredContracts)
	d.ToolSourceKeys = slices.Clone(d.ToolSourceKeys)
	return d
}

// RequireContracts negotiates a reviewed suite against *verified* backend
// offers, never arbitrary Descriptor.Capabilities text. Even successful
// negotiation does not provide Principal, Profile, Policy or lease authority.
func RequireContracts(d Definition, resourceKind ResourceKind, actual []platformplugin.CapabilityContract) error {
	if err := d.Validate(); err != nil {
		return err
	}
	if resourceKind != d.ResourceKind {
		return ErrIncompatible
	}
	if d.Status != "builtin" {
		return ErrUnavailable
	}
	if err := platformplugin.NegotiateCapabilities(d.RequiredContracts, actual); err != nil {
		return fmt.Errorf("%w: %w", ErrIncompatible, err)
	}
	return nil
}
