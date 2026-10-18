package acp

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	agentdomain "github.com/orz-i/Mender/backend/internal/agent/domain"
	isolation "github.com/orz-i/Mender/backend/internal/runtimegovernance/infrastructure/processisolation"
)

func TestAntigravityTrustedLauncherOnlyPassesExplicitYOLO(t *testing.T) {
	launcher, err := NewLocalFactory(isolation.New())
	if err != nil {
		t.Fatal(err)
	}
	binary, err := launcher.Availability(agentdomain.BackendAntigravity)
	if errors.Is(err, agentdomain.ErrUnavailable) {
		t.Skip("agy CLI not installed")
	}
	if err != nil {
		t.Fatal(err)
	}
	spec := fixtureAgent(t).Spec()
	root, err := filepath.EvalSymlinks(spec.WorkspaceRoot)
	if err != nil {
		t.Fatal(err)
	}
	spec.WorkspaceRoot = root
	spec.Backend = agentdomain.BackendAntigravity
	spec.PluginID = spec.Backend.PluginID()
	spec.Isolation = "host"
	a, err := agentdomain.NewAgent(spec)
	if err != nil {
		t.Fatal(err)
	}
	envPath := t.TempDir()
	if err := os.WriteFile(filepath.Join(envPath, "agy"), []byte("#! /bin/sh\nexit 99\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	// #nosec G302 -- isolated test-only CLI decoy must be executable.
	if err := os.Chmod(filepath.Join(envPath, "agy"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", envPath)
	for _, tc := range []struct {
		name               string
		model, effort      string
		inspect, allowYOLO bool
		expected           []string
	}{
		{"default", "", "", false, false, []string{"--input-format", "stream-json", "--output-format", "stream-json", "--disable-slash-commands", "--sandbox"}},
		{"YOLO", "gemini-3.8-flash-high", "high", false, true, []string{"--input-format", "stream-json", "--output-format", "stream-json", "--disable-slash-commands", "--sandbox", "--dangerously-skip-permissions", "--model", "gemini-3.8-flash-high", "--effort", "high"}},
		{"inspect_never_YOLO", "", "", true, true, []string{"models"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cmd, err := launcher.AntigravityCommand(context.Background(), a, root, tc.model, tc.effort, tc.inspect, tc.allowYOLO)
			if err != nil {
				t.Fatal(err)
			}
			if cmd.Path != binary || cmd.Dir != root || !slices.Equal(cmd.Args[1:], tc.expected) {
				t.Fatalf("unsafe AGY command path=%q argv=%q cwd=%q", cmd.Path, cmd.Args, cmd.Dir)
			}
			for _, v := range cmd.Env {
				if strings.HasPrefix(v, "AGY_") || strings.HasPrefix(v, "GOOGLE_") {
					t.Fatalf("inherited secret-like environment variable name %q", strings.SplitN(v, "=", 2)[0])
				}
			}
		})
	}
	for _, slug := range []string{"--dangerously-skip-permissions", "a b", "a\n"} {
		if _, err := launcher.AntigravityCommand(context.Background(), a, root, slug, "", false, false); !errors.Is(err, agentdomain.ErrInvalid) {
			t.Errorf("unsafe model accepted: %q err=%v", slug, err)
		}
	}
	for _, level := range []string{"super", "invalid"} {
		if _, err := launcher.AntigravityCommand(context.Background(), a, root, "", level, false, false); !errors.Is(err, agentdomain.ErrInvalid) {
			t.Errorf("unknown effort accepted: %q err=%v", level, err)
		}
	}
}
