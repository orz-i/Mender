package lifecycle

import (
	"errors"
	"testing"
)

func TestCloserInvokesRuntimeCloseOnlyOnce(t *testing.T) {
	wantErr := errors.New("close failed")
	calls := 0
	closer := NewCloser(func() error {
		calls++
		return wantErr
	})

	if err := closer.Close(); !errors.Is(err, wantErr) {
		t.Fatalf("first Close() error = %v, want %v", err, wantErr)
	}
	if err := closer.Close(); !errors.Is(err, wantErr) {
		t.Fatalf("second Close() error = %v, want %v", err, wantErr)
	}
	if calls != 1 {
		t.Fatalf("close calls = %d, want 1", calls)
	}
}
