package postgres

import (
	"context"
	"errors"
	"time"

	db "github.com/Felipe-candido/Barber-chat/internal/database/sqlc"
	"github.com/Felipe-candido/Barber-chat/internal/modules/shops/application"
	"github.com/Felipe-candido/Barber-chat/internal/modules/shops/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type SettingsStore struct{ queries *db.Queries }

func NewSettingsStore(queries *db.Queries) *SettingsStore { return &SettingsStore{queries: queries} }

func (s *SettingsStore) FindSettings(ctx context.Context, shopID uuid.UUID) (application.ShopSettings, bool, error) {
	row, err := s.queries.GetShopSettings(ctx, shopID)
	if errors.Is(err, pgx.ErrNoRows) {
		return application.ShopSettings{}, false, nil
	}
	if err != nil {
		return application.ShopSettings{}, false, err
	}
	settings := application.ShopSettings{ShopID: row.ID, Name: row.Name, Slug: row.Slug, Timezone: row.Timezone,
		Phone: row.Phone, Description: row.Description, Version: row.SettingsVersion}
	if row.HasAddress {
		settings.Address = &domain.Address{PostalCode: row.PostalCode, Street: row.Street, Number: row.Number,
			Complement: row.Complement, District: row.District, City: row.City, State: row.State}
	}
	return settings, true, nil
}

func (s *SettingsStore) ReplaceSettings(ctx context.Context, shopID uuid.UUID, version int64, settings domain.BasicSettings) (bool, error) {
	count, err := s.queries.ReplaceShopBasicSettings(ctx, db.ReplaceShopBasicSettingsParams{ShopID: shopID, ExpectedVersion: version,
		Name: settings.Name, Phone: settings.Phone, Description: settings.Description})
	if err != nil || count == 0 {
		return false, err
	}
	address := settings.Address
	err = s.queries.UpsertShopSettingsAddress(ctx, db.UpsertShopSettingsAddressParams{ShopID: shopID, PostalCode: address.PostalCode,
		Street: address.Street, Number: address.Number, Complement: address.Complement, District: address.District, City: address.City, State: address.State})
	return err == nil, err
}

type SettingsTransactor struct {
	pool *pgxpool.Pool
	bind func(*db.Queries) application.ShopSettingsTx
}

func NewSettingsTransactor(pool *pgxpool.Pool, bind func(*db.Queries) application.ShopSettingsTx) *SettingsTransactor {
	return &SettingsTransactor{pool: pool, bind: bind}
}

func (t *SettingsTransactor) WithinShopSettingsTx(ctx context.Context, fn func(context.Context, application.ShopSettingsTx) error) error {
	tx, err := t.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return err
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = tx.Rollback(cleanup)
	}()
	if err := fn(ctx, t.bind(db.New(tx))); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
