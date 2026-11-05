// Package localworkspace observes local filesystem paths for the Agent
// application. Authorization and approved-root checks belong to the caller.
package localworkspace

import (
	"context"
	"os"
	"path/filepath"
)

// Verifier resolves paths and reports filesystem types without granting access.
// Its zero value is ready to use.
type Verifier struct{}

func New() Verifier { return Verifier{} }

func (Verifier) ResolvePath(ctx context.Context, path string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(path)
	if ctxErr := ctx.Err(); ctxErr != nil {
		return "", ctxErr
	}
	return resolved, err
}

func (Verifier) IsDirectory(ctx context.Context, path string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	info, err := os.Stat(path)
	if ctxErr := ctx.Err(); ctxErr != nil {
		return false, ctxErr
	}
	if err != nil {
		return false, err
	}
	return info.IsDir(), nil
}

func (Verifier) IsRegularFile(ctx context.Context, path string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	info, err := os.Stat(path)
	if ctxErr := ctx.Err(); ctxErr != nil {
		return false, ctxErr
	}
	if err != nil {
		return false, err
	}
	return info.Mode().IsRegular(), nil
}
