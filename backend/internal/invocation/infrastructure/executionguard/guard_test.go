package executionguard

import (
	"context"
	"testing"
	"time"

	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
	invocationapplication "github.com/orz-i/Mender/backend/internal/invocation/application"
)

func TestProviderSerialBlocksSameProviderButNotDifferentProvider(t *testing.T) {
	t.Parallel()

	guard := &ProviderSerial{}
	first := guardRequest(t, 1, 1)
	sameProvider := guardRequest(t, 2, 1)
	otherProvider := guardRequest(t, 3, 2)

	releaseFirst, err := guard.Acquire(context.Background(), first)
	if err != nil {
		t.Fatal(err)
	}
	defer releaseFirst()

	blockedCtx, cancelBlocked := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancelBlocked()
	if _, err := guard.Acquire(blockedCtx, sameProvider); err == nil {
		t.Fatal("same Provider acquisition unexpectedly succeeded while first was held")
	}

	releaseOther, err := guard.Acquire(context.Background(), otherProvider)
	if err != nil {
		t.Fatalf("different Provider acquisition error = %v", err)
	}
	releaseOther()
}

func TestToolSerialBlocksSameToolButNotDifferentTool(t *testing.T) {
	t.Parallel()

	guard := &ToolSerial{}
	first := guardRequest(t, 1, 1)
	sameTool := guardRequest(t, 1, 1)
	otherTool := guardRequest(t, 2, 1)

	releaseFirst, err := guard.Acquire(context.Background(), first)
	if err != nil {
		t.Fatal(err)
	}
	defer releaseFirst()

	blockedCtx, cancelBlocked := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancelBlocked()
	if _, err := guard.Acquire(blockedCtx, sameTool); err == nil {
		t.Fatal("same Tool acquisition unexpectedly succeeded while first was held")
	}

	releaseOther, err := guard.Acquire(context.Background(), otherTool)
	if err != nil {
		t.Fatalf("different Tool acquisition error = %v", err)
	}
	releaseOther()
}

func TestSerialGuardReleasesAfterCriticalSection(t *testing.T) {
	t.Parallel()

	guard := &ToolSerial{}
	request := guardRequest(t, 1, 1)

	release, err := guard.Acquire(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	release()

	releaseAgain, err := guard.Acquire(context.Background(), request)
	if err != nil {
		t.Fatalf("Acquire() after release error = %v", err)
	}
	releaseAgain()
}

func TestUnlimitedHonorsCancelledContext(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := (Unlimited{}).Acquire(ctx, guardRequest(t, 1, 1)); err == nil {
		t.Fatal("Unlimited.Acquire() accepted cancelled context")
	}
}

func guardRequest(t *testing.T, toolNumber, providerNumber int) invocationapplication.ExecutionGuardRequest {
	t.Helper()

	toolID, err := catalogdomain.ParseToolID(id("tool_", toolNumber))
	if err != nil {
		t.Fatal(err)
	}
	providerID, err := catalogdomain.ParseProviderID(id("prov_", providerNumber))
	if err != nil {
		t.Fatal(err)
	}
	schema, err := catalogdomain.NewSchema([]byte("{\"type\":\"object\"}"))
	if err != nil {
		t.Fatal(err)
	}
	tool, err := catalogdomain.NewTool(
		toolID,
		providerID,
		catalogdomain.ToolSpec{
			SourceKey:   "tool",
			Name:        "tool",
			InputSchema: schema,
		},
		time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatal(err)
	}

	return invocationapplication.ExecutionGuardRequest{Tool: tool}
}

func id(prefix string, number int) string {
	const hex = "0123456789abcdef"
	value := make([]byte, 32)
	for i := range value {
		value[i] = '0'
	}
	value[31] = hex[number%16]
	return prefix + string(value)
}
