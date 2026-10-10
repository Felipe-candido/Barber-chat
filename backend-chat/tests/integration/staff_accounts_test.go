//go:build integration

package integration

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	booking "github.com/Felipe-candido/Barber-chat/internal/modules/booking/application"
	bookingpostgres "github.com/Felipe-candido/Barber-chat/internal/modules/booking/infra/postgres"
	"github.com/Felipe-candido/Barber-chat/internal/modules/booking/infra/security"
	contacts "github.com/Felipe-candido/Barber-chat/internal/modules/customers/infra"
	identity "github.com/Felipe-candido/Barber-chat/internal/modules/identity/application"
	identitypostgres "github.com/Felipe-candido/Barber-chat/internal/modules/identity/infra/postgres"
	"github.com/google/uuid"
)

func TestStaffAccountReservationAndScopedBookings(t *testing.T) {
	f := newBookingFixture(t)
	other := newBookingFixture(t)
	owner, user := uuid.New(), uuid.New()
	if _, err := f.pool.Exec(f.ctx, "INSERT INTO users(id,display_name) VALUES($1,'Owner');", owner); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(f.ctx, "INSERT INTO shop_memberships(user_id,shop_id,role) VALUES($1,$2,'owner')", owner, f.shop); err != nil {
		t.Fatal(err)
	}
	repo := identitypostgres.NewAccountRepository(f.pool)
	var wg sync.WaitGroup
	results := make(chan identity.StaffAccount, 2)
	failures := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			account, err := repo.ReserveAccount(f.ctx, owner, f.shop, f.professional, "barber@example.com")
			if err != nil {
				failures <- err
			} else {
				results <- account
			}
		}()
	}
	wg.Wait()
	close(results)
	close(failures)
	if len(results) != 1 || len(failures) != 1 {
		t.Fatal("concurrent requests did not reserve exactly once")
	}
	account := <-results
	if err := <-failures; !errors.Is(err, identity.ErrAccountExists) {
		t.Fatal(err)
	}
	if _, err := repo.ReserveAccount(f.ctx, owner, other.shop, other.professional, "other@example.com"); !errors.Is(err, identity.ErrShopAccessDenied) {
		t.Fatal("owner accessed another shop", err)
	}
	linked, err := repo.LinkAccount(f.ctx, owner, f.shop, account.ID, identity.ProviderUser{ID: user, Email: account.Email})
	if err != nil || !linked.Active || linked.UserID != user {
		t.Fatal(linked, err)
	}
	if _, err = repo.ListAccounts(f.ctx, user, f.shop); !errors.Is(err, identity.ErrShopAccessDenied) {
		t.Fatal("barber accessed account management", err)
	}
	if _, err = repo.LinkAccount(f.ctx, owner, f.shop, account.ID, identity.ProviderUser{ID: uuid.New(), Email: account.Email}); !errors.Is(err, identity.ErrAccountExists) {
		t.Fatal("linked account overwritten", err)
	}
	uc := booking.NewCreateAppointment(bookingpostgres.NewTransactor(f.pool, bindBookingQueries), contacts.ContactNormalizer{}, security.Codes{}, booking.ClockFunc(time.Now))
	input := booking.CreateAppointmentInput{ShopSlug: f.slug, ServiceID: f.service, ProfessionalID: f.professional, StartsAt: f.start, IdempotencyKey: bookingKey(), Customer: booking.CustomerInput{FirstName: "Test", LastName: "Customer", Phone: "11912345678", PhoneCountry: "BR"}}
	created, err := uc.ExecuteForStaff(f.ctx, input, user, f.shop, f.professional)
	if err != nil || created.Appointment.AppointmentID == uuid.Nil {
		t.Fatal(created, err)
	}
	if err = repo.SetAccountActive(f.ctx, owner, f.shop, account.ID, false); err != nil {
		t.Fatal(err)
	}
	if _, err = uc.ExecuteForStaff(f.ctx, input, user, f.shop, f.professional); !errors.Is(err, booking.ErrStaffBookingAccessDenied) {
		t.Fatal("revoked membership replayed", err)
	}
	if err = repo.SetAccountActive(f.ctx, owner, f.shop, account.ID, true); err != nil {
		t.Fatal(err)
	}
	replayed, err := uc.ExecuteForStaff(f.ctx, input, user, f.shop, f.professional)
	if err != nil || !replayed.Replayed || replayed.Code != created.Code {
		t.Fatal(replayed, err)
	}
	if _, err = f.pool.Exec(f.ctx, "UPDATE professionals SET active=false WHERE shop_id=$1 AND id=$2", f.shop, f.professional); err != nil {
		t.Fatal(err)
	}
	if _, err = uc.ExecuteForStaff(f.ctx, input, user, f.shop, f.professional); !errors.Is(err, booking.ErrStaffBookingAccessDenied) {
		t.Fatal("inactive professional replayed", err)
	}
	if _, err = f.pool.Exec(f.ctx, "UPDATE professionals SET active=true WHERE shop_id=$1 AND id=$2", f.shop, f.professional); err != nil {
		t.Fatal(err)
	}
	items, err := repo.ListAccounts(context.Background(), owner, f.shop)
	if err != nil || len(items) != 1 || !items[0].Active {
		t.Fatal(items, err)
	}
}

func TestStaffAccountReplacementPreservesBookingsAndIsConcurrentSafe(t *testing.T) {
	f, other := newBookingFixture(t), newBookingFixture(t)
	owner, oldUser, newUser := uuid.New(), uuid.New(), uuid.New()
	if _, err := f.pool.Exec(f.ctx, "INSERT INTO users(id,display_name) VALUES($1,'Owner')", owner); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(f.ctx, "INSERT INTO shop_memberships(user_id,shop_id,role) VALUES($1,$2,'owner')", owner, f.shop); err != nil {
		t.Fatal(err)
	}
	repo := identitypostgres.NewAccountRepository(f.pool)
	old, err := repo.ReserveAccount(f.ctx, owner, f.shop, f.professional, "wrong@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = repo.LinkAccount(f.ctx, owner, f.shop, old.ID, identity.ProviderUser{ID: oldUser, Email: old.Email}); err != nil {
		t.Fatal(err)
	}
	if _, err = f.pool.Exec(f.ctx, "INSERT INTO shop_memberships(user_id,shop_id,role) VALUES($1,$2,'owner')", oldUser, other.shop); err != nil {
		t.Fatal(err)
	}
	uc := booking.NewCreateAppointment(bookingpostgres.NewTransactor(f.pool, bindBookingQueries), contacts.ContactNormalizer{}, security.Codes{}, booking.ClockFunc(time.Now))
	input := booking.CreateAppointmentInput{ShopSlug: f.slug, ServiceID: f.service, ProfessionalID: f.professional, StartsAt: f.start, IdempotencyKey: bookingKey(), Customer: booking.CustomerInput{FirstName: "Test", LastName: "Customer", Phone: "11912345678", PhoneCountry: "BR"}}
	created, err := uc.ExecuteForStaff(f.ctx, input, oldUser, f.shop, f.professional)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = repo.ReserveReplacement(f.ctx, owner, f.shop, old.ID, "correct@example.com"); !errors.Is(err, identity.ErrAccountMustBeInactive) {
		t.Fatal("active access replaced", err)
	}
	// Effective inactivity must not bypass the explicit membership revocation.
	if _, err = f.pool.Exec(f.ctx, "UPDATE users SET active=false WHERE id=$1", oldUser); err != nil {
		t.Fatal(err)
	}
	if _, err = repo.ReserveReplacement(f.ctx, owner, f.shop, old.ID, "correct@example.com"); !errors.Is(err, identity.ErrAccountMustBeInactive) {
		t.Fatal("stored active membership removed", err)
	}
	if _, err = f.pool.Exec(f.ctx, "UPDATE users SET active=true WHERE id=$1", oldUser); err != nil {
		t.Fatal(err)
	}
	if err = repo.SetAccountActive(f.ctx, owner, f.shop, old.ID, false); err != nil {
		t.Fatal(err)
	}
	if _, err = repo.ReserveReplacement(f.ctx, owner, f.shop, old.ID, old.Email); !errors.Is(err, identity.ErrInvalidAccount) {
		t.Fatal("unchanged email accepted", err)
	}
	if _, err = repo.ReserveReplacement(f.ctx, oldUser, f.shop, old.ID, "correct@example.com"); !errors.Is(err, identity.ErrShopAccessDenied) {
		t.Fatal("non-owner replaced account", err)
	}
	// A database failure after deletion/retirement must roll the entire swap back.
	if _, err = repo.ReserveReplacement(f.ctx, owner, f.shop, old.ID, strings.Repeat("x", 255)+"@example.com"); err == nil {
		t.Fatal("invalid storage email accepted")
	}
	if _, err = repo.FindAccount(f.ctx, owner, f.shop, old.ID); err != nil {
		t.Fatal("failed transaction retired the old account", err)
	}
	var rollbackMembership int
	if err = f.pool.QueryRow(f.ctx, "SELECT count(*) FROM shop_memberships WHERE shop_id=$1 AND user_id=$2 AND NOT active", f.shop, oldUser).Scan(&rollbackMembership); err != nil || rollbackMembership != 1 {
		t.Fatal("failed transaction removed the old membership", rollbackMembership, err)
	}
	// Even an inconsistent record must never remove an owner's membership.
	if _, err = f.pool.Exec(f.ctx, "UPDATE shop_memberships SET role='owner',professional_id=NULL WHERE shop_id=$1 AND user_id=$2", f.shop, oldUser); err != nil {
		t.Fatal(err)
	}
	if _, err = repo.ReserveReplacement(f.ctx, owner, f.shop, old.ID, "correct@example.com"); !errors.Is(err, identity.ErrShopAccessDenied) {
		t.Fatal("owner membership removed", err)
	}
	if _, err = f.pool.Exec(f.ctx, "UPDATE shop_memberships SET role='barber',professional_id=$3 WHERE shop_id=$1 AND user_id=$2", f.shop, oldUser, f.professional); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	results, failures := make(chan identity.StaffAccount, 2), make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			a, e := repo.ReserveReplacement(f.ctx, owner, f.shop, old.ID, "correct@example.com")
			if e != nil {
				failures <- e
			} else {
				results <- a
			}
		}()
	}
	wg.Wait()
	if len(results) != 1 || len(failures) != 1 {
		t.Fatal("replacement was not reserved exactly once")
	}
	fresh := <-results
	if err = <-failures; !errors.Is(err, identity.ErrAccountNotFound) {
		t.Fatal("stale UUID accepted", err)
	}
	if fresh.ID == old.ID || fresh.ProfessionalID != f.professional {
		t.Fatal("professional or account identity changed incorrectly", fresh)
	}
	if _, err = repo.LinkAccount(f.ctx, owner, f.shop, old.ID, identity.ProviderUser{ID: oldUser, Email: old.Email}); !errors.Is(err, identity.ErrAccountNotFound) {
		t.Fatal("retired account linked", err)
	}
	if err = repo.SetAccountActive(f.ctx, owner, f.shop, old.ID, true); !errors.Is(err, identity.ErrAccountNotFound) {
		t.Fatal("retired account reactivated", err)
	}
	if err = repo.ReviewAccount(f.ctx, f.shop, old.ID); err != nil {
		t.Fatal(err)
	}
	items, err := repo.ListAccounts(f.ctx, owner, f.shop)
	if err != nil || len(items) != 1 || items[0].ID != fresh.ID || items[0].State != "pending" {
		t.Fatal("history exposed as current account", items, err)
	}
	var oldLocal, otherLocal, profiles, preserved, history int
	if err = f.pool.QueryRow(f.ctx, `SELECT
	 (SELECT count(*) FROM shop_memberships WHERE shop_id=$1 AND user_id=$2),
	 (SELECT count(*) FROM shop_memberships WHERE shop_id=$3 AND user_id=$2),
	 (SELECT count(*) FROM users WHERE id=$2),
	 (SELECT count(*) FROM appointments WHERE shop_id=$1 AND id=$4 AND professional_id=$5),
	 (SELECT count(*) FROM staff_accounts WHERE shop_id=$1 AND id=$6 AND replaced_at IS NOT NULL)`, f.shop, oldUser, other.shop, created.Appointment.AppointmentID, f.professional, old.ID).Scan(&oldLocal, &otherLocal, &profiles, &preserved, &history); err != nil {
		t.Fatal(err)
	}
	if oldLocal != 0 || otherLocal != 1 || profiles != 1 || preserved != 1 || history != 1 {
		t.Fatal("replacement deleted unrelated data", oldLocal, otherLocal, profiles, preserved, history)
	}
	if _, err = uc.ExecuteForStaff(f.ctx, input, oldUser, f.shop, f.professional); !errors.Is(err, booking.ErrStaffBookingAccessDenied) {
		t.Fatal("old identity retained booking access", err)
	}
	if _, err = repo.LinkAccount(f.ctx, owner, f.shop, fresh.ID, identity.ProviderUser{ID: newUser, Email: fresh.Email}); err != nil {
		t.Fatal(err)
	}
	input.StartsAt = input.StartsAt.Add(time.Hour)
	input.IdempotencyKey = bookingKey()
	if _, err = uc.ExecuteForStaff(f.ctx, input, newUser, f.shop, f.professional); err != nil {
		t.Fatal("new login could not book for preserved professional", err)
	}
	// Pending invitations also have replaceable, immutable reservation UUIDs.
	f2 := newBookingFixture(t)
	if _, err = f.pool.Exec(f.ctx, "INSERT INTO shop_memberships(user_id,shop_id,role) VALUES($1,$2,'owner')", owner, f2.shop); err != nil {
		t.Fatal(err)
	}
	pending, err := repo.ReserveAccount(f.ctx, owner, f2.shop, f2.professional, "pending-wrong@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = repo.ReserveReplacement(f.ctx, owner, f2.shop, pending.ID, "pending-correct@example.com"); err != nil {
		t.Fatal(err)
	}
	if _, err = repo.LinkAccount(f.ctx, owner, f2.shop, pending.ID, identity.ProviderUser{ID: uuid.New(), Email: pending.Email}); !errors.Is(err, identity.ErrAccountNotFound) {
		t.Fatal("late invite resurrected retired reservation", err)
	}
}
