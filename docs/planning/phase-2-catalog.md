# Phase 2 Plan — Catalog Domain and Persistence

## Objective

Establish Catalog as the first product bounded context without introducing MCP-specific types or execution behavior.

Phase 2 owns canonical Provider and Tool metadata, provider registration, provider catalog synchronization, SQLite persistence, and initial tool discovery.

It does **not** execute tools. Invocation, credentials, policy, and audit begin in Phase 3.

## Ubiquitous language

### Provider

A Provider is a registered source of tools.

Provider metadata belongs to Catalog. Connection/configuration details required by a future adapter are not part of the core Tool model and must not force MCP concepts into the domain.

### Tool

A Tool is a canonical capability known to Mender.

A Tool belongs to exactly one Provider and contains stable Mender identity, source identity, user/model-facing metadata, JSON Schemas, searchable tags, and timestamps.

### Source key

A source key is the most stable identity an adapter can provide for a tool **within one Provider**.

It is not globally unique and is not necessarily user-facing.

For protocols that provide no identity stronger than tool name, an adapter may use that name as the source key. A source-key change is therefore an identity change from Catalog's perspective.

### Namespace

A Provider namespace is a stable, human-readable Mender identifier used to disambiguate providers.

It is distinct from display name and from the opaque Provider ID.

### Synchronization

Synchronization replaces Catalog's current tool snapshot for one Provider with a newly discovered snapshot while preserving existing Tool IDs for unchanged source keys.

Synchronization is metadata ingestion only. It does not invoke tools.

## Aggregate boundaries

Catalog begins with two aggregate roots:

```text
Provider
  - ProviderID
  - Namespace
  - DisplayName
  - Description
  - CreatedAt
  - UpdatedAt

Tool
  - ToolID
  - ProviderID
  - SourceKey
  - Name
  - Title
  - Description
  - InputSchema
  - OutputSchema?
  - Tags
  - CreatedAt
  - UpdatedAt
```

Provider does not hold a Tool collection in memory. Tool catalogs can become large, so Tool lifecycle and queries are persisted independently while remaining inside the Catalog bounded context.

Deleting Provider lifecycle is intentionally deferred until provider configuration and invocation references exist.

## Identity rules

### Provider identity

- `ProviderID` is an opaque Mender-generated stable ID.
- Provider identity must not be derived from display name, URL, MCP `serverInfo.name`, or another mutable upstream value.
- `Namespace` is unique within one Mender installation and validated independently from ProviderID.
- Display name and description may change without changing ProviderID.

### Tool identity

- `ToolID` is an opaque Mender-generated stable ID.
- The logical synchronization key is `(ProviderID, SourceKey)`.
- A metadata rename does not change ToolID when SourceKey is unchanged.
- A SourceKey change creates a new Tool identity.
- Source keys only have meaning inside their Provider.
- Tool name is metadata, not Mender identity.

Opaque IDs use prefixed random identifiers generated outside the domain model through an application port. Domain code validates IDs but does not depend on randomness.

## Schema rules

Mender stores JSON Schema independently from protocol SDK types.

For Phase 2:

- Input schema is required.
- Output schema is optional.
- JSON Schema roots may be an object or boolean.
- Schema bytes are copied on construction and when exposed so callers cannot mutate domain state.
- Persistence must round-trip valid schema documents without converting them to protocol-specific structures.

Schema dialect negotiation and protocol-specific validation are adapter concerns for later phases.

## Synchronization semantics

The application synchronization use case performs:

```text
Load Provider
   |
   v
Discover source snapshot through ToolSource port
   |
   v
Load current Provider tools
   |
   v
Match by SourceKey
   |
   +--> existing key: preserve ToolID + CreatedAt, refresh metadata
   |
   +--> new key: allocate ToolID + CreatedAt
   |
   v
Atomically replace Provider tool snapshot
```

Tools absent from the new source snapshot are removed from the current Catalog snapshot.

The source snapshot must not contain duplicate SourceKeys. Duplicate keys are rejected before persistence.

Concurrent synchronization of the same Provider is not a Phase 2 requirement. The application API leaves room for later serialization/version checks if remote multi-writer operation requires them.

## Application use cases

Phase 2 introduces the following application operations.

### RegisterProvider

Input:

- Namespace.
- Display name.
- Optional description.

Behavior:

- Validate domain values.
- Allocate ProviderID.
- Reject duplicate namespace.
- Persist Provider.

### GetProvider

Returns one Provider by stable ID.

### SyncProviderTools

Inputs:

- ProviderID.
- Tool source port.

Behavior:

- Require an existing Provider.
- Discover metadata through the source port.
- Preserve IDs for unchanged SourceKeys.
- Atomically replace that Provider's Tool snapshot.

### DescribeTool

Returns one canonical Tool by ToolID.

### SearchTools

Inputs:

- Text query.
- Limit.
- Optional ProviderID filter.

Initial ranking is SQLite FTS5/BM25. Semantic/vector search is explicitly deferred.

## Application ports

Interfaces are defined near consumers.

Expected ports include:

```text
ProviderRepository
ToolRepository
ToolSearch
ToolSource
IDGenerator
Clock
```

Infrastructure implements those ports.

No repository interface belongs in the domain package.

## SQLite persistence

Catalog initially uses one SQLite database through `database/sql`.

Logical schema:

```text
providers
  id              PRIMARY KEY
  namespace       UNIQUE
  display_name
  description
  created_at
  updated_at

tools
  id              PRIMARY KEY
  provider_id     REFERENCES providers(id)
  source_key
  name
  title
  description
  input_schema
  output_schema
  tags_json
  created_at
  updated_at

UNIQUE(provider_id, source_key)

tools_fts
  tool_id
  provider_id
  name
  title
  description
  tags
```

FTS rows are maintained in the same transaction as tool snapshot replacement.

Database schema and SQL row types remain infrastructure concerns.

## Search behavior

Phase 2 search indexes:

- Tool name.
- Tool title.
- Description.
- Tags.

Search returns stable ToolID, ProviderID, name/title, description, and rank information required by the application response.

The first implementation prioritizes deterministic lexical retrieval. Embeddings and semantic ranking are deferred until usage justifies a Discovery bounded context.

## Work packages and commit segmentation

### 1. Domain design and persistence decision

- Add this plan.
- Record SQLite implementation decision.

Suggested commit:

```text
docs: define catalog domain and persistence plan
```

### 2. Catalog domain

Implement and test:

- ProviderID / ToolID.
- Namespace.
- JSON Schema value object.
- Provider.
- Tool.
- Domain validation errors.

Suggested commit:

```text
feat(catalog): add canonical provider and tool domain
```

### 3. Catalog application layer

Implement and test:

- RegisterProvider.
- GetProvider.
- SyncProviderTools.
- DescribeTool.
- SearchTools.
- Consumer-owned ports.
- Fake ports for application tests.

Suggested commit:

```text
feat(catalog): add catalog application use cases
```

### 4. SQLite infrastructure

Implement:

- Database opening/configuration.
- Catalog schema migration.
- Provider repository.
- Tool repository and atomic snapshot replacement.
- FTS search adapter.
- Integration tests using temporary databases.

Suggested commit:

```text
feat(catalog): add sqlite catalog persistence
```

### 5. Composition and documentation

Wire Catalog storage into the composition root without adding HTTP/MCP/UI features.

Update roadmap and development documentation.

Suggested commits:

```text
build(catalog): wire catalog persistence
docs: complete catalog foundation phase
```

## Exit checklist

- [ ] Canonical Tool model has no MCP SDK dependency.
- [ ] Provider identity rules are tested.
- [ ] Tool identity preservation across synchronization is tested.
- [ ] Duplicate source keys are rejected.
- [ ] Input/output schemas round-trip through persistence.
- [ ] Register/Get Provider use cases work through ports.
- [ ] Search/Describe Tool queries work through application use cases.
- [ ] SQLite repository integration tests pass.
- [ ] FTS search returns expected tools.
- [ ] `pnpm check` passes.
