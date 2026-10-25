package application

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	agentdomain "github.com/orz-i/Mender/backend/internal/agent/domain"
)

func TestExplicitAgentTaskTitleDoesNotPersistPrompt(t *testing.T) {
	ctx := context.Background()
	runner := &boundedDeadlineRunner{remaining: make(chan time.Duration, 2)}
	svc, owner := newFixtureService(t, runner)
	agent, err := svc.Create(ctx, CreateRequest{
		Name: "Task title fixture", Backend: agentdomain.BackendGrok,
		WorkspaceRoot: t.TempDir(),
	}, owner)
	if err != nil {
		t.Fatal(err)
	}
	for _, invalid := range []string{" bad", "a\nb", strings.Repeat("界", agentdomain.MaxTaskTitleRunes+1)} {
		if _, err := svc.SubmitWithOptions(ctx, agent.ID(), "private prompt", SubmitOptions{Title: invalid}, owner); !errors.Is(err, agentdomain.ErrInvalid) {
			t.Fatalf("invalid title submitted: %v", err)
		}
	}
	if tasks, err := svc.Tasks(ctx, agent.ID(), owner); err != nil || len(tasks) != 0 {
		t.Fatalf("invalid titles created Tasks: %d %v", len(tasks), err)
	}
	const title = "检查工具来源"
	const prompt = "PRIVATE_PROMPT_MUST_NOT_BE_SAVED"
	item, err := svc.SubmitWithOptions(ctx, agent.ID(), prompt, SubmitOptions{Title: title}, owner)
	if err != nil || item.Title != title {
		t.Fatalf("title not admitted: %+v %v", item, err)
	}
	if persisted, err := svc.Task(ctx, item.ID, owner); err != nil || persisted.Title != title {
		t.Fatalf("title not persisted: %+v %v", persisted, err)
	}
	// Task JSON is exposed through owner APIs; prompt must never be included.
	b, err := json.Marshal(item)
	if err != nil || bytes.Contains(b, []byte(prompt)) || !bytes.Contains(b, []byte(title)) {
		t.Fatalf("owner Task metadata disclosed prompt or lost title: %s %v", b, err)
	}
	select {
	case <-runner.remaining:
	case <-time.After(3 * time.Second):
		t.Fatal("runner failed to receive admitted Task")
	}
	completed := waitTask(t, svc, item.ID, owner, agentdomain.TaskCompleted)
	if completed.Title != title || completed.Result != "ok" {
		t.Fatalf("execution changed title or result: %+v", completed)
	}
}
