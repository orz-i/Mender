package domain

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"strings"
	"testing"

	"golang.org/x/crypto/ssh"
)

func encodeTestSSHKey(t *testing.T, key any) []byte {
	t.Helper()
	block, err := ssh.MarshalPrivateKey(key, "fixture")
	if err != nil {
		t.Fatal(err)
	}
	return pem.EncodeToMemory(block)
}

func TestSSHPrivateKeySupportsCommonStrongKeyTypes(t *testing.T) {
	rsa2048, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	rsa1024, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		t.Fatal(err)
	}
	ecdsaKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	_, ed25519Key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name    string
		raw     []byte
		allowed bool
	}{
		{"RSA 2048 OpenSSH", encodeTestSSHKey(t, rsa2048), true},
		{"RSA 2048 PEM PKCS1", pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(rsa2048)}), true},
		{"RSA 1024", encodeTestSSHKey(t, rsa1024), false},
		{"ECDSA P256", encodeTestSSHKey(t, ecdsaKey), true},
		{"Ed25519", encodeTestSSHKey(t, ed25519Key), true},
		{"invalid", []byte("not an SSH key"), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateSSHPrivateKey(tc.raw)
			if (err == nil) != tc.allowed {
				t.Fatalf("ValidateSSHPrivateKey() = %v; allowed=%t", err, tc.allowed)
			}
			normalized, err := NormalizeSSHPrivateKey(tc.raw, nil)
			if tc.allowed {
				if err != nil {
					t.Fatal(err)
				}
				if !bytes.HasPrefix(normalized, []byte("-----BEGIN OPENSSH PRIVATE KEY-----")) {
					t.Fatal("key did not normalize to OpenSSH PEM")
				}
				if err := ValidateSSHPrivateKey(normalized); err != nil {
					t.Fatal(err)
				}
			} else if err == nil {
				t.Fatal("weak or malformed SSH key accepted")
			}
			clear(normalized)
		})
	}
}

func TestSSHPrivateKeyImportsEncryptedOpenSSHWithoutStoringPassphrase(t *testing.T) {
	_, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	passphrase := []byte("generated-test-passphrase")
	block, err := ssh.MarshalPrivateKeyWithPassphrase(private, "test", passphrase)
	if err != nil {
		t.Fatal(err)
	}
	encrypted := pem.EncodeToMemory(block)
	if err := ValidateSSHPrivateKey(encrypted); err == nil || !strings.Contains(err.Error(), "passphrase required") {
		t.Fatalf("expected passphrase-required error, got %v", err)
	}
	if _, err := NormalizeSSHPrivateKey(encrypted, []byte("wrong")); err == nil || !strings.Contains(err.Error(), "incorrect passphrase") {
		t.Fatalf("expected safe incorrect-passphrase error, got %v", err)
	}
	normalized, err := NormalizeSSHPrivateKey(encrypted, passphrase)
	if err != nil {
		t.Fatal(err)
	}
	defer clear(normalized)
	if bytes.Contains(normalized, passphrase) || bytes.Equal(normalized, encrypted) {
		t.Fatal("normalized material contains passphrase or remains encrypted")
	}
	signer, err := ssh.ParsePrivateKey(normalized)
	if err != nil {
		t.Fatal(err)
	}
	original, err := ssh.NewSignerFromKey(private)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(signer.PublicKey().Marshal(), original.PublicKey().Marshal()) {
		t.Fatal("decryption changed SSH key identity")
	}
}

func TestSSHPrivateKeyImportsEncryptedLegacyRSAPEM(t *testing.T) {
	private, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	passphrase := []byte("generated-legacy-rsa-passphrase")
	// #nosec G101 -- test-only generated passphrase, never an application secret.
	block, err := x509.EncryptPEMBlock(rand.Reader, "RSA PRIVATE KEY",
		x509.MarshalPKCS1PrivateKey(private), passphrase, x509.PEMCipherAES256)
	if err != nil {
		t.Fatal(err)
	}
	encrypted := pem.EncodeToMemory(block)
	if _, err := NormalizeSSHPrivateKey(encrypted, nil); err == nil || !strings.Contains(err.Error(), "passphrase required") {
		t.Fatalf("expected passphrase prompt, got %v", err)
	}
	if _, err := NormalizeSSHPrivateKey(encrypted, []byte("wrong")); err == nil {
		t.Fatal("wrong passphrase accepted for encrypted RSA PEM")
	}
	normalized, err := NormalizeSSHPrivateKey(encrypted, passphrase)
	if err != nil {
		t.Fatal(err)
	}
	defer clear(normalized)
	signer, err := ssh.ParsePrivateKey(normalized)
	if err != nil {
		t.Fatal(err)
	}
	if signer.PublicKey().Type() != ssh.KeyAlgoRSA {
		t.Fatalf("unexpected type after encrypted RSA import: %s", signer.PublicKey().Type())
	}
	if bytes.Contains(normalized, passphrase) {
		t.Fatal("passphrase material leaked into normalized private key")
	}
}

func TestSSHPrivateKeyErrorDoesNotEchoSecret(t *testing.T) {
	secret := []byte("TOP-SECRET-UNPARSEABLE-KEY")
	_, err := NormalizeSSHPrivateKey(secret, []byte("TOP-SECRET-PASSPHRASE"))
	if err == nil || strings.Contains(err.Error(), "TOP-SECRET") {
		t.Fatal("untrusted material was accepted or leaked in an error")
	}
	_, err = NormalizeSSHPrivateKey(encodeTestSSHKey(t, mustTestEd25519Key(t)), bytes.Repeat([]byte{'x'}, 4097))
	if err == nil || !strings.Contains(err.Error(), "too long") {
		t.Fatalf("expected passphrase length error, got %v", err)
	}
}

func mustTestEd25519Key(t *testing.T) ed25519.PrivateKey {
	t.Helper()
	_, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return key
}
