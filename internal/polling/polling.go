// Package polling checks providers for session changes without slowing down the UI.
package polling

import (
	"context"
	"sync"
	"time"

	"a-gent/internal/agent"
)

// Update carries one provider's sessions or a polling error. On failure, the UI
// keeps the previous sessions but shows that their status could not be checked.
type Update struct {
	Provider string
	Sessions []agent.Session
	Err      error
}

type schedule struct {
	interval   time.Duration
	timeout    time.Duration
	maxBackoff time.Duration
}

var defaultSchedule = schedule{
	interval:   time.Second,
	timeout:    2 * time.Second,
	maxBackoff: 30 * time.Second,
}

// Run polls each provider in its own goroutine so a slow provider cannot delay
// the others. Providers must have different names and stop requests when asked
// to cancel. The results channel closes after all workers have stopped.
func Run(ctx context.Context, adapters []agent.Adapter, updates chan<- Update) {
	var workers sync.WaitGroup
	for _, adapter := range adapters {
		workers.Add(1)
		go func() {
			defer workers.Done()
			poll(ctx, adapter, updates, defaultSchedule)
		}()
	}
	workers.Wait()
	close(updates)
}

// Waits for each poll to finish before starting another so requests never overlap.
// Repeated failures increase the wait between attempts to avoid constant retries.
func poll(ctx context.Context, adapter agent.Adapter, updates chan<- Update, timing schedule) {
	delay := timing.interval
	for ctx.Err() == nil {
		requestContext, cancel := context.WithTimeout(ctx, timing.timeout)
		sessions, err := adapter.Sessions(requestContext)
		if err == nil {
			err = requestContext.Err()
		}
		cancel()
		if ctx.Err() != nil {
			return
		}

		select {
		case updates <- Update{Provider: adapter.Provider(), Sessions: sessions, Err: err}:
		case <-ctx.Done():
			return
		}

		if err == nil {
			delay = timing.interval
		} else {
			delay = min(delay*2, timing.maxBackoff)
		}

		timer := time.NewTimer(delay)
		select {
		case <-timer.C:
		case <-ctx.Done():
			timer.Stop()
			return
		}
	}
}
