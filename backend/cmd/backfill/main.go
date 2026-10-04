// Command backfill imports full COROS history into the database.
//
// Usage:
//
//	go run ./cmd/backfill --from=2024-01-01 [--health-days=365]
//
// Requires a prior login through the web dashboard (tokens are read from
// the database).
package main

import (
	"context"
	"flag"
	"log"
	"time"

	"github.com/dyatmarize/fitness-tracker/backend/internal/config"
	syncpkg "github.com/dyatmarize/fitness-tracker/backend/internal/sync"
	"github.com/dyatmarize/fitness-tracker/backend/internal/coros"
	"github.com/dyatmarize/fitness-tracker/backend/internal/store"
)

func main() {
	fromStr := flag.String("from", "2020-01-01", "start date (YYYY-MM-DD)")
	healthDays := flag.Int("health-days", 365, "trailing days of health data to import (0 to skip)")
	flag.Parse()

	cfg, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}
	from, err := time.ParseInLocation("2006-01-02", *fromStr, time.Local)
	if err != nil {
		log.Fatalf("invalid --from: %v", err)
	}

	ctx := context.Background()
	pool, err := store.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatal(err)
	}
	defer pool.Close()

	client := coros.New(cfg, pool)
	svc := coros.NewService(client, pool)

	loggedIn, err := client.LoggedIn(ctx)
	if err != nil {
		log.Fatal(err)
	}
	if !loggedIn {
		log.Fatal("not logged in: start the server and complete the COROS login at /api/auth/start first")
	}

	if err := syncpkg.Backfill(ctx, svc, pool, from, *healthDays); err != nil {
		log.Fatal(err)
	}
	log.Printf("backfill complete")
}
