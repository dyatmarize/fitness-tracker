# Fitness Tracker — Progress Log

Last updated: 2026-10-04, ~23:30 (session 1). This is the durable, in-repo record of what this project is, what is done, and exactly how to resume. Keep it updated at the end of every work session.

## What this project is

A self-hosted personal training dashboard. A Go backend acts as an MCP **client** to the official COROS MCP server (the same one used from the AI assistant), syncs workouts + daily health data into PostgreSQL, computes analytics (CTL/ATL/TSB fitness curve, weekly summaries, PRs, ride-vs-ride comparisons), and serves a React web dashboard. Single-user, runs locally via Docker Compose; Tailscale can be added later for phone access away from home.

Approved plan (session 1): `~/.commandcode/plans/coros-workout-tracker.md`

## Architecture

```
[COROS MCP (https://mcpus.coros.com/mcp for this account, OAuth 2.0 + PKCE)]
        ▲
        │ streamable HTTP, JSON-RPC — Go backend is the MCP client
[Go backend: sync worker (hourly + catch-up) → PostgreSQL → REST API]
        ▲
[React web dashboard (nginx, proxies /api)]
```

## Repo layout (as of session 1)

```
fitness-tracker/
├── docs/PROGRESS.md              ← this file
├── backend/
│   ├── cmd/server/main.go        # API + sync worker + OAuth callback, one binary
│   ├── cmd/backfill/main.go      # one-shot history import CLI
│   ├── internal/config/          # env config
│   ├── internal/coros/           # MCP client, OAuth, parsers, sync service
│   │   ├── client.go             # session mgmt, OAuth handler, token persistence
│   │   ├── callback.go           # :8787 /callback receiver for the browser flow
│   │   ├── parse.go              # regex/JSON parsers for every COROS tool
│   │   ├── parse_test.go         # fixtures captured from the REAL account — all passing
│   │   ├── service.go            # SyncActivities / SyncHealth / SyncProfile
│   │   └── models.go
│   ├── internal/store/           # pgx pool + hand-rolled repos + goose migrate
│   ├── internal/api/             # Chi router, handlers
│   ├── internal/sync/            # worker + Backfill()
│   └── migrations/001_init.sql   # 7 tables, applied & verified
├── web/                          # NOT STARTED (milestone 5)
├── deploy/docker-compose.yml    # postgres:17 + backend + web
└── .gitignore
```

## Environment facts

- Repo: `/home/dyatmarize/Documents/repositories/dyatmarize/fitness-tracker` — **not a git repo yet** (init + first commit is a pending step).
- Go module: `github.com/dyatmarize/fitness-tracker/backend`
- Tooling: Go 1.27.1, Node 24.21, Docker 29.8.1, Compose v5.5.1, psql 18.6 client, sqlc installed (unused so far).
- Key deps: `modelcontextprotocol/go-sdk v1.8.0`, `go-chi/chi v5.3.2`, `jackc/pgx v5.11.0`, `pressly/goose v3.28.0`.
- **Ports** (5432 and 5433 are taken by other projects — system PostgreSQL 18 and the `cool-app-postgres` container):
  - Postgres (this project): `127.0.0.1:5440` → container 5432, user/pass/db `fitness`
  - Backend API: `127.0.0.1:8080`
  - OAuth callback: `127.0.0.1:8787` (must be reachable from the user's browser)
  - Web (future): `8090`
- **COROS endpoint gotcha (the current blocker):** the default `https://mcp.coros.com/mcp` 302-redirects to the account's regional server. The SDK's OAuth discovery fails the issuer check on the redirect:

  ```
  metadata issuer "https://mcpus.coros.com" does not match issuer URL "https://mcp.coros.com"
  ```

  Fix: always set `COROS_MCP_URL=https://mcpus.coros.com/mcp` (US region for this account). Compose default and `.env.example` were already updated to this.
- Current runtime state at break: **no server process running, db container stopped** (`docker compose stop db`). The pgdata volume persists — nothing lost.

## Milestones

| # | Milestone | Status |
|---|-----------|--------|
| 1 | Skeleton: Go module, Chi server + `/api/healthz`, goose migrations, Docker Compose | ✅ **DONE** |
| 2 | COROS MCP client: OAuth login, tool calls, parsers | 🔶 **~90% — blocked on live OAuth test** |
| 3 | Sync worker + backfill CLI | 🔶 **Code written, untested against live API** |
| 4 | REST API + analysis module (CTL/ATL/TSB, weekly, PRs, compare) + unit tests | ⬜ PENDING |
| 5 | Web dashboard: Overview, Activities, Detail, Health, Compare (React+Vite+TS+Tailwind+Recharts) | ⬜ PENDING |
| 6 | Polish: README, apply APP_TOKEN auth to routes, full `docker compose up` E2E, `git init` + first commit | ⬜ PENDING |
| 7 | Phase 2: FIT file ingestion (lazy, 50 files/24h cap; HR curves, zones), Android app | ⬜ FUTURE |

## Detailed status

### Milestone 1 — DONE (verified)
- Server builds (`go build ./...`, `go vet` clean), migrations apply via embedded goose, `/api/healthz` returns 200 with a DB ping, all 7 tables created: `activities`, `activity_laps`, `daily_health`, `fitness_snapshots`, `sync_state`, `oauth_tokens`, `profile`.
- Compose stack works; db healthcheck green.

### Milestone 2 — IN PROGRESS (one live test remaining)
Done:
- `client.go`: persistent MCP session over `StreamableClientTransport` (SSE standalone stream disabled — stateless server), auto-reconnect with one retry on call failure, OAuth via SDK `auth.AuthorizationCodeHandler` with **dynamic client registration** (public client, PKCE, `token_endpoint_auth_method: none`, scopes incl. `offline_access`), tokens + client registration persisted in the `oauth_tokens` table, auto-refresh wrapped in a saving token source that re-persists on rotation.
- `callback.go`: local HTTP receiver on `:8787/callback` that hands the browser's code to the pending flow and shows a "COROS connected" page.
- `parse.go` + `parse_test.go`: parsers for **every** tool we sync, written against real outputs captured from the actual account during session 1. `go test ./...` is green. Parsers: `ParseSportRecords`, `ParseActivityDetail`, `ParseLaps` (JSON), `ParseDailyHealth`, `ParseSleepOverview`, `ParseSleepHrv`, `ParseDatedValues` (RHR/stress), `ParseRecovery`, `ParseTrainingLoad`, `ParseFitnessOverview`, `ParseProfile`, plus a duration parser handling `1:49:36`, `36:46`, `1h 50min`, `49h` etc.
- Login UX: `GET /api/auth/start` returns `{auth_url}` to open in a browser; `GET /api/meta/status` reports `logged_in` + sync state.

Blocked on:
- **Live OAuth test with the regional URL.** The first attempt (default URL) failed with the issuer-mismatch error above. The retry with `COROS_MCP_URL=https://mcpus.coros.com/mcp` never ran (session ended). This is the single next action.

### Milestone 3 — code written, needs live verification
- `internal/sync/worker.go`: hourly ticker (configurable `SYNC_INTERVAL`), immediate sync on start, catch-up from `sync_state.last_activity_sync` minus a 7-day overlap (catches edits), atomic running flag, status for `/api/meta/status`, manual trigger via `POST /api/sync`.
- `cmd/backfill`: `go run ./cmd/backfill --from=2020-01-01 --health-days=365` (requires prior login).
- `service.go` sync logic: activities in 29-day chunks (limit 200/page), each activity enriched via `getActivityDetail` + `queryActivityLapData` (both non-fatal if they fail), health = 30 trailing days (HRV/load capped at 7 by the API), plus daily fitness snapshot and profile.

## How to resume (exact next steps)

1. Start the database:
   ```bash
   cd ~/Documents/repositories/dyatmarize/fitness-tracker/deploy && docker compose up -d db
   ```
2. Build and run the server with the regional URL:
   ```bash
   cd ~/Documents/repositories/dyatmarize/fitness-tracker/backend
   go build -o /tmp/ft-server ./cmd/server
   DATABASE_URL='postgres://fitness:fitness@localhost:5440/fitness?sslmode=disable' \
   COROS_MCP_URL='https://mcpus.coros.com/mcp' /tmp/ft-server
   ```
3. **Complete the first login** (the milestone-2 finish line):
   ```bash
   curl -s http://localhost:8080/api/auth/start
   ```
   Open the returned `auth_url` in a browser, log into COROS, expect the "COROS connected" page on `localhost:8787/callback`.
4. Verify tokens landed:
   ```bash
   psql 'postgres://fitness:fitness@localhost:5440/fitness' -c 'SELECT client_id, expires_at FROM oauth_tokens;'
   ```
5. Trigger a sync and watch the logs:
   ```bash
   curl -X POST http://localhost:8080/api/sync
   # then check:
   psql 'postgres://fitness:fitness@localhost:5440/fitness' -c 'SELECT count(*) FROM activities;' -c 'SELECT count(*) FROM daily_health;'
   ```
6. Then run the full backfill (`--from=2020-01-01`) and confirm the fixtures below match → milestones 2 and 3 are then DONE. Continue to milestone 4 (REST API + analysis).

## Verification fixtures (captured from the real account, session 1)

Use these to prove the pipeline is correct end-to-end:

- Ride 2026-10-04: LabelId `480786453874376808`, sport 200 — 42.31 km, 1:49:36, avg 23.2 km/h, max 43.5, avg HR 148, 1728 kcal, elev 182/184 m, load 249, ATE 4.1, AnTE 3.3, focus Threshold, perf -1, location "Depok Road Bike", start -6.406, 106.794. Laps: 9×500 m groups + 43×100 m + 1 km + total (JSON).
- Ride 2026-09-27: LabelId `480623074922758445` — 64.33 km, 2:46:45, avg 23.2 km/h, avg HR 153, 2728 kcal, load 443, ATE 5.0, AnTE 4.6.
- Health 2026-10-04: sleep score 74, daily sleep 6h49m (main 3h33m, nap 3h16m), RHR 51, HRV avg 97 ms (range 76–100, baseline 88), recovery 62% / "Light training recommended" / 49 h, load short 69 / long 61 / ratio 1.13 "Optimized".
- Fitness: VO2max 35, running level 45, threshold pace 7:35 /km.
- Profile: 178 cm, 115.1 kg, born 1999-05-29, male, "dyatmarize".

## Gotchas & decisions (learned the hard way)

- `mcp.coros.com` regional redirect breaks SDK OAuth issuer validation → use `mcpus.coros.com/mcp` directly (see above).
- The `querySportRecords` MCP tool **requires all filter arguments** even when unused (zeros / empty strings) — see `service.go`.
- `queryActivityLapData` returns JSON; every other tool returns formatted text → text parsers with regexes, fixtures in `parse_test.go`. If COROS changes wording, tests will catch it.
- Sleep/HRV dates are **wake-up days** (night of D → recorded under D+1).
- Sport codes: 65535 = all, cycling = 200, running = 100/101/102/103.
- Store layer is hand-rolled pgx SQL (store/repo.go), not sqlc — simpler for this size; sqlc is installed if the query set grows.
- `requireToken` middleware exists in `internal/api` but is **not yet applied** to routes — everything is currently unauthenticated on localhost. Apply in milestone 6.
- Do NOT seed the app with the live `~/.commandcode/mcp-tokens.json` refresh token — COROS rotates refresh tokens, so the app and the AI assistant would race each other. The app has its own OAuth login for this reason.
- FIT downloads are capped at 50 files/24h — that's why FIT ingestion is deferred to phase 2 with lazy fetching.
- `pkill -f ft-server` matches your own shell command and kills it (exit 143) — use `pkill -f '^/tmp/ft-server'` or kill the PID.
- Ubuntu box also runs a system PostgreSQL 18 (port 5432) and another project's `cool-app-postgres` (5433) — hence port 5440 for this stack.
