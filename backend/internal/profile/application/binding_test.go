package application

import (
	"context"
	"errors"
	"testing"
	"time"

	catalogapplication "github.com/orz-i/Mender/backend/internal/catalog/application"
	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
	credentialdomain "github.com/orz-i/Mender/backend/internal/credential/domain"
	profiledomain "github.com/orz-i/Mender/backend/internal/profile/domain"
)

func TestAddToolStoresFullBindingConfiguration(t *testing.T) {
	t.Parallel()

	repository := newMemoryRepository()
	profile := applicationBindingProfile(t, "prof_00000000000000000000000000000021", "governed")
	if err := repository.CreateProfile(context.Background(), profile); err != nil {
		t.Fatal(err)
	}

	tool := applicationBindingTool(
		t,
		"tool_00000000000000000000000000000021",
		"prov_00000000000000000000000000000021",
		"create_issue",
	)
	add := NewAddTool(repository, bindingToolLookup{
		tool.ID(): tool,
	})

	updated, err := add.Execute(context.Background(), AddToolRequest{
		ProfileSlug:           profile.Slug(),
		ToolID:                tool.ID(),
		Direct:                true,
		Alias:                 "create_prod_issue",
		TitleOverride:         "Create production issue",
		DescriptionOverride:   "Create an issue in the production tracker.",
		PolicyConstraint:      profiledomain.PolicyRequireApproval,
		ConcurrencyConstraint: profiledomain.ConcurrencySerial,
	})
	if err != nil {
		t.Fatalf("AddTool.Execute() error = %v", err)
	}

	binding, ok := updated.Binding(tool.ID())
	if !ok {
		t.Fatal("updated Profile missing ToolBinding")
	}
	if !binding.Direct() ||
		binding.Alias() != "create_prod_issue" ||
		binding.TitleOverride() != "Create production issue" ||
		binding.DescriptionOverride() != "Create an issue in the production tracker." ||
		binding.PolicyConstraint() != profiledomain.PolicyRequireApproval ||
		binding.ConcurrencyConstraint() != profiledomain.ConcurrencySerial {
		t.Fatalf("binding = %#v", binding)
	}

	stored, err := repository.GetProfileBySlug(context.Background(), profile.Slug())
	if err != nil {
		t.Fatal(err)
	}
	storedBinding, ok := stored.Binding(tool.ID())
	if !ok || storedBinding.Alias() != "create_prod_issue" {
		t.Fatalf("stored binding = %#v", storedBinding)
	}
}

func TestReplaceToolsRejectsEffectiveDirectNameCollisionBeforeSave(t *testing.T) {
	t.Parallel()

	repository := newMemoryRepository()
	profile := applicationBindingProfile(t, "prof_00000000000000000000000000000022", "collision")
	if err := repository.CreateProfile(context.Background(), profile); err != nil {
		t.Fatal(err)
	}

	providerID := "prov_00000000000000000000000000000022"
	first := applicationBindingTool(
		t,
		"tool_00000000000000000000000000000022",
		providerID,
		"first",
	)
	second := applicationBindingTool(
		t,
		"tool_00000000000000000000000000000023",
		providerID,
		"shared",
	)
	replace := NewReplaceTools(repository, bindingToolLookup{
		first.ID():  first,
		second.ID(): second,
	})

	_, err := replace.Execute(context.Background(), ReplaceToolsRequest{
		ProfileSlug: profile.Slug(),
		Tools: []ToolBindingInput{
			{
				ToolID: first.ID(),
				Direct: true,
				Alias:  "shared",
			},
			{
				ToolID: second.ID(),
				Direct: true,
			},
		},
	})
	if !errors.Is(err, ErrBindingNameConflict) {
		t.Fatalf("ReplaceTools.Execute() error = %v, want ErrBindingNameConflict", err)
	}

	stored, err := repository.GetProfileBySlug(context.Background(), profile.Slug())
	if err != nil {
		t.Fatal(err)
	}
	if len(stored.Bindings()) != 0 {
		t.Fatalf("failed replacement persisted partial bindings = %#v", stored.Bindings())
	}
}

func TestReplaceToolsValidatesEveryToolBeforeSave(t *testing.T) {
	t.Parallel()

	repository := newMemoryRepository()
	profile := applicationBindingProfile(t, "prof_00000000000000000000000000000023", "validate-all")
	if err := repository.CreateProfile(context.Background(), profile); err != nil {
		t.Fatal(err)
	}

	valid := applicationBindingTool(
		t,
		"tool_00000000000000000000000000000024",
		"prov_00000000000000000000000000000024",
		"valid",
	)
	missing, _ := catalogdomain.ParseToolID("tool_00000000000000000000000000000025")
	replace := NewReplaceTools(repository, bindingToolLookup{
		valid.ID(): valid,
	})

	_, err := replace.Execute(context.Background(), ReplaceToolsRequest{
		ProfileSlug: profile.Slug(),
		Tools: []ToolBindingInput{
			{ToolID: valid.ID(), Direct: true},
			{ToolID: missing, Direct: true},
		},
	})
	if !errors.Is(err, catalogapplication.ErrToolNotFound) {
		t.Fatalf("ReplaceTools.Execute() error = %v, want ErrToolNotFound", err)
	}

	stored, err := repository.GetProfileBySlug(context.Background(), profile.Slug())
	if err != nil {
		t.Fatal(err)
	}
	if len(stored.Bindings()) != 0 {
		t.Fatalf("failed validation persisted partial bindings = %#v", stored.Bindings())
	}
}

func TestAddToolValidatesCredentialRefsAgainstInventory(t *testing.T) {
	t.Parallel()

	repository := newMemoryRepository()
	profile := applicationBindingProfile(
		t,
		"prof_00000000000000000000000000000026",
		"credential-scope",
	)
	if err := repository.CreateProfile(context.Background(), profile); err != nil {
		t.Fatal(err)
	}

	tool := applicationBindingTool(
		t,
		"tool_00000000000000000000000000000026",
		"prov_00000000000000000000000000000026",
		"credential_tool",
	)
	ref, err := credentialdomain.NewReference("github", "work")
	if err != nil {
		t.Fatal(err)
	}
	credential, err := credentialdomain.NewCredential(
		ref,
		"GitHub Work",
		credentialdomain.SourceNative,
		"",
		time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatal(err)
	}

	add := NewAddTool(
		repository,
		bindingToolLookup{tool.ID(): tool},
		bindingCredentialLookup{ref: credential},
	)
	updated, err := add.Execute(context.Background(), AddToolRequest{
		ProfileSlug:    profile.Slug(),
		ToolID:         tool.ID(),
		CredentialRefs: []credentialdomain.Reference{ref},
	})
	if err != nil {
		t.Fatalf("AddTool.Execute() error = %v", err)
	}
	binding, ok := updated.Binding(tool.ID())
	if !ok || len(binding.CredentialRefs()) != 1 ||
		binding.CredentialRefs()[0] != ref {
		t.Fatalf("binding CredentialRefs = %#v", binding.CredentialRefs())
	}
}

func TestReplaceToolsRejectsUnknownCredentialWithoutSave(t *testing.T) {
	t.Parallel()

	repository := newMemoryRepository()
	profile := applicationBindingProfile(
		t,
		"prof_00000000000000000000000000000027",
		"credential-missing",
	)
	if err := repository.CreateProfile(context.Background(), profile); err != nil {
		t.Fatal(err)
	}
	tool := applicationBindingTool(
		t,
		"tool_00000000000000000000000000000027",
		"prov_00000000000000000000000000000027",
		"credential_tool",
	)
	ref, err := credentialdomain.NewReference("github", "missing")
	if err != nil {
		t.Fatal(err)
	}

	replace := NewReplaceTools(
		repository,
		bindingToolLookup{tool.ID(): tool},
		bindingCredentialLookup{},
	)
	if _, err := replace.Execute(context.Background(), ReplaceToolsRequest{
		ProfileSlug: profile.Slug(),
		Tools: []ToolBindingInput{
			{
				ToolID:         tool.ID(),
				CredentialRefs: []credentialdomain.Reference{ref},
			},
		},
	}); err == nil {
		t.Fatal("ReplaceTools.Execute() accepted unknown Credential")
	}
	stored, err := repository.GetProfileBySlug(
		context.Background(),
		profile.Slug(),
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(stored.Bindings()) != 0 {
		t.Fatalf("failed replacement persisted bindings = %#v", stored.Bindings())
	}
}

type bindingCredentialLookup map[credentialdomain.Reference]credentialdomain.Credential

func (m bindingCredentialLookup) GetCredential(
	_ context.Context,
	ref credentialdomain.Reference,
) (credentialdomain.Credential, error) {
	credential, ok := m[ref]
	if !ok {
		return credentialdomain.Credential{}, errors.New("credential not found")
	}
	return credential, nil
}

type bindingToolLookup map[catalogdomain.ToolID]catalogdomain.Tool

func (m bindingToolLookup) GetTool(
	_ context.Context,
	id catalogdomain.ToolID,
) (catalogdomain.Tool, error) {
	tool, ok := m[id]
	if !ok {
		return catalogdomain.Tool{}, catalogapplication.ErrToolNotFound
	}
	return tool, nil
}

func applicationBindingProfile(
	t *testing.T,
	rawID string,
	rawSlug string,
) profiledomain.Profile {
	t.Helper()

	id, err := profiledomain.ParseProfileID(rawID)
	if err != nil {
		t.Fatal(err)
	}
	slug, err := profiledomain.NewSlug(rawSlug)
	if err != nil {
		t.Fatal(err)
	}
	profile, err := profiledomain.NewProfileWithBindings(
		id,
		slug,
		"Binding Profile",
		profiledomain.ExposureHybrid,
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	return profile
}

func applicationBindingTool(
	t *testing.T,
	rawToolID string,
	rawProviderID string,
	name string,
) catalogdomain.Tool {
	t.Helper()

	toolID, err := catalogdomain.ParseToolID(rawToolID)
	if err != nil {
		t.Fatal(err)
	}
	providerID, err := catalogdomain.ParseProviderID(rawProviderID)
	if err != nil {
		t.Fatal(err)
	}
	schema, err := catalogdomain.NewSchema([]byte(`{"type":"object"}`))
	if err != nil {
		t.Fatal(err)
	}
	tool, err := catalogdomain.NewTool(
		toolID,
		providerID,
		catalogdomain.ToolSpec{
			SourceKey:   name,
			Name:        name,
			Title:       name,
			InputSchema: schema,
		},
		time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatal(err)
	}
	return tool
}
