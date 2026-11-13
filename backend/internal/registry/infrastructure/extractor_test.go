package infrastructure

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func createTestTarGz(t *testing.T, entries []tarEntry) []byte {
	t.Helper()
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)

	for _, e := range entries {
		hdr := &tar.Header{
			Name:     e.name,
			Mode:     e.mode,
			Size:     int64(len(e.body)),
			Typeflag: e.typeflag,
		}
		if hdr.Typeflag == 0 {
			hdr.Typeflag = tar.TypeReg
		}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatalf("WriteHeader(%s) error = %v", e.name, err)
		}
		if len(e.body) > 0 {
			if _, err := tw.Write(e.body); err != nil {
				t.Fatalf("Write(%s) error = %v", e.name, err)
			}
		}
	}

	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

type tarEntry struct {
	name     string
	mode     int64
	typeflag byte
	body     []byte
}

func TestExtractorValidArchive(t *testing.T) {
	t.Parallel()

	archive := createTestTarGz(t, []tarEntry{
		{name: "mender-plugin.json", mode: 0o644, body: []byte(`{"manifestVersion":2}`)},
		{name: "bin/echo", mode: 0o755, body: []byte(`echo binary`)},
	})

	dest := t.TempDir()
	extractor := NewExtractor()
	if err := extractor.ExtractTarGzReader(context.Background(), bytes.NewReader(archive), dest); err != nil {
		t.Fatalf("ExtractTarGzReader() error = %v", err)
	}

	manifestBytes, err := os.ReadFile(filepath.Join(dest, "mender-plugin.json"))
	if err != nil {
		t.Fatalf("ReadFile(mender-plugin.json) error = %v", err)
	}
	if string(manifestBytes) != `{"manifestVersion":2}` {
		t.Fatalf("content mismatch: %s", string(manifestBytes))
	}
}

func TestExtractorZipSlipRejection(t *testing.T) {
	t.Parallel()

	testCases := []string{
		"../escaped.txt",
		"../../escaped.txt",
		"dir/../../escaped.txt",
		"/abs/escaped.txt",
		`\escaped.txt`,
	}

	extractor := NewExtractor()
	for _, tc := range testCases {
		archive := createTestTarGz(t, []tarEntry{
			{name: tc, mode: 0o644, body: []byte(`evil`)},
		})
		dest := t.TempDir()
		err := extractor.ExtractTarGzReader(context.Background(), bytes.NewReader(archive), dest)
		if err == nil || !strings.Contains(err.Error(), "security violation") {
			t.Fatalf("expected security violation for %q, got: %v", tc, err)
		}
	}
}

func TestExtractorSymlinkRejection(t *testing.T) {
	t.Parallel()

	archive := createTestTarGz(t, []tarEntry{
		{name: "symlink", mode: 0o777, typeflag: tar.TypeSymlink, body: nil},
	})
	dest := t.TempDir()
	extractor := NewExtractor()
	err := extractor.ExtractTarGzReader(context.Background(), bytes.NewReader(archive), dest)
	if err == nil || !strings.Contains(err.Error(), "links not permitted") {
		t.Fatalf("expected links not permitted error, got: %v", err)
	}
}
