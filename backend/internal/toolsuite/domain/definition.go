package domain

import (
	"errors"
	"fmt"
	"slices"
)

var ErrIncompatible = errors.New("tool suite capabilities unavailable")
var ErrUnavailable = errors.New("tool suite unavailable")
var ErrInvalid = errors.New("invalid tool suite")

type ID string

const (
	AgentDelegation  ID = "agent.delegation"
	SandboxWorkspace ID = "sandbox.workspace"
	SandboxBrowser   ID = "sandbox.browser"
)

type ResourceKind string

const (
	AgentResource   ResourceKind = "agent"
	SandboxResource ResourceKind = "sandbox"
)

type Capability string

const (
	FileRead       Capability = "file.read"
	FileSearch     Capability = "file.search"
	FileEdit       Capability = "file.edit"
	ProcessShell   Capability = "process.shell"
	AgentTask      Capability = "agent.task"
	BrowserSession Capability = "browser.session"
)

// Definition is inert, reviewed host metadata. A planned suite is not an
// executable Tool and cannot be bound merely by listing its capabilities.
type Definition struct {
	SuiteID              ID           `json:"suiteId"`
	DisplayName          string       `json:"displayName"`
	Description          string       `json:"description"`
	ResourceKind         ResourceKind `json:"resourceKind"`
	RequiredCapabilities []Capability `json:"requiredCapabilities"`
	ToolSourceKeys       []string     `json:"toolSourceKeys"`
	Status               string       `json:"status"` // builtin, unavailable
	Issue                string       `json:"issue,omitempty"`
}

func (d Definition) Validate() error {
	if d.SuiteID != AgentDelegation && d.SuiteID != SandboxWorkspace && d.SuiteID != SandboxBrowser ||
		(d.ResourceKind != AgentResource && d.ResourceKind != SandboxResource) ||
		d.DisplayName == "" || d.Description == "" || len(d.RequiredCapabilities) == 0 ||
		(d.Status != "builtin" && d.Status != "unavailable") {
		return ErrInvalid
	}
	if d.Status == "unavailable" && len(d.ToolSourceKeys) != 0 {
		return fmt.Errorf("%w: unavailable tool suite cannot advertise executable Tools", ErrInvalid)
	}
	for _, c := range d.RequiredCapabilities {
		if c != FileRead && c != FileSearch && c != FileEdit && c != ProcessShell && c != AgentTask && c != BrowserSession {
			return fmt.Errorf("%w: unknown capability %q", ErrInvalid, c)
		}
	}
	if !unique(d.RequiredCapabilities) || !unique(d.ToolSourceKeys) {
		return fmt.Errorf("%w: duplicate declarations", ErrInvalid)
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
	d.ToolSourceKeys = slices.Clone(d.ToolSourceKeys)
	return d
}

// RequireCapabilities performs complete-set, typed negotiation: a browser
// capability never substitutes for filesystem, search or process operations.
func RequireCapabilities(d Definition, resourceKind ResourceKind, actual []Capability) error {
	if err := d.Validate(); err != nil {
		return err
	}
	if resourceKind != d.ResourceKind {
		return ErrIncompatible
	}
	if d.Status != "builtin" {
		return ErrUnavailable
	}
	for _, required := range d.RequiredCapabilities {
		if !slices.Contains(actual, required) {
			return fmt.Errorf("%w: %s", ErrIncompatible, required)
		}
	}
	return nil
}
