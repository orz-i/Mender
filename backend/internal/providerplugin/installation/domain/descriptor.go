package domain

import (
	"bytes"
	"fmt"
	"slices"

	providerplugin "github.com/orz-i/Mender/backend/internal/providerplugin"
)

// VerifiedDescriptor is the immutable management-safe runtime descriptor
// snapshot verified for one external plugin installation.
type VerifiedDescriptor struct {
	descriptor providerplugin.Descriptor
}

// NewVerifiedDescriptor validates and snapshots one external plugin descriptor.
func NewVerifiedDescriptor(
	descriptor providerplugin.Descriptor,
) (VerifiedDescriptor, error) {
	descriptor = descriptor.Clone()
	if err := descriptor.Validate(); err != nil {
		return VerifiedDescriptor{}, fmt.Errorf(
			"verified plugin descriptor: %w",
			err,
		)
	}
	if descriptor.ProtocolVersion == 0 {
		return VerifiedDescriptor{}, fmt.Errorf(
			"verified plugin descriptor: protocol version must be positive",
		)
	}
	return VerifiedDescriptor{descriptor: descriptor}, nil
}

// Descriptor returns a defensive copy of the verified descriptor.
func (d VerifiedDescriptor) Descriptor() providerplugin.Descriptor {
	return d.descriptor.Clone()
}

// Equal reports whether two verified descriptors are byte-for-byte equivalent
// across the host-owned descriptor contract.
func (d VerifiedDescriptor) Equal(other VerifiedDescriptor) bool {
	left := d.descriptor
	right := other.descriptor
	return left.ID == right.ID &&
		left.Version == right.Version &&
		left.DisplayName == right.DisplayName &&
		left.Description == right.Description &&
		left.ConfigVersion == right.ConfigVersion &&
		left.ProtocolVersion == right.ProtocolVersion &&
		bytes.Equal(left.ConfigSchema, right.ConfigSchema) &&
		bytes.Equal(left.UISchema, right.UISchema) &&
		slices.Equal(left.Capabilities, right.Capabilities)
}
