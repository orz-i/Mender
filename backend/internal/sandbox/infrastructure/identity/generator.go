package identity

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
)

type Generator struct{}

func (Generator) NewAdmissionID() (string, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", fmt.Errorf("sandbox admission id: %w", err)
	}
	return "sadm_" + hex.EncodeToString(raw[:]), nil
}
