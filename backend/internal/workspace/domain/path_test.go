package domain_test

import (
	"strings"
	"testing"

	workspace "github.com/orz-i/Mender/backend/internal/workspace/domain"
)

func TestNormalizeRelativePath(t *testing.T) {
	tests := []struct {
		name      string
		raw       string
		allowRoot bool
		expected  string
		hasErr    bool
	}{
		{name: "simple relative file", raw: "src/main.go", allowRoot: false, expected: "src/main.go"},
		{name: "dot prefixed file", raw: "./src/main.go", allowRoot: false, expected: "src/main.go"},
		{name: "relative with inner back tracking", raw: "a/../b", allowRoot: false, expected: "b"},
		{name: "root dot allowed", raw: ".", allowRoot: true, expected: "."},
		{name: "empty string allowed as root", raw: "", allowRoot: true, expected: "."},
		{name: "root dot disallowed", raw: ".", allowRoot: false, hasErr: true},
		{name: "empty string disallowed", raw: "", allowRoot: false, hasErr: true},
		{name: "surrounding whitespace", raw: " src/main.go", allowRoot: false, hasErr: true},
		{name: "escape with parent", raw: "../secret", allowRoot: false, hasErr: true},
		{name: "absolute unix root", raw: "/etc/passwd", allowRoot: false, hasErr: true},
		{name: "windows backslash root", raw: `\Windows`, allowRoot: false, hasErr: true},
		{name: "windows backslash relative", raw: `foo\bar`, allowRoot: false, hasErr: true},
		{name: "windows drive path", raw: `C:\Users`, allowRoot: false, hasErr: true},
		{name: "windows drive relative", raw: "C:foo", allowRoot: false, hasErr: true},
		{name: "windows UNC path", raw: `\\server\share`, allowRoot: false, hasErr: true},
		{name: "ntfs alternate data stream", raw: "docs/report.txt:secret", allowRoot: false, hasErr: true},
		{name: "contains NUL byte", raw: "src/main.go\x00extra", allowRoot: false, hasErr: true},
		{name: "contains control character", raw: "src/main.go\x1f", allowRoot: false, hasErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := workspace.NormalizeRelativePath(tt.raw, tt.allowRoot)
			if tt.hasErr {
				if err == nil {
					t.Fatalf("NormalizeRelativePath(%q, %v) expected error, got %q", tt.raw, tt.allowRoot, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("NormalizeRelativePath(%q, %v) unexpected error: %v", tt.raw, tt.allowRoot, err)
			}
			if got != tt.expected {
				t.Fatalf("NormalizeRelativePath(%q, %v) = %q, want %q", tt.raw, tt.allowRoot, got, tt.expected)
			}
		})
	}
}

func TestNormalizeRelativePathExceedsMaxBytes(t *testing.T) {
	oversized := strings.Repeat("a", 4097)
	if _, err := workspace.NormalizeRelativePath(oversized, false); err == nil {
		t.Fatal("expected error for oversized path")
	}
}
