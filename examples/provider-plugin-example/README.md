# Mender Provider Plugin Example

This package is a standalone external Provider plugin built only against the public Go SDK:

~~~text
github.com/orz-i/Mender/sdk/providerplugin/go
~~~

It does not import any Mender backend or `internal/` package.

## Build a local package

From this directory:

~~~bash
mkdir -p bin
go build -o bin/mender-provider-example ./cmd/mender-provider-example
~~~

On Windows use `bin/mender-provider-example.exe`.

The package directory then contains:

~~~text
mender-provider.json
bin/mender-provider-example
~~~

Install it with the Mender CLI:

~~~bash
mender plugin install .
mender plugin list
~~~

Use the returned InstallationID to create a Provider:

~~~bash
mender provider add external   --installation pinst_...   --namespace example-echo   --name "Example Echo"   --config-json '{}'
~~~

The example exposes one Tool with SourceKey `echo`. Its optional `prefix`
configuration changes string values before returning them. Its optional
`credentialRef` demonstrates host-scoped Credential handoff without placing a
secret in Provider configuration.
