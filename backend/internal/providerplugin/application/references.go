package application

import (
	"context"
	"errors"
	"fmt"

	catalogapplication "github.com/orz-i/Mender/backend/internal/catalog/application"
	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
)

func (s Service) EnsureProviderUnused(
	ctx context.Context,
	providerID catalogdomain.ProviderID,
) error {
	profiles, err := s.deps.Profiles.Execute(ctx)
	if err != nil {
		return fmt.Errorf("check provider usage: list profiles: %w", err)
	}
	for _, profile := range profiles {
		for _, binding := range profile.Bindings() {
			tool, describeErr := s.deps.Catalog.DescribeTool(ctx, binding.ToolID())
			if describeErr != nil {
				if errors.Is(describeErr, catalogapplication.ErrToolNotFound) {
					return fmt.Errorf("%w: Profile %s contains an unresolved ToolID", ErrProviderInUse, profile.Slug())
				}
				return fmt.Errorf(
					"check provider usage: inspect profile %s: %w",
					profile.Slug(),
					describeErr,
				)
			}
			if tool.ProviderID() == providerID {
				return fmt.Errorf(
					"%w: profile %s selects tool %s",
					ErrProviderInUse,
					profile.Slug(),
					tool.ID(),
				)
			}
		}
	}

	policy, err := s.deps.Policy.Execute(ctx)
	if err != nil {
		return fmt.Errorf("check provider usage: inspect Policy: %w", err)
	}
	for _, rule := range policy.Rules() {
		for _, referencedProviderID := range rule.Match().ProviderIDs() {
			if referencedProviderID == providerID.String() {
				return fmt.Errorf("%w: Policy Rule %s references Provider %s", ErrProviderInUse, rule.ID(), providerID)
			}
		}
		for _, rawToolID := range rule.Match().ToolIDs() {
			toolID, parseErr := catalogdomain.ParseToolID(rawToolID)
			if parseErr != nil {
				return fmt.Errorf("%w: Policy Rule %s has invalid ToolID", ErrProviderInUse, rule.ID())
			}
			tool, lookupErr := s.deps.Catalog.DescribeTool(ctx, toolID)
			if errors.Is(lookupErr, catalogapplication.ErrToolNotFound) {
				return fmt.Errorf("%w: Policy Rule %s refers to unresolved ToolID", ErrProviderInUse, rule.ID())
			}
			if lookupErr != nil {
				return lookupErr
			}
			if tool.ProviderID() == providerID {
				return fmt.Errorf("%w: Policy Rule %s references Tool %s", ErrProviderInUse, rule.ID(), rawToolID)
			}
		}
	}
	return nil
}
