package domain_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/Felipe-candido/Barber-chat/internal/modules/identity/domain"
	"github.com/google/uuid"
)

func TestNewUser(t *testing.T) {
	providerUserID := uuid.New()

	user, err := domain.NewUser(providerUserID, "  Felipe  ")
	if err != nil {
		t.Fatalf("NewUser() error = %v", err)
	}

	if user.ID != providerUserID {
		t.Fatalf("NewUser() ID = %s, want provider ID %s", user.ID, providerUserID)
	}
	if user.DisplayName != "Felipe" {
		t.Errorf("NewUser() DisplayName = %q, want %q", user.DisplayName, "Felipe")
	}
	if !user.Active {
		t.Error("NewUser() Active = false, want true")
	}
}

func TestNewUserValidation(t *testing.T) {
	validUserID := uuid.New()

	tests := []struct {
		name        string
		userID      uuid.UUID
		displayName string
		wantErr     error
	}{
		{
			name:        "missing user ID",
			displayName: "Felipe",
			wantErr:     domain.ErrInvalidUserID,
		},
		{
			name:        "empty display name",
			userID:      validUserID,
			displayName: "   ",
			wantErr:     domain.ErrInvalidDisplayName,
		},
		{
			name:        "display name over 100 characters",
			userID:      validUserID,
			displayName: strings.Repeat("á", 101),
			wantErr:     domain.ErrInvalidDisplayName,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := domain.NewUser(tt.userID, tt.displayName)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("NewUser() error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestNewUserAcceptsOneHundredUnicodeCharacters(t *testing.T) {
	_, err := domain.NewUser(uuid.New(), strings.Repeat("á", 100))
	if err != nil {
		t.Fatalf("NewUser() error = %v, want nil", err)
	}
}
