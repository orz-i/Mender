package application

import (
	"context"
	"errors"
	"testing"

	skilldomain "github.com/orz-i/Mender/backend/internal/skill/domain"
)

func TestExportAgentPluginValidatesTrustEnablementAndSkillNames(t *testing.T) {
	t.Parallel()

	firstID := mustExportInstallationID(t, "sinst_00112233445566778899aabbccddeeff")
	secondID := mustExportInstallationID(t, "sinst_11112222333344445555666677778888")

	source := exportSource{
		items: map[skilldomain.InstallationID]DeployableInstallation{
			firstID: {
				InstallationID: firstID,
				Name:           "b-skill",
				Digest:         "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
				Trusted:        true,
				Enabled:        true,
			},
			secondID: {
				InstallationID: secondID,
				Name:           "a-skill",
				Digest:         "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
				Trusted:        true,
				Enabled:        true,
			},
		},
	}
	exporter := &recordingExporter{}
	usecase := NewExportAgentPlugin(source, exporter)
	if err := usecase.Execute(context.Background(), ExportAgentPluginRequest{
		Name:            "mender.skills",
		Version:         "1.0.0",
		Description:     "test",
		Destination:     "/tmp/mender-skills",
		InstallationIDs: []skilldomain.InstallationID{firstID, secondID},
	}); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if exporter.calls != 1 {
		t.Fatalf("exporter calls = %d, want 1", exporter.calls)
	}
	if len(exporter.pkg.Skills) != 2 ||
		exporter.pkg.Skills[0].Name != "a-skill" ||
		exporter.pkg.Skills[1].Name != "b-skill" {
		t.Fatalf("exported Skill order = %+v", exporter.pkg.Skills)
	}

	untrusted := source
	untrusted.items = cloneDeployableItems(source.items)
	item := untrusted.items[firstID]
	item.Trusted = false
	untrusted.items[firstID] = item
	if err := NewExportAgentPlugin(untrusted, &recordingExporter{}).Execute(
		context.Background(),
		ExportAgentPluginRequest{
			Name:            "mender.skills",
			Destination:     "/tmp/mender-skills",
			InstallationIDs: []skilldomain.InstallationID{firstID},
		},
	); !errors.Is(err, ErrInstallationUntrusted) {
		t.Fatalf("untrusted Execute() error = %v", err)
	}

	disabled := source
	disabled.items = cloneDeployableItems(source.items)
	item = disabled.items[firstID]
	item.Enabled = false
	disabled.items[firstID] = item
	if err := NewExportAgentPlugin(disabled, &recordingExporter{}).Execute(
		context.Background(),
		ExportAgentPluginRequest{
			Name:            "mender.skills",
			Destination:     "/tmp/mender-skills",
			InstallationIDs: []skilldomain.InstallationID{firstID},
		},
	); !errors.Is(err, ErrInstallationDisabled) {
		t.Fatalf("disabled Execute() error = %v", err)
	}

	duplicateName := source
	duplicateName.items = cloneDeployableItems(source.items)
	item = duplicateName.items[secondID]
	item.Name = "b-skill"
	duplicateName.items[secondID] = item
	if err := NewExportAgentPlugin(duplicateName, &recordingExporter{}).Execute(
		context.Background(),
		ExportAgentPluginRequest{
			Name:            "mender.skills",
			Destination:     "/tmp/mender-skills",
			InstallationIDs: []skilldomain.InstallationID{firstID, secondID},
		},
	); err == nil {
		t.Fatal("duplicate Skill name Execute() error = nil")
	}
}

type exportSource struct {
	items map[skilldomain.InstallationID]DeployableInstallation
}

func (s exportSource) GetDeployable(
	_ context.Context,
	id skilldomain.InstallationID,
) (DeployableInstallation, error) {
	item, ok := s.items[id]
	if !ok {
		return DeployableInstallation{}, ErrInstallationNotFound
	}
	return item, nil
}

func (s exportSource) Materialize(
	context.Context,
	skilldomain.InstallationID,
	string,
) error {
	return nil
}

type recordingExporter struct {
	calls int
	pkg   AgentPluginPackage
}

func (e *recordingExporter) ExportAgentPlugin(
	_ context.Context,
	pkg AgentPluginPackage,
) error {
	e.calls++
	e.pkg = pkg
	return nil
}

func cloneDeployableItems(
	source map[skilldomain.InstallationID]DeployableInstallation,
) map[skilldomain.InstallationID]DeployableInstallation {
	result := make(map[skilldomain.InstallationID]DeployableInstallation, len(source))
	for id, item := range source {
		result[id] = item
	}
	return result
}

func mustExportInstallationID(
	t *testing.T,
	raw string,
) skilldomain.InstallationID {
	t.Helper()
	id, err := skilldomain.ParseInstallationID(raw)
	if err != nil {
		t.Fatal(err)
	}
	return id
}
