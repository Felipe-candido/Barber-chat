package application

import (
	"context"
	"errors"
	"strings"

	"github.com/Felipe-candido/Barber-chat/internal/modules/shops/domain"
	"github.com/google/uuid"
)

var ErrPublicShopNotFound = errors.New("public shop not found")

type PublicShopState struct {
	ShopID                                           uuid.UUID
	Active, PublicationRequested, LegacyPublicAccess bool
}

type PublicShopReader interface {
	FindPublicShopState(context.Context, string) (PublicShopState, bool, error)
}

type OperationEligibility interface {
	CanOperateShop(context.Context, uuid.UUID) (bool, error)
}

type CheckPublicBooking struct {
	reader      PublicShopReader
	eligibility OperationEligibility
}

func NewCheckPublicBooking(reader PublicShopReader, eligibility OperationEligibility) *CheckPublicBooking {
	return &CheckPublicBooking{reader: reader, eligibility: eligibility}
}

// CanCreatePublicBooking implements booking's narrow authorization boundary.
// The caller supplies a transaction-bound reader/eligibility and resolved slug.
func (uc *CheckPublicBooking) CanCreatePublicBooking(ctx context.Context, slug string) (bool, error) {
	err := uc.Execute(ctx, slug)
	if errors.Is(err, ErrPublicShopNotFound) {
		return false, nil
	}
	return err == nil, err
}

// Transitional gate: legacy operator-created shops keep existing behavior.
// Self-service shops require explicit publication and current coverage. Readiness
// is validated by ShopPublication.Set; actual availability is checked by booking.
func (uc *CheckPublicBooking) Execute(ctx context.Context, slug string) error {
	slug = strings.TrimSpace(slug)
	if slug == "" || len(slug) > 100 {
		return ErrPublicShopNotFound
	}
	state, found, err := uc.reader.FindPublicShopState(ctx, slug)
	if err != nil {
		return err
	}
	if !found || state.ShopID == uuid.Nil || !state.Active {
		return ErrPublicShopNotFound
	}
	if state.LegacyPublicAccess {
		return nil
	}
	if !state.PublicationRequested {
		return ErrPublicShopNotFound
	}
	if uc.eligibility == nil {
		return ErrPublicShopNotFound
	}
	allowed, err := uc.eligibility.CanOperateShop(ctx, state.ShopID)
	if err != nil {
		return err
	}
	publication := domain.Publication{Requested: state.PublicationRequested}
	if !publication.PublicBookingEnabled(state.Active, allowed) {
		return ErrPublicShopNotFound
	}
	return nil
}
