package store

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Tokens is a persisted OAuth token set plus the client registration
// details needed to refresh it after a restart.
type Tokens struct {
	AccessToken  string
	RefreshToken string
	Expiry       time.Time
	TokenType    string
	Scope        string
	ClientID     string
	ClientSecret string
	AuthURL      string
	TokenURL     string
}

// GetTokens returns the stored token set, or nil when never logged in.
func GetTokens(ctx context.Context, pool *pgxpool.Pool) (*Tokens, error) {
	row := pool.QueryRow(ctx, `
		SELECT access_token, refresh_token, expires_at, token_type, scope,
		       COALESCE(client_id, ''), COALESCE(client_secret, ''),
		       COALESCE(auth_url, ''), COALESCE(token_url, '')
		FROM oauth_tokens WHERE id = 1`)
	var t Tokens
	err := row.Scan(&t.AccessToken, &t.RefreshToken, &t.Expiry, &t.TokenType, &t.Scope,
		&t.ClientID, &t.ClientSecret, &t.AuthURL, &t.TokenURL)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &t, nil
}

// SaveTokens upserts the single token row.
func SaveTokens(ctx context.Context, pool *pgxpool.Pool, t *Tokens) error {
	_, err := pool.Exec(ctx, `
		INSERT INTO oauth_tokens
			(id, access_token, refresh_token, expires_at, token_type, scope,
			 client_id, client_secret, auth_url, token_url, updated_at)
		VALUES (1, $1, $2, $3, $4, $5, $6, $7, $8, $9, now())
		ON CONFLICT (id) DO UPDATE SET
			access_token = EXCLUDED.access_token,
			refresh_token = COALESCE(EXCLUDED.refresh_token, oauth_tokens.refresh_token),
			expires_at = EXCLUDED.expires_at,
			token_type = EXCLUDED.token_type,
			scope = EXCLUDED.scope,
			client_id = COALESCE(EXCLUDED.client_id, oauth_tokens.client_id),
			client_secret = COALESCE(EXCLUDED.client_secret, oauth_tokens.client_secret),
			auth_url = COALESCE(EXCLUDED.auth_url, oauth_tokens.auth_url),
			token_url = COALESCE(EXCLUDED.token_url, oauth_tokens.token_url),
			updated_at = now()`,
		t.AccessToken, t.RefreshToken, t.Expiry, t.TokenType, t.Scope,
		t.ClientID, t.ClientSecret, t.AuthURL, t.TokenURL)
	return err
}

// Activity is a fully-synced activity record.
type Activity struct {
	LabelID        string
	SportType      int
	StartTime      time.Time
	EndTime        time.Time
	DurationS      int
	TotalTimeS     int
	DistanceM      float64
	AvgSpeedKmh    *float64
	MaxSpeedKmh    *float64
	AvgHR          *int
	MaxHR          *int
	Calories       *int
	ElevGainM      *float64
	ElevLossM      *float64
	TrainingLoad   *float64
	AerobicTE      *float64
	AnaerobicTE    *float64
	TrainingFocus  *string
	Performance    *int
	Location       *string
	StartLat       *float64
	StartLon       *float64
	RawSummary     string
	RawDetail      string
}

// UpsertActivity inserts or updates one activity by label_id.
func UpsertActivity(ctx context.Context, pool *pgxpool.Pool, a Activity) error {
	_, err := pool.Exec(ctx, `
		INSERT INTO activities
			(label_id, sport_type, start_time, end_time, duration_s, total_time_s,
			 distance_m, avg_speed_kmh, max_speed_kmh, avg_hr, max_hr, calories,
			 elevation_gain_m, elevation_loss_m, training_load, aerobic_te, anaerobic_te,
			 training_focus, performance, location, start_lat, start_lon,
			 raw_summary, raw_detail, synced_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24, now())
		ON CONFLICT (label_id) DO UPDATE SET
			sport_type = EXCLUDED.sport_type,
			start_time = EXCLUDED.start_time,
			end_time = EXCLUDED.end_time,
			duration_s = EXCLUDED.duration_s,
			total_time_s = EXCLUDED.total_time_s,
			distance_m = EXCLUDED.distance_m,
			avg_speed_kmh = EXCLUDED.avg_speed_kmh,
			max_speed_kmh = EXCLUDED.max_speed_kmh,
			avg_hr = EXCLUDED.avg_hr,
			max_hr = EXCLUDED.max_hr,
			calories = EXCLUDED.calories,
			elevation_gain_m = EXCLUDED.elevation_gain_m,
			elevation_loss_m = EXCLUDED.elevation_loss_m,
			training_load = EXCLUDED.training_load,
			aerobic_te = EXCLUDED.aerobic_te,
			anaerobic_te = EXCLUDED.anaerobic_te,
			training_focus = EXCLUDED.training_focus,
			performance = EXCLUDED.performance,
			location = EXCLUDED.location,
			start_lat = EXCLUDED.start_lat,
			start_lon = EXCLUDED.start_lon,
			raw_summary = EXCLUDED.raw_summary,
			raw_detail = EXCLUDED.raw_detail,
			synced_at = now()`,
		a.LabelID, a.SportType, a.StartTime, a.EndTime, a.DurationS, a.TotalTimeS,
		a.DistanceM, a.AvgSpeedKmh, a.MaxSpeedKmh, a.AvgHR, a.MaxHR, a.Calories,
		a.ElevGainM, a.ElevLossM, a.TrainingLoad, a.AerobicTE, a.AnaerobicTE,
		a.TrainingFocus, a.Performance, a.Location, a.StartLat, a.StartLon,
		a.RawSummary, a.RawDetail)
	return err
}

// ReplaceLaps swaps all stored lap groups for one activity.
func ReplaceLaps(ctx context.Context, pool *pgxpool.Pool, labelID string, laps []LapRow) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `DELETE FROM activity_laps WHERE label_id = $1`, labelID); err != nil {
		return err
	}
	for _, l := range laps {
		if _, err := tx.Exec(ctx, `
			INSERT INTO activity_laps (label_id, group_type, lap_index, data)
			VALUES ($1, $2, $3, $4)
			ON CONFLICT (label_id, group_type, lap_index) DO UPDATE SET data = EXCLUDED.data`,
			labelID, l.GroupType, l.LapIndex, l.Data); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

// LapRow is one lap within a lap group.
type LapRow struct {
	GroupType int
	LapIndex  int
	Data      []byte
}

// HealthPoint is one (date, metric) observation.
type HealthPoint struct {
	Date      time.Time
	Metric    string
	Value     *float64
	ValueText *string
	Raw       *string
}

// UpsertHealthPoints upserts a batch of daily health observations.
func UpsertHealthPoints(ctx context.Context, pool *pgxpool.Pool, pts []HealthPoint) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	for _, p := range pts {
		if _, err := tx.Exec(ctx, `
			INSERT INTO daily_health (date, metric, value, value_text, raw)
			VALUES ($1, $2, $3, $4, $5)
			ON CONFLICT (date, metric) DO UPDATE SET
				value = EXCLUDED.value,
				value_text = EXCLUDED.value_text,
				raw = EXCLUDED.raw`,
			p.Date, p.Metric, p.Value, p.ValueText, p.Raw); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

// UpsertFitnessSnapshot stores the fitness assessment for one day.
func UpsertFitnessSnapshot(ctx context.Context, pool *pgxpool.Pool, date time.Time, vo2max *float64, raw string) error {
	_, err := pool.Exec(ctx, `
		INSERT INTO fitness_snapshots (date, vo2max, raw, synced_at)
		VALUES ($1, $2, $3, now())
		ON CONFLICT (date) DO UPDATE SET
			vo2max = EXCLUDED.vo2max,
			raw = EXCLUDED.raw,
			synced_at = now()`,
		date, vo2max, raw)
	return err
}

// UpsertProfile stores the user profile.
func UpsertProfile(ctx context.Context, pool *pgxpool.Pool, nickname *string, heightCM, weightKG *float64, birthday *time.Time, gender, raw string) error {
	_, err := pool.Exec(ctx, `
		INSERT INTO profile (id, nickname, height_cm, weight_kg, birthday, gender, raw, updated_at)
		VALUES (1, $1, $2, $3, $4, $5, $6, now())
		ON CONFLICT (id) DO UPDATE SET
			nickname = EXCLUDED.nickname,
			height_cm = EXCLUDED.height_cm,
			weight_kg = EXCLUDED.weight_kg,
			birthday = EXCLUDED.birthday,
			gender = EXCLUDED.gender,
			raw = EXCLUDED.raw,
			updated_at = now()`,
		nickname, heightCM, weightKG, birthday, gender, raw)
	return err
}

// SyncState tracks the last successful sync timestamps.
type SyncState struct {
	LastActivitySync *time.Time
	LastHealthSync   *time.Time
}

// GetSyncState reads the singleton sync state row.
func GetSyncState(ctx context.Context, pool *pgxpool.Pool) (*SyncState, error) {
	row := pool.QueryRow(ctx, `SELECT last_activity_sync, last_health_sync FROM sync_state WHERE id = 1`)
	var s SyncState
	if err := row.Scan(&s.LastActivitySync, &s.LastHealthSync); err != nil {
		return nil, err
	}
	return &s, nil
}

// SetSyncState writes the sync timestamps.
func SetSyncState(ctx context.Context, pool *pgxpool.Pool, activityAt, healthAt *time.Time) error {
	_, err := pool.Exec(ctx, `
		UPDATE sync_state
		SET last_activity_sync = COALESCE($1, last_activity_sync),
		    last_health_sync = COALESCE($2, last_health_sync),
		    updated_at = now()
		WHERE id = 1`, activityAt, healthAt)
	return err
}
