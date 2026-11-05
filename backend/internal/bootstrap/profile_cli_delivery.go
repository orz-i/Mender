package bootstrap

import (
	"context"

	profiledelivery "github.com/orz-i/Mender/backend/internal/profile/application/delivery"
	profiledomain "github.com/orz-i/Mender/backend/internal/profile/domain"
)

func (r *Runtime) LocalProfileCLI(ctx context.Context, slug profiledomain.Slug) (profiledelivery.LocalProfileCLI, error) {
	return r.profileDelivery.LocalProfileCLI(ctx, slug)
}
