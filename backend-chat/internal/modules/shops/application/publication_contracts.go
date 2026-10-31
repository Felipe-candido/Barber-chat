package application

import (
	"errors"
	"time"

	"github.com/Felipe-candido/Barber-chat/internal/modules/shops/domain"
	"github.com/google/uuid"
)

var (
	ErrPublicationBlocked    = errors.New("shop publication prerequisites are not met")
	ErrPublicationNotManaged = errors.New("legacy shop publication is operator managed")
)

type PublicationShop struct {
	ShopID                 uuid.UUID
	Slug                   string
	SelfService, Requested bool
	Address                *domain.Address
}

// Minimum eligible duration per professional; catalog owns service eligibility.
type PublicationCandidate struct {
	ProfessionalID  uuid.UUID
	DurationMinutes int
}

type PublicationCatalog struct {
	HasService bool
	Candidates []PublicationCandidate
}

// Financial projection only. No provider IDs, secrets or network calls.
type CommercialCoverage struct {
	Status            string     `json:"status"`
	PlanKey           string     `json:"plan_key"`
	Allowed           bool       `json:"operation_allowed"`
	Reason            string     `json:"reason"`
	Until             *time.Time `json:"coverage_until"`
	CancelAtPeriodEnd bool       `json:"cancel_at_period_end"`
}

type PublicationReadiness struct {
	Address      bool `json:"address"`
	Service      bool `json:"service"`
	Professional bool `json:"professional"`
	Schedule     bool `json:"schedule"`
}

type ShopCommercialStatus struct {
	ShopID               uuid.UUID            `json:"shop_id"`
	Slug                 string               `json:"slug"`
	PublicationManaged   bool                 `json:"publication_managed"`
	Requested            bool                 `json:"publication_requested"`
	PublicBookingEnabled bool                 `json:"public_booking_enabled"`
	CanPublish           bool                 `json:"can_publish"`
	Blockers             []string             `json:"blockers"`
	Readiness            PublicationReadiness `json:"readiness"`
	Subscription         CommercialCoverage   `json:"subscription"`
}
