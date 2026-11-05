package application

import (
	"context"
	"fmt"

	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
	credentialdomain "github.com/orz-i/Mender/backend/internal/credential/domain"
	profiledomain "github.com/orz-i/Mender/backend/internal/profile/domain"
)

// ToolBindingInput is the application-layer request shape for Profile ToolBinding state.
type ToolBindingInput struct {
	ToolID                catalogdomain.ToolID
	Alias                 string
	TitleOverride         string
	DescriptionOverride   string
	PolicyConstraint      profiledomain.PolicyConstraint
	ConcurrencyConstraint profiledomain.ConcurrencyConstraint
	CredentialRefs        []credentialdomain.Reference
}

func newToolBinding(input ToolBindingInput) (profiledomain.ToolBinding, error) {
	return profiledomain.NewToolBinding(profiledomain.ToolBindingSpec{
		ToolID:                input.ToolID,
		Alias:                 input.Alias,
		TitleOverride:         input.TitleOverride,
		DescriptionOverride:   input.DescriptionOverride,
		PolicyConstraint:      input.PolicyConstraint,
		ConcurrencyConstraint: input.ConcurrencyConstraint,
		CredentialRefs:        append([]credentialdomain.Reference(nil), input.CredentialRefs...),
	})
}

func validateBindings(
	ctx context.Context,
	profile profiledomain.Profile,
	tools ToolLookup,
	credentials CredentialLookup,
) error {
	for _, binding := range profile.Bindings() {
		_, err := tools.GetTool(ctx, binding.ToolID())
		if err != nil {
			return fmt.Errorf("resolve %s: %w", binding.ToolID(), err)
		}
		for _, ref := range binding.CredentialRefs() {
			if credentials == nil {
				return fmt.Errorf("%w: credential lookup is unavailable", ErrInvalidBinding)
			}
			if _, err := credentials.GetCredential(ctx, ref); err != nil {
				return fmt.Errorf("credential %s: %w", ref, err)
			}
		}
	}
	return nil
}
