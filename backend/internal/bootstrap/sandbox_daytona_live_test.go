package bootstrap

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	catalogapplication "github.com/orz-i/Mender/backend/internal/catalog/application"
	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
	credentialapplication "github.com/orz-i/Mender/backend/internal/credential/application"
	credentialdomain "github.com/orz-i/Mender/backend/internal/credential/domain"
	gov "github.com/orz-i/Mender/backend/internal/governance/domain"
	invocationdomain "github.com/orz-i/Mender/backend/internal/invocation/domain"
	sand "github.com/orz-i/Mender/backend/internal/sandbox/domain"
	daytonadriver "github.com/orz-i/Mender/backend/internal/sandbox/infrastructure/daytona"
	workspaceplugin "github.com/orz-i/Mender/backend/internal/workspace/infrastructure/plugin"
)

const liveDaytonaTestKeyVariable = "MENDER_DAYTONA_LIVE_KEY"

// Opt-in REAL billable integration. Key exists only in process environment,
// and the persisted Credential points to that environment variable, not secret.
// ONE Daytona Sandbox with 15-minute provider TTL is provisioned and deleted.
// Cleanup checks the exact Mender-generated label, never unrelated resources.
func TestDaytonaLiveRuntimeProviderAndCleanup(t *testing.T) {
	if os.Getenv(liveDaytonaTestKeyVariable) == "" {
		t.Skip("explicit ephemeral Daytona credential required")
	}
	if os.Getenv("MENDER_DAYTONA_LIVE_CREATE") != "yes" {
		t.Skip("explicit live billable-creation opt-in required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()
	t.Setenv(DaytonaExperimentalEnvironment, "enabled")
	t.Setenv("DAYTONA_JWT_TOKEN", "")
	t.Setenv("DAYTONA_ORGANIZATION_ID", "")
	db := filepath.Join(t.TempDir(), "daytona-live-runtime.db")
	runtime, err := NewRuntime(ctx, RuntimeOptions{DatabasePath: db})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = runtime.Close() }()
	_, err = runtime.Credentials.CreateEnvironment.Execute(ctx, credentialapplication.CreateEnvironmentRequest{
		Namespace: "daytona", Name: "live-check", DisplayName: "Ephemeral Daytona E2E credential",
		EnvironmentVariable: liveDaytonaTestKeyVariable,
	})
	if err != nil {
		t.Fatalf("temporary environment credential reference: %v", err)
	}
	ref, err := credentialdomain.NewReference("daytona", "live-check")
	if err != nil {
		t.Fatal(err)
	}
	template := daytonadriver.Template{
		CredentialRef: ref,
		ImageDigest:   "docker.io/library/alpine@sha256:5291449c3df73caf6ed85e649dec1b9e818b39a5d8c871e97afc13e9cd5e8fa8",
		Target:        daytonadriver.TargetUS,
		CPU:           1, MemoryGiB: 2, DiskGiB: 3, TTLMinutes: 15,
	}
	principal := runtime.OwnerPrincipal()
	caller, err := gov.NewContext(gov.OriginInternal, principal, gov.CatalogScope())
	if err != nil {
		t.Fatal(err)
	}
	item, provisionErr := runtime.Sandboxes.CreateDaytonaSandbox(ctx, CreateDaytonaSandboxRequest{Name: "Mender Stage E live validation", Template: template}, caller)
	if item.ID() != "" {
		defer func() {
			cleanupCtx, done := context.WithTimeout(context.Background(), 75*time.Second)
			defer done()
			// Do not use Runtime Policy/temporary DB for best-effort cleanup of
			// partially provisioned, unknown and already-tombstoned remote instances.
			factory, factoryErr := daytonadriver.NewSDKFactory(runtime.Credentials.Resolver)
			if factoryErr != nil {
				t.Errorf("Daytona cleanup factory unavailable: %v", factoryErr)
				return
			}
			client, connectErr := factory.Connect(cleanupCtx, template)
			if connectErr != nil {
				t.Errorf("Daytona cleanup connection unavailable: %v", connectErr)
				return
			}
			defer func() { _ = client.Close(cleanupCtx) }()
			for generation := uint64(1); generation <= 2; generation++ {
				labels := map[string]string{
					"io.mender.sandbox.id":         item.ID().String(),
					"io.mender.sandbox.generation": fmt.Sprintf("%d", generation),
					"io.mender.sandbox.backend":    "daytona",
				}
				matches, findErr := client.FindByLabel(cleanupCtx, labels)
				if findErr != nil {
					t.Errorf("Daytona test cleanup inventory request unavailable: %v", findErr)
					continue
				}
				if len(matches) == 0 {
					// The provider's label search may lag behind an ambiguous
					// Create. A deterministic Mender name is only a locator;
					// the exact ownership labels are verified before delete.
					name := fmt.Sprintf("mender-sbx-%s-g%d", strings.TrimPrefix(item.ID().String(), "sbx_"), generation)
					byName, lookupErr := client.Get(cleanupCtx, name)
					if lookupErr == nil && byName != nil {
						snapshot, inspectErr := byName.Snapshot(cleanupCtx)
						if inspectErr == nil &&
							snapshot.Labels["io.mender.sandbox.id"] == item.ID().String() &&
							snapshot.Labels["io.mender.sandbox.generation"] == fmt.Sprintf("%d", generation) &&
							snapshot.Labels["io.mender.sandbox.backend"] == "daytona" {
							matches = append(matches, byName)
						}
					}
				}
				for _, remote := range matches {
					snapshot, inspectErr := remote.Snapshot(cleanupCtx)
					if inspectErr != nil {
						t.Errorf("Daytona test cleanup cannot verify resource: %v", inspectErr)
						continue
					}
					if snapshot.Labels["io.mender.sandbox.id"] != item.ID().String() {
						t.Error("Daytona cleanup label mismatch")
						continue
					}
					if err := remote.Destroy(cleanupCtx); err != nil {
						t.Errorf("Daytona cleanup failed: %v", err)
					} else {
						t.Log("Daytona live test remote resource deleted")
					}
				}
			}
		}()
	}
	if provisionErr != nil {
		if item.ID() != "" {
			state, _ := runtime.Sandboxes.GetRun(ctx, item.ID())
			t.Fatalf("Daytona provider returned an unverified result; persisted state=%s; error=%v", state.State, provisionErr)
		}
		t.Fatalf("Daytona live provider request denied before allocation: %v", provisionErr)
	}
	run, err := runtime.Sandboxes.GetRun(ctx, item.ID())
	if err != nil || run.State != sand.StateReady {
		t.Fatalf("remote Run not ready: %v %v", run.State, err)
	}
	t.Log("Daytona real sandbox created with pinned TTL, network block and resource constraints")
	rawConfig, err := json.Marshal(workspaceplugin.Config{SandboxID: item.ID().String()})
	if err != nil {
		t.Fatal(err)
	}
	provider, err := runtime.registerConfiguredProvider(ctx, catalogapplication.RegisterProviderRequest{Namespace: "daytona-live", DisplayName: "Daytona Live Workspace"}, workspaceplugin.ID, rawConfig, caller, true)
	if err != nil {
		t.Fatalf("remote Workspace Provider bind: %v", err)
	}
	syncResult, err := runtime.syncConfiguredProviderForOwner(ctx, provider.ID(), caller, true)
	if err != nil || syncResult.Total != 4 {
		t.Fatalf("Daytona Workspace tool sync: %v total=%d", err, syncResult.Total)
	}
	tools, err := runtime.Catalog.store.ListProviderTools(ctx, provider.ID())
	if err != nil {
		t.Fatal(err)
	}
	ids := map[string]catalogdomain.ToolID{}
	for _, tool := range tools {
		ids[tool.SourceKey()] = tool.ID()
	}
	invoke := func(sourceKey, arguments string) map[string]any {
		t.Helper()
		identifier := ids[sourceKey]
		if identifier == "" {
			t.Fatalf("missing %s tool", sourceKey)
		}
		response, execErr := runtime.CallConfiguredTool(ctx, identifier, []byte(arguments), caller)
		if execErr != nil || response.Outcome.Kind() != invocationdomain.OutcomeKindComplete {
			t.Fatalf("Daytona live %s unavailable: outcome=%s err=%v", sourceKey, response.Outcome.Kind(), execErr)
		}
		var output map[string]any
		if err := json.Unmarshal(response.Outcome.Content().Bytes(), &output); err != nil {
			t.Fatal(err)
		}
		return output
	}
	invoke("workspace.edit", `{"path":"stagee.txt","operation":"create","content":"hello Daytona\n"}`)
	read := invoke("workspace.read", `{"path":"stagee.txt"}`)
	if read["content"] != "hello Daytona" {
		t.Fatalf("remote read content mismatch: %+v", read)
	}
	result := invoke("workspace.search", `{"mode":"content","query":"Daytona"}`)
	matches, ok := result["matches"].([]any)
	if !ok || len(matches) == 0 {
		t.Fatalf("remote content search no matches: %+v", result)
	}
	edited := invoke("workspace.edit", `{"path":"stagee.txt","operation":"replace","edits":[{"oldText":"hello","newText":"verified"}]}`)
	if edited["replacements"] != float64(1) {
		t.Fatalf("Daytona remote replace: %+v", edited)
	}
	shell := invoke("workspace.shell", `{"command":"cat stagee.txt"}`)
	if !strings.Contains(fmt.Sprint(shell["stdout"]), "verified Daytona") {
		t.Fatalf("Daytona remote shell file mismatch: %+v", shell)
	}
	invoke("workspace.edit", `{"path":"stagee.txt","operation":"delete"}`)
	t.Log("Daytona live read/search/edit/shell shared remote filesystem verified")
	if _, err := runtime.Sandboxes.ChangeLifecycle(ctx, item.ID(), gov.ActionSandboxDestroy, caller); err == nil {
		t.Fatal("Daytona destroyed with bound Workspace Provider")
	}
	if err := runtime.providerConfigs.Delete(ctx, provider.ID()); err != nil {
		t.Fatal(err)
	}
	stopped, err := runtime.Sandboxes.ChangeLifecycle(ctx, item.ID(), gov.ActionSandboxStop, caller)
	if err != nil || stopped.State != sand.StateStopped {
		t.Fatalf("Daytona Stop failed: state=%s err=%v", stopped.State, err)
	}
	started, err := runtime.Sandboxes.ChangeLifecycle(ctx, item.ID(), gov.ActionSandboxStart, caller)
	if err != nil || started.State != sand.StateReady || started.Generation != 2 {
		t.Fatalf("Daytona Start failed: state=%s err=%v", started.State, err)
	}
	destroyed, err := runtime.Sandboxes.ChangeLifecycle(ctx, item.ID(), gov.ActionSandboxDestroy, caller)
	if err != nil || destroyed.State != sand.StateDestroyed {
		t.Fatalf("Daytona Destroy failed: state=%s err=%v", destroyed.State, err)
	}
	events, err := runtime.Sandboxes.LifecycleEvents(ctx, item.ID(), 10)
	if err != nil || len(events) != 3 {
		t.Fatalf("Daytona lifecycle audit unexpected: count=%d err=%v", len(events), err)
	}
	t.Log("Daytona live Stop/Start/Destroy confirmed, remote resource deleted via governed lifecycle")
}
