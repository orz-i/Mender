package bootstrap

import (
	"context"

	profiledelivery "github.com/orz-i/Mender/backend/internal/profile/application/delivery"
	profiledomain "github.com/orz-i/Mender/backend/internal/profile/domain"
)

// LocalProfileCLI remains the management-facing command recipe payload.
type LocalProfileCLI = profiledelivery.LocalProfileCLI

func (r *Runtime) LocalProfileCLI(ctx context.Context, slug profiledomain.Slug) (LocalProfileCLI, error) {
	return r.profileDelivery.LocalProfileCLI(ctx, slug)
}
