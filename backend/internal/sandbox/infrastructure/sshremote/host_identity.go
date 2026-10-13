package sshremote

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

type HostTrustStatus string

const (
	HostTrusted   HostTrustStatus = "trusted"
	HostUntrusted HostTrustStatus = "untrusted"
	HostChanged   HostTrustStatus = "changed"
)

// HostIdentity is public server identity, not a client login key. The key is
// returned only for exact pinning when the operator confirms the fingerprint.
type HostIdentity struct {
	Status        HostTrustStatus `json:"status"`
	Algorithm     string          `json:"algorithm"`
	Fingerprint   string          `json:"fingerprint"`
	HostPublicKey string          `json:"hostPublicKey"`
}

// HostTrustStore reads existing OpenSSH user trust and maintains a separate
// Mender-owned known_hosts. It does not modify the user's SSH configuration.
type HostTrustStore struct {
	ManagedFile string
	UserFile    string

	mu sync.Mutex
}

func NewHostTrustStore(managedFile string) *HostTrustStore {
	userFile := ""
	if home, err := os.UserHomeDir(); err == nil {
		userFile = filepath.Join(home, ".ssh", "known_hosts")
	}
	return &HostTrustStore{ManagedFile: managedFile, UserFile: userFile}
}

func validateSSHHostPort(host string, port uint16) error {
	if port == 0 || len(host) == 0 || len(host) > 253 ||
		(!hostnamePattern.MatchString(host) && net.ParseIP(host) == nil) || strings.Contains(host, "..") {
		return errors.New("ssh remote: invalid host or port")
	}
	return nil
}

// evaluate checks each independent trust source. A matching pin in one file
// must never mask a mismatching or revoked key in the other file.
func (s *HostTrustStore) evaluate(host string, port uint16, key ssh.PublicKey) (HostTrustStatus, []string, error) {
	if s == nil {
		return HostUntrusted, nil, nil
	}
	endpoint := net.JoinHostPort(host, fmt.Sprint(port))
	var preferred []string
	haveTrust := false
	changed := false
	for _, file := range []string{s.UserFile, s.ManagedFile} {
		if file == "" {
			continue
		}
		info, err := os.Lstat(file)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil || !info.Mode().IsRegular() {
			return HostChanged, nil, newConnectionFailure(FailureHostTrustUnavailable)
		}
		cb, err := knownhosts.New(file)
		if err != nil {
			return HostChanged, nil, newConnectionFailure(FailureHostTrustUnavailable)
		}
		err = cb(endpoint, &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: int(port)}, key)
		if err == nil {
			haveTrust = true
			continue
		}
		var keyError *knownhosts.KeyError
		if errors.As(err, &keyError) {
			if len(keyError.Want) == 0 {
				continue
			}
			changed = true
			for _, wanted := range keyError.Want {
				for _, algo := range pinnedHostKeyAlgorithms(wanted.Key) {
					if !containsHostAlgorithm(preferred, algo) {
						preferred = append(preferred, algo)
					}
				}
			}
			continue
		}
		// Revoked certificates, malformed trust, and unsafe files fail closed.
		return HostChanged, nil, newConnectionFailure(FailureHostTrustUnavailable)
	}
	if changed {
		return HostChanged, preferred, nil
	}
	if haveTrust {
		return HostTrusted, nil, nil
	}
	return HostUntrusted, nil, nil
}

func containsHostAlgorithm(algorithms []string, value string) bool {
	for _, item := range algorithms {
		if item == value {
			return true
		}
	}
	return false
}

func (s *HostTrustStore) VerifyPin(host string, port uint16, key ssh.PublicKey) error {
	status, _, err := s.evaluate(host, port, key)
	if err != nil {
		return err
	}
	if status == HostChanged {
		return newConnectionFailure(FailureHostKeyMismatch)
	}
	return nil
}

var errHostKeyCaptured = errors.New("ssh host identity captured before authentication")

// Inspect performs only a bounded SSH transport handshake to observe a
// verified server-signed host key, then stops before any user authentication.
// The observed public key is not *trusted* until a known_hosts check succeeds
// or a local operator explicitly confirms the displayed fingerprint.
func (s *HostTrustStore) Inspect(ctx context.Context, host string, port uint16) (HostIdentity, error) {
	ctx, cancel := context.WithTimeout(ctx, sshConnectBudget)
	defer cancel()
	if err := validateSSHHostPort(host, port); err != nil {
		return HostIdentity{}, err
	}
	strongAlgorithms := []string{
		ssh.KeyAlgoED25519,
		ssh.KeyAlgoECDSA256, ssh.KeyAlgoECDSA384, ssh.KeyAlgoECDSA521,
		ssh.KeyAlgoRSASHA512, ssh.KeyAlgoRSASHA256,
	}
	public, err := inspectServerHostKey(ctx, host, port, strongAlgorithms)
	if err != nil {
		return HostIdentity{}, err
	}
	keyText := strings.TrimSpace(string(ssh.MarshalAuthorizedKey(public)))
	if _, err := parseHostKey(keyText); err != nil {
		return HostIdentity{}, newConnectionFailure(FailureSSHHandshakeFailed)
	}
	status, preferred, err := s.evaluate(host, port, public)
	if err != nil {
		return HostIdentity{}, err
	}
	if status == HostChanged && len(preferred) > 0 {
		// If OpenSSH previously trusted RSA but the server also advertises
		// Ed25519, retry using its *known* key algorithm rather than falsely
		// reporting key rotation. Never offer a weaker SHA-1/DSA signature.
		trustedCandidates := make([]string, 0, len(preferred))
		for _, algorithm := range preferred {
			if containsHostAlgorithm(strongAlgorithms, algorithm) {
				trustedCandidates = append(trustedCandidates, algorithm)
			}
		}
		if len(trustedCandidates) > 0 {
			knownKey, secondErr := inspectServerHostKey(ctx, host, port, trustedCandidates)
			if secondErr == nil {
				candidateText := strings.TrimSpace(string(ssh.MarshalAuthorizedKey(knownKey)))
				if _, validErr := parseHostKey(candidateText); validErr == nil {
					candidateStatus, _, trustErr := s.evaluate(host, port, knownKey)
					if trustErr != nil {
						return HostIdentity{}, trustErr
					}
					status = candidateStatus
					public = knownKey
					keyText = candidateText
				}
			}
		}
	}
	return HostIdentity{
		Status: status, Algorithm: public.Type(),
		Fingerprint:   ssh.FingerprintSHA256(public),
		HostPublicKey: keyText,
	}, nil
}

func inspectServerHostKey(ctx context.Context, host string, port uint16, algorithms []string) (ssh.PublicKey, error) {
	ctx, cancel := context.WithTimeout(ctx, sshConnectBudget)
	defer cancel()
	endpoint := net.JoinHostPort(host, fmt.Sprint(port))
	// Reuse the same bounded, pre-auth TCP retry policy as SSH provisioning;
	// inspecting a new host must not retain the old 7-second dial limit.
	raw, err := (Dialer{}).connectTCP(ctx, endpoint)
	if err != nil {
		return nil, err
	}
	defer func() { _ = raw.Close() }()
	deadline, ok := ctx.Deadline()
	if !ok || raw.SetDeadline(deadline) != nil {
		return nil, newConnectionFailure(FailureSSHHandshakeFailed)
	}
	stopCancellationClose := context.AfterFunc(ctx, func() { _ = raw.Close() })
	defer stopCancellationClose()
	var public ssh.PublicKey
	cfg := &ssh.ClientConfig{
		User: "mender-host-identity-probe", Timeout: sshConnectBudget,
		HostKeyAlgorithms: algorithms,
		HostKeyCallback: func(_ string, _ net.Addr, key ssh.PublicKey) error {
			public = key
			return errHostKeyCaptured // abort *before* any login/authentication
		},
	}
	_, _, _, err = ssh.NewClientConn(raw, endpoint, cfg)
	if public == nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if err != nil {
			var netErr net.Error
			if errors.Is(err, context.DeadlineExceeded) || errors.As(err, &netErr) && netErr.Timeout() {
				return nil, newConnectionFailure(FailureNetworkTimeout)
			}
		}
		return nil, newConnectionFailure(FailureSSHHandshakeFailed)
	}
	if !errors.Is(err, errHostKeyCaptured) {
		return nil, newConnectionFailure(FailureSSHHandshakeFailed)
	}
	return public, nil
}

func (s *HostTrustStore) RememberPinned(host string, port uint16, keyLine string) error {
	key, err := parseHostKey(keyLine)
	if err != nil {
		return newConnectionFailure(FailureHostTrustUnavailable)
	}
	return s.Remember(host, port, key)
}

// Remember persists a cryptographically verified pinned host only after an
// admitted connection succeeds. Do not store a remote key before explicit
// first-use confirmation, and never edit ~/.ssh/known_hosts.
func (s *HostTrustStore) Remember(host string, port uint16, key ssh.PublicKey) error {
	if s == nil || s.ManagedFile == "" {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.VerifyPin(host, port, key); err != nil {
		return err
	}
	// The managed file is an append-only trust record, not an event log.
	// Do not duplicate a pin each time the same SSH host is provisioned.
	managedOnly := &HostTrustStore{ManagedFile: s.ManagedFile}
	if status, _, err := managedOnly.evaluate(host, port, key); err != nil {
		return err
	} else if status == HostTrusted {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(s.ManagedFile), 0700); err != nil {
		return newConnectionFailure(FailureHostTrustUnavailable)
	}
	if st, err := os.Lstat(s.ManagedFile); err == nil {
		if !st.Mode().IsRegular() {
			return newConnectionFailure(FailureHostTrustUnavailable)
		}
	} else if !errors.Is(err, fs.ErrNotExist) {
		return newConnectionFailure(FailureHostTrustUnavailable)
	}
	file, err := os.OpenFile(s.ManagedFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return newConnectionFailure(FailureHostTrustUnavailable)
	}
	defer func() { _ = file.Close() }()
	// Permission hardening for existing managed-file installations.
	if err := file.Chmod(0600); err != nil {
		return newConnectionFailure(FailureHostTrustUnavailable)
	}
	line := knownhosts.Line([]string{net.JoinHostPort(host, fmt.Sprint(port))}, key) + "\n"
	if _, err := file.WriteString(line); err != nil {
		return newConnectionFailure(FailureHostTrustUnavailable)
	}
	if err := file.Sync(); err != nil {
		return newConnectionFailure(FailureHostTrustUnavailable)
	}
	return nil
}
