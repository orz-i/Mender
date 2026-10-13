package plugin

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
)

var (
	ErrInvalidCapabilityContract = errors.New("invalid capability contract")
	ErrCapabilityIncompatible    = errors.New("capability contract incompatible")
)

var capabilityIDPattern = regexp.MustCompile(`^[a-z][a-z0-9]*(?:\.[a-z][a-z0-9]*)+$`)

// CapabilityID identifies a specific operation, not a Tool, Sandbox kind or
// authority grant. Contracts are inert metadata until a typed host-reviewed
// executor proves support.
type CapabilityID string

func (id CapabilityID) Validate() error {
	if len(id) == 0 || len(id) > 128 || !capabilityIDPattern.MatchString(string(id)) {
		return fmt.Errorf("%w: invalid capability ID %q", ErrInvalidCapabilityContract, id)
	}
	return nil
}

// CapabilityVersion is independent of package, configuration and extension
// wire protocol versions. Minor revisions are additive within one major.
type CapabilityVersion struct {
	Major uint32 `json:"major"`
	Minor uint32 `json:"minor"`
}

func (v CapabilityVersion) Validate() error {
	if v.Major == 0 || v.Major > 65535 || v.Minor > 65535 {
		return fmt.Errorf("%w: invalid capability version %d.%d", ErrInvalidCapabilityContract, v.Major, v.Minor)
	}
	return nil
}

type ExecutionMode string

const (
	ExecutionInline   ExecutionMode = "inline"
	ExecutionDetached ExecutionMode = "detached"
)

type ExecutionGuarantee string

const (
	GuaranteeIdempotentSubmit    ExecutionGuarantee = "idempotent_submit"
	GuaranteeReconnectObservable ExecutionGuarantee = "reconnect_observable"
	GuaranteePersistedResult     ExecutionGuarantee = "persisted_result"
	GuaranteeLogCursor           ExecutionGuarantee = "log_cursor"
	GuaranteeCancelRequest       ExecutionGuarantee = "cancel_request"
	GuaranteeCheckpointRestore   ExecutionGuarantee = "checkpoint_restore"
)

// CapabilityContract is used as an offer (implemented operations) or as a
// requirement (minimum compatible semantics). It never authorizes invocation.
// An offered minor version must be >= a required minor within the same major;
// execution modes must match, and guarantees are matched by set inclusion.
// Detached does not imply durable, machine-reboot recovery or exactly-once
// side effects without explicitly declared and verified guarantees.
type CapabilityContract struct {
	ID         CapabilityID         `json:"id"`
	Version    CapabilityVersion    `json:"version"`
	Mode       ExecutionMode        `json:"mode"`
	Guarantees []ExecutionGuarantee `json:"guarantees,omitempty"`
}

func (c CapabilityContract) Validate() error {
	if err := c.ID.Validate(); err != nil {
		return err
	}
	if err := c.Version.Validate(); err != nil {
		return err
	}
	if c.Mode != ExecutionInline && c.Mode != ExecutionDetached {
		return fmt.Errorf("%w: unknown execution mode %q", ErrInvalidCapabilityContract, c.Mode)
	}
	if c.Mode == ExecutionInline && len(c.Guarantees) != 0 {
		return fmt.Errorf("%w: detached guarantees on inline capability %s", ErrInvalidCapabilityContract, c.ID)
	}
	if len(c.Guarantees) > 16 {
		return fmt.Errorf("%w: too many guarantees for %s", ErrInvalidCapabilityContract, c.ID)
	}
	seen := map[ExecutionGuarantee]bool{}
	for _, guarantee := range c.Guarantees {
		switch guarantee {
		case GuaranteeIdempotentSubmit, GuaranteeReconnectObservable, GuaranteePersistedResult,
			GuaranteeLogCursor, GuaranteeCancelRequest, GuaranteeCheckpointRestore:
		default:
			return fmt.Errorf("%w: unknown guarantee %q", ErrInvalidCapabilityContract, guarantee)
		}
		if seen[guarantee] {
			return fmt.Errorf("%w: duplicate guarantee %q", ErrInvalidCapabilityContract, guarantee)
		}
		seen[guarantee] = true
	}
	return nil
}

func (c CapabilityContract) Clone() CapabilityContract {
	c.Guarantees = slices.Clone(c.Guarantees)
	return c
}

func CloneContracts(contracts []CapabilityContract) []CapabilityContract {
	if contracts == nil {
		return nil
	}
	result := make([]CapabilityContract, 0, len(contracts))
	for _, c := range contracts {
		result = append(result, c.Clone())
	}
	return result
}

// ValidateContracts rejects multiple claims for one operation. One capability
// has exactly one active version/mode; a distinct mode requires a distinct ID.
func ValidateContracts(contracts []CapabilityContract) error {
	if len(contracts) > 64 {
		return fmt.Errorf("%w: too many capability contracts", ErrInvalidCapabilityContract)
	}
	seen := map[CapabilityID]bool{}
	for _, c := range contracts {
		if err := c.Validate(); err != nil {
			return err
		}
		if seen[c.ID] {
			return fmt.Errorf("%w: duplicate capability %q", ErrInvalidCapabilityContract, c.ID)
		}
		seen[c.ID] = true
	}
	return nil
}

// NegotiateCapabilities compares validated inert declarations only. It does
// not check plugin installation status, backend availability, Principal, grants
// or Policy. Callers must verify the offered contract through a trusted typed
// backend before offering any actual Tool Suite.
func NegotiateCapabilities(required, offered []CapabilityContract) error {
	if err := ValidateContracts(required); err != nil {
		return err
	}
	if err := ValidateContracts(offered); err != nil {
		return err
	}
	available := make(map[CapabilityID]CapabilityContract, len(offered))
	for _, offer := range offered {
		available[offer.ID] = offer
	}
	for _, need := range required {
		got, ok := available[need.ID]
		if !ok || got.Version.Major != need.Version.Major || got.Version.Minor < need.Version.Minor || got.Mode != need.Mode {
			return fmt.Errorf("%w: %s v%d.%d / %s unavailable", ErrCapabilityIncompatible,
				need.ID, need.Version.Major, need.Version.Minor, need.Mode)
		}
		for _, guarantee := range need.Guarantees {
			if !slices.Contains(got.Guarantees, guarantee) {
				return fmt.Errorf("%w: %s lacks %s", ErrCapabilityIncompatible, need.ID, guarantee)
			}
		}
	}
	return nil
}
