package antigravity

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// The protocol fixture runs this test binary, so process, stdin and cancellation
// coverage does not depend on a shell interpreter or executable script support.
type headlessProcessFixture struct {
	BeforePrompt     []string
	AfterPrompt      []string
	ExpectedPrompt   string
	PromptMarker     string
	BlockAfterPrompt bool
}

func processFixture(t *testing.T, fixture headlessProcessFixture) string {
	t.Helper()
	raw, err := json.Marshal(fixture)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "headless-fixture.json")
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func fixtureCommand(ctx context.Context, fixturePath string) *exec.Cmd {
	executable, err := os.Executable()
	if err != nil {
		panic(err)
	}
	cmd := exec.CommandContext(ctx, executable, "-test.run=^TestHeadlessFixtureProcess$", "--", fixturePath) // #nosec G204 -- executable is this test binary and fixturePath is testing TempDir data.
	cmd.Env = append(os.Environ(), "MENDER_ANTIGRAVITY_FIXTURE_PROCESS=1")
	return cmd
}

func TestHeadlessFixtureProcess(t *testing.T) {
	if os.Getenv("MENDER_ANTIGRAVITY_FIXTURE_PROCESS") != "1" {
		return
	}
	fixturePath := os.Args[len(os.Args)-1]
	raw, err := os.ReadFile(fixturePath) // #nosec G304 G703 -- parent test passes only its own private TempDir fixture; no application input reaches this helper.
	if err != nil {
		os.Exit(10)
	}
	var fixture headlessProcessFixture
	if err := json.Unmarshal(raw, &fixture); err != nil {
		os.Exit(11)
	}
	emit := func(lines []string) {
		for _, line := range lines {
			if _, err := fmt.Fprintln(os.Stdout, line); err != nil {
				os.Exit(12)
			}
		}
	}
	emit(fixture.BeforePrompt)
	input := bufio.NewScanner(os.Stdin)
	if input.Scan() {
		if fixture.ExpectedPrompt != "" && !strings.Contains(input.Text(), fixture.ExpectedPrompt) {
			os.Exit(13)
		}
		if fixture.PromptMarker != "" {
			if err := os.WriteFile(fixture.PromptMarker, []byte("sent"), 0o600); err != nil { // #nosec G703 -- private parent-selected TempDir marker.
				os.Exit(14)
			}
		}
		emit(fixture.AfterPrompt)
		if fixture.BlockAfterPrompt {
			// Simulate a hung child until CommandContext terminates it.
			timer := time.NewTimer(time.Minute)
			defer timer.Stop()
			<-timer.C
		}
	}
	// Do not emit the Go test harness PASS line onto the protocol stream.
	os.Exit(0)
}
