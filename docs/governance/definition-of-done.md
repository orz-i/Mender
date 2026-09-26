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
