//go:build !windows

package bootstrap

func workspaceShellProcess(command string) (string, []string, error) {
	return "/bin/sh", []string{"-lc", command}, nil
}
