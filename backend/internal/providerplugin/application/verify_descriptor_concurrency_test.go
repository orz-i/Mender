package application

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	platformplugin "github.com/orz-i/Mender/backend/internal/plugin"
	installationdomain "github.com/orz-i/Mender/backend/internal/plugin/installation/domain"
	"github.com/orz-i/Mender/backend/internal/providerplugin"
)

func TestConcurrentVerifiersCloseTheirOwnProbeBeforePublishing(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	item, descriptor := verificationFixture(t)
	store := &concurrentVerificationStore{item: item}
	first, second := newBlockedVerificationProcess(), newBlockedVerificationProcess()
	t.Cleanup(func() {
		first.release()
		second.release()
	})
	results := make(chan error, 2)
	for _, processes := range []*blockedVerificationProcess{first, second} {
		go func() {
			_, err := NewVerifyDescriptor(orderedPublication{DescriptorInstallations: store, closed: &processes.closed},
				&verificationProbe{descriptor: descriptor}, processes, nil).Execute(ctx, item.ID())
			results <- err
		}()
	}
	for _, entered := range []<-chan struct{}{first.entered, second.entered} {
		select {
		case <-entered:
		case <-ctx.Done():
			t.Fatal("verifier never reached private cleanup")
		}
	}
	current, err := store.Get(ctx, item.ID())
	if err != nil {
		t.Fatal(err)
	}
	if _, published := current.VerifiedDescriptor(); published {
		t.Fatal("snapshot became executable before probe cleanup completed")
	}
	first.release()
	select {
	case err := <-results:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal("first verification did not publish after cleanup")
	}
	if second.closed.Load() {
		t.Fatal("first verification closed the other verification process")
	}
	second.release()
	select {
	case err := <-results:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal("second verification did not finish independently")
	}
}

type orderedPublication struct {
	DescriptorInstallations
	closed *atomic.Bool
}

func (s orderedPublication) SetVerifiedDescriptor(ctx context.Context, id providerplugin.InstallationID, descriptor platformplugin.Descriptor) error {
	if !s.closed.Load() {
		return fmt.Errorf("published before verification process cleanup")
	}
	return s.DescriptorInstallations.SetVerifiedDescriptor(ctx, id, descriptor)
}

type concurrentVerificationStore struct {
	mu   sync.Mutex
	item installationdomain.Installation
}

func (s *concurrentVerificationStore) Get(context.Context, providerplugin.InstallationID) (installationdomain.Installation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.item, nil
}

func (s *concurrentVerificationStore) SetVerifiedDescriptor(_ context.Context, _ providerplugin.InstallationID, descriptor platformplugin.Descriptor) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	var err error
	s.item, err = s.item.WithVerifiedDescriptor(descriptor)
	return err
}

type blockedVerificationProcess struct {
	entered chan struct{}
	resume  chan struct{}
	once    sync.Once
	closed  atomic.Bool
}

func newBlockedVerificationProcess() *blockedVerificationProcess {
	return &blockedVerificationProcess{entered: make(chan struct{}), resume: make(chan struct{})}
}

func (p *blockedVerificationProcess) release() { p.once.Do(func() { close(p.resume) }) }

func (p *blockedVerificationProcess) CloseInstallation(providerplugin.InstallationID) error {
	close(p.entered)
	<-p.resume
	p.closed.Store(true)
	return nil
}
