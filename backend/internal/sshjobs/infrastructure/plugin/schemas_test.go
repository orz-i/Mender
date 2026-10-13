package plugin

import (
	"context"
	"encoding/json"
	"testing"

	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
	invocationdomain "github.com/orz-i/Mender/backend/internal/invocation/domain"
	invschema "github.com/orz-i/Mender/backend/internal/invocation/infrastructure/jsonschema"
)

func TestSSHJobsJSONSchemasValidateRealToolArguments(t *testing.T) {
	for _, tc := range []struct {
		key    string
		schema []byte
		input  string
	}{
		{SourceSubmit, submitInputSchema, `{"idempotencyKey":"eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee","command":"echo hello"}`},
		{SourceStatus, taskInputSchema, `{"taskId":"sjob_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}`},
		{SourceResult, taskInputSchema, `{"taskId":"sjob_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}`},
		{SourceCancel, taskInputSchema, `{"taskId":"sjob_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}`},
		{SourceLogs, logInputSchema, `{"taskId":"sjob_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","stream":"stdout","cursor":0,"limit":1024}`},
	} {
		if !json.Valid(tc.schema) {
			t.Fatalf("%s invalid JSON Schema: %s", tc.key, tc.schema)
		}
		schema, err := catalogdomain.NewSchema(tc.schema)
		if err != nil {
			t.Fatal(err)
		}
		arg, err := invocationdomain.NewJSONValue([]byte(tc.input))
		if err != nil {
			t.Fatal(err)
		}
		valid, err := (invschema.Validator{}).Validate(context.Background(), schema, arg)
		if err != nil || !valid {
			t.Fatalf("%s input schema rejected example: valid=%t err=%v schema=%s", tc.key, valid, err, tc.schema)
		}
	}
}
