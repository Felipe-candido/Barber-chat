package domain_test

import (
	"errors"
	"testing"

	"github.com/Felipe-candido/Barber-chat/internal/modules/identity/domain"
	"github.com/google/uuid"
)

func TestNewMembership(t *testing.T) {
	userID := uuid.New()
	shopID := uuid.New()

	membership, err := domain.NewMembership(userID, shopID)
	if err != nil {
		t.Fatalf("NewMembership() error = %v", err)
	}

	if membership.UserID != userID {
		t.Errorf("NewMembership() UserID = %s, want %s", membership.UserID, userID)
	}
	if membership.ShopID != shopID {
		t.Errorf("NewMembership() ShopID = %s, want %s", membership.ShopID, shopID)
	}
	if !membership.Active {
		t.Error("NewMembership() Active = false, want true")
	}
}

func TestNewMembershipValidation(t *testing.T) {
	tests := []struct {
		name    string
		userID  uuid.UUID
		shopID  uuid.UUID
		wantErr error
	}{
		{
			name:    "missing user ID",
			shopID:  uuid.New(),
			wantErr: domain.ErrInvalidUserID,
		},
		{
			name:    "missing shop ID",
			userID:  uuid.New(),
			wantErr: domain.ErrInvalidShopID,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := domain.NewMembership(tt.userID, tt.shopID)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("NewMembership() error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}
