package jobs_test

import (
	"context"
	"errors"
	"github.com/Felipe-candido/Barber-chat/internal/platform/jobs"
	"io"
	"log/slog"
	"testing"
	"time"
)

func TestPeriodicRecoversFailureAndCancelsInFlight(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	calls := make(chan int, 4)
	go func() {
		defer close(done)
		n := 0
		jobs.Run(ctx, time.Millisecond, 100*time.Millisecond, func(ctx context.Context) (int, error) {
			n++
			calls <- n
			if n == 1 {
				return 0, errors.New("temporary failure")
			}
			<-ctx.Done()
			return 0, ctx.Err()
		}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	}()
	for _, want := range []int{1, 2} {
		select {
		case n := <-calls:
			if n != want {
				t.Fatal(n)
			}
		case <-time.After(time.Second):
			t.Fatal("job did not recover")
		}
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("job did not drain on cancellation")
	}
}
