// A deterministic, local-only example external Sandbox service.
// It imports the public SDK only, with no Mender backend implementation access.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"

	sandboxplugin "github.com/orz-i/Mender/sdk/sandboxplugin/go"
	sandboxv1 "github.com/orz-i/Mender/sdk/sandboxplugin/go/protocol/v1"
)

type resource struct {
	generation           uint64
	state                string
	externalID           string
	uncertainOnce        bool
	provisionOperationID string
}
type service struct {
	sandboxv1.UnimplementedSandboxPluginServiceServer
	mu        sync.Mutex
	resources map[string]*resource
	sessions  map[string]string
}

func newService() *service {
	return &service{resources: map[string]*resource{}, sessions: map[string]string{}}
}
func (*service) GetDescriptor(context.Context, *sandboxv1.GetDescriptorRequest) (*sandboxv1.GetDescriptorResponse, error) {
	return &sandboxv1.GetDescriptorResponse{Descriptor_: &sandboxv1.Descriptor{
		PluginId: "dev.mender.example.sandbox", PackageVersion: "0.1.0",
		DisplayName: "Example Sandbox", Description: "Deterministic fake remote backend without filesystem/host shell access",
		ProtocolVersion: 1, ConfigVersion: 1,
		ConfigSchemaJson: []byte(`{"type":"object","properties":{"message":{"type":"string","maxLength":120},"uncertainProvision":{"type":"boolean"}},"additionalProperties":false}`),
		Capabilities:     []string{"file.read"},
	}}, nil
}

type config struct {
	Message            string `json:"message"`
	UncertainProvision bool   `json:"uncertainProvision"`
}

func parseConfig(raw []byte) (config, error) {
	var c config
	if len(raw) > 1024 {
		return c, fmt.Errorf("config too large")
	}
	if err := json.Unmarshal(raw, &c); err != nil {
		return c, fmt.Errorf("invalid config")
	}
	if len(c.Message) > 120 {
		return c, fmt.Errorf("message too long")
	}
	return c, nil
}
func (*service) ValidateConfig(_ context.Context, req *sandboxv1.ValidateConfigRequest) (*sandboxv1.ValidateConfigResponse, error) {
	if req.GetConfigVersion() != 1 {
		return &sandboxv1.ValidateConfigResponse{Diagnostic: "unsupported config version"}, nil
	}
	_, err := parseConfig(req.GetConfigJson())
	if err != nil {
		return &sandboxv1.ValidateConfigResponse{Diagnostic: err.Error()}, nil
	}
	return &sandboxv1.ValidateConfigResponse{Valid: true}, nil
}
func (*service) InspectConfig(_ context.Context, req *sandboxv1.InspectConfigRequest) (*sandboxv1.InspectConfigResponse, error) {
	if req.GetConfigVersion() != 1 {
		return nil, fmt.Errorf("unsupported config version")
	}
	if _, err := parseConfig(req.GetConfigJson()); err != nil {
		return nil, err
	}
	return &sandboxv1.InspectConfigResponse{ConnectionSummary: "Local-only mock backend; no credentials or network access"}, nil
}
func validateOperation(id string, gen uint64) error {
	if !strings.HasPrefix(id, "sbx_") || len(id) != 36 || gen == 0 {
		return fmt.Errorf("invalid Sandbox identity or generation")
	}
	return nil
}
func (s *service) Provision(_ context.Context, req *sandboxv1.ProvisionRequest) (*sandboxv1.ProvisionResponse, error) {
	op := req.GetOperation()
	if op == nil || op.GetOperationId() == "" || op.GetExternalId() != "" {
		return nil, fmt.Errorf("missing provision key or unexpected external ID")
	}
	if err := validateOperation(op.GetSandboxId(), op.GetGeneration()); err != nil {
		return nil, err
	}
	c, err := parseConfig(op.GetConfigJson())
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	r := s.resources[op.GetSandboxId()]
	if r != nil {
		if r.provisionOperationID != op.GetOperationId() || r.generation != op.GetGeneration() {
			return nil, fmt.Errorf("resource already provisioned by another operation")
		}
		observed := "ready"
		if r.uncertainOnce {
			observed = "unknown"
		}
		return &sandboxv1.ProvisionResponse{Result: s.result(op.GetSandboxId(), r, observed)}, nil
	}
	r = &resource{generation: op.GetGeneration(), state: "ready", externalID: "mock:" + op.GetSandboxId(),
		uncertainOnce: c.UncertainProvision, provisionOperationID: op.GetOperationId()}
	s.resources[op.GetSandboxId()] = r
	state := "ready"
	if r.uncertainOnce {
		state = "unknown"
	}
	return &sandboxv1.ProvisionResponse{Result: s.result(op.GetSandboxId(), r, state)}, nil
}
func (s *service) result(id string, r *resource, state string) *sandboxv1.LifecycleResponse {
	return &sandboxv1.LifecycleResponse{SandboxId: id, Generation: r.generation, ExternalId: r.externalID, State: state}
}
func (s *service) change(op *sandboxv1.LifecycleRequest, action string) (*sandboxv1.LifecycleResponse, error) {
	if op == nil || op.GetOperationId() == "" {
		return nil, fmt.Errorf("missing operation")
	}
	if err := validateOperation(op.GetSandboxId(), op.GetGeneration()); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	r := s.resources[op.GetSandboxId()]
	if r == nil {
		return nil, fmt.Errorf("missing resource")
	}
	if action == "start" && r.state == "stopped" && op.GetGeneration() == r.generation+1 &&
		r.externalID == op.GetExternalId() {
		r.generation = op.GetGeneration()
	} else if r.generation != op.GetGeneration() ||
		(r.externalID != op.GetExternalId() && (action != "reconcile" || op.GetExternalId() != "")) {
		return nil, fmt.Errorf("stale run generation or external ID")
	}
	switch action {
	case "start":
		if r.state != "stopped" {
			return nil, fmt.Errorf("not stopped")
		}
		r.state = "ready"
	case "stop":
		if r.state != "ready" {
			return nil, fmt.Errorf("not ready")
		}
		r.state = "stopped"
	case "destroy":
		r.state = "destroyed"
	case "reconcile":
		r.uncertainOnce = false
	}
	return s.result(op.GetSandboxId(), r, r.state), nil
}
func (s *service) Inspect(_ context.Context, req *sandboxv1.InspectRequest) (*sandboxv1.InspectResponse, error) {
	op := req.GetOperation()
	if op == nil {
		return nil, fmt.Errorf("missing inspect identity")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	r := s.resources[op.GetSandboxId()]
	if r == nil {
		return nil, fmt.Errorf("resource absent")
	}
	if r.externalID != op.GetExternalId() || r.generation != op.GetGeneration() {
		return nil, fmt.Errorf("invalid run identity")
	}
	return &sandboxv1.InspectResponse{Result: s.result(op.GetSandboxId(), r, r.state)}, nil
}
func (s *service) Start(_ context.Context, r *sandboxv1.StartRequest) (*sandboxv1.StartResponse, error) {
	v, e := s.change(r.GetOperation(), "start")
	return &sandboxv1.StartResponse{Result: v}, e
}
func (s *service) Stop(_ context.Context, r *sandboxv1.StopRequest) (*sandboxv1.StopResponse, error) {
	v, e := s.change(r.GetOperation(), "stop")
	return &sandboxv1.StopResponse{Result: v}, e
}
func (s *service) Destroy(_ context.Context, r *sandboxv1.DestroyRequest) (*sandboxv1.DestroyResponse, error) {
	v, e := s.change(r.GetOperation(), "destroy")
	return &sandboxv1.DestroyResponse{Result: v}, e
}
func (s *service) Reconcile(_ context.Context, r *sandboxv1.ReconcileRequest) (*sandboxv1.ReconcileResponse, error) {
	v, e := s.change(r.GetOperation(), "reconcile")
	return &sandboxv1.ReconcileResponse{Result: v}, e
}
func (s *service) OpenSession(_ context.Context, req *sandboxv1.OpenSessionRequest) (*sandboxv1.OpenSessionResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r := s.resources[req.GetSandboxId()]
	if r == nil || r.generation != req.GetGeneration() || r.externalID != req.GetExternalId() || r.state != "ready" {
		return nil, fmt.Errorf("sandbox session not ready")
	}
	id := "session:" + req.GetSandboxId()
	s.sessions[id] = req.GetSandboxId()
	return &sandboxv1.OpenSessionResponse{SessionId: id}, nil
}
func (s *service) ExecuteWorkspace(_ context.Context, req *sandboxv1.ExecuteWorkspaceRequest) (*sandboxv1.ExecuteWorkspaceResponse, error) {
	s.mu.Lock()
	_, ok := s.sessions[req.GetSessionId()]
	s.mu.Unlock()
	if !ok {
		return nil, fmt.Errorf("session closed")
	}
	if req.GetCapability() != "file.read" {
		return nil, fmt.Errorf("capability unsupported")
	}
	var arg struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(req.GetArgumentsJson(), &arg); err != nil {
		return nil, fmt.Errorf("invalid arguments")
	}
	if arg.Path != "README.txt" {
		return nil, fmt.Errorf("file not found")
	}
	return &sandboxv1.ExecuteWorkspaceResponse{ContentJson: []byte(`{"path":"README.txt","content":"Example Sandbox managed fixture","startLine":1,"endLine":1,"truncated":false}`)}, nil
}
func (s *service) CloseSession(_ context.Context, req *sandboxv1.CloseSessionRequest) (*sandboxv1.CloseSessionResponse, error) {
	s.mu.Lock()
	delete(s.sessions, req.GetSessionId())
	s.mu.Unlock()
	return &sandboxv1.CloseSessionResponse{}, nil
}
func main() {
	if err := sandboxplugin.Serve(newService()); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
