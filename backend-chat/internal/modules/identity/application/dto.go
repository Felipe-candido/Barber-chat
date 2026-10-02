package application

import (
	"github.com/Felipe-candido/Barber-chat/internal/modules/identity/domain"
	"github.com/google/uuid"
)

// ResolveStaffIdentityInput contains the access token presented by the client.
// The token is sensitive request data and must not be logged or persisted.
type ResolveStaffIdentityInput struct {
	AccessToken string
}

// StaffIdentityOutput is the active local identity resolved from a verified
// provider subject. Provider claims and session data do not cross this boundary.
type StaffIdentityOutput struct {
	UserID      uuid.UUID
	DisplayName string
}

// ListMyShopsInput identifies the authenticated user whose accessible shops
// should be listed. UserID must come from successful authentication, never
// from a client-supplied body, query parameter or header.
type ListMyShopsInput struct {
	UserID uuid.UUID
}

// AccessibleShopOutput is a read-only summary for administrative shop selection,
// not the shop domain model. Shop data remains owned by the shops module.
// Listing a shop does not authorize later operations; each operation must
// recheck the user's access to the requested shop.
type AccessibleShopOutput struct {
	ShopID uuid.UUID
	Name   string
	Slug   string
}

// AuthorizeShopActionInput identifies the authenticated user and the shop the
// client wants to access. ShopSlug is a request, not proof of authorization.
// UserID must come from successful authentication, never from client input.
type AuthorizeShopActionInput struct {
	UserID   uuid.UUID
	ShopSlug string
}

// AuthorizedShopScope is passed to administrative use cases only after the
// user, membership and shop have all been confirmed as active.
// This is a per-operation result, not a reusable session or proof of authentication.
type AuthorizedShopScope struct {
	UserID uuid.UUID
	ShopID uuid.UUID
}

func staffIdentityOutput(user domain.User) StaffIdentityOutput {
	return StaffIdentityOutput{
		UserID:      user.ID,
		DisplayName: user.DisplayName,
	}
}
