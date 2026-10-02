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

type accessibleShopReaderFunc func(context.Context, uuid.UUID) ([]application.AccessibleShopOutput, error)

func (f accessibleShopReaderFunc) ListActiveShopsByUser(
	ctx context.Context,
	userID uuid.UUID,
) ([]application.AccessibleShopOutput, error) {
	return f(ctx, userID)
}

func TestListMyShops(t *testing.T) {
	userID := uuid.New()
	shops := []application.AccessibleShopOutput{
		{ShopID: uuid.New(), Name: "Barbershop A", Slug: "barbershop-a"},
		{ShopID: uuid.New(), Name: "Barbershop B", Slug: "barbershop-b"},
	}
	failure := errors.New("database unavailable")
	type scenario struct {
		input     application.ListMyShopsInput
		user      domain.User
		userFound bool
		userErr   error
		shops     []application.AccessibleShopOutput
		shopsErr  error
	}

	tests := []struct {
		name      string
		change    func(*scenario)
		wantErr   error
		wantCalls []string
		wantShops []application.AccessibleShopOutput
	}{
		{"missing identity", func(s *scenario) { s.input.UserID = uuid.Nil }, application.ErrInvalidStaffIdentity, nil, nil},
		{"user lookup failure", func(s *scenario) { s.userErr = failure }, failure, []string{"user"}, nil},
		{"user lookup canceled", func(s *scenario) { s.userErr = context.Canceled }, context.Canceled, []string{"user"}, nil},
		{"user lookup deadline", func(s *scenario) { s.userErr = context.DeadlineExceeded }, context.DeadlineExceeded, []string{"user"}, nil},
		{"unprovisioned user", func(s *scenario) { s.userFound = false }, application.ErrUserNotProvisioned, []string{"user"}, nil},
		{"wrong user", func(s *scenario) { s.user.ID = uuid.New() }, application.ErrIdentityMismatch, []string{"user"}, nil},
		{"empty local user ID", func(s *scenario) { s.user.ID = uuid.Nil }, application.ErrIdentityMismatch, []string{"user"}, nil},
		{"inactive user", func(s *scenario) { s.user.Active = false }, application.ErrUserInactive, []string{"user"}, nil},
		{"shop lookup failure with partial results", func(s *scenario) { s.shopsErr = failure }, failure, []string{"user", "shops"}, nil},
		{"shop lookup canceled", func(s *scenario) { s.shopsErr = context.Canceled }, context.Canceled, []string{"user", "shops"}, nil},
		{"shop lookup deadline", func(s *scenario) { s.shopsErr = context.DeadlineExceeded }, context.DeadlineExceeded, []string{"user", "shops"}, nil},
		{"empty shop ID", func(s *scenario) { s.shops[0].ShopID = uuid.Nil }, application.ErrIdentityMismatch, []string{"user", "shops"}, nil},
		{"invalid shop after a valid shop", func(s *scenario) { s.shops[1].ShopID = uuid.Nil }, application.ErrIdentityMismatch, []string{"user", "shops"}, nil},
		{"nil empty list", func(s *scenario) { s.shops = nil }, nil, []string{"user", "shops"}, []application.AccessibleShopOutput{}},
		{"empty list", func(s *scenario) { s.shops = []application.AccessibleShopOutput{} }, nil, []string{"user", "shops"}, []application.AccessibleShopOutput{}},
		{"one eligible shop", func(s *scenario) { s.shops = s.shops[:1] }, nil, []string{"user", "shops"}, shops[:1]},
		{"all eligible shops without choosing one", func(*scenario) {}, nil, []string{"user", "shops"}, shops},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := scenario{
				input:     application.ListMyShopsInput{UserID: userID},
				user:      domain.User{ID: userID, Active: true},
				userFound: true,
				shops:     append([]application.AccessibleShopOutput{}, shops...),
			}
			tt.change(&s)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			var calls []string
			repository := identityReaderStub{
				t: t,
				find: func(gotCtx context.Context, id uuid.UUID) (domain.User, bool, error) {
					calls = append(calls, "user")
					if gotCtx != ctx || id != s.input.UserID {
						t.Fatal("user lookup must receive the authenticated ID and original context")
					}
					return s.user, s.userFound, s.userErr
				},
			}
			reader := accessibleShopReaderFunc(func(gotCtx context.Context, id uuid.UUID) ([]application.AccessibleShopOutput, error) {
				calls = append(calls, "shops")
				if gotCtx != ctx || id != userID {
					t.Fatal("shop lookup must receive the verified local user ID and original context")
				}
				return s.shops, s.shopsErr
			})

			got, err := application.NewListMyShops(repository, reader).Execute(ctx, s.input)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("Execute() error = %v, want %v", err, tt.wantErr)
			}
			if !reflect.DeepEqual(calls, tt.wantCalls) {
				t.Errorf("calls = %v, want %v", calls, tt.wantCalls)
			}
			if tt.wantErr != nil {
				if got != nil {
					t.Errorf("failed listing returned shops: %+v", got)
				}
				return
			}
			if got == nil {
				t.Fatal("successful listing must return a non-nil slice")
			}
			if !reflect.DeepEqual(got, tt.wantShops) {
				t.Errorf("Execute() shops = %+v, want %+v", got, tt.wantShops)
			}
		})
	}
}

func TestListMyShopsRechecksState(t *testing.T) {
	for _, target := range []string{"deleted user", "inactive user", "changed eligible shops"} {
		t.Run(target, func(t *testing.T) {
			userID := uuid.New()
			userFound, userActive := true, true
			currentShops := []application.AccessibleShopOutput{
				{ShopID: uuid.New(), Name: "Barbershop A", Slug: "barbershop-a"},
			}
			userCalls, shopCalls := 0, 0
			repository := identityReaderStub{
				t: t,
				find: func(context.Context, uuid.UUID) (domain.User, bool, error) {
					userCalls++
					return domain.User{ID: userID, Active: userActive}, userFound, nil
				},
			}
			reader := accessibleShopReaderFunc(func(context.Context, uuid.UUID) ([]application.AccessibleShopOutput, error) {
				shopCalls++
				return currentShops, nil
			})
			uc := application.NewListMyShops(repository, reader)
			input := application.ListMyShopsInput{UserID: userID}
			got, err := uc.Execute(context.Background(), input)
			if err != nil || !reflect.DeepEqual(got, currentShops) {
				t.Fatalf("initial listing = %+v, %v; want %+v and no error", got, err, currentShops)
			}

			var wantErr error
			wantShopCalls := 1
			switch target {
			case "deleted user":
				userFound = false
				wantErr = application.ErrUserNotProvisioned
			case "inactive user":
				userActive = false
				wantErr = application.ErrUserInactive
			case "changed eligible shops":
				currentShops = []application.AccessibleShopOutput{}
				wantShopCalls = 2
			}

			got, err = uc.Execute(context.Background(), input)
			if !errors.Is(err, wantErr) {
				t.Fatalf("listing after state change error = %v, want %v", err, wantErr)
			}
			if wantErr != nil {
				if got != nil {
					t.Fatalf("denied listing returned shops: %+v", got)
				}
			} else if got == nil || len(got) != 0 {
				t.Fatalf("listing must reflect the current empty result, got %+v", got)
			}
			if userCalls != 2 || shopCalls != wantShopCalls {
				t.Errorf("calls = user:%d shops:%d, want user:2 shops:%d", userCalls, shopCalls, wantShopCalls)
			}
		})
	}
}
