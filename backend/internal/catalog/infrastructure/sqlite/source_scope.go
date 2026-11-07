package sqlite

import (
	"fmt"

	"github.com/orz-i/Mender/backend/internal/catalog/domain"
)

// The current v2 SQLite schema has only genuine Provider Tool identities.
// A typed Agent/Native query cannot inherit the legacy Provider filter or
// borrow a fabricated ProviderID. F4 retires this store in one schema cut.
func requireLegacyProviderSource(source *domain.SourceRef) error {
	if source == nil {
		return nil
	}
	if source.Validate() != nil || source.Kind() != domain.SourceProvider {
		return fmt.Errorf("legacy Catalog requires an exact Provider SourceRef")
	}
	return nil
}
