package events

import invocationdomain "github.com/orz-i/Mender/backend/internal/invocation/domain"

// NoopObserver discards best-effort Invocation lifecycle events.
type NoopObserver struct{}

func (NoopObserver) Observe(invocationdomain.Event) {}
