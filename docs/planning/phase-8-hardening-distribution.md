# Phase 8 Plan — Hardening and Distribution

Status: In Progress

## Objective

Harden Mender's local attack surfaces and provider execution boundaries, make database upgrades fail safely, establish repeatable vulnerability/SBOM/release controls, and validate the Catalog at realistic scale.

Phase 8 is the first phase that treats release artifacts as security-sensitive deliverables rather than development outputs.

It does not enable remote management, invent a credential store, or weaken the shared Invocation pipeline.

## Research baseline

Research date: 2026-09-26.

### Go vulnerability scanning

The Go project recommends `govulncheck` because it uses call reachability to reduce dependency-vulnerability noise. The reviewed current module version is:

`golang.org/x/vuln v1.8.0`

Mender pins the command invocation to that version instead of using floating `@latest`.

References:

- https://go.dev/doc/security/vuln/
- https://pkg.go.dev/golang.org/x/vuln/cmd/govulncheck

### JavaScript vulnerability scanning

Mender uses pnpm's lockfile-backed audit in production dependency scope:

`pnpm audit --prod --audit-level high`

This is a repository gate, not a replacement for dependency review on pull requests.

### GitHub dependency review

GitHub's dependency review action compares dependency changes in a pull request and can fail when newly introduced versions contain known vulnerabilities.

Phase 8 uses:

`actions/dependency-review-action@v4`

with high severity as the initial blocking threshold.

Reference:

- https://docs.github.com/en/code-security/concepts/supply-chain-security/dependency-review

### SBOM

Syft supports SPDX and CycloneDX output for files, directories, and container images.

The reviewed Syft release is:

`v1.52.0` (2026-09-17)

GitHub integration uses `anchore/sbom-action@v0.24.0` with `syft-version: v1.52.0`, so the scanner version is not inherited implicitly from the action bundle.

References:

- https://github.com/anchore/syft/releases
- https://github.com/anchore/sbom-action

### Build provenance

GitHub artifact attestations can provide signed build provenance and SBOM attestations through OIDC where the repository/account plan supports them.

Phase 8 treats attestations as an additional verifiability layer. SHA-256 checksums and native platform signatures remain independent requirements.

Reference:

- https://docs.github.com/en/actions/how-tos/secure-your-work/use-artifact-attestations/use-artifact-attestations

### Wails signing and packaging

Wails v3 provides platform signing tasks:

- macOS Developer ID signing and notarization must run on macOS.
- Windows executable/installer signing can use a trusted code-signing certificate.
- Linux deb/rpm packages can use PGP signing.

Public desktop artifacts must not be published merely because `wails3 package` produced an unsigned or ad-hoc-signed file.

References:

- https://v3.wails.io/guides/build/signing/
- https://v3.wails.io/guides/build/macos/
- https://v3.wails.io/guides/build/windows/
- https://v3.wails.io/guides/build/linux/

### MCPB

MCPB currently defines `.mcpb` ZIP bundles around a local MCP server and `manifest.json`; the current manifest specification is 0.3.

Mender is primarily an MCP/tool host, aggregator, and runtime, not one packaged local MCP provider. Phase 8 therefore does not make MCPB the Mender application distribution format.

MCPB import support may be considered later as a Provider installation source, after trust/signature/install semantics are explicitly designed.

References:

- https://github.com/modelcontextprotocol/mcpb
- https://github.com/modelcontextprotocol/mcpb/blob/main/MANIFEST.md

## Security hardening

### Loopback management request provenance

The management server remains loopback-only.

In addition to listener-address validation, every management HTTP request must reject:

- a non-loopback/non-localhost `Host`,
- a browser `Origin` whose hostname is not loopback/localhost,
- explicit `Sec-Fetch-Site: cross-site`.

Requests with no Origin remain valid for non-browser/local clients.

Mender does not add permissive CORS headers.

This reduces DNS-rebinding and hostile-web-origin exposure without pretending that loopback is equivalent to full authentication.

Remote management remains out of scope until identity/authentication/authorization have an explicit design.

### Provider HTTP transport hardening

Persisted and direct HTTP MCP Provider configuration must:

- allow `http://` only for localhost/loopback hosts,
- require `https://` for non-loopback hosts,
- reject URL userinfo so usernames/passwords/tokens cannot be embedded in persisted endpoint strings,
- continue to reject non-HTTP schemes.

This is transport hardening, not Provider trust endorsement.

### Stdio ambient authority reduction

Local stdio MCP Providers remain arbitrary native processes selected by the local operator.

Mender does not claim kernel-level sandboxing.

The concrete Phase 8 isolation improvement is:

- never invoke through a shell,
- stop inheriting the full Mender process environment,
- pass only a documented non-secret ambient allow-list needed for ordinary process execution,
- allow explicitly injected environment only at the existing infrastructure boundary.

Secret-bearing variables such as tokens, cloud credentials, SSH agent sockets, and arbitrary application environment variables must not flow to stdio Providers implicitly.

Persistent Provider configuration still has no environment/secret fields.

### OS sandbox scope

Portable kernel-level sandboxing is not implemented in Phase 8 because platform mechanisms have materially different semantics and dependencies:

- Linux typically requires a facility such as namespaces/seccomp/bubblewrap.
- macOS sandbox profiles are platform-specific and not equivalent to Linux isolation.
- Windows requires distinct restricted-token/AppContainer/job-object designs.

A future sandbox adapter must be independently testable on each target platform and fail closed when a requested isolation level is unavailable. Phase 8 does not add a misleading cross-platform abstraction without an implementation.

## Database upgrade compatibility

Each SQLite bounded context owns its migrations.

Before applying known migrations, each store must inspect the highest recorded schema version. If the database contains a version newer than the binary understands, opening fails closed.

This protects downgrade/rollback scenarios from an old binary silently operating on a newer schema.

Migration policy:

- migrations are forward-only,
- each migration is transactional,
- existing migration tables remain context-owned,
- unknown future schema versions are rejected,
- destructive migration requires an explicit backup/recovery plan,
- production release notes must call out migration behavior when a release adds one.

Phase 8 does not introduce a second ORM or centralized schema authority.

## Vulnerability and dependency gates

Repository commands:

- `pnpm security:go`
- `pnpm security:js`
- `pnpm security:check`

Go scan runs the pinned govulncheck against both:

- `backend`
- `apps/desktop`

JavaScript scan runs production-only pnpm audit at high severity.

CI adds:

- vulnerability scanning,
- pull-request dependency review,
- automated dependency-update configuration for Go, pnpm, and GitHub Actions where supported.

Security scans query external vulnerability services and therefore remain network-requiring gates.

## SBOM and release integrity

Release artifacts must include:

- SHA-256 checksum manifest,
- SPDX JSON SBOM,
- build/version metadata sufficient to identify the source tag/commit,
- native signatures for desktop artifacts intended for public distribution.

Where GitHub artifact attestations are available, release workflows also emit provenance/SBOM attestations.

Checksums do not replace code signing. Code signing does not replace SBOM/provenance.

## Release policy

Release input is a SemVer tag:

`vMAJOR.MINOR.PATCH`

A public release must be built from a clean tagged commit.

### CLI/server

Go CLI/server binaries are built for supported GOOS/GOARCH combinations without adding desktop dependencies.

Initial matrix:

- linux amd64/arm64,
- darwin amd64/arm64,
- windows amd64/arm64.

### Desktop

Desktop release artifacts are built natively on target OS runners where signature/packaging semantics require it.

Initial native release targets:

- macOS arm64 and/or universal package according to release policy,
- Windows amd64 and arm64 package paths,
- Linux amd64 with GTK4/WebKitGTK 6.0 baseline; arm64 is supported when the runner/build environment is validated.

Wails v3 currently requires GTK4/WebKitGTK 6.0 by default on Linux; the legacy GTK3 path remains opt-in only and is not made Mender's default.

### Signing

Repository files never contain private keys, certificate passwords, Apple app-specific passwords, or PGP secret material.

Release signing consumes protected CI environment secrets or native credential stores.

If required native signing credentials are absent, a workflow may build verification artifacts but must not publish them as a production desktop release.

Phase 8 can establish and test the signing workflow structure without fabricating production credentials.

## Version metadata

Release binaries should expose the build version/commit through a small protocol-independent build-info package and CLI `version` output.

Version values are injected at build time. Development builds use explicit development defaults.

Desktop metadata and release asset names use the same release version.

## Performance and Catalog scale

Catalog performance is validated independently of UI rendering.

Phase 8 adds:

- a deterministic large-catalog integration test,
- FTS search benchmarks with realistic compact metadata,
- benchmark documentation and a reproducible command.

Initial scale target:

- at least 10,000 Tools in one local Catalog,
- search returns a bounded result set without loading Tool schemas eagerly,
- benchmark reports allocations and wall time but does not use a fragile CI wall-clock threshold.

Correctness/scale behavior is a CI test; performance numbers are a tracked benchmark.

## Release/update compatibility

Mender does not implement an in-app auto-updater in Phase 8.

Upgrade flow is explicit:

1. stop old process,
2. preserve/backup the database before any release containing destructive/complex migration,
3. install the new signed artifact,
4. start the new version and allow forward migrations,
5. verify Runtime status and Provider/Profile state.

Downgrades are allowed only when the older binary supports every recorded context schema version; otherwise startup fails.

## Commit segmentation

1. `docs: define hardening and distribution architecture`
2. `fix(api): harden loopback management requests`
3. `fix(provider): reduce upstream provider ambient authority`
4. `fix(sqlite): reject unsupported future schema versions`
5. `ci: add vulnerability and dependency security gates`
6. `build(release): add versioned artifacts checksums and SBOM`
7. `test(catalog): add scale and performance baselines`
8. `docs: complete hardening and distribution phase`

Small corrective commits remain preferable to bundling unrelated fixes.

## Exit checklist

- [ ] Management HTTP rejects non-local Host/Origin and cross-site browser requests.
- [ ] menderd remains loopback-only.
- [ ] Remote HTTP Providers require TLS except loopback.
- [ ] Provider URLs cannot persist embedded userinfo credentials.
- [ ] Stdio Providers do not inherit arbitrary Mender environment variables.
- [ ] Provider tools/annotations remain untrusted for security decisions.
- [ ] OS-level sandbox limits are documented without claiming unsupported isolation.
- [ ] All SQLite stores reject unknown future schema versions.
- [ ] Upgrade/downgrade strategy is documented.
- [ ] Pinned Go and JS vulnerability scans pass.
- [ ] Pull requests have dependency review where repository capabilities permit it.
- [ ] SBOM generation is reproducible with a pinned Syft version.
- [ ] Release artifacts receive SHA-256 checksums.
- [ ] Public desktop release policy requires native signatures.
- [ ] Release provenance/attestation is configured where supported.
- [ ] No signing secrets are committed.
- [ ] MCPB compatibility was evaluated and intentionally scoped.
- [ ] Catalog 10k-scale correctness coverage exists.
- [ ] Catalog search benchmark exists and is documented.
- [ ] Full `pnpm check` passes.
