package bootstrap

import (
	"context"

	profiledelivery "github.com/orz-i/Mender/backend/internal/profile/application/delivery"
	profiledomain "github.com/orz-i/Mender/backend/internal/profile/domain"
	profilelocaldelivery "github.com/orz-i/Mender/backend/internal/profile/infrastructure/localdelivery"
	skillapplication "github.com/orz-i/Mender/backend/internal/skill/application"
	skilldomain "github.com/orz-i/Mender/backend/internal/skill/domain"
)

// Delivery is stateless: compose its consumer ports from the already-owned
// Runtime services without creating another database, cleanup path or cache.
func newProfileDelivery(profiles *Profiles, library *Skills, databasePath string) *profiledelivery.Service {
	skills := profileDeliverySkills{skills: library}
	return profiledelivery.NewService(profileDeliveryProfiles{profiles: profiles},
		skills, skills, skills, skills,
		profilelocaldelivery.Environment{DatabasePath: databasePath})
}

type profileDeliveryProfiles struct{ profiles *Profiles }

func (p profileDeliveryProfiles) Snapshot(ctx context.Context, slug profiledomain.Slug) (profiledelivery.ProfileSnapshot, error) {
	profile, err := p.profiles.Get.Execute(ctx, slug)
	if err != nil {
		return profiledelivery.ProfileSnapshot{}, err
	}
	return profiledelivery.ProfileSnapshot{ID: profile.ID(), Slug: profile.Slug(),
		Revision: profile.Revision(), Skills: profile.Skills(), HasTools: len(profile.ToolIDs()) != 0}, nil
}
func (p profileDeliveryProfiles) Revision(ctx context.Context, id profiledomain.ProfileID) (uint64, error) {
	return p.profiles.GetRevision.Execute(ctx, id)
}

type profileDeliverySkills struct{ skills *Skills }

func (s profileDeliverySkills) GetDeployable(ctx context.Context, id skilldomain.InstallationID) (profiledelivery.SkillStatus, error) {
	item, err := runtimeSkillDeploymentSource(s).GetDeployable(ctx, id)
	if err != nil {
		return profiledelivery.SkillStatus{}, err
	}
	return profiledelivery.SkillStatus{Name: item.Name, Trusted: item.Trusted, Enabled: item.Enabled}, nil
}
func (s profileDeliverySkills) DeployForProfile(ctx context.Context, id skilldomain.InstallationID,
	target skilldomain.DeploymentTarget, root, profileID string, revision uint64,
) (skilldomain.Deployment, error) {
	return s.skills.deployForProfile(ctx, id, target, root, profileID, revision)
}
func (s profileDeliverySkills) Undeploy(ctx context.Context, id skilldomain.DeploymentID) error {
	return s.skills.Undeploy(ctx, id)
}
func (s profileDeliverySkills) GetDeployment(ctx context.Context, id skilldomain.DeploymentID) (skilldomain.Deployment, error) {
	return s.skills.store.GetDeployment(ctx, id)
}
func (s profileDeliverySkills) ListOwnedDeployments(ctx context.Context, profileID string) ([]skillapplication.DeploymentInspection, error) {
	return s.skills.listOwnedDeployments(ctx, profileID)
}
func (s profileDeliverySkills) ExportAgentPlugin(ctx context.Context, request skillapplication.ExportAgentPluginRequest) error {
	return s.skills.ExportAgentPlugin(ctx, request)
}

var _ profiledelivery.Profiles = profileDeliveryProfiles{}
var _ profiledelivery.SkillSource = profileDeliverySkills{}
var _ profiledelivery.SkillDeployments = profileDeliverySkills{}
var _ profiledelivery.DeploymentRecords = profileDeliverySkills{}
var _ profiledelivery.AgentPluginExporter = profileDeliverySkills{}
