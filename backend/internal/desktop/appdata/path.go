package appdata

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const directoryName = "Mender"

func ResolveDatabasePath() (string, error) {
	return resolveDatabasePath(os.Getenv("MENDER_DB"), os.UserConfigDir)
}

func resolveDatabasePath(
	explicit string,
	userConfigDir func() (string, error),
) (string, error) {
	path := strings.TrimSpace(explicit)
	if path == "" {
		base, err := userConfigDir()
		if err != nil {
			return "", fmt.Errorf("resolve user config directory: %w", err)
		}
		path = filepath.Join(base, directoryName, "mender.db")
	}

	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve desktop database path: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(absolute), 0o700); err != nil {
		return "", fmt.Errorf("create desktop database directory: %w", err)
	}
	return absolute, nil
}
