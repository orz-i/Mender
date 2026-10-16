package acp

import (
	"context"
	"errors"
	"testing"
	"time"

	agentdomain "github.com/orz-i/Mender/backend/internal/agent/domain"
	processisolation "github.com/orz-i/Mender/backend/internal/runtimegovernance/infrastructure/processisolation"
)

func TestLocalAgentBackendAvailabilityIsInert(t *testing.T) {
	launcher, err := NewLocalFactory(processisolation.New())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := launcher.Availability("unknown"); !errors.Is(err, agentdomain.ErrUnavailable) {
		t.Fatal("unknown CLI accepted", err)
	}
	// Neither lookup nor Command invokes a model; no test touches
	// a real credential/session or modifies a user workspace.
	_, _ = launcher.Availability(agentdomain.BackendCodex)
	_, _ = launcher.Availability(agentdomain.BackendGrok)
	_, _ = launcher.Availability(agentdomain.BackendOpenCode)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_, err = launcher.Command(ctx, fixtureAgent(t))
	if err != nil && !errors.Is(err, agentdomain.ErrUnavailable) {
		t.Fatalf("unexpected launcher diagnostic: %v", err)
	}
}
