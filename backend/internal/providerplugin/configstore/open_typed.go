package configstore

import (
	"context"
	"fmt"

	v3schema "github.com/orz-i/Mender/backend/internal/schema/v3"
)

// OpenTyped reuses the existing configured Provider/credential-reference
// repository on a verified v3 schema. No old Provider-v2 migration or plugin
// activation is performed. The F4 Runtime owns connection/activation order.
func OpenTyped(ctx context.Context, path string) (*Store, error) {
	db, err := v3schema.OpenContextStore(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("open typed Provider config sqlite: %w", err)
	}
	return &Store{db: db}, nil
}
