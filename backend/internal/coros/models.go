package coros

import "time"

// ActivitySummary is one entry from querySportRecords.
type ActivitySummary struct {
	LabelID       string
	SportType     int
	StartTime     time.Time
	EndTime       time.Time
	DurationS     int
	DistanceM     float64
	AvgSpeedKmh   *float64
	AvgHR         *int
	Calories      *int
	Location      *string
	StartLat      *float64
	StartLon      *float64
	Raw           string
}

// ActivityDetail is the parsed result of getActivityDetail.
type ActivityDetail struct {
	DurationS    int
	TotalTimeS   int
	DistanceM     float64
	Calories      *int
	AvgSpeedKmh   *float64
	MaxSpeedKmh   *float64
	AvgHR        *int
	ElevGainM     *float64
	ElevLossM     *float64
	TrainingLoad  *float64
	AerobicTE     *float64
	AnaerobicTE   *float64
	TrainingFocus *string
	Performance   *int
	Raw           string
}

// HealthPoint is one (date, metric) observation parsed from health tools.
type HealthPoint struct {
	Date      time.Time
	Metric    string
	Value     *float64
	ValueText *string
}

// FitnessOverview is the parsed fitness assessment snapshot.
type FitnessOverview struct {
	VO2max         *float64
	RunningLevel   *float64
	ThresholdPace  *string
	Raw            string
}

// Profile is the parsed user profile.
type Profile struct {
	Nickname  *string
	HeightCM  *float64
	WeightKG  *float64
	Birthday  *time.Time
	Gender    *string
	Raw       string
}
