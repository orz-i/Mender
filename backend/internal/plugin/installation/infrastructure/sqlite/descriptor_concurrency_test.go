package sqlite

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	platformplugin "github.com/orz-i/Mender/backend/internal/plugin"
)

func TestConcurrentDescriptorPublicationCannotReplaceWinningSnapshot(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "concurrent-descriptor.db")
	first, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = first.Close() }()
	second, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = second.Close() }()
	item := testInstallation(t, "pinst_00000000000000000000000000000011", "dev.example.concurrent", "1.0.0", "d")
	if err := first.Create(ctx, item); err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	type publication struct {
		descriptor platformplugin.Descriptor
		err        error
	}
	results := make(chan publication, 2)
	for index, store := range []*Store{first, second} {
		descriptor := platformplugin.Descriptor{PluginID: item.PluginID(), PackageVersion: item.PackageVersion(),
			Extension: item.Extension(), DisplayName: []string{"First", "Second"}[index], ConfigVersion: 1}
		go func() {
			<-start
			results <- publication{descriptor: descriptor, err: store.SetVerifiedDescriptor(ctx, item.ID(), descriptor)}
		}()
	}
	close(start)
	var winner platformplugin.Descriptor
	accepted := 0
	for range 2 {
		result := <-results
		if result.err == nil {
			accepted++
			winner = result.descriptor
		} else if !strings.Contains(result.err.Error(), "immutable") {
			t.Fatalf("competing publication returned unexpected error: %v", result.err)
		}
	}
	if accepted != 1 {
		t.Fatalf("accepted %d competing snapshots, want exactly one", accepted)
	}
	current, err := first.Get(ctx, item.ID())
	if err != nil {
		t.Fatal(err)
	}
	snapshot, ok := current.VerifiedDescriptor()
	if !ok || snapshot.DisplayName != winner.DisplayName {
		t.Fatalf("winning snapshot was replaced: %+v winner=%+v", snapshot, winner)
	}
	if err := second.SetVerifiedDescriptor(ctx, item.ID(), winner); err != nil {
		t.Fatalf("identical snapshot publication is not idempotent: %v", err)
	}
}
