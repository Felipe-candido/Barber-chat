package domain

import "errors"

var (
	ErrInvalidUserID = errors.New("user ID is required")

	ErrInvalidDisplayName = errors.New(
		"display name must contain between 1 and 100 characters",
	)

	ErrInvalidShopID = errors.New("shop ID is required")
)
