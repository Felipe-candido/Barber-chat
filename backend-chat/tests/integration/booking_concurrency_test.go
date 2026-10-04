//go:build integration

package integration

import (
	"errors"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestBookingConcurrentSameSlot(t *testing.T) {
	f := newBookingFixture(t)
	blocker, err := f.pool.Begin(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer blocker.Rollback(f.ctx)
	if _, err := blocker.Exec(f.ctx, "SELECT id FROM professionals WHERE shop_id=$1 AND id=$2 FOR UPDATE", f.shop, f.professional); err != nil {
		t.Fatal(err)
	}
	// Both clients observed the same free slot. Release independent HTTP calls
	// together; the pool provides separate PostgreSQL transactions/connections.
	start := make(chan struct{})
	responses := make(chan *httptest.ResponseRecorder, 2)
	var ready sync.WaitGroup
	ready.Add(2)
	for range 2 {
		go func() {
			key := bookingKey()
			ready.Done()
			<-start
			responses <- f.request("POST", "/appointments", f.body(f.start, f.service), key)
		}()
	}
	ready.Wait()
	close(start)
	// Do not merely hope goroutines overlap: verify two distinct server sessions
	// are blocked on the held agenda lock before allowing either to proceed.
	waitForBookingWaiters(t, f, 2)
	if err := blocker.Rollback(f.ctx); err != nil {
		t.Fatal(err)
	}
	counts := map[int]int{}
	for range 2 {
		w := <-responses
		counts[w.Code]++
		if w.Code != 201 && w.Code != 409 {
			t.Fatalf("unexpected response: %d %s", w.Code, w.Body.String())
		}
	}
	if counts[201] != 1 || counts[409] != 1 {
		t.Fatalf("race did not have exactly one winner: %v", counts)
	}
	var appointments, customers, requests int
	if err := f.pool.QueryRow(f.ctx, `SELECT (SELECT count(*) FROM appointments WHERE shop_id=$1),
        (SELECT count(*) FROM customers WHERE shop_id=$1),(SELECT count(*) FROM booking_requests WHERE shop_id=$1)`, f.shop).Scan(&appointments, &customers, &requests); err != nil {
		t.Fatal(err)
	}
	if appointments != 1 || customers != 1 || requests != 1 {
		t.Fatalf("loser partially committed: appointments=%d customers=%d requests=%d", appointments, customers, requests)
	}
}

func waitForBookingWaiters(t *testing.T, f *bookingFixture, want int) {
	t.Helper()
	deadline := time.NewTimer(2 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		var waiting int
		err := f.pool.QueryRow(f.ctx, "SELECT count(*) FROM pg_stat_activity WHERE datname=current_database() AND wait_event_type='Lock' AND query LIKE '%LockProfessionalCalendar%' AND pid<>pg_backend_pid()").Scan(&waiting)
		if err != nil {
			t.Fatal(err)
		}
		if waiting >= want {
			return
		}
		select {
		case <-deadline.C:
			t.Fatalf("did not observe %d independent blocked database sessions", want)
		case <-ticker.C:
		}
	}
}

func TestBookingConcurrentSameRequestKey(t *testing.T) {
	f := newBookingFixture(t)
	key := bookingKey()
	start := make(chan struct{})
	responses := make(chan *httptest.ResponseRecorder, 2)
	for range 2 {
		go func() { <-start; responses <- f.request("POST", "/appointments", f.body(f.start, f.service), key) }()
	}
	close(start)
	one, two := <-responses, <-responses
	if one.Code != 201 {
		one, two = two, one
	}
	created := bookingResult(t, one, 201)
	replayed := bookingResult(t, two, 200)
	if created.Code != replayed.Code || created.Appointment.ID != replayed.Appointment.ID || !replayed.Replayed {
		t.Fatal("concurrent retry created a different booking or code")
	}
}

func TestBookingConcurrentCalendarBlock(t *testing.T) {
	f := newBookingFixture(t)
	start := make(chan struct{})
	bookingDone := make(chan *httptest.ResponseRecorder, 1)
	blockDone := make(chan error, 1)
	go func() {
		<-start
		bookingDone <- f.request("POST", "/appointments", f.body(f.start, f.service), bookingKey())
	}()
	go func() {
		<-start
		_, err := f.pool.Exec(f.ctx, "INSERT INTO schedule_blocks (id,shop_id,professional_id,starts_at,ends_at) VALUES ($1,$2,$3,$4,$5)", uuid.New(), f.shop, f.professional, f.start, f.start.Add(time.Hour))
		blockDone <- err
	}()
	close(start)
	bookingResult, blockErr := <-bookingDone, <-blockDone
	if bookingResult.Code == 201 {
		var constraint *pgconn.PgError
		if !errors.As(blockErr, &constraint) || constraint.Code != "23P01" {
			t.Fatalf("block ignored committed appointment: %v", blockErr)
		}
	} else if bookingResult.Code == 409 {
		if blockErr != nil {
			t.Fatalf("both operations failed: %v", blockErr)
		}
	} else {
		t.Fatalf("unexpected booking response: %d %s", bookingResult.Code, bookingResult.Body.String())
	}
}
