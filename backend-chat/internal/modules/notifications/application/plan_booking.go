package application

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

var ErrPlanningUnavailable = errors.New("booking notification planning unavailable")

type BookingPermission struct {
	ShopID, AppointmentID, CustomerID, ActorID uuid.UUID
	Allowed                                    bool
	// ActorID is nil for public bookings; staff identity comes from authentication.
	RecordedAt time.Time
}

type PlanBookingInput struct {
	Preparation PrepareBookingNotificationsInput
	CustomerID  uuid.UUID
	ActorID     uuid.UUID
}

type MessageShop struct {
	ShopID                        uuid.UUID
	Name, Slug, Address, Timezone string
}

// All stores must share the booking transaction. This use case neither opens
// a transaction nor sends externally.
type PlanBookingNotifications struct {
	enabled     bool
	prepare     *PrepareBookingNotifications
	store       PlanningStore
	permissions BookingPermissionRecorder
	shops       MessageShopReader
}

func NewPlanBookingNotifications(enabled bool, prepare *PrepareBookingNotifications, store PlanningStore, permissions BookingPermissionRecorder, shops MessageShopReader) *PlanBookingNotifications {
	return &PlanBookingNotifications{enabled, prepare, store, permissions, shops}
}

func (uc *PlanBookingNotifications) Execute(ctx context.Context, input PlanBookingInput) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if uc == nil {
		return ErrPlanningUnavailable
	}
	// The deployment switch keeps existing booking usable before migration/setup.
	// Database invalidation remains independent of this switch.
	if !uc.enabled {
		return nil
	}
	p := input.Preparation
	if p.ShopID == uuid.Nil || p.AppointmentID == uuid.Nil || input.CustomerID == uuid.Nil ||
		p.ScheduleGeneration != 1 || !validTime(p.CreatedAt) || uc.prepare == nil ||
		uc.store == nil || uc.permissions == nil || uc.shops == nil {
		return ErrInvalidPreparation
	}
	policy, err := uc.store.Policy(ctx, p.ShopID)
	if err != nil {
		return fmt.Errorf("read booking notification policy: %w", err)
	}
	p.Policy = policy
	if policy.Enabled && p.PermissionGranted {
		shop, found, err := uc.shops.FindMessageShop(ctx, p.ShopID)
		if err != nil {
			return fmt.Errorf("read notification shop: %w", err)
		}
		if !found || shop.ShopID != p.ShopID || shop.Slug != p.Snapshot.ShopSlug || shop.Timezone != p.Snapshot.ShopTimezone {
			return ErrInvalidPreparation
		}
		p.Snapshot.ShopName, p.Snapshot.ShopAddress = shop.Name, shop.Address
	}
	prepared, err := uc.prepare.Execute(ctx, p)
	if err != nil {
		return err
	}
	if err := uc.permissions.RecordBookingPermission(ctx, BookingPermission{
		ShopID: p.ShopID, AppointmentID: p.AppointmentID, CustomerID: input.CustomerID, ActorID: input.ActorID,
		Allowed: p.PermissionGranted, RecordedAt: p.CreatedAt,
	}); err != nil {
		return fmt.Errorf("record booking message permission: %w", err)
	}
	if err := uc.store.Save(ctx, p, prepared); err != nil {
		return fmt.Errorf("save booking notification plan: %w", err)
	}
	return nil
}
