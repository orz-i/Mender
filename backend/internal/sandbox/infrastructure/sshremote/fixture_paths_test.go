package sshremote

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The in-process SFTP server encodes Windows drive paths as /C:/... . Remote
// protocol paths remain POSIX paths; direct fixture file I/O uses native paths.
func remoteFixturePath(local string) string {
	remote := filepath.ToSlash(local)
	if filepath.VolumeName(local) != "" {
		remote = "/" + remote
	}
	return remote
}

func localFixturePath(remote string) string {
	if len(remote) > 3 && remote[0] == '/' && remote[2] == ':' {
		remote = strings.TrimPrefix(remote, "/")
	}
	return filepath.FromSlash(remote)
}

func requirePOSIXJobFixture(t *testing.T) {
	t.Helper()
	probe := filepath.Join(t.TempDir(), "private-directory")
	if err := os.Mkdir(probe, 0o700); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(probe)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0o077 != 0 {
		t.Skip("local SFTP fixture cannot represent private POSIX permissions on this filesystem; this does not limit Windows SSH clients connecting to a Linux server")
	}
	dir, err := os.Open(probe) // #nosec G304 -- fixed child of this test's private TempDir.
	if err != nil {
		t.Fatal(err)
	}
	err = dir.Sync()
	_ = dir.Close()
	if err != nil {
		t.Skip("local SFTP durability fixture requires directory fsync support; remote Linux SSH behavior is unchanged")
	}
}
