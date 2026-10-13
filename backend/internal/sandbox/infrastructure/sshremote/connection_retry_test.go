package sshremote

import (
	"context"
	"errors"
	"net"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	sandboxdomain "github.com/orz-i/Mender/backend/internal/sandbox/domain"
)

func TestSSHConnectRetriesOnlyTransientTCPDial(t *testing.T) {
	template, creds := sshFixture(t)
	var attempts atomic.Int32
	dialer := Dialer{
		Credentials: creds,
		tcpDial: func(ctx context.Context, network, address string) (net.Conn, error) {
			if attempts.Add(1) == 1 {
				return nil, &net.DNSError{Err: "sensitive transient dial diagnostic", IsTimeout: true}
			}
			return (&net.Dialer{}).DialContext(ctx, network, address)
		},
	}
	conn, err := dialer.Connect(context.Background(), template)
	if err != nil {
		t.Fatalf("second bounded attempt should connect with pinned identity: %v", err)
	}
	defer func() { _ = conn.Close() }()
	if attempts.Load() != sshTCPDialAttempts {
		t.Fatalf("dial attempts %d, want %d", attempts.Load(), sshTCPDialAttempts)
	}
	if _, err := conn.SFTP.Stat(template.Root); err != nil {
		t.Fatal(err)
	}
	if sshConnectBudget < 45*time.Second || sshTCPAttemptTimeout < 20*time.Second {
		t.Fatal("SSH resilience regression: shorter deadline than accepted design")
	}
}

func TestSSHConnectExhaustionAndImmediateRefusal(t *testing.T) {
	template, creds := sshFixture(t)
	for _, tc := range []struct {
		name     string
		err      error
		want     ConnectionFailureCode
		attempts int32
	}{
		{"timeout", &net.DNSError{Err: "secret timeout", IsTimeout: true}, FailureNetworkTimeout, 2},
		{"refused", syscall.ECONNREFUSED, FailureNetworkRefused, 1},
		{"not found", errors.New("server internal secret"), FailureNetworkUnreachable, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var attempts atomic.Int32
			dialer := Dialer{Credentials: creds, tcpDial: func(context.Context, string, string) (net.Conn, error) {
				attempts.Add(1)
				return nil, tc.err
			}}
			_, err := dialer.Connect(context.Background(), template)
			expectConnectionFailure(t, err, tc.want)
			if attempts.Load() != tc.attempts {
				t.Fatalf("dial attempts %d, want %d", attempts.Load(), tc.attempts)
			}
			if strings.Contains(err.Error(), "secret") {
				t.Fatal("TCP diagnostic was exposed")
			}
		})
	}
}

func TestSSHConnectCancelledDuringDialDoesNotRetry(t *testing.T) {
	template, creds := sshFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var attempts atomic.Int32
	dialer := Dialer{Credentials: creds, tcpDial: func(ctx context.Context, _, _ string) (net.Conn, error) {
		attempts.Add(1)
		<-ctx.Done()
		return nil, ctx.Err()
	}}
	go func() { time.Sleep(35 * time.Millisecond); cancel() }()
	start := time.Now()
	_, err := dialer.Connect(ctx, template)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled dial should retain context cancellation: %v", err)
	}
	if attempts.Load() != 1 || time.Since(start) > time.Second {
		t.Fatal("SSH retry ignored cancellation or attempted another dial")
	}
}

func TestSSHConnectNeverRetriesAuthenticationOrHostIdentityFailures(t *testing.T) {
	for _, tc := range []struct {
		name    string
		prepare func(*testing.T, *Template, *fixtureCredentials)
		want    ConnectionFailureCode
	}{
		{"host_key_mismatch", func(t *testing.T, template *Template, _ *fixtureCredentials) {
			_, unrelated := testKeys(t)
			template.HostPublicKey = unrelated
		}, FailureHostKeyMismatch},
		{"authentication_failed", func(t *testing.T, _ *Template, creds *fixtureCredentials) {
			key, _ := testKeys(t)
			creds.secret = key
		}, FailureAuthenticationFailed},
		{"workspace_missing", func(_ *testing.T, template *Template, _ *fixtureCredentials) {
			template.Root += "/nonexistent"
		}, FailureWorkspaceMissing},
	} {
		t.Run(tc.name, func(t *testing.T) {
			template, creds := sshFixture(t)
			tc.prepare(t, &template, &creds)
			var attempts atomic.Int32
			dialer := Dialer{Credentials: creds, tcpDial: func(ctx context.Context, network, address string) (net.Conn, error) {
				attempts.Add(1)
				return (&net.Dialer{}).DialContext(ctx, network, address)
			}}
			_, err := dialer.Connect(context.Background(), template)
			expectConnectionFailure(t, err, tc.want)
			if attempts.Load() != 1 {
				t.Fatalf("SSH identity/auth/workspace failure retried %d times", attempts.Load())
			}
			if !errors.Is(err, sandboxdomain.ErrBackendUnavailable) {
				t.Fatal("SSH failure lost its existing backend-unavailable semantics")
			}
		})
	}
}
