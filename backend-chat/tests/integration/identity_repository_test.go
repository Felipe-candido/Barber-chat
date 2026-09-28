//go:build integration

package integration

import (
	"context"
	"testing"

	db "github.com/Felipe-candido/Barber-chat/internal/database/sqlc"
	"github.com/Felipe-candido/Barber-chat/internal/modules/identity/domain"
	identitypostgres "github.com/Felipe-candido/Barber-chat/internal/modules/identity/infra/postgres"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Reuse the migration test's disposable database, real schema and provisioned
// fixtures. A savepoint restores the state required by subsequent migration checks.
func testIdentityRepository(t *testing.T, ctx context.Context, tx pgx.Tx) {
	t.Helper()
	if _, err := tx.Exec(ctx, "SAVEPOINT identity_repository"); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if _, err := tx.Exec(ctx, "ROLLBACK TO SAVEPOINT identity_repository"); err != nil {
			t.Error(err)
		}
	}()
	userID := uuid.MustParse("20000000-0000-4000-8000-000000000001")
	shopA := uuid.MustParse("10000000-0000-4000-8000-000000000001")
	shopB := uuid.MustParse("10000000-0000-4000-8000-000000000002")
	otherUser := uuid.New()
	for _, statement := range []string{
		"INSERT INTO auth.users (id) VALUES ($1)",
		"INSERT INTO public.users (id, display_name) VALUES ($1, 'Other user')",
	} {
		if _, err := tx.Exec(ctx, statement, otherUser); err != nil {
			t.Fatal(err)
		}
	}
	repo := identitypostgres.NewRepository(db.New(tx))
	user, found, err := repo.FindUserByID(ctx, userID)
	if err != nil || !found || user != (domain.User{ID: userID, DisplayName: "Identity test", Active: true}) {
		t.Fatalf("unexpected provisioned user: %+v, %v, %v", user, found, err)
	}
	if got, found, err := repo.FindUserByID(ctx, uuid.New()); err != nil || found || got != (domain.User{}) {
		t.Fatalf("missing user: %+v, %v, %v", got, found, err)
	}
	if got, err := repo.ListMembershipsByUser(ctx, otherUser); err != nil || got == nil || len(got) != 0 {
		t.Fatalf("user without memberships: %+v, %v", got, err)
	}
	if _, err := tx.Exec(ctx, "INSERT INTO public.shop_memberships (user_id, shop_id) VALUES ($1, $2)", otherUser, shopA); err != nil {
		t.Fatal(err)
	}
	// The other user belongs to A only. B must not be inferred from someone else's membership.
	if got, found, err := repo.FindMembership(ctx, otherUser, shopB); err != nil || found || got != (domain.Membership{}) {
		t.Fatalf("cross-user membership lookup: %+v, %v, %v", got, found, err)
	}
	if _, err := tx.Exec(ctx, "UPDATE public.users SET active = false WHERE id = $1", userID); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, "UPDATE public.shop_memberships SET active = false WHERE user_id = $1 AND shop_id = $2", userID, shopB); err != nil {
		t.Fatal(err)
	}
	user, found, err = repo.FindUserByID(ctx, userID)
	if err != nil || !found || user.ID != userID || user.Active {
		t.Fatalf("inactive user was hidden or reactivated: %+v, %v, %v", user, found, err)
	}
	membership, found, err := repo.FindMembership(ctx, userID, shopB)
	wantInactive := domain.Membership{UserID: userID, ShopID: shopB, Active: false}
	if err != nil || !found || membership != wantInactive {
		t.Fatalf("inactive membership was hidden or reactivated: %+v, %v, %v", membership, found, err)
	}
	memberships, err := repo.ListMembershipsByUser(ctx, userID)
	if err != nil || len(memberships) != 2 {
		t.Fatalf("unexpected memberships: %+v, %v", memberships, err)
	}
	if memberships[0] != (domain.Membership{UserID: userID, ShopID: shopA, Active: true}) || memberships[1] != wantInactive {
		t.Fatalf("list changed ordering, IDs or active flags: %+v", memberships)
	}
	otherMemberships, err := repo.ListMembershipsByUser(ctx, otherUser)
	if err != nil || len(otherMemberships) != 1 || otherMemberships[0] != (domain.Membership{UserID: otherUser, ShopID: shopA, Active: true}) {
		t.Fatalf("list leaked another user's memberships: %+v, %v", otherMemberships, err)
	}
}
