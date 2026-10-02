package application

import (
	"context"

	"github.com/Felipe-candido/Barber-chat/internal/modules/identity/domain"
	"github.com/google/uuid"
)

// AccessTokenVerifier authenticates an access token and returns its subject as
// the provider user UUID. Implementations must validate the signature, allowed
// algorithm, issuer, audience and expiration before returning an ID.
// Invalid or expired credentials must return or wrap ErrInvalidAccessToken.
// Operational failures (such as a JWKS fetch failure) and context cancellation
// must remain distinct errors, preserving their causes for errors.Is/errors.As.
// Errors must not include tokens or other credentials.
type AccessTokenVerifier interface {
	VerifyAccessToken(context.Context, string) (uuid.UUID, error)
}

// IdentityRepository provides the local identity data required by the planned
// identity resolution and shop authorization use cases. Find methods return
// found=false only when the requested record does not exist; inactive records
// are returned so the application layer can explicitly deny access.
// FindUserByID must return the requested user ID when found is true.
// FindMembership takes userID then shopID and must return that exact pair.
// ListMembershipsByUser includes inactive records and returns a non-nil empty
// slice when there are no memberships. Database failures remain errors.
type IdentityRepository interface {
	FindUserByID(context.Context, uuid.UUID) (domain.User, bool, error)
	FindMembership(context.Context, uuid.UUID, uuid.UUID) (domain.Membership, bool, error)
	ListMembershipsByUser(context.Context, uuid.UUID) ([]domain.Membership, error)
}

// AccessibleShopReader provides the read-only projection needed for shop
// selection. Only an active user with an active membership in an active shop
// is eligible. The user ID must come from successful authentication.
// Results contain each eligible shop once, ordered by name then shop ID.
// No eligible shops yields a non-nil empty slice; database failures remain errors.
// This port grants no lasting authorization and does not own shop mutations.
type AccessibleShopReader interface {
	ListActiveShopsByUser(context.Context, uuid.UUID) ([]AccessibleShopOutput, error)
}

// ShopResolver resolves an active shop requested by its public slug. Resolving
// a shop does not authorize the user; authorization still requires an active
// user and an active membership for the returned shop ID.
// Missing or inactive shops return found=false. A found shop has a nonzero UUID.
type ShopResolver interface {
	LookupActiveShop(context.Context, string) (uuid.UUID, bool, error)
}
