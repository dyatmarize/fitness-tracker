# Fitness Tracker — Approved Plan (Session 1, 2026-10-04)

This is the plan approved before implementation started, preserved in-repo for reference.
For current status and resume instructions see [PROGRESS.md](./PROGRESS.md).
Note: two details were superseded during implementation — the MCP endpoint for this
account is `https://mcpus.coros.com/mcp` (regional redirect breaks OAuth issuer
validation), and the store layer is hand-rolled pgx SQL rather than sqlc-generated.

## Goal
Personal training dashboard: Go backend syncs workouts + daily health data from the official COROS MCP (`https://mcp.coros.com/mcp`) into PostgreSQL, computes full analytics (CTL/ATL/TSB, weekly summaries, PRs, week-over-week comparisons), and serves a React web dashboard. Runs locally via Docker Compose (Tailscale later for phone access anywhere).

## Project location
`/home/dyatmarize/Documents/repositories/dyatmarize/fitness-tracker` — repo already prepared (currently only an empty `docs/`; keep it for project docs/ADR notes).

## Architecture

```
[COROS MCP (mcp.coros.com/mcp, OAuth)]
        ▲
        │ streamable HTTP, JSON-RPC (Go backend = MCP client)
[Go backend: sync worker (cron + catch-up) → PostgreSQL → REST API]
        ▲
[React web dashboard (nginx, proxies /api)]
```

Monorepo layout inside the existing repo:

```
fitness-tracker/
├── docs/                          # existing dir — project docs, decisions
├── backend/
│   ├── cmd/server/main.go        # API + sync worker, single binary
│   ├── cmd/backfill/main.go      # one-shot historical import CLI
│   ├── internal/
│   │   ├── coros/                # MCP client + OAuth token management
│   │   ├── sync/                 # scheduled + catch-up sync orchestrator
│   │   ├── store/                # pgx repositories (sqlc-generated)
│   │   ├── api/                  # Chi handlers + middleware
│   │   ├── analysis/             # CTL/ATL/TSB, weekly, PRs, compare
│   │   └── config/
│   ├── migrations/               # goose SQL migrations
│   ├── Dockerfile
│   └── go.mod
├── web/                           # Vite + React + TS + Tailwind + Recharts
├── deploy/docker-compose.yml     # postgres:17 + backend + web
└── README.md
```

## COROS integration (backend/internal/coros)

- **Transport**: official Go SDK `github.com/modelcontextprotocol/go-sdk` (streamable HTTP client). Fallback: `mark3labs/mcp-go`.
- **Auth**: one-time OAuth flow (browser login, PKCE) → refresh token stored in Postgres `oauth_tokens` table. COROS MCP is stateless HTTP (per official FAQ), so a long-running Go client is fine.
  - **Fallback if dynamic client registration fails**: seed tokens once from `~/.commandcode/mcp-tokens.json` (copy the refresh token into the app's own store — do NOT share the live file, to avoid refresh-rotation races with Command Code).
- **Tools called**:
  - `querySportRecords` (list by date range), `getActivityDetail`, `queryActivityLapData`
  - `queryDailyHealthData`, `querySleepData`, `querySleepHrv`, `queryRestingHeartRate`, `queryStressLevel`, `queryRecoveryStatus`
  - `queryTrainingLoadAssessment`, `queryFitnessAssessmentOverview` (VO2max, race predictions)
  - `queryUserInfo`, `queryDevices`
- Store every raw response in JSONB columns so schema changes on COROS's side never lose data.

## Sync strategy (set-and-forget)

- On startup: catch-up from `sync_state.last_activity_sync` minus a 7-day overlap window (catches edits).
- Every hour (configurable `SYNC_INTERVAL`): incremental activity sync + trailing 30 days of health data.
- `POST /api/sync` for manual trigger. Backfill CLI: `go run ./cmd/backfill --from=2020-01-01`.
- Upserts keyed by `label_id` (activities) and `(date, metric)` (health).

## Database schema (migrations/001_init.sql)

```sql
activities (
  label_id TEXT PRIMARY KEY, sport_type INT, start_time TIMESTAMPTZ NOT NULL,
  end_time TIMESTAMPTZ, duration_s INT, distance_m DOUBLE PRECISION,
  avg_speed_kmh / max_speed_kmh / avg_hr / max_hr / calories / elevation_gain_m / elevation_loss_m,
  training_load / aerobic_te / anaerobic_te DOUBLE PRECISION,
  training_focus TEXT, performance INT, location TEXT,
  start_lat / start_lon DOUBLE PRECISION, raw_detail JSONB, synced_at TIMESTAMPTZ
);
activity_laps (id BIGSERIAL PK, label_id FK, lap_index INT, split JSONB);
daily_health (date DATE, metric TEXT, value DOUBLE PRECISION, raw JSONB, PK(date, metric));
fitness_snapshots (date DATE PK, vo2max, race predictions…, raw JSONB);
sync_state (last_activity_sync, last_health_sync, updated_at);
oauth_tokens (…);
```

## Analysis module (internal/analysis — pure functions, unit-tested)

- **CTL/ATL/TSB**: exponentially weighted daily load — CTL τ=42d, ATL τ=7d, TSB = CTL−ATL. Zero-fill rest days.
- Weekly summaries (distance/time/load/calories per ISO week) + week-over-week deltas.
- PRs (fastest avg speed per distance band), auto "previous similar ride" comparison (same sport + nearest distance) producing the delta table like today's ride vs Sep 27.
- HR zone distribution: deferred to FIT phase (needs per-second samples).

## REST API (Chi, `internal/api`)

```
GET  /api/activities?from&to&sport&limit      GET /api/activities/{labelId}
GET  /api/activities/{labelId}/laps           POST /api/activities/compare {a,b}
GET  /api/health/daily?from&to&metrics        GET /api/stats/weekly?weeks=12
GET  /api/stats/fitness?days=180              GET /api/stats/prs?sport=
GET  /api/fitness/overview                    POST /api/sync
GET  /api/meta/status                         GET /api/healthz
```

Auth: single bearer token from env (`APP_TOKEN`) — it's a personal app.

## Web dashboard (web/)

- **Overview**: KPI cards (this week), weekly volume bars, CTL/ATL/TSB chart, recovery card, recent rides.
- **Activities**: filterable table → **Detail**: metrics, laps, auto week-over-week comparison vs previous similar ride.
- **Health**: sleep, HRV, RHR, stress charts. **Compare**: pick any two activities → delta table.
- Stack: Vite + React + TypeScript, Tailwind, Recharts. nginx serves build + proxies `/api`.

## Deployment

`deploy/docker-compose.yml`: `postgres:17` (volume), `backend` (multi-stage Go build), `web` (nginx). Env: `DATABASE_URL`, `COROS_MCP_URL`, `APP_TOKEN`, `SYNC_INTERVAL`. Run from repo root: `docker compose up -d`. Catch-up sync means no gaps even when the PC is off. Optional later: Tailscale on PC + phone for anywhere access (no port forwarding).

## Key libraries

`go-chi/chi/v5`, `jackc/pgx/v5` + `sqlc` (type-safe queries), `pressly/goose` (migrations), `modelcontextprotocol/go-sdk` (MCP client + OAuth), `robfig/cron` (or plain ticker). Phase 2 FIT parsing: `github.com/muktihari/fit`.

## Milestones

1. Skeleton: Go module, Chi server + `/api/healthz`, goose migrations, Docker Compose up.
2. COROS MCP client: OAuth login, `querySportRecords` + `getActivityDetail` round-trip logged.
3. Sync worker + backfill CLI: full history imported, upserts verified.
4. REST API + analysis module (with unit tests for CTL/ATL math).
5. Web dashboard: Overview, Activities, Detail, Health, Compare.
6. Polish: README, token auth, `docker compose up -d` end-to-end.
7. Phase 2 (separate later effort): FIT file ingestion (lazy download — 50 files/24h cap; HR curves, zone distribution), Android app (Kotlin + Compose against same API).

## Verification

- Unit tests: CTL/ATL/TSB against hand-computed fixtures; weekly aggregation.
- Integration tests: `testcontainers-go` Postgres for store upserts.
- E2E: run backfill → confirm today's ride (LabelId `480786453874376808`: 42.31 km, 23.2 km/h, 148 bpm avg HR, load 249) and Sep 27 (`480623074922758445`: 64.33 km, 153 bpm) are stored with matching values; cross-check weekly totals against a COROS MCP query from this session.
- Dashboard: trigger sync, verify Overview charts render and the Compare page reproduces the earlier Sep 27 vs Oct 4 delta table.

## Risks / notes

- COROS MCP tool list may evolve → raw JSONB storage makes schema drift non-destructive.
- Don't share Command Code's live token file; use own OAuth authorization (or one-time copy) to avoid refresh-rotation races.
- FIT downloads capped at 50/24h → lazy, rate-limited fetching in phase 2.
- MCP gives summary-level data; per-second streams only via FIT (phase 2).
