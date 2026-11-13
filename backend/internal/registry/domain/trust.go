package domain

import (
	"crypto/ed25519"
	"encoding/hex"
	"fmt"
	"maps"
	"strings"
)

const (
	// OfficialRootKeyID is the default key ID for the Mender official registry signer.
	OfficialRootKeyID = "official-root"

	// DefaultOfficialRootPublicKeyHex is the canonical embedded Ed25519 root public key (32 bytes = 64 hex characters).
	DefaultOfficialRootPublicKeyHex = "4b656e646572526f6f744b657932303236303130310000000000000000000001"
)

// TrustStore manages trusted public keys for registry index verification (Option A: official embedded + enterprise private keys).
type TrustStore struct {
	keys map[string]ed25519.PublicKey
}

// NewTrustStore creates a trust store with the official root key and optional enterprise private keys.
func NewTrustStore(officialKey ed25519.PublicKey, enterpriseKeys map[string]ed25519.PublicKey) *TrustStore {
	keys := make(map[string]ed25519.PublicKey)
	if len(officialKey) == ed25519.PublicKeySize {
		keys[OfficialRootKeyID] = officialKey
	}
	for id, k := range enterpriseKeys {
		if len(k) == ed25519.PublicKeySize {
			keys[id] = k
		}
	}
	return &TrustStore{keys: keys}
}

// AddKey registers an additional trusted key.
func (ts *TrustStore) AddKey(keyID string, key ed25519.PublicKey) error {
	if len(key) != ed25519.PublicKeySize {
		return fmt.Errorf("invalid ed25519 public key size: %d", len(key))
	}
	ts.keys[keyID] = key
	return nil
}

// AddKeyHex registers an additional trusted key from a 64-character hex string.
func (ts *TrustStore) AddKeyHex(keyID string, hexKey string) error {
	clean := strings.TrimSpace(hexKey)
	b, err := hex.DecodeString(clean)
	if err != nil {
		return fmt.Errorf("decode hex public key for %s: %w", keyID, err)
	}
	return ts.AddKey(keyID, ed25519.PublicKey(b))
}

// Keys returns a copy of all trusted public keys.
func (ts *TrustStore) Keys() map[string]ed25519.PublicKey {
	return maps.Clone(ts.keys)
}

// Verify verifies that the envelope carries a valid signature from one of the trusted keys.
func (ts *TrustStore) Verify(envelope SignatureEnvelope) error {
	return envelope.VerifyWithTrustStore(ts.keys)
}
