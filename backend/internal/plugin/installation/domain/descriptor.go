package domain

import (
	"encoding/json"
	"fmt"

	platformplugin "github.com/orz-i/Mender/backend/internal/plugin"
)

// VerifiedDescriptor is one immutable host-reviewed extension descriptor snapshot.
type VerifiedDescriptor struct{ descriptor platformplugin.Descriptor }

func NewVerifiedDescriptor(d platformplugin.Descriptor) (VerifiedDescriptor, error) {
	d = d.Clone()
	if err := d.Validate(); err != nil {
		return VerifiedDescriptor{}, fmt.Errorf("verified plugin descriptor: %w", err)
	}
	return VerifiedDescriptor{descriptor: d}, nil
}
func (d VerifiedDescriptor) Descriptor() platformplugin.Descriptor { return d.descriptor.Clone() }
func (d VerifiedDescriptor) Equal(other VerifiedDescriptor) bool {
	a, _ := json.Marshal(d.descriptor)
	b, _ := json.Marshal(other.descriptor)
	return string(a) == string(b)
}
