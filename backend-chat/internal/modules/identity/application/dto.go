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
