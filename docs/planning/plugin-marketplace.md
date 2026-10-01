# Plugin Marketplace

Status: In Progress

## Goal

Mender now has a first-class Plugin Marketplace that is the shared discovery and lifecycle surface for plugin packages.

The Marketplace manages **plugin packages**, not configured runtime resources.

Configured Provider instances remain under `Providers`; Provider plugin packages live in Marketplace.

## First supported plugin kind

- Provider Plugin

The page itself is plugin-kind agnostic. Future plugin kinds join by registering Marketplace adapters without changing the page layout, routing, filters, or detail interaction.

## Marketplace model

The App layer consumes a normalized projection:

```text
MarketplacePlugin
  key
  kind
  kindLabel
  displayName
  description
  source
  sourceLabel
  version
  status
  enabled
  capabilities[]
  actions
  technicalDetails[]  # detail only
  actionRef?          # internal lifecycle reference
  issue?
```

A Marketplace adapter owns:

- loading one plugin kind;
- projecting its native data into `MarketplacePlugin`;
- optional enable/disable lifecycle;
- optional removal lifecycle.

Provider Plugin is the first adapter.

## Provider Plugin aggregation

Provider Marketplace data is composed from:

- `listProviderPlugins()` for built-in and enabled external descriptors;
- `listProviderPluginInstallations()` for exact external installation state;
- Provider plugin issues for degraded installation state.

`ProviderPluginInstallation` now includes optional verified descriptor metadata:

- displayName;
- description;
- configVersion;
- protocolVersion;
- capabilities.

These fields come from the already persisted immutable verified descriptor snapshot. They do not activate plugin code and do not expose secrets.

This keeps disabled/degraded external plugins visually stable instead of falling back to a guessed name derived from pluginId.

## Information architecture

```text
Plugin Marketplace
  -> Search
  -> Type / Source / Status filters
  -> Plugin inventory table
  -> Focused Plugin Details Dialog
     -> overview
     -> Technical Details
```

Primary list columns:

- Plugin
- Type
- Source
- Version
- Status
- Actions

Technical identifiers are hidden from the primary table.

Plugin ID, Installation ID, digest, installed timestamp, config version, and protocol version are visible only in Technical Details.

## Lifecycle rules

Built-in Provider plugins:

- visible in Marketplace;
- always available through the built-in source;
- cannot be removed;
- expose no external installation controls.

External Provider plugin installations:

- installed through Desktop local-directory capability or CLI;
- enabled/disabled from Marketplace;
- removable only when not referenced by configured Providers;
- exact InstallationID remains authoritative;
- disabled installations remain visible;
- degraded disabled installations cannot be enabled from Marketplace;
- enabled degraded installations can still be disabled/removed.

Web Marketplace never accepts arbitrary server filesystem paths.

## Install capability

Marketplace receives host installation capabilities as generic `MarketplaceInstaller[]` values.

The first installer adapts the existing Desktop-only Provider plugin local-directory picker:

```text
installProviderPlugin()
  -> providerPluginInstaller(...)
  -> MarketplaceInstaller
  -> Plugin Marketplace primary action
```

Future plugin installers can be added without adding Provider-specific props to the Marketplace page.

## Providers relationship

`Providers` manages configured Provider instances and their connection/authentication state.

`Plugin Marketplace` manages Provider plugin packages.

The Providers page no longer owns:

- Manage Plugins action;
- external package installation UI;
- enable/disable/remove installation UI.

Providers still loads available Provider plugin descriptors because Provider creation/edit/rebind requires them.

Provider degradation is summarized in Providers as an attention notice; exact installation diagnostics live in Marketplace.

## UX rules

- Follow the current Command Workbench visual system.
- No statistic cards outside Overview.
- One table cell = one information point.
- No technical IDs in primary tables.
- Details use focused Dialogs.
- Plugin type and source filter options derive from normalized Marketplace data.
- Local package installation appears only when the current host supplies an installer capability.
- Web never accepts a server filesystem path.
- Degraded plugin state is fail-closed for enable actions.

## Verification

- [ ] Marketplace route and Sidebar/Quick Navigation entry exist.
- [ ] Built-in MCP, Model Tools, OpenAPI, and Workspace Provider plugins render.
- [ ] External Provider plugin installations render alongside built-ins.
- [ ] Disabled external installations remain visible.
- [ ] Verified descriptor metadata keeps external plugin names stable across enable/disable.
- [ ] PluginID / InstallationID / digest remain detail-only.
- [ ] Type/source/status filters are available.
- [ ] Plugin overview and Technical Details are separate Dialog views.
- [ ] Enable/disable/remove lifecycle uses generic Marketplace actions.
- [ ] Degraded disabled plugins cannot be enabled.
- [ ] Desktop local package installation is injected through MarketplaceInstaller.
- [ ] Web exposes no arbitrary path install action.
- [ ] Providers no longer exposes duplicate plugin-management UI.
- [ ] i18n, targeted Go/TS tests, Web build, and Desktop build pass.

## Evidence

Browser QA used an isolated Runtime containing:

- four built-in Provider plugins;
- one external `Example Echo` plugin installed from the repository example package.

The external plugin was disabled and re-enabled through Marketplace. Its display name remained `Example Echo` while disabled because the installation management projection now carries the persisted verified descriptor metadata.

Representative captures:

- `/tmp/mender-plugin-marketplace-final.png`
- `/tmp/mender-plugin-marketplace-external.png`
- `/tmp/mender-plugin-marketplace-detail.png`
- `/tmp/mender-plugin-marketplace-mobile.png`
