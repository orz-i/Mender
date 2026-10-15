package bootstrap

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	catalogapplication "github.com/orz-i/Mender/backend/internal/catalog/application"
	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
	profileapplication "github.com/orz-i/Mender/backend/internal/profile/application"
	profiledomain "github.com/orz-i/Mender/backend/internal/profile/domain"
)

func TestDeleteConfiguredProviderRejectsProfileReference(t *testing.T) {
	t.Parallel()

	runtime, err := NewRuntime(context.Background(), RuntimeOptions{
		DatabasePath: filepath.Join(t.TempDir(), "mender.db"),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runtime.Close() })

	provider, err := runtime.Catalog.RegisterProvider.Execute(
		context.Background(),
		catalogapplication.RegisterProviderRequest{
			Namespace:   "owned",
			DisplayName: "Owned",
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	schema, _ := catalogdomain.NewSchema([]byte(`{"type":"object"}`))
	tool, err := catalogdomain.NewTool(
		mustBootstrapToolID(t, "tool_00000000000000000000000000000011"),
		provider.ID(),
		catalogdomain.ToolSpec{
			SourceKey:   "echo",
			Name:        "echo",
			InputSchema: schema,
		},
		systemClock{}.Now(),
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := runtime.Catalog.store.ReplaceProviderTools(
		context.Background(),
		provider.ID(),
		[]catalogdomain.Tool{tool},
	); err != nil {
		t.Fatal(err)
	}

	profile, err := runtime.Profiles.Create.Execute(
		context.Background(),
		profileapplication.CreateProfileRequest{
			Slug:        "default",
			DisplayName: "Default",
			Mode:        profiledomain.ExposureHybrid,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	_, err = runtime.Profiles.AddTool.Execute(
		context.Background(),
		profileapplication.AddToolRequest{
			ProfileSlug: profile.Slug(),
			ToolID:      tool.ID(),
			Direct:      true,
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	err = runtime.DeleteConfiguredProvider(context.Background(), provider.ID())
	if !errors.Is(err, ErrProviderInUse) {
		t.Fatalf("DeleteConfiguredProvider() error = %v, want ErrProviderInUse", err)
	}
}

func mustBootstrapToolID(t *testing.T, raw string) catalogdomain.ToolID {
	t.Helper()
	id, err := catalogdomain.ParseToolID(raw)
	if err != nil {
		t.Fatal(err)
	}
	return id
}
