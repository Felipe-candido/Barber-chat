package application

import (
	"context"
	"strings"
)

type ManageAppointment struct {
	transactions BookingTransactor
	codes        AppointmentCodes
	clock        Clock
}

func NewManageAppointment(tx BookingTransactor, codes AppointmentCodes, clock Clock) *ManageAppointment {
	return &ManageAppointment{tx, codes, clock}
}

func (uc *ManageAppointment) Lookup(ctx context.Context, input AppointmentCodeInput) (AppointmentDetails, error) {
	return uc.execute(ctx, input, false)
}
func (uc *ManageAppointment) Cancel(ctx context.Context, input AppointmentCodeInput) (AppointmentDetails, error) {
	return uc.execute(ctx, input, true)
}

func (uc *ManageAppointment) execute(ctx context.Context, input AppointmentCodeInput, cancel bool) (AppointmentDetails, error) {
	digest, err := uc.codes.Hash(input.Code)
	if err != nil {
		return AppointmentDetails{}, ErrAppointmentNotFound
	}
	var output AppointmentDetails
	err = uc.transactions.WithinBookingTx(ctx, func(ctx context.Context, tx BookingTx) error {
		shop, err := resolveShop(ctx, tx.Shops, strings.TrimSpace(input.ShopSlug))
		if err != nil {
			return err
		}
		appointment, found, err := tx.Appointments.FindByCode(ctx, shop.ShopID, digest)
		if err != nil {
			return err
		}
		if !found {
			return ErrAppointmentNotFound
		}
		if appointment.ShopID != shop.ShopID {
			return ErrInconsistentBookingData
		}
		if cancel {
			// Match creation's lock order: professional first, appointment second.
			if err := tx.Schedule.LockProfessionalSchedule(ctx, shop.ShopID, appointment.ProfessionalID); err != nil {
				return err
			}
			appointment, found, err = tx.Appointments.FindByCode(ctx, shop.ShopID, digest)
			if err != nil {
				return err
			}
			if !found {
				return ErrAppointmentNotFound
			}
			updated, err := appointment.Cancel(uc.clock.Now())
			if err != nil {
				return err
			}
			if updated.Version != appointment.Version {
				if err := tx.Appointments.SaveCancellation(ctx, updated); err != nil {
					return err
				}
			}
			appointment = updated
		}
		output = AppointmentDetails{Appointment: receiptOf(appointment, shop.Name), CanCancel: appointment.CanCancel(uc.clock.Now())}
		return nil
	})
	if err != nil {
		return AppointmentDetails{}, err
	}
	return output, nil
}
