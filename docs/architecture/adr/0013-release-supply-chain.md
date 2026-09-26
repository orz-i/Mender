# ADR-0013: Require verifiable release artifacts and supply-chain evidence

- Status: Accepted
- Date: 2026-09-26

## Context

Phase 7 proved that Mender can build and package a Wails desktop application, but an ad-hoc-signed development package is not a production release.

Mender ships executable code that launches Provider processes and invokes remote tools. Release provenance, dependency vulnerabilities, checksums, and platform signatures therefore affect the security boundary.

The repository currently has ordinary build/test CI but no dedicated vulnerability scan, SBOM, dependency review, checksum manifest, or release publication policy.

## Decision

Mender adds pinned, reproducible supply-chain controls:

- Go source vulnerability analysis with `govulncheck v1.8.0`,
- production JavaScript dependency audit with pnpm,
- GitHub dependency review on pull requests where repository capability allows,
- SPDX JSON SBOM generation using Syft `v1.52.0`,
- SHA-256 checksum manifests for release artifacts,
- native platform signing as a prerequisite for public desktop distribution.

Native signing secrets remain outside the repository and are consumed only through protected CI environments or platform credential stores.

GitHub artifact/SBOM attestations are emitted when the repository/account plan supports them. Attestation is additive; it does not replace checksums or native code signatures.

Public release automation must fail rather than silently publish unsigned desktop artifacts when required signing credentials are unavailable.

MCPB is not adopted as Mender's own application package format. It remains a candidate future import/install format for local MCP Providers.

## Alternatives considered

### Rely only on GitHub release HTTPS

Rejected because transport integrity does not identify build provenance or allow offline artifact verification.

### Checksums without signatures

Checksums detect corruption only when the checksum source itself is trusted. Native code signing and provenance provide different guarantees.

### Native signatures without SBOM

Signatures identify the publisher/build artifact but do not provide dependency inventory for later vulnerability response.

### Add a large release framework immediately

Rejected. Existing Go/Wails build systems already produce artifacts. Phase 8 adds focused integrity/provenance steps without replacing them.

## Consequences

Positive:

- release consumers can verify artifact integrity,
- maintainers can identify shipped dependencies after release,
- pull requests get earlier dependency vulnerability feedback,
- desktop publication has an explicit trust threshold.

Trade-offs:

- vulnerability/SBOM scans require network access,
- native signing requires platform credentials and protected CI configuration,
- release CI becomes more complex and platform-specific.

## Follow-up

Phase 8 implements security gates, SBOM/checksum tooling, release workflow structure, and release documentation.

Production signing credentials are repository/operator configuration and are not generated or committed by Mender.
