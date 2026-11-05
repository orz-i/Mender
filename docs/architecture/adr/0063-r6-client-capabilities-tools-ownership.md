# ADR-0063: Separate client capabilities and consolidate Tools ownership in R6

- Status: Accepted for R6
- Date: 2026-11-05
- Baseline: `4b64e8a` (R5)
- Builds on [ADR-0042](0042-p2-contracts-isolation-complexity.md) and the local owner decisions in [ADR-0052](0052-optional-agent-governed-tool-placement.md) and [ADR-0053](0053-agent-approval-inbox-and-scoped-trust.md).

## Client contract

`packages/sdk/src/contracts/management-client.ts` defines the common
management contract. HTTP and Wails each implement that contract.
`contracts/local-owner.ts` defines separate optional Agent approval and Tool
placement capabilities. Desktop supplies these objects to the app at
composition time. The browser supplies the common client. Approval navigation,
polling and workspace placement controls consume the capability they require.

The desktop host also supplies the callbacks for local Profile CLI recipes
and opening the system browser for OAuth, alongside the existing host callbacks.
Backend owner authorization remains the enforcement point. Capability
presence governs UI availability. OpenAPI and the generated Wails surface
retain their existing operations. R6 temporarily exported `MenderClient`
as a type alias for the common contract. R7 moved the remaining consumers to
`ManagementClient` and retired the alias.

## Feature ownership

- `domains/tools/services` owns Tool service listing and configuration,
  including creation, editing, authentication and installation rebinding.
- `domains/tools/catalog` owns the Provider-scoped Tool browse, search,
  inspection and invocation flow.
- `domains/tools/workspace-targets` owns service Workspace target management.
  The Tools feature exposes route components and source labeling from its
  public entry. Existing routes continue to use canonical `providerId`.
- `shared/plugin-configuration` provides schema defaults, validation and
  configuration controls for Tool services and external Sandbox creation.
  Each feature owns installation selection, credential inventory queries
  and mutation submission.

ESLint keeps the common contract and HTTP adapter independent of local owner
capabilities. It guards shared code from feature imports, keeps external
consumers on the public Tools entry and keeps UI primitives independent of the
app and SDK.

## Verification

- Windows frontend lint, typecheck, localization coverage and Web/Desktop
  frontend builds passed. Vitest passed 485 tests across 72 files.
- HTTP and desktop binding Go suites passed in ordinary and race runs.
  SDK and Wails binding generation produced no diff. The desktop architecture
  check and four ESLint boundary rejection probes passed.
- The existing service-route integration test submits an invocation and
  verifies the selected Tool ID, arguments and rendered result. Existing
  tests cover creation, authentication, rebinding, Profile selection and
  external Sandbox review. Client tests exercise independently supplied owner
  capabilities and the common client's lack of those operations.

R6 verification covers the changed surfaces. The full repository gate is part
of R7. Native Wails window interaction and real external service integration
still require environment acceptance.
