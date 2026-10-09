package binding

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/orz-i/Mender/backend/internal/bootstrap"
)

func TestDesktopSandboxActivityReadOnlyAndBounded(t *testing.T) {
	ctx := context.Background()
	runtime, err := bootstrap.NewRuntime(ctx, bootstrap.RuntimeOptions{DatabasePath: filepath.Join(t.TempDir(), "desktop-activity.db")})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = runtime.Close() }()
	service := NewService(ctx, runtime)
	item, err := service.CreateLocalSandbox(CreateLocalSandboxInput{Name: "Desktop activity", Root: t.TempDir(), Isolation: "host"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.StopSandbox(item.SandboxID); err != nil {
		t.Fatal(err)
	}
	if _, err := service.StartSandbox(item.SandboxID); err != nil {
		t.Fatal(err)
	}
	if _, err := service.DestroySandbox(item.SandboxID); err != nil {
		t.Fatal(err)
	}
	activity, err := service.GetSandboxActivity(item.SandboxID, 2)
	if err != nil {
		t.Fatal(err)
	}
	if activity.Limit != 2 || activity.SandboxID != item.SandboxID || len(activity.Events) != 2 {
		t.Fatalf("bounded desktop activity %+v", activity)
	}
	for _, event := range activity.Events {
		if _, err := time.Parse(time.RFC3339Nano, event.OccurredAt); err != nil {
			t.Fatalf("invalid UTC timestamp: %s", event.OccurredAt)
		}
		if event.Action == "" || event.Kind == "" || event.Origin == "" {
			t.Fatalf("incomplete activity %+v", event)
		}
	}
	if _, err := service.GetSandboxActivity(item.SandboxID, 101); err == nil {
		t.Fatal("unbounded desktop Activity accepted")
	}
	if _, err := service.GetSandboxActivity("sbx_invalid", 25); err == nil {
		t.Fatal("invalid Sandbox ID accepted")
	}
}
