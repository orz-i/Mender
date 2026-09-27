package plugin

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

func TestLoadOpenAPIDocumentSupportsReviewedVersions(t *testing.T) {
	t.Parallel()

	versions := []string{"3.0.4", "3.1.2", "3.2.1"}
	for _, version := range versions {
		version := version
		t.Run(version, func(t *testing.T) {
			t.Parallel()

			path := filepath.Join(t.TempDir(), "openapi.yaml")
			spec := "openapi: " + version + "\n" +
				"info:\n" +
				"  title: Demo\n" +
				"  version: '1'\n" +
				"paths: {}\n"
			if err := os.WriteFile(path, []byte(spec), 0o600); err != nil {
				t.Fatal(err)
			}

			parsed, err := loadOpenAPIDocument(
				t.Context(),
				newSourceLoader(),
				SourceConfig{Kind: SourceFile, Path: path},
			)
			if err != nil {
				t.Fatalf("loadOpenAPIDocument() error = %v", err)
			}
			if parsed.sourceVersion != version {
				t.Fatalf("source version = %q, want %q", parsed.sourceVersion, version)
			}
			if parsed.document.OpenAPIMajorMinor() == "" {
				t.Fatalf("normalized parser version = %q", parsed.document.OpenAPI)
			}
			if parsed.location == nil || parsed.location.Scheme != "file" {
				t.Fatalf("document location = %v", parsed.location)
			}
		})
	}
}

func TestLoadOpenAPIDocumentResolvesInternalRefs(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "openapi.yaml")
	spec := `openapi: 3.1.2
info:
  title: Demo
  version: "1"
paths:
  /users:
    post:
      operationId: createUser
      requestBody:
        required: true
        content:
          application/json:
            schema:
              $ref: '#/components/schemas/User'
      responses:
        "200":
          description: ok
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/User'
components:
  schemas:
    User:
      type: object
      required: [name]
      properties:
        name:
          type: string
`
	if err := os.WriteFile(path, []byte(spec), 0o600); err != nil {
		t.Fatal(err)
	}

	parsed, err := loadOpenAPIDocument(
		t.Context(),
		newSourceLoader(),
		SourceConfig{Kind: SourceFile, Path: path},
	)
	if err != nil {
		t.Fatalf("loadOpenAPIDocument() error = %v", err)
	}
	if parsed.document.Components == nil ||
		parsed.document.Components.Schemas["User"] == nil ||
		parsed.document.Components.Schemas["User"].Value == nil {
		t.Fatal("internal schema reference was not resolved")
	}
}

func TestLoadOpenAPIDocumentRejectsExternalFileRef(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	mainPath := filepath.Join(dir, "openapi.yaml")
	externalPath := filepath.Join(dir, "schemas.yaml")
	if err := os.WriteFile(
		externalPath,
		[]byte("User:\n  type: object\n"),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	spec := `openapi: 3.1.2
info:
  title: Demo
  version: "1"
paths:
  /users:
    get:
      operationId: listUsers
      responses:
        "200":
          description: ok
          content:
            application/json:
              schema:
                $ref: './schemas.yaml#/User'
`
	if err := os.WriteFile(mainPath, []byte(spec), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := loadOpenAPIDocument(
		t.Context(),
		newSourceLoader(),
		SourceConfig{Kind: SourceFile, Path: mainPath},
	)
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "external") &&
		!strings.Contains(strings.ToLower(err.Error()), "disallowed") {
		t.Fatalf("loadOpenAPIDocument() error = %v, want external-ref rejection", err)
	}
}

func TestLoadOpenAPIDocumentDoesNotFetchExternalURLRef(t *testing.T) {
	t.Parallel()

	var externalCalls atomic.Int32
	var serverURL string
	mux := http.NewServeMux()
	mux.HandleFunc("/openapi.yaml", func(w http.ResponseWriter, _ *http.Request) {
		spec := `openapi: 3.1.2
info:
  title: Demo
  version: "1"
paths:
  /users:
    get:
      operationId: listUsers
      responses:
        "200":
          description: ok
          content:
            application/json:
              schema:
                $ref: '` + serverURL + `/schema.yaml#/User'
`
		_, _ = w.Write([]byte(spec))
	})
	mux.HandleFunc("/schema.yaml", func(w http.ResponseWriter, _ *http.Request) {
		externalCalls.Add(1)
		_, _ = w.Write([]byte("User:\n  type: object\n"))
	})
	server := httptest.NewServer(mux)
	serverURL = server.URL
	t.Cleanup(server.Close)

	_, err := loadOpenAPIDocument(
		t.Context(),
		newSourceLoader(),
		SourceConfig{Kind: SourceURL, URL: server.URL + "/openapi.yaml"},
	)
	if err == nil {
		t.Fatal("loadOpenAPIDocument() error = nil, want external-ref rejection")
	}
	if got := externalCalls.Load(); got != 0 {
		t.Fatalf("external ref HTTP calls = %d, want 0", got)
	}
}

func TestBoundedDocumentDiagnostic(t *testing.T) {
	t.Parallel()

	value := strings.Repeat("x", maxDocumentDiagnosticRunes+100)
	got := boundedDocumentDiagnostic(value)
	if len([]rune(got)) != maxDocumentDiagnosticRunes+1 {
		t.Fatalf("bounded diagnostic runes = %d", len([]rune(got)))
	}
	if !strings.HasSuffix(got, "…") {
		t.Fatalf("bounded diagnostic = %q", got)
	}
}
