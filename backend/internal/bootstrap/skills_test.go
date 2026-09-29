package bootstrap

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	skillapplication "github.com/orz-i/Mender/backend/internal/skill/application"
	skilldomain "github.com/orz-i/Mender/backend/internal/skill/domain"
	"github.com/orz-i/Mender/backend/internal/skill/infrastructure/agentskill"
)

func TestRuntimeSkillLibraryIncludesBuiltInAndManagedInstallations(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	temp := t.TempDir()
	runtime, err := NewRuntime(ctx, RuntimeOptions{
		DatabasePath: filepath.Join(temp, "mender.db"),
		SkillRoot:    filepath.Join(temp, "managed-skills"),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runtime.Close() })

	items, err := runtime.Skills.List(ctx)
	if err != nil {
		t.Fatalf("Skills.List() error = %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("initial Skills.List() count = %d, want 1", len(items))
	}
	builtIn := items[0]
	if builtIn.Name != "mender-tools" || !builtIn.BuiltIn ||
		builtIn.SourceKind != SkillSourceBuiltIn ||
		!builtIn.Trusted || !builtIn.Enabled {
		t.Fatalf("built-in Skill = %+v", builtIn)
	}
	inspection, err := runtime.Skills.Inspect(ctx, builtIn.InstallationID)
	if err != nil {
		t.Fatalf("Inspect(built-in) error = %v", err)
	}
	if inspection.Metadata.Name != "mender-tools" ||
		len(inspection.Resources) != 1 ||
		inspection.Resources[0].Path != "SKILL.md" {
		t.Fatalf("built-in inspection = %+v", inspection)
	}
	if _, err := runtime.Skills.SetTrust(ctx, builtIn.InstallationID, false); !errors.Is(err, ErrBuiltInSkillImmutable) {
		t.Fatalf("SetTrust(built-in) error = %v", err)
	}
	if err := runtime.Skills.Remove(ctx, builtIn.InstallationID); !errors.Is(err, ErrBuiltInSkillImmutable) {
		t.Fatalf("Remove(built-in) error = %v", err)
	}

	source := writeRuntimeSkill(t, filepath.Join(temp, "example-skill"))
	managed, err := runtime.Skills.InstallLocal(ctx, source)
	if err != nil {
		t.Fatalf("Skills.InstallLocal() error = %v", err)
	}
	if managed.Trusted() || managed.Enabled() {
		t.Fatalf("managed initial state trusted=%v enabled=%v", managed.Trusted(), managed.Enabled())
	}

	items, err = runtime.Skills.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 {
		t.Fatalf("Skills.List() after install count = %d, want 2", len(items))
	}
	var managedProjection SkillInstallation
	for _, item := range items {
		if item.InstallationID == managed.ID() {
			managedProjection = item
		}
	}
	if managedProjection.InstallationID == "" ||
		managedProjection.BuiltIn ||
		managedProjection.SourceKind != string(skilldomain.SourceLocalDirectory) ||
		managedProjection.Trusted ||
		managedProjection.Enabled {
		t.Fatalf("managed projection = %+v", managedProjection)
	}

	if _, err := runtime.Skills.SetEnabled(ctx, managed.ID(), true); err == nil {
		t.Fatal("SetEnabled(untrusted) error = nil")
	}
	if _, err := runtime.Skills.SetTrust(ctx, managed.ID(), true); err != nil {
		t.Fatalf("SetTrust(managed) error = %v", err)
	}
	enabled, err := runtime.Skills.SetEnabled(ctx, managed.ID(), true)
	if err != nil {
		t.Fatalf("SetEnabled(managed) error = %v", err)
	}
	if !enabled.Trusted() || !enabled.Enabled() {
		t.Fatalf("enabled managed state trusted=%v enabled=%v", enabled.Trusted(), enabled.Enabled())
	}

	managedInspection, err := runtime.Skills.Inspect(ctx, managed.ID())
	if err != nil {
		t.Fatalf("Inspect(managed) error = %v", err)
	}
	if managedInspection.Installation.InstallationID != managed.ID() ||
		managedInspection.Metadata.Name != "example-skill" {
		t.Fatalf("managed inspection = %+v", managedInspection)
	}

	if err := runtime.Skills.Remove(ctx, managed.ID()); err != nil {
		t.Fatalf("Skills.Remove(managed) error = %v", err)
	}
	items, err = runtime.Skills.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Name != "mender-tools" {
		t.Fatalf("Skills.List() after remove = %+v", items)
	}
}

func TestRuntimeSkillDeploymentsDetectDriftAndGuardInstallationState(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	temp := t.TempDir()
	runtime, err := NewRuntime(ctx, RuntimeOptions{
		DatabasePath: filepath.Join(temp, "mender.db"),
		SkillRoot:    filepath.Join(temp, "managed-skills"),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runtime.Close() })

	items, err := runtime.Skills.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var builtIn SkillInstallation
	for _, item := range items {
		if item.Name == "mender-tools" && item.BuiltIn {
			builtIn = item
			break
		}
	}
	if builtIn.InstallationID == "" {
		t.Fatal("built-in mender-tools not found")
	}

	projectRoot := filepath.Join(temp, "project")
	if err := os.MkdirAll(projectRoot, 0o750); err != nil {
		t.Fatal(err)
	}
	builtInDeployment, err := runtime.Skills.Deploy(
		ctx,
		builtIn.InstallationID,
		skilldomain.DeploymentAgentsProject,
		projectRoot,
	)
	if err != nil {
		t.Fatalf("Deploy(built-in) error = %v", err)
	}
	wantBuiltInDestination := filepath.Join(
		projectRoot,
		".agents",
		"skills",
		"mender-tools",
	)
	if builtInDeployment.Destination() != wantBuiltInDestination {
		t.Fatalf(
			"built-in destination = %q, want %q",
			builtInDeployment.Destination(),
			wantBuiltInDestination,
		)
	}
	builtInInspection, err := runtime.Skills.InspectDeployment(
		ctx,
		builtInDeployment.ID(),
	)
	if err != nil {
		t.Fatal(err)
	}
	if builtInInspection.Drift != skilldomain.DeploymentCurrent {
		t.Fatalf("built-in deployment drift = %q, want current", builtInInspection.Drift)
	}

	deploymentRoot, err := os.OpenRoot(wantBuiltInDestination)
	if err != nil {
		t.Fatal(err)
	}
	skillFile, err := deploymentRoot.Open("SKILL.md")
	if err != nil {
		_ = deploymentRoot.Close()
		t.Fatal(err)
	}
	raw, err := io.ReadAll(skillFile)
	if err != nil {
		_ = skillFile.Close()
		_ = deploymentRoot.Close()
		t.Fatal(err)
	}
	if err := skillFile.Close(); err != nil {
		_ = deploymentRoot.Close()
		t.Fatal(err)
	}
	skillFile, err = deploymentRoot.OpenFile(
		"SKILL.md",
		os.O_WRONLY|os.O_TRUNC,
		0o600,
	)
	if err != nil {
		_ = deploymentRoot.Close()
		t.Fatal(err)
	}
	if _, err := skillFile.Write(append(raw, []byte("\n# local drift\n")...)); err != nil {
		_ = skillFile.Close()
		_ = deploymentRoot.Close()
		t.Fatal(err)
	}
	if err := skillFile.Close(); err != nil {
		_ = deploymentRoot.Close()
		t.Fatal(err)
	}
	if err := deploymentRoot.Close(); err != nil {
		t.Fatal(err)
	}
	builtInInspection, err = runtime.Skills.InspectDeployment(
		ctx,
		builtInDeployment.ID(),
	)
	if err != nil {
		t.Fatal(err)
	}
	if builtInInspection.Drift != skilldomain.DeploymentDrifted {
		t.Fatalf("drifted built-in deployment = %q, want drifted", builtInInspection.Drift)
	}
	if err := runtime.Skills.Undeploy(ctx, builtInDeployment.ID()); err != nil {
		t.Fatalf("Undeploy(built-in) error = %v", err)
	}
	builtInInspection, err = runtime.Skills.InspectDeployment(
		ctx,
		builtInDeployment.ID(),
	)
	if !errors.Is(err, skillapplication.ErrDeploymentNotFound) {
		t.Fatalf("InspectDeployment(removed) error = %v", err)
	}

	source := writeRuntimeSkill(t, filepath.Join(temp, "managed-skill"))
	managed, err := runtime.Skills.InstallLocal(ctx, source)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.Skills.SetTrust(ctx, managed.ID(), true); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.Skills.SetEnabled(ctx, managed.ID(), true); err != nil {
		t.Fatal(err)
	}

	managedDeployment, err := runtime.Skills.Deploy(
		ctx,
		managed.ID(),
		skilldomain.DeploymentGitHubProject,
		projectRoot,
	)
	if err != nil {
		t.Fatalf("Deploy(managed) error = %v", err)
	}
	if _, err := runtime.Skills.SetEnabled(ctx, managed.ID(), false); !errors.Is(err, skillapplication.ErrInstallationInUse) {
		t.Fatalf("SetEnabled(false while deployed) error = %v", err)
	}
	if _, err := runtime.Skills.SetTrust(ctx, managed.ID(), false); !errors.Is(err, skillapplication.ErrInstallationInUse) {
		t.Fatalf("SetTrust(false while deployed) error = %v", err)
	}
	if err := runtime.Skills.Remove(ctx, managed.ID()); !errors.Is(err, skillapplication.ErrInstallationInUse) {
		t.Fatalf("Remove(while deployed) error = %v", err)
	}

	deployments, err := runtime.Skills.ListDeployments(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(deployments) != 1 ||
		deployments[0].Deployment.ID() != managedDeployment.ID() ||
		deployments[0].Drift != skilldomain.DeploymentCurrent {
		t.Fatalf("ListDeployments() = %+v", deployments)
	}

	if err := runtime.Skills.Undeploy(ctx, managedDeployment.ID()); err != nil {
		t.Fatalf("Undeploy(managed) error = %v", err)
	}
	if _, err := runtime.Skills.SetEnabled(ctx, managed.ID(), false); err != nil {
		t.Fatalf("SetEnabled(false after undeploy) error = %v", err)
	}
	if _, err := runtime.Skills.SetTrust(ctx, managed.ID(), false); err != nil {
		t.Fatalf("SetTrust(false after undeploy) error = %v", err)
	}
	if err := runtime.Skills.Remove(ctx, managed.ID()); err != nil {
		t.Fatalf("Remove(after undeploy) error = %v", err)
	}
}

func TestRuntimeExportsPortableAgentPlugin(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	temp := t.TempDir()
	runtime, err := NewRuntime(ctx, RuntimeOptions{
		DatabasePath: filepath.Join(temp, "mender.db"),
		SkillRoot:    filepath.Join(temp, "managed-skills"),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runtime.Close() })

	items, err := runtime.Skills.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var builtIn SkillInstallation
	for _, item := range items {
		if item.Name == "mender-tools" && item.BuiltIn {
			builtIn = item
			break
		}
	}
	if builtIn.InstallationID == "" {
		t.Fatal("built-in mender-tools not found")
	}

	source := writeRuntimeSkill(t, filepath.Join(temp, "export-skill"))
	managed, err := runtime.Skills.InstallLocal(ctx, source)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.Skills.SetTrust(ctx, managed.ID(), true); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.Skills.SetEnabled(ctx, managed.ID(), true); err != nil {
		t.Fatal(err)
	}

	exportParent := filepath.Join(temp, "exports")
	if err := os.MkdirAll(exportParent, 0o750); err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(exportParent, "mender-skills")
	request := skillapplication.ExportAgentPluginRequest{
		Name:        "mender.skills",
		Version:     "1.0.0",
		Description: "Mender exported Skills",
		Destination: destination,
		InstallationIDs: []skilldomain.InstallationID{
			managed.ID(),
			builtIn.InstallationID,
		},
	}
	if err := runtime.Skills.ExportAgentPlugin(ctx, request); err != nil {
		t.Fatalf("ExportAgentPlugin() error = %v", err)
	}

	exportRoot, err := os.OpenRoot(destination)
	if err != nil {
		t.Fatal(err)
	}
	manifestFile, err := exportRoot.Open("plugin.json")
	if err != nil {
		_ = exportRoot.Close()
		t.Fatal(err)
	}
	rawManifest, err := io.ReadAll(manifestFile)
	if err != nil {
		_ = manifestFile.Close()
		_ = exportRoot.Close()
		t.Fatal(err)
	}
	if err := manifestFile.Close(); err != nil {
		_ = exportRoot.Close()
		t.Fatal(err)
	}
	var manifest map[string]any
	if err := json.Unmarshal(rawManifest, &manifest); err != nil {
		_ = exportRoot.Close()
		t.Fatal(err)
	}
	if manifest["$schema"] != "https://agent-plugins.org/schemas/1.0.0/plugin.schema.json" ||
		manifest["name"] != "mender.skills" ||
		manifest["version"] != "1.0.0" {
		_ = exportRoot.Close()
		t.Fatalf("plugin.json = %#v", manifest)
	}
	if _, err := exportRoot.Lstat("mcp.json"); !errors.Is(err, os.ErrNotExist) {
		_ = exportRoot.Close()
		t.Fatalf("mcp.json exists or inspect failed: %v", err)
	}
	if err := exportRoot.Close(); err != nil {
		t.Fatal(err)
	}

	for _, expected := range []SkillInstallation{
		builtIn,
		managedSkillProjection(managed),
	} {
		snapshot, err := agentskill.LoadDirectory(
			filepath.Join(destination, "skills", expected.Name),
		)
		if err != nil {
			t.Fatalf("load exported %s: %v", expected.Name, err)
		}
		if snapshot.Digest != expected.Digest {
			t.Fatalf(
				"exported %s digest = %s, want %s",
				expected.Name,
				snapshot.Digest,
				expected.Digest,
			)
		}
	}

	if err := runtime.Skills.ExportAgentPlugin(ctx, request); !errors.Is(err, skillapplication.ErrAgentPluginTargetExists) {
		t.Fatalf("ExportAgentPlugin(existing) error = %v", err)
	}
	if err := runtime.Skills.Remove(ctx, managed.ID()); err != nil {
		t.Fatalf("Skills.Remove(managed) cleanup error = %v", err)
	}
}

func TestRuntimeUsesDefaultSkillRootBesideDatabase(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	temp := t.TempDir()
	databasePath := filepath.Join(temp, "state", "mender.db")
	if err := os.MkdirAll(filepath.Dir(databasePath), 0o750); err != nil {
		t.Fatal(err)
	}
	runtime, err := NewRuntime(ctx, RuntimeOptions{DatabasePath: databasePath})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runtime.Close() })

	source := writeRuntimeSkill(t, filepath.Join(temp, "source-skill"))
	item, err := runtime.Skills.InstallLocal(ctx, source)
	if err != nil {
		t.Fatal(err)
	}
	expected := filepath.Join(filepath.Dir(databasePath), "skills", item.ID().String())
	if info, err := os.Stat(expected); err != nil || !info.IsDir() {
		t.Fatalf("default managed Skill root %q: info=%v err=%v", expected, info, err)
	}
	if err := runtime.Skills.Remove(ctx, item.ID()); err != nil {
		t.Fatalf("Skills.Remove() cleanup error = %v", err)
	}
}

func writeRuntimeSkill(t *testing.T, root string) string {
	t.Helper()
	if err := os.MkdirAll(root, 0o750); err != nil {
		t.Fatal(err)
	}
	content := "---\n" +
		"name: " + filepath.Base(root) + "\n" +
		"description: Runtime Skill Library test.\n" +
		"---\n# Skill\n"
	if err := os.WriteFile(filepath.Join(root, "SKILL.md"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return root
}
