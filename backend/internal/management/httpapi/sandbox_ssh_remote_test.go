package httpapi

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/orz-i/Mender/backend/internal/bootstrap"
	governancedomain "github.com/orz-i/Mender/backend/internal/governance/domain"
	policyapp "github.com/orz-i/Mender/backend/internal/policy/application"
	policydomain "github.com/orz-i/Mender/backend/internal/policy/domain"
	"golang.org/x/crypto/ssh"
)

func TestSSHRemoteHTTPRejectsMissingCredentialWithoutNetworkOrResource(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	runtime, err := bootstrap.NewRuntime(ctx, bootstrap.RuntimeOptions{DatabasePath: filepath.Join(t.TempDir(), "ssh-http.db")})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = runtime.Close() }()
	server := httptest.NewServer(New(runtime))
	defer server.Close()
	// Host inspection validates its inputs without ever contacting a remote host.
	requestJSON(t, server.Client(), http.MethodPost,
		server.URL+"/api/v1/sandboxes/ssh-remote/host-identity",
		map[string]any{"host": "bad..host", "port": 22}, http.StatusBadRequest)
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	pub, err := ssh.NewPublicKey(priv.Public())
	if err != nil {
		t.Fatal(err)
	}
	// #nosec G101 -- credential://ssh/unavailable is a nonsecret fixture reference, never private key bytes.
	template := map[string]any{
		"name": "Remote Dev", "host": "127.0.0.1", "port": 2222, "user": "agent", "root": "/home/agent/work",
		"hostPublicKey": strings.TrimSpace(string(ssh.MarshalAuthorizedKey(pub))),
		"credentialRef": "credential://ssh/unavailable",
	}
	failed := requestJSON(t, server.Client(), http.MethodPost, server.URL+"/api/v1/sandboxes/ssh-remote", template, http.StatusServiceUnavailable)
	if failed["code"] != "ssh_remote_credential_unavailable" {
		t.Fatalf("missing SSH credential did not return a safe specific stage: %#v", failed)
	}
	if message, ok := failed["message"].(string); !ok || !strings.Contains(message, "ssh_remote_failure:credential_unavailable") ||
		strings.Contains(message, "127.0.0.1") || strings.Contains(message, "/home/agent/work") {
		t.Fatal("SSH provisioning error lost its stage or leaked remote configuration")
	}
	empty := requestJSON(t, server.Client(), http.MethodGet, server.URL+"/api/v1/sandboxes", nil, http.StatusOK)
	if arr, ok := empty["sandboxes"].([]any); !ok || len(arr) != 0 {
		t.Fatalf("unverified SSH created a durable resource: %+v", empty)
	}
	// Root passes validation but, like any other login, cannot provision without the Keychain credential.
	template["user"] = "root"
	requestJSON(t, server.Client(), http.MethodPost, server.URL+"/api/v1/sandboxes/ssh-remote", template, http.StatusServiceUnavailable)
	template["user"] = "agent"
	template["hostPublicKey"] = "ssh-ed25519 invalid"
	requestJSON(t, server.Client(), http.MethodPost, server.URL+"/api/v1/sandboxes/ssh-remote", template, http.StatusBadRequest)
	template["hostPublicKey"] = strings.TrimSpace(string(ssh.MarshalAuthorizedKey(pub)))
	if _, err := runtime.Policy.CreateRule.Execute(ctx, policyapp.RuleInput{
		DisplayName: "deny remote provision", Enabled: true, Decision: policydomain.DecisionDeny,
		Match: policydomain.MatchSpec{Actions: []governancedomain.Action{governancedomain.ActionSandboxProvision}},
	}); err != nil {
		t.Fatal(err)
	}
	requestJSON(t, server.Client(), http.MethodPost, server.URL+"/api/v1/sandboxes/ssh-remote", template, http.StatusForbidden)
	// The same Sandbox provisioning policy also prevents pre-auth identity probes.
	requestJSON(t, server.Client(), http.MethodPost,
		server.URL+"/api/v1/sandboxes/ssh-remote/host-identity",
		map[string]any{"host": "198.51.100.1", "port": 22}, http.StatusForbidden)
	after := requestJSON(t, server.Client(), http.MethodGet, server.URL+"/api/v1/sandboxes", nil, http.StatusOK)
	if arr, ok := after["sandboxes"].([]any); !ok || len(arr) != 0 {
		t.Fatalf("denied SSH created a durable resource: %+v", after)
	}
}
