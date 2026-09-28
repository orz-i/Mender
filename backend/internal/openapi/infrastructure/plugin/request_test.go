package plugin

import (
	"io"
	"strings"
	"testing"
)

func TestBuildHTTPRequestSerializesParametersAndJSONBody(t *testing.T) {
	t.Parallel()

	binding := operationBinding{
		method:    "POST",
		path:      "/users/{id}",
		serverURL: "https://api.example.test/v1",
		parameters: []parameterBinding{
			{Name: "id", In: "path", Style: "simple", Required: true},
			{Name: "tag", In: "query", Style: "form", Explode: true},
			{Name: "fields", In: "query", Style: "form", Explode: false},
			{Name: "X-Trace", In: "header", Style: "simple", Required: true},
			{Name: "session", In: "cookie", Style: "form"},
		},
		requestBody: &requestBodyBinding{
			MediaType: "application/json",
			Required:  true,
		},
	}

	request, err := buildHTTPRequest(
		t.Context(),
		binding,
		[]byte(`{
			"path":{"id":"a/b"},
			"query":{"tag":["red","blue"],"fields":["name","email"]},
			"headers":{"X-Trace":"trace-1"},
			"cookies":{"session":"s-1"},
			"body":{"name":"Alice"}
		}`),
	)
	if err != nil {
		t.Fatalf("buildHTTPRequest() error = %v", err)
	}

	if request.Method != "POST" {
		t.Fatalf("method = %q", request.Method)
	}
	if request.URL.String() != "https://api.example.test/v1/users/a%2Fb?fields=name%2Cemail&tag=blue&tag=red" &&
		request.URL.String() != "https://api.example.test/v1/users/a%2Fb?fields=name%2Cemail&tag=red&tag=blue" {
		t.Fatalf("URL = %q", request.URL.String())
	}
	if request.Header.Get("X-Trace") != "trace-1" {
		t.Fatalf("X-Trace = %q", request.Header.Get("X-Trace"))
	}
	if request.Header.Get("Content-Type") != "application/json" {
		t.Fatalf("Content-Type = %q", request.Header.Get("Content-Type"))
	}
	if !strings.Contains(request.Header.Get("Cookie"), "session=s-1") {
		t.Fatalf("Cookie = %q", request.Header.Get("Cookie"))
	}
	rawBody, err := io.ReadAll(request.Body)
	if err != nil {
		t.Fatal(err)
	}
	if string(rawBody) != `{"name":"Alice"}` {
		t.Fatalf("body = %s", rawBody)
	}
}

func TestBuildHTTPRequestRejectsMissingRequiredParameter(t *testing.T) {
	t.Parallel()

	_, err := buildHTTPRequest(
		t.Context(),
		operationBinding{
			method:    "GET",
			path:      "/users/{id}",
			serverURL: "https://api.example.test",
			parameters: []parameterBinding{
				{Name: "id", In: "path", Style: "simple", Required: true},
			},
		},
		[]byte(`{}`),
	)
	if err == nil || !strings.Contains(err.Error(), "required value is missing") {
		t.Fatalf("buildHTTPRequest() error = %v", err)
	}
}

func TestBuildHTTPRequestRejectsHeaderInjection(t *testing.T) {
	t.Parallel()

	_, err := buildHTTPRequest(
		t.Context(),
		operationBinding{
			method:    "GET",
			path:      "/users",
			serverURL: "https://api.example.test",
			parameters: []parameterBinding{
				{Name: "X-Trace", In: "header", Style: "simple"},
			},
		},
		[]byte("{\"headers\":{\"X-Trace\":\"safe\\r\\nInjected: yes\"}}"),
	)
	if err == nil || !strings.Contains(err.Error(), "invalid header value") {
		t.Fatalf("buildHTTPRequest() error = %v", err)
	}
}

func TestBuildHTTPRequestOptionalBodyCanBeOmitted(t *testing.T) {
	t.Parallel()

	request, err := buildHTTPRequest(
		t.Context(),
		operationBinding{
			method:    "POST",
			path:      "/events",
			serverURL: "https://api.example.test",
			requestBody: &requestBodyBinding{
				MediaType: "application/json",
				Required:  false,
			},
		},
		[]byte(`{}`),
	)
	if err != nil {
		t.Fatal(err)
	}
	if request.Body != nil {
		t.Fatal("request body is present for omitted optional body")
	}
	if request.Header.Get("Content-Type") != "" {
		t.Fatalf("Content-Type = %q", request.Header.Get("Content-Type"))
	}
}

func TestSerializeQueryArrayExplodeModes(t *testing.T) {
	t.Parallel()

	exploded, err := buildHTTPRequest(
		t.Context(),
		operationBinding{
			method:    "GET",
			path:      "/search",
			serverURL: "https://api.example.test",
			parameters: []parameterBinding{
				{Name: "tag", In: "query", Style: "form", Explode: true},
			},
		},
		[]byte(`{"query":{"tag":["a","b"]}}`),
	)
	if err != nil {
		t.Fatal(err)
	}
	if values := exploded.URL.Query()["tag"]; len(values) != 2 {
		t.Fatalf("exploded tag values = %v", values)
	}

	joined, err := buildHTTPRequest(
		t.Context(),
		operationBinding{
			method:    "GET",
			path:      "/search",
			serverURL: "https://api.example.test",
			parameters: []parameterBinding{
				{Name: "tag", In: "query", Style: "form", Explode: false},
			},
		},
		[]byte(`{"query":{"tag":["a","b"]}}`),
	)
	if err != nil {
		t.Fatal(err)
	}
	if got := joined.URL.Query().Get("tag"); got != "a,b" {
		t.Fatalf("joined tag = %q", got)
	}
}
