package coros

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// LapData is one lap within a lap group, kept as raw JSON.
type LapData struct {
	GroupType int
	LapIndex  int
	Data      []byte
}

// ---------- helpers ----------

var commaRe = regexp.MustCompile(",")

func parseNum(s string) (float64, bool) {
	v, err := strconv.ParseFloat(strings.TrimSpace(commaRe.ReplaceAllString(s, "")), 64)
	return v, err == nil
}

func parseInt(s string) (int, bool) {
	v, err := strconv.Atoi(strings.TrimSpace(commaRe.ReplaceAllString(s, "")))
	return v, err == nil
}

func f64ptr(s string) *float64 {
	if v, ok := parseNum(s); ok {
		return &v
	}
	return nil
}

func intptr(s string) *int {
	if v, ok := parseInt(s); ok {
		return &v
	}
	return nil
}

func strptr(s string) *string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	return &s
}

// parseDuration handles the formats COROS emits: "1:49:36", "36:46",
// "1h 50min", "52 min", "3h 44min", "49h". Returns seconds.
func parseDuration(s string) (int, bool) {
	s = strings.TrimSpace(s)
	if s == "" || strings.EqualFold(s, "no data") {
		return 0, false
	}
	if strings.Contains(s, ":") {
		parts := strings.Split(s, ":")
		if len(parts) < 2 || len(parts) > 3 {
			return 0, false
		}
		var total int
		for _, p := range parts {
			v, ok := parseInt(p)
			if !ok {
				return 0, false
			}
			total = total*60 + v
		}
		return total, true
	}
	var total int
	rest := s
	if i := strings.IndexByte(rest, 'h'); i >= 0 {
		if v, ok := parseInt(rest[:i]); ok {
			total += v * 3600
		} else {
			return 0, false
		}
		rest = strings.TrimSpace(rest[i+1:])
	}
	rest = strings.TrimSuffix(strings.TrimSpace(rest), "min")
	rest = strings.TrimSpace(rest)
	if rest != "" {
		v, ok := parseInt(rest)
		if !ok {
			return 0, false
		}
		total += v * 60
	}
	return total, true
}

func durPtr(s string) *int {
	if v, ok := parseDuration(s); ok {
		return &v
	}
	return nil
}

// durMinF64ptr parses a duration and returns it in minutes.
func durMinF64ptr(s string) *float64 {
	if v, ok := parseDuration(s); ok {
		f := float64(v) / 60
		return &f
	}
	return nil
}

// intF64ptr parses an integer into a float64 pointer.
func intF64ptr(s string) *float64 {
	if v, ok := parseInt(s); ok {
		f := float64(v)
		return &f
	}
	return nil
}

func parseDate8(s string) (time.Time, bool) {
	t, err := time.ParseInLocation("20060102", s, time.Local)
	return t, err == nil
}

func parseDate10(s string) (time.Time, bool) {
	t, err := time.ParseInLocation("2006-01-02", s, time.Local)
	return t, err == nil
}

func trimQuotes(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 2 {
		if (s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '\'' && s[len(s)-1] == '\'') {
			return s[1 : len(s)-1]
		}
	}
	return s
}

// ---------- querySportRecords ----------

var (
	reRecordHead  = regexp.MustCompile(`^\d+\.\s+(.+?)\s+[-—]\s+(\d{4}-\d{2}-\d{2})\s*$`)
	reCoords      = regexp.MustCompile(`Start Coordinates:\s*(-?[\d.]+),\s*(-?[\d.]+)`)
	reTimeWindow  = regexp.MustCompile(`startTimestamp=(\d+)\s*\|\s*endTimestamp=(\d+)`)
	reDurDist     = regexp.MustCompile(`Duration:\s*([^|]+?)\s*\|\s*Distance:\s*([\d.,]+)\s*km`)
	reSpeedHrCal  = regexp.MustCompile(`Average Speed:\s*([\d.]+)\s*km/h\s*\|\s*Avg HR:\s*(\d+)\s*bpm\s*\|\s*Calories:\s*([\d,]+)\s*kcal`)
	reLabelSport  = regexp.MustCompile(`LabelId:\s*(\d+)\s*\|\s*SportType:\s*(\d+)`)
)

// ParseSportRecords parses the text output of querySportRecords.
func ParseSportRecords(text string) []ActivitySummary {
	text = trimQuotes(text)
	var out []ActivitySummary
	var cur *ActivitySummary
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if m := reRecordHead.FindStringSubmatch(line); m != nil {
			if cur != nil {
				out = append(out, *cur)
			}
			cur = &ActivitySummary{}
			continue
		}
		if cur == nil {
			continue
		}
		if strings.HasPrefix(line, "Location:") {
			cur.Location = strptr(strings.TrimPrefix(line, "Location:"))
			continue
		}
		if m := reCoords.FindStringSubmatch(line); m != nil {
			cur.StartLat = f64ptr(m[1])
			cur.StartLon = f64ptr(m[2])
			continue
		}
		if m := reTimeWindow.FindStringSubmatch(line); m != nil {
			start, _ := strconv.ParseInt(m[1], 10, 64)
			end, _ := strconv.ParseInt(m[2], 10, 64)
			cur.StartTime = time.Unix(start, 0)
			cur.EndTime = time.Unix(end, 0)
			continue
		}
		if m := reDurDist.FindStringSubmatch(line); m != nil {
			cur.DurationS, _ = parseDuration(m[1])
			if v, ok := parseNum(m[2]); ok {
				cur.DistanceM = v * 1000
			}
			continue
		}
		if m := reSpeedHrCal.FindStringSubmatch(line); m != nil {
			cur.AvgSpeedKmh = f64ptr(m[1])
			cur.AvgHR = intptr(m[2])
			cur.Calories = intptr(m[3])
			continue
		}
		if m := reLabelSport.FindStringSubmatch(line); m != nil {
			cur.LabelID = m[1]
			st, _ := parseInt(m[2])
			cur.SportType = st
			continue
		}
	}
	if cur != nil {
		out = append(out, *cur)
	}
	for i := range out {
		out[i].Raw = text
	}
	return out
}

// ---------- getActivityDetail ----------

var (
	reWorkoutTime   = regexp.MustCompile(`^Workout Time:\s*(\S+)`)
	reDistance      = regexp.MustCompile(`^Distance:\s*([\d.,]+)\s*km`)
	reTotalTime     = regexp.MustCompile(`^Total Time:\s*(\S+)`)
	reCalories      = regexp.MustCompile(`^Calories:\s*([\d,]+)\s*kcal`)
	reAvgSpeed      = regexp.MustCompile(`^Average Speed:\s*([\d.]+)\s*km/h`)
	reMaxSpeed      = regexp.MustCompile(`^Max Speed:\s*([\d.]+)\s*km/h`)
	reAvgHR         = regexp.MustCompile(`^Average Heart Rate:\s*(\d+)\s*bpm`)
	reElev          = regexp.MustCompile(`^Elevation Gain / Loss:\s*([\d.]+)\s*m\s*/\s*([\d.]+)\s*m`)
	reTrainingLoad  = regexp.MustCompile(`^Training Load:\s*([\d.]+)`)
	reAerobicTE     = regexp.MustCompile(`^Aerobic TE:\s*([\d.]+)`)
	reAnaerobicTE   = regexp.MustCompile(`^Anaerobic TE:\s*([\d.]+)`)
	reTrainingFocus = regexp.MustCompile(`^Training Focus:\s*(.+)$`)
	rePerformance   = regexp.MustCompile(`^Performance:\s*(-?\d+)`)
)

// ParseActivityDetail parses the text output of getActivityDetail.
func ParseActivityDetail(text string) (ActivityDetail, error) {
	text = trimQuotes(text)
	var d ActivityDetail
	found := false
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if m := reWorkoutTime.FindStringSubmatch(line); m != nil {
			d.DurationS, _ = parseDuration(m[1])
			found = true
		} else if m := reDistance.FindStringSubmatch(line); m != nil {
			if v, ok := parseNum(m[1]); ok {
				d.DistanceM = v * 1000
				found = true
			}
		} else if m := reTotalTime.FindStringSubmatch(line); m != nil {
			d.TotalTimeS, _ = parseDuration(m[1])
		} else if m := reCalories.FindStringSubmatch(line); m != nil {
			d.Calories = intptr(m[1])
		} else if m := reAvgSpeed.FindStringSubmatch(line); m != nil {
			d.AvgSpeedKmh = f64ptr(m[1])
		} else if m := reMaxSpeed.FindStringSubmatch(line); m != nil {
			d.MaxSpeedKmh = f64ptr(m[1])
		} else if m := reAvgHR.FindStringSubmatch(line); m != nil {
			d.AvgHR = intptr(m[1])
		} else if m := reElev.FindStringSubmatch(line); m != nil {
			d.ElevGainM = f64ptr(m[1])
			d.ElevLossM = f64ptr(m[2])
		} else if m := reTrainingLoad.FindStringSubmatch(line); m != nil {
			d.TrainingLoad = f64ptr(m[1])
		} else if m := reAerobicTE.FindStringSubmatch(line); m != nil {
			d.AerobicTE = f64ptr(m[1])
		} else if m := reAnaerobicTE.FindStringSubmatch(line); m != nil {
			d.AnaerobicTE = f64ptr(m[1])
		} else if m := reTrainingFocus.FindStringSubmatch(line); m != nil {
			d.TrainingFocus = strptr(m[1])
		} else if m := rePerformance.FindStringSubmatch(line); m != nil {
			d.Performance = intptr(m[1])
		}
	}
	if !found {
		return d, fmt.Errorf("coros: unrecognized activity detail output")
	}
	d.Raw = text
	return d, nil
}

// ---------- queryActivityLapData (JSON) ----------

type lapGroupsEnvelope struct {
	LapGroups []struct {
		Type int             `json:"type"`
		Laps []json.RawMessage `json:"laps"`
	} `json:"lapGroups"`
}

type lapIndexOnly struct {
	LapIndex int `json:"lapIndex"`
}

// ParseLaps extracts every lap of every lap group from the JSON output
// of queryActivityLapData.
func ParseLaps(raw []byte) ([]LapData, error) {
	var env lapGroupsEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, fmt.Errorf("coros: parse laps: %w", err)
	}
	var out []LapData
	for _, g := range env.LapGroups {
		for _, lapRaw := range g.Laps {
			var idx lapIndexOnly
			if err := json.Unmarshal(lapRaw, &idx); err != nil {
				continue
			}
			out = append(out, LapData{GroupType: g.Type, LapIndex: idx.LapIndex, Data: lapRaw})
		}
	}
	return out, nil
}

// ---------- queryDailyHealthData ----------

var (
	reDaySection  = regexp.MustCompile(`^---\s*(\d{8})\s*---`)
	reSteps       = regexp.MustCompile(`Steps:\s*([\d,]+)`)
	reHealthCal   = regexp.MustCompile(`Calories:\s*([\d,]+)\s*kcal`)
	reExercise    = regexp.MustCompile(`Exercise:\s*(.+)$`)
	reSleepSum    = regexp.MustCompile(`Total:\s*([^|]+?)\s*\|\s*Deep:\s*([^|]+?)\s*\|\s*Light:\s*([^|]+?)\s*\|\s*REM:\s*([^|]+?)\s*\|\s*Awake:\s*(.+)$`)
	reSleepHR     = regexp.MustCompile(`Sleep HR:\s*Avg\s*(\d+)\s*bpm\s*\|\s*Min\s*(\d+)\s*bpm\s*\|\s*Max\s*(\d+)\s*bpm`)
)

// ParseDailyHealth parses the text output of queryDailyHealthData.
func ParseDailyHealth(text string) []HealthPoint {
	text = trimQuotes(text)
	var out []HealthPoint
	var date time.Time
	inSection := false
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if m := reDaySection.FindStringSubmatch(line); m != nil {
			if d, ok := parseDate8(m[1]); ok {
				date = d
				inSection = true
			}
			continue
		}
		if !inSection {
			continue
		}
		if m := reSteps.FindStringSubmatch(line); m != nil {
			out = append(out, HealthPoint{Date: date, Metric: "steps", Value: f64ptr(m[1])})
		}
		if m := reHealthCal.FindStringSubmatch(line); m != nil {
			out = append(out, HealthPoint{Date: date, Metric: "calories", Value: f64ptr(m[1])})
		}
		if m := reExercise.FindStringSubmatch(line); m != nil {
			out = append(out, HealthPoint{Date: date, Metric: "exercise_min", Value: durMinF64ptr(m[1])})
		}
		if m := reSleepSum.FindStringSubmatch(line); m != nil {
			out = append(out,
				HealthPoint{Date: date, Metric: "sleep_total_min", Value: durMinF64ptr(m[1])},
				HealthPoint{Date: date, Metric: "sleep_deep_min", Value: durMinF64ptr(m[2])},
				HealthPoint{Date: date, Metric: "sleep_light_min", Value: durMinF64ptr(m[3])},
				HealthPoint{Date: date, Metric: "sleep_rem_min", Value: durMinF64ptr(m[4])},
				HealthPoint{Date: date, Metric: "sleep_awake_min", Value: durMinF64ptr(m[5])},
			)
		}
		if m := reSleepHR.FindStringSubmatch(line); m != nil {
			out = append(out,
				HealthPoint{Date: date, Metric: "sleep_hr_avg", Value: intF64ptr(m[1])},
				HealthPoint{Date: date, Metric: "sleep_hr_min", Value: intF64ptr(m[2])},
				HealthPoint{Date: date, Metric: "sleep_hr_max", Value: intF64ptr(m[3])},
			)
		}
	}
	return out
}

// ---------- querySleepOverview ----------

var (
	reDateLine     = regexp.MustCompile(`^(\d{4}-\d{2}-\d{2})$`)
	reSleepScore   = regexp.MustCompile(`^Sleep Score:\s*(\d+)`)
	reDailySleep   = regexp.MustCompile(`^Daily Sleep:\s*([^(]+)`)
	reMainSleep    = regexp.MustCompile(`^Main Sleep \(asleep\):\s*(.+)$`)
	reMainPeriod   = regexp.MustCompile(`^Main Sleep Period \(incl\. awake\):\s*(.+)$`)
	reDeepRatio    = regexp.MustCompile(`^Deep Sleep Ratio:\s*([\d.]+)%`)
	reLightRatio   = regexp.MustCompile(`^Light Sleep Ratio:\s*([\d.]+)%`)
	reRemRatio     = regexp.MustCompile(`^REM Ratio:\s*([\d.]+)%`)
	reAwakeRatio   = regexp.MustCompile(`^Awake Ratio:\s*([\d.]+)%`)
	reAwakeTime    = regexp.MustCompile(`^Awake Time:\s*(.+)$`)
	reAwakeCount   = regexp.MustCompile(`^Awake Count \(>5 min\):\s*(\d+)`)
	reNapsTotal    = regexp.MustCompile(`^Naps Total \(asleep\):\s*(.+)$`)
	reNapsPeriod   = regexp.MustCompile(`^Naps Period \(incl\. awake\):\s*(.+)$`)
)

// ParseSleepOverview parses the text output of querySleepOverview.
func ParseSleepOverview(text string) []HealthPoint {
	text = trimQuotes(text)
	var out []HealthPoint
	var date time.Time
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if m := reDateLine.FindStringSubmatch(line); m != nil {
			if d, ok := parseDate10(m[1]); ok {
				date = d
			}
			continue
		}
		if date.IsZero() {
			continue
		}
		add := func(metric string, v *float64) {
			if v != nil {
				out = append(out, HealthPoint{Date: date, Metric: metric, Value: v})
			}
		}
		if m := reSleepScore.FindStringSubmatch(line); m != nil {
			add("sleep_score", f64ptr(m[1]))
		} else if m := reDailySleep.FindStringSubmatch(line); m != nil {
			if v := durPtr(m[1]); v != nil {
				f := float64(*v) / 60
				add("sleep_daily_min", &f)
			}
		} else if m := reMainSleep.FindStringSubmatch(line); m != nil {
			if v := durPtr(m[1]); v != nil {
				f := float64(*v) / 60
				add("sleep_main_min", &f)
			}
		} else if m := reMainPeriod.FindStringSubmatch(line); m != nil {
			if v := durPtr(m[1]); v != nil {
				f := float64(*v) / 60
				add("sleep_main_period_min", &f)
			}
		} else if m := reDeepRatio.FindStringSubmatch(line); m != nil {
			add("sleep_deep_ratio", f64ptr(m[1]))
		} else if m := reLightRatio.FindStringSubmatch(line); m != nil {
			add("sleep_light_ratio", f64ptr(m[1]))
		} else if m := reRemRatio.FindStringSubmatch(line); m != nil {
			add("sleep_rem_ratio", f64ptr(m[1]))
		} else if m := reAwakeRatio.FindStringSubmatch(line); m != nil {
			add("sleep_awake_ratio", f64ptr(m[1]))
		} else if m := reAwakeTime.FindStringSubmatch(line); m != nil {
			if v := durPtr(m[1]); v != nil {
				f := float64(*v) / 60
				add("sleep_awake_time_min", &f)
			}
		} else if m := reAwakeCount.FindStringSubmatch(line); m != nil {
			add("sleep_awake_count", f64ptr(m[1]))
		} else if m := reNapsTotal.FindStringSubmatch(line); m != nil {
			if v := durPtr(m[1]); v != nil {
				f := float64(*v) / 60
				add("nap_min", &f)
			}
		} else if m := reNapsPeriod.FindStringSubmatch(line); m != nil {
			if v := durPtr(m[1]); v != nil {
				f := float64(*v) / 60
				add("nap_period_min", &f)
			}
		}
	}
	return out
}

// ---------- querySleepHrv (assessment section) ----------

var (
	reHrvDate   = regexp.MustCompile(`^(\d{4}-\d{2}-\d{2}):$`)
	reHrvAvg    = regexp.MustCompile(`HRV Avg:\s*([\d.]+)\s*ms`)
	reHrvRange  = regexp.MustCompile(`Normal Range:\s*([\d.]+)\s*-\s*([\d.]+)\s*ms`)
	reHrvBase   = regexp.MustCompile(`Baseline:\s*([\d.]+)\s*ms`)
)

// ParseSleepHrv parses the assessment section of querySleepHrv.
func ParseSleepHrv(text string) []HealthPoint {
	text = trimQuotes(text)
	var out []HealthPoint
	var date time.Time
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if m := reHrvDate.FindStringSubmatch(line); m != nil {
			if d, ok := parseDate10(m[1]); ok {
				date = d
			}
			continue
		}
		if date.IsZero() {
			continue
		}
		if m := reHrvAvg.FindStringSubmatch(line); m != nil {
			out = append(out, HealthPoint{Date: date, Metric: "hrv_avg", Value: f64ptr(m[1])})
		} else if m := reHrvRange.FindStringSubmatch(line); m != nil {
			out = append(out,
				HealthPoint{Date: date, Metric: "hrv_normal_low", Value: f64ptr(m[1])},
				HealthPoint{Date: date, Metric: "hrv_normal_high", Value: f64ptr(m[2])},
			)
		} else if m := reHrvBase.FindStringSubmatch(line); m != nil {
			out = append(out, HealthPoint{Date: date, Metric: "hrv_baseline", Value: f64ptr(m[1])})
		}
	}
	return out
}

// ---------- queryRestingHeartRate / queryStressLevel ----------

var reDatedValue = regexp.MustCompile(`^(\d{4}-\d{2}-\d{2}):\s*(\d+(?:\.\d+)?)\s*(bpm)?$`)

// ParseDatedValues parses outputs of the form "YYYY-MM-DD: 51 bpm".
// metric is the name to store under.
func ParseDatedValues(text, metric string) []HealthPoint {
	text = trimQuotes(text)
	var out []HealthPoint
	for _, line := range strings.Split(text, "\n") {
		if m := reDatedValue.FindStringSubmatch(strings.TrimSpace(line)); m != nil {
			if d, ok := parseDate10(m[1]); ok {
				out = append(out, HealthPoint{Date: d, Metric: metric, Value: f64ptr(m[2])})
			}
		}
	}
	return out
}

// ---------- queryRecoveryStatus ----------

var (
	reRecoveryPct  = regexp.MustCompile(`^Recovery:\s*([\d.]+)%`)
	reRecoveryLvl = regexp.MustCompile(`^Level:\s*(.+)$`)
	reRecoveryHrs = regexp.MustCompile(`^Estimated Full Recovery:\s*([\d.]+)h`)
)

// ParseRecovery parses queryRecoveryStatus; values are attributed to today.
func ParseRecovery(text string, today time.Time) []HealthPoint {
	text = trimQuotes(text)
	var out []HealthPoint
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if m := reRecoveryPct.FindStringSubmatch(line); m != nil {
			out = append(out, HealthPoint{Date: today, Metric: "recovery_pct", Value: f64ptr(m[1])})
		} else if m := reRecoveryLvl.FindStringSubmatch(line); m != nil {
			out = append(out, HealthPoint{Date: today, Metric: "recovery_level", ValueText: strptr(m[1])})
		} else if m := reRecoveryHrs.FindStringSubmatch(line); m != nil {
			out = append(out, HealthPoint{Date: today, Metric: "recovery_full_h", Value: f64ptr(m[1])})
		}
	}
	return out
}

// ---------- queryTrainingLoadAssessment ----------

var (
	reLoadShort  = regexp.MustCompile(`^Short-Term Load:\s*([\d.]+)`)
	reLoadLong   = regexp.MustCompile(`^Long-Term Load:\s*([\d.]+)`)
	reLoadRatio  = regexp.MustCompile(`^Load Ratio:\s*([\d.]+)`)
	reLoadComm   = regexp.MustCompile(`^Comment:\s*(.+)$`)
)

// ParseTrainingLoad parses queryTrainingLoadAssessment.
func ParseTrainingLoad(text string) []HealthPoint {
	text = trimQuotes(text)
	var out []HealthPoint
	var date time.Time
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if m := reDateLine.FindStringSubmatch(line); m != nil {
			if d, ok := parseDate10(m[1]); ok {
				date = d
			}
			continue
		}
		if date.IsZero() {
			continue
		}
		if m := reLoadComm.FindStringSubmatch(line); m != nil {
			out = append(out, HealthPoint{Date: date, Metric: "load_comment", ValueText: strptr(m[1])})
		} else if m := reLoadShort.FindStringSubmatch(line); m != nil {
			out = append(out, HealthPoint{Date: date, Metric: "load_short", Value: f64ptr(m[1])})
		} else if m := reLoadLong.FindStringSubmatch(line); m != nil {
			out = append(out, HealthPoint{Date: date, Metric: "load_long", Value: f64ptr(m[1])})
		} else if m := reLoadRatio.FindStringSubmatch(line); m != nil {
			out = append(out, HealthPoint{Date: date, Metric: "load_ratio", Value: f64ptr(m[1])})
		}
	}
	return out
}

// ---------- queryFitnessAssessmentOverview ----------

var (
	reVO2max       = regexp.MustCompile(`^VO2max:\s*([\d.]+)`)
	reRunningLevel = regexp.MustCompile(`^Running Level:\s*([\d.]+)`)
	reThresholdPace = regexp.MustCompile(`^Threshold Pace:\s*(.+)$`)
)

// ParseFitnessOverview parses queryFitnessAssessmentOverview.
func ParseFitnessOverview(text string) (FitnessOverview, error) {
	text = trimQuotes(text)
	var f FitnessOverview
	found := false
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if m := reVO2max.FindStringSubmatch(line); m != nil {
			f.VO2max = f64ptr(m[1])
			found = true
		} else if m := reRunningLevel.FindStringSubmatch(line); m != nil {
			f.RunningLevel = f64ptr(m[1])
		} else if m := reThresholdPace.FindStringSubmatch(line); m != nil {
			f.ThresholdPace = strptr(m[1])
		}
	}
	if !found {
		return f, fmt.Errorf("coros: unrecognized fitness overview output")
	}
	f.Raw = text
	return f, nil
}

// ---------- queryUserInfo ----------

var (
	reHeight   = regexp.MustCompile(`^Height:\s*([\d.]+)\s*cm`)
	reWeight   = regexp.MustCompile(`^Weight:\s*([\d.]+)\s*kg`)
	reBirthday = regexp.MustCompile(`^Birthday:\s*(\d{4}-\d{2}-\d{2})`)
	reGender   = regexp.MustCompile(`^Gender:\s*(.+)$`)
	reNickname = regexp.MustCompile(`^Nickname:\s*(.+)$`)
)

// ParseProfile parses queryUserInfo.
func ParseProfile(text string) (Profile, error) {
	text = trimQuotes(text)
	var p Profile
	found := false
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if m := reHeight.FindStringSubmatch(line); m != nil {
			p.HeightCM = f64ptr(m[1])
			found = true
		} else if m := reWeight.FindStringSubmatch(line); m != nil {
			p.WeightKG = f64ptr(m[1])
		} else if m := reBirthday.FindStringSubmatch(line); m != nil {
			if d, ok := parseDate10(m[1]); ok {
				p.Birthday = &d
			}
		} else if m := reGender.FindStringSubmatch(line); m != nil {
			p.Gender = strptr(m[1])
		} else if m := reNickname.FindStringSubmatch(line); m != nil {
			p.Nickname = strptr(m[1])
		}
	}
	if !found {
		return p, fmt.Errorf("coros: unrecognized profile output")
	}
	p.Raw = text
	return p, nil
}
