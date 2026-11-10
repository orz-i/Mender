# Runtime v3 upgrade and recovery

Mender uses a single typed v3 SQLite schema and a single Tool SourceRef-aware Invocation. The old Provider-v2 resource-facade schema is **not** supported. Startup must reject an existing v2 database without partial changes, remote activation or automatic migration.

For a clean v3 installation, stop Mender and create a fresh app data directory or remove the retired legacy SQLite database **after a consistent verified backup**. The Desktop default is `~/Library/Application Support/Mender/mender.db` on macOS; CLI/menderd default is `~/.mender/mender.db` unless `MENDER_DB`/`--db` points elsewhere. On first start, Mender creates its pinned empty v3 image. Do not copy an old v2 database back into the v3 location; restore old data only with the matching historical binary in an isolated environment.

No legacy ToolID, Agent Task, SSH Job, Sandbox grant or Profile exposure is automatically imported. Operators must reconfigure new Agent/Workspace/Sandbox resources and grants deliberately. A prior backup is recovery evidence only and must never be injected into active v3 Tool discovery.

For backend architecture and verification requirements see [v3 system context](../architecture/context.md) and [runtime hard-cut acceptance](../governance/runtime-v3-hardcut.md). For release/build operations see [release procedures](release.md).
