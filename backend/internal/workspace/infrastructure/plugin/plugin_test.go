package plugin

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
	invocationapplication "github.com/orz-i/Mender/backend/internal/invocation/application"
	invocationdomain "github.com/orz-i/Mender/backend/internal/invocation/domain"
	providerplugin "github.com/orz-i/Mender/backend/internal/providerplugin"
)

func TestWorkspacePluginDiscoversStableFileTools(t *testing.T) {
	t.Parallel()

	instance := openWorkspaceTestInstance(t, t.TempDir())
	tools, err := instance.DiscoverTools(context.Background(), catalogdomain.Provider{})
	if err != nil {
		t.Fatal(err)
	}
	keys := make([]string, 0, len(tools))
	for _, tool := range tools {
		keys = append(keys, tool.SourceKey)
	}
	if !slices.Equal(keys, []string{sourceKeyRead, sourceKeySearch, sourceKeyEdit, sourceKeyShell}) {
		t.Fatalf("SourceKeys = %v", keys)
	}
}

func TestWorkspaceReadIsBoundedAndRootContained(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(root, "notes.txt"),
		[]byte("one\ntwo\nthree\nfour\n"),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	instance := openWorkspaceTestInstance(t, root)

	outcome := executeWorkspaceTool(t, instance, sourceKeyRead, map[string]any{
		"path":   "notes.txt",
		"offset": 2,
		"limit":  2,
	})
	requireComplete(t, outcome)
	var result readOutput
	if err := json.Unmarshal(outcome.Content().Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Content != "two\nthree" || result.StartLine != 2 || result.EndLine != 3 || !result.Truncated {
		t.Fatalf("read output = %+v", result)
	}

	failed := executeWorkspaceTool(t, instance, sourceKeyRead, map[string]any{
		"path": "../outside.txt",
	})
	requireToolFailure(t, failed)
}

func TestWorkspaceSearchFilesAndContent(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "src", "nested"), 0o750); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"main.go":              "package main\n",
		"src/app.go":           "package app\nfunc GovernanceFactExtractor() {}\n",
		"src/nested/helper.go": "package nested\n// governancefactextractor\n",
		"src/readme.md":        "GovernanceFactExtractor docs\n",
	}
	for name, content := range files {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	instance := openWorkspaceTestInstance(t, root)

	filesOutcome := executeWorkspaceTool(t, instance, sourceKeySearch, map[string]any{
		"mode":    "files",
		"pattern": "**/*.go",
	})
	requireComplete(t, filesOutcome)
	var fileResult searchOutput
	if err := json.Unmarshal(filesOutcome.Content().Bytes(), &fileResult); err != nil {
		t.Fatal(err)
	}
	gotPaths := make([]string, 0, len(fileResult.Matches))
	for _, match := range fileResult.Matches {
		gotPaths = append(gotPaths, match.Path)
	}
	wantPaths := []string{"main.go", "src/app.go", "src/nested/helper.go"}
	if !slices.Equal(gotPaths, wantPaths) {
		t.Fatalf("file matches = %v, want %v", gotPaths, wantPaths)
	}

	contentOutcome := executeWorkspaceTool(t, instance, sourceKeySearch, map[string]any{
		"mode":    "content",
		"path":    "src",
		"query":   "governancefactextractor",
		"pattern": "**/*.go",
	})
	requireComplete(t, contentOutcome)
	var contentResult searchOutput
	if err := json.Unmarshal(contentOutcome.Content().Bytes(), &contentResult); err != nil {
		t.Fatal(err)
	}
	if len(contentResult.Matches) != 2 {
		t.Fatalf("content matches = %+v, want 2", contentResult.Matches)
	}
}

func TestWorkspaceEditCreateReplaceDeleteAndAmbiguity(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	instance := openWorkspaceTestInstance(t, root)

	create := executeWorkspaceTool(t, instance, sourceKeyEdit, map[string]any{
		"path":      "draft.txt",
		"operation": "create",
		"content":   "alpha beta beta\n",
	})
	requireComplete(t, create)

	ambiguous := executeWorkspaceTool(t, instance, sourceKeyEdit, map[string]any{
		"path":      "draft.txt",
		"operation": "replace",
		"edits": []map[string]any{
			{"oldText": "beta", "newText": "gamma"},
		},
	})
	requireToolFailure(t, ambiguous)
	data := readWorkspaceTestFile(t, root, "draft.txt")
	if string(data) != "alpha beta beta\n" {
		t.Fatalf("ambiguous edit mutated file: %q", data)
	}

	replace := executeWorkspaceTool(t, instance, sourceKeyEdit, map[string]any{
		"path":      "draft.txt",
		"operation": "replace",
		"edits": []map[string]any{
			{
				"oldText":              "beta",
				"newText":              "gamma",
				"expectedReplacements": 2,
			},
		},
	})
	requireComplete(t, replace)
	data = readWorkspaceTestFile(t, root, "draft.txt")
	if string(data) != "alpha gamma gamma\n" {
		t.Fatalf("replace content = %q", data)
	}

	deleteOutcome := executeWorkspaceTool(t, instance, sourceKeyEdit, map[string]any{
		"path":      "draft.txt",
		"operation": "delete",
	})
	requireComplete(t, deleteOutcome)
	if _, err := os.Stat(filepath.Join(root, "draft.txt")); !os.IsNotExist(err) {
		t.Fatalf("deleted file Stat() error = %v", err)
	}
}

func TestWorkspaceFileToolsRejectSymlinkTargets(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(outside, "secret.txt"), filepath.Join(root, "secret-link")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	instance := openWorkspaceTestInstance(t, root)

	requireToolFailure(t, executeWorkspaceTool(t, instance, sourceKeyRead, map[string]any{
		"path": "secret-link",
	}))
	requireToolFailure(t, executeWorkspaceTool(t, instance, sourceKeyEdit, map[string]any{
		"path":      "secret-link",
		"operation": "delete",
	}))
}

func openWorkspaceTestInstance(t *testing.T, root string) *Instance {
	t.Helper()
	raw, err := json.Marshal(Config{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	opened, err := New().Open(context.Background(), providerplugin.OpenRequest{
		ConfigVersion: ConfigVersion,
		Config:        raw,
	})
	if err != nil {
		t.Fatal(err)
	}
	instance, ok := opened.(*Instance)
	if !ok {
		t.Fatalf("Open() instance = %T", opened)
	}
	t.Cleanup(func() { _ = instance.Close() })
	return instance
}

func executeWorkspaceTool(
	t *testing.T,
	instance *Instance,
	sourceKey string,
	input any,
) invocationdomain.Outcome {
	t.Helper()
	raw, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	arguments, err := invocationdomain.NewJSONValue(raw)
	if err != nil {
		t.Fatal(err)
	}
	outcome, err := instance.Execute(
		context.Background(),
		invocationapplication.ExecutionRequest{
			Tool:      workspaceTestTool(t, sourceKey),
			Arguments: arguments,
		},
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf("Execute(%s) error = %v", sourceKey, err)
	}
	return outcome
}

func workspaceTestTool(t *testing.T, sourceKey string) catalogdomain.Tool {
	t.Helper()
	providerID, err := catalogdomain.ParseProviderID("prov_00000000000000000000000000000051")
	if err != nil {
		t.Fatal(err)
	}
	toolID, err := catalogdomain.ParseToolID("tool_00000000000000000000000000000051")
	if err != nil {
		t.Fatal(err)
	}
	schema, err := catalogdomain.NewSchema([]byte("{\"type\":\"object\"}"))
	if err != nil {
		t.Fatal(err)
	}
	tool, err := catalogdomain.NewTool(
		toolID,
		providerID,
		catalogdomain.ToolSpec{
			SourceKey:   sourceKey,
			Name:        sourceKey,
			InputSchema: schema,
		},
		time.Now(),
	)
	if err != nil {
		t.Fatal(err)
	}
	return tool
}

func mkdirAllTest(path string) error {
	return os.MkdirAll(path, 0o750)
}

func readWorkspaceTestFile(t *testing.T, rootPath, relative string) []byte {
	t.Helper()
	root, err := os.OpenRoot(rootPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = root.Close() }()
	data, err := root.ReadFile(filepath.FromSlash(relative))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func unmarshalOutcome(t *testing.T, outcome invocationdomain.Outcome, target any) {
	t.Helper()
	if err := json.Unmarshal(outcome.Content().Bytes(), target); err != nil {
		t.Fatal(err)
	}
}

func requireComplete(t *testing.T, outcome invocationdomain.Outcome) {
	t.Helper()
	if outcome.Kind() != invocationdomain.OutcomeKindComplete {
		t.Fatalf("outcome = %s message=%q", outcome.Kind(), outcome.Message())
	}
}

func requireToolFailure(t *testing.T, outcome invocationdomain.Outcome) {
	t.Helper()
	if outcome.Kind() != invocationdomain.OutcomeKindFailed ||
		outcome.FailureSource() != invocationdomain.FailureSourceTool ||
		outcome.FailureCode() != invocationdomain.FailureToolError {
		t.Fatalf(
			"outcome = %s/%s/%s message=%q",
			outcome.Kind(),
			outcome.FailureSource(),
			outcome.FailureCode(),
			outcome.Message(),
		)
	}
}
