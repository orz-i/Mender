package domain

import (
	"strings"
	"time"
)

const (
	maxProviderDisplayNameLength = 160
	maxProviderDescriptionLength = 4096
)

// Provider is a registered source of Tools.
type Provider struct {
	id          ProviderID
	namespace   Namespace
	displayName string
	description string
	createdAt   time.Time
	updatedAt   time.Time
}

// NewProvider constructs a Provider while enforcing Catalog invariants.
func NewProvider(
	id ProviderID,
	namespace Namespace,
	displayName string,
	description string,
	now time.Time,
) (Provider, error) {
	if err := validateProviderFields(id, namespace, displayName, description); err != nil {
		return Provider{}, err
	}
	if now.IsZero() {
		return Provider{}, validationError("now", "must not be zero")
	}

	now = now.UTC()
	return Provider{
		id:          id,
		namespace:   namespace,
		displayName: strings.TrimSpace(displayName),
		description: strings.TrimSpace(description),
		createdAt:   now,
		updatedAt:   now,
	}, nil
}

// RestoreProvider rehydrates a persisted Provider while enforcing invariants.
func RestoreProvider(
	id ProviderID,
	namespace Namespace,
	displayName string,
	description string,
	createdAt time.Time,
	updatedAt time.Time,
) (Provider, error) {
	if err := validateProviderFields(id, namespace, displayName, description); err != nil {
		return Provider{}, err
	}
	if createdAt.IsZero() || updatedAt.IsZero() {
		return Provider{}, validationError("timestamps", "must not be zero")
	}
	if updatedAt.Before(createdAt) {
		return Provider{}, validationError("updated_at", "must not be before created_at")
	}

	return Provider{
		id:          id,
		namespace:   namespace,
		displayName: strings.TrimSpace(displayName),
		description: strings.TrimSpace(description),
		createdAt:   createdAt.UTC(),
		updatedAt:   updatedAt.UTC(),
	}, nil
}

func validateProviderFields(
	id ProviderID,
	namespace Namespace,
	displayName string,
	description string,
) error {
	if _, err := ParseProviderID(id.String()); err != nil {
		return err
	}
	if _, err := NewNamespace(namespace.String()); err != nil {
		return err
	}

	displayName = strings.TrimSpace(displayName)
	if displayName == "" {
		return validationError("display_name", "must not be empty")
	}
	if len(displayName) > maxProviderDisplayNameLength {
		return validationError("display_name", "is too long")
	}
	if len(strings.TrimSpace(description)) > maxProviderDescriptionLength {
		return validationError("description", "is too long")
	}

	return nil
}

func (p Provider) ID() ProviderID {
	return p.id
}

func (p Provider) Namespace() Namespace {
	return p.namespace
}

func (p Provider) DisplayName() string {
	return p.displayName
}

func (p Provider) Description() string {
	return p.description
}

func (p Provider) CreatedAt() time.Time {
	return p.createdAt
}

func (p Provider) UpdatedAt() time.Time {
	return p.updatedAt
}
