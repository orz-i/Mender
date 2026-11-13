package domain

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strings"
)

// Signature represents one signature entry in a SignatureEnvelope.
type Signature struct {
	KeyID string `json:"keyId"`
	Sig   string `json:"sig"` // Hex or base64 encoded Ed25519 signature (64 bytes)
}

// SigBytes decodes the signature from hex or base64 into 64 bytes.
func (s Signature) SigBytes() ([]byte, error) {
	clean := strings.TrimSpace(s.Sig)
	if clean == "" {
		return nil, fmt.Errorf("empty signature")
	}

	// Try hex first (128 hex characters for 64 bytes)
	if decoded, err := hex.DecodeString(clean); err == nil && len(decoded) == ed25519.SignatureSize {
		return decoded, nil
	}

	// Try standard base64
	if decoded, err := base64.StdEncoding.DecodeString(clean); err == nil && len(decoded) == ed25519.SignatureSize {
		return decoded, nil
	}

	// Try URL base64 without padding
	if decoded, err := base64.RawURLEncoding.DecodeString(clean); err == nil && len(decoded) == ed25519.SignatureSize {
		return decoded, nil
	}

	return nil, fmt.Errorf("signature is not a valid 64-byte hex or base64 string")
}

// SignatureEnvelope packages a signed CatalogIndex together with its signatures.
type SignatureEnvelope struct {
	Payload    CatalogIndex `json:"payload"`
	Signatures []Signature  `json:"signatures"`
}

// CanonicalPayload returns the RFC 8785 canonical bytes of the enclosed CatalogIndex.
func (e SignatureEnvelope) CanonicalPayload() ([]byte, error) {
	return CanonicalJSON(e.Payload)
}

// VerifyWithKey verifies that at least one signature in the envelope is valid for the given Ed25519 public key.
func (e SignatureEnvelope) VerifyWithKey(key ed25519.PublicKey) error {
	if len(key) != ed25519.PublicKeySize {
		return fmt.Errorf("invalid ed25519 public key size %d", len(key))
	}
	canonical, err := e.CanonicalPayload()
	if err != nil {
		return fmt.Errorf("canonical payload: %w", err)
	}

	for _, sig := range e.Signatures {
		sigBytes, err := sig.SigBytes()
		if err != nil {
			continue
		}
		if ed25519.Verify(key, canonical, sigBytes) {
			return nil
		}
	}
	return ErrInvalidSignature
}

// VerifyWithTrustStore verifies that at least one signature in the envelope matches a trusted public key.
// trustedKeys maps keyID -> ed25519.PublicKey. If a signature's KeyID is in the map, its key is tested.
// If KeyID is not in the map or empty, all trusted keys are checked.
func (e SignatureEnvelope) VerifyWithTrustStore(trustedKeys map[string]ed25519.PublicKey) error {
	if len(trustedKeys) == 0 {
		return fmt.Errorf("%w: trust store is empty", ErrNoTrustedSigner)
	}
	if len(e.Signatures) == 0 {
		return fmt.Errorf("%w: envelope has no signatures", ErrNoTrustedSigner)
	}

	canonical, err := e.CanonicalPayload()
	if err != nil {
		return fmt.Errorf("canonical payload: %w", err)
	}

	verified := false
	for _, sig := range e.Signatures {
		sigBytes, err := sig.SigBytes()
		if err != nil {
			continue
		}

		if specificKey, exists := trustedKeys[sig.KeyID]; exists {
			if ed25519.Verify(specificKey, canonical, sigBytes) {
				verified = true
				break
			}
		} else {
			// Check against all trusted keys
			for _, key := range trustedKeys {
				if ed25519.Verify(key, canonical, sigBytes) {
					verified = true
					break
				}
			}
			if verified {
				break
			}
		}
	}

	if !verified {
		return fmt.Errorf("%w: no valid signature verified against trusted keys", ErrInvalidSignature)
	}
	return nil
}

// SignIndex signs a CatalogIndex using the provided Ed25519 private key and returns a SignatureEnvelope.
func SignIndex(index CatalogIndex, keyID string, privateKey ed25519.PrivateKey) (SignatureEnvelope, error) {
	if len(privateKey) != ed25519.PrivateKeySize {
		return SignatureEnvelope{}, fmt.Errorf("invalid ed25519 private key size %d", len(privateKey))
	}
	canonical, err := index.CanonicalJSON()
	if err != nil {
		return SignatureEnvelope{}, fmt.Errorf("canonicalize index: %w", err)
	}

	sigBytes := ed25519.Sign(privateKey, canonical)
	return SignatureEnvelope{
		Payload: index,
		Signatures: []Signature{
			{
				KeyID: keyID,
				Sig:   hex.EncodeToString(sigBytes),
			},
		},
	}, nil
}
