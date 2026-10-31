package application

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
)

var (
	ErrRegistrationNotConfigured = errors.New("registration is not configured")
	ErrRegistrationUnavailable   = errors.New("registration account lookup unavailable")
	ErrEmailNotConfirmed         = errors.New("confirmed email required")
)

// RegistrationAccount comes from Auth's authenticated user endpoint, never
// user_metadata or client JSON. It carries no password or administrative role.
type RegistrationAccount struct {
	UserID         uuid.UUID
	EmailConfirmed bool
	Anonymous      bool
}

type RegistrationAccountReader interface {
	ReadRegistrationAccount(context.Context, string) (RegistrationAccount, error)
}

// RegistrationIdentity only permits starting onboarding. It is NOT a local
// staff identity and must not be installed as StaffIdentity in request context.
type RegistrationIdentity struct{ UserID uuid.UUID }

type VerifyRegistrationAccount struct {
	verifier AccessTokenVerifier
	accounts RegistrationAccountReader
}

func NewVerifyRegistrationAccount(verifier AccessTokenVerifier, accounts RegistrationAccountReader) *VerifyRegistrationAccount {
	return &VerifyRegistrationAccount{verifier: verifier, accounts: accounts}
}

func (uc *VerifyRegistrationAccount) Execute(ctx context.Context, token string) (RegistrationIdentity, error) {
	if uc.verifier == nil || uc.accounts == nil {
		return RegistrationIdentity{}, ErrRegistrationNotConfigured
	}
	id, err := uc.verifier.VerifyAccessToken(ctx, token)
	if err != nil {
		return RegistrationIdentity{}, fmt.Errorf("verify registration token: %w", err)
	}
	if id == uuid.Nil {
		return RegistrationIdentity{}, ErrInvalidAccessToken
	}
	account, err := uc.accounts.ReadRegistrationAccount(ctx, token)
	if err != nil {
		return RegistrationIdentity{}, fmt.Errorf("read registration account: %w", err)
	}
	if account.UserID != id {
		return RegistrationIdentity{}, ErrIdentityMismatch
	}
	if account.Anonymous {
		return RegistrationIdentity{}, ErrInvalidAccessToken
	}
	if !account.EmailConfirmed {
		return RegistrationIdentity{}, ErrEmailNotConfirmed
	}
	return RegistrationIdentity{UserID: id}, nil
}
