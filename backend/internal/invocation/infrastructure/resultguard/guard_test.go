package resultguard

import (
	"context"
	"testing"
	"time"

	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
	invocationdomain "github.com/orz-i/Mender/backend/internal/invocation/domain"
	invocationjsonschema "github.com/orz-i/Mender/backend/internal/invocation/infrastructure/jsonschema"
)

func TestGuardValidatesDeclaredOutputSchema(t *testing.T) {
	t.Parallel()

	guard := New(invocationjsonschema.Validator{}, DefaultMaxResultBytes)
	tool := resultTool(t, []byte(`{"type":"object","properties":{"ok":{"type":"boolean"}},"required":["ok"]}`))

	valid := completeOutcome(t, `{"ok":true}`)
	got, err := guard.Guard(context.Background(), tool, valid)
	if err != nil {
		t.Fatalf("Guard(valid) error = %v", err)
	}
	if got.Kind() != invocationdomain.OutcomeKindComplete {
		t.Fatalf("Guard(valid) kind = %q", got.Kind())
	}

	invalid := completeOutcome(t, `{"ok":"yes"}`)
	got, err = guard.Guard(context.Background(), tool, invalid)
	if err != nil {
		t.Fatalf("Guard(invalid) error = %v", err)
	}
	if got.Kind() != invocationdomain.OutcomeKindFailed ||
		got.FailureSource() != invocationdomain.FailureSourceResultGuard ||
		got.FailureCode() != invocationdomain.FailureOutputSchemaMismatch {
		t.Fatalf(
			"Guard(invalid) = kind %q source %q code %q",
			got.Kind(),
			got.FailureSource(),
			got.FailureCode(),
		)
	}
}

func TestGuardRejectsOversizedCanonicalResultWithoutTruncation(t *testing.T) {
	t.Parallel()

	guard := New(invocationjsonschema.Validator{}, 16)
	tool := resultTool(t, nil)
	outcome := completeOutcome(t, `{"message":"this is larger than sixteen bytes"}`)

	got, err := guard.Guard(context.Background(), tool, outcome)
	if err != nil {
		t.Fatalf("Guard() error = %v", err)
	}
	if got.Kind() != invocationdomain.OutcomeKindFailed ||
		got.FailureCode() != invocationdomain.FailureResultTooLarge {
		t.Fatalf("Guard() = kind %q code %q", got.Kind(), got.FailureCode())
	}
	if !got.Content().IsZero() {
		t.Fatal("oversized result was retained/truncated as canonical content")
	}
}

func TestGuardLeavesNonCompleteOutcomesUnchanged(t *testing.T) {
	t.Parallel()

	guard := New(invocationjsonschema.Validator{}, 1)
	tool := resultTool(t, []byte(`{"type":"object"}`))
	failed, err := invocationdomain.Failed(
		invocationdomain.FailureSourceTool,
		invocationdomain.FailureToolError,
		"tool failed",
	)
	if err != nil {
		t.Fatal(err)
	}

	got, err := guard.Guard(context.Background(), tool, failed)
	if err != nil {
		t.Fatalf("Guard() error = %v", err)
	}
	if got.Kind() != invocationdomain.OutcomeKindFailed ||
		got.FailureSource() != invocationdomain.FailureSourceTool {
		t.Fatalf("Guard() changed Tool failure = %#v", got)
	}
}

func TestGuardTreatsUnevaluableOutputSchemaAsInternalFailure(t *testing.T) {
	t.Parallel()

	guard := New(invocationjsonschema.Validator{}, DefaultMaxResultBytes)
	tool := resultTool(t, []byte(`{
		"$schema":"https://json-schema.org/draft/2019-09/schema",
		"type":"object"
	}`))
	outcome := completeOutcome(t, `{}`)

	if _, err := guard.Guard(context.Background(), tool, outcome); err == nil {
		t.Fatal("Guard() expected output schema evaluation error")
	}
}

func resultTool(t *testing.T, outputRaw []byte) catalogdomain.Tool {
	t.Helper()

	toolID, err := catalogdomain.ParseToolID("tool_00000000000000000000000000000001")
	if err != nil {
		t.Fatal(err)
	}
	providerID, err := catalogdomain.ParseProviderID("prov_00000000000000000000000000000001")
	if err != nil {
		t.Fatal(err)
	}
	input, err := catalogdomain.NewSchema([]byte(`{"type":"object"}`))
	if err != nil {
		t.Fatal(err)
	}

	var output *catalogdomain.Schema
	if outputRaw != nil {
		schema, schemaErr := catalogdomain.NewSchema(outputRaw)
		if schemaErr != nil {
			t.Fatal(schemaErr)
		}
		output = &schema
	}

	tool, err := catalogdomain.NewTool(
		toolID,
		providerID,
		catalogdomain.ToolSpec{
			SourceKey:    "result-test",
			Name:         "result-test",
			InputSchema:  input,
			OutputSchema: output,
		},
		time.Date(2026, 9, 26, 0, 0, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatal(err)
	}
	return tool
}

func completeOutcome(t *testing.T, raw string) invocationdomain.Outcome {
	t.Helper()
	value, err := invocationdomain.NewJSONValue([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	outcome, err := invocationdomain.Complete(value)
	if err != nil {
		t.Fatal(err)
	}
	return outcome
}
