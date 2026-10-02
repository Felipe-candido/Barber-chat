//go:build integration

package integration

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Runs only inside TestIdentityMigrations' disposable, rollback-only database.
// The minimal auth.users fixture does not create a real Supabase Auth account.
func testShopProvisioning(t *testing.T, ctx context.Context, tx pgx.Tx) {
	t.Helper()
	exec := func(sql string) {
		t.Helper()
		if _, err := tx.Exec(ctx, sql); err != nil {
			t.Fatal(err)
		}
	}
	exec("SAVEPOINT shop_provisioning_fixture")
	defer func() {
		if _, err := tx.Exec(ctx, "ROLLBACK TO SAVEPOINT shop_provisioning_fixture; RELEASE SAVEPOINT shop_provisioning_fixture"); err != nil {
			t.Error(err)
		}
	}()
	exec(`INSERT INTO auth.users(id) VALUES
		('30000000-0000-4000-8000-000000000001'),
		('30000000-0000-4000-8000-000000000002')`)

	contents, err := os.ReadFile(filepath.Join("..", "..", "db", "supabase", "provision_shop_and_user.sql"))
	if err != nil {
		t.Fatal(err)
	}
	template := strings.ReplaceAll(string(contents), "\r\n", "\n")
	// Keep the actual script, but leave transaction control to the test fixture.
	if strings.Count(template, "\nBEGIN;\n") != 1 || strings.Count(template, "\nCOMMIT;") != 1 {
		t.Fatal("expected one explicit provisioning transaction")
	}
	template = strings.Replace(template, "\nBEGIN;\n", "\n", 1)
	template = strings.Replace(template, "\nCOMMIT;", "\n", 1)
	type input struct {
		userID, displayName, shopName, slug, timezone string
	}
	valid := input{
		userID:      "30000000-0000-4000-8000-000000000001",
		displayName: "Ana",
		shopName:    "Barbearia de teste",
		slug:        "provisioning-test",
		timezone:    "America/Sao_Paulo",
	}
	quote := func(value string) string { return "'" + strings.ReplaceAll(value, "'", "''") + "'" }
	script := func(values input) string {
		t.Helper()
		result := template
		for original, replacement := range map[string]string{
			"target_user_id UUID := '00000000-0000-0000-0000-000000000000'": "target_user_id UUID := " + quote(values.userID),
			"target_display_name TEXT := 'SUBSTITUA PELO NOME DO USUARIO'":  "target_display_name TEXT := " + quote(values.displayName),
			"target_shop_name TEXT := 'SUBSTITUA PELO NOME DA BARBEARIA'":   "target_shop_name TEXT := " + quote(values.shopName),
			"target_shop_slug TEXT := 'substitua-o-slug'":                   "target_shop_slug TEXT := " + quote(values.slug),
			"target_shop_timezone TEXT := 'America/Sao_Paulo'":              "target_shop_timezone TEXT := " + quote(values.timezone),
		} {
			if strings.Count(result, original) != 1 {
				t.Fatalf("missing or ambiguous input declaration: %s", original)
			}
			result = strings.Replace(result, original, replacement, 1)
		}
		return result
	}
	counts := func() [3]int {
		t.Helper()
		var result [3]int
		err := tx.QueryRow(ctx, `SELECT (SELECT count(*) FROM public.shops),
			(SELECT count(*) FROM public.users), (SELECT count(*) FROM public.shop_memberships)`).
			Scan(&result[0], &result[1], &result[2])
		if err != nil {
			t.Fatal(err)
		}
		return result
	}
	run := func(name string, check func(*testing.T)) {
		t.Run(name, func(t *testing.T) {
			exec("SAVEPOINT shop_provisioning_case")
			defer exec("ROLLBACK TO SAVEPOINT shop_provisioning_case; RELEASE SAVEPOINT shop_provisioning_case")
			check(t)
		})
	}
	reject := func(t *testing.T, sql, message string) {
		t.Helper()
		before := counts()
		exec("SAVEPOINT shop_provisioning_attempt")
		_, err := tx.Exec(ctx, sql)
		exec("ROLLBACK TO SAVEPOINT shop_provisioning_attempt; RELEASE SAVEPOINT shop_provisioning_attempt")
		if err == nil || !strings.Contains(err.Error(), message) {
			t.Fatalf("expected %q, got %v", message, err)
		}
		if after := counts(); after != before {
			t.Fatalf("failed provisioning changed row counts: before=%v after=%v", before, after)
		}
	}

	run("creates records and repeats without duplicates", func(t *testing.T) {
		exec(script(valid))
		exec(script(valid))
		if got := counts(); got != [3]int{1, 1, 1} {
			t.Fatalf("unexpected row counts: %v", got)
		}
		var userID, displayName, shopID, shopName, slug, timezone string
		var userActive, shopActive, membershipActive bool
		err := tx.QueryRow(ctx, `SELECT u.id::text, u.display_name, s.id::text,
			s.name, s.slug, s.timezone, u.active, s.active, m.active
			FROM public.users u JOIN public.shop_memberships m ON m.user_id = u.id
			JOIN public.shops s ON s.id = m.shop_id
			WHERE u.id = current_setting('barber_chat.provisioned_user_id')::uuid
			AND s.id = current_setting('barber_chat.provisioned_shop_id')::uuid`).
			Scan(&userID, &displayName, &shopID, &shopName, &slug, &timezone, &userActive, &shopActive, &membershipActive)
		if err != nil {
			t.Fatal(err)
		}
		id, err := uuid.Parse(shopID)
		if err != nil || id == uuid.Nil || shopID == userID || userID != valid.userID ||
			displayName != valid.displayName || shopName != valid.shopName || slug != valid.slug ||
			timezone != valid.timezone || !userActive || !shopActive || !membershipActive {
			t.Fatal("provisioned identity, shop or membership does not match inputs")
		}
	})
	run("reuses profile for another shop without overwriting it", func(t *testing.T) {
		exec(script(valid))
		other := valid
		other.slug, other.shopName, other.displayName = "another-shop", "Another shop", "Do not overwrite"
		exec(script(other))
		var displayName string
		if err := tx.QueryRow(ctx, "SELECT display_name FROM public.users WHERE id = $1", valid.userID).Scan(&displayName); err != nil {
			t.Fatal(err)
		}
		if got := counts(); got != [3]int{2, 1, 2} || displayName != valid.displayName {
			t.Fatalf("unexpected profile reuse: counts=%v name=%q", got, displayName)
		}
	})
	run("links another Auth user to the same matching shop", func(t *testing.T) {
		exec(script(valid))
		other := valid
		other.userID, other.displayName = "30000000-0000-4000-8000-000000000002", "Bruno"
		exec(script(other))
		if got := counts(); got != [3]int{1, 2, 2} {
			t.Fatalf("unexpected row counts: %v", got)
		}
	})
	run("accepts names with quotes and trims surrounding spaces", func(t *testing.T) {
		values := valid
		values.shopName, values.displayName = "  Barbearia O'Neil  ", "  João  "
		exec(script(values))
		var shopName, displayName string
		if err := tx.QueryRow(ctx, "SELECT s.name, u.display_name FROM public.shops s CROSS JOIN public.users u").Scan(&shopName, &displayName); err != nil {
			t.Fatal(err)
		}
		if shopName != "Barbearia O'Neil" || displayName != "João" {
			t.Fatal("names were not preserved and trimmed")
		}
	})

	for _, tc := range []struct {
		name, message string
		change        func(*input)
	}{
		{"nil UUID", "Replace target_user_id", func(v *input) { v.userID = uuid.Nil.String() }},
		{"unknown Auth UUID", "User UUID not found", func(v *input) { v.userID = "30000000-0000-4000-8000-000000000099" }},
		{"blank display name", "Provide a user display name", func(v *input) { v.displayName = "   " }},
		{"long display name", "Provide a user display name", func(v *input) { v.displayName = strings.Repeat("a", 101) }},
		{"blank shop name", "Provide a shop name", func(v *input) { v.shopName = "   " }},
		{"long shop name", "Provide a shop name", func(v *input) { v.shopName = strings.Repeat("a", 151) }},
		{"unsafe slug", "Provide a lowercase shop slug", func(v *input) { v.slug = "other/shop" }},
		{"uppercase slug", "Provide a lowercase shop slug", func(v *input) { v.slug = "Other-Shop" }},
		{"long slug", "Provide a lowercase shop slug", func(v *input) { v.slug = strings.Repeat("a", 101) }},
		{"unknown timezone", "Provide a recognized timezone", func(v *input) { v.timezone = "Invalid/Zone" }},
	} {
		run(tc.name, func(t *testing.T) {
			values := valid
			tc.change(&values)
			reject(t, script(values), tc.message)
		})
	}
	for _, tc := range []struct{ name, update, message string }{
		{"disabled user", "UPDATE public.users SET active = false", "User is disabled"},
		{"disabled shop", "UPDATE public.shops SET active = false", "Shop is disabled"},
		{"disabled membership", "UPDATE public.shop_memberships SET active = false", "Membership is disabled"},
	} {
		run(tc.name, func(t *testing.T) {
			exec(script(valid))
			exec(tc.update)
			reject(t, script(valid), tc.message)
		})
	}
	run("disabled profile cannot leave an orphan new shop", func(t *testing.T) {
		exec(script(valid))
		exec("UPDATE public.users SET active = false")
		other := valid
		other.slug = "must-not-be-created"
		reject(t, script(other), "User is disabled")
	})
	for _, field := range []string{"name", "timezone"} {
		run("slug collision with different "+field, func(t *testing.T) {
			exec(script(valid))
			other := valid
			other.userID = "30000000-0000-4000-8000-000000000002"
			if field == "name" {
				other.shopName = "A different shop"
			} else {
				other.timezone = "UTC"
			}
			reject(t, script(other), "Slug already belongs to a shop with different data")
		})
	}
	run("missing Supabase identity migration", func(t *testing.T) {
		exec("ALTER TABLE public.users DROP CONSTRAINT users_auth_user_fk")
		reject(t, script(valid), "Apply db/supabase/migrations")
	})
	run("plain PostgreSQL is rejected instead of skipping Auth checks", func(t *testing.T) {
		exec("ALTER TABLE public.users DROP CONSTRAINT users_auth_user_fk; DROP TABLE auth.users")
		reject(t, script(valid), "Run this script in Supabase")
	})
}
