// Package query owns transport-independent management read models and queries.
package query

import (
	"context"
	"time"

	catalogapplication "github.com/orz-i/Mender/backend/internal/catalog/application"
	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
	"github.com/orz-i/Mender/backend/internal/providerplugin"
)

// ProviderView contains only management-safe data. It never contains the raw
// persisted configuration or a live Provider/plugin connection.
type ProviderView struct {
	ProviderID    string
	Namespace     string
	DisplayName   string
	Description   string
	CreatedAt     time.Time
	UpdatedAt     time.Time
	ToolCount     int
	Configuration *ProviderConfigurationView
	Source        *ToolSourceView
}

type ProviderConfigurationView struct {
	PluginID          string
	InstallationID    string
	ConfigVersion     int
	ConnectionSummary string
	Plugin            *ProviderPluginView
	SafeConfig        any
	HasSafeConfig     bool
	Connection        *ProviderConnectionView
}

type ProviderPluginView struct {
	Version     string
	DisplayName string
}

type ProviderConnectionView struct {
	Transport             string
	Endpoint              string
	Headers               map[string]string
	OAuthCredentialRef    string
	CredentialHeaders     []CredentialHeader
	Executable            string
	Args                  []string
	Directory             string
	CredentialEnvironment []CredentialEnvironment
}

type CredentialHeader struct {
	Name          string
	Scheme        string
	CredentialRef string
}

type CredentialEnvironment struct {
	Name          string
	CredentialRef string
}

// ToolSourceOwner is inert attribution from exact resource-to-facade bindings,
// not an authority to invoke a Tool or acquire a resource.
type ToolSourceOwner struct {
	Kind       string
	ResourceID string
	Name       string
}

// Discoverable is shared by management source selection and MCP discovery.
// Availability is an inert host observation, never an execution grant.
func (source ToolSourceOwner) Discoverable(unavailable bool) bool {
	return catalogapplication.SourceDiscoverable(source.Kind, source.Name, source.ResourceID, unavailable)
}

type ToolSourceView struct {
	ToolSourceOwner
	Selectable bool
}

// ProviderViewSource is a consumer-owned read boundary. Host composition adapts
// the owning contexts' application APIs and inert capability observations.
type ProviderViewSource interface {
	ListProviders(context.Context) ([]catalogdomain.Provider, error)
	ListProviderConfigs(context.Context) ([]providerplugin.Configuration, error)
	ListProviderToolCounts(context.Context) (map[catalogdomain.ProviderID]int, error)
	ProviderPluginDescriptorForConfig(context.Context, providerplugin.Configuration) (providerplugin.Descriptor, bool, error)
	ProviderToolSource(context.Context, catalogdomain.Provider, *providerplugin.Configuration) (ToolSourceView, error)
}

// ProviderProjector adapts plugin-owned configuration formats at infrastructure
// boundaries; query/application must not import an individual plugin adapter.
type ProviderProjector func(catalogdomain.Provider, *providerplugin.Configuration, *providerplugin.Descriptor, int) ProviderView

type ListProviderViews struct {
	source  ProviderViewSource
	project ProviderProjector
}

func NewListProviderViews(source ProviderViewSource, project ProviderProjector) ListProviderViews {
	return ListProviderViews{source: source, project: project}
}

func (q ListProviderViews) Execute(ctx context.Context) ([]ProviderView, error) {
	providers, err := q.source.ListProviders(ctx)
	if err != nil {
		return nil, err
	}
	configs, err := q.source.ListProviderConfigs(ctx)
	if err != nil {
		return nil, err
	}
	counts, err := q.source.ListProviderToolCounts(ctx)
	if err != nil {
		return nil, err
	}
	configByProvider := make(map[catalogdomain.ProviderID]providerplugin.Configuration, len(configs))
	for _, config := range configs {
		configByProvider[config.ProviderID] = config
	}
	views := make([]ProviderView, 0, len(providers))
	for _, provider := range providers {
		var config *providerplugin.Configuration
		var descriptor *providerplugin.Descriptor
		if stored, found := configByProvider[provider.ID()]; found {
			config = &stored
			// Preserve the existing degraded-inventory behavior: a missing or
			// invalid descriptor hides metadata, not the registered Provider.
			if value, ok, err := q.source.ProviderPluginDescriptorForConfig(ctx, stored); err == nil && ok {
				descriptor = &value
			}
		}
		view := q.project(provider, config, descriptor, counts[provider.ID()])
		source, err := q.source.ProviderToolSource(ctx, provider, config)
		if err != nil {
			return nil, err
		}
		view.Source = &source
		views = append(views, view)
	}
	return views, nil
}

// SourceOwnerForProvider projects a configured, independently owned Provider only.
// Agent and native tools are projected through Catalog typed SourceRef, not this list.
func SourceOwnerForProvider(provider catalogdomain.Provider, configured bool) ToolSourceOwner {
	if !configured {
		return ToolSourceOwner{Kind: "unclaimed"}
	}
	return ToolSourceOwner{Kind: "provider", ResourceID: provider.ID().String(), Name: provider.DisplayName()}
}
