package postgres

import (
	"context"
	"errors"
	"strings"

	db "github.com/Felipe-candido/Barber-chat/internal/database/sqlc"
	notifications "github.com/Felipe-candido/Barber-chat/internal/modules/notifications/application"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type MessageShops struct{ queries *db.Queries }

func NewMessageShops(q *db.Queries) *MessageShops { return &MessageShops{q} }

func (s *MessageShops) LockDeliveryShop(ctx context.Context, shop uuid.UUID) (bool, error) {
	active, err := s.queries.LockNotificationDeliveryShop(ctx, shop)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	return active, err
}

func (s *MessageShops) FindMessageShop(ctx context.Context, shopID uuid.UUID) (notifications.MessageShop, bool, error) {
	r, err := s.queries.GetNotificationMessageShop(ctx, shopID)
	if errors.Is(err, pgx.ErrNoRows) {
		return notifications.MessageShop{}, false, nil
	}
	if err != nil {
		return notifications.MessageShop{}, false, err
	}
	parts := []string{r.Street + ", " + r.Number}
	if r.Complement != "" {
		parts = append(parts, r.Complement)
	}
	parts = append(parts, r.District, r.City+" - "+r.State, "CEP "+r.PostalCode)
	return notifications.MessageShop{ShopID: r.ID, Name: r.Name, Slug: r.Slug, Timezone: r.Timezone, Address: strings.Join(parts, ", ")}, true, nil
}
