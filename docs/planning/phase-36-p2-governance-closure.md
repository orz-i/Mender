# Phase 36 — P2 architecture governance closure

- Status: CONTRACT ACCEPTED; implementation pending.
- Architecture: [ADR-0042](../architecture/adr/0042-p2-contracts-isolation-complexity.md)
- Operate on `main` with document-first, bounded commits. Preserve `.agents/`, `.pi/`, `.intent-loop/`.

## Outcomes and checks

- **A · cross-context invariants:** Agent schema v5 upgrade, immutable claimed facade config version, exact Claim version, two-Runtime regression, persistence contract consistency.
- **B · native plugin authority:** first-installation external Provider restricted-host where verified, existing explicit host retained, UI warning/confirmation and i18n, isolation negative tests.
- **C · maintainability:** isolate pure Provider form/config projection and one Core Runtime wiring responsibility without altering identity or contracts. Unit and integration parity checks.
- **Close:** full repo Go/race/lint + UI + SDK/generation + security gates, separate commits, no production mutations. Document incomplete external packaged/host acceptance honestly.

## Constraints

No new compatibility bridge or speculative plugin framework. No mixing Tool Suite, Agent, Sandbox and Plugin ownership. Do not modify the production App/SQLite/Tunnel/SSH process. A confirmation is not an effective sandbox boundary: host mode remains high authority. No mass upgrade of persisted Runtime isolation choices.
