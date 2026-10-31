package postgres

import (
	"context"
	"errors"

	db "github.com/Felipe-candido/Barber-chat/internal/database/sqlc"
	sales "github.com/Felipe-candido/Barber-chat/internal/modules/sales/application"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type SalesGuard struct{ q *db.Queries }

func NewSalesGuard(q *db.Queries) *SalesGuard { return &SalesGuard{q} }

var _ sales.SalesAccess = (*SalesGuard)(nil)

func (g *SalesGuard) ReadSalesAccess(ctx context.Context, scope sales.SalesScope) (sales.SalesAuthority, error) {
	r, err := g.q.ReadSalesAccess(ctx, db.ReadSalesAccessParams{UserID: scope.ActorID, ShopID: scope.ShopID})
	if errors.Is(err, pgx.ErrNoRows) {
		return sales.SalesAuthority{}, sales.ErrSalesAccessDenied
	}
	if err != nil {
		return sales.SalesAuthority{}, err
	}
	a := sales.SalesAuthority{Scope: scope}
	if r.Role == "owner" {
		a.Owner = true
	} else if r.Role == "barber" && r.ProfessionalID != uuid.Nil {
		a.ProfessionalID = r.ProfessionalID
	} else {
		return a, sales.ErrSalesAccessDenied
	}
	return a, nil
}
func (g *SalesGuard) RequireSalesAccess(ctx context.Context, scope sales.SalesScope) (sales.SalesAuthority, error) {
	seen, err := g.ReadSalesAccess(ctx, scope)
	if err != nil {
		return seen, err
	}
	if !seen.Owner {
		id, err := g.q.LockProductStaffProfessional(ctx, db.LockProductStaffProfessionalParams{UserID: scope.ActorID, ShopID: scope.ShopID})
		if errors.Is(err, pgx.ErrNoRows) {
			return seen, sales.ErrSalesAccessDenied
		}
		if err != nil {
			return seen, err
		}
		if id != seen.ProfessionalID {
			return seen, sales.ErrSalesAccessDenied
		}
	}
	r, err := g.q.LockProductAccess(ctx, db.LockProductAccessParams{UserID: scope.ActorID, ShopID: scope.ShopID})
	if errors.Is(err, pgx.ErrNoRows) {
		return seen, sales.ErrSalesAccessDenied
	}
	if err != nil {
		return seen, err
	}
	current := sales.SalesAuthority{Scope: scope, Owner: r.Role == "owner"}
	if !current.Owner {
		current.ProfessionalID = r.ProfessionalID
	}
	if current.Owner != seen.Owner || current.ProfessionalID != seen.ProfessionalID {
		return current, sales.ErrSalesAccessDenied
	}
	if _, err := g.q.LockActiveProductShop(ctx, scope.ShopID); errors.Is(err, pgx.ErrNoRows) {
		return current, sales.ErrSalesAccessDenied
	} else if err != nil {
		return current, err
	}
	return current, nil
}
