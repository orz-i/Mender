package domain

import (
	"crypto/rsa"
	"fmt"

	"golang.org/x/crypto/ssh"
)

// ValidateSSHPrivateKey verifies an unencrypted Ed25519/ECDSA or RSA>=3072
// key before it can be stored as credential://ssh/*. Never include secret
// content or parser diagnostics in errors returned to management clients.
func ValidateSSHPrivateKey(raw []byte) error {
	if len(raw) == 0 || len(raw) > 64*1024 {
		return fmt.Errorf("ssh private key: secret must be 1..65536 bytes")
	}
	signer, err := ssh.ParsePrivateKey(raw)
	if err != nil {
		return fmt.Errorf("ssh private key: invalid or encrypted private key")
	}
	switch signer.PublicKey().Type() {
	case ssh.KeyAlgoED25519, ssh.KeyAlgoECDSA256, ssh.KeyAlgoECDSA384, ssh.KeyAlgoECDSA521:
		return nil
	case ssh.KeyAlgoRSA:
		if pub, ok := signer.PublicKey().(ssh.CryptoPublicKey); ok {
			if rsaKey, ok := pub.CryptoPublicKey().(*rsa.PublicKey); ok && rsaKey.N.BitLen() >= 3072 {
				return nil
			}
		}
	}
	return fmt.Errorf("ssh private key: require Ed25519, ECDSA or RSA >=3072-bit key")
}
