# Phase 26 — Profile to Sandbox Delegation

Status: In progress. Governed by ADR-0036.

Goal: allow a Profile MCP service to acquire one explicitly authorized Sandbox capability without changing ownership or weakening Policy.

Acceptance: deny by default; grant and revoke by the Sandbox owner only; only named existing Profile IDs and read/search/edit/shell capabilities; trusted Profile Principal and matching Origin/Scope; atomic lease reservation checks; immutable grant/revoke audit; Policy enforcement remains independent; no lifecycle authority granted; typed HTTP/SDK/Wails and focused UI; temporary-database end-to-end MCP tests and full repository gates.

Work packages: A docs; B SQLite v8 and atomic authorization; C management API/SDK/Wails; D UI; E acceptance and closeout. Separate commits. No changes to user Sandboxes, secrets, or Tunnel. Exclude skill directories. Deferred: remote individual identities and revoking live in-flight leases.
