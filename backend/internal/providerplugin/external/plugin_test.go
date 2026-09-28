package external

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	catalogapplication "github.com/orz-i/Mender/backend/internal/catalog/application"
	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
	credentialdomain "github.com/orz-i/Mender/backend/internal/credential/domain"
	invocationapplication "github.com/orz-i/Mender/backend/internal/invocation/application"
	invocationdomain "github.com/orz-i/Mender/backend/internal/invocation/domain"
	providerplugin "github.com/orz-i/Mender/backend/internal/providerplugin"
)

func TestExternalPluginLifecycleAndOutcomeMapping(t *testing.T) {
	t.Parallel()

	plugin, supervisor := newTestExternalPlugin(t, "adapter")
	t.Cleanup(func() { _ = supervisor.CloseAll() })

	descriptor := plugin.Descriptor()
	if descriptor.ID.String() != "dev.example.echo" ||
		descriptor.Version != "1.0.0" ||
		descriptor.ConfigVersion != 1 {
		t.Fatalf("Descriptor() = %+v", descriptor)
	}

	rawConfig := json.RawMessage("{\"credentialRef\":\"credential://demo/token\"}")
	if err := plugin.ValidateConfig(t.Context(), rawConfig); err != nil {
		t.Fatalf("ValidateConfig() error = %v", err)
	}
	metadata, err := plugin.InspectConfig(t.Context(), rawConfig)
	if err != nil {
		t.Fatalf("InspectConfig() error = %v", err)
	}
	if len(metadata.CredentialRefs) != 1 ||
		metadata.CredentialRefs[0].String() != "credential://demo/token" {
		t.Fatalf("InspectConfig() = %+v", metadata)
	}

	providerID, err := catalogdomain.ParseProviderID(
		"prov_0123456789abcdef0123456789abcdef",
	)
	if err != nil {
		t.Fatal(err)
	}
	resolver := &testCredentialResolver{
		values: map[credentialdomain.Reference][]byte{},
	}
	ref, _ := credentialdomain.ParseReference("credential://demo/token")
	resolver.values[ref] = []byte("adapter-secret")

	instanceRaw, err := plugin.Open(
		t.Context(),
		providerplugin.OpenRequest{
			ProviderID:    providerID,
			ConfigVersion: 1,
			Config:        rawConfig,
			Credentials:   resolver,
		},
	)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	instance := instanceRaw.(*Instance)
	t.Cleanup(func() { _ = instance.Close() })

	tools, err := instance.DiscoverTools(
		t.Context(),
		catalogdomain.Provider{},
	)
	if err != nil {
		t.Fatalf("DiscoverTools() error = %v", err)
	}
	if len(tools) != 1 ||
		tools[0].SourceKey != "echo" ||
		tools[0].Name != "echo" ||
		len(tools[0].InputSchema) == 0 ||
		len(tools[0].OutputSchema) == 0 {
		t.Fatalf("DiscoverTools() = %+v", tools)
	}

	tool := externalToolFromDiscovery(t, providerID, tools[0])
	reporter := &testProgressReporter{}
	complete := executeExternalTool(
		t,
		instance,
		tool,
		resolver,
		reporter,
		map[string]any{"value": "hello"},
	)
	if complete.Kind() != invocationdomain.OutcomeKindComplete ||
		!bytes.Equal(complete.Content().Bytes(), []byte("{\"echo\":\"hello\"}")) {
		t.Fatalf(
			"complete outcome = kind=%q content=%s",
			complete.Kind(),
			complete.Content().Bytes(),
		)
	}
	if got := reporter.Messages(); len(got) != 1 || got[0] != "working" {
		t.Fatalf("progress = %#v", got)
	}

	input := executeExternalTool(
		t,
		instance,
		tool,
		resolver,
		&testProgressReporter{},
		map[string]any{"mode": "input"},
	)
	if input.Kind() != invocationdomain.OutcomeKindInputRequired ||
		input.RequestID() != "request-1" {
		t.Fatalf("input-required outcome = %+v", input)
	}

	task := executeExternalTool(
		t,
		instance,
		tool,
		resolver,
		&testProgressReporter{},
		map[string]any{"mode": "task"},
	)
	if task.Kind() != invocationdomain.OutcomeKindTask ||
		task.TaskID() != "task-1" ||
		task.TaskStatus() != "queued" {
		t.Fatalf("task outcome = %+v", task)
	}

	secretFailure := executeExternalTool(
		t,
		instance,
		tool,
		resolver,
		&testProgressReporter{},
		map[string]any{"mode": "secret_fail"},
	)
	if secretFailure.Kind() != invocationdomain.OutcomeKindFailed ||
		secretFailure.FailureSource() != invocationdomain.FailureSourceProvider ||
		secretFailure.FailureCode() != invocationdomain.FailureProviderError {
		t.Fatalf("failure outcome = %+v", secretFailure)
	}
	if strings.Contains(secretFailure.Message(), "adapter-secret") ||
		!strings.Contains(secretFailure.Message(), "[redacted]") {
		t.Fatalf(
			"failure message was not redacted: %q",
			secretFailure.Message(),
		)
	}

	if err := instance.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if err := instance.Close(); err != nil {
		t.Fatalf("Close() second error = %v", err)
	}
	if _, err := instance.DiscoverTools(
		t.Context(),
		catalogdomain.Provider{},
	); err == nil {
		t.Fatal("DiscoverTools() after Close error = nil")
	}
}

func TestExternalPluginRejectsOversizedConfigBeforeRPC(t *testing.T) {
	t.Parallel()

	raw := json.RawMessage(bytes.Repeat([]byte(" "), maxExternalConfigBytes+1))
	if err := validateExternalConfig(raw); err == nil ||
		!strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("validateExternalConfig() error = %v", err)
	}
}

func TestExternalPluginRejectsInvalidAndUndeclaredCredentialMetadata(t *testing.T) {
	t.Parallel()

	plugin, supervisor := newTestExternalPlugin(t, "adapter")
	t.Cleanup(func() { _ = supervisor.CloseAll() })

	if err := plugin.ValidateConfig(
		t.Context(),
		json.RawMessage("{\"invalid\":true}"),
	); err == nil || !strings.Contains(err.Error(), "config was rejected") {
		t.Fatalf("ValidateConfig(invalid) error = %v", err)
	}

	undeclared, undeclaredSupervisor := newTestExternalPlugin(
		t,
		"undeclaredinspect",
	)
	t.Cleanup(func() { _ = undeclaredSupervisor.CloseAll() })
	_, err := undeclared.InspectConfig(
		t.Context(),
		json.RawMessage("{\"name\":\"demo\"}"),
	)
	if err == nil ||
		!strings.Contains(err.Error(), "not declared in persisted config") {
		t.Fatalf("InspectConfig(undeclared) error = %v", err)
	}
}

func TestExternalPluginRejectsToolCredentialOutsideProviderDeclaration(t *testing.T) {
	t.Parallel()

	plugin, supervisor := newTestExternalPlugin(t, "toolundeclared")
	t.Cleanup(func() { _ = supervisor.CloseAll() })

	providerID, _ := catalogdomain.ParseProviderID(
		"prov_0123456789abcdef0123456789abcdef",
	)
	ref, _ := credentialdomain.ParseReference("credential://demo/token")
	resolver := &testCredentialResolver{
		values: map[credentialdomain.Reference][]byte{
			ref: []byte("adapter-secret"),
		},
	}
	_, err := plugin.Open(
		t.Context(),
		providerplugin.OpenRequest{
			ProviderID:    providerID,
			ConfigVersion: 1,
			Config: json.RawMessage(
				"{\"credentialRef\":\"credential://demo/token\"}",
			),
			Credentials: resolver,
		},
	)
	if err == nil ||
		!strings.Contains(err.Error(), "was not declared by Provider config") {
		t.Fatalf("Open() error = %v", err)
	}
}

func TestExternalPluginRejectsInvalidDescriptorSchema(t *testing.T) {
	t.Parallel()

	installation, resolved := testExternalInstallation(t, "invalidschema")
	resolver := &fakePackageResolver{resolved: resolved}
	supervisor, err := NewSupervisor(resolver, DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = supervisor.CloseAll() })

	if _, err := NewPlugin(t.Context(), installation, supervisor); err == nil ||
		!strings.Contains(err.Error(), "config schema") {
		t.Fatalf("NewPlugin() error = %v", err)
	}
}

func TestExternalPluginFailsClosedWhenListToolsProcessCrashes(t *testing.T) {
	t.Parallel()

	plugin, supervisor := newTestExternalPlugin(t, "crashlist")
	t.Cleanup(func() { _ = supervisor.CloseAll() })

	providerID, _ := catalogdomain.ParseProviderID(
		"prov_0123456789abcdef0123456789abcdef",
	)
	_, err := plugin.Open(
		t.Context(),
		providerplugin.OpenRequest{
			ProviderID:    providerID,
			ConfigVersion: 1,
			Config:        json.RawMessage("{}"),
		},
	)
	if err == nil || !strings.Contains(err.Error(), "list tools") {
		t.Fatalf("Open() error = %v", err)
	}
}

func TestExternalPluginRejectsDuplicateAndInvalidToolDescriptors(t *testing.T) {
	t.Parallel()

	for _, mode := range []string{"duplicatetools", "invalidtoolschema"} {
		t.Run(mode, func(t *testing.T) {
			t.Parallel()

			plugin, supervisor := newTestExternalPlugin(t, mode)
			t.Cleanup(func() { _ = supervisor.CloseAll() })
			providerID, _ := catalogdomain.ParseProviderID(
				"prov_0123456789abcdef0123456789abcdef",
			)
			_, err := plugin.Open(
				t.Context(),
				providerplugin.OpenRequest{
					ProviderID:    providerID,
					ConfigVersion: 1,
					Config:        json.RawMessage("{}"),
				},
			)
			if err == nil {
				t.Fatalf("Open(%s) error = nil", mode)
			}
		})
	}
}

func TestExternalInstanceFailsClosedWhenExecuteProcessCrashes(t *testing.T) {
	t.Parallel()

	plugin, supervisor := newTestExternalPlugin(t, "crashexec")
	t.Cleanup(func() { _ = supervisor.CloseAll() })

	providerID, _ := catalogdomain.ParseProviderID(
		"prov_0123456789abcdef0123456789abcdef",
	)
	instanceRaw, err := plugin.Open(
		t.Context(),
		providerplugin.OpenRequest{
			ProviderID:    providerID,
			ConfigVersion: 1,
			Config:        json.RawMessage("{}"),
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	instance := instanceRaw.(*Instance)
	tools, err := instance.DiscoverTools(t.Context(), catalogdomain.Provider{})
	if err != nil {
		t.Fatal(err)
	}
	tool := externalToolFromDiscovery(t, providerID, tools[0])

	_, err = executeExternalToolRaw(
		t,
		instance,
		tool,
		nil,
		&testProgressReporter{},
		map[string]any{"value": "crash"},
	)
	if err == nil {
		t.Fatal("Execute() error = nil")
	}
	deadline := time.Now().Add(2 * time.Second)
	for instance.Reusable() && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if instance.Reusable() {
		t.Fatal("crashed external instance remained reusable")
	}
	if err := instance.Close(); err != nil {
		t.Fatalf("Close() after process crash error = %v", err)
	}
}

func TestExternalPluginBoundsControlPlaneRPCs(t *testing.T) {
	t.Parallel()

	providerID, err := catalogdomain.ParseProviderID(
		"prov_0123456789abcdef0123456789abcdef",
	)
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name string
		mode string
		call func(*Plugin) error
	}{
		{
			name: "validate",
			mode: "hangvalidate",
			call: func(plugin *Plugin) error {
				return plugin.ValidateConfig(context.Background(), json.RawMessage("{}"))
			},
		},
		{
			name: "inspect",
			mode: "hanginspect",
			call: func(plugin *Plugin) error {
				_, inspectErr := plugin.InspectConfig(context.Background(), json.RawMessage("{}"))
				return inspectErr
			},
		},
		{
			name: "open",
			mode: "hangopen",
			call: func(plugin *Plugin) error {
				_, openErr := plugin.Open(context.Background(), providerplugin.OpenRequest{
					ProviderID:    providerID,
					ConfigVersion: 1,
					Config:        json.RawMessage("{}"),
				})
				return openErr
			},
		},
		{
			name: "list-tools",
			mode: "hanglist",
			call: func(plugin *Plugin) error {
				_, openErr := plugin.Open(context.Background(), providerplugin.OpenRequest{
					ProviderID:    providerID,
					ConfigVersion: 1,
					Config:        json.RawMessage("{}"),
				})
				return openErr
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			options := DefaultOptions()
			options.ValidateConfigTimeout = 75 * time.Millisecond
			options.InspectConfigTimeout = 75 * time.Millisecond
			options.OpenTimeout = 75 * time.Millisecond
			options.ListToolsTimeout = 75 * time.Millisecond
			plugin, supervisor := newTestExternalPluginWithOptions(t, test.mode, options)
			t.Cleanup(func() { _ = supervisor.CloseAll() })

			started := time.Now()
			callErr := test.call(plugin)
			if callErr == nil {
				t.Fatal("control-plane RPC error = nil")
			}
			if elapsed := time.Since(started); elapsed > 2*time.Second {
				t.Fatalf("control-plane RPC elapsed = %s, want bounded host deadline", elapsed)
			}
			if !strings.Contains(strings.ToLower(callErr.Error()), "deadline") {
				t.Fatalf("control-plane RPC error = %v, want deadline", callErr)
			}
		})
	}
}

func TestExternalInstancePropagatesExecutionDeadline(t *testing.T) {
	t.Parallel()

	plugin, supervisor := newTestExternalPlugin(t, "slowexec")
	t.Cleanup(func() { _ = supervisor.CloseAll() })

	providerID, _ := catalogdomain.ParseProviderID(
		"prov_0123456789abcdef0123456789abcdef",
	)
	instanceRaw, err := plugin.Open(
		t.Context(),
		providerplugin.OpenRequest{
			ProviderID:    providerID,
			ConfigVersion: 1,
			Config:        json.RawMessage("{}"),
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	instance := instanceRaw.(*Instance)
	t.Cleanup(func() { _ = instance.Close() })
	tools, err := instance.DiscoverTools(t.Context(), catalogdomain.Provider{})
	if err != nil {
		t.Fatal(err)
	}
	tool := externalToolFromDiscovery(t, providerID, tools[0])

	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()
	_, err = executeExternalToolRawContext(
		t,
		ctx,
		instance,
		tool,
		nil,
		&testProgressReporter{},
		map[string]any{"value": "slow"},
	)
	if err == nil || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Execute() error = %v", err)
	}
}

func TestExternalInstanceCredentialScopeAndMalformedStreams(t *testing.T) {
	t.Parallel()

	plugin, supervisor := newTestExternalPlugin(t, "adapter")
	t.Cleanup(func() { _ = supervisor.CloseAll() })

	providerID, _ := catalogdomain.ParseProviderID(
		"prov_0123456789abcdef0123456789abcdef",
	)
	ref, _ := credentialdomain.ParseReference("credential://demo/token")
	allowedResolver := &testCredentialResolver{
		values: map[credentialdomain.Reference][]byte{
			ref: []byte("adapter-secret"),
		},
	}
	instanceRaw, err := plugin.Open(
		t.Context(),
		providerplugin.OpenRequest{
			ProviderID:    providerID,
			ConfigVersion: 1,
			Config: json.RawMessage(
				"{\"credentialRef\":\"credential://demo/token\"}",
			),
			Credentials: allowedResolver,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	instance := instanceRaw.(*Instance)
	t.Cleanup(func() { _ = instance.Close() })
	tools, err := instance.DiscoverTools(
		t.Context(),
		catalogdomain.Provider{},
	)
	if err != nil {
		t.Fatal(err)
	}
	tool := externalToolFromDiscovery(t, providerID, tools[0])

	deniedResolver := &testCredentialResolver{
		err: invocationapplication.ErrCredentialOutsideScope,
	}
	_, err = executeExternalToolRaw(
		t,
		instance,
		tool,
		deniedResolver,
		&testProgressReporter{},
		map[string]any{"value": "blocked"},
	)
	if !errors.Is(err, invocationapplication.ErrCredentialOutsideScope) {
		t.Fatalf("Execute(denied) error = %v", err)
	}

	for _, mode := range []string{"no_terminal", "extra_terminal"} {
		_, err := executeExternalToolRaw(
			t,
			instance,
			tool,
			allowedResolver,
			&testProgressReporter{},
			map[string]any{"mode": mode},
		)
		if err == nil {
			t.Fatalf("Execute(%s) error = nil", mode)
		}
	}
}

func newTestExternalPlugin(
	t *testing.T,
	mode string,
) (*Plugin, *Supervisor) {
	t.Helper()
	options := DefaultOptions()
	options.StartTimeout = 10 * time.Second
	options.DescriptorTimeout = 5 * time.Second
	options.MaxStderrBytes = 4096
	return newTestExternalPluginWithOptions(t, mode, options)
}

func newTestExternalPluginWithOptions(
	t *testing.T,
	mode string,
	options Options,
) (*Plugin, *Supervisor) {
	t.Helper()

	installation, resolved := testExternalInstallation(t, mode)
	resolver := &fakePackageResolver{resolved: resolved}
	supervisor, err := NewSupervisor(resolver, options)
	if err != nil {
		t.Fatal(err)
	}
	plugin, err := NewPlugin(t.Context(), installation, supervisor)
	if err != nil {
		_ = supervisor.CloseAll()
		t.Fatalf("NewPlugin() error = %v", err)
	}
	return plugin, supervisor
}

func externalToolFromDiscovery(
	t *testing.T,
	providerID catalogdomain.ProviderID,
	discovered catalogapplication.DiscoveredTool,
) catalogdomain.Tool {
	t.Helper()

	input, err := catalogdomain.NewSchema(discovered.InputSchema)
	if err != nil {
		t.Fatal(err)
	}
	var output *catalogdomain.Schema
	if len(discovered.OutputSchema) != 0 {
		schema, schemaErr := catalogdomain.NewSchema(discovered.OutputSchema)
		if schemaErr != nil {
			t.Fatal(schemaErr)
		}
		output = &schema
	}
	toolID, err := catalogdomain.ParseToolID(
		"tool_0123456789abcdef0123456789abcdef",
	)
	if err != nil {
		t.Fatal(err)
	}
	tool, err := catalogdomain.NewTool(
		toolID,
		providerID,
		catalogdomain.ToolSpec{
			SourceKey:    discovered.SourceKey,
			Name:         discovered.Name,
			Title:        discovered.Title,
			Description:  discovered.Description,
			InputSchema:  input,
			OutputSchema: output,
			Tags:         discovered.Tags,
		},
		time.Date(2026, 9, 28, 4, 0, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatal(err)
	}
	return tool
}

func executeExternalTool(
	t *testing.T,
	instance *Instance,
	tool catalogdomain.Tool,
	resolver invocationapplication.CredentialResolver,
	reporter invocationapplication.ProgressReporter,
	arguments any,
) invocationdomain.Outcome {
	t.Helper()

	outcome, err := executeExternalToolRaw(
		t,
		instance,
		tool,
		resolver,
		reporter,
		arguments,
	)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	return outcome
}

func executeExternalToolRaw(
	t *testing.T,
	instance *Instance,
	tool catalogdomain.Tool,
	resolver invocationapplication.CredentialResolver,
	reporter invocationapplication.ProgressReporter,
	arguments any,
) (invocationdomain.Outcome, error) {
	t.Helper()
	return executeExternalToolRawContext(
		t,
		t.Context(),
		instance,
		tool,
		resolver,
		reporter,
		arguments,
	)
}

func executeExternalToolRawContext(
	t *testing.T,
	ctx context.Context,
	instance *Instance,
	tool catalogdomain.Tool,
	resolver invocationapplication.CredentialResolver,
	reporter invocationapplication.ProgressReporter,
	arguments any,
) (invocationdomain.Outcome, error) {
	t.Helper()

	raw, err := json.Marshal(arguments)
	if err != nil {
		t.Fatal(err)
	}
	value, err := invocationdomain.NewJSONValue(raw)
	if err != nil {
		t.Fatal(err)
	}
	invocationID, err := invocationdomain.ParseInvocationID(
		"inv_0123456789abcdef0123456789abcdef",
	)
	if err != nil {
		t.Fatal(err)
	}
	return instance.Execute(
		ctx,
		invocationapplication.ExecutionRequest{
			InvocationID: invocationID,
			Tool:         tool,
			Arguments:    value,
		},
		resolver,
		reporter,
	)
}

type testCredentialResolver struct {
	mu     sync.Mutex
	values map[credentialdomain.Reference][]byte
	err    error
	refs   []credentialdomain.Reference
}

func (r *testCredentialResolver) Resolve(
	_ context.Context,
	ref credentialdomain.Reference,
) ([]byte, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.refs = append(r.refs, ref)
	if r.err != nil {
		return nil, r.err
	}
	value, ok := r.values[ref]
	if !ok {
		return nil, errors.New("credential not found")
	}
	return append([]byte(nil), value...), nil
}

type testProgressReporter struct {
	mu       sync.Mutex
	messages []string
}

func (r *testProgressReporter) Report(
	update invocationdomain.ProgressUpdate,
) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.messages = append(r.messages, update.Message())
}

func (r *testProgressReporter) Messages() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.messages...)
}
