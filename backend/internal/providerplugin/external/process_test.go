package external

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
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
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestMain(m *testing.M) {
	if os.Getenv(plugintransport.Handshake.MagicCookieKey) == plugintransport.Handshake.MagicCookieValue {
		executable, _ := os.Executable()
		mode := filepath.Base(executable)
		if strings.Contains(mode, "slow") {
			time.Sleep(750 * time.Millisecond)
		}
		if strings.Contains(mode, "startupcrash") {
			os.Exit(23)
		}
		if strings.Contains(mode, "badhandshake") {
			_, _ = fmt.Fprintln(os.Stdout, "not-a-mender-plugin-handshake")
			os.Exit(25)
		}
		server := &helperPluginServer{
			mode:     mode,
			sessions: make(map[string]helperSession),
		}
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

	installation, resolved := testExternalInstallation(t, "startupcrash")
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

	installation, resolved := testExternalInstallation(t, "idmismatch")
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

func TestSupervisorRejectsInvalidHandshake(t *testing.T) {
	t.Parallel()

	installation, resolved := testExternalInstallation(t, "badhandshake")
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

	if _, err := supervisor.Ensure(context.Background(), installation); err == nil {
		t.Fatal("Ensure() error = nil")
	}
}

func TestSupervisorRejectsPackageVersionMismatch(t *testing.T) {
	t.Parallel()

	installation, resolved := testExternalInstallation(t, "versionmismatch")
	resolver := &fakePackageResolver{resolved: resolved}
	supervisor, err := NewSupervisor(resolver, DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = supervisor.CloseAll() })

	_, err = supervisor.Ensure(context.Background(), installation)
	if err == nil || !strings.Contains(err.Error(), "package version mismatch") {
		t.Fatalf("Ensure() error = %v", err)
	}
}

func TestSupervisorRejectsUnsupportedProtocolBeforeLaunch(t *testing.T) {
	t.Parallel()

	installation, resolved := testExternalInstallation(t, "good")
	resolved.Manifest.ProtocolVersions = []uint32{99}
	resolver := &fakePackageResolver{resolved: resolved}
	supervisor, err := NewSupervisor(resolver, DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = supervisor.CloseAll() })

	if _, err := supervisor.Ensure(context.Background(), installation); err == nil ||
		!strings.Contains(err.Error(), "no mutually supported version") {
		t.Fatalf("Ensure() error = %v", err)
	}
	if resolver.CallCount() != 1 {
		t.Fatalf("Resolve() calls = %d, want 1", resolver.CallCount())
	}
}

func TestSupervisorRejectsOversizedDescriptorMessage(t *testing.T) {
	t.Parallel()

	installation, resolved := testExternalInstallation(t, "oversizeddescriptor")
	resolver := &fakePackageResolver{resolved: resolved}
	supervisor, err := NewSupervisor(resolver, DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = supervisor.CloseAll() })

	if _, err := supervisor.Ensure(context.Background(), installation); err == nil {
		t.Fatal("Ensure() error = nil")
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
	mode     string
	mu       sync.Mutex
	sessions map[string]helperSession
}

type helperSession struct {
	credentialRef string
}

func (s *helperPluginServer) GetDescriptor(
	_ context.Context,
	_ *providerv1.GetDescriptorRequest,
) (*providerv1.GetDescriptorResponse, error) {
	pluginID := "dev.example.echo"
	if strings.Contains(s.mode, "idmismatch") {
		pluginID = "dev.example.other"
	}
	packageVersion := "1.0.0"
	if strings.Contains(s.mode, "versionmismatch") ||
		strings.Contains(s.mode, "version2") {
		packageVersion = "2.0.0"
	}
	description := "Test Provider plugin"
	if strings.Contains(s.mode, "oversizeddescriptor") {
		description = strings.Repeat("x", plugintransport.MaxMessageBytes+1024)
	}
	configSchema := []byte(`{"type":"object"}`)
	if strings.Contains(s.mode, "invalidschema") {
		configSchema = []byte("{")
	}
	return &providerv1.GetDescriptorResponse{
		Descriptor_: &providerv1.Descriptor{
			PluginId:         pluginID,
			PackageVersion:   packageVersion,
			DisplayName:      "Example Echo",
			Description:      description,
			ProtocolVersion:  pluginprotocol.Version1,
			ConfigVersion:    1,
			ConfigSchemaJson: configSchema,
		},
	}, nil
}

func (s *helperPluginServer) ValidateConfig(
	ctx context.Context,
	request *providerv1.ValidateConfigRequest,
) (*providerv1.ValidateConfigResponse, error) {
	if strings.Contains(s.mode, "hangvalidate") {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	if request.GetConfigVersion() != 1 {
		return &providerv1.ValidateConfigResponse{
			Valid: false,
			Diagnostics: []*providerv1.Diagnostic{{
				Severity: providerv1.Diagnostic_SEVERITY_ERROR,
				Path:     "configVersion",
				Code:     "unsupported",
				Message:  "unsupported config version",
			}},
		}, nil
	}
	var config map[string]any
	if err := json.Unmarshal(request.GetConfigJson(), &config); err != nil {
		return &providerv1.ValidateConfigResponse{
			Valid: false,
			Diagnostics: []*providerv1.Diagnostic{{
				Severity: providerv1.Diagnostic_SEVERITY_ERROR,
				Path:     "$",
				Code:     "invalid_json",
				Message:  "invalid JSON",
			}},
		}, nil
	}
	if invalid, _ := config["invalid"].(bool); invalid {
		return &providerv1.ValidateConfigResponse{
			Valid: false,
			Diagnostics: []*providerv1.Diagnostic{{
				Severity: providerv1.Diagnostic_SEVERITY_ERROR,
				Path:     "invalid",
				Code:     "invalid",
				Message:  "config was rejected",
			}},
		}, nil
	}
	return &providerv1.ValidateConfigResponse{Valid: true}, nil
}

func (s *helperPluginServer) InspectConfig(
	ctx context.Context,
	request *providerv1.InspectConfigRequest,
) (*providerv1.InspectConfigResponse, error) {
	if strings.Contains(s.mode, "hanginspect") {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	var config map[string]any
	if err := json.Unmarshal(request.GetConfigJson(), &config); err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid config")
	}
	refs := make([]string, 0, 1)
	if strings.Contains(s.mode, "undeclaredinspect") {
		refs = append(refs, "credential://demo/missing")
	} else if ref, ok := config["credentialRef"].(string); ok && strings.TrimSpace(ref) != "" {
		refs = append(refs, ref)
	}
	return &providerv1.InspectConfigResponse{
		CredentialRefs:    refs,
		ConnectionSummary: "helper process",
	}, nil
}

func (s *helperPluginServer) Open(
	ctx context.Context,
	request *providerv1.OpenRequest,
) (*providerv1.OpenResponse, error) {
	if strings.Contains(s.mode, "hangopen") {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	if strings.TrimSpace(request.GetInstanceId()) == "" {
		return nil, status.Error(codes.InvalidArgument, "instance id is required")
	}
	var config map[string]any
	if err := json.Unmarshal(request.GetConfigJson(), &config); err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid config")
	}
	credentialRef, _ := config["credentialRef"].(string)
	if credentialRef != "" && !helperHasCredential(
		request.GetDiscoveryCredentials(),
		credentialRef,
	) {
		return nil, status.Error(codes.Unauthenticated, "missing discovery credential")
	}
	sessionID := "session:" + request.GetInstanceId()
	s.mu.Lock()
	s.sessions[sessionID] = helperSession{credentialRef: credentialRef}
	s.mu.Unlock()
	return &providerv1.OpenResponse{SessionId: sessionID}, nil
}

func (s *helperPluginServer) ListTools(
	ctx context.Context,
	request *providerv1.ListToolsRequest,
) (*providerv1.ListToolsResponse, error) {
	if strings.Contains(s.mode, "hanglist") {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	if strings.Contains(s.mode, "crashlist") {
		os.Exit(31)
	}
	session, ok := s.helperSession(request.GetSessionId())
	if !ok {
		return nil, status.Error(codes.NotFound, "session not found")
	}
	if session.credentialRef != "" && !helperHasCredential(
		request.GetDiscoveryCredentials(),
		session.credentialRef,
	) {
		return nil, status.Error(codes.Unauthenticated, "missing discovery credential")
	}
	credentialRefs := make([]string, 0, 1)
	if session.credentialRef != "" {
		credentialRefs = append(credentialRefs, session.credentialRef)
	}
	if strings.Contains(s.mode, "toolundeclared") {
		credentialRefs = append(credentialRefs, "credential://demo/other")
	}
	inputSchema := []byte(`{
		"type":"object",
		"additionalProperties":false,
		"properties":{
			"value":{},
			"mode":{"type":"string"}
		}
	}`)
	if strings.Contains(s.mode, "invalidtoolschema") {
		inputSchema = []byte("{")
	}
	tool := &providerv1.ToolDescriptor{
		SourceKey:        "echo",
		Name:             "echo",
		Title:            "Echo",
		Description:      "Echo one value through the helper plugin",
		InputSchemaJson:  inputSchema,
		OutputSchemaJson: []byte(`{"type":"object","properties":{"echo":{}}}`),
		Tags:             []string{"external", "test"},
		CredentialRefs:   credentialRefs,
	}
	tools := []*providerv1.ToolDescriptor{tool}
	if strings.Contains(s.mode, "duplicatetools") {
		tools = append(tools, tool)
	}
	return &providerv1.ListToolsResponse{Tools: tools}, nil
}

func (s *helperPluginServer) ExecuteTool(
	request *providerv1.ExecuteToolRequest,
	stream grpc.ServerStreamingServer[providerv1.ExecuteToolResponse],
) error {
	if strings.Contains(s.mode, "crashexec") {
		os.Exit(32)
	}
	if strings.Contains(s.mode, "slowexec") {
		select {
		case <-time.After(time.Second):
		case <-stream.Context().Done():
			return stream.Context().Err()
		}
	}
	session, ok := s.helperSession(request.GetSessionId())
	if !ok {
		return status.Error(codes.NotFound, "session not found")
	}
	if request.GetSourceKey() != "echo" {
		return status.Error(codes.NotFound, "tool not found")
	}
	if session.credentialRef != "" && !helperHasCredential(
		request.GetCredentials(),
		session.credentialRef,
	) {
		return status.Error(codes.Unauthenticated, "missing execution credential")
	}
	if err := stream.Send(&providerv1.ExecuteToolResponse{
		Event: &providerv1.ExecuteToolResponse_Progress{
			Progress: &providerv1.Progress{Message: "working"},
		},
	}); err != nil {
		return err
	}

	var arguments map[string]any
	if err := json.Unmarshal(request.GetArgumentsJson(), &arguments); err != nil {
		return status.Error(codes.InvalidArgument, "invalid arguments")
	}
	mode, _ := arguments["mode"].(string)
	switch mode {
	case "input":
		return stream.Send(&providerv1.ExecuteToolResponse{
			Event: &providerv1.ExecuteToolResponse_InputRequired{
				InputRequired: &providerv1.InputRequired{
					RequestId:       "request-1",
					Message:         "Need another value",
					InputSchemaJson: []byte(`{"type":"string"}`),
				},
			},
		})
	case "task":
		return stream.Send(&providerv1.ExecuteToolResponse{
			Event: &providerv1.ExecuteToolResponse_Task{
				Task: &providerv1.Task{
					TaskId:  "task-1",
					Status:  "queued",
					Message: "Queued",
				},
			},
		})
	case "tool_fail":
		return stream.Send(&providerv1.ExecuteToolResponse{
			Event: &providerv1.ExecuteToolResponse_Failure{
				Failure: &providerv1.Failure{
					Kind:    providerv1.Failure_KIND_TOOL_ERROR,
					Message: "tool failed",
				},
			},
		})
	case "provider_fail":
		return stream.Send(&providerv1.ExecuteToolResponse{
			Event: &providerv1.ExecuteToolResponse_Failure{
				Failure: &providerv1.Failure{
					Kind:    providerv1.Failure_KIND_PROVIDER_ERROR,
					Message: "provider failed",
				},
			},
		})
	case "secret_fail":
		secret := ""
		if len(request.GetCredentials()) != 0 {
			secret = string(request.GetCredentials()[0].GetValue())
		}
		return stream.Send(&providerv1.ExecuteToolResponse{
			Event: &providerv1.ExecuteToolResponse_Failure{
				Failure: &providerv1.Failure{
					Kind:    providerv1.Failure_KIND_PROVIDER_ERROR,
					Message: "credential=" + secret,
				},
			},
		})
	case "no_terminal":
		return nil
	case "extra_terminal":
		if err := stream.Send(helperCompleteEvent(arguments["value"])); err != nil {
			return err
		}
		return stream.Send(helperCompleteEvent("second"))
	default:
		return stream.Send(helperCompleteEvent(arguments["value"]))
	}
}

func (s *helperPluginServer) Close(
	_ context.Context,
	request *providerv1.CloseRequest,
) (*providerv1.CloseResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.sessions[request.GetSessionId()]; !exists {
		return nil, status.Error(codes.NotFound, "session not found")
	}
	delete(s.sessions, request.GetSessionId())
	return &providerv1.CloseResponse{}, nil
}

func (s *helperPluginServer) helperSession(sessionID string) (helperSession, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	session, ok := s.sessions[sessionID]
	return session, ok
}

func helperHasCredential(
	credentials []*providerv1.Credential,
	reference string,
) bool {
	for _, credential := range credentials {
		if credential != nil &&
			credential.GetReference() == reference &&
			len(credential.GetValue()) != 0 {
			return true
		}
	}
	return false
}

func helperCompleteEvent(value any) *providerv1.ExecuteToolResponse {
	raw, err := json.Marshal(map[string]any{"echo": value})
	if err != nil {
		panic(fmt.Sprintf("marshal helper result: %v", err))
	}
	return &providerv1.ExecuteToolResponse{
		Event: &providerv1.ExecuteToolResponse_Complete{
			Complete: &providerv1.Complete{ContentJson: raw},
		},
	}
}

func testExternalInstallation(
	t *testing.T,
	mode string,
) (installationdomain.Installation, installationapplication.ResolvedPackage) {
	t.Helper()
	return testExternalInstallationVersion(
		t,
		mode,
		"pinst_0123456789abcdef0123456789abcdef",
		"1.0.0",
		"a",
	)
}

func testExternalInstallationVersion(
	t *testing.T,
	mode string,
	rawID string,
	packageVersion string,
	digestRune string,
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
	id, err := installationdomain.ParseInstallationID(rawID)
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
		"version": "` + packageVersion + `",
		"protocolVersions": [1],
		"displayName": "Example Echo",
		"description": "Test Provider plugin",
		"entrypoints": {"` + runtime.GOOS + `/` + runtime.GOARCH + `": "` + entrypoint + `"}
	}`)
	installation, err := installationdomain.NewInstallation(
		id,
		pluginID,
		packageVersion,
		installationdomain.SourceLocal,
		filepath.Dir(executable),
		"sha256:"+strings.Repeat(digestRune, 64),
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
