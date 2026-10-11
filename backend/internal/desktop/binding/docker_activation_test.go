package binding

import (
	"context"
	"errors"
	"testing"
)

func TestDockerDesktopActivationRequiresTrustedHostCallback(t *testing.T) {
	ordinary := NewService(context.Background(), nil)
	if err := ordinary.ConnectDockerDesktop(); err == nil {
		t.Fatal("regular binding unexpectedly connects Docker")
	}
	calls := 0
	denied := errors.New("rejected daemon")
	desktop := NewServiceWithDockerActivation(context.Background(), nil, func(context.Context) error {
		calls++
		return denied
	})
	if err := desktop.ConnectDockerDesktop(); !errors.Is(err, denied) || calls != 1 {
		t.Fatalf("desktop activation got %v after %d callbacks", err, calls)
	}
}
