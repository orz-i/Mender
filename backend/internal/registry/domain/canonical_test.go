package domain

import (
	"testing"
)

func TestCanonicalJSONKeysSortedLexicographicallyUTF16(t *testing.T) {
	t.Parallel()

	input := map[string]any{
		"z": 1,
		"a": 2,
		"m": 3,
	}
	out, err := CanonicalJSON(input)
	if err != nil {
		t.Fatalf("CanonicalJSON() error = %v", err)
	}
	want := `{"a":2,"m":3,"z":1}`
	if string(out) != want {
		t.Fatalf("got %q, want %q", string(out), want)
	}
}

func TestCanonicalJSONWhitespaceOmitted(t *testing.T) {
	t.Parallel()

	raw := []byte(`{
		"pluginId": "dev.example.echo",
		"version": "1.0.0",
		"items": [ 1,  2, 3 ]
	}`)
	out, err := CanonicalJSON(raw)
	if err != nil {
		t.Fatalf("CanonicalJSON() error = %v", err)
	}
	want := `{"items":[1,2,3],"pluginId":"dev.example.echo","version":"1.0.0"}`
	if string(out) != want {
		t.Fatalf("got %q, want %q", string(out), want)
	}
}

func TestCanonicalJSONStringEscapes(t *testing.T) {
	t.Parallel()

	input := map[string]string{
		"text": "hello\nworld\t\"quote\"\\backslash",
	}
	out, err := CanonicalJSON(input)
	if err != nil {
		t.Fatalf("CanonicalJSON() error = %v", err)
	}
	want := `{"text":"hello\nworld\t\"quote\"\\backslash"}`
	if string(out) != want {
		t.Fatalf("got %q, want %q", string(out), want)
	}
}

func TestCanonicalJSONNoForwardSlashEscape(t *testing.T) {
	t.Parallel()

	input := map[string]string{
		"url": "https://mender.io/plugins/echo.tar.gz",
	}
	out, err := CanonicalJSON(input)
	if err != nil {
		t.Fatalf("CanonicalJSON() error = %v", err)
	}
	want := `{"url":"https://mender.io/plugins/echo.tar.gz"}`
	if string(out) != want {
		t.Fatalf("got %q, want %q", string(out), want)
	}
}

func TestCanonicalJSONDeterministicForStruct(t *testing.T) {
	t.Parallel()

	type Sample struct {
		B string `json:"b"`
		A int    `json:"a"`
	}
	s := Sample{B: "hello", A: 42}
	out1, err := CanonicalJSON(s)
	if err != nil {
		t.Fatal(err)
	}
	out2, err := CanonicalJSON(s)
	if err != nil {
		t.Fatal(err)
	}
	if string(out1) != string(out2) {
		t.Fatalf("CanonicalJSON not deterministic: %q vs %q", string(out1), string(out2))
	}
	want := `{"a":42,"b":"hello"}`
	if string(out1) != want {
		t.Fatalf("got %q, want %q", string(out1), want)
	}
}
