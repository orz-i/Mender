# Phase 37 — hard-cut deprecated MCP and Workspace compatibility

- Status: In progress
- Decision: [ADR-0043](../architecture/adr/0043-hard-retire-provider-compatibility.md)
- User outcome: “继续收敛边界，硬切的方式，清理数据，清理代码”.
- Commit docs first, bounded commits thereafter. Preserve `.agents/`, `.pi/`, `.intent-loop/` outside Git.

## Work clusters / done checks

- **M1 — SQLite single source:** schema v4 validates canonical coverage, drops legacy MCP/marker tables atomically; invalid legacy mapping or Workspace v1 blocks upgrade with no partial deletion. Unit/fixture tests.
- **M2 — runtime hard cut:** delete import/double-write and legacy Workspace start migrator, fully remove retired Go adapter/configstore; generic Provider registration, activation, governed deletion and refs remain.
- **M3 — transport/API hard cut:** remove legacy HTTP/Wails routes/methods/DTOs and generated SDK shapes; only generic typed Provider `config` accepted. Update callers/tests. Tests reject deleted endpoints and preserve canonical MCP HTTP/stdio behavior.
- **M4 — close:** full checks, security, clean tracked tree, data upgrade rehearsal on **isolated copied snapshot only**, no app interruption, no live data mutation, document actual before/after and next restart requirements.

## Safety

The live Mender process is currently using the old dual-store runtime. Do not drop tables in its open DB while it runs. Do not erase backups, active credentials, Profiles, Agent history or SSH obligations. The one-way migration on next rebuild/restart removes only previously verified duplicate/obsolete tables. Fail closed on gaps rather than importing missing legacy data.
