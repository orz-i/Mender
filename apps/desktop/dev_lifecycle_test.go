package main

import (
	"os"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

// Refresh otherwise sends an immediate SIGKILL to the Wails primary process
// group. The Tunnel client runs in a separate OS session, so only the Wails
// SIGTERM -> OnShutdown -> Runtime.Close path can stop that child cleanly.
func TestDevelopmentWailsPrimaryAllowsGracefulRuntimeShutdown(t *testing.T) {
	raw, err := os.ReadFile("build/config.yml")
	if err != nil {
		t.Fatal(err)
	}

	var configuration struct {
		DevMode struct {
			Executes []struct {
				Command         string `yaml:"cmd"`
				Type            string `yaml:"type"`
				ExitPolicy      string `yaml:"exit_policy"`
				ShutdownTimeout string `yaml:"shutdown_timeout"`
			} `yaml:"executes"`
		} `yaml:"dev_mode"`
	}
	if err := yaml.Unmarshal(raw, &configuration); err != nil {
		t.Fatalf("parse Wails dev config: %v", err)
	}

	primaryCount := 0
	for _, step := range configuration.DevMode.Executes {
		if step.Type != "primary" {
			continue
		}
		primaryCount++
		if step.Command != "wails3 task run" || step.ExitPolicy != "shutdown" {
			t.Fatalf("unexpected Wails primary: %+v", step)
		}
		grace, err := time.ParseDuration(step.ShutdownTimeout)
		if err != nil || grace < 5*time.Second || grace > 30*time.Second {
			t.Fatalf("Wails primary must allow bounded SIGTERM cleanup, got %q: %v", step.ShutdownTimeout, err)
		}
	}
	if primaryCount != 1 {
		t.Fatalf("want exactly one Wails primary, got %d", primaryCount)
	}
}
