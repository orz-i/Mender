package executionguard

import (
	"context"
	"sync"

	invocationapplication "github.com/orz-i/Mender/backend/internal/invocation/application"
)

// Unlimited admits every execution immediately while still honoring cancellation.
type Unlimited struct{}

func (Unlimited) Acquire(
	ctx context.Context,
	_ invocationapplication.ExecutionGuardRequest,
) (func(), error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return func() {}, nil
}

// ProviderSerial serializes executions that target the same Provider.
type ProviderSerial struct {
	serial keyedSerial
}

func NewProviderSerial() *ProviderSerial {
	return &ProviderSerial{}
}

func (g *ProviderSerial) Acquire(
	ctx context.Context,
	request invocationapplication.ExecutionGuardRequest,
) (func(), error) {
	return g.serial.acquire(ctx, request.Tool.ProviderID().String())
}

// ToolSerial serializes executions that target the same canonical Tool.
type ToolSerial struct {
	serial keyedSerial
}

func NewToolSerial() *ToolSerial {
	return &ToolSerial{}
}

func (g *ToolSerial) Acquire(
	ctx context.Context,
	request invocationapplication.ExecutionGuardRequest,
) (func(), error) {
	return g.serial.acquire(ctx, request.Tool.ID().String())
}

type keyedSerial struct {
	mu      sync.Mutex
	entries map[string]*entry
}

type entry struct {
	token chan struct{}
	refs  int
}

func (s *keyedSerial) acquire(ctx context.Context, key string) (func(), error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	s.mu.Lock()
	if s.entries == nil {
		s.entries = make(map[string]*entry)
	}
	current := s.entries[key]
	if current == nil {
		current = &entry{token: make(chan struct{}, 1)}
		s.entries[key] = current
	}
	current.refs++
	s.mu.Unlock()

	select {
	case current.token <- struct{}{}:
		var once sync.Once
		return func() {
			once.Do(func() {
				<-current.token
				s.releaseRef(key, current)
			})
		}, nil
	case <-ctx.Done():
		s.releaseRef(key, current)
		return nil, ctx.Err()
	}
}

func (s *keyedSerial) releaseRef(key string, current *entry) {
	s.mu.Lock()
	defer s.mu.Unlock()

	current.refs--
	if current.refs == 0 {
		delete(s.entries, key)
	}
}
