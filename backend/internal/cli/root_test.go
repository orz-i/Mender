package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"path/filepath"
	"strings"
	"testing"

	"github.com/orz-i/Mender/backend/internal/buildinfo"
)

func TestProviderPluginCommandSurface(t *testing.T) {
	t.Parallel()

	root := NewRootCommand(Options{
		Stdin:  strings.NewReader(""),
		Stdout: io.Discard,
		Stderr: io.Discard,
	})
	for _, path := range [][]string{
		{"plugin", "list"},
		{"plugin", "install"},
		{"plugin", "inspect"},
		{"plugin", "enable"},
		{"plugin", "disable"},
		{"plugin", "remove"},
		{"provider", "add", "external"},
	} {
		command, _, err := root.Find(path)
		if err != nil {
			t.Fatalf("Find(%v) error = %v", path, err)
		}
		if command == nil {
			t.Fatalf("Find(%v) command = nil", path)
		}
	}

	external, _, err := root.Find([]string{"provider", "add", "external"})
	if err != nil {
		t.Fatal(err)
	}
	for _, flag := range []string{
		"namespace",
		"name",
		"installation",
		"config-json",
		"config-file",
	} {
		if external.Flags().Lookup(flag) == nil {
			t.Fatalf("provider add external missing --%s", flag)
		}
	}
}

func TestProviderAddRetiresLegacyTransportAliases(t *testing.T) {
	t.Parallel()

	root := NewRootCommand(Options{
		Stdin:  strings.NewReader(""),
		Stdout: io.Discard,
		Stderr: io.Discard,
	})
	add, _, err := root.Find([]string{"provider", "add"})
	if err != nil {
		t.Fatal(err)
	}

	for _, name := range []string{"http", "stdio"} {
		for _, child := range add.Commands() {
			if child.Name() == name {
				t.Fatalf("retired provider add %s alias is still registered", name)
			}
		}
		mcp, _, err := root.Find([]string{"provider", "add", "mcp", name})
		if err != nil || mcp.Name() != name {
			t.Fatalf("canonical provider add mcp %s missing: %v", name, err)
		}

		root.SetArgs([]string{"provider", "add", name})
		if err := root.ExecuteContext(context.Background()); err == nil {
			t.Fatalf("retired provider add %s command unexpectedly accepted", name)
		}
	}
}

func TestCredentialNativeCommandDoesNotExposeSecretFlag(t *testing.T) {
	t.Parallel()

	root := NewRootCommand(Options{Stdin: strings.NewReader("secret"), Stdout: io.Discard, Stderr: io.Discard})
	command, _, err := root.Find([]string{"credential", "add", "native"})
	if err != nil {
		t.Fatal(err)
	}
	if command.Flags().Lookup("secret") != nil {
		t.Fatal("native Credential command exposes a secret command-line flag")
	}
	if command.Flags().Lookup("secret-stdin") == nil {
		t.Fatal("native Credential command is missing --secret-stdin")
	}
}

func TestPolicyCLIWorkflow(t *testing.T) {
	t.Parallel()

	dbPath := filepath.Join(t.TempDir(), "mender.db")
	execute := func(args ...string) map[string]any {
		t.Helper()
		var stdout, stderr bytes.Buffer
		root := NewRootCommand(Options{
			Stdin:  strings.NewReader(""),
			Stdout: &stdout,
			Stderr: &stderr,
		})
		root.SetArgs(append([]string{"--db", dbPath, "--output", "json"}, args...))
		if err := root.ExecuteContext(context.Background()); err != nil {
			t.Fatalf("%v: error = %v stderr=%q", args, err, stderr.String())
		}
		var value map[string]any
		if err := json.Unmarshal(stdout.Bytes(), &value); err != nil {
			t.Fatalf("%v: decode JSON: %v output=%q", args, err, stdout.String())
		}
		return value
	}

	policy := execute("policy", "show")
	if policy["defaultDecision"] != "allow" {
		t.Fatalf("fresh Policy = %#v", policy)
	}

	policy = execute("policy", "set-default", "deny")
	if policy["defaultDecision"] != "deny" {
		t.Fatalf("set-default Policy = %#v", policy)
	}

	rule := execute(
		"policy", "rule", "add",
		"--action", "tool.invoke",
		"--name", "Allow internal Catalog",
		"--decision", "allow",
		"--origin", "internal",
		"--scope", "catalog",
	)
	ruleID, ok := rule["ruleId"].(string)
	if !ok || !strings.HasPrefix(ruleID, "rule_") {
		t.Fatalf("created Rule = %#v", rule)
	}
	if _, exists := rule["priority"]; exists {
		t.Fatalf("created Rule unexpectedly has priority = %#v", rule)
	}

	policy = execute("policy", "show")
	rules, ok := policy["rules"].([]any)
	if !ok || len(rules) != 1 {
		t.Fatalf("Policy rules = %#v", policy)
	}

	disabled := execute("policy", "rule", "disable", ruleID)
	if disabled["enabled"] != false {
		t.Fatalf("disabled Rule = %#v", disabled)
	}

	removed := execute("policy", "rule", "remove", ruleID)
	if removed["ruleId"] != ruleID {
		t.Fatalf("removed Rule = %#v", removed)
	}

	policy = execute("policy", "show")
	rules, ok = policy["rules"].([]any)
	if !ok || len(rules) != 0 {
		t.Fatalf("Policy after remove = %#v", policy)
	}
}

func TestPolicyRuleAddRejectsInvalidResourceAsUsage(t *testing.T) {
	t.Parallel()

	var stdout, stderr bytes.Buffer
	code := Run(
		context.Background(),
		[]string{
			"policy", "rule", "add",
			"--action", "tool.invoke",
			"--name", "Invalid",
			"--decision", "deny",
			"--resource", "regex:path:.*",
		},
		&stdout,
		&stderr,
	)
	if code != ExitUsage {
		t.Fatalf("exit code = %d, want %d; stderr=%q", code, ExitUsage, stderr.String())
	}
	if !strings.Contains(stderr.String(), "resource.operator") {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestCredentialEnvironmentCLIWorkflow(t *testing.T) {
	t.Parallel()

	dbPath := filepath.Join(t.TempDir(), "mender.db")
	var stdout, stderr bytes.Buffer
	root := NewRootCommand(Options{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr})
	root.SetArgs([]string{
		"--db", dbPath,
		"--output", "json",
		"credential", "add", "environment",
		"demo", "work",
		"--display-name", "Demo work",
		"--env", "DEMO_WORK_TOKEN",
	})
	if err := root.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("credential add error = %v stderr=%q", err, stderr.String())
	}
	var created map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created["reference"] != "credential://demo/work" ||
		created["source"] != "environment" ||
		created["environmentVariable"] != "DEMO_WORK_TOKEN" {
		t.Fatalf("created Credential = %#v", created)
	}
	if _, exists := created["secret"]; exists {
		t.Fatalf("created Credential leaked secret field: %#v", created)
	}

	stdout.Reset()
	stderr.Reset()
	root = NewRootCommand(Options{Stdout: &stdout, Stderr: &stderr})
	root.SetArgs([]string{"--db", dbPath, "--output", "json", "credential", "list"})
	if err := root.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("credential list error = %v stderr=%q", err, stderr.String())
	}
	var listed map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &listed); err != nil {
		t.Fatal(err)
	}
	items, ok := listed["credentials"].([]any)
	if !ok || len(items) != 1 {
		t.Fatalf("credential list = %#v", listed)
	}
}

func TestDatabasePathResolution(t *testing.T) {
	t.Parallel()

	t.Run("explicit", func(t *testing.T) {
		t.Parallel()
		a := &app{
			options: normalizeOptions(Options{
				Getenv:  func(string) string { return "/ignored/env.db" },
				HomeDir: func() (string, error) { return "/ignored/home", nil },
			}),
			dbPath: filepath.Join(t.TempDir(), "explicit.db"),
		}
		got, err := a.resolveDatabasePath()
		if err != nil {
			t.Fatal(err)
		}
		if filepath.Base(got) != "explicit.db" {
			t.Fatalf("database path = %q", got)
		}
	})

	t.Run("default home", func(t *testing.T) {
		t.Parallel()
		home := t.TempDir()
		a := &app{
			options: normalizeOptions(Options{
				Getenv:  func(string) string { return "" },
				HomeDir: func() (string, error) { return home, nil },
			}),
		}
		got, err := a.resolveDatabasePath()
		if err != nil {
			t.Fatal(err)
		}
		want := filepath.Join(home, ".mender", "mender.db")
		if got != want {
			t.Fatalf("database path = %q, want %q", got, want)
		}
	})

	t.Run("environment", func(t *testing.T) {
		t.Parallel()
		a := &app{
			options: normalizeOptions(Options{
				Getenv: func(key string) string {
					if key == "MENDER_DB" {
						return filepath.Join(t.TempDir(), "env.db")
					}
					return ""
				},
				HomeDir: func() (string, error) { return "/ignored/home", nil },
			}),
		}
		got, err := a.resolveDatabasePath()
		if err != nil {
			t.Fatal(err)
		}
		if filepath.Base(got) != "env.db" {
			t.Fatalf("database path = %q", got)
		}
	})
}

func TestVersionCommandJSON(t *testing.T) {
	oldVersion := buildinfo.Version
	oldCommit := buildinfo.Commit
	oldBuildDate := buildinfo.BuildDate
	t.Cleanup(func() {
		buildinfo.Version = oldVersion
		buildinfo.Commit = oldCommit
		buildinfo.BuildDate = oldBuildDate
	})

	buildinfo.Version = "v1.2.3"
	buildinfo.Commit = "abcdef123456"
	buildinfo.BuildDate = "2026-09-26T12:00:00Z"

	var stdout, stderr bytes.Buffer
	root := NewRootCommand(Options{Stdout: &stdout, Stderr: &stderr})
	root.SetArgs([]string{"--output", "json", "version"})
	if err := root.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("version error = %v stderr=%s", err, stderr.String())
	}

	var value map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &value); err != nil {
		t.Fatalf("version JSON error = %v output=%q", err, stdout.String())
	}
	if value["version"] != "v1.2.3" ||
		value["commit"] != "abcdef123456" ||
		value["buildDate"] != "2026-09-26T12:00:00Z" {
		t.Fatalf("version output = %#v", value)
	}
}

func TestProfileCreateListAndShowJSON(t *testing.T) {
	t.Parallel()

	dbPath := filepath.Join(t.TempDir(), "mender.db")
	execute := func(args ...string) map[string]any {
		t.Helper()

		var stdout, stderr bytes.Buffer
		root := NewRootCommand(Options{Stdout: &stdout, Stderr: &stderr})
		root.SetArgs(append([]string{"--db", dbPath, "--output", "json"}, args...))
		if err := root.ExecuteContext(context.Background()); err != nil {
			t.Fatalf("command %v error = %v stderr=%s", args, err, stderr.String())
		}

		decoder := json.NewDecoder(&stdout)
		var value map[string]any
		if err := decoder.Decode(&value); err != nil {
			t.Fatalf("command %v JSON error = %v output=%q", args, err, stdout.String())
		}
		if decoder.More() {
			t.Fatalf("command %v emitted more than one JSON document", args)
		}
		return value
	}

	created := execute(
		"profile", "create",
		"--slug", "default",
		"--name", "Default",
		"--mode", "hybrid",
	)
	if created["slug"] != "default" || created["mode"] != "hybrid" {
		t.Fatalf("created profile = %#v", created)
	}

	listed := execute("profile", "list")
	profiles, ok := listed["profiles"].([]any)
	if !ok || len(profiles) != 1 {
		t.Fatalf("profile list = %#v", listed)
	}

	shown := execute("profile", "show", "default")
	if shown["slug"] != "default" {
		t.Fatalf("shown profile = %#v", shown)
	}
}

func TestProviderListJSONIsSingleDocument(t *testing.T) {
	t.Parallel()

	var stdout, stderr bytes.Buffer
	root := NewRootCommand(Options{Stdout: &stdout, Stderr: &stderr})
	root.SetArgs([]string{
		"--db", filepath.Join(t.TempDir(), "mender.db"),
		"--output", "json",
		"provider", "list",
	})
	if err := root.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("provider list error = %v stderr=%s", err, stderr.String())
	}

	decoder := json.NewDecoder(&stdout)
	var value map[string]any
	if err := decoder.Decode(&value); err != nil {
		t.Fatalf("JSON error = %v output=%q", err, stdout.String())
	}
	if _, ok := value["providers"]; !ok {
		t.Fatalf("provider list = %#v", value)
	}
	var extra any
	if err := decoder.Decode(&extra); err == nil {
		t.Fatalf("provider list emitted multiple JSON documents: %q", stdout.String())
	}
}

func TestProviderPluginsJSONListsBuiltInMCP(t *testing.T) {
	t.Parallel()

	var stdout, stderr bytes.Buffer
	root := NewRootCommand(Options{Stdout: &stdout, Stderr: &stderr})
	root.SetArgs([]string{
		"--db", filepath.Join(t.TempDir(), "mender.db"),
		"--output", "json",
		"provider", "plugins",
	})
	if err := root.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("provider plugins error = %v stderr=%s", err, stderr.String())
	}

	var value struct {
		Plugins []struct {
			PluginID string `json:"pluginId"`
		} `json:"plugins"`
	}
	if err := json.NewDecoder(&stdout).Decode(&value); err != nil {
		t.Fatalf("provider plugins JSON error = %v output=%q", err, stdout.String())
	}
	if len(value.Plugins) != 5 ||
		value.Plugins[0].PluginID != "io.mender.provider.agent-delegation" ||
		value.Plugins[1].PluginID != "io.mender.provider.mcp" ||
		value.Plugins[2].PluginID != "io.mender.provider.model-tools" ||
		value.Plugins[3].PluginID != "io.mender.provider.openapi" ||
		value.Plugins[4].PluginID != "io.mender.provider.workspace" {
		t.Fatalf("provider plugins = %#v", value.Plugins)
	}
}

func TestProfileAddToolRejectsInvalidBindingPolicyAsUsage(t *testing.T) {
	t.Parallel()

	var stdout, stderr bytes.Buffer
	code := Run(
		context.Background(),
		[]string{
			"profile", "add-tool",
			"default",
			"tool_00000000000000000000000000000001",
			"--policy", "allow",
		},
		&stdout,
		&stderr,
	)
	if code != ExitUsage {
		t.Fatalf("exit code = %d, want %d; stderr=%q", code, ExitUsage, stderr.String())
	}
	if !strings.Contains(stderr.String(), "policy must be inherit") {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestRunExitCodes(t *testing.T) {
	t.Parallel()

	var stdout, stderr bytes.Buffer
	code := Run(
		context.Background(),
		[]string{"profile", "create"},
		&stdout,
		&stderr,
	)
	if code != ExitUsage {
		t.Fatalf("usage exit code = %d, want %d; stderr=%q", code, ExitUsage, stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	code = Run(
		context.Background(),
		[]string{"--db", filepath.Join(t.TempDir(), "mender.db"), "profile", "show", "missing"},
		&stdout,
		&stderr,
	)
	if code != ExitConfig {
		t.Fatalf("config exit code = %d, want %d; stderr=%q", code, ExitConfig, stderr.String())
	}
}
