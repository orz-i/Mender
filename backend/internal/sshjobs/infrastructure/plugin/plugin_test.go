package plugin

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
	gov "github.com/orz-i/Mender/backend/internal/governance/domain"
	invocationapp "github.com/orz-i/Mender/backend/internal/invocation/application"
	invocationdomain "github.com/orz-i/Mender/backend/internal/invocation/domain"
	providerplugin "github.com/orz-i/Mender/backend/internal/providerplugin"
	sandboxdomain "github.com/orz-i/Mender/backend/internal/sandbox/domain"
	sshremote "github.com/orz-i/Mender/backend/internal/sandbox/infrastructure/sshremote"
)

const fixtureSandboxID = "sbx_00000000000000000000000000000051"
const fixtureTaskID = "sjob_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

type fakePort struct {
	hits        []string
	id          sandboxdomain.ID
	args        sshremote.JobRequest
	unavailable bool
}

func (f *fakePort) record(id sandboxdomain.ID, key string) { f.hits = append(f.hits, key); f.id = id }
func (f *fakePort) VerifySSHJobResource(_ context.Context, id sandboxdomain.ID) error {
	f.record(id, "verify")
	if f.unavailable {
		return sandboxdomain.ErrBackendUnavailable
	}
	return nil
}
func (f *fakePort) SubmitSSHJob(_ context.Context, id sandboxdomain.ID, _ gov.Context, req sshremote.JobRequest) (sshremote.JobSnapshot, error) {
	f.record(id, SourceSubmit)
	f.args = req
	return sshremote.JobSnapshot{TaskID: fixtureTaskID, State: sshremote.JobUnknown}, sshremote.ErrJobUnknown
}
func (f *fakePort) SSHJobStatus(_ context.Context, id sandboxdomain.ID, _ gov.Context, taskID string) (sshremote.JobSnapshot, error) {
	f.record(id, SourceStatus)
	return sshremote.JobSnapshot{TaskID: taskID, State: sshremote.JobRunning}, nil
}
func (f *fakePort) SSHJobResult(_ context.Context, id sandboxdomain.ID, _ gov.Context, taskID string) (sshremote.JobSnapshot, error) {
	f.record(id, SourceResult)
	return sshremote.JobSnapshot{TaskID: taskID, State: sshremote.JobCompleted}, nil
}
func (f *fakePort) SSHJobCancel(_ context.Context, id sandboxdomain.ID, _ gov.Context, taskID string) (sshremote.JobSnapshot, error) {
	f.record(id, SourceCancel)
	return sshremote.JobSnapshot{TaskID: taskID, State: sshremote.JobUnknown, CancelRequested: true}, nil
}
func (f *fakePort) SSHJobLogs(_ context.Context, id sandboxdomain.ID, _ gov.Context, taskID, stream string, cursor int64, limit int) (sshremote.JobLogs, error) {
	f.record(id, SourceLogs)
	return sshremote.JobLogs{TaskID: taskID, Stream: stream, Content: "redacted-fixture", Cursor: cursor + 16}, nil
}
func fakeTool(t *testing.T, key string) catalogdomain.Tool {
	t.Helper()
	providerID, _ := catalogdomain.ParseProviderID("prov_00000000000000000000000000000051")
	toolID, _ := catalogdomain.ParseToolID("tool_00000000000000000000000000000051")
	schema, _ := catalogdomain.NewSchema([]byte(`{"type":"object"}`))
	tool, err := catalogdomain.NewTool(toolID, providerID, catalogdomain.ToolSpec{SourceKey: key, Name: "fixture", InputSchema: schema}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	return tool
}
func fakeCaller(t *testing.T) gov.Context {
	t.Helper()
	owner, _ := gov.NewPrincipal(gov.PrincipalLocalUser, "os:fixture")
	caller, err := gov.NewContext(gov.OriginInternal, owner, gov.CatalogScope())
	if err != nil {
		t.Fatal(err)
	}
	return caller
}
func fakeInstance(t *testing.T) (*Plugin, *Instance, *fakePort) {
	t.Helper()
	fake := &fakePort{}
	plugin := New(fake)
	config, _ := json.Marshal(Config{SandboxID: fixtureSandboxID})
	instance, err := plugin.Open(context.Background(), providerplugin.OpenRequest{ConfigVersion: ConfigVersion, Config: config})
	if err != nil {
		t.Fatal(err)
	}
	return plugin, instance.(*Instance), fake
}
func TestSSHJobsToolsRemainInertAndDistinctFromMCPTasks(t *testing.T) {
	plugin, instance, fake := fakeInstance(t)
	tools, err := instance.DiscoverTools(context.Background(), catalogdomain.Provider{})
	if err != nil {
		t.Fatal(err)
	}
	keys := make([]string, 0, len(tools))
	for _, tool := range tools {
		keys = append(keys, tool.SourceKey)
	}
	if !slices.Equal(keys, []string{SourceSubmit, SourceStatus, SourceLogs, SourceResult, SourceCancel}) {
		t.Fatalf("wrong Job tools %v", keys)
	}
	if len(fake.hits) != 0 {
		t.Fatal("Tool discovery invoked SSH Jobs backend")
	}
	if err = plugin.ValidateConfig(context.Background(), json.RawMessage(`{"sandboxId":"`+fixtureSandboxID+`"}`)); err != nil {
		t.Fatal(err)
	}
	if len(fake.hits) != 1 || fake.hits[0] != "verify" {
		t.Fatal("config validation did not perform metadata-only exact check")
	}
	fake.hits = nil
	for _, tc := range []struct {
		key   string
		input any
	}{
		{SourceSubmit, map[string]any{"idempotencyKey": strings.Repeat("1", 32), "command": "echo private command"}},
		{SourceStatus, map[string]any{"taskId": fixtureTaskID}},
		{SourceLogs, map[string]any{"taskId": fixtureTaskID, "stream": "stdout", "cursor": 5, "limit": 100}},
		{SourceResult, map[string]any{"taskId": fixtureTaskID}},
		{SourceCancel, map[string]any{"taskId": fixtureTaskID}},
	} {
		raw, _ := json.Marshal(tc.input)
		input, err := invocationdomain.NewJSONValue(raw)
		if err != nil {
			t.Fatal(err)
		}
		output, err := instance.Execute(context.Background(), invocationapp.ExecutionRequest{Tool: fakeTool(t, tc.key), Arguments: input, Context: fakeCaller(t)}, nil, nil)
		if err != nil || output.Kind() != invocationdomain.OutcomeKindComplete {
			t.Fatalf("%s returned task-only or invalid outcome: %s %v", tc.key, output.Kind(), err)
		}
		if !json.Valid(output.Content().Bytes()) {
			t.Fatalf("%s JSON invalid", tc.key)
		}
		if strings.Contains(string(output.Content().Bytes()), "private command") {
			t.Fatal("command leaked in Job receipt")
		}
		if fake.id.String() != fixtureSandboxID || fake.hits[len(fake.hits)-1] != tc.key {
			t.Fatalf("%s routed outside immutable SandboxID", tc.key)
		}
	}
	config, _ := json.Marshal(Config{SandboxID: fixtureSandboxID})
	facts, err := plugin.ExtractGovernanceFacts(context.Background(), config, fakeTool(t, SourceSubmit), func() invocationdomain.JSONValue {
		v, _ := invocationdomain.NewJSONValue([]byte(`{"idempotencyKey":"11111111111111111111111111111111","command":"SECRET PAYLOAD"}`))
		return v
	}())
	if err != nil || facts.Knowledge() != invocationdomain.FactKnowledgeKnown || len(facts.Resources()) != 1 || facts.Resources()[0].Value() != fixtureSandboxID {
		t.Fatalf("governance facts %+v %v", facts, err)
	}
	for _, effect := range facts.Effects() {
		if strings.Contains(string(effect), "SECRET") {
			t.Fatal("command in Policy facts")
		}
	}
}
func TestSSHJobsPluginRejectsInvalidTaskInputsBeforeBackend(t *testing.T) {
	_, instance, fake := fakeInstance(t)
	for _, tc := range []struct {
		source  string
		payload string
	}{
		{SourceSubmit, `{"idempotencyKey":"invalid","command":"echo no"}`},
		{SourceSubmit, `{"idempotencyKey":"11111111111111111111111111111111","command":"echo no","sandboxId":"sbx_00000000000000000000000000000099"}`},
		{SourceStatus, `{"taskId":"invalid"}`},
		{SourceCancel, `{"taskId":"invalid"}`},
		{SourceLogs, `{"taskId":"` + fixtureTaskID + `","cursor":-1}`},
		{SourceLogs, `{"taskId":"` + fixtureTaskID + `","stream":"other"}`},
	} {
		arg, err := invocationdomain.NewJSONValue([]byte(tc.payload))
		if err != nil {
			t.Fatal(err)
		}
		_, err = instance.Execute(context.Background(), invocationapp.ExecutionRequest{Tool: fakeTool(t, tc.source), Arguments: arg, Context: fakeCaller(t)}, nil, nil)
		if !errors.Is(err, sshremote.ErrJobInvalid) {
			t.Fatalf("invalid %s accepted: %v", tc.source, err)
		}
	}
	if len(fake.hits) != 0 {
		t.Fatal("invalid input reached remote Job backend")
	}
}
