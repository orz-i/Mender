// Package localdelivery supplies local-host metadata for Profile delivery.
package localdelivery

import (
	"context"
	"fmt"
	"path/filepath"
	"runtime"

	profiledelivery "github.com/orz-i/Mender/backend/internal/profile/application/delivery"
)

type Environment struct{ DatabasePath string }

func (e Environment) LocalContext(_ context.Context) (profiledelivery.LocalCLIContext, error) {
	if e.DatabasePath == "" {
		return profiledelivery.LocalCLIContext{}, fmt.Errorf("local database path unavailable")
	}
	absolute, err := filepath.Abs(e.DatabasePath)
	if err != nil {
		return profiledelivery.LocalCLIContext{}, fmt.Errorf("local database path: %w", err)
	}
	return profiledelivery.LocalCLIContext{DatabasePath: absolute, Platform: runtime.GOOS}, nil
}

var _ profiledelivery.CLIEnvironment = Environment{}
