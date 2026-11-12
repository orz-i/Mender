package application

import (
	"testing"
	"time"

	agentdomain "github.com/orz-i/Mender/backend/internal/agent/domain"
)

func TestTaskEventBroadcaster_SubscribeAndBroadcast(t *testing.T) {
	broadcaster := NewTaskEventBroadcaster()
	taskID := agentdomain.TaskID("atask_00000000000000000000000000000001")
	otherTaskID := agentdomain.TaskID("atask_00000000000000000000000000000002")

	ch1, unsub1 := broadcaster.Subscribe(taskID)
	defer unsub1()

	ch2, unsub2 := broadcaster.Subscribe(taskID)
	defer unsub2()

	otherCh, otherUnsub := broadcaster.Subscribe(otherTaskID)
	defer otherUnsub()

	if broadcaster.SubscriberCount(taskID) != 2 {
		t.Fatalf("expected 2 subscribers for taskID, got %d", broadcaster.SubscriberCount(taskID))
	}
	if broadcaster.SubscriberCount(otherTaskID) != 1 {
		t.Fatalf("expected 1 subscriber for otherTaskID, got %d", broadcaster.SubscriberCount(otherTaskID))
	}

	evt := agentdomain.TaskEvent{
		ID:        agentdomain.TaskEventID("aevt_00000000000000000000000000000001"),
		TaskID:    taskID,
		Seq:       1,
		Type:      agentdomain.TaskEventMessageChunk,
		Payload:   map[string]any{"chunk": "hello"},
		CreatedAt: time.Now().UTC(),
	}

	broadcaster.Broadcast(evt)

	select {
	case received := <-ch1:
		if received.ID != evt.ID || received.Seq != 1 {
			t.Fatalf("ch1 received unexpected event: %+v", received)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("ch1 timed out waiting for event")
	}

	select {
	case received := <-ch2:
		if received.ID != evt.ID || received.Seq != 1 {
			t.Fatalf("ch2 received unexpected event: %+v", received)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("ch2 timed out waiting for event")
	}

	// Verify otherTaskID did not receive the event
	select {
	case received := <-otherCh:
		t.Fatalf("otherCh unexpectedly received event: %+v", received)
	case <-time.After(50 * time.Millisecond):
		// Expected
	}

	// Unsubscribe ch1
	unsub1()
	if broadcaster.SubscriberCount(taskID) != 1 {
		t.Fatalf("expected 1 subscriber after unsub1, got %d", broadcaster.SubscriberCount(taskID))
	}

	// Channel should be closed
	_, ok := <-ch1
	if ok {
		t.Fatal("expected ch1 to be closed after unsubscribe")
	}

	// Broadcast second event
	evt2 := agentdomain.TaskEvent{
		ID:        agentdomain.TaskEventID("aevt_00000000000000000000000000000002"),
		TaskID:    taskID,
		Seq:       2,
		Type:      agentdomain.TaskEventTerminal,
		Payload:   map[string]any{"status": "completed"},
		CreatedAt: time.Now().UTC(),
	}

	broadcaster.Broadcast(evt2)

	select {
	case received := <-ch2:
		if received.ID != evt2.ID || received.Seq != 2 {
			t.Fatalf("ch2 received unexpected event: %+v", received)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("ch2 timed out waiting for second event")
	}

	unsub2()
	if broadcaster.SubscriberCount(taskID) != 0 {
		t.Fatalf("expected 0 subscribers after unsub2, got %d", broadcaster.SubscriberCount(taskID))
	}
}

func TestTaskEventBroadcaster_MultipleUnsubscribeSafe(t *testing.T) {
	broadcaster := NewTaskEventBroadcaster()
	taskID := agentdomain.TaskID("atask_00000000000000000000000000000001")

	_, unsub := broadcaster.Subscribe(taskID)
	// Calling unsub multiple times must not panic
	unsub()
	unsub()
	unsub()
}
