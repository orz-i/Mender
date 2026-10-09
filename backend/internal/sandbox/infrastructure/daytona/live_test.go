package daytona

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	credentialdomain "github.com/orz-i/Mender/backend/internal/credential/domain"
	sandboxdomain "github.com/orz-i/Mender/backend/internal/sandbox/domain"
)

// Credential bytes exist only in this opt-in test process environment; never
// store them in Mender SQLite, source files, command arguments or diagnostics.
type liveTestResolver struct{}

func (liveTestResolver) Resolve(_ context.Context, ref credentialdomain.Reference) ([]byte, error) {
	if ref.String() != "credential://daytona/api-key" {
		return nil, errors.New("unknown test Credential Reference")
	}
	value := os.Getenv("MENDER_DAYTONA_LIVE_KEY")
	if value == "" {
		return nil, errors.New("missing ephemeral Daytona credential")
	}
	return []byte(value), nil
}

// Opt-in READ-ONLY credential, API and label-list validation. Never allocates
// or deletes a billable resource. No existing Sandbox metadata is printed.
func TestDaytonaLiveReadOnlyAuthentication(t *testing.T) {
	if os.Getenv("MENDER_DAYTONA_LIVE_KEY") == "" {
		t.Skip("explicit ephemeral MENDER_DAYTONA_LIVE_KEY required")
	}
	if len(os.Getenv("MENDER_DAYTONA_LIVE_KEY")) > 4096 {
		t.Fatal("unexpected credential length")
	}
	t.Setenv("DAYTONA_JWT_TOKEN", "")
	t.Setenv("DAYTONA_ORGANIZATION_ID", "")
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()
	factory, err := NewSDKFactory(liveTestResolver{})
	if err != nil {
		t.Fatal(err)
	}
	client, err := factory.Connect(ctx, validTemplate())
	if err != nil {
		t.Fatalf("Daytona SDK connection failed: %v", err)
	}
	defer closeRemote(client)
	labels := map[string]string{labelSandboxID: "sbx_ffffffffffffffffffffffffffffffff", labelGeneration: "1", labelBackend: "daytona"}
	matched, err := client.FindByLabel(ctx, labels)
	if err != nil {
		t.Fatalf("read-only Daytona SDK list request failed: %v", err)
	}
	if len(matched) > 1 {
		t.Fatal("unexpected multiple matches for non-existent test Sandbox label")
	}
	t.Log("Daytona read-only SDK API authentication and label-filtered inventory succeeded")
}

func TestDaytonaEphemeralKeySanity(t *testing.T) {
	value := os.Getenv("MENDER_DAYTONA_LIVE_KEY")
	if value == "" {
		t.Skip("ephemeral Daytona credential required")
	}
	if !strings.HasPrefix(value, "dtn_") {
		t.Fatal("unexpected credential prefix")
	}
}

// Read-only orphan check for a previously failed test. Never lists unrelated
// inventory metadata in logs and never creates resources. Cleanup operates on
// EXACT observed Mender labels, not a caller-supplied arbitrary Sandbox ID.
func TestDaytonaLiveReconcileFailedCreate(t *testing.T) {
	if os.Getenv("MENDER_DAYTONA_LIVE_KEY") == "" {
		t.Skip("explicit ephemeral credential required")
	}
	target := os.Getenv("MENDER_DAYTONA_RECOVER_ID")
	if target == "" {
		t.Skip("prior Sandbox ID required")
	}
	if _, err := sandboxdomain.ParseID(target); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 65*time.Second)
	defer cancel()
	factory, err := NewSDKFactory(liveTestResolver{})
	if err != nil {
		t.Fatal(err)
	}
	client, err := factory.Connect(ctx, validTemplate())
	if err != nil {
		t.Fatal(err)
	}
	defer closeRemote(client)
	labels := map[string]string{labelSandboxID: target, labelGeneration: "1", labelBackend: "daytona"}
	matches, err := client.FindByLabel(ctx, labels)
	if err != nil {
		t.Fatalf("read-only orphan inventory query failed: %v", err)
	}
	t.Logf("previous create exact-labelled remote matches: %d", len(matches))
	deterministicName := "mender-sbx-" + strings.TrimPrefix(target, "sbx_") + "-g1"
	byName, getErr := client.Get(ctx, deterministicName)
	if getErr == nil && byName != nil {
		snapshot, inspectErr := byName.Snapshot(ctx)
		if inspectErr != nil {
			t.Fatalf("orphan resource observation failed: %v", inspectErr)
		}
		if snapshot.Labels[labelSandboxID] != target {
			t.Fatal("unexpected resource identity: refusing cleanup")
		}
		if len(matches) == 0 {
			matches = []RemoteSandbox{byName}
		}
		t.Log("previous test resource was found by deterministic Mender name")
	}
	if len(matches) == 0 {
		t.Log("no previous test resource found by Mender label/name")
		return
	}
	if os.Getenv("MENDER_DAYTONA_LIVE_CLEANUP") != "yes" {
		t.Fatal("matching billable resource found; explicit cleanup flag required")
	}
	for _, remote := range matches {
		snapshot, inspectErr := remote.Snapshot(ctx)
		if inspectErr != nil {
			t.Fatal(inspectErr)
		}
		if snapshot.Labels[labelSandboxID] != target || snapshot.Labels[labelBackend] != "daytona" {
			t.Fatal("cleanup identity mismatch")
		}
		if err := remote.Destroy(ctx); err != nil {
			t.Fatalf("Daytona remote cleanup failed: %v", err)
		}
		t.Log("previous test remote resource successfully deleted")
	}
}
