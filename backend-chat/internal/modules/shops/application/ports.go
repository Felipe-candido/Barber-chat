package application

import (
	"context"
	"github.com/Felipe-candido/Barber-chat/internal/modules/shops/domain"
	"github.com/google/uuid"
)

type InitialOwnerStore interface {
	// Insert if missing, preserve existing name/status, then lock the local user.
	EnsureActiveUser(context.Context, uuid.UUID, string) error
	CreateOwnerMembership(context.Context, uuid.UUID, uuid.UUID) error
	RequireActiveOwner(context.Context, uuid.UUID, uuid.UUID) error
}

type ShopCreationStore interface {
	FindCreation(context.Context, uuid.UUID, [32]byte) (ShopCreationRecord, bool, error)
	Create(context.Context, domain.Shop) error
	RecordCreation(context.Context, uuid.UUID, [32]byte, [32]byte, uuid.UUID) error
}

type ShopCreationTx struct {
	Owners InitialOwnerStore
	Shops  ShopCreationStore
}

// The callback runs once. All adapters share this transaction; success requires
// commit. Never call Auth, Stripe or another external service inside the callback.
type ShopCreationTransactor interface {
	WithinShopCreationTx(context.Context, func(context.Context, ShopCreationTx) error) error
}

// Implemented by identity infrastructure using the same transaction as settings.
// Holds locks on the active user/membership until commit, preventing stale access.
type SettingsOwnerAccess interface {
	RequireSettingsOwner(context.Context, uuid.UUID, uuid.UUID) error
}

type ShopSettingsStore interface {
	FindSettings(context.Context, uuid.UUID) (ShopSettings, bool, error)
	// Update name/contact and address atomically, only at the expected version.
	ReplaceSettings(context.Context, uuid.UUID, int64, domain.BasicSettings) (bool, error)
}

type ShopSettingsTx struct {
	Access SettingsOwnerAccess
	Shops  ShopSettingsStore
}

type ShopSettingsTransactor interface {
	WithinShopSettingsTx(context.Context, func(context.Context, ShopSettingsTx) error) error
}

type PublicationStore interface {
	LockPublicationShop(context.Context, uuid.UUID) error
	FindPublicationShop(context.Context, uuid.UUID) (PublicationShop, bool, error)
	SetPublicationRequested(context.Context, uuid.UUID, bool) error
}

type PublicationCatalogReader interface {
	ReadPublicationCatalog(context.Context, uuid.UUID) (PublicationCatalog, error)
}

type PublicationScheduleReader interface {
	HasPublicationSchedule(context.Context, uuid.UUID, []PublicationCandidate) (bool, error)
}

type CommercialCoverageReader interface {
	ReadCommercialCoverage(context.Context, uuid.UUID) (CommercialCoverage, error)
}

type PublicationTx struct {
	Access   SettingsOwnerAccess
	Shops    PublicationStore
	Catalog  PublicationCatalogReader
	Schedule PublicationScheduleReader
	Billing  CommercialCoverageReader
}

// A coherent local snapshot, with authorization held until commit. No Stripe
// calls or automatic retries: serialization/commit failures are uncertain writes.
type PublicationTransactor interface {
	WithinPublicationTx(context.Context, func(context.Context, PublicationTx) error) error
}
