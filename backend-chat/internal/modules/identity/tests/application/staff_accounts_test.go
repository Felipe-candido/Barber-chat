package application_test

import (
	"context"
	"errors"
	"testing"

	app "github.com/Felipe-candido/Barber-chat/internal/modules/identity/application"
	"github.com/google/uuid"
)

type accountStore struct {
	account                              app.StaffAccount
	reserveErr, linkErr                  error
	reserveCalls, linkCalls, reviewCalls int
	actor, shop                          uuid.UUID
	reviewID                             uuid.UUID
}

func (s *accountStore) ListAccounts(context.Context, uuid.UUID, uuid.UUID) ([]app.StaffAccount, error) {
	return []app.StaffAccount{s.account}, nil
}
func (s *accountStore) ReserveAccount(_ context.Context, actor, shop, professional uuid.UUID, email string) (app.StaffAccount, error) {
	s.reserveCalls++
	s.actor = actor
	s.shop = shop
	s.account.ProfessionalID = professional
	s.account.Email = email
	return s.account, s.reserveErr
}
func (s *accountStore) FindAccount(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) (app.StaffAccount, error) {
	return s.account, nil
}
func (s *accountStore) ReserveReplacement(_ context.Context, actor, shop, original uuid.UUID, email string) (app.StaffAccount, error) {
	s.reserveCalls++
	s.actor, s.shop = actor, shop
	s.account.Email = email
	return s.account, s.reserveErr
}
func (s *accountStore) LinkAccount(_ context.Context, actor, shop, id uuid.UUID, user app.ProviderUser) (app.StaffAccount, error) {
	s.linkCalls++
	if id != s.account.ID || user.Email != s.account.Email {
		return app.StaffAccount{}, app.ErrIdentityMismatch
	}
	if s.linkErr != nil {
		return app.StaffAccount{}, s.linkErr
	}
	s.account.UserID = user.ID
	s.account.State = "linked"
	s.account.Active = true
	return s.account, nil
}
func (s *accountStore) ReviewAccount(_ context.Context, shop, id uuid.UUID) error {
	s.reviewCalls++
	s.reviewID = id
	if id != s.account.ID || shop != s.shop {
		return errors.New("review lost reservation ID")
	}
	return nil
}
func (s *accountStore) SetAccountActive(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, bool) error {
	return nil
}

type accountProvider struct {
	user  app.ProviderUser
	err   error
	calls int
}

func (p *accountProvider) Invite(context.Context, string) (app.ProviderUser, error) {
	p.calls++
	return p.user, p.err
}
func (p *accountProvider) FindUser(context.Context, uuid.UUID) (app.ProviderUser, error) {
	p.calls++
	return p.user, p.err
}

func TestStaffAccountInviteDurableBoundary(t *testing.T) {
	for _, tc := range []struct {
		name                             string
		reserveErr, providerErr, linkErr error
		email                            string
		id                               uuid.UUID
		want                             error
		links, reviews, calls            int
	}{
		{"success", nil, nil, nil, "BARBER@example.com", uuid.New(), nil, 1, 0, 1},
		{"duplicate", app.ErrAccountExists, nil, nil, "barber@example.com", uuid.New(), app.ErrAccountExists, 0, 0, 0},
		{"denied owner", app.ErrShopAccessDenied, nil, nil, "barber@example.com", uuid.New(), app.ErrShopAccessDenied, 0, 0, 0},
		{"Auth timeout", nil, context.DeadlineExceeded, nil, "barber@example.com", uuid.New(), app.ErrAccountNeedsReview, 0, 1, 1},
		{"Auth rejected", nil, app.ErrAccountProviderRejected, nil, "barber@example.com", uuid.New(), app.ErrAccountNeedsReview, 0, 1, 1},
		{"DB commit failed", nil, nil, errors.New("connection lost"), "barber@example.com", uuid.New(), app.ErrAccountNeedsReview, 1, 1, 1},
		{"wrong email", nil, nil, nil, "other@example.com", uuid.New(), app.ErrAccountNeedsReview, 0, 1, 1},
		{"empty provider ID", nil, nil, nil, "barber@example.com", uuid.Nil, app.ErrAccountNeedsReview, 0, 1, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := &accountStore{account: app.StaffAccount{ID: uuid.New(), State: "pending"}, reserveErr: tc.reserveErr, linkErr: tc.linkErr}
			provider := &accountProvider{user: app.ProviderUser{ID: tc.id, Email: tc.email}, err: tc.providerErr}
			actor, shop, professional := uuid.New(), uuid.New(), uuid.New()
			result, err := app.NewStaffAccounts(store, provider).Invite(context.Background(), actor, shop, professional, "  BARBER@example.com  ")
			if !errors.Is(err, tc.want) || store.linkCalls != tc.links || store.reviewCalls != tc.reviews || provider.calls != tc.calls {
				t.Fatal("unexpected provisioning steps", err, store, provider)
			}
			if store.actor != actor || store.shop != shop || store.account.Email != "barber@example.com" {
				t.Fatal("tenant or email changed")
			}
			if tc.reviews > 0 && store.reviewID != store.account.ID {
				t.Fatal("review lost the durable reservation ID")
			}
			if err == nil && (result.State != "linked" || !result.Active || result.UserID != tc.id) {
				t.Fatal("incorrect account", result)
			}
		})
	}
}

func TestStaffAccountInputAndRecovery(t *testing.T) {
	for _, email := range []string{"", "Display <barber@example.com>", "broken", "a\r\n@example.com"} {
		if _, err := app.NormalizeStaffEmail(email); !errors.Is(err, app.ErrInvalidAccount) {
			t.Fatal("invalid email accepted", email, err)
		}
	}
	store := &accountStore{account: app.StaffAccount{ID: uuid.New(), Email: "barber@example.com", State: "review"}}
	actor, shop := uuid.New(), uuid.New()
	if _, err := app.NewStaffAccounts(store, nil).Invite(context.Background(), actor, shop, uuid.New(), "barber@example.com"); !errors.Is(err, app.ErrAccountNotConfigured) || store.reserveCalls != 0 {
		t.Fatal("unconfigured provider reserved an account", err)
	}
	provider := &accountProvider{user: app.ProviderUser{ID: uuid.New(), Email: "other@example.com"}}
	uc := app.NewStaffAccounts(store, provider)
	if _, err := uc.LinkExisting(context.Background(), actor, shop, store.account.ID, provider.user.ID); !errors.Is(err, app.ErrInvalidAccount) || store.linkCalls != 0 {
		t.Fatal("linked wrong Auth email", err)
	}
	provider.user.Email = "BARBER@example.com"
	result, err := uc.LinkExisting(context.Background(), actor, shop, store.account.ID, provider.user.ID)
	if err != nil || result.UserID != provider.user.ID || store.linkCalls != 1 {
		t.Fatal("could not complete verified recovery", result, err)
	}
}

func TestStaffAccountReplacementUsesFreshReservation(t *testing.T) {
	for _, tc := range []struct {
		name                    string
		reserveErr, providerErr error
		want                    error
		calls, reviews          int
	}{
		{"success", nil, nil, nil, 1, 0},
		{"active access", app.ErrAccountMustBeInactive, nil, app.ErrAccountMustBeInactive, 0, 0},
		{"stale request", app.ErrAccountNotFound, nil, app.ErrAccountNotFound, 0, 0},
		{"another shop", app.ErrShopAccessDenied, nil, app.ErrShopAccessDenied, 0, 0},
		{"ambiguous Auth", nil, context.DeadlineExceeded, app.ErrAccountNeedsReview, 1, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			original, actor, shop := uuid.New(), uuid.New(), uuid.New()
			store := &accountStore{account: app.StaffAccount{ID: uuid.New(), ProfessionalID: uuid.New(), State: "pending"}, reserveErr: tc.reserveErr}
			provider := &accountProvider{user: app.ProviderUser{ID: uuid.New(), Email: "correct@example.com"}, err: tc.providerErr}
			result, err := app.NewStaffAccounts(store, provider).Replace(context.Background(), actor, shop, original, " CORRECT@example.com ")
			if !errors.Is(err, tc.want) || provider.calls != tc.calls || store.reviewCalls != tc.reviews || store.actor != actor || store.shop != shop || store.account.Email != "correct@example.com" {
				t.Fatal("replacement boundary failed", result, err, store, provider)
			}
			if tc.reviews > 0 && (store.reviewID == original || store.reviewID != store.account.ID) {
				t.Fatal("review targeted retired account")
			}
			if err == nil && (result.ID == original || result.State != "linked") {
				t.Fatal("replacement reused retired UUID", result)
			}
		})
	}
	store := &accountStore{}
	uc := app.NewStaffAccounts(store, nil)
	if _, err := uc.Replace(context.Background(), uuid.New(), uuid.New(), uuid.New(), "correct@example.com"); !errors.Is(err, app.ErrAccountNotConfigured) || store.reserveCalls != 0 {
		t.Fatal("unconfigured replacement changed database", err)
	}
}
