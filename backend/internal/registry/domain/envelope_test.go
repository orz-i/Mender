package domain

import (
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"testing"
)

func TestEd25519SignAndVerify(t *testing.T) {
	t.Parallel()

	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey() error = %v", err)
	}

	idx := sampleIndex()
	envelope, err := SignIndex(idx, "test-key-1", priv)
	if err != nil {
		t.Fatalf("SignIndex() error = %v", err)
	}

	// Verify with exact public key
	if err := envelope.VerifyWithKey(pub); err != nil {
		t.Fatalf("VerifyWithKey() error = %v", err)
	}

	// Verify with TrustStore
	trustStore := NewTrustStore(pub, nil)
	trustStore.AddKey("test-key-1", pub)
	if err := trustStore.Verify(envelope); err != nil {
		t.Fatalf("TrustStore.Verify() error = %v", err)
	}

	// Tampered payload must fail verification
	tampered := envelope
	tampered.Payload.Sequence = 9999
	if err := tampered.VerifyWithKey(pub); !errors.Is(err, ErrInvalidSignature) {
		t.Fatalf("expected ErrInvalidSignature for tampered payload, got: %v", err)
	}

	// Different public key must fail verification
	otherPub, _, _ := ed25519.GenerateKey(rand.Reader)
	if err := envelope.VerifyWithKey(otherPub); !errors.Is(err, ErrInvalidSignature) {
		t.Fatalf("expected ErrInvalidSignature for wrong key, got: %v", err)
	}
}

func TestTrustStoreMultiKey(t *testing.T) {
	t.Parallel()

	officialPub, _, _ := ed25519.GenerateKey(rand.Reader)
	corpPub, corpPriv, _ := ed25519.GenerateKey(rand.Reader)

	trustStore := NewTrustStore(officialPub, map[string]ed25519.PublicKey{
		"enterprise-key": corpPub,
	})

	idx := sampleIndex()
	corpEnvelope, err := SignIndex(idx, "enterprise-key", corpPriv)
	if err != nil {
		t.Fatalf("SignIndex() error = %v", err)
	}

	if err := trustStore.Verify(corpEnvelope); err != nil {
		t.Fatalf("TrustStore failed to verify enterprise signature: %v", err)
	}

	// Untrusted key
	_, untrustedPriv, _ := ed25519.GenerateKey(rand.Reader)
	untrustedEnvelope, _ := SignIndex(idx, "untrusted", untrustedPriv)
	if err := trustStore.Verify(untrustedEnvelope); !errors.Is(err, ErrInvalidSignature) {
		t.Fatalf("expected ErrInvalidSignature for untrusted signature, got: %v", err)
	}
}
