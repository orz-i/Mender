package bootstrap

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"
	"time"

	v3fixture "github.com/orz-i/Mender/backend/internal/testfixtures/v3"

	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
	governancedomain "github.com/orz-i/Mender/backend/internal/governance/domain"
	invocationdomain "github.com/orz-i/Mender/backend/internal/invocation/domain"
	providerplugin "github.com/orz-i/Mender/backend/internal/providerplugin"
	providerconfigstore "github.com/orz-i/Mender/backend/internal/providerplugin/configstore"
)

func TestInvocationGovernanceFactsUseReviewedBuiltInExtractorWithoutActivation(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "mender.db")
	v3fixture.Prepare(t, path)
	store, err := providerconfigstore.OpenTyped(ctx, path)
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
	seedProviderTestOwner(t, path, providerID)
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
	facts, err := newInvocationGovernanceFactExtractor(store, registry, nil).Extract(
		ctx,
		tool,
		arguments,
		governancedomain.Context{},
	)
	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}
	if facts.Facts.Knowledge() != invocationdomain.FactKnowledgeKnown {
		t.Fatalf("Knowledge() = %s, want known", facts.Facts.Knowledge())
	}
	if got := facts.Facts.Effects(); len(got) != 1 || got[0] != "filesystem.write" {
		t.Fatalf("Effects() = %v", got)
	}
	resources := facts.Facts.Resources()
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
	path := filepath.Join(t.TempDir(), "mender.db")
	v3fixture.Prepare(t, path)
	store, err := providerconfigstore.OpenTyped(ctx, path)
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
	facts, err := newInvocationGovernanceFactExtractor(store, registry, nil).Extract(ctx, tool, arguments, governancedomain.Context{})
	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}
	if facts.Facts.Knowledge() != invocationdomain.FactKnowledgeUnknown {
		t.Fatalf("Knowledge() = %s, want unknown", facts.Facts.Knowledge())
	}
}

func TestInvocationGovernanceFactsDoNotTrustExternalInstallations(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "mender.db")
	v3fixture.Prepare(t, path)
	store, err := providerconfigstore.OpenTyped(ctx, path)
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
	seedProviderTestOwner(t, path, providerID)
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
		nil,
	).Extract(ctx, mustGovernanceTool(t, providerID, "workspace.edit"), arguments, governancedomain.Context{})
	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}
	if facts.Facts.Knowledge() != invocationdomain.FactKnowledgeUnknown {
		t.Fatalf("Knowledge() = %s, want unknown", facts.Facts.Knowledge())
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
	source, err := catalogdomain.ProviderSource(providerID)
	if err != nil {
		t.Fatal(err)
	}
	tool, err := catalogdomain.NewToolFromSource(
		mustBootstrapToolID(t, "tool_00000000000000000000000000000031"),
		source,
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

func TestLegacyInvocationFactExtractorRejectsUncomposedTypedSources(t *testing.T) {
	t.Parallel()
	for _, spec := range []struct {
		kind    catalogdomain.SourceKind
		id, key string
	}{
		{catalogdomain.SourceAgent, "agent_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "agent.delegate"},
		{catalogdomain.SourceNative, "workspace", "workspace.read"},
		{catalogdomain.SourceNative, "jobs", "jobs.status"},
	} {
		t.Run(string(spec.kind)+":"+spec.id, func(t *testing.T) {
			source, err := catalogdomain.NewSourceRef(spec.kind, spec.id)
			if err != nil {
				t.Fatal(err)
			}
			id, err := catalogdomain.ParseToolID("tool_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
			if err != nil {
				t.Fatal(err)
			}
			schema, err := catalogdomain.NewSchema([]byte(`{"type":"object"}`))
			if err != nil {
				t.Fatal(err)
			}
			item, err := catalogdomain.NewToolFromSource(id, source, catalogdomain.ToolSpec{
				SourceKey: spec.key, Name: "test", InputSchema: schema,
			}, time.Now().UTC())
			if err != nil {
				t.Fatal(err)
			}
			args, err := invocationdomain.NewJSONValue([]byte(`{}`))
			if err != nil {
				t.Fatal(err)
			}
			if _, err := newInvocationGovernanceFactExtractor(nil, nil, nil).Extract(
				context.Background(), item, args, governancedomain.Context{},
			); err == nil {
				t.Fatal("legacy Provider-v2 Runtime accepted uncomposed Agent/native source")
			}
		})
	}
}

func seedProviderTestOwner(t *testing.T, path string, id catalogdomain.ProviderID) {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	_, err = db.Exec(`INSERT INTO providers(id,namespace,display_name,description,created_at,updated_at) VALUES(?,?,?,?,?,?)`,
		id.String(), id.String(), "Provider", "", "2026-11-09T00:00:00Z", "2026-11-09T00:00:00Z")
	if err != nil {
		t.Fatal(err)
	}
}
