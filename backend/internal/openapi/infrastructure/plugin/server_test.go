package plugin

import (
	"strings"
	"testing"
)

func TestResolveEffectiveServerPrecedenceAndVariables(t *testing.T) {
	t.Parallel()

	parsed := loadSchemaFixture(t, `openapi: 3.1.2
info:
  title: Demo
  version: "1"
servers:
  - url: https://root.example.test/{version}
    variables:
      version:
        default: v1
paths:
  /users:
    servers:
      - url: https://path.example.test/base
    get:
      operationId: listUsers
      servers:
        - url: https://operation.example.test/{tenant}
          variables:
            tenant:
              default: acme
      responses:
        "200":
          description: ok
`)
	pathItem := parsed.document.Paths.Value("/users")
	operation := pathItem.Get

	server, err := resolveEffectiveServer(parsed, Config{}, pathItem, operation)
	if err != nil {
		t.Fatalf("resolveEffectiveServer() error = %v", err)
	}
	if server != "https://operation.example.test/acme" {
		t.Fatalf("server = %q", server)
	}

	override, err := resolveEffectiveServer(
		parsed,
		Config{ServerURL: "https://override.example.test/api"},
		pathItem,
		operation,
	)
	if err != nil {
		t.Fatalf("override error = %v", err)
	}
	if override != "https://override.example.test/api" {
		t.Fatalf("override = %q", override)
	}
}

func TestResolveEffectiveServerUsesPathThenRoot(t *testing.T) {
	t.Parallel()

	parsed := loadSchemaFixture(t, `openapi: 3.1.2
info:
  title: Demo
  version: "1"
servers:
  - url: https://root.example.test/api
paths:
  /path:
    servers:
      - url: https://path.example.test/api
    get:
      operationId: path
      responses:
        "200":
          description: ok
  /root:
    get:
      operationId: root
      responses:
        "200":
          description: ok
`)

	pathItem := parsed.document.Paths.Value("/path")
	server, err := resolveEffectiveServer(parsed, Config{}, pathItem, pathItem.Get)
	if err != nil {
		t.Fatal(err)
	}
	if server != "https://path.example.test/api" {
		t.Fatalf("path server = %q", server)
	}

	rootItem := parsed.document.Paths.Value("/root")
	server, err = resolveEffectiveServer(parsed, Config{}, rootItem, rootItem.Get)
	if err != nil {
		t.Fatal(err)
	}
	if server != "https://root.example.test/api" {
		t.Fatalf("root server = %q", server)
	}
}

func TestResolveEffectiveServerRelativeURLSource(t *testing.T) {
	t.Parallel()

	parsed := loadSchemaFixture(t, `openapi: 3.1.2
info:
  title: Demo
  version: "1"
servers:
  - url: /v1
paths: {}
`)
	parsed.location.Scheme = "https"
	parsed.location.Host = "example.test"
	parsed.location.Path = "/spec/openapi.yaml"

	server, err := resolveEffectiveServer(parsed, Config{}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if server != "https://example.test/v1" {
		t.Fatalf("server = %q", server)
	}
}

func TestResolveEffectiveServerRejectsRelativeFileSource(t *testing.T) {
	t.Parallel()

	parsed := loadSchemaFixture(t, `openapi: 3.1.2
info:
  title: Demo
  version: "1"
servers:
  - url: /v1
paths: {}
`)
	_, err := resolveEffectiveServer(parsed, Config{}, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "explicit serverUrl") {
		t.Fatalf("resolveEffectiveServer() error = %v", err)
	}
}

func TestResolveEffectiveServerRejectsRemoteHTTP(t *testing.T) {
	t.Parallel()

	parsed := loadSchemaFixture(t, `openapi: 3.1.2
info:
  title: Demo
  version: "1"
servers:
  - url: http://example.test/api
paths: {}
`)
	_, err := resolveEffectiveServer(parsed, Config{}, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "https") {
		t.Fatalf("resolveEffectiveServer() error = %v", err)
	}
}
