package plugin

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	catalogapplication "github.com/orz-i/Mender/backend/internal/catalog/application"
	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
	credentialdomain "github.com/orz-i/Mender/backend/internal/credential/domain"
	invocationapplication "github.com/orz-i/Mender/backend/internal/invocation/application"
	invocationdomain "github.com/orz-i/Mender/backend/internal/invocation/domain"
	providerplugin "github.com/orz-i/Mender/backend/internal/providerplugin"
)

func TestPluginOpenIsLocalAndExecuteResolvesCredential(t *testing.T) {
	t.Parallel()

	ref, err := credentialdomain.NewReference("model", "api")
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(Config{
		Engine:        EngineOpenAI,
		Model:         "gpt-test",
		CredentialRef: ref,
		Capabilities:  []Capability{CapabilityWebSearch},
	})
	if err != nil {
		t.Fatal(err)
	}

	engine := &testModelToolEngine{
		result: SearchResult{
			Answer: "answer",
			Sources: []Source{
				{URL: "https://example.com/source", Title: "Source"},
			},
			Queries: []string{"provider query"},
		},
	}
	factoryCalls := 0
	plugin := &Plugin{
		newEngine: func(config Config) (EngineAdapter, error) {
			factoryCalls++
			if config.Engine != EngineOpenAI || config.Model != "gpt-test" {
				t.Fatalf("engine config = %+v", config)
			}
			return engine, nil
		},
	}

	instanceRaw, err := plugin.Open(t.Context(), providerplugin.OpenRequest{
		ConfigVersion: ConfigVersion,
		Config:        raw,
		Credentials:   panicCredentialResolver{},
	})
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	if factoryCalls != 1 {
		t.Fatalf("engine factory calls = %d, want 1", factoryCalls)
	}
	instance := instanceRaw.(*Instance)
	t.Cleanup(func() { _ = instance.Close() })

	tools, err := instance.DiscoverTools(t.Context(), catalogdomain.Provider{})
	if err != nil {
		t.Fatalf("DiscoverTools() error = %v", err)
	}
	if len(tools) != 1 {
		t.Fatalf("tools = %+v", tools)
	}
	tool := modelToolFromDiscovery(t, tools[0])

	secret := []byte("api-secret")
	resolver := &testModelCredentialResolver{
		ref:    ref,
		secret: secret,
	}
	arguments, err := invocationdomain.NewJSONValue(
		[]byte(`{"query":"search this"}`),
	)
	if err != nil {
		t.Fatal(err)
	}
	outcome, err := instance.Execute(
		t.Context(),
		invocationapplication.ExecutionRequest{
			Tool:      tool,
			Arguments: arguments,
		},
		resolver,
		nil,
	)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if outcome.Kind() != invocationdomain.OutcomeKindComplete {
		t.Fatalf("outcome = %#v", outcome)
	}
	if resolver.calls != 1 || resolver.lastRef != ref {
		t.Fatalf("resolver = calls=%d ref=%s", resolver.calls, resolver.lastRef)
	}
	if engine.requests != 1 || engine.lastRequest.Query != "search this" {
		t.Fatalf("engine = requests=%d request=%+v", engine.requests, engine.lastRequest)
	}
	for index, value := range secret {
		if value != 0 {
			t.Fatalf("secret byte %d = %d, want zero", index, value)
		}
	}

	var result SearchResult
	if err := json.Unmarshal(outcome.Content().Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Answer != "answer" || len(result.Sources) != 1 {
		t.Fatalf("result = %+v", result)
	}
}

func TestInstanceMapsToolErrorToCanonicalFailure(t *testing.T) {
	t.Parallel()

	instance, tool, resolver := testModelToolInstance(t, &testModelToolEngine{
		err: newToolError("hosted web search was not executed"),
	})
	arguments, _ := invocationdomain.NewJSONValue([]byte(`{"query":"q"}`))
	outcome, err := instance.Execute(
		t.Context(),
		invocationapplication.ExecutionRequest{Tool: tool, Arguments: arguments},
		resolver,
		nil,
	)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if outcome.Kind() != invocationdomain.OutcomeKindFailed ||
		outcome.FailureSource() != invocationdomain.FailureSourceTool ||
		outcome.FailureCode() != invocationdomain.FailureToolError {
		t.Fatalf("outcome = %#v", outcome)
	}
}

func TestInstancePreservesCredentialResolverError(t *testing.T) {
	t.Parallel()

	instance, tool, resolver := testModelToolInstance(t, &testModelToolEngine{})
	sentinel := errors.New("credential outside scope")
	resolver.err = sentinel
	arguments, _ := invocationdomain.NewJSONValue([]byte(`{"query":"q"}`))
	_, err := instance.Execute(
		t.Context(),
		invocationapplication.ExecutionRequest{Tool: tool, Arguments: arguments},
		resolver,
		nil,
	)
	if !errors.Is(err, sentinel) {
		t.Fatalf("Execute() error = %v, want sentinel", err)
	}
}

func TestDefaultEngineFactoryVariants(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		engine Engine
		check  func(EngineAdapter) bool
	}{
		{EngineOpenAI, func(value EngineAdapter) bool {
			_, ok := value.(*openAIEngine)
			return ok
		}},
		{EngineAnthropic, func(value EngineAdapter) bool {
			_, ok := value.(*anthropicEngine)
			return ok
		}},
		{EngineGemini, func(value EngineAdapter) bool {
			_, ok := value.(*geminiEngine)
			return ok
		}},
	} {
		adapter, err := defaultEngineFactory(Config{
			Engine: test.engine,
			Model:  "model",
		})
		if err != nil {
			t.Fatalf("defaultEngineFactory(%s) error = %v", test.engine, err)
		}
		if !test.check(adapter) {
			t.Fatalf("defaultEngineFactory(%s) = %T", test.engine, adapter)
		}
		_ = adapter.Close()
	}
}

func testModelToolInstance(
	t *testing.T,
	engine EngineAdapter,
) (*Instance, catalogdomain.Tool, *testModelCredentialResolver) {
	t.Helper()

	ref, err := credentialdomain.NewReference("model", "api")
	if err != nil {
		t.Fatal(err)
	}
	config := Config{
		Engine:        EngineOpenAI,
		Model:         "model",
		CredentialRef: ref,
		Capabilities:  []Capability{CapabilityWebSearch},
	}
	tools := discoveredCapabilityTools(config)
	return &Instance{
		config: config,
		engine: engine,
		tools:  tools,
	}, modelToolFromDiscovery(t, tools[0]), &testModelCredentialResolver{
		ref:    ref,
		secret: []byte("secret"),
	}
}

func modelToolFromDiscovery(
	t *testing.T,
	discovered catalogapplication.DiscoveredTool,
) catalogdomain.Tool {
	t.Helper()

	toolID, err := catalogdomain.ParseToolID(
		"tool_00000000000000000000000000000088",
	)
	if err != nil {
		t.Fatal(err)
	}
	providerID, err := catalogdomain.ParseProviderID(
		"prov_00000000000000000000000000000088",
	)
	if err != nil {
		t.Fatal(err)
	}
	inputSchema, err := catalogdomain.NewSchema(discovered.InputSchema)
	if err != nil {
		t.Fatal(err)
	}
	output, err := catalogdomain.NewSchema(discovered.OutputSchema)
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
			InputSchema:  inputSchema,
			OutputSchema: &output,
			Tags:         discovered.Tags,
		},
		time.Now(),
	)
	if err != nil {
		t.Fatal(err)
	}
	return tool
}

type panicCredentialResolver struct{}

func (panicCredentialResolver) Resolve(
	context.Context,
	credentialdomain.Reference,
) ([]byte, error) {
	panic("credential resolution must not happen during Provider open")
}

type testModelCredentialResolver struct {
	ref     credentialdomain.Reference
	secret  []byte
	err     error
	calls   int
	lastRef credentialdomain.Reference
}

func (r *testModelCredentialResolver) Resolve(
	_ context.Context,
	ref credentialdomain.Reference,
) ([]byte, error) {
	r.calls++
	r.lastRef = ref
	if r.err != nil {
		return nil, r.err
	}
	if ref != r.ref {
		return nil, errors.New("unexpected credential reference")
	}
	return r.secret, nil
}

type testModelToolEngine struct {
	result      SearchResult
	err         error
	requests    int
	lastRequest WebSearchRequest
	closed      int
}

func (e *testModelToolEngine) WebSearch(
	_ context.Context,
	request WebSearchRequest,
	_ []byte,
) (SearchResult, error) {
	e.requests++
	e.lastRequest = request
	return e.result, e.err
}

func (e *testModelToolEngine) Close() error {
	e.closed++
	return nil
}
