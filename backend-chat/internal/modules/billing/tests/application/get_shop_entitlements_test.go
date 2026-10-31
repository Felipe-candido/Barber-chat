package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Felipe-candido/Barber-chat/internal/modules/billing/application"
	"github.com/Felipe-candido/Barber-chat/internal/modules/billing/domain"
	"github.com/google/uuid"
)

type readerFunc func(context.Context, uuid.UUID) (application.ShopBillingState, bool, error)

func (f readerFunc) FindShopBillingState(ctx context.Context, id uuid.UUID) (application.ShopBillingState, bool, error) {
	return f(ctx, id)
}

func TestGetShopEntitlements(t *testing.T) {
	shopID := uuid.New()
	now := time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC)
	failure := errors.New("database unavailable")
	for _, test := range []struct {
		name                        string
		found, active, subscription bool
		shopID, subscriptionShopID  uuid.UUID
		readErr, wantErr            error
	}{
		{"no subscription", true, true, false, shopID, shopID, nil, nil},
		{"paid subscription", true, true, true, shopID, shopID, nil, nil},
		{"missing shop", false, false, false, uuid.Nil, uuid.Nil, nil, application.ErrShopUnavailable},
		{"inactive shop", true, false, true, shopID, shopID, nil, application.ErrShopUnavailable},
		{"wrong shop", true, true, false, uuid.New(), shopID, nil, application.ErrBillingStateMismatch},
		{"cross tenant subscription", true, true, true, shopID, uuid.New(), nil, application.ErrBillingStateMismatch},
		{"database failure", false, false, false, uuid.Nil, uuid.Nil, failure, failure},
		{"database deadline", false, false, false, uuid.Nil, uuid.Nil, context.DeadlineExceeded, context.DeadlineExceeded},
	} {
		t.Run(test.name, func(t *testing.T) {
			calls := 0
			reader := readerFunc(func(ctx context.Context, id uuid.UUID) (application.ShopBillingState, bool, error) {
				calls++
				if id != shopID {
					t.Fatal("reader received wrong tenant")
				}
				state := application.ShopBillingState{ShopID: test.shopID, Active: test.active}
				if test.subscription {
					state.Subscription = &domain.Subscription{ShopID: test.subscriptionShopID, Status: domain.SubscriptionActive,
						PaidFrom: now.Add(-time.Hour), PaidUntil: now.Add(time.Hour)}
				}
				return state, test.found, test.readErr
			})
			uc := application.NewGetShopEntitlements(reader, application.ClockFunc(func() time.Time { return now }))
			got, err := uc.Execute(context.Background(), application.GetShopEntitlementsInput{ShopID: shopID})
			if calls != 1 {
				t.Fatalf("reader calls=%d", calls)
			}
			if test.wantErr != nil {
				if !errors.Is(err, test.wantErr) || got != (application.ShopEntitlementsOutput{}) {
					t.Fatalf("got %+v, %v", got, err)
				}
				return
			}
			if err != nil || got.ShopID != shopID || got.Entitlement.Allowed != test.subscription {
				t.Fatalf("got %+v, %v", got, err)
			}
			if !test.subscription && got.Entitlement.Reason != domain.AccessSubscriptionRequired {
				t.Fatal(got)
			}
		})
	}
}

func TestGetShopEntitlementsRejectsInvalidInputBeforeReading(t *testing.T) {
	reader := readerFunc(func(context.Context, uuid.UUID) (application.ShopBillingState, bool, error) {
		t.Fatal("invalid input must not reach persistence")
		return application.ShopBillingState{}, false, nil
	})
	clock := application.ClockFunc(func() time.Time { t.Fatal("clock must not be read"); return time.Time{} })
	uc := application.NewGetShopEntitlements(reader, clock)
	if _, err := uc.Execute(context.Background(), application.GetShopEntitlementsInput{}); !errors.Is(err, application.ErrInvalidShopID) {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := uc.Execute(ctx, application.GetShopEntitlementsInput{ShopID: uuid.New()}); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}

func TestGetShopEntitlementsPreservesContextAndInvalidState(t *testing.T) {
	type key struct{}
	ctx := context.WithValue(context.Background(), key{}, "request")
	id := uuid.New()
	reader := readerFunc(func(got context.Context, shopID uuid.UUID) (application.ShopBillingState, bool, error) {
		if got != ctx || got.Value(key{}) != "request" {
			t.Fatal("request context lost")
		}
		return application.ShopBillingState{ShopID: id, Active: true,
			Subscription: &domain.Subscription{ShopID: id, Status: domain.SubscriptionActive}}, true, nil
	})
	uc := application.NewGetShopEntitlements(reader, application.ClockFunc(time.Now))
	got, err := uc.Execute(ctx, application.GetShopEntitlementsInput{ShopID: id})
	if !errors.Is(err, domain.ErrInvalidSubscription) || got.Entitlement.Allowed {
		t.Fatalf("got %+v, %v", got, err)
	}
}
