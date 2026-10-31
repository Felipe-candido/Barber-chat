package application_test

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	app "github.com/Felipe-candido/Barber-chat/internal/modules/booking/application"
	"github.com/Felipe-candido/Barber-chat/internal/modules/booking/domain"
	contacts "github.com/Felipe-candido/Barber-chat/internal/modules/customers/infra"
	"github.com/google/uuid"
)

type staffBookingFake struct {
	shop                                  app.ShopContext
	actor, professional                   uuid.UUID
	allowed                               bool
	steps                                 []string
	newRequest, omitEligibility, eligible bool
	eligibilityErr                        error
}

func (f *staffBookingFake) WithinBookingTx(ctx context.Context, fn func(context.Context, app.BookingTx) error) error {
	var eligibility app.StaffBookingEligibility = f
	if f.omitEligibility {
		eligibility = nil
	}
	return fn(ctx, app.BookingTx{Shops: f, Schedule: f, Staff: f, StaffEligibility: eligibility, Catalog: f, Idempotency: f})
}
func (f *staffBookingFake) ResolveActiveShop(context.Context, string) (app.ShopContext, bool, error) {
	return f.shop, true, nil
}
func (f *staffBookingFake) LockProfessionalSchedule(_ context.Context, shop, professional uuid.UUID) error {
	if shop != f.shop.ShopID || professional != f.professional {
		return errors.New("wrong calendar")
	}
	f.steps = append(f.steps, "calendar")
	return nil
}
func (f *staffBookingFake) LoadCalendar(context.Context, app.AvailabilityQuery) (domain.Calendar, bool, error) {
	panic("a replay must not create a second booking")
}
func (f *staffBookingFake) CanCreateBooking(_ context.Context, actor, shop, professional uuid.UUID) (bool, error) {
	if actor != f.actor || shop != f.shop.ShopID || professional != f.professional {
		return false, errors.New("wrong scope")
	}
	f.steps = append(f.steps, "permission")
	return f.allowed, nil
}
func (f *staffBookingFake) ClaimCreateAppointment(context.Context, app.IdempotencyClaim) (app.ClaimedReceipt, bool, error) {
	f.steps = append(f.steps, "claim")
	return app.ClaimedReceipt{Code: "original-code"}, !f.newRequest, nil
}

func (f *staffBookingFake) CanCreateStaffBooking(_ context.Context, shop uuid.UUID) (bool, error) {
	if shop != f.shop.ShopID {
		return false, errors.New("wrong commercial scope")
	}
	f.steps = append(f.steps, "coverage")
	return f.eligible, f.eligibilityErr
}

func (f *staffBookingFake) FindBookableSelection(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) (app.BookableSelection, bool, error) {
	f.steps = append(f.steps, "catalog")
	return app.BookableSelection{}, false, nil
}

func (f *staffBookingFake) ListBookableProfessionals(context.Context, uuid.UUID, uuid.UUID) ([]app.Professional, error) {
	panic("not used by creation")
}
func (f *staffBookingFake) CompleteCreateAppointment(context.Context, uuid.UUID, string, app.ClaimedReceipt) error {
	panic("replay must not write")
}

func TestStaffReplayStillRechecksCurrentPermission(t *testing.T) {
	for _, allowed := range []bool{true, false} {
		t.Run(map[bool]string{true: "active", false: "revoked"}[allowed], func(t *testing.T) {
			f := &staffBookingFake{shop: app.ShopContext{ShopID: uuid.New(), Slug: "shop", Name: "Shop", Timezone: "UTC", SelfService: true}, actor: uuid.New(), professional: uuid.New(), allowed: allowed}
			uc := app.NewCreateAppointment(f, contacts.ContactNormalizer{}, nil, app.ClockFunc(time.Now))
			input := app.CreateAppointmentInput{ShopSlug: "shop", ServiceID: uuid.New(), ProfessionalID: f.professional, StartsAt: time.Date(2026, 10, 20, 9, 30, 0, 0, time.UTC), IdempotencyKey: strings.Repeat("a", 64), Customer: app.CustomerInput{FirstName: "Test", LastName: "Customer", Phone: "11912345678", PhoneCountry: "BR"}}
			output, err := uc.ExecuteForStaff(context.Background(), input, f.actor, f.shop.ShopID, f.professional)
			if allowed {
				if err != nil || !output.Replayed || output.Code != "original-code" {
					t.Fatal(output, err)
				}
			} else if !errors.Is(err, app.ErrStaffBookingAccessDenied) || output.Code != "" {
				t.Fatal("revoked access received a receipt", output, err)
			}
			if !reflect.DeepEqual(f.steps, []string{"claim", "calendar", "permission"}) {
				t.Fatal("lock order changed", f.steps)
			}
			f.steps = nil
			input.ProfessionalID = uuid.New()
			if _, err = uc.ExecuteForStaff(context.Background(), input, f.actor, f.shop.ShopID, f.professional); !errors.Is(err, app.ErrStaffBookingAccessDenied) || len(f.steps) != 0 {
				t.Fatal("barber could choose another professional", err)
			}
			input.ProfessionalID = f.professional
			if _, err = uc.ExecuteForStaff(context.Background(), input, f.actor, uuid.New(), f.professional); !errors.Is(err, app.ErrStaffBookingAccessDenied) || len(f.steps) != 0 {
				t.Fatal("cross-tenant booking accepted", err)
			}
		})
	}
}

func TestStaffBookingCommercialGate(t *testing.T) {
	dependencyErr := errors.New("financial read failed")
	for _, test := range []struct {
		name                           string
		selfService, eligible, missing bool
		failure, want                  error
		steps                          []string
	}{
		{"unpaid", true, false, false, nil, app.ErrBookingSubscriptionRequired, []string{"claim", "calendar", "permission", "coverage"}},
		{"paid", true, true, false, nil, app.ErrBookingOptionNotFound, []string{"claim", "calendar", "permission", "coverage", "catalog"}},
		{"missing adapter fails closed", true, false, true, nil, app.ErrBookingEligibilityUnavailable, []string{"claim", "calendar", "permission"}},
		{"dependency failure is not payment denial", true, false, false, dependencyErr, app.ErrBookingEligibilityUnavailable, []string{"claim", "calendar", "permission", "coverage"}},
		{"manual shops retain legacy behavior", false, false, true, nil, app.ErrBookingOptionNotFound, []string{"claim", "calendar", "permission", "catalog"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			f := &staffBookingFake{shop: app.ShopContext{ShopID: uuid.New(), Slug: "shop", Name: "Shop", Timezone: "UTC", SelfService: test.selfService},
				actor: uuid.New(), professional: uuid.New(), allowed: true, newRequest: true, eligible: test.eligible, omitEligibility: test.missing, eligibilityErr: test.failure}
			uc := app.NewCreateAppointment(f, contacts.ContactNormalizer{}, nil, app.ClockFunc(time.Now))
			input := app.CreateAppointmentInput{ShopSlug: "shop", ServiceID: uuid.New(), ProfessionalID: f.professional, StartsAt: time.Date(2026, 10, 20, 9, 30, 0, 0, time.UTC), IdempotencyKey: strings.Repeat("a", 64), Customer: app.CustomerInput{FirstName: "Test", LastName: "Customer", Phone: "11912345678", PhoneCountry: "BR"}}
			result, err := uc.ExecuteForStaff(context.Background(), input, f.actor, f.shop.ShopID, f.professional)
			if !errors.Is(err, test.want) || result.Code != "" || !reflect.DeepEqual(f.steps, test.steps) {
				t.Fatal(result, err, f.steps)
			}
			if test.failure != nil && !errors.Is(err, test.failure) {
				t.Fatal("operational cause lost", err)
			}
		})
	}
}
