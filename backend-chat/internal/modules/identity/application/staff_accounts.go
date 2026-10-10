package application

import (
	"context"
	"net/mail"
	"strings"
	"time"

	"github.com/google/uuid"
)

type StaffAccounts struct {
	store    StaffAccountStore
	provider StaffAccountProvider
}

func NewStaffAccounts(store StaffAccountStore, provider StaffAccountProvider) *StaffAccounts {
	return &StaffAccounts{store: store, provider: provider}
}

func NormalizeStaffEmail(value string) (string, error) {
	value = strings.TrimSpace(value)
	address, err := mail.ParseAddress(value)
	if err != nil || address.Address != value || len(value) > 254 || !strings.Contains(value, "@") {
		return "", ErrInvalidAccount
	}
	return strings.ToLower(value), nil
}

func (uc *StaffAccounts) List(ctx context.Context, actorID, shopID uuid.UUID) ([]StaffAccount, error) {
	if actorID == uuid.Nil || shopID == uuid.Nil {
		return nil, ErrShopAccessDenied
	}
	return uc.store.ListAccounts(ctx, actorID, shopID)
}

func (uc *StaffAccounts) Invite(ctx context.Context, actorID, shopID, professionalID uuid.UUID, email string) (StaffAccount, error) {
	if actorID == uuid.Nil || shopID == uuid.Nil {
		return StaffAccount{}, ErrShopAccessDenied
	}
	if professionalID == uuid.Nil {
		return StaffAccount{}, ErrInvalidAccount
	}
	email, err := NormalizeStaffEmail(email)
	if err != nil {
		return StaffAccount{}, err
	}
	if uc.provider == nil {
		return StaffAccount{}, ErrAccountNotConfigured
	}
	account, err := uc.store.ReserveAccount(ctx, actorID, shopID, professionalID, email)
	if err != nil {
		return StaffAccount{}, err
	}
	if account.ID == uuid.Nil || account.ProfessionalID != professionalID || account.Email != email || account.State != "pending" {
		return StaffAccount{}, ErrIdentityMismatch
	}
	return uc.inviteReserved(ctx, actorID, shopID, account)
}

// Replacing a login never changes Auth globally, the professional or bookings.
// A new reservation UUID prevents stale requests from linking the old account.
func (uc *StaffAccounts) Replace(ctx context.Context, actorID, shopID, accountID uuid.UUID, email string) (StaffAccount, error) {
	if actorID == uuid.Nil || shopID == uuid.Nil {
		return StaffAccount{}, ErrShopAccessDenied
	}
	if accountID == uuid.Nil {
		return StaffAccount{}, ErrInvalidAccount
	}
	email, err := NormalizeStaffEmail(email)
	if err != nil {
		return StaffAccount{}, err
	}
	if uc.provider == nil {
		return StaffAccount{}, ErrAccountNotConfigured
	}
	account, err := uc.store.ReserveReplacement(ctx, actorID, shopID, accountID, email)
	if err != nil {
		return StaffAccount{}, err
	}
	if account.ID == uuid.Nil || account.ID == accountID || account.ProfessionalID == uuid.Nil || account.Email != email || account.State != "pending" {
		return StaffAccount{}, ErrIdentityMismatch
	}
	return uc.inviteReserved(ctx, actorID, shopID, account)
}

func (uc *StaffAccounts) inviteReserved(ctx context.Context, actorID, shopID uuid.UUID, account StaffAccount) (StaffAccount, error) {
	email := account.Email
	user, err := uc.provider.Invite(ctx, email)
	if err == nil && (user.ID == uuid.Nil || !strings.EqualFold(user.Email, email)) {
		err = ErrIdentityMismatch
	}
	if err == nil {
		user.Email = email
		linked, linkErr := uc.store.LinkAccount(ctx, actorID, shopID, account.ID, user)
		if linkErr == nil {
			return linked, nil
		}
	}
	// The invite may already exist even after a timeout or a failed DB commit.
	// A durable pending/review record prevents another automatic invitation.
	reviewCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_ = uc.store.ReviewAccount(reviewCtx, shopID, account.ID)
	return StaffAccount{}, ErrAccountNeedsReview
}

// Recovery is explicit: fetch the UUID from Auth and match its email to the
// original request. Never trust an arbitrary UUID or overwrite an owner link.
func (uc *StaffAccounts) LinkExisting(ctx context.Context, actorID, shopID, accountID, userID uuid.UUID) (StaffAccount, error) {
	if actorID == uuid.Nil || shopID == uuid.Nil {
		return StaffAccount{}, ErrShopAccessDenied
	}
	if accountID == uuid.Nil || userID == uuid.Nil {
		return StaffAccount{}, ErrInvalidAccount
	}
	if uc.provider == nil {
		return StaffAccount{}, ErrAccountNotConfigured
	}
	account, err := uc.store.FindAccount(ctx, actorID, shopID, accountID)
	if err != nil {
		return StaffAccount{}, err
	}
	user, err := uc.provider.FindUser(ctx, userID)
	if err != nil {
		return StaffAccount{}, err
	}
	if user.ID != userID || !strings.EqualFold(user.Email, account.Email) {
		return StaffAccount{}, ErrInvalidAccount
	}
	user.Email = account.Email
	return uc.store.LinkAccount(ctx, actorID, shopID, accountID, user)
}

func (uc *StaffAccounts) SetActive(ctx context.Context, actorID, shopID, accountID uuid.UUID, active bool) error {
	if actorID == uuid.Nil || shopID == uuid.Nil {
		return ErrShopAccessDenied
	}
	if accountID == uuid.Nil {
		return ErrInvalidAccount
	}
	return uc.store.SetAccountActive(ctx, actorID, shopID, accountID, active)
}
