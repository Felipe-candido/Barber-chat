//go:build integration

package integration

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestSelfServicePublicBookingRequiresPaymentAndPreservesReplay(t *testing.T) {
	f := newBookingFixture(t)
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := f.pool.Exec(f.ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec("UPDATE shops SET provisioning_source='self_service' WHERE id=$1", f.shop)
	key := bookingKey()
	body := f.body(f.start, f.service)
	for _, published := range []bool{false, true} {
		exec("UPDATE shops SET publication_requested=$1 WHERE id=$2", published, f.shop)
		w := f.request(http.MethodPost, "/appointments", body, key)
		if w.Code != 404 {
			t.Fatalf("unpaid public write: %d %s", w.Code, w.Body.String())
		}
		w = f.request(http.MethodGet, "/professionals?service_id="+f.service.String(), nil, "")
		if w.Code != 404 {
			t.Fatalf("unpaid public read: %d %s", w.Code, w.Body.String())
		}
	}
	exec("INSERT INTO billing_accounts(shop_id,stripe_customer_id) VALUES ($1,$2)", f.shop, "cus_"+f.shop.String())
	now := time.Now().UTC()
	exec(`INSERT INTO billing_subscriptions(id,shop_id,stripe_customer_id,stripe_subscription_id,plan_key,status,paid_from,paid_until)
		VALUES ($1,$2,$3,$4,'monthly','active',$5,$6)`, uuid.New(), f.shop, "cus_"+f.shop.String(), "sub_"+f.shop.String(), now.Add(-time.Hour), now.Add(time.Hour))
	w := f.request(http.MethodPost, "/appointments", body, key)
	if w.Code != 201 {
		t.Fatalf("paid public write: %d %s", w.Code, w.Body.String())
	}
	var receipt struct {
		Code     string `json:"code"`
		Replayed bool   `json:"replayed"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &receipt); err != nil || receipt.Code == "" || receipt.Replayed {
		t.Fatal("invalid first receipt", err)
	}
	exec("UPDATE billing_subscriptions SET status='suspended' WHERE shop_id=$1", f.shop)
	exec("UPDATE shops SET publication_requested=false WHERE id=$1", f.shop)
	w = f.request(http.MethodPost, "/appointments", f.body(f.start.Add(time.Hour), f.service), bookingKey())
	if w.Code != 404 {
		t.Fatalf("suspended new write: %d %s", w.Code, w.Body.String())
	}
	w = f.request(http.MethodPost, "/appointments", body, key)
	var replay struct {
		Code     string `json:"code"`
		Replayed bool   `json:"replayed"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &replay); err != nil || w.Code != 200 || !replay.Replayed || replay.Code != receipt.Code {
		t.Fatalf("saved receipt unavailable after suspension: %d %s", w.Code, w.Body.String())
	}
	for _, route := range []string{"/appointments/lookup", "/appointments/cancel"} {
		w := f.request(http.MethodPost, route, map[string]string{"code": receipt.Code}, "")
		if w.Code != 200 {
			t.Fatalf("existing reservation blocked: %s %d %s", route, w.Code, w.Body.String())
		}
	}
}
