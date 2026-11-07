package domain

import (
	"encoding/hex"
	"fmt"
	"strings"
)

// Ref identifies a shared, explicitly registered Workspace. It conveys no
// filesystem authority, directory mount or permission on its own.
type Ref string

const workspaceRefPrefix = "wrk_"

func ParseRef(raw string) (Ref, error) {
	if !strings.HasPrefix(raw, workspaceRefPrefix) ||
		len(raw) != len(workspaceRefPrefix)+32 {
		return "", fmt.Errorf("workspace ref: expected wrk_ plus 32 lowercase hex digits")
	}
	encoded := strings.TrimPrefix(raw, workspaceRefPrefix)
	if encoded != strings.ToLower(encoded) {
		return "", fmt.Errorf("workspace ref: uppercase encoding forbidden")
	}
	if _, err := hex.DecodeString(encoded); err != nil {
		return "", fmt.Errorf("workspace ref: invalid hex: %w", err)
	}
	return Ref(raw), nil
}

func (r Ref) String() string { return string(r) }

func (r Ref) Validate() error {
	_, err := ParseRef(r.String())
	return err
}
