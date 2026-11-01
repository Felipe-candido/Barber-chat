package postgres

import (
	"context"
	"errors"
	"strings"
	"time"

	db "github.com/Felipe-candido/Barber-chat/internal/database/sqlc"
	app "github.com/Felipe-candido/Barber-chat/internal/modules/notifications/application"
	"github.com/Felipe-candido/Barber-chat/internal/modules/notifications/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Connections struct {
	pool  *pgxpool.Pool
	owner func(*db.Queries) app.ConnectionOwnerAccess
}

func NewConnections(pool *pgxpool.Pool, owner func(*db.Queries) app.ConnectionOwnerAccess) *Connections {
	return &Connections{pool, owner}
}

func (s *Connections) withOwner(ctx context.Context, scope app.ConnectionScope, fn func(*db.Queries) error) error {
	if scope.ActorID == uuid.Nil || scope.ShopID == uuid.Nil {
		return app.ErrConnectionAccessDenied
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() {
		c, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		_ = tx.Rollback(c)
	}()
	q := db.New(tx)
	if err := s.owner(q).RequireConnectionOwner(ctx, scope); err != nil {
		return err
	}
	if err := fn(q); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
func (s *Connections) ReadOwner(ctx context.Context, scope app.ConnectionScope) (domain.Connection, error) {
	var c domain.Connection
	err := s.withOwner(ctx, scope, func(q *db.Queries) error {
		r, err := q.ReadWhatsAppConnection(ctx, scope.ShopID)
		if errors.Is(err, pgx.ErrNoRows) {
			c = domain.Connection{ShopID: scope.ShopID, State: domain.ConnectionNotConfigured}
			return nil
		}
		if err != nil {
			return err
		}
		c = connectionOf(r)
		return nil
	})
	return c, err
}
func (s *Connections) ClaimOwner(ctx context.Context, scope app.ConnectionScope, desired bool) (domain.Connection, error) {
	var c domain.Connection
	err := s.withOwner(ctx, scope, func(q *db.Queries) error {
		id, err := uuid.NewRandom()
		if err != nil {
			return err
		}
		if err := q.EnsureWhatsAppConnection(ctx, db.EnsureWhatsAppConnectionParams{ShopID: scope.ShopID, InstanceName: "bc_" + strings.ReplaceAll(id.String(), "-", "")}); err != nil {
			return err
		}
		token, err := uuid.NewRandom()
		if err != nil {
			return err
		}
		r, err := q.ClaimOwnerWhatsAppAction(ctx, db.ClaimOwnerWhatsAppActionParams{ShopID: scope.ShopID, DesiredConnected: desired, OperationToken: pgtype.UUID{Bytes: token, Valid: true}})
		if errors.Is(err, pgx.ErrNoRows) {
			return app.ErrConnectionBusy
		}
		if err != nil {
			return err
		}
		c = connectionOf(db.ReadWhatsAppConnectionRow(r))
		return nil
	})
	return c, err
}
func (s *Connections) ClaimDue(ctx context.Context) (domain.Connection, bool, error) {
	token, err := uuid.NewRandom()
	if err != nil {
		return domain.Connection{}, false, err
	}
	r, err := db.New(s.pool).ClaimDueWhatsAppConnection(ctx, pgtype.UUID{Bytes: token, Valid: true})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Connection{}, false, nil
	}
	if err != nil {
		return domain.Connection{}, false, err
	}
	return connectionOf(db.ReadWhatsAppConnectionRow(r)), true, nil
}
func (s *Connections) Complete(ctx context.Context, c domain.Connection, o app.ConnectionObservation) (bool, error) {
	if !o.State.Valid() || o.State == domain.ConnectionNotConfigured || c.OperationToken == uuid.Nil {
		return false, app.ErrProviderProtocol
	}
	count, err := db.New(s.pool).CompleteWhatsAppObservation(ctx, db.CompleteWhatsAppObservationParams{
		ShopID: c.ShopID, Generation: c.Generation, OperationToken: pgtype.UUID{Bytes: c.OperationToken, Valid: true}, State: string(o.State), ErrorCode: o.ErrorCode,
	})
	return count == 1, err
}
func (s *Connections) MarkDirty(ctx context.Context, name string) error {
	return db.New(s.pool).MarkWhatsAppConnectionDirty(ctx, name)
}
func connectionOf(r db.ReadWhatsAppConnectionRow) domain.Connection {
	return domain.Connection{ShopID: r.ShopID, InstanceName: r.InstanceName, Generation: r.Generation, DesiredConnected: r.DesiredConnected, State: domain.ConnectionState(r.State),
		VerifiedAt: r.VerifiedAt.Time.UTC(), LastConnectedAt: r.LastConnectedAt.Time.UTC(), ErrorCode: r.ErrorCode, OperationToken: r.OperationToken.Bytes, ObservedAt: r.ObservedAt.Time.UTC()}
}
