package domain

import (
	"crypto/rsa"
	"encoding/pem"
	"errors"
	"fmt"

	"golang.org/x/crypto/ssh"
)

const maxSSHPrivateKeyBytes = 64 * 1024

// ValidateSSHPrivateKey accepts supported unencrypted login keys already held
// in the native OS credential store. Only safe error categories are returned.
func ValidateSSHPrivateKey(raw []byte) error {
	_, err := parseSSHPrivateKey(raw, nil)
	return err
}

// NormalizeSSHPrivateKey imports an existing PEM/OpenSSH key. When supplied,
// the passphrase is used only for this operation; the resulting unencrypted
// OpenSSH PEM is stored solely in the native OS keyring, never as metadata.
// The caller owns and must clear the returned bytes after writing them.
func NormalizeSSHPrivateKey(raw, passphrase []byte) ([]byte, error) {
	key, err := parseSSHPrivateKey(raw, passphrase)
	if err != nil {
		return nil, err
	}
	block, err := ssh.MarshalPrivateKey(key, "")
	if err != nil {
		return nil, fmt.Errorf("ssh private key: unsupported key type")
	}
	normalized := pem.EncodeToMemory(block)
	if len(normalized) > maxSSHPrivateKeyBytes {
		clear(normalized)
		return nil, fmt.Errorf("ssh private key: secret exceeds 65536 bytes")
	}
	return normalized, nil
}

func parseSSHPrivateKey(raw, passphrase []byte) (any, error) {
	if len(raw) == 0 || len(raw) > maxSSHPrivateKeyBytes {
		return nil, fmt.Errorf("ssh private key: secret must be 1..65536 bytes")
	}
	if len(passphrase) > 4096 {
		return nil, fmt.Errorf("ssh private key: passphrase too long")
	}
	key, err := ssh.ParseRawPrivateKey(raw)
	if err != nil {
		var missing *ssh.PassphraseMissingError
		if !errors.As(err, &missing) {
			return nil, fmt.Errorf("ssh private key: invalid key format")
		}
		if len(passphrase) == 0 {
			return nil, fmt.Errorf("ssh private key: passphrase required")
		}
		key, err = ssh.ParseRawPrivateKeyWithPassphrase(raw, passphrase)
		if err != nil {
			return nil, fmt.Errorf("ssh private key: incorrect passphrase or invalid encrypted key")
		}
	}
	signer, err := ssh.NewSignerFromKey(key)
	if err != nil {
		return nil, fmt.Errorf("ssh private key: unsupported key type")
	}
	switch signer.PublicKey().Type() {
	case ssh.KeyAlgoED25519, ssh.KeyAlgoECDSA256, ssh.KeyAlgoECDSA384, ssh.KeyAlgoECDSA521:
		return key, nil
	case ssh.KeyAlgoRSA:
		if pub, ok := signer.PublicKey().(ssh.CryptoPublicKey); ok {
			if rsaKey, ok := pub.CryptoPublicKey().(*rsa.PublicKey); ok && rsaKey.N.BitLen() >= 2048 {
				return key, nil
			}
		}
		return nil, fmt.Errorf("ssh private key: RSA key must be at least 2048 bits")
	}
	return nil, fmt.Errorf("ssh private key: unsupported key type")
}
