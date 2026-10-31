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
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type CreationStore struct{ queries *db.Queries }

func NewCreationStore(queries *db.Queries) *CreationStore { return &CreationStore{queries: queries} }

func (s *CreationStore) FindCreation(ctx context.Context, userID uuid.UUID, key [32]byte) (application.ShopCreationRecord, bool, error) {
	row, err := s.queries.FindShopCreation(ctx, db.FindShopCreationParams{UserID: userID, KeyHash: key[:]})
	if errors.Is(err, pgx.ErrNoRows) {
		return application.ShopCreationRecord{}, false, nil
	}
	if err != nil {
		return application.ShopCreationRecord{}, false, err
	}
	if len(row.Fingerprint) != 32 {
		return application.ShopCreationRecord{}, false, application.ErrCreationMismatch
	}
	record := application.ShopCreationRecord{Shop: domain.Shop{ID: row.ID, Name: row.Name, Slug: row.Slug, Timezone: row.Timezone, Active: row.Active}}
	record.Shop.Address = domain.Address{PostalCode: row.PostalCode, Street: row.Street, Number: row.Number, Complement: row.Complement, District: row.District, City: row.City, State: row.State}
	copy(record.Fingerprint[:], row.Fingerprint)
	return record, true, nil
}

func (s *CreationStore) Create(ctx context.Context, shop domain.Shop) error {
	err := s.queries.InsertSelfServiceShop(ctx, db.InsertSelfServiceShopParams{ID: shop.ID, Name: shop.Name, Slug: shop.Slug, Timezone: shop.Timezone})
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "shops_slug_key" {
		return application.ErrSlugTaken
	}
	if err != nil {
		return err
	}
	address := shop.Address
	return s.queries.InsertShopAddress(ctx, db.InsertShopAddressParams{ShopID: shop.ID, PostalCode: address.PostalCode,
		Street: address.Street, Number: address.Number, Complement: address.Complement, District: address.District, City: address.City, State: address.State})
}

func (s *CreationStore) RecordCreation(ctx context.Context, userID uuid.UUID, key, fingerprint [32]byte, shopID uuid.UUID) error {
	return s.queries.RecordShopCreation(ctx, db.RecordShopCreationParams{UserID: userID, KeyHash: key[:], Fingerprint: fingerprint[:], ShopID: shopID})
}

func (r *Resolver) FindPublicShopState(ctx context.Context, slug string) (application.PublicShopState, bool, error) {
	row, err := r.queries.GetPublicShopState(ctx, slug)
	if errors.Is(err, pgx.ErrNoRows) {
		return application.PublicShopState{}, false, nil
	}
	if err != nil {
		return application.PublicShopState{}, false, err
	}
	return application.PublicShopState{ShopID: row.ID, Active: row.Active, PublicationRequested: row.PublicationRequested, LegacyPublicAccess: row.ProvisioningSource == "manual"}, true, nil
}

type CreationTransactor struct {
	pool *pgxpool.Pool
	bind func(*db.Queries) application.ShopCreationTx
}

func NewCreationTransactor(pool *pgxpool.Pool, bind func(*db.Queries) application.ShopCreationTx) *CreationTransactor {
	return &CreationTransactor{pool: pool, bind: bind}
}

func (t *CreationTransactor) WithinShopCreationTx(ctx context.Context, fn func(context.Context, application.ShopCreationTx) error) error {
	tx, err := t.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return err
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = tx.Rollback(cleanup)
	}()
	if err := fn(ctx, t.bind(db.New(tx))); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
