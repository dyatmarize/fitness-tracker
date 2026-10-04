package coros

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/dyatmarize/fitness-tracker/backend/internal/store"
)

// Service performs high-level sync operations against COROS.
type Service struct {
	client *Client
	pool   *pgxpool.Pool
}

// NewService creates a sync service.
func NewService(client *Client, pool *pgxpool.Pool) *Service {
	return &Service{client: client, pool: pool}
}

func dateStr(t time.Time) string { return t.In(time.Local).Format("20060102") }

// SyncActivities syncs all activities in [from, to] (inclusive, local dates).
// It returns the number of activities synced.
func (s *Service) SyncActivities(ctx context.Context, from, to time.Time) (int, error) {
	count := 0
	start := from
	for start.Before(to) || start.Equal(to) {
		end := start.AddDate(0, 0, 29)
		if end.After(to) {
			end = to
		}
		text, err := s.client.CallText(ctx, "querySportRecords", map[string]any{
			"startDate":          dateStr(start),
			"endDate":            dateStr(end),
			"sportTypeCodes":     []int{65535},
			"minDistanceKm":      0,
			"maxDistanceKm":      0,
			"minDurationMinutes": 0,
			"maxDurationMinutes": 0,
			"maxAveragePace":     "",
			"locationKeyword":    "",
			"limit":              200,
		})
		if err != nil {
			return count, fmt.Errorf("querySportRecords %s..%s: %w", dateStr(start), dateStr(end), err)
		}
		records := ParseSportRecords(text)
		for _, rec := range records {
			if err := s.syncOneActivity(ctx, rec); err != nil {
				s.client.logf("activity %s: %v", rec.LabelID, err)
				continue
			}
			count++
		}
		if end.Equal(to) {
			break
		}
		start = end.AddDate(0, 0, 1)
	}
	return count, nil
}

func (s *Service) syncOneActivity(ctx context.Context, rec ActivitySummary) error {
	act := store.Activity{
		LabelID:     rec.LabelID,
		SportType:   rec.SportType,
		StartTime:   rec.StartTime,
		EndTime:     rec.EndTime,
		DurationS:   rec.DurationS,
		DistanceM:   rec.DistanceM,
		AvgSpeedKmh: rec.AvgSpeedKmh,
		AvgHR:       rec.AvgHR,
		Calories:     rec.Calories,
		Location:    rec.Location,
		StartLat:    rec.StartLat,
		StartLon:    rec.StartLon,
		RawSummary:  rec.Raw,
	}

	// Enrich with the detail call; failure is non-fatal (summary still stored).
	if detailText, err := s.client.CallText(ctx, "getActivityDetail", map[string]any{
		"labelId":   rec.LabelID,
		"sportType": rec.SportType,
	}); err == nil {
		if d, perr := ParseActivityDetail(detailText); perr == nil {
			if d.DurationS != 0 {
				act.DurationS = d.DurationS
			}
			act.TotalTimeS = d.TotalTimeS
			if d.DistanceM != 0 {
				act.DistanceM = d.DistanceM
			}
			if d.Calories != nil {
				act.Calories = d.Calories
			}
			act.MaxSpeedKmh = d.MaxSpeedKmh
			if d.AvgSpeedKmh != nil {
				act.AvgSpeedKmh = d.AvgSpeedKmh
			}
			if d.AvgHR != nil {
				act.AvgHR = d.AvgHR
			}
			act.ElevGainM = d.ElevGainM
			act.ElevLossM = d.ElevLossM
			act.TrainingLoad = d.TrainingLoad
			act.AerobicTE = d.AerobicTE
			act.AnaerobicTE = d.AnaerobicTE
			act.TrainingFocus = d.TrainingFocus
			act.Performance = d.Performance
			act.RawDetail = d.Raw
		}
	} else {
		s.client.logf("activity %s: detail: %v", rec.LabelID, err)
	}

	if err := store.UpsertActivity(ctx, s.pool, act); err != nil {
		return fmt.Errorf("upsert: %w", err)
	}

	// Laps are optional too.
	if lapJSON, err := s.client.CallJSON(ctx, "queryActivityLapData", map[string]any{
		"labelId":   rec.LabelID,
		"sportType": rec.SportType,
	}); err == nil {
		laps, perr := ParseLaps(lapJSON)
		if perr == nil && len(laps) > 0 {
			rows := make([]store.LapRow, len(laps))
			for i, l := range laps {
				rows[i] = store.LapRow{GroupType: l.GroupType, LapIndex: l.LapIndex, Data: l.Data}
			}
			if err := store.ReplaceLaps(ctx, s.pool, rec.LabelID, rows); err != nil {
				s.client.logf("activity %s: laps: %v", rec.LabelID, err)
			}
		}
	}

	return nil
}

// SyncHealth syncs the trailing days of daily health data plus the
// current-day snapshots (recovery, fitness, profile).
func (s *Service) SyncHealth(ctx context.Context, days int) error {
	if days < 1 {
		days = 30
	}
	today := time.Now().In(time.Local)

	var pts []store.HealthPoint
	appendPts := func(more []HealthPoint) {
		for _, p := range more {
			pts = append(pts, store.HealthPoint{Date: p.Date, Metric: p.Metric, Value: p.Value, ValueText: p.ValueText})
		}
	}

	if text, err := s.client.CallText(ctx, "queryDailyHealthData", map[string]any{"days": days}); err != nil {
		return fmt.Errorf("queryDailyHealthData: %w", err)
	} else {
		appendPts(ParseDailyHealth(text))
	}

	if text, err := s.client.CallText(ctx, "querySleepOverview", map[string]any{"days": days}); err != nil {
		s.client.logf("querySleepOverview: %v", err)
	} else {
		appendPts(ParseSleepOverview(text))
	}

	hrvDays := days
	if hrvDays > 7 {
		hrvDays = 7
	}
	if text, err := s.client.CallText(ctx, "querySleepHrv", map[string]any{"days": hrvDays}); err != nil {
		s.client.logf("querySleepHrv: %v", err)
	} else {
		appendPts(ParseSleepHrv(text))
	}

	if text, err := s.client.CallText(ctx, "queryRestingHeartRate", map[string]any{"days": days}); err != nil {
		s.client.logf("queryRestingHeartRate: %v", err)
	} else {
		appendPts(ParseDatedValues(text, "rhr"))
	}

	if text, err := s.client.CallText(ctx, "queryStressLevel", map[string]any{"days": days}); err != nil {
		s.client.logf("queryStressLevel: %v", err)
	} else {
		appendPts(ParseDatedValues(text, "stress_avg"))
	}

	loadDays := days
	if loadDays > 7 {
		loadDays = 7
	}
	if text, err := s.client.CallText(ctx, "queryTrainingLoadAssessment", map[string]any{"days": loadDays}); err != nil {
		s.client.logf("queryTrainingLoadAssessment: %v", err)
	} else {
		appendPts(ParseTrainingLoad(text))
	}

	if text, err := s.client.CallText(ctx, "queryRecoveryStatus", map[string]any{}); err != nil {
		s.client.logf("queryRecoveryStatus: %v", err)
	} else {
		appendPts(ParseRecovery(text, today))
	}

	if len(pts) > 0 {
		if err := store.UpsertHealthPoints(ctx, s.pool, pts); err != nil {
			return fmt.Errorf("upsert health: %w", err)
		}
	}

	// Fitness snapshot for today.
	if text, err := s.client.CallText(ctx, "queryFitnessAssessmentOverview", map[string]any{}); err != nil {
		s.client.logf("queryFitnessAssessmentOverview: %v", err)
	} else if f, perr := ParseFitnessOverview(text); perr == nil {
		if err := store.UpsertFitnessSnapshot(ctx, s.pool, today, f.VO2max, f.Raw); err != nil {
			s.client.logf("fitness snapshot: %v", err)
		}
	}

	s.SyncProfile(ctx)
	return nil
}

// SyncProfile stores the user profile.
func (s *Service) SyncProfile(ctx context.Context) {
	text, err := s.client.CallText(ctx, "queryUserInfo", map[string]any{})
	if err != nil {
		s.client.logf("queryUserInfo: %v", err)
		return
	}
	p, err := ParseProfile(text)
	if err != nil {
		s.client.logf("parse profile: %v", err)
		return
	}
	if err := store.UpsertProfile(ctx, s.pool, p.Nickname, p.HeightCM, p.WeightKG, p.Birthday, deref(p.Gender), p.Raw); err != nil {
		s.client.logf("upsert profile: %v", err)
	}
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
