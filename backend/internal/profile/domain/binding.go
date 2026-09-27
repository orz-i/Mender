package domain

import (
	"fmt"
	"regexp"
	"slices"
	"strings"

	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
	credentialdomain "github.com/orz-i/Mender/backend/internal/credential/domain"
)

const (
	maxBindingAliasLength       = 128
	maxBindingTitleLength       = 255
	maxBindingDescriptionLength = 16384
	maxBindingCredentialRefs    = 32
)

var bindingAliasPattern = regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9_.-]*[A-Za-z0-9])?$`)

// PolicyConstraint can only preserve or tighten the global Policy decision.
type PolicyConstraint string

const (
	PolicyInherit         PolicyConstraint = "inherit"
	PolicyRequireApproval PolicyConstraint = "require_approval"
	PolicyDeny            PolicyConstraint = "deny"
)

// ConcurrencyConstraint controls binding-local execution overlap.
type ConcurrencyConstraint string

const (
	ConcurrencyInherit ConcurrencyConstraint = "inherit"
	ConcurrencySerial  ConcurrencyConstraint = "serial"
)

// ToolBindingSpec is the input used to construct one Profile-owned ToolBinding.
type ToolBindingSpec struct {
	ToolID                catalogdomain.ToolID
	Direct                bool
	Alias                 string
	TitleOverride         string
	DescriptionOverride   string
	PolicyConstraint      PolicyConstraint
	ConcurrencyConstraint ConcurrencyConstraint
	CredentialRefs        []credentialdomain.Reference
}

// ToolBinding is the Profile-owned relationship to one canonical Tool.
type ToolBinding struct {
	toolID                catalogdomain.ToolID
	direct                bool
	alias                 string
	titleOverride         string
	descriptionOverride   string
	policyConstraint      PolicyConstraint
	concurrencyConstraint ConcurrencyConstraint
	credentialRefs        []credentialdomain.Reference
}

// NewToolBinding constructs and normalizes one ToolBinding.
func NewToolBinding(spec ToolBindingSpec) (ToolBinding, error) {
	if _, err := catalogdomain.ParseToolID(spec.ToolID.String()); err != nil {
		return ToolBinding{}, fmt.Errorf("tool binding: %w", err)
	}

	alias := strings.TrimSpace(spec.Alias)
	if len(alias) > maxBindingAliasLength {
		return ToolBinding{}, fmt.Errorf("tool binding alias: is too long")
	}
	if alias != "" && !bindingAliasPattern.MatchString(alias) {
		return ToolBinding{}, fmt.Errorf(
			"tool binding alias: must use alphanumeric, underscore, dot, or hyphen and begin/end alphanumeric",
		)
	}

	title := strings.TrimSpace(spec.TitleOverride)
	if len(title) > maxBindingTitleLength {
		return ToolBinding{}, fmt.Errorf("tool binding title override: is too long")
	}

	description := strings.TrimSpace(spec.DescriptionOverride)
	if len(description) > maxBindingDescriptionLength {
		return ToolBinding{}, fmt.Errorf("tool binding description override: is too long")
	}

	policy := spec.PolicyConstraint
	if policy == "" {
		policy = PolicyInherit
	}
	switch policy {
	case PolicyInherit, PolicyRequireApproval, PolicyDeny:
	default:
		return ToolBinding{}, fmt.Errorf("tool binding policy constraint: unknown value %q", policy)
	}

	credentialRefs, err := normalizeCredentialRefs(spec.CredentialRefs)
	if err != nil {
		return ToolBinding{}, err
	}

	concurrency := spec.ConcurrencyConstraint
	if concurrency == "" {
		concurrency = ConcurrencyInherit
	}
	switch concurrency {
	case ConcurrencyInherit, ConcurrencySerial:
	default:
		return ToolBinding{}, fmt.Errorf(
			"tool binding concurrency constraint: unknown value %q",
			concurrency,
		)
	}

	return ToolBinding{
		toolID:                spec.ToolID,
		direct:                spec.Direct,
		alias:                 alias,
		titleOverride:         title,
		descriptionOverride:   description,
		policyConstraint:      policy,
		concurrencyConstraint: concurrency,
		credentialRefs:        credentialRefs,
	}, nil
}

// DefaultToolBinding constructs a behavior-compatible binding from legacy membership state.
func DefaultToolBinding(toolID catalogdomain.ToolID, direct bool) (ToolBinding, error) {
	return NewToolBinding(ToolBindingSpec{
		ToolID:                toolID,
		Direct:                direct,
		PolicyConstraint:      PolicyInherit,
		ConcurrencyConstraint: ConcurrencyInherit,
	})
}

func (b ToolBinding) ToolID() catalogdomain.ToolID {
	return b.toolID
}

func (b ToolBinding) Direct() bool {
	return b.direct
}

func (b ToolBinding) Alias() string {
	return b.alias
}

func (b ToolBinding) TitleOverride() string {
	return b.titleOverride
}

func (b ToolBinding) DescriptionOverride() string {
	return b.descriptionOverride
}

func (b ToolBinding) PolicyConstraint() PolicyConstraint {
	return b.policyConstraint
}

func (b ToolBinding) ConcurrencyConstraint() ConcurrencyConstraint {
	return b.concurrencyConstraint
}

func (b ToolBinding) CredentialRefs() []credentialdomain.Reference {
	return slices.Clone(b.credentialRefs)
}

func normalizeCredentialRefs(
	refs []credentialdomain.Reference,
) ([]credentialdomain.Reference, error) {
	if len(refs) > maxBindingCredentialRefs {
		return nil, fmt.Errorf("tool binding credential refs: too many references")
	}
	result := make([]credentialdomain.Reference, 0, len(refs))
	seen := make(map[credentialdomain.Reference]struct{}, len(refs))
	for _, ref := range refs {
		parsed, err := credentialdomain.ParseReference(ref.String())
		if err != nil {
			return nil, fmt.Errorf("tool binding credential ref: %w", err)
		}
		if _, exists := seen[parsed]; exists {
			return nil, fmt.Errorf("tool binding credential refs: duplicate %s", parsed)
		}
		seen[parsed] = struct{}{}
		result = append(result, parsed)
	}
	return result, nil
}

func (b ToolBinding) withDirect(direct bool) ToolBinding {
	b.direct = direct
	return b
}

// ToolSelection is the legacy membership input retained while adapters migrate to ToolBinding.
// Deprecated: use ToolBindingSpec and NewToolBinding.
type ToolSelection struct {
	ToolID catalogdomain.ToolID
	Direct bool
}
