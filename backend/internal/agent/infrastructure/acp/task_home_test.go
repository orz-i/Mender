package acp

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"testing"
	"time"

	agentapplication "github.com/orz-i/Mender/backend/internal/agent/application"
	agentdomain "github.com/orz-i/Mender/backend/internal/agent/domain"
)

func TestGovernedAgentHomeIsPrivateNonInheritedAndReaped(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("fixed macOS private HOME boundary")
	}
	cmd := exec.Command("/bin/sh", "-c", "test \"$HOME\" != \"$OWNER_HOME\" && printf scratch > \"$HOME/stamp\"")
	cmd.Env = []string{
		"PATH=/usr/bin:/bin",
		"HOME=/Users/owner/old",
		"OWNER_HOME=/Users/owner/old",
		"HOME=/Users/owner/duplicate",
	}
	release, err := scopeGovernedAgentHome(cmd)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = release() }()
	var got []string
	for _, entry := range cmd.Env {
		if strings.HasPrefix(entry, "HOME=") {
			got = append(got, strings.TrimPrefix(entry, "HOME="))
		}
	}
	if len(got) != 1 {
		t.Fatalf("governed HOME was missing or duplicated: %d", len(got))
	}
	privateHome := got[0]
	if !strings.HasPrefix(privateHome, "/private/tmp/mender-agent-home-") {
		t.Fatal("governed HOME is not inside the trusted macOS temporary root")
	}
	info, err := os.Stat(privateHome)
	if err != nil || !info.IsDir() || info.Mode().Perm() != 0o700 {
		t.Fatalf("governed HOME must be a private directory: %v", err)
	}
	if err := cmd.Run(); err != nil {
		t.Fatalf("child could not write to its private HOME: %v", err)
	}
	privateRoot, err := os.OpenRoot(privateHome)
	if err != nil {
		t.Fatal(err)
	}
	content, readErr := privateRoot.ReadFile("stamp")
	_ = privateRoot.Close()
	if readErr != nil || string(content) != "scratch" {
		t.Fatal("child did not use its private HOME")
	}
	if err := release(); err != nil {
		t.Fatalf("cleanup private HOME: %v", err)
	}
	if _, err := os.Stat(privateHome); !os.IsNotExist(err) {
		t.Fatal("governed child HOME survived release")
	}
}

func TestGovernedACPTaskReapsItsPrivateHomeAfterSession(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("macOS governed task private HOME lifecycle")
	}
	agent := fixtureAgent(t)
	runner := fixtureRunner("mcp-http")
	original := runner.NewCommandWithConfig
	var command *exec.Cmd
	runner.NewCommandWithConfig = func(ctx context.Context, a agentdomain.Agent, cfg agentapplication.RunConfig) (*exec.Cmd, error) {
		created, err := original(ctx, a, cfg)
		command = created
		return created, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
	defer cancel()
	result, err := runner.ExecuteConfigured(ctx, agent, "bounded private-home task", agentapplication.RunConfig{
		WorkspaceRoot: agent.Spec().WorkspaceRoot,
		ToolSupply: agentapplication.ToolSupply{Delivery: agentapplication.ToolDeliveryMCPHTTP,
			Endpoint: "http://127.0.0.1:12345/mcp", BearerToken: strings.Repeat("a", 64)},
	})
	if err != nil || result != "fixture completed" || command == nil {
		t.Fatalf("governed task did not finish cleanly: %v", err)
	}
	var home string
	for _, variable := range command.Env {
		if strings.HasPrefix(variable, "HOME=") {
			if home != "" {
				t.Fatal("ACP command has duplicate HOME variables")
			}
			home = strings.TrimPrefix(variable, "HOME=")
		}
	}
	if !strings.HasPrefix(home, "/private/tmp/mender-agent-home-") {
		t.Fatal("ACP command inherited the real user HOME")
	}
	if _, err := os.Stat(home); !os.IsNotExist(err) {
		t.Fatal("ACP task left its private HOME after subprocess reaping")
	}
}

func TestGovernedAgentHomeRejectsAmbientEnvironmentInheritance(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("fixed macOS private HOME boundary")
	}
	cmd := exec.Command("/bin/true")
	if _, err := scopeGovernedAgentHome(cmd); !errors.Is(err, agentdomain.ErrUnavailable) {
		t.Fatalf("governed command without explicit Env inherited host secrets: %v", err)
	}
}
