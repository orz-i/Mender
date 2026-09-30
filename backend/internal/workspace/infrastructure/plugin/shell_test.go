package plugin

import (
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestWorkspaceShellRunsFromContainedCwdWithReducedEnvironment(t *testing.T) {
	root := t.TempDir()
	work := filepath.Join(root, "nested")
	if err := mkdirAllTest(work); err != nil {
		t.Fatal(err)
	}
	t.Setenv("MENDER_WORKSPACE_TEST_SECRET", "must-not-leak")

	instance := openWorkspaceTestInstance(t, root)
	outcome := executeWorkspaceTool(t, instance, sourceKeyShell, map[string]any{
		"command": shellPrintCwdAndSecretCommand(),
		"cwd":     "nested",
	})
	requireComplete(t, outcome)

	var result shellOutput
	unmarshalOutcome(t, outcome, &result)
	if result.ExitCode != 0 || result.TimedOut {
		t.Fatalf("shell result = %+v", result)
	}
	lines := strings.Split(result.Stdout, "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) == "" {
		t.Fatalf("stdout = %q, want cwd", result.Stdout)
	}
	gotCwd, err := filepath.EvalSymlinks(strings.TrimSpace(lines[0]))
	if err != nil {
		t.Fatal(err)
	}
	wantCwd, err := filepath.EvalSymlinks(work)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Clean(gotCwd) != filepath.Clean(wantCwd) {
		t.Fatalf("cwd = %q, want %q", gotCwd, wantCwd)
	}
	if strings.Contains(result.Stdout, "must-not-leak") {
		t.Fatalf("shell inherited non-allowlisted secret: %q", result.Stdout)
	}
}

func TestWorkspaceShellRejectsEscapingCwd(t *testing.T) {
	t.Parallel()

	instance := openWorkspaceTestInstance(t, t.TempDir())
	outcome := executeWorkspaceTool(t, instance, sourceKeyShell, map[string]any{
		"command": "echo unreachable",
		"cwd":     "../outside",
	})
	requireToolFailure(t, outcome)
}

func TestWorkspaceShellReportsNonZeroExitWithoutTurningItIntoRuntimeFailure(t *testing.T) {
	t.Parallel()

	instance := openWorkspaceTestInstance(t, t.TempDir())
	outcome := executeWorkspaceTool(t, instance, sourceKeyShell, map[string]any{
		"command": shellExitCommand(7),
	})
	requireComplete(t, outcome)

	var result shellOutput
	unmarshalOutcome(t, outcome, &result)
	if result.ExitCode != 7 || result.TimedOut {
		t.Fatalf("shell result = %+v, want exitCode=7 timedOut=false", result)
	}
}

func TestWorkspaceShellAppliesToolTimeout(t *testing.T) {
	t.Parallel()

	instance := openWorkspaceTestInstance(t, t.TempDir())
	outcome := executeWorkspaceTool(t, instance, sourceKeyShell, map[string]any{
		"command":   shellSleepCommand(),
		"timeoutMs": 50,
	})
	requireComplete(t, outcome)

	var result shellOutput
	unmarshalOutcome(t, outcome, &result)
	if !result.TimedOut {
		t.Fatalf("shell result = %+v, want timedOut", result)
	}
}

func TestBoundedCaptureTruncatesWithoutShortWrites(t *testing.T) {
	t.Parallel()

	capture := newBoundedCapture(5)
	if n, err := capture.Write([]byte("abc")); err != nil || n != 3 {
		t.Fatalf("first Write() = %d, %v", n, err)
	}
	if n, err := capture.Write([]byte("defgh")); err != nil || n != 5 {
		t.Fatalf("second Write() = %d, %v", n, err)
	}
	if got := string(capture.Bytes()); got != "abcde" {
		t.Fatalf("Bytes() = %q, want %q", got, "abcde")
	}
	if !capture.Truncated() {
		t.Fatal("Truncated() = false, want true")
	}
}

func shellPrintCwdAndSecretCommand() string {
	if runtime.GOOS == "windows" {
		return "cd && echo %MENDER_WORKSPACE_TEST_SECRET%"
	}
	return "pwd; printf '\\n%s\\n' \"$MENDER_WORKSPACE_TEST_SECRET\""
}

func shellExitCommand(code int) string {
	if runtime.GOOS == "windows" {
		return "exit /b " + itoaTest(code)
	}
	return "exit " + itoaTest(code)
}

func shellSleepCommand() string {
	if runtime.GOOS == "windows" {
		return "ping 127.0.0.1 -n 3 >NUL"
	}
	return "sleep 1"
}

func itoaTest(value int) string {
	switch value {
	case 7:
		return "7"
	default:
		panic("unsupported test integer")
	}
}
