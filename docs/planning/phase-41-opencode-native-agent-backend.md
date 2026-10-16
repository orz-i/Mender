# Phase 41 — OpenCode native ACP Agent backend

- Status: active (2026-10-16).
- Authority: [ADR-0048](../architecture/adr/0048-opencode-native-acp-agent-backend.md).

## Intent contract

**Goal:** the local OpenCode CLI is a first-class built-in Agent Backend. Its marketplace card can open the Agent-specific creation flow. An owner may select it once, and use model/effort/workspace defaults or task overrides under the existing Agent Delegation and Profile governance. Nothing in this change constructs an Agent planning/inference loop.

**Evidence:** local `opencode v2.0.30` and explicit `opencode acp` confirmed. Safe test-created ACP v1 Session offered 44 models, thought level as selector ID `effort`, and mode options; no prompts sent.

**Non-goals:** automatic login, creating a user Agent without its workspace/isolation selection, synthetic model list or universal thinking levels, managing OpenCode background service/API, direct remote Agent execution, implicit ACP permissions, hidden compatibility adapters.

**Preservation:** hard-cut to v8 schema; preserve all historical and active identities; do not touch `.agents/`, `.pi/`, `.intent-loop/`, `.commandcode/`. No paid external prompt or modification of the live production Agent inventory for tests.

## Sequential checkpoints

| Commit | Scope | Gate |
|---|---|---|
| A | ADR-0048 and phase contract | Docs before implementation |
| B | Domain/plugin ID, bounded executable launcher and forward-only v8 table constraint migration | Go domain/Agent store/launcher tests, FK and identity snapshots |
| C | Marketplace, desktop/UI, OpenAPI/SDK and CLI surface | Catalogue/creation/permissions tests, i18n/typecheck and generated binding parity |
| D | Integration & close | Real local ACP v1 handshake/session and inspection (no prompt), fixture task path, `pnpm check`, documentation and clean tracked git status |

Do not treat a partial check, detected executable or available model inventory as proof of real model entitlement. If client capability requests are not supported, continue failing them closed.
