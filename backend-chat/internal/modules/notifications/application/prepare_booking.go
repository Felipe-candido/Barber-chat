package application

import (
	"context"
	"fmt"

	"github.com/Felipe-candido/Barber-chat/internal/modules/notifications/domain"
	"github.com/google/uuid"
)

type PrepareBookingNotifications struct {
	sealer PayloadSealer
}

func NewPrepareBookingNotifications(sealer PayloadSealer) *PrepareBookingNotifications {
	return &PrepareBookingNotifications{sealer: sealer}
}

// Execute calculates the plan and protects its snapshot. It deliberately has no
// database commit, provider call or clock; the future booking adapter supplies
// authoritative data and persists the result within the booking transaction.
func (uc *PrepareBookingNotifications) Execute(ctx context.Context, input PrepareBookingNotificationsInput) (PreparedBookingNotifications, error) {
	if err := ctx.Err(); err != nil {
		return PreparedBookingNotifications{}, err
	}
	if input.ShopID == uuid.Nil || input.AppointmentID == uuid.Nil || input.ScheduleGeneration < 1 {
		return PreparedBookingNotifications{}, ErrInvalidPreparation
	}
	schedule, err := input.Policy.Schedule(input.CreatedAt, input.Snapshot.StartsAt)
	if err != nil {
		return PreparedBookingNotifications{}, err
	}
	if schedule.SkipReason != "" {
		return PreparedBookingNotifications{Schedule: schedule}, nil
	}
	if !input.PermissionGranted {
		return PreparedBookingNotifications{Schedule: domain.Schedule{
			PolicyVersion: input.Policy.Version, SkipReason: domain.SkipPermissionMissing,
		}}, nil
	}
	if !input.Snapshot.Valid() {
		return PreparedBookingNotifications{}, ErrInvalidSnapshot
	}
	if uc == nil || uc.sealer == nil {
		return PreparedBookingNotifications{}, ErrInvalidPreparation
	}
	scope := PayloadScope{
		ShopID: input.ShopID, AppointmentID: input.AppointmentID,
		ScheduleGeneration: input.ScheduleGeneration, ExpiresAt: schedule.RetainUntil(),
	}
	snapshot := input.Snapshot
	snapshot.StartsAt, snapshot.EndsAt = snapshot.StartsAt.UTC(), snapshot.EndsAt.UTC()
	payload, err := uc.sealer.Seal(ctx, scope, snapshot)
	if err != nil {
		return PreparedBookingNotifications{}, fmt.Errorf("protect booking notification payload: %w", err)
	}
	return PreparedBookingNotifications{Schedule: schedule, Scope: scope, Payload: payload}, nil
}
