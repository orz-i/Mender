package application

import (
	"context"
	"fmt"

	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
	profiledomain "github.com/orz-i/Mender/backend/internal/profile/domain"
)

// ToolBindingInput is the application-layer request shape for Profile ToolBinding state.
type ToolBindingInput struct {
	ToolID                catalogdomain.ToolID
	Direct                bool
	Alias                 string
	TitleOverride         string
	DescriptionOverride   string
	PolicyConstraint      profiledomain.PolicyConstraint
	ConcurrencyConstraint profiledomain.ConcurrencyConstraint
}

// ReplaceToolSelection is retained for source compatibility while adapters migrate.
// Deprecated: use ToolBindingInput.
type ReplaceToolSelection = ToolBindingInput

func newToolBinding(input ToolBindingInput) (profiledomain.ToolBinding, error) {
	return profiledomain.NewToolBinding(profiledomain.ToolBindingSpec{
		ToolID:                input.ToolID,
		Direct:                input.Direct,
		Alias:                 input.Alias,
		TitleOverride:         input.TitleOverride,
		DescriptionOverride:   input.DescriptionOverride,
		PolicyConstraint:      input.PolicyConstraint,
		ConcurrencyConstraint: input.ConcurrencyConstraint,
	})
}

func validateBindingToolsAndNames(
	ctx context.Context,
	profile profiledomain.Profile,
	tools ToolLookup,
) error {
	type key struct {
		providerID catalogdomain.ProviderID
		name       string
	}

	seenDirectNames := make(map[key]catalogdomain.ToolID)
	for _, binding := range profile.Bindings() {
		tool, err := tools.GetTool(ctx, binding.ToolID())
		if err != nil {
			return fmt.Errorf("resolve %s: %w", binding.ToolID(), err)
		}
		if !binding.Direct() {
			continue
		}

		name := binding.Alias()
		if name == "" {
			name = tool.Name()
		}
		collisionKey := key{
			providerID: tool.ProviderID(),
			name:       name,
		}
		if existing, exists := seenDirectNames[collisionKey]; exists &&
			existing != binding.ToolID() {
			return fmt.Errorf(
				"%w: provider %s name %q used by %s and %s",
				ErrBindingNameConflict,
				tool.ProviderID(),
				name,
				existing,
				binding.ToolID(),
			)
		}
		seenDirectNames[collisionKey] = binding.ToolID()
	}
	return nil
}
