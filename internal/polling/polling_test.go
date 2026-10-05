package polling

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"a-gent/internal/agent"
)

type testAdapter struct {
	name  string
	fetch func(context.Context) ([]agent.Session, error)
}

func (adapter testAdapter) Provider() string { return adapter.name }
func (adapter testAdapter) Sessions(ctx context.Context) ([]agent.Session, error) {
	return adapter.fetch(ctx)
}

func receive(t *testing.T, updates <-chan Update) Update {
	t.Helper()
	select {
	case update := <-updates:
		return update
	case <-time.After(2 * time.Second):
		t.Fatal("provider did not publish")
		return Update{}
	}
}

func TestProvidersRunIndependentlyAndStopTogether(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	blocked := make(chan struct{})
	exited := make(chan struct{})
	updates := make(chan Update)
	adapters := []agent.Adapter{
		testAdapter{name: "slow", fetch: func(ctx context.Context) ([]agent.Session, error) {
			close(blocked)
			<-ctx.Done()
			close(exited)
			return nil, ctx.Err()
		}},
		testAdapter{name: "healthy", fetch: func(context.Context) ([]agent.Session, error) { return []agent.Session{{ID: "live"}}, nil }},
	}
	go Run(ctx, adapters, updates)
	update := receive(t, updates)
	if update.Provider != "healthy" || len(update.Sessions) != 1 {
		t.Fatalf("update = %+v", update)
	}
	<-blocked
	cancel()
	select {
	case _, ok := <-updates:
		if ok {
			t.Fatal("published after cancellation")
		}
	case <-time.After(time.Second):
		t.Fatal("workers did not stop")
	}
	select {
	case <-exited:
	default:
		t.Fatal("updates closed before worker exited")
	}
}

func TestPollingTimesOutThenRecoversWithoutOverlapping(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	updates := make(chan Update)
	var calls atomic.Int32
	var active atomic.Int32
	adapter := testAdapter{name: "provider", fetch: func(ctx context.Context) ([]agent.Session, error) {
		if active.Add(1) != 1 {
			t.Error("overlapping provider requests")
		}
		defer active.Add(-1)
		if calls.Add(1) == 1 {
			<-ctx.Done()
			return nil, ctx.Err()
		}
		return []agent.Session{{ID: "recovered"}}, nil
	}}
	done := make(chan struct{})
	go func() {
		defer close(done)
		poll(ctx, adapter, updates, schedule{time.Millisecond, 10 * time.Millisecond, 10 * time.Millisecond})
	}()
	first := receive(t, updates)
	if !errors.Is(first.Err, context.DeadlineExceeded) {
		t.Fatalf("first error = %v", first.Err)
	}
	second := receive(t, updates)
	if second.Err != nil || second.Sessions[0].ID != "recovered" {
		t.Fatalf("recovery = %+v", second)
	}
	cancel()
	<-done
}

func TestSlowConsumerBoundsWorkAndCancellationUnblocksDelivery(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	updates := make(chan Update)
	called := make(chan struct{})
	var calls atomic.Int32
	adapter := testAdapter{name: "provider", fetch: func(context.Context) ([]agent.Session, error) {
		if calls.Add(1) == 1 {
			close(called)
		}
		return nil, nil
	}}
	done := make(chan struct{})
	go func() {
		defer close(done)
		poll(ctx, adapter, updates, schedule{time.Millisecond, time.Second, time.Second})
	}()
	<-called
	time.Sleep(10 * time.Millisecond)
	if calls.Load() != 1 {
		t.Fatal("worker accumulated snapshots while consumer was blocked")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("blocked delivery prevented shutdown")
	}
}

func TestBackoffCapsAndResetsAfterSuccess(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	updates := make(chan Update)
	starts := make(chan time.Time, 10)
	calls := 0
	adapter := testAdapter{name: "provider", fetch: func(context.Context) ([]agent.Session, error) {
		starts <- time.Now()
		calls++
		if calls <= 3 {
			return nil, errors.New("offline")
		}
		return nil, nil
	}}
	done := make(chan struct{})
	go func() {
		defer close(done)
		poll(ctx, adapter, updates, schedule{10 * time.Millisecond, time.Second, 30 * time.Millisecond})
	}()
	previous := time.Time{}
	for index, minimum := range []time.Duration{0, 20 * time.Millisecond, 30 * time.Millisecond, 30 * time.Millisecond, 10 * time.Millisecond} {
		receive(t, updates)
		start := <-starts
		if index > 0 && start.Sub(previous) < minimum {
			t.Fatalf("retry %d ran before its delay", index)
		}
		previous = start
	}
	cancel()
	<-done
}
