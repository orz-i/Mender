package sqlite

import (
	"context"
	"strings"
	"testing"

	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
	credentialdomain "github.com/orz-i/Mender/backend/internal/credential/domain"
	profiledomain "github.com/orz-i/Mender/backend/internal/profile/domain"
)

func TestProfileToolBindingGovernanceRoundTrip(t *testing.T) {
	t.Parallel()

	store := openTestStore(t)
	id, _ := profiledomain.ParseProfileID("prof_00000000000000000000000000000011")
	slug, _ := profiledomain.NewSlug("governed")
	toolID, _ := catalogdomain.ParseToolID("tool_00000000000000000000000000000011")
	credentialRef, _ := credentialdomain.NewReference("github", "work")

	binding, err := profiledomain.NewToolBinding(profiledomain.ToolBindingSpec{
		ToolID:                toolID,
		Direct:                true,
		Alias:                 "create_prod_issue",
		TitleOverride:         "Create production issue",
		DescriptionOverride:   "Create an issue in the production tracker.",
		PolicyConstraint:      profiledomain.PolicyRequireApproval,
		ConcurrencyConstraint: profiledomain.ConcurrencySerial,
		CredentialRefs:        []credentialdomain.Reference{credentialRef},
	})
	if err != nil {
		t.Fatal(err)
	}
	profile, err := profiledomain.NewProfileWithBindings(
		id,
		slug,
		"Governed",
		profiledomain.ExposureHybrid,
		[]profiledomain.ToolBinding{binding},
	)
	if err != nil {
		t.Fatal(err)
	}

	if err := store.CreateProfile(context.Background(), profile); err != nil {
		t.Fatalf("CreateProfile() error = %v", err)
	}

	loaded, err := store.GetProfileBySlug(context.Background(), slug)
	if err != nil {
		t.Fatalf("GetProfileBySlug() error = %v", err)
	}
	got, ok := loaded.Binding(toolID)
	if !ok {
		t.Fatal("Binding() missing after round trip")
	}
	if !got.Direct() ||
		got.Alias() != "create_prod_issue" ||
		got.TitleOverride() != "Create production issue" ||
		got.DescriptionOverride() != "Create an issue in the production tracker." ||
		got.PolicyConstraint() != profiledomain.PolicyRequireApproval ||
		got.ConcurrencyConstraint() != profiledomain.ConcurrencySerial ||
		len(got.CredentialRefs()) != 1 || got.CredentialRefs()[0] != credentialRef {
		t.Fatalf("round-trip binding = %#v", got)
	}
}

func TestProfileRestoreFailsClosedOnMalformedBindingState(t *testing.T) {
	t.Parallel()

	store := openTestStore(t)
	id, _ := profiledomain.ParseProfileID("prof_00000000000000000000000000000012")
	slug, _ := profiledomain.NewSlug("malformed")
	toolID, _ := catalogdomain.ParseToolID("tool_00000000000000000000000000000012")

	binding, err := profiledomain.NewToolBinding(profiledomain.ToolBindingSpec{ToolID: toolID, Direct: false})
	if err != nil {
		t.Fatal(err)
	}
	profile, err := profiledomain.NewProfileWithBindings(
		id,
		slug,
		"Malformed",
		profiledomain.ExposureDynamic,
		[]profiledomain.ToolBinding{binding},
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.CreateProfile(context.Background(), profile); err != nil {
		t.Fatal(err)
	}

	if _, err := store.db.ExecContext(
		context.Background(),
		"UPDATE profile_tools SET policy_constraint = 'allow' WHERE profile_id = ?",
		id.String(),
	); err != nil {
		t.Fatal(err)
	}

	_, err = store.GetProfileBySlug(context.Background(), slug)
	if err == nil || !strings.Contains(err.Error(), "unknown value") {
		t.Fatalf("GetProfileBySlug() error = %v, want malformed binding rejection", err)
	}
}
