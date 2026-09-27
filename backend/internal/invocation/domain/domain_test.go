package domain

import (
	"bytes"
	"testing"
)

func TestInvocationIDValidation(t *testing.T) {
	t.Parallel()

	valid := "inv_0123456789abcdef0123456789abcdef"
	id, err := ParseInvocationID(valid)
	if err != nil {
		t.Fatalf("ParseInvocationID() error = %v", err)
	}
	if id.String() != valid {
		t.Fatalf("InvocationID = %q", id)
	}

	for _, value := range []string{"", "tool_0123456789abcdef0123456789abcdef", "inv_1234", "inv_0123456789ABCDEF0123456789ABCDEF"} {
		if _, err := ParseInvocationID(value); err == nil {
			t.Errorf("ParseInvocationID(%q) expected error", value)
		}
	}
}

func TestJSONValueIsImmutableAndAcceptsAnyJSONRoot(t *testing.T) {
	t.Parallel()

	for _, raw := range [][]byte{
		[]byte(`{"name":"mender"}`),
		[]byte(`["a","b"]`),
		[]byte(`"value"`),
		[]byte(`42`),
		[]byte(`true`),
		[]byte(`null`),
	} {
		if _, err := NewJSONValue(raw); err != nil {
			t.Errorf("NewJSONValue(%s) error = %v", raw, err)
		}
	}

	raw := []byte(`{"secret":"unchanged"}`)
	value, err := NewJSONValue(raw)
	if err != nil {
		t.Fatal(err)
	}
	raw[0] = '['
	exposed := value.Bytes()
	exposed[0] = '['
	if got := value.Bytes(); !bytes.Equal(got, []byte(`{"secret":"unchanged"}`)) {
		t.Fatalf("JSONValue mutated: %s", got)
	}
}

func TestResultVariants(t *testing.T) {
	t.Parallel()

	content, _ := NewJSONValue([]byte(`{"ok":true}`))
	complete, err := Complete(content)
	if err != nil {
		t.Fatal(err)
	}
	if complete.Kind() != ResultKindComplete {
		t.Fatalf("complete kind = %q", complete.Kind())
	}

	input, err := InputRequired(
		"req-1",
		"Choose a repository",
		[]byte(`{"type":"string"}`),
	)
	if err != nil {
		t.Fatal(err)
	}
	if input.Kind() != ResultKindInputRequired || input.RequestID() != "req-1" {
		t.Fatalf("input required = %#v", input)
	}

	task, err := Task("task-1", "running", "queued remotely")
	if err != nil {
		t.Fatal(err)
	}
	if task.Kind() != ResultKindTask || task.TaskID() != "task-1" {
		t.Fatalf("task = %#v", task)
	}

	rejected, err := Rejected(RejectionPolicyDenied, "denied by policy")
	if err != nil {
		t.Fatal(err)
	}
	if rejected.Kind() != OutcomeKindRejected || rejected.RejectionCode() != RejectionPolicyDenied {
		t.Fatalf("rejected = %#v", rejected)
	}

	failed, err := Failed(FailureSourceProvider, FailureProviderError, "provider execution failed")
	if err != nil {
		t.Fatal(err)
	}
	if failed.Kind() != OutcomeKindFailed ||
		failed.FailureSource() != FailureSourceProvider ||
		failed.FailureCode() != FailureProviderError {
		t.Fatalf("failed = %#v", failed)
	}
}

func TestOutcomeRejectsInvalidFailureCombinations(t *testing.T) {
	t.Parallel()

	if _, err := Rejected(RejectionCode("unknown"), "no"); err == nil {
		t.Fatal("Rejected() accepted unknown code")
	}
	if _, err := Failed(FailureSourceTool, FailureProviderError, "wrong source"); err == nil {
		t.Fatal("Failed() accepted mismatched source/code")
	}
	if _, err := Failed(FailureSourceProvider, FailureProviderError, ""); err == nil {
		t.Fatal("Failed() accepted empty message")
	}
}
