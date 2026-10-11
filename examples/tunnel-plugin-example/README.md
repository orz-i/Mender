# Tunnel Transport fixture

This is a native third-party `tunnel.transport` protocol example. It uses
only `sdk/tunnelplugin/go`, is an operator-trusted executable, and never
exposes a public MCP listener or talks to OpenAI.

After Mender has started its managed loopback MCP server, the fixture makes
a bounded HTTP GET to that **exact** loopback `/mcp` endpoint, then reports
only the observed local service status. `ready` means that the local MCP
endpoint responded; it does **not** mean public or platform reachability.

Build via `GOWORK=off go build -o bin/mender-tunnel-example ./cmd/mender-tunnel-example`.
Mender owns TunnelID, Profile association, Credential Reference, process
lifecycle, startup rollback and resource usage governance. The ephemeral
Credential is supplied only on Connect and never logged or persisted here.
