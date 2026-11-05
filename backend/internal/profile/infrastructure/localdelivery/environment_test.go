package localdelivery

import (
	"context"
	"path/filepath"
	"runtime"
	"testing"
)

func TestEnvironmentResolvesOnlyTheConfiguredLocalDatabase(t *testing.T) {
	selected := filepath.Join("profile-delivery-fixture", "runtime.db")
	want, err := filepath.Abs(selected)
	if err != nil {
		t.Fatal(err)
	}
	host, err := (Environment{DatabasePath: selected}).LocalContext(context.Background())
	if err != nil || host.DatabasePath != want || host.Platform != runtime.GOOS {
		t.Fatalf("host context changed: %+v %v", host, err)
	}
	if _, err := (Environment{}).LocalContext(context.Background()); err == nil {
		t.Fatal("missing database silently fell back to the current directory")
	}
}
