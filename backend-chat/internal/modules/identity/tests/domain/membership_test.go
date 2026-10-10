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

	membership, err := domain.NewMembership(userID, shopID, domain.RoleOwner, uuid.Nil)
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
	if membership.Role != domain.RoleOwner || membership.ProfessionalID != uuid.Nil {
		t.Fatal("unexpected owner membership", membership)
	}
	professionalID := uuid.New()
	barber, err := domain.NewMembership(userID, shopID, domain.RoleBarber, professionalID)
	if err != nil || barber.Role != domain.RoleBarber || barber.ProfessionalID != professionalID {
		t.Fatal("barber must retain its professional association", barber, err)
	}
	for _, role := range []domain.Role{"", "admin"} {
		if _, err := domain.NewMembership(userID, shopID, role, uuid.Nil); !errors.Is(err, domain.ErrInvalidRole) {
			t.Fatal("role must be explicit and valid", role, err)
		}
	}
	if _, err := domain.NewMembership(userID, shopID, domain.RoleBarber, uuid.Nil); !errors.Is(err, domain.ErrInvalidProfessionalID) {
		t.Fatal("barber without professional accepted", err)
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
			_, err := domain.NewMembership(tt.userID, tt.shopID, domain.RoleOwner, uuid.Nil)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("NewMembership() error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}
