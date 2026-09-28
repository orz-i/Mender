# Mender Provider Plugin Go SDK

This module is the public Go SDK for external Mender Provider plugins.

Plugin implementations import:

~~~go
import providerplugin "github.com/orz-i/Mender/sdk/providerplugin/go"
~~~

They do not need to import Mender backend packages, protobuf-generated types, or HashiCorp go-plugin directly.

## Implement a plugin

Implement `providerplugin.Plugin`:

~~~text
Descriptor
ValidateConfig
InspectConfig
Open
ListTools
ExecuteTool
Close
~~~

Then serve it from `main`:

~~~go
func main() {
    if err := providerplugin.Serve(myPlugin); err != nil {
        log.Fatal(err)
    }
}
~~~

`EventSender` maps execution into Mender's canonical progress and terminal outcome model. An execution may emit multiple progress events and must emit exactly one terminal event.

Credentials passed to `Open`, `ListTools`, or `ExecuteTool` are ephemeral, host-scoped values. Do not persist them or place them in logs. `FindCredential` returns a copy of a credential value, and `ZeroCredentials` can be used to clear caller-owned copies.

## Protocol compatibility

The SDK owns process transport and Provider Plugin Protocol negotiation. Non-Go plugins may implement the versioned protobuf contract under `contracts/providerplugin/v1` directly.

Package version, protocol version, and Provider config version are separate compatibility dimensions.

## Example

See `examples/provider-plugin-example` for a complete installable package with:

- a manifest;
- JSON Schema configuration;
- optional Credential Reference handling;
- Tool discovery;
- progress reporting;
- governed execution;
- tests and packaging instructions.
