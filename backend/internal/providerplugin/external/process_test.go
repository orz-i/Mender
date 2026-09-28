package external

import (
	"context"
	"crypto/sha256"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	providerplugin "github.com/orz-i/Mender/backend/internal/providerplugin"
	installationapplication "github.com/orz-i/Mender/backend/internal/providerplugin/installation/application"
	installationdomain "github.com/orz-i/Mender/backend/internal/providerplugin/installation/domain"
	"github.com/orz-i/Mender/backend/internal/providerplugin/manifest"
	pluginprotocol "github.com/orz-i/Mender/sdk/providerplugin/go/protocol"
	providerv1 "github.com/orz-i/Mender/sdk/providerplugin/go/protocol/v1"
	plugintransport "github.com/orz-i/Mender/sdk/providerplugin/go/transport"
)

func TestMain(m *testing.M) {
	if os.Getenv(plugintransport.Handshake.MagicCookieKey) == plugintransport.Handshake.MagicCookieValue {
		executable, _ := os.Executable()
		mode := filepath.Base(executable)
		if strings.Contains(mode, "slow") {
			time.Sleep(750 * time.Millisecond)
		}
		if strings.Contains(mode, "crash") {
			os.Exit(23)
		}
		server := &helperPluginServer{mode: mode}
		if err := plugintransport.Serve(pluginprotocol.SupportedVersions(), server); err != nil {
			os.Exit(24)
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func TestSupervisorEnsureReusesHealthyProcess(t *testing.T) {
	t.Parallel()

	installation, resolved := testExternalInstallation(t, "good")
	resolver := &fakePackageResolver{resolved: resolved}
	supervisor, err := NewSupervisor(resolver, Options{
		StartTimeout:      10 * time.Second,
		DescriptorTimeout: 5 * time.Second,
		MaxStderrBytes:    4096,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = supervisor.CloseAll() })

	first, err := supervisor.Ensure(context.Background(), installation)
	if err != nil {
		t.Fatalf("Ensure(first) error = %v", err)
	}
	second, err := supervisor.Ensure(context.Background(), installation)
	if err != nil {
		t.Fatalf("Ensure(second) error = %v", err)
	}
	if first != second {
		t.Fatal("Ensure() did not reuse the healthy installation process")
	}
	if first.ProtocolVersion() != pluginprotocol.Version1 {
		t.Fatalf("ProtocolVersion() = %d", first.ProtocolVersion())
	}
	if resolver.CallCount() != 1 {
		t.Fatalf("Resolve() calls = %d, want 1", resolver.CallCount())
	}

	if err := supervisor.CloseInstallation(installation.ID()); err != nil {
		t.Fatalf("CloseInstallation() error = %v", err)
	}
	if !first.Exited() {
		t.Fatal("CloseInstallation() left process running")
	}
}

func TestSupervisorConcurrentEnsureStartsOneProcess(t *testing.T) {
	t.Parallel()

	installation, resolved := testExternalInstallation(t, "good")
	resolver := &fakePackageResolver{resolved: resolved}
	supervisor, err := NewSupervisor(resolver, DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = supervisor.CloseAll() })

	const count = 8
	results := make(chan *Process, count)
	errors := make(chan error, count)
	var wg sync.WaitGroup
	for range count {
		wg.Add(1)
		go func() {
			defer wg.Done()
			process, ensureErr := supervisor.Ensure(context.Background(), installation)
			results <- process
			errors <- ensureErr
		}()
	}
	wg.Wait()
	close(results)
	close(errors)

	for ensureErr := range errors {
		if ensureErr != nil {
			t.Fatalf("Ensure() error = %v", ensureErr)
		}
	}
	var first *Process
	for process := range results {
		if first == nil {
			first = process
			continue
		}
		if process != first {
			t.Fatal("concurrent Ensure() returned different processes")
		}
	}
	if resolver.CallCount() != 1 {
		t.Fatalf("Resolve() calls = %d, want 1", resolver.CallCount())
	}
}

func TestSupervisorRejectsExecutableChecksumMismatch(t *testing.T) {
	t.Parallel()

	installation, resolved := testExternalInstallation(t, "good")
	resolved.ExecutableChecksum = make([]byte, sha256.Size)
	resolver := &fakePackageResolver{resolved: resolved}
	supervisor, err := NewSupervisor(resolver, DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = supervisor.CloseAll() })

	if _, err := supervisor.Ensure(context.Background(), installation); err == nil ||
		!strings.Contains(strings.ToLower(err.Error()), "checksum") {
		t.Fatalf("Ensure() error = %v", err)
	}
}

func TestSupervisorReportsPluginCrashDuringStartup(t *testing.T) {
	t.Parallel()

	installation, resolved := testExternalInstallation(t, "crash")
	resolver := &fakePackageResolver{resolved: resolved}
	supervisor, err := NewSupervisor(resolver, Options{
		StartTimeout:      time.Second,
		DescriptorTimeout: time.Second,
		MaxStderrBytes:    1024,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = supervisor.CloseAll() })

	if _, err := supervisor.Ensure(context.Background(), installation); err == nil ||
		!strings.Contains(err.Error(), "start") {
		t.Fatalf("Ensure() error = %v", err)
	}
}

func TestSupervisorRejectsDescriptorIdentityMismatch(t *testing.T) {
	t.Parallel()

	installation, resolved := testExternalInstallation(t, "mismatch")
	resolver := &fakePackageResolver{resolved: resolved}
	supervisor, err := NewSupervisor(resolver, DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = supervisor.CloseAll() })

	_, err = supervisor.Ensure(context.Background(), installation)
	if err == nil || !strings.Contains(err.Error(), "descriptor plugin id mismatch") {
		t.Fatalf("Ensure() error = %v", err)
	}
}

func TestSupervisorRejectsDisabledInstallationBeforeLaunch(t *testing.T) {
	t.Parallel()

	installation, resolved := testExternalInstallation(t, "good")
	installation, err := installation.WithStatus(installationdomain.StatusDisabled)
	if err != nil {
		t.Fatal(err)
	}
	resolver := &fakePackageResolver{resolved: resolved}
	supervisor, err := NewSupervisor(resolver, DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = supervisor.CloseAll() })

	if _, err := supervisor.Ensure(context.Background(), installation); err == nil ||
		!strings.Contains(err.Error(), "is disabled") {
		t.Fatalf("Ensure() error = %v", err)
	}
	if resolver.CallCount() != 0 {
		t.Fatalf("Resolve() calls = %d, want 0", resolver.CallCount())
	}
}

func TestSupervisorStartTimeout(t *testing.T) {
	t.Parallel()

	installation, resolved := testExternalInstallation(t, "slow")
	resolver := &fakePackageResolver{resolved: resolved}
	supervisor, err := NewSupervisor(resolver, Options{
		StartTimeout:      100 * time.Millisecond,
		DescriptorTimeout: time.Second,
		MaxStderrBytes:    1024,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = supervisor.CloseAll() })

	started := time.Now()
	if _, err := supervisor.Ensure(context.Background(), installation); err == nil {
		t.Fatal("Ensure() error = nil")
	}
	if elapsed := time.Since(started); elapsed > 3*time.Second {
		t.Fatalf("Ensure() timeout took %s", elapsed)
	}
}

func TestSupervisorCloseAllPreventsFurtherEnsure(t *testing.T) {
	t.Parallel()

	installation, resolved := testExternalInstallation(t, "good")
	resolver := &fakePackageResolver{resolved: resolved}
	supervisor, err := NewSupervisor(resolver, DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}

	process, err := supervisor.Ensure(context.Background(), installation)
	if err != nil {
		t.Fatal(err)
	}
	if err := supervisor.CloseAll(); err != nil {
		t.Fatal(err)
	}
	if !process.Exited() {
		t.Fatal("CloseAll() left process running")
	}
	if _, err := supervisor.Ensure(context.Background(), installation); err == nil ||
		!strings.Contains(err.Error(), "supervisor is closed") {
		t.Fatalf("Ensure(after close) error = %v", err)
	}
}

func TestBoundedWriterKeepsTail(t *testing.T) {
	t.Parallel()

	writer := newBoundedWriter(5)
	if _, err := writer.Write([]byte("abc")); err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Write([]byte("defg")); err != nil {
		t.Fatal(err)
	}
	if got := string(writer.bytes()); got != "cdefg" {
		t.Fatalf("bytes() = %q, want %q", got, "cdefg")
	}
	if _, err := writer.Write([]byte("123456")); err != nil {
		t.Fatal(err)
	}
	if got := string(writer.bytes()); got != "23456" {
		t.Fatalf("bytes() = %q, want %q", got, "23456")
	}
}

type fakePackageResolver struct {
	mu       sync.Mutex
	resolved installationapplication.ResolvedPackage
	err      error
	calls    int
}

func (r *fakePackageResolver) Resolve(
	_ context.Context,
	_ installationdomain.Installation,
) (installationapplication.ResolvedPackage, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls++
	return r.resolved, r.err
}

func (r *fakePackageResolver) CallCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.calls
}

type helperPluginServer struct {
	providerv1.UnimplementedProviderPluginServiceServer
	mode string
}

func (s *helperPluginServer) GetDescriptor(
	_ context.Context,
	_ *providerv1.GetDescriptorRequest,
) (*providerv1.GetDescriptorResponse, error) {
	pluginID := "dev.example.echo"
	if strings.Contains(s.mode, "mismatch") {
		pluginID = "dev.example.other"
	}
	return &providerv1.GetDescriptorResponse{
		Descriptor_: &providerv1.Descriptor{
			PluginId:        pluginID,
			PackageVersion:  "1.0.0",
			DisplayName:     "Example Echo",
			Description:     "Test Provider plugin",
			ProtocolVersion: pluginprotocol.Version1,
			ConfigVersion:   1,
		},
	}, nil
}

func testExternalInstallation(
	t *testing.T,
	mode string,
) (installationdomain.Installation, installationapplication.ResolvedPackage) {
	t.Helper()

	executable := copyTestExecutable(t, mode)
	content, err := os.ReadFile(executable) // #nosec G304 -- test helper path is created by copyTestExecutable.
	if err != nil {
		t.Fatal(err)
	}
	checksum := sha256.Sum256(content)

	pluginID, err := providerplugin.ParseID("dev.example.echo")
	if err != nil {
		t.Fatal(err)
	}
	id, err := installationdomain.ParseInstallationID(
		"pinst_0123456789abcdef0123456789abcdef",
	)
	if err != nil {
		t.Fatal(err)
	}
	entrypoint := "bin/plugin"
	if runtime.GOOS == "windows" {
		entrypoint += ".exe"
	}
	manifestJSON := []byte(`{
		"manifestVersion": 1,
		"pluginId": "dev.example.echo",
		"version": "1.0.0",
		"protocolVersions": [1],
		"displayName": "Example Echo",
		"description": "Test Provider plugin",
		"entrypoints": {"` + runtime.GOOS + `/` + runtime.GOARCH + `": "` + entrypoint + `"}
	}`)
	installation, err := installationdomain.NewInstallation(
		id,
		pluginID,
		"1.0.0",
		installationdomain.SourceLocal,
		filepath.Dir(executable),
		"sha256:"+strings.Repeat("a", 64),
		manifestJSON,
		installationdomain.StatusEnabled,
		time.Date(2026, 9, 28, 4, 0, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatal(err)
	}
	return installation, installationapplication.ResolvedPackage{
		Manifest:           mustParseManifest(t, manifestJSON),
		EntrypointPath:     executable,
		WorkingDirectory:   filepath.Dir(executable),
		ExecutableChecksum: checksum[:],
	}
}

func mustParseManifest(t *testing.T, raw []byte) manifest.Manifest {
	t.Helper()
	parsed, err := manifest.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	return parsed
}

func copyTestExecutable(t *testing.T, mode string) string {
	t.Helper()

	source, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	sourceFile, err := os.Open(source) // #nosec G304 -- source is the current Go test executable.
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = sourceFile.Close() }()

	name := "plugin-" + mode
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	destination := filepath.Join(t.TempDir(), name)
	destinationFile, err := os.OpenFile(destination, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o700) // #nosec G304,G302 -- test plugin must be executable and destination is under t.TempDir.
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.Copy(destinationFile, sourceFile); err != nil {
		_ = destinationFile.Close()
		t.Fatal(err)
	}
	if err := destinationFile.Close(); err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" {
		if err := os.Chmod(destination, 0o700); err != nil { // #nosec G302 -- test plugin must be executable.
			t.Fatal(err)
		}
	}
	return destination
}
