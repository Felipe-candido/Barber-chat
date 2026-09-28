package domain

import (
	"github.com/google/uuid"
)

type Membership struct {
	UserID uuid.UUID
	ShopID uuid.UUID
	Active bool
}

func NewMembership(userID, shopID uuid.UUID) (Membership, error) {
	if userID == uuid.Nil {
		return Membership{}, ErrInvalidUserID
	}

	if shopID == uuid.Nil {
		return Membership{}, ErrInvalidShopID
	}

	return Membership{
		UserID: userID,
		ShopID: shopID,
		Active: true,
	}, nil
}
