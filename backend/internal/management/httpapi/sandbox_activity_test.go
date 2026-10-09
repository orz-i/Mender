package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/orz-i/Mender/backend/internal/bootstrap"
	governancedomain "github.com/orz-i/Mender/backend/internal/governance/domain"
	policyapplication "github.com/orz-i/Mender/backend/internal/policy/application"
	policydomain "github.com/orz-i/Mender/backend/internal/policy/domain"
)

func TestSandboxActivityIncludesPolicyDenialAndLifecycleAfterTombstone(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	runtime, err := bootstrap.NewRuntime(ctx, bootstrap.RuntimeOptions{DatabasePath: filepath.Join(t.TempDir(), "activity.db")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runtime.Close() })
	server := httptest.NewServer(New(runtime))
	t.Cleanup(server.Close)
	created := requestJSON(t, server.Client(), http.MethodPost, server.URL+"/api/v1/sandboxes/local", map[string]any{"name": "Activity sample", "root": t.TempDir(), "isolation": "host"}, http.StatusCreated)
	id := created["sandboxId"].(string)
	endpoint := server.URL + "/api/v1/sandboxes/" + id
	initial := requestJSON(t, server.Client(), http.MethodGet, endpoint+"/activity?limit=1", nil, http.StatusOK)
	if len(initial["events"].([]any)) != 1 || initial["limit"] != float64(1) {
		t.Fatalf("bounded activity %+v", initial)
	}
	rule, err := runtime.Policy.CreateRule.Execute(ctx, policyapplication.RuleInput{DisplayName: "Deny stop", Enabled: true, Decision: policydomain.DecisionDeny, Match: policydomain.MatchSpec{Actions: []governancedomain.Action{governancedomain.ActionSandboxStop}}})
	if err != nil {
		t.Fatal(err)
	}
	requestJSON(t, server.Client(), http.MethodPost, endpoint+"/stop", nil, http.StatusForbidden)
	if err := runtime.Policy.DeleteRule.Execute(ctx, rule.ID()); err != nil {
		t.Fatal(err)
	}
	requestJSON(t, server.Client(), http.MethodPost, endpoint+"/stop", nil, http.StatusOK)
	requestJSON(t, server.Client(), http.MethodPost, endpoint+"/start", nil, http.StatusOK)
	requestJSON(t, server.Client(), http.MethodDelete, endpoint, nil, http.StatusOK)
	result := requestJSON(t, server.Client(), http.MethodGet, endpoint+"/activity?limit=50", nil, http.StatusOK)
	if result["sandboxId"] != id || result["limit"] != float64(50) {
		t.Fatalf("activity identity %+v", result)
	}
	events := result["events"].([]any)
	if len(events) < 8 {
		t.Fatalf("admissions and lifecycle audit missing: %+v", events)
	}
	foundDenial, foundCompletedDestroy := false, false
	previous := time.Now().UTC().Add(5 * time.Minute)
	for _, value := range events {
		entry := value.(map[string]any)
		for _, secretField := range []string{"principalRef", "policyReason", "scopeReference", "arguments", "remoteUrl", "credentialRef", "externalId"} {
			if _, found := entry[secretField]; found {
				t.Fatalf("sensitive field %q exposed in Activity", secretField)
			}
		}
		timestamp, ok := entry["occurredAt"].(string)
		if !ok {
			t.Fatalf("missing time %+v", entry)
		}
		instant, err := time.Parse(time.RFC3339Nano, timestamp)
		if err != nil {
			t.Fatal(err)
		}
		if instant.After(previous) {
			t.Fatalf("activity not newest-first: %s after %s", instant, previous)
		}
		previous = instant
		if entry["kind"] == "admission" && entry["action"] == "sandbox.stop" && entry["status"] == "denied" {
			foundDenial = true
		}
		if entry["kind"] == "lifecycle" && entry["action"] == "sandbox.destroy" && entry["status"] == "completed" && entry["nextState"] == "destroyed" {
			foundCompletedDestroy = true
		}
	}
	if !foundDenial || !foundCompletedDestroy {
		t.Fatalf("required audit events absent: denial=%v destroy=%v", foundDenial, foundCompletedDestroy)
	}
	requestJSON(t, server.Client(), http.MethodGet, endpoint+"/activity?limit=0", nil, http.StatusBadRequest)
	requestJSON(t, server.Client(), http.MethodGet, endpoint+"/activity?limit=101", nil, http.StatusBadRequest)
	requestJSON(t, server.Client(), http.MethodGet, endpoint+"/activity?limit=invalid", nil, http.StatusBadRequest)
	requestJSON(t, server.Client(), http.MethodGet, server.URL+"/api/v1/sandboxes/sbx_00000000000000000000000000000000/activity", nil, http.StatusNotFound)
	requestJSON(t, server.Client(), http.MethodGet, server.URL+"/api/v1/sandboxes/invalid/activity", nil, http.StatusBadRequest)
}
