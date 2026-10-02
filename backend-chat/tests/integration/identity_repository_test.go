//go:build integration

package integration

import (
	"context"
	"reflect"
	"testing"

	db "github.com/Felipe-candido/Barber-chat/internal/database/sqlc"
	"github.com/Felipe-candido/Barber-chat/internal/modules/identity/application"
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
	t.Run("accessible shops", func(t *testing.T) {
		testIdentityAccessibleShops(t, ctx, tx)
	})
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

// Run only inside TestIdentityMigrations' disposable database and Auth fixture.
// Nested transactions restore both the setup and each scenario after execution.
func testIdentityAccessibleShops(t *testing.T, ctx context.Context, tx pgx.Tx) {
	t.Helper()
	fixtureTx, err := tx.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := fixtureTx.Rollback(ctx); err != nil {
			t.Error(err)
		}
	}()

	userID := uuid.MustParse("20000000-0000-4000-8000-000000000001")
	shopA := uuid.MustParse("10000000-0000-4000-8000-000000000001")
	shopB := uuid.MustParse("10000000-0000-4000-8000-000000000002")
	otherUser, noMembershipUser := uuid.New(), uuid.New()
	inactiveShop, inactiveMembershipShop, otherShop := uuid.New(), uuid.New(), uuid.New()
	for _, statement := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO auth.users (id) VALUES ($1), ($2)`, []any{otherUser, noMembershipUser}},
		{`INSERT INTO public.users (id, display_name) VALUES ($1, 'Other user'), ($2, 'No memberships')`, []any{otherUser, noMembershipUser}},
		{`INSERT INTO public.shops (id, name, slug, active) VALUES
			($1, 'Shop inactive', 'selection-inactive-shop', false),
			($2, 'Membership inactive', 'selection-inactive-membership', true),
			($3, 'Other user only', 'selection-other-user', true)`, []any{inactiveShop, inactiveMembershipShop, otherShop}},
		{`INSERT INTO public.shop_memberships (user_id, shop_id, active) VALUES
			($1, $2, true), ($1, $3, false), ($4, $5, true), ($4, $6, true)`,
			[]any{userID, inactiveShop, inactiveMembershipShop, otherUser, shopA, otherShop}},
	} {
		if _, err := fixtureTx.Exec(ctx, statement.sql, statement.args...); err != nil {
			t.Fatal(err)
		}
	}

	a := application.AccessibleShopOutput{ShopID: shopA, Name: "One", Slug: "identity-test-one"}
	b := application.AccessibleShopOutput{ShopID: shopB, Name: "Two", Slug: "identity-test-two"}
	empty := []application.AccessibleShopOutput{}
	tests := []struct {
		name   string
		userID uuid.UUID
		change string
		args   []any
		want   []application.AccessibleShopOutput
	}{
		{name: "eligible shops only", userID: userID, want: []application.AccessibleShopOutput{a, b}},
		{name: "another user's memberships", userID: otherUser, want: []application.AccessibleShopOutput{
			a, {ShopID: otherShop, Name: "Other user only", Slug: "selection-other-user"},
		}},
		{name: "no memberships", userID: noMembershipUser, want: empty},
		{name: "unknown user", userID: uuid.New(), want: empty},
		{name: "empty user ID", userID: uuid.Nil, want: empty},
		{name: "inactive user", userID: userID,
			change: `UPDATE public.users SET active = false WHERE id = $1`, args: []any{userID}, want: empty},
		{name: "inactive membership", userID: userID,
			change: `UPDATE public.shop_memberships SET active = false WHERE user_id = $1 AND shop_id = $2`, args: []any{userID, shopA},
			want: []application.AccessibleShopOutput{b}},
		{name: "inactive shop", userID: userID,
			change: `UPDATE public.shops SET active = false WHERE id = $1`, args: []any{shopB},
			want: []application.AccessibleShopOutput{a}},
		{name: "all memberships inactive", userID: userID,
			change: `UPDATE public.shop_memberships SET active = false WHERE user_id = $1`, args: []any{userID}, want: empty},
		{name: "all linked shops inactive", userID: userID,
			change: `UPDATE public.shops SET active = false WHERE id IN ($1, $2, $3, $4)`,
			args:   []any{shopA, shopB, inactiveShop, inactiveMembershipShop}, want: empty},
		{name: "deleted membership", userID: userID,
			change: `DELETE FROM public.shop_memberships WHERE user_id = $1 AND shop_id = $2`, args: []any{userID, shopA},
			want: []application.AccessibleShopOutput{b}},
		{name: "deleted local profile", userID: userID,
			change: `DELETE FROM public.users WHERE id = $1`, args: []any{userID}, want: empty},
		{name: "alphabetical order rather than ID order", userID: userID,
			change: `UPDATE public.shops SET name = CASE WHEN id = $1 THEN 'Zulu' ELSE 'Alpha' END WHERE id IN ($1, $2)`,
			args:   []any{shopA, shopB}, want: []application.AccessibleShopOutput{
				{ShopID: shopB, Name: "Alpha", Slug: b.Slug}, {ShopID: shopA, Name: "Zulu", Slug: a.Slug},
			}},
		{name: "same names ordered by ID", userID: userID,
			change: `UPDATE public.shops SET name = 'Same name' WHERE id IN ($1, $2)`, args: []any{shopA, shopB},
			want: []application.AccessibleShopOutput{
				{ShopID: shopA, Name: "Same name", Slug: a.Slug}, {ShopID: shopB, Name: "Same name", Slug: b.Slug},
			}},
		{name: "current shop name and slug", userID: userID,
			change: `UPDATE public.shops SET name = 'Renamed shop', slug = 'selection-renamed' WHERE id = $1`, args: []any{shopA},
			want: []application.AccessibleShopOutput{{ShopID: shopA, Name: "Renamed shop", Slug: "selection-renamed"}, b}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scenarioTx, err := fixtureTx.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer func() {
				if err := scenarioTx.Rollback(ctx); err != nil {
					t.Error(err)
				}
			}()
			if tt.change != "" {
				if _, err := scenarioTx.Exec(ctx, tt.change, tt.args...); err != nil {
					t.Fatal(err)
				}
			}

			repo := identitypostgres.NewRepository(db.New(scenarioTx))
			got, err := repo.ListActiveShopsByUser(ctx, tt.userID)
			if err != nil || !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("ListActiveShopsByUser() = %+v, %v; want %+v and no error", got, err, tt.want)
			}
		})
	}
}
