package application_test

import (
	"context"
	"errors"
	app "github.com/Felipe-candido/Barber-chat/internal/modules/identity/application"
	"github.com/google/uuid"
	"testing"
)

type registrationVerifier func(context.Context, string) (uuid.UUID, error)

func (f registrationVerifier) VerifyAccessToken(ctx context.Context, token string) (uuid.UUID, error) {
	return f(ctx, token)
}

type registrationReader func(context.Context, string) (app.RegistrationAccount, error)

func (f registrationReader) ReadRegistrationAccount(ctx context.Context, token string) (app.RegistrationAccount, error) {
	return f(ctx, token)
}

func TestVerifyRegistrationAccount(t *testing.T) {
	id := uuid.New()
	for _, test := range []struct {
		name                     string
		user                     uuid.UUID
		confirmed, anonymous     bool
		verifyErr, readErr, want error
	}{
		{"confirmed", id, true, false, nil, nil, nil}, {"unconfirmed", id, false, false, nil, nil, app.ErrEmailNotConfirmed},
		{"anonymous", id, true, true, nil, nil, app.ErrInvalidAccessToken}, {"mismatch", uuid.New(), true, false, nil, nil, app.ErrIdentityMismatch},
		{"expired", id, true, false, app.ErrInvalidAccessToken, nil, app.ErrInvalidAccessToken},
		{"lookup unavailable", id, true, false, nil, app.ErrRegistrationUnavailable, app.ErrRegistrationUnavailable},
		{"canceled", id, true, false, nil, context.Canceled, context.Canceled},
	} {
		t.Run(test.name, func(t *testing.T) {
			calls := 0
			v := registrationVerifier(func(context.Context, string) (uuid.UUID, error) { return id, test.verifyErr })
			r := registrationReader(func(context.Context, string) (app.RegistrationAccount, error) {
				calls++
				return app.RegistrationAccount{UserID: test.user, EmailConfirmed: test.confirmed, Anonymous: test.anonymous}, test.readErr
			})
			got, err := app.NewVerifyRegistrationAccount(v, r).Execute(context.Background(), "test-token")
			if test.want == nil {
				if err != nil || got.UserID != id {
					t.Fatalf("%+v %v", got, err)
				}
			} else if !errors.Is(err, test.want) || got.UserID != uuid.Nil {
				t.Fatalf("%+v %v", got, err)
			}
			if test.verifyErr != nil && calls != 0 {
				t.Fatal("unverified token reached Auth lookup")
			}
		})
	}
	if _, err := app.NewVerifyRegistrationAccount(nil, nil).Execute(context.Background(), "token"); !errors.Is(err, app.ErrRegistrationNotConfigured) {
		t.Fatal(err)
	}
}
