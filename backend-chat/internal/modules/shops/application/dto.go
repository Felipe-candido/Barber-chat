package application

import (
	"github.com/Felipe-candido/Barber-chat/internal/modules/shops/domain"
	"github.com/google/uuid"
)

// OwnerID comes only from verified registration context. One confirmed Supabase
// account can create its own shop, never claim an existing shop or choose a role.
type CreateShopInput struct {
	OwnerID                                           uuid.UUID
	DisplayName, Name, Slug, Timezone, IdempotencyKey string
	Address                                           domain.Address
}

type CreatedShop struct {
	Shop     domain.Shop
	Replayed bool
}
type ShopCreationRecord struct {
	Shop        domain.Shop
	Fingerprint [32]byte
}

// Address is nil only for legacy shops whose business address is not registered.
// Reading legacy settings is allowed; every settings update requires an address.
type ShopSettings struct {
	ShopID                                   uuid.UUID
	Name, Slug, Timezone, Phone, Description string
	Address                                  *domain.Address
	Version                                  int64
}

type SettingsScope struct {
	// Both IDs come from the verified HTTP scope, never from request JSON.
	ActorID, ShopID uuid.UUID
}

type UpdateShopSettingsInput struct {
	Scope           SettingsScope
	ExpectedVersion int64
	Settings        domain.BasicSettings
}
