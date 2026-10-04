package coros

import (
	"testing"
	"time"
)

const sportRecordsSample = `"Sport Records — 2026-10-04 to 2026-10-04 (1 records)
========================

1. Cycling — 2026-10-04
   Location: Depok Road Bike
   Start Coordinates: -6.406000, 106.793999
   Time Window: startTimestamp=1791068666 | endTimestamp=1791077412
   Duration: 1:49:36 | Distance: 42.31 km
   Average Speed: 23.2 km/h | Avg HR: 148 bpm | Calories: 1728 kcal
   LabelId: 480786453874376808 | SportType: 200"`

func TestParseSportRecords(t *testing.T) {
	recs := ParseSportRecords(sportRecordsSample)
	if len(recs) != 1 {
		t.Fatalf("got %d records, want 1", len(recs))
	}
	r := recs[0]
	if r.LabelID != "480786453874376808" {
		t.Errorf("LabelID = %q", r.LabelID)
	}
	if r.SportType != 200 {
		t.Errorf("SportType = %d", r.SportType)
	}
	if r.DurationS != 1*3600+49*60+36 {
		t.Errorf("DurationS = %d", r.DurationS)
	}
	if r.DistanceM != 42310 {
		t.Errorf("DistanceM = %v", r.DistanceM)
	}
	if r.AvgSpeedKmh == nil || *r.AvgSpeedKmh != 23.2 {
		t.Errorf("AvgSpeedKmh = %v", r.AvgSpeedKmh)
	}
	if r.AvgHR == nil || *r.AvgHR != 148 {
		t.Errorf("AvgHR = %v", r.AvgHR)
	}
	if r.Calories == nil || *r.Calories != 1728 {
		t.Errorf("Calories = %v", r.Calories)
	}
	if r.Location == nil || *r.Location != "Depok Road Bike" {
		t.Errorf("Location = %v", r.Location)
	}
	if r.StartLat == nil || *r.StartLat != -6.406 {
		t.Errorf("StartLat = %v", r.StartLat)
	}
	if r.StartTime.Unix() != 1791068666 {
		t.Errorf("StartTime = %v", r.StartTime)
	}
	if r.EndTime.Unix() != 1791077412 {
		t.Errorf("EndTime = %v", r.EndTime)
	}
}

const activityDetailSample = `🚴 Cycling Activity Details
========================================

Workout Time: 1:49:36
Distance: 42.31 km
Total Time: 2:25:46
Calories: 1728 kcal
Average Speed: 23.2 km/h
Moving Average Speed: 23.2 km/h
Max Speed: 43.5 km/h
Average Heart Rate: 148 bpm
Elevation Gain / Loss: 182 m / 184 m
Training Load: 249
Efficiency Factor: 655.35
Variation Index: 655.35
Aerobic TE: 4.1
Anaerobic TE: 3.3
Training Focus: Threshold
Performance: -1`

func TestParseActivityDetail(t *testing.T) {
	d, err := ParseActivityDetail(activityDetailSample)
	if err != nil {
		t.Fatal(err)
	}
	if d.DurationS != 1*3600+49*60+36 {
		t.Errorf("DurationS = %d", d.DurationS)
	}
	if d.TotalTimeS != 2*3600+25*60+46 {
		t.Errorf("TotalTimeS = %d", d.TotalTimeS)
	}
	if d.DistanceM != 42310 {
		t.Errorf("DistanceM = %v", d.DistanceM)
	}
	if d.MaxSpeedKmh == nil || *d.MaxSpeedKmh != 43.5 {
		t.Errorf("MaxSpeedKmh = %v", d.MaxSpeedKmh)
	}
	if d.ElevGainM == nil || *d.ElevGainM != 182 {
		t.Errorf("ElevGainM = %v", d.ElevGainM)
	}
	if d.ElevLossM == nil || *d.ElevLossM != 184 {
		t.Errorf("ElevLossM = %v", d.ElevLossM)
	}
	if d.TrainingLoad == nil || *d.TrainingLoad != 249 {
		t.Errorf("TrainingLoad = %v", d.TrainingLoad)
	}
	if d.AerobicTE == nil || *d.AerobicTE != 4.1 {
		t.Errorf("AerobicTE = %v", d.AerobicTE)
	}
	if d.TrainingFocus == nil || *d.TrainingFocus != "Threshold" {
		t.Errorf("TrainingFocus = %v", d.TrainingFocus)
	}
	if d.Performance == nil || *d.Performance != -1 {
		t.Errorf("Performance = %v", d.Performance)
	}
}

const dailyHealthSample = `"Daily Health Data — Last 3 days | Resting HR: 61 bpm | HRV Baseline: 42 ms
Note: sleep entries are dated by their wake-up day.

--- 20261002 ---
Steps: 0 | Calories: 0 kcal | Exercise: 0 min

--- 20261003 ---
Steps: 1,987 | Calories: 530 kcal | Exercise: 52 min

--- 20261004 ---
Steps: 1,150 | Calories: 1,988 kcal | Exercise: 1h 50min
Sleep Summary:
  Total: 3h 44min | Deep: 43 min | Light: 2h 26min | REM: 24 min | Awake: 11 min
  Sleep HR: Avg 53 bpm | Min 45 bpm | Max 71 bpm"`

func TestParseDailyHealth(t *testing.T) {
	pts := ParseDailyHealth(dailyHealthSample)
	byKey := map[string]float64{}
	for _, p := range pts {
		byKey[p.Date.Format("20060102")+"|"+p.Metric] = *p.Value
	}
	if v := byKey["20261003|steps"]; v != 1987 {
		t.Errorf("steps 1003 = %v", v)
	}
	if v := byKey["20261003|exercise_min"]; v != 52 {
		t.Errorf("exercise 1003 = %v", v)
	}
	if v := byKey["20261004|steps"]; v != 1150 {
		t.Errorf("steps 1004 = %v", v)
	}
	if v := byKey["20261004|calories"]; v != 1988 {
		t.Errorf("calories 1004 = %v", v)
	}
	if v := byKey["20261004|exercise_min"]; v != 110 {
		t.Errorf("exercise 1004 = %v", v)
	}
	if v := byKey["20261004|sleep_total_min"]; v != 224 {
		t.Errorf("sleep_total 1004 = %v", v)
	}
	if v := byKey["20261004|sleep_deep_min"]; v != 43 {
		t.Errorf("sleep_deep 1004 = %v", v)
	}
	if v := byKey["20261004|sleep_light_min"]; v != 146 {
		t.Errorf("sleep_light 1004 = %v", v)
	}
	if v := byKey["20261004|sleep_rem_min"]; v != 24 {
		t.Errorf("sleep_rem 1004 = %v", v)
	}
	if v := byKey["20261004|sleep_hr_avg"]; v != 53 {
		t.Errorf("sleep_hr_avg 1004 = %v", v)
	}
	if v := byKey["20261004|sleep_hr_max"]; v != 71 {
		t.Errorf("sleep_hr_max 1004 = %v", v)
	}
}

const sleepOverviewSample = `"Sleep Overview
========================
Note: each record below is dated by its wake-up day.

2026-10-02
Sleep Score: 0
Sleep detail for this day is not available yet.

2026-10-04
Sleep Score: 74
Daily Sleep: 6h 49min (incl. naps)
Main Sleep (asleep): 3h 33min
Main Sleep Period (incl. awake): 3h 44min
Sleep metrics scope: daily
Deep Sleep Ratio: 19%
Light Sleep Ratio: 65%
REM Ratio: 11%
Awake Ratio: 5%
Awake Time: 11 min
Awake Count (>5 min): 0
Main Sleep Window: 2026-10-04 01:30 - 2026-10-04 05:14
Naps Total (asleep): 3h 16min
Naps Period (incl. awake): 3h 26min
Nap Window: 2026-10-04 13:35 - 2026-10-04 17:01"`

func TestParseSleepOverview(t *testing.T) {
	pts := ParseSleepOverview(sleepOverviewSample)
	byKey := map[string]float64{}
	for _, p := range pts {
		byKey[p.Date.Format("2006-01-02")+"|"+p.Metric] = *p.Value
	}
	if v := byKey["2026-10-04|sleep_score"]; v != 74 {
		t.Errorf("score = %v", v)
	}
	if v := byKey["2026-10-04|sleep_daily_min"]; v != 409 {
		t.Errorf("daily = %v", v)
	}
	if v := byKey["2026-10-04|sleep_main_min"]; v != 213 {
		t.Errorf("main = %v", v)
	}
	if v := byKey["2026-10-04|sleep_deep_ratio"]; v != 19 {
		t.Errorf("deep ratio = %v", v)
	}
	if v := byKey["2026-10-04|sleep_awake_count"]; v != 0 {
		t.Errorf("awake count = %v", v)
	}
	if v := byKey["2026-10-04|nap_min"]; v != 196 {
		t.Errorf("nap = %v", v)
	}
	if v := byKey["2026-10-02|sleep_score"]; v != 0 {
		t.Errorf("1002 score = %v", v)
	}
}

const sleepHrvSample = `"Sleep HRV — 2026-10-02 to 2026-10-04
========================
Note: dates are wake-up days (each value comes from the night that ended that morning).

HRV Assessment — Last 3 days
========================

2026-10-04:
  HRV Avg: 97 ms — Normal
  Normal Range: 76 - 100 ms
  Baseline: 88 ms
2026-10-02:
  No data

Sleep HRV Time Series — Last 3 days
========================

2026-10-04:
  timestamp=1791053690, timezone=28, hrv=89 ms, status=4, confidence=88810"`

func TestParseSleepHrv(t *testing.T) {
	pts := ParseSleepHrv(sleepHrvSample)
	byKey := map[string]float64{}
	for _, p := range pts {
		byKey[p.Date.Format("2006-01-02")+"|"+p.Metric] = *p.Value
	}
	if v := byKey["2026-10-04|hrv_avg"]; v != 97 {
		t.Errorf("hrv_avg = %v", v)
	}
	if v := byKey["2026-10-04|hrv_normal_low"]; v != 76 {
		t.Errorf("hrv_normal_low = %v", v)
	}
	if v := byKey["2026-10-04|hrv_normal_high"]; v != 100 {
		t.Errorf("hrv_normal_high = %v", v)
	}
	if v := byKey["2026-10-04|hrv_baseline"]; v != 88 {
		t.Errorf("hrv_baseline = %v", v)
	}
	if len(pts) != 4 {
		t.Errorf("expected 4 points, got %d", len(pts))
	}
}

func TestParseRestingHR(t *testing.T) {
	pts := ParseDatedValues(`"Resting Heart Rate — Last 3 days
========================

2026-10-04: 51 bpm
2026-10-03: No data"`, "rhr")
	if len(pts) != 1 {
		t.Fatalf("got %d points, want 1", len(pts))
	}
	if pts[0].Date.Format("2006-01-02") != "2026-10-04" || *pts[0].Value != 51 {
		t.Errorf("point = %+v", pts[0])
	}
}

func TestParseRecovery(t *testing.T) {
	today := time.Date(2026, 10, 4, 0, 0, 0, 0, time.Local)
	pts := ParseRecovery(`"Recovery Status
========================

Recovery: 62%
Level: Light training recommended
Estimated Full Recovery: 49h"`, today)
	byMetric := map[string]float64{}
	textMetric := ""
	for _, p := range pts {
		if p.Value != nil {
			byMetric[p.Metric] = *p.Value
		}
		if p.ValueText != nil {
			textMetric = p.Metric + "=" + *p.ValueText
		}
	}
	if byMetric["recovery_pct"] != 62 {
		t.Errorf("recovery_pct = %v", byMetric["recovery_pct"])
	}
	if byMetric["recovery_full_h"] != 49 {
		t.Errorf("recovery_full_h = %v", byMetric["recovery_full_h"])
	}
	if textMetric != "recovery_level=Light training recommended" {
		t.Errorf("recovery_level = %q", textMetric)
	}
}

const trainingLoadSample = `"Training Load Assessment
========================

2026-10-04
Comment: Optimized
Short-Term Load: 69
Long-Term Load: 61
Load Ratio: 1.13

2026-10-03
Comment: Performance
Short-Term Load: 38
Long-Term Load: 56
Load Ratio: 0.67"`

func TestParseTrainingLoad(t *testing.T) {
	pts := ParseTrainingLoad(trainingLoadSample)
	byKey := map[string]float64{}
	comments := map[string]string{}
	for _, p := range pts {
		if p.Value != nil {
			byKey[p.Date.Format("2006-01-02")+"|"+p.Metric] = *p.Value
		}
		if p.ValueText != nil {
			comments[p.Date.Format("2006-01-02")] = *p.ValueText
		}
	}
	if v := byKey["2026-10-04|load_short"]; v != 69 {
		t.Errorf("load_short = %v", v)
	}
	if v := byKey["2026-10-04|load_long"]; v != 61 {
		t.Errorf("load_long = %v", v)
	}
	if v := byKey["2026-10-04|load_ratio"]; v != 1.13 {
		t.Errorf("load_ratio = %v", v)
	}
	if comments["2026-10-04"] != "Optimized" {
		t.Errorf("comment = %q", comments["2026-10-04"])
	}
	if v := byKey["2026-10-03|load_short"]; v != 38 {
		t.Errorf("1003 load_short = %v", v)
	}
}

func TestParseFitnessOverview(t *testing.T) {
	f, err := ParseFitnessOverview(`"Fitness Assessment Overview
========================

VO2max: 35
Running Level: 45
Threshold Pace: 7:35 /km
5 km Prediction: 36:46
10 km Prediction: 1:19:38
Half Marathon Prediction: 3:04:53
Marathon Prediction: 6:45:51"`)
	if err != nil {
		t.Fatal(err)
	}
	if f.VO2max == nil || *f.VO2max != 35 {
		t.Errorf("VO2max = %v", f.VO2max)
	}
	if f.RunningLevel == nil || *f.RunningLevel != 45 {
		t.Errorf("RunningLevel = %v", f.RunningLevel)
	}
	if f.ThresholdPace == nil || *f.ThresholdPace != "7:35 /km" {
		t.Errorf("ThresholdPace = %v", f.ThresholdPace)
	}
}

func TestParseProfile(t *testing.T) {
	p, err := ParseProfile(`"User Profile Information
========================

Height: 178.0 cm
Weight: 115.1 kg
Birthday: 1999-05-29 (Age: 27)
Gender: Male
Nickname: dyatmarize"`)
	if err != nil {
		t.Fatal(err)
	}
	if p.HeightCM == nil || *p.HeightCM != 178.0 {
		t.Errorf("HeightCM = %v", p.HeightCM)
	}
	if p.WeightKG == nil || *p.WeightKG != 115.1 {
		t.Errorf("WeightKG = %v", p.WeightKG)
	}
	if p.Birthday == nil || p.Birthday.Format("2006-01-02") != "1999-05-29" {
		t.Errorf("Birthday = %v", p.Birthday)
	}
	if p.Nickname == nil || *p.Nickname != "dyatmarize" {
		t.Errorf("Nickname = %v", p.Nickname)
	}
}

func TestParseLaps(t *testing.T) {
	raw := []byte(`{"source":"activityDetail","labelId":"x","sportType":200,"lapGroups":[
		{"type":11,"lapDistance":500000,"laps":[
			{"lapIndex":1,"distance":500000,"time":846.17,"avgSpeedV2":2127.0,"avgHr":129},
			{"lapIndex":2,"distance":500000,"time":706.70,"avgSpeedV2":2547.0,"avgHr":135}
		]},
		{"type":2,"lapDistance":500000,"laps":[
			{"lapIndex":1,"distance":4231286,"time":6576.27,"avgSpeedV2":2316.0,"avgHr":148}
		]}
	]}`)
	laps, err := ParseLaps(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(laps) != 3 {
		t.Fatalf("got %d laps, want 3", len(laps))
	}
	if laps[0].GroupType != 11 || laps[0].LapIndex != 1 {
		t.Errorf("lap[0] = %+v", laps[0])
	}
	if laps[2].GroupType != 2 || laps[2].LapIndex != 1 {
		t.Errorf("lap[2] = %+v", laps[2])
	}
}

func TestParseDuration(t *testing.T) {
	cases := map[string]int{
		"1:49:36":   1*3600 + 49*60 + 36,
		"36:46":     36*60 + 46,
		"1h 50min":  110 * 60,
		"52 min":    52 * 60,
		"3h 44min":  224 * 60,
		"49h":       49 * 3600,
		"0 min":     0,
		"11 min":    11 * 60,
		"2h 26min":  146 * 60,
	}
	for in, want := range cases {
		got, ok := parseDuration(in)
		if !ok || got != want {
			t.Errorf("parseDuration(%q) = %d, %v; want %d", in, got, ok, want)
		}
	}
	if _, ok := parseDuration("No data"); ok {
		t.Errorf("parseDuration(No data) should fail")
	}
}
