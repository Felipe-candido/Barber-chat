//go:build integration

package integration

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

// This test requires an explicitly configured, empty, disposable PostgreSQL DB.
// The auth fixture proves FK behavior, not Supabase sign-in or JWT validation.
func TestIdentityMigrations(t *testing.T) {
	dsn := os.Getenv("IDENTITY_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set IDENTITY_TEST_DATABASE_URL to an empty disposable PostgreSQL database")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal("could not connect to disposable identity test database")
	}
	defer conn.Close(context.Background())
	tx, err := conn.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	exec := func(sql string) {
		t.Helper()
		if _, err := tx.Exec(ctx, sql); err != nil {
			t.Fatal(err)
		}
	}
	// Refuse the configured application database or an actual Supabase Auth schema.
	exec(`DO $$ BEGIN
		IF to_regclass('public.shops') IS NOT NULL OR to_regclass('auth.users') IS NOT NULL
		   OR to_regclass('public.users') IS NOT NULL THEN
			RAISE EXCEPTION 'Identity migration tests require an empty disposable database';
		END IF;
	END $$;`)
	read := func(path string) string {
		t.Helper()
		contents, err := os.ReadFile(filepath.Join("..", "..", path))
		if err != nil {
			t.Fatal(err)
		}
		return string(contents)
	}
	migrate := func(path string, up bool) {
		t.Helper()
		before, after, found := strings.Cut(read(path), "-- +goose Down")
		if !found {
			t.Fatal("expected a reversible SQL migration")
		}
		if up {
			exec(before)
		} else {
			exec(after)
		}
	}
	const common = "db/migrations/00004_create_identity_tables.sql"
	const provider = "db/supabase/migrations/00001_link_auth_users.sql"
	const roles = "db/migrations/00006_simple_staff_access.sql"
	const accounts = "db/migrations/00007_staff_accounts.sql"
	const replacement = "db/migrations/00008_staff_account_replacement.sql"
	migrate("db/migrations/00001_enable_btree_gist.sql", true)
	migrate("db/migrations/00002_create_catalog_tables.sql", true)
	migrate("db/migrations/00003_service_currency.sql", true)
	migrate(common, true)
	// The provider migration must fail on plain PostgreSQL, rather than skip its FK.
	exec("SAVEPOINT missing_auth")
	providerUp, _, _ := strings.Cut(read(provider), "-- +goose Down")
	if _, err := tx.Exec(ctx, providerUp); err == nil {
		t.Fatal("Supabase migration must reject missing auth.users")
	}
	exec("ROLLBACK TO SAVEPOINT missing_auth")
	exec(`CREATE SCHEMA auth;
		CREATE TABLE auth.users (id UUID PRIMARY KEY);
		DO $$ BEGIN
			IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'anon') THEN CREATE ROLE anon NOLOGIN; END IF;
			IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'authenticated') THEN CREATE ROLE authenticated NOLOGIN; END IF;
		END $$;
		GRANT ALL ON public.users, public.shop_memberships TO anon, authenticated;`)
	migrate(provider, true)
	migrate("db/migrations/00005_booking_flow.sql", true)
	// Legacy members are upgraded once; new inserts must specify the role.
	exec(`INSERT INTO shops(id,name,slug) VALUES ('90000000-0000-4000-8000-000000000001','Legacy','legacy');
		INSERT INTO auth.users VALUES ('90000000-0000-4000-8000-000000000002');
		INSERT INTO public.users(id,display_name) VALUES ('90000000-0000-4000-8000-000000000002','Legacy owner');
		INSERT INTO shop_memberships(user_id,shop_id) VALUES ('90000000-0000-4000-8000-000000000002','90000000-0000-4000-8000-000000000001');`)
	migrate(roles, true)
	exec(`INSERT INTO professionals(id,shop_id,name) VALUES ('90000000-0000-4000-8000-000000000003','90000000-0000-4000-8000-000000000001','Barber');
	DO $$ BEGIN
		IF (SELECT role FROM shop_memberships WHERE user_id='90000000-0000-4000-8000-000000000002') <> 'owner' THEN
			RAISE EXCEPTION 'Legacy access was not preserved';
		END IF;
		BEGIN
			INSERT INTO shop_memberships(user_id,shop_id) VALUES ('90000000-0000-4000-8000-000000000002','90000000-0000-4000-8000-000000000001');
			RAISE EXCEPTION 'Missing role accepted';
		EXCEPTION WHEN not_null_violation THEN NULL; END;
		BEGIN
			INSERT INTO shop_memberships(user_id,shop_id,role) VALUES ('90000000-0000-4000-8000-000000000002','90000000-0000-4000-8000-000000000001','admin');
			RAISE EXCEPTION 'Unknown role accepted';
		EXCEPTION WHEN check_violation THEN NULL; END;
		BEGIN
			INSERT INTO shop_memberships(user_id,shop_id,role) VALUES ('90000000-0000-4000-8000-000000000002','90000000-0000-4000-8000-000000000001','barber');
			RAISE EXCEPTION 'Unlinked barber accepted';
		EXCEPTION WHEN check_violation THEN NULL; END;
		BEGIN
			INSERT INTO shop_memberships(user_id,shop_id,role,professional_id) VALUES ('90000000-0000-4000-8000-000000000002','90000000-0000-4000-8000-000000000001','barber','00000000-0000-0000-0000-000000000000');
			RAISE EXCEPTION 'Nil professional accepted';
		EXCEPTION WHEN check_violation THEN NULL; END;
	END $$;
	UPDATE shop_memberships SET role='barber', professional_id='90000000-0000-4000-8000-000000000003'
	WHERE user_id='90000000-0000-4000-8000-000000000002';
	INSERT INTO shops(id,name,slug) VALUES ('90000000-0000-4000-8000-000000000004','Other','legacy-other');
	DO $$ BEGIN
		BEGIN
			INSERT INTO shop_memberships(user_id,shop_id,role,professional_id) VALUES ('90000000-0000-4000-8000-000000000002','90000000-0000-4000-8000-000000000004','barber','90000000-0000-4000-8000-000000000003');
			RAISE EXCEPTION 'Foreign shop professional accepted';
		EXCEPTION WHEN foreign_key_violation THEN NULL; END;
	END $$;
	DELETE FROM auth.users WHERE id='90000000-0000-4000-8000-000000000002';
	DELETE FROM professionals WHERE id='90000000-0000-4000-8000-000000000003';
	DELETE FROM shops WHERE id='90000000-0000-4000-8000-000000000001';`)
	exec("DELETE FROM shops WHERE id='90000000-0000-4000-8000-000000000004'")
	migrate(accounts, true)
	migrate(replacement, true)
	exec(`DO $$ BEGIN
		IF NOT (SELECT relrowsecurity FROM pg_class WHERE oid='public.staff_accounts'::regclass)
		   OR has_table_privilege('anon','public.staff_accounts','SELECT')
		   OR has_table_privilege('authenticated','public.staff_accounts','INSERT') THEN
			RAISE EXCEPTION 'Staff accounts are exposed to browser roles';
		END IF;
	END $$;`)
	t.Run("shop provisioning", func(t *testing.T) {
		testShopProvisioning(t, ctx, tx)
	})
	exec(`INSERT INTO public.shops (id, name, slug) VALUES
		('10000000-0000-4000-8000-000000000001', 'One', 'identity-test-one'),
		('10000000-0000-4000-8000-000000000002', 'Two', 'identity-test-two');
		INSERT INTO auth.users VALUES ('20000000-0000-4000-8000-000000000001');`)
	// Execute the actual provisioning script inside the rollback-only test transaction.
	provision := read("db/supabase/provision_membership.sql")
	provision = strings.Replace(provision, "BEGIN;", "", 1)
	provision = strings.Replace(provision, "COMMIT;", "", 1)
	provision = strings.Replace(provision, "target_user_id UUID := '00000000-0000-0000-0000-000000000000'", "target_user_id UUID := '20000000-0000-4000-8000-000000000001'", 1)
	provision = strings.Replace(provision, "target_display_name TEXT := 'SUBSTITUA PELO NOME'", "target_display_name TEXT := 'Identity test'", 1)
	provision = strings.Replace(provision, "target_shop_slug TEXT := 'barbearia-do-felipe'", "target_shop_slug TEXT := 'identity-test-one'", 1)
	exec(provision)
	exec(provision)
	exec(strings.Replace(provision, "identity-test-one", "identity-test-two", 1))
	t.Run("repository", func(t *testing.T) {
		testIdentityRepository(t, ctx, tx)
	})
	exec(`DO $$ BEGIN
		IF (SELECT count(*) FROM public.users) <> 1 OR (SELECT count(*) FROM public.shop_memberships) <> 2 THEN
			RAISE EXCEPTION 'Provisioning is not repeatable or does not support multiple shops';
		END IF;
		BEGIN
			INSERT INTO public.users(id, display_name) VALUES ('20000000-0000-4000-8000-000000000099', 'Missing Auth');
			RAISE EXCEPTION 'Missing Auth user accepted';
		EXCEPTION WHEN foreign_key_violation THEN NULL; END;
		BEGIN
			INSERT INTO public.shop_memberships(user_id, shop_id, role) SELECT user_id, shop_id, role FROM public.shop_memberships LIMIT 1;
			RAISE EXCEPTION 'Duplicate membership accepted';
		EXCEPTION WHEN unique_violation THEN NULL; END;
		BEGIN
			INSERT INTO public.shop_memberships(user_id,shop_id,role) VALUES ('20000000-0000-4000-8000-000000000099', '10000000-0000-4000-8000-000000000001','owner');
			RAISE EXCEPTION 'Missing local user accepted';
		EXCEPTION WHEN foreign_key_violation THEN NULL; END;
		BEGIN
			INSERT INTO public.shop_memberships(user_id,shop_id,role) VALUES ('20000000-0000-4000-8000-000000000001', '10000000-0000-4000-8000-000000000099','owner');
			RAISE EXCEPTION 'Missing shop accepted';
		EXCEPTION WHEN foreign_key_violation THEN NULL; END;
		IF EXISTS (SELECT 1 FROM pg_class WHERE oid IN ('public.users'::regclass, 'public.shop_memberships'::regclass) AND NOT relrowsecurity) THEN
			RAISE EXCEPTION 'RLS must be enabled';
		END IF;
		IF has_table_privilege('anon', 'public.users', 'SELECT')
		   OR has_table_privilege('authenticated', 'public.shop_memberships', 'INSERT')
		   OR has_table_privilege('authenticated', 'public.users', 'TRUNCATE') THEN
			RAISE EXCEPTION 'Browser roles retain direct privileges';
		END IF;
	END $$;`)
	for _, update := range []string{
		"UPDATE public.users SET active = false",
		"UPDATE public.shop_memberships SET active = false",
		"UPDATE public.shops SET active = false",
	} {
		exec("SAVEPOINT inactive")
		exec(update)
		if _, err := tx.Exec(ctx, provision); err == nil {
			t.Fatal("provisioning must not reactivate suspended access")
		}
		exec("ROLLBACK TO SAVEPOINT inactive")
	}
	// Even if SELECT were granted later, policy-free RLS denies browser reads.
	exec(`GRANT SELECT ON public.users, public.shop_memberships TO authenticated;
		SET LOCAL ROLE authenticated;
		DO $$ BEGIN
			IF EXISTS (SELECT 1 FROM public.users) OR EXISTS (SELECT 1 FROM public.shop_memberships) THEN
				RAISE EXCEPTION 'Browser can read identity data without a policy';
			END IF;
		END $$;
		RESET ROLE;
		DELETE FROM auth.users WHERE id = '20000000-0000-4000-8000-000000000001';
		DO $$ BEGIN
			IF EXISTS (SELECT 1 FROM public.users) OR EXISTS (SELECT 1 FROM public.shop_memberships)
			   OR (SELECT count(*) FROM public.shops) <> 2 THEN
				RAISE EXCEPTION 'Auth deletion cascade affected the wrong records';
			END IF;
		END $$;`)
	migrate(provider, false)
	migrate(replacement, false)
	migrate(accounts, false)
	migrate(roles, false)
	migrate(common, false)
	migrate(common, true)
	migrate(roles, true)
	migrate(provider, true)
	migrate(accounts, true)
	migrate(replacement, true)
}
