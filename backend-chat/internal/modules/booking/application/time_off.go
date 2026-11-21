package application

import (
	"context"
	"errors"
	"time"

	"github.com/Felipe-candido/Barber-chat/internal/modules/booking/domain"
	"github.com/google/uuid"
)

var (
	ErrTimeOffAppointments = errors.New("time off overlaps existing appointments")
	ErrTimeOffOverlap      = errors.New("time off overlaps another block")
	ErrTimeOffConflict     = errors.New("time off ID was already used")
)

type TimeOffBlock struct {
	ID               uuid.UUID
	ProfessionalID   uuid.UUID
	ProfessionalName string
	StartsAt         time.Time
	EndsAt           time.Time
}
type TimeOffProfessional struct {
	ID   uuid.UUID
	Name string
}
type TimeOffOutput struct {
	Today         string
	MaxDate       string
	ShopTimezone  string
	Professionals []TimeOffProfessional
	Blocks        []TimeOffBlock
}
type TimeOffStore interface {
	LockProfessionalSchedule(context.Context, uuid.UUID, uuid.UUID) error
	ListTimeOffProfessionals(context.Context, uuid.UUID, uuid.UUID) ([]TimeOffProfessional, error)
	ListTimeOff(context.Context, uuid.UUID, uuid.UUID, time.Time) ([]TimeOffBlock, error)
	FindTimeOff(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) (TimeOffBlock, bool, error)
	CreateTimeOff(context.Context, uuid.UUID, TimeOffBlock) error
	DeleteTimeOff(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) error
}
type TimeOffTx struct {
	Access StaffBookingAuthorizer
	Shops  AgendaShopReader
	Store  TimeOffStore
}
type TimeOffTransactor interface {
	WithinTimeOffTx(context.Context, func(context.Context, TimeOffTx) error) error
}
type ManageTimeOff struct {
	transactions TimeOffTransactor
	clock        Clock
}

func NewManageTimeOff(tx TimeOffTransactor, clock Clock) *ManageTimeOff {
	return &ManageTimeOff{tx, clock}
}

func requireTimeOffAccess(ctx context.Context, tx TimeOffTx, scope StaffBookingScope, professional uuid.UUID) error {
	if scope.UserID == uuid.Nil || scope.ShopID == uuid.Nil || (scope.ProfessionalID != uuid.Nil && professional != scope.ProfessionalID) {
		return ErrStaffBookingAccessDenied
	}
	allowed, err := tx.Access.CanCreateBooking(ctx, scope.UserID, scope.ShopID, professional)
	if err != nil {
		return err
	}
	if !allowed {
		return ErrStaffBookingAccessDenied
	}
	return nil
}

func (uc *ManageTimeOff) List(ctx context.Context, scope StaffBookingScope) (TimeOffOutput, error) {
	var out TimeOffOutput
	err := uc.transactions.WithinTimeOffTx(ctx, func(ctx context.Context, tx TimeOffTx) error {
		if err := requireTimeOffAccess(ctx, tx, scope, scope.ProfessionalID); err != nil {
			return err
		}
		shop, found, err := tx.Shops.FindAgendaShop(ctx, scope.ShopID)
		if err != nil {
			return err
		}
		if !found {
			return ErrShopNotFound
		}
		loc, err := time.LoadLocation(shop.Timezone)
		if err != nil || shop.Timezone == "" || shop.ShopID != scope.ShopID {
			return ErrInconsistentBookingData
		}
		now := uc.clock.Now().In(loc)
		out = TimeOffOutput{Today: now.Format("2006-01-02"), MaxDate: now.AddDate(2, 0, 0).Format("2006-01-02"), ShopTimezone: shop.Timezone}
		out.Professionals, err = tx.Store.ListTimeOffProfessionals(ctx, scope.ShopID, scope.ProfessionalID)
		if err != nil {
			return err
		}
		out.Blocks, err = tx.Store.ListTimeOff(ctx, scope.ShopID, scope.ProfessionalID, now)
		return err
	})
	return out, err
}

// The resource UUID is chosen once by the client. Repeating the same PUT recovers
// an uncertain response without creating another block or changing an existing one.
func (uc *ManageTimeOff) Save(ctx context.Context, scope StaffBookingScope, professional, id uuid.UUID, from, through string) (TimeOffBlock, error) {
	var out TimeOffBlock
	if scope.UserID == uuid.Nil || scope.ShopID == uuid.Nil {
		return out, ErrStaffBookingAccessDenied
	}
	if id == uuid.Nil || professional == uuid.Nil {
		return out, domain.ErrInvalidTimeOff
	}
	if scope.ProfessionalID != uuid.Nil && scope.ProfessionalID != professional {
		return out, ErrStaffBookingAccessDenied
	}
	err := uc.transactions.WithinTimeOffTx(ctx, func(ctx context.Context, tx TimeOffTx) error {
		if err := tx.Store.LockProfessionalSchedule(ctx, scope.ShopID, professional); err != nil {
			return err
		}
		if err := requireTimeOffAccess(ctx, tx, scope, professional); err != nil {
			return err
		}
		shop, found, err := tx.Shops.FindAgendaShop(ctx, scope.ShopID)
		if err != nil {
			return err
		}
		if !found {
			return ErrShopNotFound
		}
		period, err := domain.TimeOffInterval(from, through, shop.Timezone)
		if err != nil {
			return err
		}
		previous, found, err := tx.Store.FindTimeOff(ctx, scope.ShopID, professional, id)
		if err != nil {
			return err
		}
		if found {
			if !previous.StartsAt.Equal(period.StartsAt) || !previous.EndsAt.Equal(period.EndsAt) {
				return ErrTimeOffConflict
			}
			out = previous
			return nil
		}
		loc, _ := time.LoadLocation(shop.Timezone)
		now := uc.clock.Now().In(loc)
		if from < now.Format("2006-01-02") || through > now.AddDate(2, 0, 0).Format("2006-01-02") {
			return domain.ErrInvalidTimeOff
		}
		out = TimeOffBlock{ID: id, ProfessionalID: professional, StartsAt: period.StartsAt, EndsAt: period.EndsAt}
		if err := tx.Store.CreateTimeOff(ctx, scope.ShopID, out); err != nil {
			return err
		}
		out, found, err = tx.Store.FindTimeOff(ctx, scope.ShopID, professional, id)
		if err == nil && !found {
			return ErrInconsistentBookingData
		}
		return err
	})
	return out, err
}

func (uc *ManageTimeOff) Delete(ctx context.Context, scope StaffBookingScope, professional, id uuid.UUID) error {
	if scope.UserID == uuid.Nil || scope.ShopID == uuid.Nil {
		return ErrStaffBookingAccessDenied
	}
	if professional == uuid.Nil || id == uuid.Nil {
		return domain.ErrInvalidTimeOff
	}
	if scope.ProfessionalID != uuid.Nil && scope.ProfessionalID != professional {
		return ErrStaffBookingAccessDenied
	}
	return uc.transactions.WithinTimeOffTx(ctx, func(ctx context.Context, tx TimeOffTx) error {
		if err := tx.Store.LockProfessionalSchedule(ctx, scope.ShopID, professional); err != nil {
			return err
		}
		if err := requireTimeOffAccess(ctx, tx, scope, professional); err != nil {
			return err
		}
		return tx.Store.DeleteTimeOff(ctx, scope.ShopID, professional, id)
	})
}
