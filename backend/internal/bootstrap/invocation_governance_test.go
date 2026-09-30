package bootstrap

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"

	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
	invocationdomain "github.com/orz-i/Mender/backend/internal/invocation/domain"
	providerplugin "github.com/orz-i/Mender/backend/internal/providerplugin"
	providerconfigstore "github.com/orz-i/Mender/backend/internal/providerplugin/configstore"
)

func TestInvocationGovernanceFactsUseReviewedBuiltInExtractorWithoutActivation(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store, err := providerconfigstore.Open(ctx, filepath.Join(t.TempDir(), "mender.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })

	registry := providerplugin.NewRegistry()
	plugin := &governanceTestPlugin{}
	if err := registry.Register(plugin); err != nil {
		t.Fatal(err)
	}

	providerID := mustGovernanceProviderID(t, "prov_00000000000000000000000000000031")
	config, err := providerplugin.NewConfiguration(
		providerID,
		plugin.Descriptor().ID,
		1,
		json.RawMessage("{\"root\":\"/workspace\"}"),
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Save(ctx, config); err != nil {
		t.Fatal(err)
	}

	tool := mustGovernanceTool(t, providerID, "workspace.edit")
	arguments, err := invocationdomain.NewJSONValue([]byte("{\"path\":\"src/main.go\"}"))
	if err != nil {
		t.Fatal(err)
	}
	facts, err := newInvocationGovernanceFactExtractor(store, registry).Extract(
		ctx,
		tool,
		arguments,
	)
	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}
	if facts.Knowledge() != invocationdomain.FactKnowledgeKnown {
		t.Fatalf("Knowledge() = %s, want known", facts.Knowledge())
	}
	if got := facts.Effects(); len(got) != 1 || got[0] != "filesystem.write" {
		t.Fatalf("Effects() = %v", got)
	}
	resources := facts.Resources()
	if len(resources) != 1 ||
		resources[0].Kind() != "workspace_path" ||
		resources[0].Value() != "src/main.go" {
		t.Fatalf("Resources() = %#v", resources)
	}
	if plugin.extracts != 1 {
		t.Fatalf("extract calls = %d, want 1", plugin.extracts)
	}
	if plugin.opens != 0 {
		t.Fatalf("Open calls = %d, want 0", plugin.opens)
	}
}

func TestInvocationGovernanceFactsRemainUnknownWithoutReviewedBuiltInExtractor(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store, err := providerconfigstore.Open(ctx, filepath.Join(t.TempDir(), "mender.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	registry := providerplugin.NewRegistry()

	providerID := mustGovernanceProviderID(t, "prov_00000000000000000000000000000032")
	tool := mustGovernanceTool(t, providerID, "unconfigured")
	arguments, err := invocationdomain.NewJSONValue([]byte("{}"))
	if err != nil {
		t.Fatal(err)
	}
	facts, err := newInvocationGovernanceFactExtractor(store, registry).Extract(ctx, tool, arguments)
	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}
	if facts.Knowledge() != invocationdomain.FactKnowledgeUnknown {
		t.Fatalf("Knowledge() = %s, want unknown", facts.Knowledge())
	}
}

func TestInvocationGovernanceFactsDoNotTrustExternalInstallations(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store, err := providerconfigstore.Open(ctx, filepath.Join(t.TempDir(), "mender.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })

	registry := providerplugin.NewRegistry()
	plugin := &governanceTestPlugin{}
	if err := registry.Register(plugin); err != nil {
		t.Fatal(err)
	}
	providerID := mustGovernanceProviderID(t, "prov_00000000000000000000000000000033")
	installationID, err := providerplugin.ParseInstallationID(
		"pinst_00000000000000000000000000000033",
	)
	if err != nil {
		t.Fatal(err)
	}
	config, err := providerplugin.NewInstalledConfiguration(
		providerID,
		plugin.Descriptor().ID,
		installationID,
		1,
		json.RawMessage("{\"root\":\"/workspace\"}"),
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Save(ctx, config); err != nil {
		t.Fatal(err)
	}

	arguments, err := invocationdomain.NewJSONValue([]byte("{\"path\":\"src/main.go\"}"))
	if err != nil {
		t.Fatal(err)
	}
	facts, err := newInvocationGovernanceFactExtractor(
		store,
		registry,
	).Extract(ctx, mustGovernanceTool(t, providerID, "workspace.edit"), arguments)
	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}
	if facts.Knowledge() != invocationdomain.FactKnowledgeUnknown {
		t.Fatalf("Knowledge() = %s, want unknown", facts.Knowledge())
	}
	if plugin.extracts != 0 || plugin.opens != 0 {
		t.Fatalf("external config invoked built-in plugin: extracts=%d opens=%d", plugin.extracts, plugin.opens)
	}
}

func mustGovernanceProviderID(t *testing.T, value string) catalogdomain.ProviderID {
	t.Helper()
	id, err := catalogdomain.ParseProviderID(value)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func mustGovernanceTool(
	t *testing.T,
	providerID catalogdomain.ProviderID,
	sourceKey string,
) catalogdomain.Tool {
	t.Helper()
	schema, err := catalogdomain.NewSchema([]byte("{\"type\":\"object\"}"))
	if err != nil {
		t.Fatal(err)
	}
	tool, err := catalogdomain.NewTool(
		mustBootstrapToolID(t, "tool_00000000000000000000000000000031"),
		providerID,
		catalogdomain.ToolSpec{
			SourceKey:   sourceKey,
			Name:        sourceKey,
			InputSchema: schema,
		},
		systemClock{}.Now(),
	)
	if err != nil {
		t.Fatal(err)
	}
	return tool
}

type governanceTestPlugin struct {
	extracts int
	opens    int
}

func (*governanceTestPlugin) Descriptor() providerplugin.Descriptor {
	return providerplugin.Descriptor{
		ID:            providerplugin.ID("io.mender.provider.governance-test"),
		Version:       "1.0.0",
		DisplayName:   "Governance Test",
		ConfigVersion: 1,
	}
}

func (*governanceTestPlugin) ValidateConfig(context.Context, json.RawMessage) error {
	return nil
}

func (*governanceTestPlugin) InspectConfig(
	context.Context,
	json.RawMessage,
) (providerplugin.ConfigMetadata, error) {
	return providerplugin.NewConfigMetadata(nil)
}

func (p *governanceTestPlugin) Open(
	context.Context,
	providerplugin.OpenRequest,
) (providerplugin.Instance, error) {
	p.opens++
	return nil, errors.New("Open must not be called by governance extraction")
}

func (p *governanceTestPlugin) ExtractGovernanceFacts(
	_ context.Context,
	config json.RawMessage,
	tool catalogdomain.Tool,
	arguments invocationdomain.JSONValue,
) (invocationdomain.GovernanceFacts, error) {
	p.extracts++
	if !json.Valid(config) || tool.SourceKey() != "workspace.edit" || len(arguments.Bytes()) == 0 {
		return invocationdomain.GovernanceFacts{}, errors.New("unexpected extractor input")
	}
	resource, err := invocationdomain.NewResourceFact("workspace_path", "src/main.go")
	if err != nil {
		return invocationdomain.GovernanceFacts{}, err
	}
	return invocationdomain.KnownGovernanceFacts(
		[]invocationdomain.EffectFact{"filesystem.write"},
		[]invocationdomain.ResourceFact{resource},
	)
}

var _ providerplugin.Plugin = (*governanceTestPlugin)(nil)
var _ providerplugin.GovernanceFactProvider = (*governanceTestPlugin)(nil)
