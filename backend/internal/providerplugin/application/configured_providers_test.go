package application

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	catalogapplication "github.com/orz-i/Mender/backend/internal/catalog/application"
	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
	governancedomain "github.com/orz-i/Mender/backend/internal/governance/domain"
	provideractivationapplication "github.com/orz-i/Mender/backend/internal/provideractivation/application"
	"github.com/orz-i/Mender/backend/internal/providerplugin"
	sandboxdomain "github.com/orz-i/Mender/backend/internal/sandbox/domain"
)

func TestRegisterAdmissionRejectsBeforePluginWorkAndCompensatesIdentity(t *testing.T) {
	t.Parallel()
	deps, catalog, configs, plugin := managementFixture(t)
	denied := errors.New("activation denied")
	deps.Activation = activationFixture{err: denied}
	_, err := NewService(deps).RegisterConfiguredProvider(t.Context(), catalogapplication.RegisterProviderRequest{},
		plugin.descriptor.ID, json.RawMessage(`{}`), managementContext(t))
	if !errors.Is(err, denied) {
		t.Fatalf("registration lost activation denial: %v", err)
	}
	if catalog.registered != 1 || catalog.removed != 1 || plugin.validated != 0 || plugin.inspected != 0 || len(configs.saves) != 0 {
		t.Fatalf("denied acquisition performed plugin work or failed compensation: catalog=%+v plugin=%+v saves=%d", catalog, plugin, len(configs.saves))
	}
}

func TestGenericRegistrationCannotAcquireManagedFacadeAuthority(t *testing.T) {
	t.Parallel()
	deps, catalog, _, plugin := managementFixture(t)
	deps.IsManagedFacade = func(providerplugin.ID) bool { return true }
	_, err := NewService(deps).RegisterConfiguredProvider(t.Context(), catalogapplication.RegisterProviderRequest{},
		plugin.descriptor.ID, json.RawMessage(`{}`), managementContext(t))
	if !errors.Is(err, sandboxdomain.ErrNotAuthorized) || catalog.registered != 0 || plugin.validated != 0 {
		t.Fatalf("generic registration crossed owner boundary: err=%v registrations=%d validations=%d", err, catalog.registered, plugin.validated)
	}
}

func TestUpdateRestoresExactConfigurationWhenMetadataCommitFails(t *testing.T) {
	t.Parallel()
	deps, catalog, configs, _ := managementFixture(t)
	metadataErr, restoreErr := errors.New("metadata commit failed"), errors.New("configuration restore failed")
	catalog.updateErr = metadataErr
	configs.failSaveAt, configs.saveErr = 2, restoreErr
	_, err := NewService(deps).UpdateConfiguredProvider(t.Context(), catalog.provider.ID(),
		catalogapplication.UpdateProviderRequest{DisplayName: "Edited"}, json.RawMessage(`{"revision":2}`), managementContext(t))
	if !errors.Is(err, metadataErr) || !errors.Is(err, restoreErr) {
		t.Fatalf("update lost commit or compensation error: %v", err)
	}
	if len(configs.saves) != 2 || configs.saves[0].Revision == configs.original.Revision || configs.saves[1].Revision != configs.original.Revision {
		t.Fatalf("update did not attempt exact configuration restoration: %+v", configs.saves)
	}
}

func TestRebindRestoresSnapshotAndPinAfterPreparationOrActivationFailure(t *testing.T) {
	t.Parallel()
	for _, phase := range []string{"candidate cleanup", "route activation"} {
		t.Run(phase, func(t *testing.T) {
			deps, catalog, configs, plugin := managementFixture(t)
			failure := errors.New(phase + " failed")
			current, err := providerplugin.NewInstalledConfiguration(catalog.provider.ID(), plugin.descriptor.ID,
				"pinst_00000000000000000000000000000001", 1, json.RawMessage(`{"revision":1}`))
			if err != nil {
				t.Fatal(err)
			}
			configs.original, configs.current = current, current
			instances := &instancesFixture{}
			deps.Instances = instances
			if phase == "candidate cleanup" {
				plugin.candidate.closeErr = failure
			} else {
				instances.firstReloadErr = failure
			}
			_, err = NewService(deps).RebindProviderInstallation(t.Context(), catalog.provider.ID(),
				"pinst_00000000000000000000000000000002", json.RawMessage(`{"revision":2}`), managementContext(t))
			if !errors.Is(err, failure) || catalog.restored != 1 || len(catalog.tools) != 1 || catalog.tools[0].ID() != catalog.originalTool.ID() {
				t.Fatalf("rebind lost failure or canonical snapshot: err=%v restores=%d tools=%+v", err, catalog.restored, catalog.tools)
			}
			if phase == "candidate cleanup" {
				if len(configs.saves) != 0 || instances.reloads != 0 {
					t.Fatal("failed candidate cleanup changed installation pin or route")
				}
			} else if len(configs.saves) != 2 || configs.current.InstallationID == nil || *configs.current.InstallationID != *current.InstallationID || instances.reloads != 2 {
				t.Fatalf("failed route activation did not restore exact pin/route: configs=%+v reloads=%d", configs.saves, instances.reloads)
			}
		})
	}
}

func managementFixture(t *testing.T) (Dependencies, *catalogFixture, *configurationsFixture, *managementPlugin) {
	t.Helper()
	now := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	namespace, err := catalogdomain.NewNamespace("fixture")
	if err != nil {
		t.Fatal(err)
	}
	provider, err := catalogdomain.NewProvider("prov_00000000000000000000000000000001", namespace, "Fixture", "", now)
	if err != nil {
		t.Fatal(err)
	}
	schema, err := catalogdomain.NewSchema([]byte(`{"type":"object"}`))
	if err != nil {
		t.Fatal(err)
	}
	tool, err := catalogdomain.NewTool("tool_00000000000000000000000000000001", provider.ID(), catalogdomain.ToolSpec{
		SourceKey: "echo", Name: "echo", InputSchema: schema,
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	plugin := &managementPlugin{descriptor: providerplugin.Descriptor{ID: "dev.example.management", Version: "1.0.0", DisplayName: "Fixture", ConfigVersion: 1}, candidate: &candidateFixture{}}
	config, err := providerplugin.NewConfiguration(provider.ID(), plugin.descriptor.ID, 1, json.RawMessage(`{"revision":1}`))
	if err != nil {
		t.Fatal(err)
	}
	catalog := &catalogFixture{provider: provider, originalTool: tool, tools: []catalogdomain.Tool{tool}}
	configs := &configurationsFixture{original: config, current: config}
	resolver := resolverFixture{plugin: plugin}
	return Dependencies{Catalog: catalog, Configurations: configs, Builtins: resolver, Resolver: resolver, Installations: resolver,
		Activation: activationFixture{}, Clock: managementClock{now}, IsManagedFacade: func(providerplugin.ID) bool { return false }}, catalog, configs, plugin
}

func managementContext(t *testing.T) governancedomain.Context {
	t.Helper()
	principal, err := governancedomain.NewPrincipal(governancedomain.PrincipalService, "provider-management-test")
	if err != nil {
		t.Fatal(err)
	}
	ctx, err := governancedomain.NewContext(governancedomain.OriginInternal, principal, governancedomain.CatalogScope())
	if err != nil {
		t.Fatal(err)
	}
	return ctx
}

// Unused embedded ports panic if a use case unexpectedly crosses another
// boundary; each fixture implements only the operations exercised here.
type catalogFixture struct {
	Catalog
	provider            catalogdomain.Provider
	originalTool        catalogdomain.Tool
	tools               []catalogdomain.Tool
	registered, removed int
	restored            int
	updateErr           error
}

func (c *catalogFixture) Register(context.Context, catalogapplication.RegisterProviderRequest) (catalogdomain.Provider, error) {
	c.registered++
	return c.provider, nil
}
func (c *catalogFixture) Remove(context.Context, catalogdomain.ProviderID) error {
	c.removed++
	return nil
}
func (c *catalogFixture) Get(context.Context, catalogdomain.ProviderID) (catalogdomain.Provider, error) {
	return c.provider, nil
}
func (c *catalogFixture) Update(context.Context, catalogapplication.UpdateProviderRequest) (catalogdomain.Provider, error) {
	return catalogdomain.Provider{}, c.updateErr
}
func (c *catalogFixture) Snapshot(context.Context, catalogdomain.ProviderID) ([]catalogdomain.Tool, error) {
	return append([]catalogdomain.Tool(nil), c.tools...), nil
}
func (c *catalogFixture) Sync(context.Context, catalogdomain.ProviderID, catalogapplication.ToolSource) (catalogapplication.SyncProviderToolsResult, error) {
	c.tools = nil
	return catalogapplication.SyncProviderToolsResult{}, nil
}
func (c *catalogFixture) Restore(_ context.Context, _ catalogdomain.ProviderID, tools []catalogdomain.Tool) error {
	c.restored++
	c.tools = append([]catalogdomain.Tool(nil), tools...)
	return nil
}

type configurationsFixture struct {
	original, current providerplugin.Configuration
	saves             []providerplugin.Configuration
	failSaveAt        int
	saveErr           error
}

func (c *configurationsFixture) GetProviderConfig(context.Context, catalogdomain.ProviderID) (providerplugin.Configuration, error) {
	return c.current, nil
}
func (c *configurationsFixture) Save(_ context.Context, config providerplugin.Configuration) error {
	c.saves = append(c.saves, config)
	if len(c.saves) == c.failSaveAt {
		return c.saveErr
	}
	c.current = config
	return nil
}

type managementPlugin struct {
	providerplugin.Plugin
	descriptor           providerplugin.Descriptor
	validated, inspected int
	candidate            *candidateFixture
}

func (p *managementPlugin) Descriptor() providerplugin.Descriptor { return p.descriptor }
func (p *managementPlugin) ValidateConfig(context.Context, json.RawMessage) error {
	p.validated++
	return nil
}
func (p *managementPlugin) InspectConfig(context.Context, json.RawMessage) (providerplugin.ConfigMetadata, error) {
	p.inspected++
	return providerplugin.ConfigMetadata{}, nil
}
func (p *managementPlugin) Open(context.Context, providerplugin.OpenRequest) (providerplugin.Instance, error) {
	return p.candidate, nil
}

type candidateFixture struct {
	providerplugin.Instance
	closeErr error
}

func (c *candidateFixture) Close() error { return c.closeErr }

type resolverFixture struct{ plugin *managementPlugin }

func (r resolverFixture) Lookup(providerplugin.ID) (providerplugin.Plugin, error) {
	return r.plugin, nil
}
func (r resolverFixture) Resolve(context.Context, providerplugin.Configuration) (providerplugin.Resolution, error) {
	return providerplugin.Resolution{Plugin: r.plugin, Descriptor: r.plugin.descriptor}, nil
}
func (r resolverFixture) ResolveInstallation(context.Context, providerplugin.InstallationID) (providerplugin.Resolution, error) {
	return providerplugin.Resolution{Plugin: r.plugin, Descriptor: r.plugin.descriptor}, nil
}

type activationFixture struct{ err error }

func (a activationFixture) Execute(ctx context.Context, _ provideractivationapplication.Request, operation provideractivationapplication.Operation) error {
	if a.err != nil {
		return a.err
	}
	return operation(ctx)
}

type instancesFixture struct {
	Instances
	firstReloadErr error
	reloads        int
}

func (i *instancesFixture) Reload(context.Context, catalogdomain.ProviderID) (providerplugin.Instance, error) {
	i.reloads++
	if i.reloads == 1 {
		return nil, i.firstReloadErr
	}
	return nil, nil
}

type managementClock struct{ now time.Time }

func (c managementClock) Now() time.Time { return c.now }
