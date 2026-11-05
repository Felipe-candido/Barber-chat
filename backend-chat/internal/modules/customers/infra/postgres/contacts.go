package postgres

import (
	"context"
	"errors"
	db "github.com/Felipe-candido/Barber-chat/internal/database/sqlc"
	app "github.com/Felipe-candido/Barber-chat/internal/modules/customers/application"
	"github.com/Felipe-candido/Barber-chat/internal/modules/customers/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"strings"
	"time"
)

type ContactTransactor struct {
	pool   *pgxpool.Pool
	access func(*db.Queries) app.ContactAccess
}

func NewContactTransactor(pool *pgxpool.Pool, access func(*db.Queries) app.ContactAccess) *ContactTransactor {
	return &ContactTransactor{pool, access}
}
func (t *ContactTransactor) WithinContactTx(ctx context.Context, fn func(context.Context, app.ContactTx) error) error {
	tx, err := t.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return err
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		_ = tx.Rollback(cleanup)
	}()
	q := db.New(tx)
	if err = fn(ctx, app.ContactTx{Access: t.access(q), Contacts: NewContactRepository(q)}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

type ContactRepository struct{ q *db.Queries }

func NewContactRepository(q *db.Queries) *ContactRepository { return &ContactRepository{q} }
func contactModel(c db.Customer) domain.Contact {
	return domain.Contact{ID: c.ID, ShopID: c.ShopID, FirstName: c.FirstName, LastName: c.LastName, Phone: c.PhoneE164, CreatedAt: c.CreatedAt.Time.UTC()}
}
func (r *ContactRepository) Create(ctx context.Context, c domain.Contact) (domain.Contact, error) {
	row, err := r.q.CreateCustomerContact(ctx, db.CreateCustomerContactParams{ID: c.ID, ShopID: c.ShopID, FirstName: c.FirstName, LastName: c.LastName, PhoneE164: c.Phone})
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "customers_shop_id_phone_e164_key" {
		return domain.Contact{}, app.ErrContactExists
	}
	if err != nil {
		return domain.Contact{}, err
	}
	return contactModel(row), nil
}
func (r *ContactRepository) Find(ctx context.Context, shop, id uuid.UUID) (domain.Contact, bool, error) {
	c, err := r.q.GetCustomerContact(ctx, db.GetCustomerContactParams{ShopID: shop, ID: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Contact{}, false, nil
	}
	return contactModel(c), err == nil, err
}
func (r *ContactRepository) List(ctx context.Context, shop uuid.UUID, in app.ContactQuery) ([]domain.Contact, error) {
	pattern := ""
	if in.Name != "" {
		pattern = "%" + strings.NewReplacer("\\", "\\\\", "%", "\\%", "_", "\\_").Replace(in.Name) + "%"
	}
	p := db.ListCustomerContactsParams{ShopID: shop, NamePattern: pattern, Phone: in.Phone, PageLimit: int32(in.Limit), AfterAt: pgtype.Timestamptz{Time: time.Unix(0, 0).UTC(), Valid: true}}
	if in.After != nil {
		p.HasCursor = true
		p.AfterAt = pgtype.Timestamptz{Time: in.After.CreatedAt, Valid: true}
		p.AfterID = in.After.ID
	}
	rows, err := r.q.ListCustomerContacts(ctx, p)
	if err != nil {
		return nil, err
	}
	out := make([]domain.Contact, 0, len(rows))
	for _, row := range rows {
		out = append(out, contactModel(row))
	}
	return out, nil
}
