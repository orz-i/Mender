# Plugin Marketplace

Status: Complete

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
  createAction?       # detail-only handoff into the owning resource workflow
  actionRef?          # internal lifecycle reference
  issue?
```

A Marketplace adapter owns:

- loading one plugin kind;
- projecting its native data into `MarketplacePlugin`;
- optional enable/disable lifecycle;
- optional removal lifecycle;
- optional resource-creation handoff for plugins that can create a configured resource.

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
  -> Responsive Plugin card grid
  -> Key-cursor Previous / Next navigation
  -> /plugins/:pluginKey
     -> Plugin overview
     -> Capabilities / issue state
     -> Technical details
     -> Package lifecycle actions
     -> Add -> owning configured-resource creation flow
```

Primary card anatomy:

- display name;
- short description, visually clamped so one long description cannot distort the grid;
- plugin type;
- source;
- version;
- status;
- concise capability summary.

Cards are discovery/navigation surfaces rather than lifecycle-control surfaces. Activating a card opens the dedicated detail route.

Cursor pagination uses the stable normalized Marketplace plugin `key` as the client cursor. The catalog exposes Previous / Next navigation and does not present page-number semantics.

Technical identifiers are hidden from the catalog cards. Plugin ID, Installation ID, digest, installed timestamp, config version, and protocol version are visible only on the detail page.

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

A create-capable Provider plugin detail exposes `Add provider`. That action navigates to the existing Providers creation workflow with the exact built-in or external plugin identity preselected. Marketplace does not duplicate the Provider configuration form.

Provider degradation is summarized in Providers as an attention notice; exact installation diagnostics live in Marketplace.

## UX rules

- Follow the current Command Workbench visual system and existing shadcn/base-nova component language.
- Marketplace discovery uses a responsive card grid; cards remain concise and description text is clamped.
- No statistic cards outside Overview.
- No technical IDs on discovery cards.
- Plugin details use a dedicated route rather than a Dialog so metadata, lifecycle controls, and the create handoff have stable navigation semantics.
- Cursor controls expose Previous / Next only; no numeric page selector is shown.
- Plugin type and source filter options derive from normalized Marketplace data.
- Local package installation appears only when the current host supplies an installer capability.
- Web never accepts a server filesystem path.
- Degraded plugin state is fail-closed for enable actions.
- `Add` delegates to the configured-resource owner; for Provider plugins this is the existing Providers create flow.

## Verification

### Card catalog / routed-detail iteration

- [x] Marketplace results render as a responsive card grid.
- [x] Marketplace uses key-cursor Previous / Next navigation rather than table/page-number pagination.
- [x] Clicking a card navigates to `/plugins/:pluginKey`.
- [x] Detail route preserves overview, capabilities, issue, technical metadata, and package lifecycle behavior.
- [x] Create-capable details expose `Add provider` and open the existing Providers create flow with the exact plugin preselected.
- [x] Desktop and mobile rendered QA pass for catalog -> detail -> Add.
- [x] Targeted tests, i18n, typecheck, lint, Web build, and Desktop build pass.

### Existing baseline

- [x] Marketplace route and Sidebar/Quick Navigation entry exist.
- [x] Built-in MCP, Model Tools, OpenAPI, and Workspace Provider plugins render.
- [x] External Provider plugin installations render alongside built-ins.
- [x] Disabled external installations remain visible.
- [x] Verified descriptor metadata keeps external plugin names stable across enable/disable.
- [x] PluginID / InstallationID / digest remain detail-only.
- [x] Type/source/status filters are available.
- [x] Plugin overview and Technical Details are available on the dedicated plugin detail route.
- [x] Enable/disable/remove lifecycle uses generic Marketplace actions.
- [x] Degraded disabled plugins cannot be enabled.
- [x] Desktop local package installation is injected through MarketplaceInstaller.
- [x] Web exposes no arbitrary path install action.
- [x] Providers no longer exposes duplicate plugin-management UI.
- [x] i18n, targeted Go/TS tests, Web build, and Desktop build pass.

## Evidence

Browser QA used an isolated Runtime containing:

- four built-in Provider plugins;
- one external `Example Echo` plugin installed from the repository example package.

The external plugin was disabled and re-enabled through Marketplace. Its display name remained `Example Echo` while disabled because the installation management projection now carries the persisted verified descriptor metadata.

Representative baseline captures:

- `/tmp/mender-plugin-marketplace-final-qa.png`
- `/tmp/mender-plugin-marketplace-detail.png`
- `/tmp/mender-plugin-marketplace-technical.png`
- `/tmp/mender-plugin-marketplace-mobile.png`

Card catalog / routed-detail iteration captures:

- `/tmp/mender-marketplace-card-grid.png`
- `/tmp/mender-marketplace-detail.png`
- `/tmp/mender-marketplace-add-provider.png`
- `/tmp/mender-marketplace-mobile.png`
- `/tmp/mender-marketplace-detail-mobile.png`
- `/tmp/mender-marketplace-qa.png`

Rendered QA verified the route `Plugin Marketplace -> MCP detail -> Add provider -> Providers create dialog`; the MCP plugin remained preselected after the handoff. The 390 x 844 viewport had no horizontal overflow. The browser QA preset passed with network, console, and page-error checks enabled.

Final repository verification:

- `pnpm check`: PASS
- `pnpm security:check`: PASS
- i18n: 638 / 638 zh-CN
- Vitest: 10 files / 67 tests
- Web production build: PASS
- Desktop production build: PASS
- govulncheck: no reachable vulnerabilities
- production dependency audit: no known vulnerabilities
