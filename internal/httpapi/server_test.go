package httpapi

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func testLogger() *slog.Logger { return slog.New(slog.NewJSONHandler(io.Discard, nil)) }

func TestHealthDoesNotDependOnDatabase(t *testing.T) {
	h := NewHandler(func(context.Context) error { t.Fatal("liveness must not query database"); return nil }, time.Second, testLogger())
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/health", nil))
	if w.Code != 200 || w.Body.String() != "{\"status\":\"ok\"}\n" {
		t.Fatalf("unexpected health: %d %s", w.Code, w.Body)
	}
	if w.Header().Get("Content-Type") != "application/json" {
		t.Fatal("missing JSON content type")
	}
}

func TestReadiness(t *testing.T) {
	for _, available := range []bool{true, false} {
		h := NewHandler(func(ctx context.Context) error {
			if _, ok := ctx.Deadline(); !ok {
				t.Error("missing readiness deadline")
			}
			if !available {
				return errors.New("postgres://secret@host")
			}
			return nil
		}, time.Second, testLogger())
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/ready", nil))
		want := 200
		if !available {
			want = 503
		}
		if w.Code != want {
			t.Fatalf("got %d, want %d", w.Code, want)
		}
		if !available && w.Body.String() != "{\"status\":\"unavailable\"}\n" {
			t.Fatal("unexpected failure response")
		}
	}
}

func TestRoutes(t *testing.T) {
	h := NewHandler(func(context.Context) error { return nil }, time.Second, testLogger())
	for _, test := range []struct {
		method, path string
		status       int
	}{
		{"POST", "/health", 405}, {"GET", "/missing", 404},
	} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(test.method, test.path, nil))
		if w.Code != test.status {
			t.Fatalf("%s %s: got %d", test.method, test.path, w.Code)
		}
	}
}

func TestGracefulShutdownDrainsRequest(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	entered, release := make(chan struct{}), make(chan struct{})
	defer close(release)
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(entered)
		<-release
		w.WriteHeader(http.StatusNoContent)
	})
	done := make(chan error, 1)
	go func() { done <- Serve(ctx, listener, h, 2*time.Second, testLogger()) }()
	response := make(chan error, 1)
	go func() {
		client := &http.Client{Timeout: 3 * time.Second}
		res, err := client.Get("http://" + listener.Addr().String())
		if err == nil {
			defer res.Body.Close()
			if res.StatusCode != 204 {
				err = errors.New("request not drained")
			}
		}
		response <- err
	}()
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("request did not start")
	}
	cancel()
	select {
	case err := <-done:
		t.Fatalf("server stopped before draining: %v", err)
	case <-time.After(30 * time.Millisecond):
	}
	release <- struct{}{}
	if err := <-response; err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("shutdown hung")
	}
}
