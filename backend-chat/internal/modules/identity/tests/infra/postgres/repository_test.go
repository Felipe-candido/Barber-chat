package postgres_test

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"

	db "github.com/Felipe-candido/Barber-chat/internal/database/sqlc"
	"github.com/Felipe-candido/Barber-chat/internal/modules/identity/domain"
	identitypostgres "github.com/Felipe-candido/Barber-chat/internal/modules/identity/infra/postgres"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
)

// Exercise the real generated queries and adapter without changing production
// interfaces. These doubles verify mappings and errors, not PostgreSQL semantics.
type databaseStub struct {
	t        *testing.T
	queryRow func(context.Context, string, ...any) pgx.Row
	query    func(context.Context, string, ...any) (pgx.Rows, error)
}

func (s databaseStub) Exec(context.Context, string, ...any) (pgconn.CommandTag, error) {
	s.t.Fatal("the identity read repository must not write")
	return pgconn.CommandTag{}, nil
}

func (s databaseStub) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	s.t.Helper()
	if s.queryRow == nil {
		s.t.Fatal("unexpected QueryRow call")
	}
	return s.queryRow(ctx, sql, args...)
}

func (s databaseStub) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	s.t.Helper()
	if s.query == nil {
		s.t.Fatal("unexpected Query call")
	}
	return s.query(ctx, sql, args...)
}

type rowStub struct {
	values []any
	err    error
}

func (r rowStub) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}
	if len(dest) != len(r.values) {
		return errors.New("unexpected column count")
	}
	for i, value := range r.values {
		switch d := dest[i].(type) {
		case *uuid.UUID:
			*d = value.(uuid.UUID)
		case *string:
			*d = value.(string)
		case *bool:
			*d = value.(bool)
		default:
			return fmt.Errorf("unexpected scan destination %T", d)
		}
	}
	return nil
}

type rowsStub struct {
	rows   []rowStub
	index  int
	err    error
	closed bool
}

func (r *rowsStub) Close()                                       { r.closed = true }
func (r *rowsStub) Err() error                                   { return r.err }
func (r *rowsStub) CommandTag() pgconn.CommandTag                { return pgconn.CommandTag{} }
func (r *rowsStub) FieldDescriptions() []pgconn.FieldDescription { return nil }
func (r *rowsStub) RawValues() [][]byte                          { return nil }
func (r *rowsStub) Conn() *pgx.Conn                              { return nil }
func (r *rowsStub) TypeMap() *pgtype.Map                         { return nil }
func (r *rowsStub) Values() ([]any, error)                       { return r.rows[r.index-1].values, nil }
func (r *rowsStub) Scan(dest ...any) error                       { return r.rows[r.index-1].Scan(dest...) }
func (r *rowsStub) Next() bool {
	if r.closed || r.index == len(r.rows) {
		r.Close()
		return false
	}
	r.index++
	return true
}

func TestFindUserByID(t *testing.T) {
	id := uuid.New()
	failure := errors.New("connection lost")
	for _, tt := range []struct {
		name    string
		row     rowStub
		found   bool
		want    domain.User
		wantErr error
	}{
		{"active", rowStub{values: []any{id, "João", true}}, true, domain.User{ID: id, DisplayName: "João", Active: true}, nil},
		{"inactive", rowStub{values: []any{id, "João", false}}, true, domain.User{ID: id, DisplayName: "João", Active: false}, nil},
		{"missing", rowStub{err: pgx.ErrNoRows}, false, domain.User{}, nil},
		{"wrapped missing", rowStub{err: fmt.Errorf("scan: %w", pgx.ErrNoRows)}, false, domain.User{}, nil},
		{"database failure", rowStub{err: failure}, false, domain.User{}, failure},
		{"canceled", rowStub{err: context.Canceled}, false, domain.User{}, context.Canceled},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			store := databaseStub{t: t, queryRow: func(gotCtx context.Context, _ string, args ...any) pgx.Row {
				if gotCtx != ctx || !reflect.DeepEqual(args, []any{id}) {
					t.Fatal("user lookup must preserve context and user ID")
				}
				return tt.row
			}}
			got, found, err := identitypostgres.NewRepository(db.New(store)).FindUserByID(ctx, id)
			if !errors.Is(err, tt.wantErr) || found != tt.found || got != tt.want {
				t.Fatalf("FindUserByID() = %+v, %v, %v; want %+v, %v, %v", got, found, err, tt.want, tt.found, tt.wantErr)
			}
		})
	}
}

func TestFindMembership(t *testing.T) {
	userID, shopID := uuid.New(), uuid.New()
	failure := errors.New("scan failed")
	for _, tt := range []struct {
		name    string
		row     rowStub
		found   bool
		want    domain.Membership
		wantErr error
	}{
		{"active", rowStub{values: []any{userID, shopID, true}}, true, domain.Membership{UserID: userID, ShopID: shopID, Active: true}, nil},
		{"inactive", rowStub{values: []any{userID, shopID, false}}, true, domain.Membership{UserID: userID, ShopID: shopID, Active: false}, nil},
		{"missing", rowStub{err: pgx.ErrNoRows}, false, domain.Membership{}, nil},
		{"wrapped missing", rowStub{err: fmt.Errorf("scan: %w", pgx.ErrNoRows)}, false, domain.Membership{}, nil},
		{"database failure", rowStub{err: failure}, false, domain.Membership{}, failure},
		{"deadline", rowStub{err: context.DeadlineExceeded}, false, domain.Membership{}, context.DeadlineExceeded},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			store := databaseStub{t: t, queryRow: func(gotCtx context.Context, _ string, args ...any) pgx.Row {
				if gotCtx != ctx || !reflect.DeepEqual(args, []any{userID, shopID}) {
					t.Fatal("membership lookup must preserve context and the user/shop pair in order")
				}
				return tt.row
			}}
			got, found, err := identitypostgres.NewRepository(db.New(store)).FindMembership(ctx, userID, shopID)
			if !errors.Is(err, tt.wantErr) || found != tt.found || got != tt.want {
				t.Fatalf("FindMembership() = %+v, %v, %v; want %+v, %v, %v", got, found, err, tt.want, tt.found, tt.wantErr)
			}
		})
	}
}

func TestListMembershipsByUser(t *testing.T) {
	userID, shopA, shopB := uuid.New(), uuid.New(), uuid.New()
	failure := errors.New("connection lost")
	first := rowStub{values: []any{userID, shopA, true}}
	second := rowStub{values: []any{userID, shopB, false}}
	for _, tt := range []struct {
		name     string
		rows     []rowStub
		queryErr error
		rowsErr  error
		wantErr  error
	}{
		{name: "empty"},
		{name: "active and inactive", rows: []rowStub{first, second}},
		{name: "query failure", queryErr: failure, wantErr: failure},
		{name: "canceled", queryErr: context.Canceled, wantErr: context.Canceled},
		{name: "scan failure after first row", rows: []rowStub{first, {err: failure}}, wantErr: failure},
		{name: "iteration failure after first row", rows: []rowStub{first}, rowsErr: failure, wantErr: failure},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			rows := &rowsStub{rows: tt.rows, err: tt.rowsErr}
			store := databaseStub{t: t, query: func(gotCtx context.Context, _ string, args ...any) (pgx.Rows, error) {
				if gotCtx != ctx || !reflect.DeepEqual(args, []any{userID}) {
					t.Fatal("list must preserve context and user ID")
				}
				return rows, tt.queryErr
			}}
			got, err := identitypostgres.NewRepository(db.New(store)).ListMembershipsByUser(ctx, userID)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("ListMembershipsByUser() error = %v, want %v", err, tt.wantErr)
			}
			if tt.queryErr == nil && !rows.closed {
				t.Error("result set was not closed")
			}
			if tt.wantErr != nil {
				if got != nil {
					t.Fatal("failure must not return a partial result")
				}
				return
			}
			want := []domain.Membership{}
			if len(tt.rows) > 0 {
				want = []domain.Membership{{UserID: userID, ShopID: shopA, Active: true}, {UserID: userID, ShopID: shopB, Active: false}}
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("ListMembershipsByUser() = %+v, want %+v", got, want)
			}
		})
	}
}
