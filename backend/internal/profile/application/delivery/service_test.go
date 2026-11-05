package delivery

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	profile "github.com/orz-i/Mender/backend/internal/profile/domain"
	skillapp "github.com/orz-i/Mender/backend/internal/skill/application"
	skill "github.com/orz-i/Mender/backend/internal/skill/domain"
)

type profilesFixture struct {
	snapshot  ProfileSnapshot
	getErr    error
	revisions int
	revision  func(int) (uint64, error)
}

func (p *profilesFixture) Snapshot(context.Context, profile.Slug) (ProfileSnapshot, error) {
	return p.snapshot, p.getErr
}
func (p *profilesFixture) Revision(context.Context, profile.ProfileID) (uint64, error) {
	p.revisions++
	if p.revision != nil {
		return p.revision(p.revisions)
	}
	return p.snapshot.Revision, nil
}

type sourceFixture func(context.Context, skill.InstallationID) (SkillStatus, error)

func (f sourceFixture) GetDeployable(ctx context.Context, id skill.InstallationID) (SkillStatus, error) {
	return f(ctx, id)
}

type deploymentsFixture struct {
	deploy   func(context.Context, skill.InstallationID, skill.DeploymentTarget, string, string, uint64) (skill.Deployment, error)
	undeploy func(context.Context, skill.DeploymentID) error
}

func (d deploymentsFixture) DeployForProfile(ctx context.Context, id skill.InstallationID, target skill.DeploymentTarget, root, owner string, revision uint64) (skill.Deployment, error) {
	return d.deploy(ctx, id, target, root, owner, revision)
}
func (d deploymentsFixture) Undeploy(ctx context.Context, id skill.DeploymentID) error {
	return d.undeploy(ctx, id)
}

type recordsFixture struct {
	get  func(context.Context, skill.DeploymentID) (skill.Deployment, error)
	list func(context.Context, string) ([]skillapp.DeploymentInspection, error)
}

func (r recordsFixture) GetDeployment(ctx context.Context, id skill.DeploymentID) (skill.Deployment, error) {
	return r.get(ctx, id)
}
func (r recordsFixture) ListOwnedDeployments(ctx context.Context, id string) ([]skillapp.DeploymentInspection, error) {
	return r.list(ctx, id)
}

type exporterFixture func(context.Context, skillapp.ExportAgentPluginRequest) error

func (f exporterFixture) ExportAgentPlugin(ctx context.Context, r skillapp.ExportAgentPluginRequest) error {
	return f(ctx, r)
}

type environmentFixture func(context.Context) (LocalCLIContext, error)

func (f environmentFixture) LocalContext(ctx context.Context) (LocalCLIContext, error) { return f(ctx) }

func selectedProfile() ProfileSnapshot {
	return ProfileSnapshot{ID: "prof_00000000000000000000000000000001", Slug: "delivery", Revision: 7,
		Skills: []skill.InstallationID{
			"sinst_00000000000000000000000000000001",
			"sinst_00000000000000000000000000000002",
			"sinst_00000000000000000000000000000003",
		}, HasTools: true}
}
func trustedSource() sourceFixture {
	return func(_ context.Context, id skill.InstallationID) (SkillStatus, error) {
		return SkillStatus{Name: id.String(), Trusted: true, Enabled: true}, nil
	}
}
func deploymentRecord(t *testing.T, index int, id skill.InstallationID, root, owner string, revision uint64) skill.Deployment {
	t.Helper()
	item, err := skill.NewDeployment(skill.DeploymentID(fmt.Sprintf("sdep_%032x", index)), id,
		skill.DeploymentAgentsProject, root, filepath.Join(root, fmt.Sprintf("skill-%d", index)), "sha256:"+strings.Repeat("a", 64), time.Unix(123, 0))
	if err != nil {
		t.Fatal(err)
	}
	if owner != "" {
		item, err = item.WithProfile(owner, revision)
		if err != nil {
			t.Fatal(err)
		}
	}
	return item
}

func TestDeploymentPreflightsAllSelectedSkillsBeforeAnyWrite(t *testing.T) {
	for _, kind := range []string{"untrusted", "disabled", "duplicate", "missing"} {
		t.Run(kind, func(t *testing.T) {
			profiles := &profilesFixture{snapshot: selectedProfile()}
			checked, written := 0, 0
			unavailable := errors.New("selected installation unavailable")
			source := sourceFixture(func(context.Context, skill.InstallationID) (SkillStatus, error) {
				checked++
				status := SkillStatus{Name: fmt.Sprint(checked), Trusted: true, Enabled: true}
				if kind == "duplicate" {
					status.Name = "same"
				}
				if checked == 2 {
					switch kind {
					case "untrusted":
						status.Trusted = false
					case "disabled":
						status.Enabled = false
					case "missing":
						return status, unavailable
					}
				}
				return status, nil
			})
			deployments := deploymentsFixture{deploy: func(context.Context, skill.InstallationID, skill.DeploymentTarget, string, string, uint64) (skill.Deployment, error) {
				written++
				return skill.Deployment{}, nil
			}}
			service := NewService(profiles, source, deployments, nil, nil, nil)
			result, err := service.DeployProfileSkills(context.Background(), profiles.snapshot.Slug, skill.DeploymentAgentsProject, "unused root")
			if err == nil || result != nil || checked != 2 || written != 0 || profiles.revisions != 0 {
				t.Fatalf("preflight escaped: result=%v err=%v checked=%d written=%d revisions=%d", result, err, checked, written, profiles.revisions)
			}
			if kind == "missing" && !errors.Is(err, unavailable) {
				t.Fatalf("preflight lost cause: %v", err)
			}
		})
	}
}

func TestDeploymentChecksRevisionBeforeEveryWriteAndAfterFinalWrite(t *testing.T) {
	for _, changedAt := range []int{0, 1, 2, 4} {
		t.Run(fmt.Sprint(changedAt), func(t *testing.T) {
			snapshot := selectedProfile()
			profiles := &profilesFixture{snapshot: snapshot}
			profiles.revision = func(call int) (uint64, error) {
				if call == changedAt {
					return snapshot.Revision + 1, nil
				}
				return snapshot.Revision, nil
			}
			root := t.TempDir()
			var created, removed []skill.DeploymentID
			deployments := deploymentsFixture{
				deploy: func(_ context.Context, id skill.InstallationID, target skill.DeploymentTarget, gotRoot, owner string, revision uint64) (skill.Deployment, error) {
					if target != skill.DeploymentAgentsProject || gotRoot != root || owner != snapshot.ID.String() || revision != snapshot.Revision {
						t.Fatalf("deployment authority changed: %s %q %q %d", target, gotRoot, owner, revision)
					}
					item := deploymentRecord(t, len(created)+1, id, root, owner, revision)
					created = append(created, item.ID())
					return item, nil
				},
				undeploy: func(_ context.Context, id skill.DeploymentID) error { removed = append(removed, id); return nil },
			}
			service := NewService(profiles, trustedSource(), deployments, nil, nil, nil)
			result, err := service.DeployProfileSkills(context.Background(), snapshot.Slug, skill.DeploymentAgentsProject, root)
			if changedAt == 0 {
				if err != nil || len(result) != 3 || profiles.revisions != 4 || len(removed) != 0 {
					t.Fatalf("successful snapshot changed: %+v %v checks=%d removed=%v", result, err, profiles.revisions, removed)
				}
				for _, item := range result {
					if item.Drift != skill.DeploymentCurrent || item.CurrentDigest != item.Deployment.Digest() || item.Deployment.ProfileRevision() != snapshot.Revision {
						t.Fatalf("snapshot result changed: %+v", item)
					}
				}
			} else {
				want := slices.Clone(created)
				slices.Reverse(want)
				if err == nil || result != nil || len(created) != changedAt-1 || profiles.revisions != changedAt || !slices.Equal(removed, want) {
					t.Fatalf("revision fence failed: result=%v err=%v created=%v removed=%v checks=%d", result, err, created, removed, profiles.revisions)
				}
			}
		})
	}
}

type compensationContextKey struct{}

func TestCancelledDeliveryCompensatesOnlyNewRecordsInReverseWithoutCancellation(t *testing.T) {
	snapshot := selectedProfile()
	profiles := &profilesFixture{snapshot: snapshot}
	ctx, cancel := context.WithCancel(context.WithValue(context.Background(), compensationContextKey{}, "delivery"))
	defer cancel()
	cleanupErr := errors.New("first cleanup failed")
	var created, removed []skill.DeploymentID
	root := t.TempDir()
	deployments := deploymentsFixture{
		deploy: func(_ context.Context, id skill.InstallationID, _ skill.DeploymentTarget, _ string, owner string, revision uint64) (skill.Deployment, error) {
			item := deploymentRecord(t, len(created)+1, id, root, owner, revision)
			created = append(created, item.ID())
			if len(created) == 2 {
				cancel()
			}
			return item, nil
		},
		undeploy: func(cleanup context.Context, id skill.DeploymentID) error {
			if cleanup.Err() != nil || cleanup.Done() != nil || cleanup.Value(compensationContextKey{}) != "delivery" {
				t.Fatalf("compensation lost uncancelled context: %v", cleanup.Err())
			}
			removed = append(removed, id)
			if len(removed) == 1 {
				return cleanupErr
			}
			return nil
		},
	}
	result, err := NewService(profiles, trustedSource(), deployments, nil, nil, nil).DeployProfileSkills(ctx, snapshot.Slug, skill.DeploymentAgentsProject, root)
	if result != nil || !errors.Is(err, context.Canceled) || !errors.Is(err, cleanupErr) || len(created) != 2 || !slices.Equal(removed, []skill.DeploymentID{created[1], created[0]}) {
		t.Fatalf("compensation changed: result=%v err=%v created=%v removed=%v", result, err, created, removed)
	}
}

func TestHistoricalDeploymentsRemainBoundToTheirOriginalOwner(t *testing.T) {
	snapshot := selectedProfile()
	snapshot.Skills = nil // current membership need not retain historical deliveries
	profiles := &profilesFixture{snapshot: snapshot}
	root := t.TempDir()
	owned := deploymentRecord(t, 1, "sinst_00000000000000000000000000000001", root, snapshot.ID.String(), 3)
	other := deploymentRecord(t, 2, owned.InstallationID(), root, "prof_00000000000000000000000000000002", 4)
	legacy := deploymentRecord(t, 3, owned.InstallationID(), root, "", 0)
	byID := map[skill.DeploymentID]skill.Deployment{owned.ID(): owned, other.ID(): other, legacy.ID(): legacy}
	records := recordsFixture{
		get: func(_ context.Context, id skill.DeploymentID) (skill.Deployment, error) { return byID[id], nil },
		list: func(_ context.Context, owner string) ([]skillapp.DeploymentInspection, error) {
			if owner != snapshot.ID.String() {
				t.Fatalf("wrong owner query %s", owner)
			}
			return []skillapp.DeploymentInspection{{Deployment: owned, Drift: skill.DeploymentDrifted}}, nil
		},
	}
	var removed []skill.DeploymentID
	deployments := deploymentsFixture{undeploy: func(_ context.Context, id skill.DeploymentID) error { removed = append(removed, id); return nil }}
	service := NewService(profiles, nil, deployments, records, nil, nil)
	listed, err := service.ListProfileSkillDeployments(context.Background(), snapshot.Slug)
	if err != nil || len(listed) != 1 || listed[0].Drift != skill.DeploymentDrifted || listed[0].Deployment.ProfileRevision() != 3 {
		t.Fatalf("historical projection changed: %+v %v", listed, err)
	}
	for _, id := range []skill.DeploymentID{other.ID(), legacy.ID()} {
		if err := service.UndeployProfileSkill(context.Background(), snapshot.Slug, id); !errors.Is(err, skillapp.ErrDeploymentNotFound) {
			t.Fatalf("cross-owner removal %s: %v", id, err)
		}
	}
	if err := service.UndeployUnassignedSkill(context.Background(), owned.ID()); !errors.Is(err, ErrSkillDeliveryOwned) {
		t.Fatalf("owned deployment allowed unscoped cleanup: %v", err)
	}
	if err := service.UndeployProfileSkill(context.Background(), snapshot.Slug, owned.ID()); err != nil {
		t.Fatal(err)
	}
	if err := service.UndeployUnassignedSkill(context.Background(), legacy.ID()); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(removed, []skill.DeploymentID{owned.ID(), legacy.ID()}) {
		t.Fatalf("ownership changed cleanup: %v", removed)
	}
}

func TestAgentPluginExportPinsSelectedSkillsAndReturnsTheExportedSnapshotRevision(t *testing.T) {
	snapshot := selectedProfile()
	profiles := &profilesFixture{snapshot: snapshot}
	calls := 0
	exporter := exporterFixture(func(_ context.Context, request skillapp.ExportAgentPluginRequest) error {
		calls++
		if !slices.Equal(request.InstallationIDs, snapshot.Skills) {
			t.Fatalf("caller broadened Skill selection: %v", request.InstallationIDs)
		}
		profiles.snapshot.Revision++ // export reports the captured snapshot, not a later revision
		return nil
	})
	service := NewService(profiles, nil, nil, nil, exporter, nil)
	revision, err := service.ExportProfileAgentPlugin(context.Background(), snapshot.Slug, skillapp.ExportAgentPluginRequest{
		InstallationIDs: []skill.InstallationID{"sinst_00000000000000000000000000000009"}, MCP: &skillapp.AgentPluginMCP{URL: "https://example.test/mcp"},
	})
	if err != nil || revision != snapshot.Revision || calls != 1 || profiles.revisions != 0 {
		t.Fatalf("export snapshot changed: revision=%d err=%v calls=%d checks=%d", revision, err, calls, profiles.revisions)
	}
	profiles.snapshot.HasTools = false
	if _, err := service.ExportProfileAgentPlugin(context.Background(), snapshot.Slug, skillapp.ExportAgentPluginRequest{MCP: &skillapp.AgentPluginMCP{URL: "https://example.test/mcp"}}); err == nil || calls != 1 {
		t.Fatalf("MCP export without selected Tools: %v calls=%d", err, calls)
	}
}

func TestLocalCLIRequiresProfileAndUsesOnlyInjectedHostMetadata(t *testing.T) {
	profiles := &profilesFixture{snapshot: selectedProfile()}
	calls := 0
	host := LocalCLIContext{DatabasePath: "host-selected-database", Platform: "fixture-platform"}
	environment := environmentFixture(func(context.Context) (LocalCLIContext, error) { calls++; return host, nil })
	service := NewService(profiles, nil, nil, nil, nil, environment)
	unavailable := errors.New("unknown Profile")
	profiles.getErr = unavailable
	if _, err := service.LocalProfileCLI(context.Background(), profiles.snapshot.Slug); !errors.Is(err, unavailable) || calls != 0 {
		t.Fatalf("unknown Profile consulted host: %v calls=%d", err, calls)
	}
	profiles.getErr = nil
	recipe, err := service.LocalProfileCLI(context.Background(), profiles.snapshot.Slug)
	if err != nil || recipe.DatabasePath != host.DatabasePath || recipe.Platform != host.Platform || recipe.ProfileSlug != profiles.snapshot.Slug.String() {
		t.Fatalf("host metadata changed: %+v %v", recipe, err)
	}
	base := []string{"mender", "--db", host.DatabasePath}
	for _, command := range [][]string{recipe.Search, recipe.Describe, recipe.Call, recipe.MCPStdio} {
		if !reflect.DeepEqual(command[:3], base) {
			t.Fatalf("command changed host target: %v", command)
		}
	}
	if !slices.Equal(recipe.MCPStdio[3:], []string{"serve", "mcp", "--profile", recipe.ProfileSlug, "--stdio"}) {
		t.Fatalf("stdio lost explicit Profile: %v", recipe.MCPStdio)
	}
	recipe.Search[0] = "changed"
	if recipe.Describe[0] != "mender" || recipe.Call[0] != "mender" || recipe.MCPStdio[0] != "mender" {
		t.Fatal("CLI token arrays alias each other")
	}
}
