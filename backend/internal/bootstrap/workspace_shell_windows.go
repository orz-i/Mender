//go:build windows

package bootstrap

import (
	"fmt"
	"os"
	"path/filepath"
)

func workspaceShellProcess(command string) (string, []string, error) {
	executable := os.Getenv("COMSPEC")
	if executable == "" {
		root := os.Getenv("SystemRoot")
		if root == "" {
			return "", nil, fmt.Errorf("workspace shell: COMSPEC and SystemRoot are unavailable")
		}
		executable = filepath.Join(root, "System32", "cmd.exe")
	}
	if !filepath.IsAbs(executable) {
		return "", nil, fmt.Errorf("workspace shell: COMSPEC must be absolute")
	}
	return executable, []string{"/d", "/s", "/c", command}, nil
}
