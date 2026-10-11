package processisolation

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"time"

	runtimegovernancedomain "github.com/orz-i/Mender/backend/internal/runtimegovernance/domain"
)

type BackendInfo struct {
	Name                    string
	RestrictedHostAvailable bool
}

type CommandSpec struct {
	Executable       string
	Arguments        []string
	WorkingDirectory string
	PermittedRoot    string
	RootWritable     bool
	Environment      []string
}

type Launcher struct {
	goos        string
	backendPath string
	homeDir     string
	info        BackendInfo
}

func New() *Launcher {
	home, _ := os.UserHomeDir()
	if strings.TrimSpace(home) == "" {
		home = string(filepath.Separator) + "nonexistent"
	}
	home = canonicalExistingPath(home)

	goos := runtime.GOOS
	var (
		path string
		name string
	)
	switch goos {
	case "darwin":
		name = "seatbelt"
		if _, err := os.Stat("/usr/bin/sandbox-exec"); err == nil {
			path = "/usr/bin/sandbox-exec"
		}
	case "linux":
		name = "bubblewrap"
		path, _ = exec.LookPath("bwrap")
	default:
		name = "none"
	}
	if path == "" || !probeBackend(goos, path, home) {
		path = ""
		name = "none"
	}
	return newLauncher(goos, path, home, name)
}

func probeBackend(goos, path, homeDir string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	switch goos {
	case "darwin":
		return probeDarwinBackend(ctx, path, homeDir)
	case "linux":
		cmd := exec.CommandContext( // #nosec G204 -- detected host backend.
			ctx,
			path,
			"--ro-bind", "/", "/",
			"--",
			"/usr/bin/true",
		)
		return cmd.Run() == nil
	default:
		return false
	}
}

func probeDarwinBackend(ctx context.Context, path, homeDir string) bool {
	base, err := os.MkdirTemp(homeDir, ".mender-seatbelt-probe-")
	if err != nil {
		return false
	}
	defer func() { _ = os.RemoveAll(base) }()

	root := filepath.Join(base, "workspace")
	other := filepath.Join(base, "other")
	if err := os.MkdirAll(root, 0o700); err != nil {
		return false
	}
	if err := os.MkdirAll(other, 0o700); err != nil {
		return false
	}
	secret := filepath.Join(other, "secret")
	if err := os.WriteFile(secret, []byte("secret"), 0o600); err != nil {
		return false
	}

	testLauncher := &Launcher{
		goos:        "darwin",
		backendPath: path,
		homeDir:     canonicalExistingPath(homeDir),
		info: BackendInfo{
			Name:                    "seatbelt",
			RestrictedHostAvailable: true,
		},
	}
	cmd, err := testLauncher.Command(
		ctx,
		runtimegovernancedomain.IsolationRestrictedHost,
		CommandSpec{
			Executable: "/bin/sh",
			Arguments: []string{
				"-c",
				"echo ok > inside; test \"$(cat inside)\" = ok; if cat \"$1\" >/dev/null 2>&1; then exit 77; fi",
				"sh",
				secret,
			},
			WorkingDirectory: root,
			PermittedRoot:    root,
			RootWritable:     true,
			Environment:      []string{"PATH=/usr/bin:/bin", "HOME=" + homeDir, "TMPDIR=" + os.TempDir()},
		},
	)
	if err != nil {
		return false
	}
	if err := cmd.Run(); err != nil {
		return false
	}
	rootHandle, err := os.OpenRoot(root)
	if err != nil {
		return false
	}
	defer func() { _ = rootHandle.Close() }()
	contents, err := rootHandle.ReadFile("inside")
	return err == nil && string(contents) == "ok\n"
}

func newLauncher(goos, backendPath, homeDir, name string) *Launcher {
	available := strings.TrimSpace(backendPath) != ""
	if !available {
		name = "none"
	}
	return &Launcher{
		goos:        goos,
		backendPath: backendPath,
		homeDir:     filepath.Clean(homeDir),
		info: BackendInfo{
			Name:                    name,
			RestrictedHostAvailable: available,
		},
	}
}

func (l *Launcher) Info() BackendInfo {
	if l == nil {
		return BackendInfo{Name: "none"}
	}
	return l.info
}

func (l *Launcher) Command(
	ctx context.Context,
	mode runtimegovernancedomain.IsolationMode,
	spec CommandSpec,
) (*exec.Cmd, error) {
	if err := mode.Validate(); err != nil {
		return nil, err
	}
	normalized, err := normalizeSpec(spec)
	if err != nil {
		return nil, err
	}

	var cmd *exec.Cmd
	switch mode {
	case runtimegovernancedomain.IsolationHost:
		cmd = exec.CommandContext( // #nosec G204 -- executable and arguments are host-owned specs.
			ctx,
			normalized.Executable,
			normalized.Arguments...,
		)
	case runtimegovernancedomain.IsolationRestrictedHost:
		if l == nil || !l.info.RestrictedHostAvailable {
			return nil, fmt.Errorf("restricted_host process isolation is unavailable")
		}
		switch l.goos {
		case "darwin":
			cmd, err = l.darwinCommand(ctx, normalized)
			if err != nil {
				return nil, err
			}
		case "linux":
			cmd = l.linuxCommand(ctx, normalized)
		default:
			return nil, fmt.Errorf("restricted_host process isolation is unsupported on %s", l.goos)
		}
	default:
		return nil, fmt.Errorf("unsupported process isolation mode %q", mode)
	}
	cmd.Dir = normalized.WorkingDirectory
	if normalized.Environment != nil {
		cmd.Env = slices.Clone(normalized.Environment)
	}
	configureCommand(cmd)
	return cmd, nil
}

func normalizeSpec(spec CommandSpec) (CommandSpec, error) {
	spec.Executable = filepath.Clean(strings.TrimSpace(spec.Executable))
	spec.WorkingDirectory = filepath.Clean(strings.TrimSpace(spec.WorkingDirectory))
	spec.PermittedRoot = filepath.Clean(strings.TrimSpace(spec.PermittedRoot))
	if !filepath.IsAbs(spec.Executable) {
		return CommandSpec{}, fmt.Errorf("process executable must be absolute")
	}
	if !filepath.IsAbs(spec.WorkingDirectory) {
		return CommandSpec{}, fmt.Errorf("process working directory must be absolute")
	}
	if !filepath.IsAbs(spec.PermittedRoot) {
		return CommandSpec{}, fmt.Errorf("process permitted root must be absolute")
	}

	root, err := canonicalPath(spec.PermittedRoot)
	if err != nil {
		return CommandSpec{}, fmt.Errorf("process permitted root: %w", err)
	}
	working, err := canonicalPath(spec.WorkingDirectory)
	if err != nil {
		return CommandSpec{}, fmt.Errorf("process working directory: %w", err)
	}
	inside, err := pathWithin(root, working)
	if err != nil {
		return CommandSpec{}, err
	}
	if !inside {
		return CommandSpec{}, fmt.Errorf("process working directory must be inside permitted root")
	}
	spec.PermittedRoot = root
	spec.WorkingDirectory = working
	spec.Arguments = slices.Clone(spec.Arguments)
	spec.Environment = slices.Clone(spec.Environment)
	return spec, nil
}

func canonicalPath(value string) (string, error) {
	absolute, err := filepath.Abs(value)
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return "", err
	}
	return filepath.Clean(resolved), nil
}

func canonicalExistingPath(value string) string {
	resolved, err := canonicalPath(value)
	if err != nil {
		return filepath.Clean(value)
	}
	return resolved
}

func pathWithin(root, candidate string) (bool, error) {
	relative, err := filepath.Rel(root, candidate)
	if err != nil {
		return false, fmt.Errorf("compare process root: %w", err)
	}
	return relative == "." ||
		(relative != ".." &&
			!strings.HasPrefix(relative, ".."+string(filepath.Separator))), nil
}

func (l *Launcher) darwinCommand(
	ctx context.Context,
	spec CommandSpec,
) (*exec.Cmd, error) {
	profile, err := l.darwinProfile(spec)
	if err != nil {
		return nil, err
	}
	args := []string{"-p", profile, spec.Executable}
	args = append(args, spec.Arguments...)
	return exec.CommandContext(ctx, l.backendPath, args...), nil // #nosec G204 -- backend path is fixed/detected by the host.
}

func (l *Launcher) darwinProfile(spec CommandSpec) (string, error) {
	var builder strings.Builder
	builder.WriteString("(version 1)\n")
	builder.WriteString("(deny default)\n")
	builder.WriteString("(allow process-exec)\n")
	builder.WriteString("(allow process-fork)\n")
	builder.WriteString("(allow signal (target same-sandbox))\n")

	home := canonicalExistingPath(l.homeDir)
	rootInsideHome, err := pathWithin(home, spec.PermittedRoot)
	if err != nil {
		return "", fmt.Errorf("build Seatbelt home boundary: %w", err)
	}

	// Keep system/runtime reads available while excluding the Runtime owner's
	// HOME inside the allow filter. The permitted root is then reopened with a
	// separate allow. This avoids explicit deny precedence and prevents newly
	// created HOME siblings from becoming readable after policy construction.
	if spec.PermittedRoot == home {
		builder.WriteString("(allow file-read*)\n")
	} else {
		builder.WriteString("(allow file-read*\n")
		builder.WriteString("  (require-all\n")
		builder.WriteString("    (subpath \"/\")\n")
		builder.WriteString("    (require-not (literal ")
		builder.WriteString(sbplString(home))
		builder.WriteString("))\n")
		builder.WriteString("    (require-not (subpath ")
		builder.WriteString(sbplString(home))
		builder.WriteString("))))\n")

		builder.WriteString("(allow file-read* (literal ")
		builder.WriteString(sbplString(spec.PermittedRoot))
		builder.WriteString(") (subpath ")
		builder.WriteString(sbplString(spec.PermittedRoot))
		builder.WriteString("))\n")

		if rootInsideHome {
			for _, ancestor := range homeAncestors(home, spec.PermittedRoot) {
				builder.WriteString("(allow file-read-metadata (literal ")
				builder.WriteString(sbplString(ancestor))
				builder.WriteString("))\n")
			}
		}
	}

	for _, tempRoot := range writableRuntimeRoots() {
		builder.WriteString("(allow file-read* (literal ")
		builder.WriteString(sbplString(tempRoot))
		builder.WriteString(") (subpath ")
		builder.WriteString(sbplString(tempRoot))
		builder.WriteString("))\n")
	}

	builder.WriteString("(allow sysctl-read)\n")
	// Go uses macOS Security.framework/trustd when verifying system-trusted TLS
	// certificates. Permit only this per-user trust agent: unrestricted Mach
	// lookups would unnecessarily weaken the deny-by-default Seatbelt profile.
	builder.WriteString("(allow mach-lookup (global-name \"com.apple.trustd.agent\"))\n")
	builder.WriteString("(allow network-outbound)\n")
	builder.WriteString("(allow network-inbound)\n")

	if spec.RootWritable {
		builder.WriteString("(allow file-write* (literal ")
		builder.WriteString(sbplString(spec.PermittedRoot))
		builder.WriteString(") (subpath ")
		builder.WriteString(sbplString(spec.PermittedRoot))
		builder.WriteString("))\n")
	}
	for _, tempRoot := range writableRuntimeRoots() {
		rootInsideTemp, compareErr := pathWithin(tempRoot, spec.PermittedRoot)
		if compareErr != nil {
			return "", fmt.Errorf("build Seatbelt temp boundary: %w", compareErr)
		}
		if !spec.RootWritable && rootInsideTemp {
			builder.WriteString("(allow file-write*\n  (require-all\n    (subpath ")
			builder.WriteString(sbplString(tempRoot))
			builder.WriteString(")\n    (require-not (literal ")
			builder.WriteString(sbplString(spec.PermittedRoot))
			builder.WriteString("))\n    (require-not (subpath ")
			builder.WriteString(sbplString(spec.PermittedRoot))
			builder.WriteString("))))\n")
			continue
		}
		builder.WriteString("(allow file-write* (literal ")
		builder.WriteString(sbplString(tempRoot))
		builder.WriteString(") (subpath ")
		builder.WriteString(sbplString(tempRoot))
		builder.WriteString("))\n")
	}
	for _, device := range []string{"/dev/null", "/dev/tty"} {
		builder.WriteString("(allow file-write* (literal ")
		builder.WriteString(sbplString(device))
		builder.WriteString("))\n")
	}
	builder.WriteString("(allow file-write* (subpath \"/dev/fd\"))\n")
	return builder.String(), nil
}

func homeAncestors(home, permittedRoot string) []string {
	if home == permittedRoot {
		return nil
	}
	relative, err := filepath.Rel(home, permittedRoot)
	if err != nil || relative == "." || strings.HasPrefix(relative, "..") {
		return nil
	}
	parts := strings.Split(relative, string(filepath.Separator))
	current := home
	result := []string{home}
	for _, part := range parts[:len(parts)-1] {
		current = filepath.Join(current, part)
		result = append(result, current)
	}
	return result
}

func writableRuntimeRoots() []string {
	values := []string{os.TempDir(), "/private/tmp", "/tmp"}
	result := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		canonical := canonicalExistingPath(value)
		if canonical == "" {
			continue
		}
		if _, ok := seen[canonical]; ok {
			continue
		}
		seen[canonical] = struct{}{}
		result = append(result, canonical)
	}
	return result
}

func sbplString(value string) string {
	replacer := strings.NewReplacer(
		"\\", "\\\\",
		"\"", "\\\"",
		"\n", "\\n",
		"\r", "\\r",
		"\t", "\\t",
	)
	return "\"" + replacer.Replace(value) + "\""
}

func (l *Launcher) linuxCommand(ctx context.Context, spec CommandSpec) *exec.Cmd {
	args := []string{
		"--die-with-parent",
		"--new-session",
		"--unshare-pid",
		"--tmpfs", "/",
		"--proc", "/proc",
		"--dev", "/dev",
	}
	for _, path := range []string{"/usr", "/bin", "/sbin", "/lib", "/lib64", "/etc"} {
		if info, err := os.Stat(path); err == nil && info.IsDir() {
			args = append(args, "--ro-bind", path, path)
		}
	}
	// Establish the private temporary filesystem before binding an explicit
	// permitted root that may itself live below /tmp.
	args = append(args, "--tmpfs", "/tmp")
	args = append(args, directoryScaffold(spec.PermittedRoot)...)
	if spec.RootWritable {
		args = append(args, "--bind", spec.PermittedRoot, spec.PermittedRoot)
	} else {
		args = append(args, "--ro-bind", spec.PermittedRoot, spec.PermittedRoot)
	}
	args = append(
		args,
		"--chdir", spec.WorkingDirectory,
		"--",
		spec.Executable,
	)
	args = append(args, spec.Arguments...)
	return exec.CommandContext(ctx, l.backendPath, args...) // #nosec G204 -- backend path is detected by the host.
}

func directoryScaffold(root string) []string {
	clean := filepath.Clean(root)
	var paths []string
	for parent := filepath.Dir(clean); parent != string(filepath.Separator) && parent != "."; parent = filepath.Dir(parent) {
		paths = append(paths, parent)
	}
	slices.Reverse(paths)
	args := make([]string, 0, len(paths)*2)
	for _, path := range paths {
		if path == "/tmp" {
			continue
		}
		args = append(args, "--dir", path)
	}
	return args
}
