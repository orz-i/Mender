package plugin

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"testing"

	credentialdomain "github.com/orz-i/Mender/backend/internal/credential/domain"
)

func TestSelectOperationSecurityAnonymousAndBearer(t *testing.T) {
	t.Parallel()

	parsed := loadSchemaFixture(t, `openapi: 3.1.2
info:
  title: Demo
  version: "1"
components:
  securitySchemes:
    BearerAuth:
      type: http
      scheme: bearer
paths:
  /public:
    get:
      operationId: public
      security: []
      responses:
        "200":
          description: ok
  /private:
    get:
      operationId: private
      security:
        - BearerAuth: []
      responses:
        "200":
          description: ok
`)

	public := parsed.document.Paths.Value("/public").Get
	bindings, err := selectOperationSecurity(parsed.document, public, nil)
	if err != nil {
		t.Fatalf("public security error = %v", err)
	}
	if len(bindings) != 0 {
		t.Fatalf("public bindings = %+v", bindings)
	}

	ref := mustCredentialRef(t, "demo", "token")
	private := parsed.document.Paths.Value("/private").Get
	bindings, err = selectOperationSecurity(
		parsed.document,
		private,
		map[string]credentialdomain.Reference{"BearerAuth": ref},
	)
	if err != nil {
		t.Fatalf("private security error = %v", err)
	}
	if len(bindings) != 1 ||
		bindings[0].Kind != securityBearer ||
		bindings[0].Reference != ref {
		t.Fatalf("private bindings = %+v", bindings)
	}
}

func TestSelectOperationSecurityPreservesANDAndOR(t *testing.T) {
	t.Parallel()

	parsed := loadSchemaFixture(t, `openapi: 3.1.2
info:
  title: Demo
  version: "1"
components:
  securitySchemes:
    QueryKey:
      type: apiKey
      in: query
      name: api_key
    HeaderKey:
      type: apiKey
      in: header
      name: X-API-Key
    OAuth:
      type: oauth2
      flows:
        clientCredentials:
          tokenUrl: https://auth.example.test/token
          scopes: {}
paths:
  /data:
    get:
      operationId: data
      security:
        - OAuth: []
        - QueryKey: []
          HeaderKey: []
      responses:
        "200":
          description: ok
`)

	queryRef := mustCredentialRef(t, "demo", "query")
	headerRef := mustCredentialRef(t, "demo", "header")
	operation := parsed.document.Paths.Value("/data").Get

	_, err := selectOperationSecurity(
		parsed.document,
		operation,
		map[string]credentialdomain.Reference{"QueryKey": queryRef},
	)
	if err == nil || !strings.Contains(err.Error(), "HeaderKey") {
		t.Fatalf("partial AND error = %v", err)
	}

	bindings, err := selectOperationSecurity(
		parsed.document,
		operation,
		map[string]credentialdomain.Reference{
			"QueryKey":  queryRef,
			"HeaderKey": headerRef,
		},
	)
	if err != nil {
		t.Fatalf("OR fallback error = %v", err)
	}
	if len(bindings) != 2 {
		t.Fatalf("bindings = %+v", bindings)
	}
	if bindings[0].SchemeName != "HeaderKey" ||
		bindings[1].SchemeName != "QueryKey" {
		t.Fatalf("deterministic AND order = %+v", bindings)
	}
}

func TestApplyOperationSecurityLocations(t *testing.T) {
	t.Parallel()

	headerRef := mustCredentialRef(t, "demo", "header")
	queryRef := mustCredentialRef(t, "demo", "query")
	cookieRef := mustCredentialRef(t, "demo", "cookie")
	bearerRef := mustCredentialRef(t, "demo", "bearer")

	resolver := &testCredentialResolver{
		values: map[credentialdomain.Reference][]byte{
			headerRef: []byte("header-secret"),
			queryRef:  []byte("query-secret"),
			cookieRef: []byte("cookie-secret"),
			bearerRef: []byte("bearer-secret"),
		},
	}

	request := &http.Request{
		Method: http.MethodGet,
		URL:    &url.URL{Scheme: "https", Host: "api.example.test", Path: "/data"},
		Header: make(http.Header),
	}
	err := applyOperationSecurity(t.Context(), request, []securityBinding{
		{
			SchemeName: "HeaderKey",
			Kind:       securityAPIKey,
			In:         "header",
			Name:       "X-Key",
			Reference:  headerRef,
		},
		{
			SchemeName: "QueryKey",
			Kind:       securityAPIKey,
			In:         "query",
			Name:       "api_key",
			Reference:  queryRef,
		},
		{
			SchemeName: "CookieKey",
			Kind:       securityAPIKey,
			In:         "cookie",
			Name:       "session_key",
			Reference:  cookieRef,
		},
		{
			SchemeName: "BearerAuth",
			Kind:       securityBearer,
			In:         "header",
			Name:       "Authorization",
			Reference:  bearerRef,
		},
	}, resolver)
	if err != nil {
		t.Fatalf("applyOperationSecurity() error = %v", err)
	}

	if got := request.Header.Get("X-Key"); got != "header-secret" {
		t.Fatalf("X-Key = %q", got)
	}
	if got := request.Header.Get("Authorization"); got != "Bearer bearer-secret" {
		t.Fatalf("Authorization = %q", got)
	}
	if got := request.URL.Query().Get("api_key"); got != "query-secret" {
		t.Fatalf("api_key = %q", got)
	}
	if got := request.Header.Get("Cookie"); !strings.Contains(got, "session_key=cookie-secret") {
		t.Fatalf("Cookie = %q", got)
	}
	if len(resolver.calls) != 4 {
		t.Fatalf("resolver calls = %v", resolver.calls)
	}
}

func TestApplyOperationSecurityPropagatesScopedCredentialError(t *testing.T) {
	t.Parallel()

	ref := mustCredentialRef(t, "demo", "blocked")
	sentinel := errors.New("credential outside scope")
	resolver := &testCredentialResolver{
		errs: map[credentialdomain.Reference]error{ref: sentinel},
	}
	request, err := http.NewRequestWithContext(
		context.Background(),
		http.MethodGet,
		"https://api.example.test",
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}

	err = applyOperationSecurity(context.Background(), request, []securityBinding{
		{
			SchemeName: "BearerAuth",
			Kind:       securityBearer,
			Reference:  ref,
		},
	}, resolver)
	if !errors.Is(err, sentinel) {
		t.Fatalf("applyOperationSecurity() error = %v, want sentinel", err)
	}
}

func TestApplyOperationSecurityRejectsControlCharacters(t *testing.T) {
	t.Parallel()

	ref := mustCredentialRef(t, "demo", "bad")
	resolver := &testCredentialResolver{
		values: map[credentialdomain.Reference][]byte{
			ref: {'a', 13, 10, 'b'},
		},
	}
	request, _ := http.NewRequest(http.MethodGet, "https://api.example.test", nil)
	err := applyOperationSecurity(t.Context(), request, []securityBinding{
		{
			SchemeName: "HeaderKey",
			Kind:       securityAPIKey,
			In:         "header",
			Name:       "X-Key",
			Reference:  ref,
		},
	}, resolver)
	if err == nil || !strings.Contains(err.Error(), "control characters") {
		t.Fatalf("applyOperationSecurity() error = %v", err)
	}
}

func mustCredentialRef(
	t *testing.T,
	namespace string,
	name string,
) credentialdomain.Reference {
	t.Helper()
	ref, err := credentialdomain.NewReference(namespace, name)
	if err != nil {
		t.Fatal(err)
	}
	return ref
}

type testCredentialResolver struct {
	values map[credentialdomain.Reference][]byte
	errs   map[credentialdomain.Reference]error
	calls  []credentialdomain.Reference
}

func (r *testCredentialResolver) Resolve(
	_ context.Context,
	ref credentialdomain.Reference,
) ([]byte, error) {
	r.calls = append(r.calls, ref)
	if err := r.errs[ref]; err != nil {
		return nil, err
	}
	return append([]byte(nil), r.values[ref]...), nil
}
