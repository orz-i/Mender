package delivery

// Service owns Profile delivery orchestration. Each port consumes an existing
// module's use case; no Skill persistence or filesystem mechanism lives here.
type Service struct {
	profiles    Profiles
	source      SkillSource
	deployments SkillDeployments
	records     DeploymentRecords
	exporter    AgentPluginExporter
	environment CLIEnvironment
}

func NewService(profiles Profiles, source SkillSource, deployments SkillDeployments,
	records DeploymentRecords, exporter AgentPluginExporter, environment CLIEnvironment,
) *Service {
	return &Service{profiles: profiles, source: source, deployments: deployments,
		records: records, exporter: exporter, environment: environment}
}
