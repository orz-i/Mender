package localroot

import (
	"context"
	"os"
	"path/filepath"

	app "github.com/orz-i/Mender/backend/internal/workspace/application"
)

// Verifier checks an existing, canonical directory without creating files,
// mounting a Sandbox, probing a backend or changing filesystem authority.
type Verifier struct{}

func (Verifier) CanonicalLocalRoot(ctx context.Context, root string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if !filepath.IsAbs(root) || filepath.Clean(root) != root {
		return "", app.ErrRegistryInvalid
	}
	canonical, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", err
	}
	if canonical != root {
		return "", app.ErrRegistryInvalid
	}
	info, err := os.Stat(canonical)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", app.ErrRegistryInvalid
	}
	return canonical, nil
}

var _ app.RootVerifier = Verifier{}
