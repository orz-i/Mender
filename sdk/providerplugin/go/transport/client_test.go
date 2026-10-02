package transport

import (
	"bytes"
	"crypto/sha256"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestStartClientVerifiesPluginExecutableBeforeCustomWrapper(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("wrapper marker assertion uses /bin/sh")
	}
	root := t.TempDir()
	executable := filepath.Join(root, "provider")
	if err := os.WriteFile(executable, []byte("not-the-expected-binary"), 0o700); err != nil { // #nosec G306 -- executable fixture must be runnable.
		t.Fatal(err)
	}
	marker := filepath.Join(root, "wrapper-ran")
	wrapper := exec.Command("/bin/sh", "-c", "echo ran > "+shellQuoteForTest(marker)) // #nosec G204 -- marker is test-owned under t.TempDir.

	wrong := sha256.Sum256([]byte("different-binary"))
	_, err := StartClient(ClientOptions{
		Executable:          executable,
		WorkingDirectory:    root,
		Versions:            []uint32{1},
		ExecutableChecksum:  wrong[:],
		StartTimeout:        time.Second,
		PluginLogBufferSize: 1024,
		Command:             wrapper,
	})
	if err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("StartClient() error = %v, want checksum mismatch", err)
	}
	if _, statErr := os.Stat(marker); !os.IsNotExist(statErr) {
		t.Fatalf("custom wrapper ran before executable verification: %v", statErr)
	}
}

func TestVerifyExecutableChecksumAcceptsExactBytes(t *testing.T) {
	root := t.TempDir()
	executable := filepath.Join(root, "provider")
	data := []byte("provider-bytes")
	if err := os.WriteFile(executable, data, 0o700); err != nil { // #nosec G306 -- executable fixture must be runnable.
		t.Fatal(err)
	}
	expected := sha256.Sum256(data)
	if err := verifyExecutableChecksum(executable, expected[:]); err != nil {
		t.Fatal(err)
	}
	wrong := bytes.Repeat([]byte{0x42}, sha256.Size)
	if err := verifyExecutableChecksum(executable, wrong); err == nil {
		t.Fatal("verifyExecutableChecksum accepted wrong checksum")
	}
}

func shellQuoteForTest(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}
