// Package sshremote provides a first-party SSH-backed remote Workspace.
// SSH is transport/authentication only; it does not isolate the remote login
// from other files or processes that the account is allowed to access.
package sshremote

import (
	"context"
	"crypto/rsa"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"path"
	"regexp"
	"strings"
	"time"

	credentialdomain "github.com/orz-i/Mender/backend/internal/credential/domain"
	invocationapp "github.com/orz-i/Mender/backend/internal/invocation/application"
	sandboxdomain "github.com/orz-i/Mender/backend/internal/sandbox/domain"
	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

var hostnamePattern = regexp.MustCompile(`^[a-zA-Z0-9](?:[a-zA-Z0-9.-]{0,251}[a-zA-Z0-9])?$`)
var loginPattern = regexp.MustCompile(`^[a-z_][a-z0-9_-]{0,62}$`)

type Template struct {
	Host          string                     `json:"host"`
	Port          uint16                     `json:"port"`
	User          string                     `json:"user"`
	Root          string                     `json:"root"`
	HostPublicKey string                     `json:"hostPublicKey"`
	CredentialRef credentialdomain.Reference `json:"credentialRef"`
}

func (t Template) Validate() error {
	if len(t.Host) == 0 || len(t.Host) > 253 || (!hostnamePattern.MatchString(t.Host) && net.ParseIP(t.Host) == nil) ||
		strings.Contains(t.Host, "..") || !loginPattern.MatchString(t.User) || t.User == "root" || t.Port == 0 ||
		len(t.Root) > 1024 || !strings.HasPrefix(t.Root, "/") || path.Clean(t.Root) != t.Root || t.Root == "/" ||
		strings.ContainsAny(t.Root, "\x00\r\n") || t.CredentialRef.Namespace() != "ssh" {
		return fmt.Errorf("ssh remote: invalid host, non-root user, port, workspace root or SSH credential reference")
	}
	if _, err := credentialdomain.ParseReference(t.CredentialRef.String()); err != nil {
		return fmt.Errorf("ssh remote: SSH credential reference invalid")
	}
	_, err := parseHostKey(t.HostPublicKey)
	return err
}
func parseHostKey(raw string) (ssh.PublicKey, error) {
	if len(raw) < 45 || len(raw) > 4096 || strings.ContainsAny(raw, "\r\n") {
		return nil, fmt.Errorf("ssh remote: pinned host public key invalid")
	}
	k, comment, opts, rest, err := ssh.ParseAuthorizedKey([]byte(raw))
	if err != nil || len(opts) != 0 || len(rest) != 0 || strings.ContainsAny(comment, "\r\n") {
		return nil, fmt.Errorf("ssh remote: require exactly one pinned Ed25519, ECDSA or RSA host public key")
	}
	switch k.Type() {
	case ssh.KeyAlgoED25519, ssh.KeyAlgoECDSA256, ssh.KeyAlgoECDSA384, ssh.KeyAlgoECDSA521:
		return k, nil
	case ssh.KeyAlgoRSA:
		if pub, ok := k.(ssh.CryptoPublicKey); ok {
			if rsaKey, ok := pub.CryptoPublicKey().(*rsa.PublicKey); ok && rsaKey.N.BitLen() >= 2048 {
				return k, nil
			}
		}
	}
	return nil, fmt.Errorf("ssh remote: pinned host key must be Ed25519, ECDSA or RSA >=2048 bits")
}

type Connection struct {
	SSH  *ssh.Client
	SFTP *sftp.Client
}

func (c *Connection) Close() error {
	if c == nil {
		return nil
	}
	if c.SFTP != nil {
		_ = c.SFTP.Close()
	}
	if c.SSH != nil {
		return c.SSH.Close()
	}
	return nil
}

type Dialer struct {
	Credentials invocationapp.CredentialResolver
}

func (d Dialer) Connect(ctx context.Context, t Template) (*Connection, error) {
	if d.Credentials == nil || t.Validate() != nil {
		return nil, sandboxdomain.ErrBackendUnavailable
	}
	material, err := d.Credentials.Resolve(ctx, t.CredentialRef)
	if err != nil {
		return nil, fmt.Errorf("ssh remote: credential unavailable: %w", sandboxdomain.ErrBackendUnavailable)
	}
	defer clear(material)
	if err = credentialdomain.ValidateSSHPrivateKey(material); err != nil {
		return nil, fmt.Errorf("ssh remote: credential is not a valid SSH key: %w", sandboxdomain.ErrBackendUnavailable)
	}
	signer, err := ssh.ParsePrivateKey(material)
	if err != nil {
		return nil, sandboxdomain.ErrBackendUnavailable
	}
	// RSA keys are valid when using SHA-2 signatures, never the legacy SHA-1 ssh-rsa algorithm.
	if signer.PublicKey().Type() == ssh.KeyAlgoRSA {
		algorithmSigner, ok := signer.(ssh.AlgorithmSigner)
		if !ok {
			return nil, sandboxdomain.ErrBackendUnavailable
		}
		signer, err = ssh.NewSignerWithAlgorithms(algorithmSigner, []string{ssh.KeyAlgoRSASHA512, ssh.KeyAlgoRSASHA256})
		if err != nil {
			return nil, sandboxdomain.ErrBackendUnavailable
		}
	}
	public, err := parseHostKey(t.HostPublicKey)
	if err != nil {
		return nil, sandboxdomain.ErrBackendUnavailable
	}
	endpoint := net.JoinHostPort(t.Host, fmt.Sprint(t.Port))
	ctx, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()
	raw, err := (&net.Dialer{Timeout: 8 * time.Second}).DialContext(ctx, "tcp", endpoint)
	if err != nil {
		return nil, sandboxdomain.ErrBackendUnavailable
	}
	_ = raw.SetDeadline(time.Now().Add(10 * time.Second))
	config := &ssh.ClientConfig{
		User: t.User, Auth: []ssh.AuthMethod{ssh.PublicKeys(signer)},
		HostKeyCallback: ssh.FixedHostKey(public), Timeout: 10 * time.Second,
		HostKeyAlgorithms: pinnedHostKeyAlgorithms(public),
	}
	conn, channels, requests, err := ssh.NewClientConn(raw, endpoint, config)
	if err != nil {
		_ = raw.Close()
		return nil, sandboxdomain.ErrBackendUnavailable
	}
	client := ssh.NewClient(conn, channels, requests)
	sftpClient, err := sftp.NewClient(client)
	if err != nil {
		_ = client.Close()
		return nil, sandboxdomain.ErrBackendUnavailable
	}
	result := &Connection{SSH: client, SFTP: sftpClient}
	if err = ValidateRoot(sftpClient, t.Root); err != nil {
		_ = result.Close()
		return nil, sandboxdomain.ErrBackendUnavailable
	}
	// Keep a bounded deadline through SFTP handshake and root verification.
	// A half-open or malicious SSH server must not block create indefinitely.
	_ = raw.SetDeadline(time.Time{})
	return result, nil
}

// Explicitly exclude SHA-1 ssh-rsa host signatures, even for a pinned RSA key.
func pinnedHostKeyAlgorithms(key ssh.PublicKey) []string {
	if key.Type() == ssh.KeyAlgoRSA {
		return []string{ssh.KeyAlgoRSASHA512, ssh.KeyAlgoRSASHA256}
	}
	return []string{key.Type()}
}

// ValidateRoot does not imply confinement for process.shell. It prevents
// unintentional traversal into symlinked paths for structured SFTP operations.
func ValidateRoot(client interface {
	Lstat(string) (fs.FileInfo, error)
}, root string) error {
	if root == "/" || !strings.HasPrefix(root, "/") || path.Clean(root) != root {
		return fmt.Errorf("ssh remote: invalid workspace root")
	}
	current := ""
	for _, part := range strings.Split(strings.TrimPrefix(root, "/"), "/") {
		current += "/" + part
		info, err := client.Lstat(current)
		if err != nil {
			return err
		}
		if info.Mode()&fs.ModeSymlink != 0 || !info.IsDir() {
			return errors.New("ssh remote: root must be a directory without symbolic links")
		}
	}
	return nil
}
