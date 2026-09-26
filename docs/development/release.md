# Release and Distribution

Mender treats release artifacts as security-sensitive deliverables.

The automated release workflow currently publishes the pure-Go CLI and daemon. Desktop artifacts have additional native signing requirements and are not published by the generic Go release workflow.

## Release input

Public releases use a SemVer tag:

```text
vMAJOR.MINOR.PATCH
```

Prerelease suffixes are accepted by the local builder when explicitly supplied.

A release build must start from a clean tagged commit.

Local verification may set `MENDER_ALLOW_DIRTY=1`, but artifacts produced that way are development verification artifacts and must not be published.

## Build metadata

Release builds inject:

- version,
- source commit,
- build date.

Inspect a CLI binary with:

```bash
mender --output json version
```

Development builds report `dev` / `unknown` defaults.

The daemon also writes its build metadata to stderr during startup.

## Go CLI/server artifacts

Build the full supported pure-Go matrix:

```bash
MENDER_VERSION=v0.1.0 pnpm release:go
```

The initial matrix is:

- linux/amd64,
- linux/arm64,
- darwin/amd64,
- darwin/arm64,
- windows/amd64,
- windows/arm64.

Each target produces versioned, uniquely named `mender` and `menderd` binaries under `dist/release/go`.

The release builder sets `CGO_ENABLED=0` and injects version metadata with Go linker flags. It does not depend on Wails.

## SBOM

The repository pins Syft in:

```text
.syft-version
```

Generate the SPDX JSON SBOM:

```bash
pnpm release:sbom
```

The generated file is:

```text
dist/release/mender-go.spdx.json
```

Do not silently upgrade the Syft version in CI. Review and commit the version change.

## Checksums

Generate SHA-256 checksums after the final release files and SBOM exist:

```bash
pnpm release:checksums
```

Verify them:

```bash
pnpm release:checksums:verify
```

The manifest is:

```text
dist/release/SHA256SUMS
```

Checksums verify artifact integrity. They do not replace publisher identity, native code signing, or provenance.

## GitHub release workflow

`.github/workflows/release.yml` runs on version tags.

It:

1. builds the six-target Go CLI/server matrix,
2. generates an SPDX SBOM,
3. generates and verifies SHA-256 checksums,
4. creates GitHub artifact attestations when supported,
5. publishes the versioned artifacts, build metadata, SBOM, and checksum manifest to the GitHub Release.

Artifact attestation runs automatically for public repositories. Private/internal repositories may opt in with:

```text
MENDER_ENABLE_ATTESTATIONS=true
```

only when their GitHub plan supports artifact attestations.

The release workflow uses GitHub OIDC-backed attestations. No long-lived attestation signing key is stored in the repository.

## Desktop release policy

A successful `wails3 package` is not by itself a production desktop release.

Public desktop artifacts require native platform signatures.

### macOS

The supported desktop baseline is macOS 13 Ventura or newer, matching the Go 1.27 runtime/toolchain requirement. Public macOS applications require a Developer ID signature and notarization.

The checked-in Wails tasks provide signing/notarization entry points under the Darwin task namespace. Signing identity and notarization credentials must come from the native credential store or protected CI environment.

An ad-hoc signature is acceptable only for local development verification.

### Windows

Public Windows executables/installers require an Authenticode signing certificate from a trusted publisher identity.

The checked-in Wails Windows tasks support executable and installer signing. Certificate material/passwords must not be committed.

### Linux

Linux AppImage artifacts are accompanied by release checksums/provenance.

When deb/rpm repository-style packages are publicly distributed, the Wails packaging tasks support PGP signing. Private PGP keys and passphrases remain outside the repository.

## Desktop CI publication

Phase 8 intentionally does not publish unsigned desktop packages from generic CI.

Before automated desktop publication is enabled, each target platform must have:

- protected signing credentials,
- a native runner or validated build environment,
- signature verification after packaging,
- release artifact naming tied to the SemVer tag,
- SBOM/checksum/provenance handling equivalent to the Go release path.

If the required signing credentials are unavailable, CI may build a verification artifact, but it must not label or publish it as a production desktop release.

## Signing secret policy

Never commit:

- private signing keys,
- PFX/P12 certificate files containing private keys,
- certificate passwords,
- Apple app-specific passwords,
- notarization credentials,
- PGP secret keys,
- cloud KMS credentials,
- GitHub personal access tokens used for signing/release.

Use protected CI secrets, OIDC federation, hardware-backed keys, or native credential stores.

## Dependency security

Run the network-requiring dependency scans:

```bash
pnpm security:check
```

This runs:

- pinned `govulncheck` against `backend`,
- pinned `govulncheck` against `apps/desktop`,
- production-only pnpm audit with a high-severity blocking threshold.

Pull requests also use GitHub dependency review where repository capabilities permit it.

## MCPB scope

MCPB is not Mender's application distribution format.

MCPB packages a local MCP server plus manifest. Mender is primarily the host/aggregator/runtime that consumes Providers and exposes profiles.

A future Provider installer may accept MCPB as an installation source only after package trust, signature verification, filesystem extraction, executable ownership, update, and uninstall semantics are explicitly designed.

## Release checklist

Before publishing:

- clean tagged commit,
- `pnpm check` passes,
- `pnpm security:check` passes,
- Go release matrix succeeds,
- SBOM is generated,
- SHA-256 checksums verify,
- release migration notes are reviewed,
- no signing secret is present in source/artifacts/logs,
- any public desktop artifact is natively signed,
- attestations are emitted where supported.
