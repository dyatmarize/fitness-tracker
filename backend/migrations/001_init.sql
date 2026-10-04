-- +goose Up
CREATE TABLE activities (
    label_id         TEXT PRIMARY KEY,
    sport_type       INT NOT NULL,
    start_time       TIMESTAMPTZ NOT NULL,
    end_time         TIMESTAMPTZ,
    duration_s       INT NOT NULL DEFAULT 0,
    total_time_s     INT NOT NULL DEFAULT 0,
    distance_m       DOUBLE PRECISION NOT NULL DEFAULT 0,
    avg_speed_kmh    DOUBLE PRECISION,
    max_speed_kmh    DOUBLE PRECISION,
    avg_hr           INT,
    max_hr           INT,
    calories         INT,
    elevation_gain_m DOUBLE PRECISION,
    elevation_loss_m DOUBLE PRECISION,
    training_load    DOUBLE PRECISION,
    aerobic_te       DOUBLE PRECISION,
    anaerobic_te     DOUBLE PRECISION,
    training_focus   TEXT,
    performance      INT,
    location         TEXT,
    start_lat        DOUBLE PRECISION,
    start_lon        DOUBLE PRECISION,
    raw_summary      TEXT,
    raw_detail       TEXT,
    synced_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_activities_start_time ON activities (start_time DESC);
CREATE INDEX idx_activities_sport_type ON activities (sport_type);

CREATE TABLE activity_laps (
    label_id   TEXT NOT NULL REFERENCES activities(label_id) ON DELETE CASCADE,
    group_type INT NOT NULL,
    lap_index  INT NOT NULL,
    data       JSONB,
    PRIMARY KEY (label_id, group_type, lap_index)
);

CREATE TABLE daily_health (
    date       DATE NOT NULL,
    metric     TEXT NOT NULL,
    value      DOUBLE PRECISION,
    value_text TEXT,
    raw        TEXT,
    PRIMARY KEY (date, metric)
);

CREATE TABLE fitness_snapshots (
    date      DATE PRIMARY KEY,
    vo2max    DOUBLE PRECISION,
    raw       TEXT,
    synced_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE sync_state (
    id                 INT PRIMARY KEY DEFAULT 1 CHECK (id = 1),
    last_activity_sync TIMESTAMPTZ,
    last_health_sync   TIMESTAMPTZ,
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT now()
);

INSERT INTO sync_state (id) VALUES (1) ON CONFLICT DO NOTHING;

CREATE TABLE oauth_tokens (
    id            INT PRIMARY KEY DEFAULT 1 CHECK (id = 1),
    access_token  TEXT NOT NULL,
    refresh_token TEXT,
    expires_at    TIMESTAMPTZ NOT NULL,
    token_type    TEXT NOT NULL DEFAULT 'Bearer',
    scope         TEXT,
    client_id     TEXT,
    client_secret TEXT,
    auth_url      TEXT,
    token_url     TEXT,
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE profile (
    id         INT PRIMARY KEY DEFAULT 1 CHECK (id = 1),
    nickname   TEXT,
    height_cm  DOUBLE PRECISION,
    weight_kg  DOUBLE PRECISION,
    birthday   DATE,
    gender     TEXT,
    raw        TEXT,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE IF EXISTS profile;
DROP TABLE IF EXISTS oauth_tokens;
DROP TABLE IF EXISTS sync_state;
DROP TABLE IF EXISTS fitness_snapshots;
DROP TABLE IF EXISTS daily_health;
DROP TABLE IF EXISTS activity_laps;
DROP TABLE IF EXISTS activities;
