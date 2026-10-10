package domain

import "errors"

var (
	ErrInvalidUserID = errors.New("user ID is required")

	ErrInvalidDisplayName = errors.New(
		"display name must contain between 1 and 100 characters",
	)

	ErrInvalidShopID         = errors.New("shop ID is required")
	ErrInvalidRole           = errors.New("role must be owner or barber")
	ErrInvalidProfessionalID = errors.New("barber access requires a professional ID")
)
