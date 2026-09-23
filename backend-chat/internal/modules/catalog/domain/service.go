package domain

import (
	"errors"
	"math"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
)

var (
	ErrInvalidShopID   = errors.New("shop ID is required")
	ErrInvalidName     = errors.New("service name must contain between 1 and 100 characters")
	ErrInvalidDuration = errors.New("duration must be between 1 and 2147483647 minutes")
	ErrInvalidPrice    = errors.New("price cannot be negative")
)

type Service struct {
	ID              uuid.UUID
	ShopID          uuid.UUID
	Name            string
	Description     string
	DurationMinutes int
	PriceCents      int64
	Currency        string
	Active          bool
}

func NewService(shopID uuid.UUID, name, description string, durationMinutes int, priceCents int64) (Service, error) {
	name = strings.TrimSpace(name)

	if shopID == uuid.Nil {
		return Service{}, ErrInvalidShopID
	}

	if name == "" || utf8.RuneCountInString(name) > 100 {
		return Service{}, ErrInvalidName
	}

	if durationMinutes <= 0 || int64(durationMinutes) > math.MaxInt32 {
		return Service{}, ErrInvalidDuration
	}

	if priceCents < 0 {
		return Service{}, ErrInvalidPrice
	}

	return Service{
		ID:              uuid.New(),
		ShopID:          shopID,
		Name:            name,
		Description:     strings.TrimSpace(description),
		DurationMinutes: durationMinutes,
		PriceCents:      priceCents,
		Currency:        "BRL",
		Active:          true,
	}, nil
}
