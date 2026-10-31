package postgres

import (
	"context"
	"errors"

	db "github.com/Felipe-candido/Barber-chat/internal/database/sqlc"
	catalog "github.com/Felipe-candido/Barber-chat/internal/modules/catalog/application"
	"github.com/jackc/pgx/v5"
)

type ProductGuard struct{ q *db.Queries }

func NewProductGuard(q *db.Queries) *ProductGuard { return &ProductGuard{q} }

var _ catalog.ProductAccess = (*ProductGuard)(nil)

func (g *ProductGuard) RequireProductOwner(ctx context.Context, scope catalog.ProductScope) error {
	return g.require(ctx, scope, true)
}
func (g *ProductGuard) RequireProductStaff(ctx context.Context, scope catalog.ProductScope) error {
	return g.require(ctx, scope, false)
}

func (g *ProductGuard) require(ctx context.Context, scope catalog.ProductScope, owner bool) error {
	seen, err := g.q.GetMembership(ctx, db.GetMembershipParams{UserID: scope.ActorID, ShopID: scope.ShopID})
	if errors.Is(err, pgx.ErrNoRows) {
		return catalog.ErrProductAccessDenied
	}
	if err != nil {
		return err
	}
	if !seen.Active || (seen.Role != "owner" && seen.Role != "barber") || (owner && seen.Role != "owner") {
		return catalog.ErrProductAccessDenied
	}
	// Calendar mutations already lock professional before membership. Preserve
	// that order, then reject a role/link changed between the preliminary reads.
	if seen.Role == "barber" {
		id, err := g.q.LockProductStaffProfessional(ctx, db.LockProductStaffProfessionalParams{UserID: scope.ActorID, ShopID: scope.ShopID})
		if errors.Is(err, pgx.ErrNoRows) {
			return catalog.ErrProductAccessDenied
		}
		if err != nil {
			return err
		}
		if id != seen.ProfessionalID {
			return catalog.ErrProductAccessDenied
		}
	}
	current, err := g.q.LockProductAccess(ctx, db.LockProductAccessParams{UserID: scope.ActorID, ShopID: scope.ShopID})
	if errors.Is(err, pgx.ErrNoRows) {
		return catalog.ErrProductAccessDenied
	}
	if err != nil {
		return err
	}
	if current.Role != seen.Role || current.ProfessionalID != seen.ProfessionalID {
		return catalog.ErrProductAccessDenied
	}
	_, err = g.q.LockActiveProductShop(ctx, scope.ShopID)
	if errors.Is(err, pgx.ErrNoRows) {
		return catalog.ErrProductAccessDenied
	}
	return err
}
