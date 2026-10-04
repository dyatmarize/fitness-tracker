// Package sync runs the periodic COROS sync worker.
package sync

import (
	"context"
	"log"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/dyatmarize/fitness-tracker/backend/internal/config"
	"github.com/dyatmarize/fitness-tracker/backend/internal/coros"
	"github.com/dyatmarize/fitness-tracker/backend/internal/store"
)

// Worker periodically syncs activities and health data.
type Worker struct {
	cfg    *config.Config
	pool   *pgxpool.Pool
	client *coros.Client
	svc    *coros.Service

	running     atomic.Bool
	lastRunUnix atomic.Int64
	lastErr     atomic.Value // string
}

// NewWorker creates a sync worker.
func NewWorker(cfg *config.Config, pool *pgxpool.Pool, client *coros.Client) *Worker {
	return &Worker{cfg: cfg, pool: pool, client: client, svc: coros.NewService(client, pool)}
}

// Run blocks until ctx ends, syncing immediately and then every interval.
func (w *Worker) Run(ctx context.Context) {
	w.Sync(ctx)
	ticker := time.NewTicker(w.cfg.SyncInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.Sync(ctx)
		}
	}
}

// Sync performs one incremental pass. Safe to call concurrently (no-ops if
// another pass is already running); returns whether a pass started.
func (w *Worker) Sync(ctx context.Context) bool {
	if !w.running.CompareAndSwap(false, true) {
		return false
	}
	go func() {
		defer w.running.Store(false)
		passCtx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()
		if err := w.pass(passCtx); err != nil {
			w.lastErr.Store(err.Error())
			log.Printf("[sync] error: %v", err)
		} else {
			w.lastErr.Store("")
		}
		w.lastRunUnix.Store(time.Now().Unix())
	}()
	return true
}

// pass is one full incremental sync.
func (w *Worker) pass(ctx context.Context) error {
	loggedIn, err := w.client.LoggedIn(ctx)
	if err != nil {
		return err
	}
	if !loggedIn {
		return coros.ErrNotLoggedIn
	}

	state, err := store.GetSyncState(ctx, w.pool)
	if err != nil {
		return err
	}

	// Activities: catch up from the last sync minus a 7-day overlap.
	from := time.Now().In(time.Local).AddDate(0, 0, -7)
	if state.LastActivitySync != nil {
		if t := state.LastActivitySync.In(time.Local).AddDate(0, 0, -7); t.Before(from) {
			from = t
		}
	}
	to := time.Now().In(time.Local)

	n, err := w.svc.SyncActivities(ctx, from, to)
	if err != nil {
		return err
	}
	if err := store.SetSyncState(ctx, w.pool, &to, nil); err != nil {
		return err
	}
	log.Printf("[sync] activities: %d upserted", n)

	// Health: trailing 30 days.
	if err := w.svc.SyncHealth(ctx, 30); err != nil {
		return err
	}
	now := time.Now()
	if err := store.SetSyncState(ctx, w.pool, nil, &now); err != nil {
		return err
	}
	log.Printf("[sync] health: updated")
	return nil
}

// Status reports the worker state for the API.
type Status struct {
	Running bool   `json:"running"`
	LastRun string `json:"last_run,omitempty"`
	LastError string `json:"last_error,omitempty"`
}

// CurrentStatus snapshots the worker state.
func (w *Worker) CurrentStatus() Status {
	s := Status{Running: w.running.Load()}
	if v := w.lastRunUnix.Load(); v != 0 {
		s.LastRun = time.Unix(v, 0).Format(time.RFC3339)
	}
	if v, ok := w.lastErr.Load().(string); ok {
		s.LastError = v
	}
	return s
}

// Backfill imports full history from the given date and exits.
func Backfill(ctx context.Context, svc *coros.Service, pool *pgxpool.Pool, from time.Time, healthDays int) error {
	to := time.Now().In(time.Local)
	n, err := svc.SyncActivities(ctx, from, to)
	if err != nil {
		return err
	}
	if err := store.SetSyncState(ctx, pool, &to, nil); err != nil {
		return err
	}
	log.Printf("[backfill] activities: %d upserted", n)

	if healthDays > 0 {
		if err := svc.SyncHealth(ctx, healthDays); err != nil {
			return err
		}
		now := time.Now()
		if err := store.SetSyncState(ctx, pool, nil, &now); err != nil {
			return err
		}
		log.Printf("[backfill] health: updated")
	}
	return nil
}
