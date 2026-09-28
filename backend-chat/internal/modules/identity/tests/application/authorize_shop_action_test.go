package application_test

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/Felipe-candido/Barber-chat/internal/modules/identity/application"
	"github.com/Felipe-candido/Barber-chat/internal/modules/identity/domain"
	"github.com/google/uuid"
)

type shopResolverFunc func(context.Context, string) (uuid.UUID, bool, error)

func (f shopResolverFunc) LookupActiveShop(ctx context.Context, slug string) (uuid.UUID, bool, error) {
	return f(ctx, slug)
}

type authorizationRepositoryStub struct {
	t          *testing.T
	user       func(context.Context, uuid.UUID) (domain.User, bool, error)
	membership func(context.Context, uuid.UUID, uuid.UUID) (domain.Membership, bool, error)
}

func (s authorizationRepositoryStub) FindUserByID(ctx context.Context, id uuid.UUID) (domain.User, bool, error) {
	return s.user(ctx, id)
}

func (s authorizationRepositoryStub) FindMembership(ctx context.Context, userID, shopID uuid.UUID) (domain.Membership, bool, error) {
	return s.membership(ctx, userID, shopID)
}

func (s authorizationRepositoryStub) ListMembershipsByUser(context.Context, uuid.UUID) ([]domain.Membership, error) {
	s.t.Fatal("authorization must query the requested membership, not select one from a list")
	return nil, nil
}

func TestAuthorizeShopAction(t *testing.T) {
	userID, shopID := uuid.New(), uuid.New()
	failure := errors.New("database unavailable")
	type scenario struct {
		input           application.AuthorizeShopActionInput
		user            domain.User
		userFound       bool
		userErr         error
		shopID          uuid.UUID
		shopFound       bool
		shopErr         error
		membership      domain.Membership
		membershipFound bool
		membershipErr   error
	}

	tests := []struct {
		name      string
		change    func(*scenario)
		wantErr   error
		wantCalls []string
	}{
		{"missing identity", func(s *scenario) { s.input.UserID = uuid.Nil }, application.ErrInvalidStaffIdentity, nil},
		{"empty slug", func(s *scenario) { s.input.ShopSlug = "" }, application.ErrShopAccessDenied, nil},
		{"whitespace slug", func(s *scenario) { s.input.ShopSlug = " \t\n" }, application.ErrShopAccessDenied, nil},
		{"user lookup failure", func(s *scenario) { s.userErr = failure }, failure, []string{"user"}},
		{"user lookup canceled", func(s *scenario) { s.userErr = context.Canceled }, context.Canceled, []string{"user"}},
		{"unprovisioned user", func(s *scenario) { s.userFound = false }, application.ErrUserNotProvisioned, []string{"user"}},
		{"wrong user", func(s *scenario) { s.user.ID = uuid.New() }, application.ErrIdentityMismatch, []string{"user"}},
		{"empty local user ID", func(s *scenario) { s.user.ID = uuid.Nil }, application.ErrIdentityMismatch, []string{"user"}},
		{"inactive user", func(s *scenario) { s.user.Active = false }, application.ErrUserInactive, []string{"user"}},
		{"shop lookup failure", func(s *scenario) { s.shopErr = failure }, failure, []string{"user", "shop"}},
		{"shop lookup deadline", func(s *scenario) { s.shopErr = context.DeadlineExceeded }, context.DeadlineExceeded, []string{"user", "shop"}},
		{"missing or inactive shop", func(s *scenario) { s.shopFound = false }, application.ErrShopAccessDenied, []string{"user", "shop"}},
		{"empty shop ID", func(s *scenario) { s.shopID = uuid.Nil }, application.ErrIdentityMismatch, []string{"user", "shop"}},
		{"membership lookup failure", func(s *scenario) { s.membershipErr = failure }, failure, []string{"user", "shop", "membership"}},
		{"membership lookup canceled", func(s *scenario) { s.membershipErr = context.Canceled }, context.Canceled, []string{"user", "shop", "membership"}},
		{"missing membership", func(s *scenario) { s.membershipFound = false }, application.ErrShopAccessDenied, []string{"user", "shop", "membership"}},
		{"membership for another user", func(s *scenario) { s.membership.UserID = uuid.New() }, application.ErrIdentityMismatch, []string{"user", "shop", "membership"}},
		{"membership for another shop", func(s *scenario) { s.membership.ShopID = uuid.New() }, application.ErrIdentityMismatch, []string{"user", "shop", "membership"}},
		{"inactive membership", func(s *scenario) { s.membership.Active = false }, application.ErrShopAccessDenied, []string{"user", "shop", "membership"}},
		{"active membership", func(s *scenario) {}, nil, []string{"user", "shop", "membership"}},
		{"trimmed slug", func(s *scenario) { s.input.ShopSlug = " \tbarbearia\n " }, nil, []string{"user", "shop", "membership"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := scenario{
				input:           application.AuthorizeShopActionInput{UserID: userID, ShopSlug: "barbearia"},
				user:            domain.User{ID: userID, Active: true},
				userFound:       true,
				shopID:          shopID,
				shopFound:       true,
				membership:      domain.Membership{UserID: userID, ShopID: shopID, Active: true},
				membershipFound: true,
			}
			tt.change(&s)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			var calls []string
			repository := authorizationRepositoryStub{
				t: t,
				user: func(gotCtx context.Context, id uuid.UUID) (domain.User, bool, error) {
					calls = append(calls, "user")
					if gotCtx != ctx || id != s.input.UserID {
						t.Fatal("user lookup must receive the authenticated ID and original context")
					}
					return s.user, s.userFound, s.userErr
				},
				membership: func(gotCtx context.Context, gotUserID, gotShopID uuid.UUID) (domain.Membership, bool, error) {
					calls = append(calls, "membership")
					if gotCtx != ctx || gotUserID != userID || gotShopID != shopID {
						t.Fatal("membership lookup must be scoped to the authenticated user and requested shop")
					}
					return s.membership, s.membershipFound, s.membershipErr
				},
			}
			shops := shopResolverFunc(func(gotCtx context.Context, slug string) (uuid.UUID, bool, error) {
				calls = append(calls, "shop")
				if gotCtx != ctx || slug != "barbearia" {
					t.Fatal("shop lookup must receive the trimmed slug and original context")
				}
				return s.shopID, s.shopFound, s.shopErr
			})

			got, err := application.NewAuthorizeShopAction(repository, shops).Execute(ctx, s.input)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("Execute() error = %v, want %v", err, tt.wantErr)
			}
			if !reflect.DeepEqual(calls, tt.wantCalls) {
				t.Errorf("calls = %v, want %v", calls, tt.wantCalls)
			}
			want := application.AuthorizedShopScope{}
			if tt.wantErr == nil {
				want = application.AuthorizedShopScope{UserID: userID, ShopID: shopID}
			}
			if got != want {
				t.Errorf("Execute() scope = %+v, want %+v", got, want)
			}
		})
	}
}

func TestAuthorizeShopActionRechecksSuspensions(t *testing.T) {
	for _, target := range []string{"user", "shop", "membership"} {
		t.Run(target, func(t *testing.T) {
			userID, shopID := uuid.New(), uuid.New()
			userActive, shopActive, membershipActive := true, true, true
			repository := authorizationRepositoryStub{
				t: t,
				user: func(context.Context, uuid.UUID) (domain.User, bool, error) {
					return domain.User{ID: userID, Active: userActive}, true, nil
				},
				membership: func(context.Context, uuid.UUID, uuid.UUID) (domain.Membership, bool, error) {
					return domain.Membership{UserID: userID, ShopID: shopID, Active: membershipActive}, true, nil
				},
			}
			shops := shopResolverFunc(func(context.Context, string) (uuid.UUID, bool, error) {
				return shopID, shopActive, nil
			})
			uc := application.NewAuthorizeShopAction(repository, shops)
			input := application.AuthorizeShopActionInput{UserID: userID, ShopSlug: "barbearia"}
			if _, err := uc.Execute(context.Background(), input); err != nil {
				t.Fatalf("initial authorization failed: %v", err)
			}
			wantErr := application.ErrShopAccessDenied
			switch target {
			case "user":
				userActive = false
				wantErr = application.ErrUserInactive
			case "shop":
				shopActive = false
			case "membership":
				membershipActive = false
			}
			got, err := uc.Execute(context.Background(), input)
			if !errors.Is(err, wantErr) || got != (application.AuthorizedShopScope{}) {
				t.Fatalf("authorization after suspension = %+v, %v; want empty scope and %v", got, err, wantErr)
			}
		})
	}
}
