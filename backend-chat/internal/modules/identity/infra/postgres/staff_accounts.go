package postgres

import (
	"context"
	"errors"
	"time"

	db "github.com/Felipe-candido/Barber-chat/internal/database/sqlc"
	"github.com/Felipe-candido/Barber-chat/internal/modules/identity/application"
	"github.com/Felipe-candido/Barber-chat/internal/modules/identity/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type AccountRepository struct{ pool *pgxpool.Pool }

var _ application.StaffAccountStore = (*AccountRepository)(nil)

func NewAccountRepository(pool *pgxpool.Pool) *AccountRepository {
	return &AccountRepository{pool: pool}
}

func (r *AccountRepository) asOwner(ctx context.Context, actorID, shopID uuid.UUID, fn func(*db.Queries) error) error {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return err
	}
	defer func() {
		rollbackCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = tx.Rollback(rollbackCtx)
	}()
	q := db.New(tx)
	if _, err = q.LockAccountOwner(ctx, db.LockAccountOwnerParams{UserID: actorID, ShopID: shopID}); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return application.ErrShopAccessDenied
		}
		return err
	}
	if err = fn(q); err != nil {
		return err
	}
	err = tx.Commit(ctx)
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return application.ErrAccountExists
	}
	return err
}

func (r *AccountRepository) ListAccounts(ctx context.Context, actorID, shopID uuid.UUID) ([]application.StaffAccount, error) {
	items := make([]application.StaffAccount, 0)
	err := r.asOwner(ctx, actorID, shopID, func(q *db.Queries) error {
		rows, err := q.ListStaffAccounts(ctx, shopID)
		if err != nil {
			return err
		}
		for _, row := range rows {
			items = append(items, application.StaffAccount{ID: row.ID, ProfessionalID: row.ProfessionalID, Email: row.Email, UserID: row.UserID, State: row.State, Active: row.Active})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return items, nil
}

func accountFromRow(row db.FindStaffAccountRow) application.StaffAccount {
	state := row.State
	if state == "linked" && row.UserID == uuid.Nil {
		state = "review"
	}
	return application.StaffAccount{ID: row.ID, ProfessionalID: row.ProfessionalID, Email: row.Email, UserID: row.UserID, State: state}
}

func findAccount(ctx context.Context, q *db.Queries, shopID, accountID uuid.UUID) (application.StaffAccount, error) {
	row, err := q.FindStaffAccount(ctx, db.FindStaffAccountParams{ShopID: shopID, ID: accountID})
	if errors.Is(err, pgx.ErrNoRows) {
		return application.StaffAccount{}, application.ErrAccountNotFound
	}
	if err != nil {
		return application.StaffAccount{}, err
	}
	return accountFromRow(row), nil
}

func (r *AccountRepository) FindAccount(ctx context.Context, actorID, shopID, accountID uuid.UUID) (application.StaffAccount, error) {
	var account application.StaffAccount
	err := r.asOwner(ctx, actorID, shopID, func(q *db.Queries) error {
		var err error
		account, err = findAccount(ctx, q, shopID, accountID)
		return err
	})
	if err != nil {
		return application.StaffAccount{}, err
	}
	return account, nil
}

func (r *AccountRepository) ReserveAccount(ctx context.Context, actorID, shopID, professionalID uuid.UUID, email string) (application.StaffAccount, error) {
	id, err := uuid.NewRandom()
	if err != nil {
		return application.StaffAccount{}, err
	}
	err = r.asOwner(ctx, actorID, shopID, func(q *db.Queries) error {
		p, err := q.LockAccountProfessional(ctx, db.LockAccountProfessionalParams{ShopID: shopID, ID: professionalID})
		if errors.Is(err, pgx.ErrNoRows) {
			return application.ErrAccountNotFound
		}
		if err != nil {
			return err
		}
		if !p.Active {
			return application.ErrShopAccessDenied
		}
		_, err = q.ReserveStaffAccount(ctx, db.ReserveStaffAccountParams{ID: id, ShopID: shopID, ProfessionalID: professionalID, Email: email})
		if errors.Is(err, pgx.ErrNoRows) {
			return application.ErrAccountExists
		}
		return err
	})
	if err != nil {
		return application.StaffAccount{}, err
	}
	return application.StaffAccount{ID: id, ProfessionalID: professionalID, Email: email, State: "pending"}, nil
}

func (r *AccountRepository) LinkAccount(ctx context.Context, actorID, shopID, accountID uuid.UUID, user application.ProviderUser) (application.StaffAccount, error) {
	var account application.StaffAccount
	err := r.asOwner(ctx, actorID, shopID, func(q *db.Queries) error {
		var err error
		account, err = findAccount(ctx, q, shopID, accountID)
		if err != nil {
			return err
		}
		p, err := q.LockAccountProfessional(ctx, db.LockAccountProfessionalParams{ShopID: shopID, ID: account.ProfessionalID})
		if err != nil {
			return err
		}
		if !p.Active {
			return application.ErrShopAccessDenied
		}
		// Re-read after the common professional lock; another request may have linked it.
		account, err = findAccount(ctx, q, shopID, accountID)
		if err != nil {
			return err
		}
		if account.State == "linked" || account.UserID != uuid.Nil {
			return application.ErrAccountExists
		}
		if user.ID == actorID || user.ID == uuid.Nil || user.Email != account.Email {
			return application.ErrInvalidAccount
		}
		profile, err := domain.NewUser(user.ID, p.Name)
		if err != nil {
			return err
		}
		if err = q.InsertStaffProfile(ctx, db.InsertStaffProfileParams{ID: profile.ID, DisplayName: profile.DisplayName}); err != nil {
			return err
		}
		active, err := q.LockStaffProfile(ctx, user.ID)
		if err != nil {
			return err
		}
		if !active {
			return application.ErrUserInactive
		}
		if err = q.InsertBarberMembership(ctx, db.InsertBarberMembershipParams{UserID: user.ID, ShopID: shopID, ProfessionalID: pgtype.UUID{Bytes: account.ProfessionalID, Valid: true}}); err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == "23505" {
				return application.ErrAccountExists
			}
			return err
		}
		m, err := q.GetMembership(ctx, db.GetMembershipParams{UserID: user.ID, ShopID: shopID})
		if err != nil {
			return err
		}
		if m.Role != "barber" || m.ProfessionalID != account.ProfessionalID || !m.Active {
			return application.ErrAccountExists
		}
		changed, err := q.LinkStaffAccount(ctx, db.LinkStaffAccountParams{ShopID: shopID, ID: accountID, AuthUserID: pgtype.UUID{Bytes: user.ID, Valid: true}})
		if err != nil {
			return err
		}
		if changed != 1 {
			return application.ErrAccountNotFound
		}
		account.UserID = user.ID
		account.State = "linked"
		account.Active = true
		return nil
	})
	if err != nil {
		return application.StaffAccount{}, err
	}
	return account, nil
}

func (r *AccountRepository) ReserveReplacement(ctx context.Context, actorID, shopID, accountID uuid.UUID, email string) (application.StaffAccount, error) {
	id, err := uuid.NewRandom()
	if err != nil {
		return application.StaffAccount{}, err
	}
	var original application.StaffAccount
	err = r.asOwner(ctx, actorID, shopID, func(q *db.Queries) error {
		var err error
		original, err = findAccount(ctx, q, shopID, accountID)
		if err != nil {
			return err
		}
		p, err := q.LockAccountProfessional(ctx, db.LockAccountProfessionalParams{ShopID: shopID, ID: original.ProfessionalID})
		if err != nil {
			return err
		}
		if !p.Active {
			return application.ErrShopAccessDenied
		}
		// Serialize replacement, linking and activation on the same professional.
		original, err = findAccount(ctx, q, shopID, accountID)
		if err != nil {
			return err
		}
		if original.Email == email {
			return application.ErrInvalidAccount
		}
		if original.UserID != uuid.Nil {
			m, err := q.LockAccountMembership(ctx, db.LockAccountMembershipParams{ShopID: shopID, UserID: original.UserID})
			if err != nil && !errors.Is(err, pgx.ErrNoRows) {
				return err
			}
			if err == nil {
				if m.Role != "barber" || m.ProfessionalID != original.ProfessionalID {
					return application.ErrShopAccessDenied
				}
				// Use stored membership state, not effective activity of user/professional.
				if m.Active {
					return application.ErrAccountMustBeInactive
				}
				changed, err := q.RemoveInactiveBarberMembership(ctx, db.RemoveInactiveBarberMembershipParams{ShopID: shopID, UserID: original.UserID, ProfessionalID: pgtype.UUID{Bytes: original.ProfessionalID, Valid: true}})
				if err != nil {
					return err
				}
				if changed != 1 {
					return application.ErrAccountNotFound
				}
			}
		}
		changed, err := q.RetireStaffAccount(ctx, db.RetireStaffAccountParams{ShopID: shopID, ID: accountID})
		if err != nil {
			return err
		}
		if changed != 1 {
			return application.ErrAccountNotFound
		}
		_, err = q.ReserveStaffAccount(ctx, db.ReserveStaffAccountParams{ID: id, ShopID: shopID, ProfessionalID: original.ProfessionalID, Email: email})
		if errors.Is(err, pgx.ErrNoRows) {
			return application.ErrAccountExists
		}
		return err
	})
	if err != nil {
		return application.StaffAccount{}, err
	}
	return application.StaffAccount{ID: id, ProfessionalID: original.ProfessionalID, Email: email, State: "pending"}, nil
}

func (r *AccountRepository) ReviewAccount(ctx context.Context, shopID, accountID uuid.UUID) error {
	return db.New(r.pool).ReviewStaffAccount(ctx, db.ReviewStaffAccountParams{ShopID: shopID, ID: accountID})
}

func (r *AccountRepository) SetAccountActive(ctx context.Context, actorID, shopID, accountID uuid.UUID, active bool) error {
	return r.asOwner(ctx, actorID, shopID, func(q *db.Queries) error {
		account, err := findAccount(ctx, q, shopID, accountID)
		if err != nil {
			return err
		}
		if account.UserID == uuid.Nil || account.State != "linked" {
			return application.ErrAccountNeedsReview
		}
		p, err := q.LockAccountProfessional(ctx, db.LockAccountProfessionalParams{ShopID: shopID, ID: account.ProfessionalID})
		if err != nil {
			return err
		}
		// A replacement may have retired this UUID while we waited for the lock.
		account, err = findAccount(ctx, q, shopID, accountID)
		if err != nil {
			return err
		}
		profileActive, err := q.LockStaffProfile(ctx, account.UserID)
		if err != nil {
			return err
		}
		if active && (!p.Active || !profileActive) {
			return application.ErrShopAccessDenied
		}
		changed, err := q.SetBarberAccountActive(ctx, db.SetBarberAccountActiveParams{ShopID: shopID, UserID: account.UserID, ProfessionalID: pgtype.UUID{Bytes: account.ProfessionalID, Valid: true}, Active: active})
		if err != nil {
			return err
		}
		if changed != 1 {
			return application.ErrAccountNotFound
		}
		return nil
	})
}
