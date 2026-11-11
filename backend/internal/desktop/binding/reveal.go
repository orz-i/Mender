package binding

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// RevealPath opens the operating system file manager focused on targetPath.
func (s *Service) RevealPath(targetPath string) error {
	if targetPath == "" {
		return errors.New("target path must not be empty")
	}
	info, err := os.Stat(targetPath)
	if err != nil {
		return fmt.Errorf("stat target path: %w", err)
	}

	switch runtime.GOOS {
	case "windows":
		if info.IsDir() {
			return exec.Command("explorer", targetPath).Start()
		}
		return exec.Command("explorer", "/select,", targetPath).Start()
	case "darwin":
		return exec.Command("open", "-R", targetPath).Start()
	default:
		if info.IsDir() {
			return exec.Command("xdg-open", targetPath).Start()
		}
		return exec.Command("xdg-open", filepath.Dir(targetPath)).Start()
	}
}
