// Package requestctx carries verified identity and authorized shop data between
// HTTP middleware and handlers. Business use cases receive these values explicitly.
package requestctx

import (
	"context"

	identityapp "github.com/Felipe-candido/Barber-chat/internal/modules/identity/application"
)

type staffIdentityKey struct{}
type shopScopeKey struct{}

// WithStaffIdentity returns a child context containing a copy of identity.
// The caller must obtain identity from successful authentication first.
// Storing an identity does not grant shop access.
func WithStaffIdentity(
	ctx context.Context,
	identity identityapp.StaffIdentityOutput,
) context.Context {
	return context.WithValue(ctx, staffIdentityKey{}, identity)
}

// StaffIdentityFromContext reports whether a typed identity is present.
// It does not verify credentials or recheck the user's current active state.
func StaffIdentityFromContext(
	ctx context.Context,
) (identityapp.StaffIdentityOutput, bool) {
	identity, ok := ctx.Value(staffIdentityKey{}).(identityapp.StaffIdentityOutput)

	return identity, ok
}

// WithShopScope returns a child context containing a copy of scope.
// The caller must obtain scope from successful shop authorization first.
// The scope belongs to the current operation; it is not a reusable session.
func WithShopScope(
	ctx context.Context,
	scope identityapp.AuthorizedShopScope,
) context.Context {
	return context.WithValue(ctx, shopScopeKey{}, scope)
}

// ShopScopeFromContext reports whether a typed authorized scope is present.
// It does not query or validate memberships; consumers must check the bool.
func ShopScopeFromContext(
	ctx context.Context,
) (identityapp.AuthorizedShopScope, bool) {
	scope, ok := ctx.Value(shopScopeKey{}).(identityapp.AuthorizedShopScope)

	return scope, ok
}
