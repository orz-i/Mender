//go:build windows

package sshremote

func lockFileEx(fd uintptr) error {
	return nil
}

func unlockFile(fd uintptr) error {
	return nil
}
