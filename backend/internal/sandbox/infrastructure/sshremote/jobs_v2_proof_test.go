package sshremote

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestV2ResolutionRequiresFreshRemoteDurabilityProof(t *testing.T) {
	ctx := context.Background()
	jobs, _, runner := fixtureV2Jobs(t)
	if err := jobs.Prepare(ctx); err != nil {
		t.Fatal(err)
	}
	submitted, err := jobs.SubmitV2(ctx, JobRequest{
		IdempotencyKey: strings.Repeat("2", 32), Command: "echo must-remain-one-shot",
	}, 1)
	if err != nil {
		t.Fatal(err)
	}
	before, err := jobs.InspectRecovery(ctx)
	if err != nil || len(before.Records) != 1 || !before.Records[0].Recoverable {
		t.Fatalf("missing candidate before recovery: %+v %v", before, err)
	}
	result, err := jobs.ResolveV2UnderFence(ctx, V2ResolveRequest{
		TaskID: submitted.TaskID, Fingerprint: before.Records[0].Fingerprint,
		Generation: 1, Actor: "owner:fixture", Reason: "inspected and reviewed evidence",
	})
	if err != nil || result.Activity != "quiescent_proved" {
		t.Fatalf("valid initial remote sync not proven: %+v %v", result, err)
	}

	// The JSON marker is still present, valid, and visible. An independent
	// durability proof now fails, modelling a lost remote sync ACK. Neither
	// owner observation nor the lifecycle may call the job quiescent.
	runner.muV2.Lock()
	runner.proofErr = errors.New("remote fsync acknowledgement unavailable")
	runner.muV2.Unlock()
	inspection, err := jobs.InspectRecovery(ctx)
	if err != nil || len(inspection.Records) != 1 ||
		inspection.Records[0].Activity != "uncertain" || inspection.Records[0].Recoverable {
		t.Fatalf("unproved disk marker permitted recovery or quiescence: %+v %v", inspection, err)
	}
	if err := jobs.Quiescent(ctx); !errors.Is(err, ErrJobUnknown) {
		t.Fatalf("unproved marker released lifecycle: %v", err)
	}
	snapshot, err := jobs.Status(ctx, submitted.TaskID)
	if err != nil || snapshot.ExitCode != nil || snapshot.State != JobUnknown {
		t.Fatalf("operational recovery fabricated a shell exit: %+v %v", snapshot, err)
	}
	runner.muV2.Lock()
	runner.proofErr = nil
	runner.muV2.Unlock()

	// A fresh fsync under the exact old launch lock can establish durability
	// after a lost ACK. It never writes a second resolution or launches code.
	rechecked, err := jobs.InspectRecovery(ctx)
	if err != nil || rechecked.Records[0].Phase != "resolved_before_launch" ||
		rechecked.Records[0].Activity != "quiescent_proved" {
		t.Fatalf("fresh durability certificate not recognized: %+v %v", rechecked, err)
	}
	if err = jobs.Quiescent(ctx); err != nil {
		t.Fatalf("fresh durability proof did not release lifecycle: %v", err)
	}
	runner.muV2.Lock()
	proofs, decisions := runner.proofCalls, runner.recovered
	runner.muV2.Unlock()
	if proofs < 4 || decisions != 1 {
		t.Fatalf("fsync proof did not stay separate from resolution: probes=%d writes=%d", proofs, decisions)
	}
}

func TestV2ResolutionProofRejectsArbitraryPathsAndHashes(t *testing.T) {
	validDir := "/tmp/mender-private/sjob_" + strings.Repeat("a", 32)
	validHash := strings.Repeat("f", 64)
	for _, tc := range []struct{ dir, hash string }{
		{"../relative", validHash},
		{"/tmp/other-directory", validHash},
		{validDir, "not-a-sha256"},
		{"/tmp/mender-private/sjob_" + strings.Repeat("b", 31), validHash},
	} {
		if _, err := v2ResolutionProofShell(tc.dir, tc.hash); !errors.Is(err, ErrJobInvalid) {
			t.Fatalf("accepted unsafe proof request dir=%q: %v", tc.dir, err)
		}
	}
	if _, err := v2ResolutionProofShell(validDir, validHash); err != nil {
		t.Fatalf("rejected exact test-owned identity: %v", err)
	}
}
