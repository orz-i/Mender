package main

import (
	"context"
	"encoding/json"
	"testing"

	sandboxv1 "github.com/orz-i/Mender/sdk/sandboxplugin/go/protocol/v1"
)

const fixtureID = "sbx_0123456789abcdef0123456789abcdef"

func TestLifecycleAndWorkspace(t *testing.T) {
	t.Parallel()
	service := newService()
	ctx := context.Background()
	descriptor, err := service.GetDescriptor(ctx, &sandboxv1.GetDescriptorRequest{})
	if err != nil || descriptor.GetDescriptor_().GetProtocolVersion() != 1 {
		t.Fatalf("missing descriptor %+v %v", descriptor, err)
	}
	invalid, err := service.ValidateConfig(ctx, &sandboxv1.ValidateConfigRequest{ConfigVersion: 1, ConfigJson: []byte("!")})
	if err != nil || invalid.GetValid() {
		t.Fatalf("invalid config accepted %+v %v", invalid, err)
	}
	valid, err := service.ValidateConfig(ctx, &sandboxv1.ValidateConfigRequest{ConfigVersion: 1, ConfigJson: []byte("{}")})
	if err != nil || !valid.GetValid() {
		t.Fatalf("valid config rejected %+v %v", valid, err)
	}
	op := &sandboxv1.LifecycleRequest{SandboxId: fixtureID, Generation: 1, OperationId: "op:provision", ConfigJson: []byte(`{"uncertainProvision":true}`)}
	provision, err := service.Provision(ctx, &sandboxv1.ProvisionRequest{Operation: op})
	if err != nil || provision.GetResult().GetState() != "unknown" {
		t.Fatalf("uncertain provision: %+v %v", provision, err)
	}
	again, err := service.Provision(ctx, &sandboxv1.ProvisionRequest{Operation: op})
	if err != nil || again.GetResult().GetExternalId() != provision.GetResult().GetExternalId() {
		t.Fatalf("same idempotency key was replayed as another resource: %+v %v", again, err)
	}
	another := &sandboxv1.LifecycleRequest{SandboxId: op.GetSandboxId(), Generation: op.GetGeneration(),
		OperationId: "different-operation", ConfigJson: op.GetConfigJson()}
	if _, err := service.Provision(ctx, &sandboxv1.ProvisionRequest{Operation: another}); err == nil {
		t.Fatal("another provisioning key duplicated the resource")
	}
	op.ExternalId = provision.GetResult().GetExternalId()
	op.OperationId = "op:reconcile"
	observed, err := service.Reconcile(ctx, &sandboxv1.ReconcileRequest{Operation: op})
	if err != nil || observed.GetResult().GetState() != "ready" {
		t.Fatalf("reconcile: %+v %v", observed, err)
	}
	session, err := service.OpenSession(ctx, &sandboxv1.OpenSessionRequest{SandboxId: fixtureID, Generation: 1, ExternalId: op.ExternalId, ConfigJson: []byte("{}")})
	if err != nil || session.GetSessionId() == "" {
		t.Fatalf("open: %+v %v", session, err)
	}
	_, err = service.ExecuteWorkspace(ctx, &sandboxv1.ExecuteWorkspaceRequest{SessionId: session.SessionId, Capability: "process.shell", ArgumentsJson: []byte("{}")})
	if err == nil {
		t.Fatal("shell capability must not run")
	}
	reply, err := service.ExecuteWorkspace(ctx, &sandboxv1.ExecuteWorkspaceRequest{SessionId: session.SessionId, Capability: "file.read", ArgumentsJson: []byte(`{"path":"README.txt"}`)})
	if err != nil || !json.Valid(reply.GetContentJson()) {
		t.Fatalf("read: %+v %v", reply, err)
	}
	_, err = service.CloseSession(ctx, &sandboxv1.CloseSessionRequest{SessionId: session.SessionId})
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.ExecuteWorkspace(ctx, &sandboxv1.ExecuteWorkspaceRequest{SessionId: session.SessionId, Capability: "file.read", ArgumentsJson: []byte("{}")})
	if err == nil {
		t.Fatal("closed session still works")
	}
	op.OperationId = "op:destroy"
	destroyed, err := service.Destroy(ctx, &sandboxv1.DestroyRequest{Operation: op})
	if err != nil || destroyed.GetResult().GetState() != "destroyed" {
		t.Fatalf("destroy: %+v %v", destroyed, err)
	}
}

func TestNoProvisionWithoutExactOperationIdentity(t *testing.T) {
	t.Parallel()
	service := newService()
	for _, op := range []*sandboxv1.LifecycleRequest{
		{SandboxId: fixtureID, Generation: 1, ConfigJson: []byte("{}")},
		{SandboxId: fixtureID, Generation: 0, OperationId: "op", ConfigJson: []byte("{}")},
		{SandboxId: fixtureID, Generation: 1, OperationId: "op", ExternalId: "forged", ConfigJson: []byte("{}")},
	} {
		if _, err := service.Provision(context.Background(), &sandboxv1.ProvisionRequest{Operation: op}); err == nil {
			t.Fatalf("provision accepted missing identity: %+v", op)
		}
	}
}
