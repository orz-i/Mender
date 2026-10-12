package bootstrap

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"

	agentdomain "github.com/orz-i/Mender/backend/internal/agent/domain"
	governancedomain "github.com/orz-i/Mender/backend/internal/governance/domain"
	platformplugin "github.com/orz-i/Mender/backend/internal/plugin"
	installationdomain "github.com/orz-i/Mender/backend/internal/plugin/installation/domain"
	providerplugin "github.com/orz-i/Mender/backend/internal/providerplugin"
	sandboxdomain "github.com/orz-i/Mender/backend/internal/sandbox/domain"
	sandboxpluginregistry "github.com/orz-i/Mender/backend/internal/sandbox/infrastructure/pluginregistry"
)

// PluginCatalogueEntry is an inert, host-reviewed projection of exactly one
// built-in Extension or one exact installation; it never launches plugin code.
type PluginCatalogueEntry struct {
	Key             string              `json:"key"`
	PluginID        string              `json:"pluginId"`
	ExtensionID     string              `json:"extensionId"`
	Kind            platformplugin.Kind `json:"kind"`
	DisplayName     string              `json:"displayName"`
	Description     string              `json:"description"`
	PackageVersion  string              `json:"packageVersion"`
	SourceKind      string              `json:"sourceKind"`
	Status          string              `json:"status"`
	Enabled         bool                `json:"enabled"`
	InstallationID  string              `json:"installationId,omitempty"`
	Digest          string              `json:"digest,omitempty"`
	InstalledAt     string              `json:"installedAt,omitempty"`
	ConfigVersion   uint32              `json:"configVersion,omitempty"`
	ProtocolVersion uint32              `json:"protocolVersion,omitempty"`
	ConfigSchema    json.RawMessage     `json:"configSchema,omitempty"`
	Capabilities    []string            `json:"capabilities"`
	UsageCount      int                 `json:"usageCount"`
	Issue           string              `json:"issue,omitempty"`
}

func pluginEntry(descriptor platformplugin.Descriptor, kind string, installation *installationdomain.Installation, usage int) PluginCatalogueEntry {
	key := descriptor.PluginID.String() + "@builtin"
	status := "enabled"
	enabled := true
	result := PluginCatalogueEntry{
		Key: key, PluginID: descriptor.PluginID.String(), ExtensionID: descriptor.Extension.ID.String(),
		Kind: descriptor.Extension.Kind, DisplayName: descriptor.DisplayName,
		Description: descriptor.Description, PackageVersion: descriptor.PackageVersion,
		SourceKind: kind, Status: status, Enabled: enabled,
		ConfigVersion: descriptor.ConfigVersion, ProtocolVersion: descriptor.Extension.ProtocolVersion,
		ConfigSchema: slices.Clone(descriptor.ConfigSchema),
		Capabilities: slices.Clone(descriptor.Capabilities), UsageCount: usage,
	}
	if installation != nil {
		result.Key = descriptor.PluginID.String() + "@" + installation.ID().String()
		result.InstallationID = installation.ID().String()
		result.Digest = installation.Digest()
		result.InstalledAt = installation.InstalledAt().Format(time.RFC3339Nano)
		result.Status = string(installation.Status())
		result.Enabled = installation.Status() == installationdomain.StatusEnabled
	}
	if result.Capabilities == nil {
		result.Capabilities = []string{}
	}
	return result
}

func (r *Runtime) ListPluginCatalogue(ctx context.Context) ([]PluginCatalogueEntry, error) {
	if r == nil || r.pluginInstallations == nil || r.tunnelPlugins == nil || r.Sandboxes == nil || r.Agents == nil || r.agentLauncher == nil {
		return nil, fmt.Errorf("plugin catalogue unavailable")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	installations, err := r.ListPluginInstallations(ctx)
	if err != nil {
		return nil, err
	}
	configured, err := r.providerConfigs.List(ctx)
	if err != nil {
		return nil, err
	}
	sandboxes, err := r.Sandboxes.List(ctx)
	if err != nil {
		return nil, err
	}
	tunnels, err := r.Tunnels.List.Execute(ctx)
	if err != nil {
		return nil, err
	}
	usage := map[string]int{}
	// A software contribution is not a resource; each configured instance
	// holds a stable, countable exact identity for guarded destruction.
	for _, c := range configured {
		key := c.PluginID.String() + "@builtin"
		if c.InstallationID != nil {
			key = c.PluginID.String() + "@" + c.InstallationID.String()
		}
		usage[key]++
	}
	for _, sandbox := range sandboxes {
		binding := sandbox.Spec().Binding
		key := binding.PluginID.String() + "@builtin"
		if binding.InstallationID != "" {
			key = binding.PluginID.String() + "@" + binding.InstallationID.String()
		}
		usage[key]++
	}
	for _, tunnel := range tunnels {
		binding := tunnel.Binding()
		key := binding.PluginID.String() + "@builtin"
		if binding.InstallationID != "" {
			key = binding.PluginID.String() + "@" + binding.InstallationID.String()
		}
		usage[key]++
	}
	internalCaller, err := governancedomain.NewContext(
		governancedomain.OriginInternal, r.ownerPrincipal, governancedomain.CatalogScope(),
	)
	if err != nil {
		return nil, err
	}
	agents, err := r.Agents.List(ctx, internalCaller)
	if err != nil {
		return nil, err
	}
	for _, a := range agents {
		usage[a.Spec().PluginID.String()+"@builtin"]++
	}
	result := make([]PluginCatalogueEntry, 0, len(installations)+12)
	for _, p := range r.ProviderPluginDescriptors() {
		descriptor, err := providerpluginPlatformDescriptor(p)
		if err != nil {
			return nil, err
		}
		result = append(result, pluginEntry(descriptor, "builtin", nil, usage[descriptor.PluginID.String()+"@builtin"]))
	}
	dockerRegistered := false
	daytonaRegistered := false
	for _, entry := range r.Sandboxes.backendPlugins.Entries() {
		if entry.Kind == sandboxdomain.BackendExternal {
			continue
		}
		if entry.Kind == sandboxdomain.BackendDocker {
			dockerRegistered = true
		}
		if entry.Kind == sandboxdomain.BackendDaytona {
			daytonaRegistered = true
		}
		descriptor := entry.Descriptor
		if err := descriptor.Validate(); err != nil {
			return nil, err
		}
		item := pluginEntry(descriptor, "builtin", nil, usage[descriptor.PluginID.String()+"@builtin"])
		if entry.Kind == sandboxdomain.BackendDocker && !r.Sandboxes.DockerAvailable() {
			item.Status = "unavailable"
			item.Enabled = false
			item.Issue = r.Sandboxes.DockerStatus()
		}
		result = append(result, item)
	}
	if !dockerRegistered {
		descriptor, err := sandboxpluginregistry.DockerCatalogueDescriptor()
		if err != nil {
			return nil, err
		}
		item := pluginEntry(descriptor, "builtin", nil, usage[descriptor.PluginID.String()+"@builtin"])
		item.Status = "unavailable"
		item.Enabled = false
		item.Issue = r.Sandboxes.DockerStatus()
		result = append(result, item)
	}
	if !daytonaRegistered {
		// Daytona's implementation exists but is explicitly quarantined
		// until the host operator enables experimental remote execution.
		// Catalogue projection never resolves a Credential or calls Daytona.
		descriptor, err := sandboxpluginregistry.DaytonaCatalogueDescriptor()
		if err != nil {
			return nil, err
		}
		item := pluginEntry(descriptor, "builtin", nil, usage[descriptor.PluginID.String()+"@builtin"])
		item.Status = "unavailable"
		item.Enabled = false
		item.Issue = r.Sandboxes.DaytonaStatus()
		result = append(result, item)
	}
	// E2B is only a product roadmap preview, not an installed extension:
	// a distinct @planned identity and source MUST NOT enter the Registry
	// or acquire a Sandbox, even through an exact PluginID request.
	result = append(result, PluginCatalogueEntry{
		Key: "io.mender.sandbox.e2b@planned", PluginID: "io.mender.sandbox.e2b",
		ExtensionID: "main", Kind: platformplugin.SandboxBackend,
		DisplayName: "E2B", Description: "Planned Sandbox backend; not implemented or available for provisioning",
		SourceKind: "planned", Status: "planned", Enabled: false,
		Capabilities: []string{}, UsageCount: 0,
	})
	// Agent backends are configured resources and do not produce Tools by
	// themselves. The marketplace snapshot checks only executable existence;
	// it never starts an Agent, probes accounts or imports session secrets.
	for _, backend := range []struct {
		kind              agentdomain.Backend
		name, description string
	}{
		{agentdomain.BackendCodex, "Codex Agent", "Codex CLI through the separately installed codex-acp adapter"},
		{agentdomain.BackendGrok, "Grok Agent", "Grok CLI through its native ACP stdio protocol"},
	} {
		descriptor := platformplugin.Descriptor{
			PluginID: backend.kind.PluginID(), PackageVersion: "1.0.0",
			Extension:   platformplugin.Extension{ID: "main", Kind: platformplugin.AgentBackend, ProtocolVersion: 1},
			DisplayName: backend.name, Description: backend.description, ConfigVersion: 1,
			Capabilities: []string{"acp.v1", "tasks", "local_workspace"},
		}
		if err := descriptor.Validate(); err != nil {
			return nil, err
		}
		item := pluginEntry(descriptor, "builtin", nil, usage[descriptor.PluginID.String()+"@builtin"])
		if _, err := r.agentLauncher.Availability(backend.kind); err != nil {
			item.Status = "unavailable"
			item.Enabled = false
			item.Issue = err.Error()
		}
		result = append(result, item)
	}
	tunnelBuiltin := r.tunnelPlugins.BuiltinDescriptor()
	result = append(result, pluginEntry(tunnelBuiltin, "builtin", nil, usage[tunnelBuiltin.PluginID.String()+"@builtin"]))
	for i := range installations {
		item := installations[i]
		descriptor, ok := item.VerifiedDescriptor()
		if !ok {
			manifest, err := item.Manifest()
			if err != nil {
				return nil, err
			}
			descriptor = platformplugin.Descriptor{
				PluginID: item.PluginID(), PackageVersion: item.PackageVersion(), Extension: item.Extension(),
				DisplayName: manifest.DisplayName, Description: manifest.Description, ConfigVersion: 1,
			}
		}
		entry := pluginEntry(descriptor, "local", &item, usage[item.PluginID().String()+"@"+item.ID().String()])
		if !ok {
			entry.Issue = "No verified extension descriptor; package is unavailable until explicitly verified"
			entry.Status = "invalid"
			entry.Enabled = false
		}
		result = append(result, entry)
	}
	slices.SortFunc(result, func(a, b PluginCatalogueEntry) int { return strings.Compare(a.Key, b.Key) })
	return result, nil
}

func providerpluginPlatformDescriptor(descriptor providerplugin.Descriptor) (platformplugin.Descriptor, error) {
	return providerplugin.PlatformDescriptor(descriptor)
}
