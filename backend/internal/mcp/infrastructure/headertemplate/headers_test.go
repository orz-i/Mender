package headertemplate

import (
	"strings"
	"testing"
)

func TestValidateSensitiveHeadersRequireEnvironmentReference(t *testing.T) {
	t.Parallel()

	if err := Validate(map[string]string{
		"Authorization": "Bearer ${ANYSEARCH_API_KEY}",
		"X-Client":      "mcp/1.0.0",
	}); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}

	if err := Validate(map[string]string{
		"Authorization": "Bearer plaintext-secret",
	}); err == nil {
		t.Fatal("Validate() expected plaintext Authorization rejection")
	}
	if err := Validate(map[string]string{
		"Authorization": "Bearer plaintext-secret ${DUMMY}",
	}); err == nil {
		t.Fatal("Validate() expected mixed plaintext/environment Authorization rejection")
	}
	if err := Validate(map[string]string{
		"X-Access-Token": "plaintext-secret",
	}); err == nil {
		t.Fatal("Validate() expected custom token header rejection")
	}
}

func TestResolveSupportsCommonEnvironmentReferenceSyntax(t *testing.T) {
	t.Parallel()

	headers, err := Resolve(
		map[string]string{
			"Authorization": "Bearer ${ANYSEARCH_API_KEY}",
			"X-One":         "${env:ONE}",
			"X-Two":         "{env:TWO}",
		},
		func(name string) (string, bool) {
			values := map[string]string{
				"ANYSEARCH_API_KEY": "as_sk_test",
				"ONE":               "one",
				"TWO":               "two",
			}
			value, ok := values[name]
			return value, ok
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if headers["Authorization"] != "Bearer as_sk_test" ||
		headers["X-One"] != "one" ||
		headers["X-Two"] != "two" {
		t.Fatalf("resolved headers = %#v", headers)
	}
}

func TestResolveReportsMissingEnvironmentWithoutSecretMaterial(t *testing.T) {
	t.Parallel()

	_, err := Resolve(
		map[string]string{"Authorization": "Bearer ${MISSING_TOKEN}"},
		func(string) (string, bool) { return "", false },
	)
	if err == nil || !strings.Contains(err.Error(), "MISSING_TOKEN") {
		t.Fatalf("Resolve() error = %v", err)
	}
}

func TestValidateRejectsTransportControlledHeader(t *testing.T) {
	t.Parallel()

	if err := Validate(map[string]string{"Host": "example.test"}); err == nil {
		t.Fatal("Validate() expected Host rejection")
	}
}
