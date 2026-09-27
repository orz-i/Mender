package providerplugin

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

func TestRegistryRegisterLookupAndDescriptors(t *testing.T) {
	t.Parallel()

	registry := NewRegistry()
	second := stubPlugin{descriptor: testDescriptor("com.example.provider.beta", "Beta")}
	first := stubPlugin{descriptor: testDescriptor("com.example.provider.alpha", "Alpha")}

	if err := registry.Register(second); err != nil {
		t.Fatalf("Register(second) error = %v", err)
	}
	if err := registry.Register(first); err != nil {
		t.Fatalf("Register(first) error = %v", err)
	}

	got, err := registry.Lookup(first.descriptor.ID)
	if err != nil {
		t.Fatalf("Lookup() error = %v", err)
	}
	if got.Descriptor().ID != first.descriptor.ID {
		t.Fatalf("Lookup() plugin id = %s, want %s", got.Descriptor().ID, first.descriptor.ID)
	}

	descriptors := registry.Descriptors()
	if len(descriptors) != 2 {
		t.Fatalf("Descriptors() len = %d, want 2", len(descriptors))
	}
	if descriptors[0].ID != first.descriptor.ID || descriptors[1].ID != second.descriptor.ID {
		t.Fatalf(
			"Descriptors() order = [%s, %s], want [%s, %s]",
			descriptors[0].ID,
			descriptors[1].ID,
			first.descriptor.ID,
			second.descriptor.ID,
		)
	}
}

func TestRegistryRejectsDuplicatePluginID(t *testing.T) {
	t.Parallel()

	registry := NewRegistry()
	plugin := stubPlugin{descriptor: testDescriptor("com.example.provider.same", "Same")}
	if err := registry.Register(plugin); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	err := registry.Register(plugin)
	if !errors.Is(err, ErrDuplicate) {
		t.Fatalf("Register(duplicate) error = %v, want ErrDuplicate", err)
	}
}

func TestRegistryReturnsNotFound(t *testing.T) {
	t.Parallel()

	id, err := ParseID("com.example.provider.missing")
	if err != nil {
		t.Fatal(err)
	}

	_, err = NewRegistry().Lookup(id)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("Lookup() error = %v, want ErrNotFound", err)
	}
}

func TestRegistrySnapshotsDescriptorAtRegistration(t *testing.T) {
	t.Parallel()

	plugin := &countingPlugin{
		descriptor: testDescriptor("com.example.provider.counted", "Counted"),
	}
	registry := NewRegistry()
	if err := registry.Register(plugin); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	_ = registry.Descriptors()
	_ = registry.Descriptors()
	if plugin.descriptorCalls != 1 {
		t.Fatalf("Descriptor() calls = %d, want 1", plugin.descriptorCalls)
	}
}

func testDescriptor(id, name string) Descriptor {
	return Descriptor{
		ID:            ID(id),
		Version:       "1.0.0",
		DisplayName:   name,
		ConfigVersion: 1,
	}
}

type stubPlugin struct {
	descriptor Descriptor
}

func (p stubPlugin) Descriptor() Descriptor {
	return p.descriptor
}

func (stubPlugin) ValidateConfig(context.Context, json.RawMessage) error {
	return nil
}

func (stubPlugin) InspectConfig(
	context.Context,
	json.RawMessage,
) (ConfigMetadata, error) {
	return ConfigMetadata{}, nil
}

func (stubPlugin) Open(context.Context, OpenRequest) (Instance, error) {
	return nil, errors.New("not implemented")
}

type countingPlugin struct {
	descriptor      Descriptor
	descriptorCalls int
}

func (p *countingPlugin) Descriptor() Descriptor {
	p.descriptorCalls++
	return p.descriptor
}

func (*countingPlugin) ValidateConfig(context.Context, json.RawMessage) error {
	return nil
}

func (*countingPlugin) InspectConfig(
	context.Context,
	json.RawMessage,
) (ConfigMetadata, error) {
	return ConfigMetadata{}, nil
}

func (*countingPlugin) Open(
	context.Context,
	OpenRequest,
) (Instance, error) {
	return nil, errors.New("not implemented")
}
