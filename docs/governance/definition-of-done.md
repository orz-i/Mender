# Definition of Done

A change is done when all criteria relevant to its scope are satisfied.

## Scope and design

- The change has a clear problem statement and bounded scope.
- Non-goals are understood for significant work.
- Accepted ADRs are followed.
- Architecture changes have an ADR.
- Domain language remains consistent across affected layers.

## Implementation

- Dependency direction remains valid.
- Infrastructure/framework types do not leak into domain code.
- Cross-context behavior uses explicit boundaries.
- New abstractions have a current consumer.
- Error, cancellation, and timeout behavior are considered where relevant.
- Security-sensitive data is not logged or persisted unnecessarily.

## Verification

- Tests cover behavior at the appropriate layer.
- Static analysis passes.
- Formatting passes.
- Build passes for affected artifacts.
- Integration/contract tests exist for changed infrastructure boundaries.
- Regression coverage accompanies bug fixes where practical.
- Persistence changes test upgrade behavior and rejection of unsupported future schema versions where applicable.

## Documentation

- User/developer documentation reflects changed behavior.
- Contracts are regenerated reproducibly if applicable.
- ADR index is updated for new/superseded decisions.
- Planning exit criteria are updated when completing a milestone.

## Reviewability

- Commits are logically segmented.
- No unrelated formatting or refactoring obscures the change.
- Generated files are separated or clearly identified when they would otherwise hide source changes.

## Security

For changes involving tools or execution:

- Policy path is not bypassed.
- Credential resolution remains at the execution boundary.
- Audit data excludes secret values.
- Untrusted provider metadata is not treated as an enforcement guarantee.
- New process/network boundaries minimize ambient authority and document any unavailable sandbox guarantee.

For release/distribution changes:

- dependency vulnerability scans pass,
- generated release artifacts have reproducible SBOM/checksum generation,
- signing credentials are not committed or logged,
- public desktop artifacts require native platform signatures,
- migrations and downgrade implications are documented in release notes when applicable.
