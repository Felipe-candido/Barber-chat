package postgres_test

import (
	"context"
	"errors"
	"reflect"
	"testing"

	db "github.com/Felipe-candido/Barber-chat/internal/database/sqlc"
	"github.com/Felipe-candido/Barber-chat/internal/modules/identity/application"
	identitypostgres "github.com/Felipe-candido/Barber-chat/internal/modules/identity/infra/postgres"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// These tests exercise the generated query and adapter, not SQL filtering.
// PostgreSQL integration tests verify membership isolation and active states.
func TestListActiveShopsByUser(t *testing.T) {
	userID, shopA, shopB := uuid.New(), uuid.New(), uuid.New()
	failure := errors.New("connection lost")
	first := rowStub{values: []any{shopA, "Barbershop A", "barbershop-a"}}
	second := rowStub{values: []any{shopB, "Barbershop B", "barbershop-b"}}
	firstShop := application.AccessibleShopOutput{ShopID: shopA, Name: "Barbershop A", Slug: "barbershop-a"}
	secondShop := application.AccessibleShopOutput{ShopID: shopB, Name: "Barbershop B", Slug: "barbershop-b"}

	tests := []struct {
		name     string
		rows     []rowStub
		queryErr error
		rowsErr  error
		want     []application.AccessibleShopOutput
		wantErr  error
	}{
		{name: "empty", want: []application.AccessibleShopOutput{}},
		{name: "one shop", rows: []rowStub{first}, want: []application.AccessibleShopOutput{firstShop}},
		{name: "multiple shops", rows: []rowStub{first, second}, want: []application.AccessibleShopOutput{firstShop, secondShop}},
		{name: "query failure", queryErr: failure, wantErr: failure},
		{name: "query canceled", queryErr: context.Canceled, wantErr: context.Canceled},
		{name: "query deadline", queryErr: context.DeadlineExceeded, wantErr: context.DeadlineExceeded},
		{name: "scan failure on first row", rows: []rowStub{{err: failure}}, wantErr: failure},
		{name: "scan failure after first row", rows: []rowStub{first, {err: failure}}, wantErr: failure},
		{name: "iteration failure without results", rowsErr: failure, wantErr: failure},
		{name: "iteration failure after first row", rows: []rowStub{first}, rowsErr: failure, wantErr: failure},
		{name: "iteration canceled", rows: []rowStub{first}, rowsErr: context.Canceled, wantErr: context.Canceled},
		{name: "iteration deadline", rows: []rowStub{first}, rowsErr: context.DeadlineExceeded, wantErr: context.DeadlineExceeded},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			rows := &rowsStub{rows: tt.rows, err: tt.rowsErr}
			queryCalls := 0
			store := databaseStub{t: t, query: func(gotCtx context.Context, _ string, args ...any) (pgx.Rows, error) {
				queryCalls++
				if gotCtx != ctx || !reflect.DeepEqual(args, []any{userID}) {
					t.Fatal("accessible shops query must preserve context and the user ID")
				}
				return rows, tt.queryErr
			}}

			got, err := identitypostgres.NewRepository(db.New(store)).ListActiveShopsByUser(ctx, userID)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("ListActiveShopsByUser() error = %v, want %v", err, tt.wantErr)
			}
			if queryCalls != 1 {
				t.Errorf("query calls = %d, want 1", queryCalls)
			}
			if tt.queryErr == nil && !rows.closed {
				t.Error("result set was not closed")
			}
			if tt.wantErr != nil {
				if got != nil {
					t.Fatal("failure must not return a partial shop list")
				}
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("ListActiveShopsByUser() = %+v, want %+v", got, tt.want)
			}
		})
	}
}
