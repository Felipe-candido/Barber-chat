package application_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/Felipe-candido/Barber-chat/internal/modules/identity/application"
	"github.com/Felipe-candido/Barber-chat/internal/modules/identity/domain"
	"github.com/google/uuid"
)

type tokenVerifierFunc func(context.Context, string) (uuid.UUID, error)

func (f tokenVerifierFunc) VerifyAccessToken(ctx context.Context, token string) (uuid.UUID, error) {
	return f(ctx, token)
}

type identityReaderStub struct {
	t    *testing.T
	find func(context.Context, uuid.UUID) (domain.User, bool, error)
}

func (s identityReaderStub) FindUserByID(ctx context.Context, id uuid.UUID) (domain.User, bool, error) {
	return s.find(ctx, id)
}

func (s identityReaderStub) FindMembership(context.Context, uuid.UUID, uuid.UUID) (domain.Membership, bool, error) {
	s.t.Fatal("authentication must not query memberships")
	return domain.Membership{}, false, nil
}

func (s identityReaderStub) ListMembershipsByUser(context.Context, uuid.UUID) ([]domain.Membership, error) {
	s.t.Fatal("authentication must not list memberships")
	return nil, nil
}

func TestAuthenticate(t *testing.T) {
	userID := uuid.New()
	activeUser := domain.User{ID: userID, DisplayName: "Felipe", Active: true}
	verifierFailure := errors.New("JWKS unavailable")
	repositoryFailure := errors.New("database unavailable")

	tests := []struct {
		name       string
		token      string
		subject    uuid.UUID
		verifyErr  error
		user       domain.User
		found      bool
		repoErr    error
		wantErr    error
		wantVerify bool
		wantLookup bool
	}{
		{name: "empty token", wantErr: application.ErrInvalidAccessToken},
		{name: "whitespace token", token: " \t\n", wantErr: application.ErrInvalidAccessToken},
		{name: "invalid token", token: "invalid", verifyErr: application.ErrInvalidAccessToken,
			wantErr: application.ErrInvalidAccessToken, wantVerify: true},
		{name: "wrapped invalid token", token: "expired", verifyErr: fmt.Errorf("expired: %w", application.ErrInvalidAccessToken),
			wantErr: application.ErrInvalidAccessToken, wantVerify: true},
		{name: "empty subject", token: "token", wantErr: application.ErrInvalidAccessToken, wantVerify: true},
		{name: "verifier failure", token: "token", verifyErr: verifierFailure,
			wantErr: verifierFailure, wantVerify: true},
		{name: "verifier cancellation", token: "token", verifyErr: context.Canceled,
			wantErr: context.Canceled, wantVerify: true},
		{name: "repository failure", token: "token", subject: userID, repoErr: repositoryFailure,
			wantErr: repositoryFailure, wantVerify: true, wantLookup: true},
		{name: "repository deadline", token: "token", subject: userID, repoErr: context.DeadlineExceeded,
			wantErr: context.DeadlineExceeded, wantVerify: true, wantLookup: true},
		{name: "unprovisioned user", token: "token", subject: userID,
			wantErr: application.ErrUserNotProvisioned, wantVerify: true, wantLookup: true},
		{name: "inactive user", token: "token", subject: userID, user: domain.User{ID: userID}, found: true,
			wantErr: application.ErrUserInactive, wantVerify: true, wantLookup: true},
		{name: "mismatched user", token: "token", subject: userID, user: domain.User{ID: uuid.New(), Active: true}, found: true,
			wantErr: application.ErrIdentityMismatch, wantVerify: true, wantLookup: true},
		{name: "empty local user ID", token: "token", subject: userID, user: domain.User{Active: true}, found: true,
			wantErr: application.ErrIdentityMismatch, wantVerify: true, wantLookup: true},
		{name: "active user", token: "token", subject: userID, user: activeUser, found: true,
			wantVerify: true, wantLookup: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			verifyCalls, lookupCalls := 0, 0
			verifier := tokenVerifierFunc(func(gotCtx context.Context, token string) (uuid.UUID, error) {
				verifyCalls++
				if gotCtx != ctx || token != tt.token {
					t.Fatal("verifier must receive the original context and token")
				}
				return tt.subject, tt.verifyErr
			})
			repository := identityReaderStub{t: t, find: func(gotCtx context.Context, id uuid.UUID) (domain.User, bool, error) {
				lookupCalls++
				if verifyCalls != 1 || gotCtx != ctx || id != tt.subject {
					t.Fatal("repository must receive the verified subject and original context after verification")
				}
				return tt.user, tt.found, tt.repoErr
			}}

			got, err := application.NewAuthenticate(verifier, repository).Execute(ctx, application.ResolveStaffIdentityInput{AccessToken: tt.token})
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("Execute() error = %v, want %v", err, tt.wantErr)
			}
			if errors.Is(err, application.ErrInvalidAccessToken) != (tt.wantErr == application.ErrInvalidAccessToken) {
				t.Fatal("operational errors must not be classified as invalid credentials")
			}
			if (verifyCalls == 1) != tt.wantVerify || verifyCalls > 1 {
				t.Errorf("verifier calls = %d, want call = %v", verifyCalls, tt.wantVerify)
			}
			if (lookupCalls == 1) != tt.wantLookup || lookupCalls > 1 {
				t.Errorf("repository calls = %d, want call = %v", lookupCalls, tt.wantLookup)
			}
			if tt.wantErr != nil {
				if got != (application.StaffIdentityOutput{}) {
					t.Errorf("failed authentication returned an identity: %+v", got)
				}
				return
			}
			want := application.StaffIdentityOutput{UserID: userID, DisplayName: "Felipe"}
			if got != want {
				t.Errorf("Execute() = %+v, want %+v", got, want)
			}
		})
	}
}
