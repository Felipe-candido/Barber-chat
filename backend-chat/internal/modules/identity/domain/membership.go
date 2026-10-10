package domain

import (
	"github.com/google/uuid"
)

type Membership struct {
	UserID         uuid.UUID
	ShopID         uuid.UUID
	Active         bool
	Role           Role
	ProfessionalID uuid.UUID
}

func NewMembership(userID, shopID uuid.UUID, role Role, professionalID uuid.UUID) (Membership, error) {
	if userID == uuid.Nil {
		return Membership{}, ErrInvalidUserID
	}

	if shopID == uuid.Nil {
		return Membership{}, ErrInvalidShopID
	}
	if !role.Valid() {
		return Membership{}, ErrInvalidRole
	}
	if role == RoleBarber && professionalID == uuid.Nil {
		return Membership{}, ErrInvalidProfessionalID
	}

	return Membership{
		UserID:         userID,
		ShopID:         shopID,
		Active:         true,
		Role:           role,
		ProfessionalID: professionalID,
	}, nil
}
