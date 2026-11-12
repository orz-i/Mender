package application

import (
	"sync"

	agentdomain "github.com/orz-i/Mender/backend/internal/agent/domain"
)

// TaskEventBroadcaster is an in-memory pub-sub event broadcaster for task events.
type TaskEventBroadcaster struct {
	mu          sync.RWMutex
	subscribers map[agentdomain.TaskID]map[uint64]chan agentdomain.TaskEvent
	nextSubID   uint64
}

// NewTaskEventBroadcaster creates a new TaskEventBroadcaster.
func NewTaskEventBroadcaster() *TaskEventBroadcaster {
	return &TaskEventBroadcaster{
		subscribers: make(map[agentdomain.TaskID]map[uint64]chan agentdomain.TaskEvent),
	}
}

// Subscribe registers an active subscriber for events belonging to taskID.
// It returns a receive-only channel with a buffer and an unsubscribe function.
// Calling the unsubscribe function unregisters the channel and closes it.
func (b *TaskEventBroadcaster) Subscribe(taskID agentdomain.TaskID) (<-chan agentdomain.TaskEvent, func()) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.nextSubID++
	subID := b.nextSubID
	ch := make(chan agentdomain.TaskEvent, 256)

	if b.subscribers[taskID] == nil {
		b.subscribers[taskID] = make(map[uint64]chan agentdomain.TaskEvent)
	}
	b.subscribers[taskID][subID] = ch

	var once sync.Once
	unsubscribe := func() {
		once.Do(func() {
			b.mu.Lock()
			defer b.mu.Unlock()
			if subs, ok := b.subscribers[taskID]; ok {
				if subCh, exists := subs[subID]; exists {
					delete(subs, subID)
					close(subCh)
				}
				if len(subs) == 0 {
					delete(b.subscribers, taskID)
				}
			}
		})
	}

	return ch, unsubscribe
}

// Broadcast sends the event to all active subscribers for the event's TaskID.
// Sends are non-blocking with a generous buffer so slow consumers do not stall
// the agent execution loop.
func (b *TaskEventBroadcaster) Broadcast(event agentdomain.TaskEvent) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	subs, ok := b.subscribers[event.TaskID]
	if !ok || len(subs) == 0 {
		return
	}

	for _, ch := range subs {
		select {
		case ch <- event:
		default:
			// Buffer full, drop to prevent stalling the execution thread
		}
	}
}

// SubscriberCount returns the number of active subscribers for a taskID.
func (b *TaskEventBroadcaster) SubscriberCount(taskID agentdomain.TaskID) int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return len(b.subscribers[taskID])
}
