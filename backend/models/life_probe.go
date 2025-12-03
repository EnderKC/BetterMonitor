package models

import (
	"encoding/json"
	"errors"
	"math"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	LifeDataTypeHeartRate     = "heart_rate"
	LifeDataTypeStepsDetailed = "steps_detailed"
	LifeDataTypeEnergy        = "energy_detailed"
	LifeDataTypeSleepDetailed = "sleep_detailed"
	LifeDataTypeFocusStatus   = "focus_status"
	LifeDataTypeScreenEvent   = "screen_event"
)

// LifeProbe represents a wearable/phone probe device
type LifeProbe struct {
	gorm.Model
	Name              string     `json:"name" gorm:"not null"`
	DeviceID          string     `json:"device_id" gorm:"uniqueIndex;not null"`
	Description       string     `json:"description"`
	Tags              string     `json:"tags"`
	AllowPublicView   bool       `json:"allow_public_view" gorm:"default:true"`
	LastSyncAt        *time.Time `json:"last_sync_at"`
	BatteryLevel      *float64   `json:"battery_level"`
	LatestHeartRate   float64    `json:"latest_heart_rate"`
	LatestHeartRateAt *time.Time `json:"latest_heart_rate_at"`
	LatestFocusStatus *bool      `json:"latest_focus_status"`
	LatestFocusReason string     `json:"latest_focus_reason"`
	FocusUpdatedAt    *time.Time `json:"focus_updated_at"`
}

// LifeLoggerEvent stores raw incoming payloads for auditing
type LifeLoggerEvent struct {
	gorm.Model
	LifeProbeID  uint            `json:"life_probe_id" gorm:"index"`
	EventID      string          `json:"event_id" gorm:"uniqueIndex;not null"`
	DeviceID     string          `json:"device_id" gorm:"index"`
	DataType     string          `json:"data_type" gorm:"index"`
	Timestamp    time.Time       `json:"timestamp" gorm:"index"`
	BatteryLevel *float64        `json:"battery_level"`
	Payload      json.RawMessage `json:"payload" gorm:"type:json"`
}

// LifeHeartRate stores parsed heart rate metrics
type LifeHeartRate struct {
	gorm.Model
	LifeProbeID uint      `json:"life_probe_id" gorm:"index"`
	EventID     string    `json:"event_id" gorm:"index"`
	MeasureTime time.Time `json:"measure_time" gorm:"index"`
	Value       float64   `json:"value"`
	Unit        string    `json:"unit"`
}

// LifeStepSample stores each segmented step/energy value
type LifeStepSample struct {
	gorm.Model
	LifeProbeID uint      `json:"life_probe_id" gorm:"index"`
	EventID     string    `json:"event_id" gorm:"index"`
	SampleType  string    `json:"sample_type" gorm:"index"`
	StartTime   time.Time `json:"start_time" gorm:"index"`
	EndTime     time.Time `json:"end_time" gorm:"index"`
	Value       float64   `json:"value"`
}

// LifeStepDailyTotal aggregates steps per day for quick lookups
type LifeStepDailyTotal struct {
	gorm.Model
	LifeProbeID uint      `json:"life_probe_id" gorm:"uniqueIndex:idx_probe_day_type"`
	Day         time.Time `json:"day" gorm:"uniqueIndex:idx_probe_day_type"`
	SampleType  string    `json:"sample_type" gorm:"uniqueIndex:idx_probe_day_type"`
	Total       float64   `json:"total"`
}

// LifeSleepSegment stores sleep session segments
type LifeSleepSegment struct {
	gorm.Model
	LifeProbeID uint      `json:"life_probe_id" gorm:"index"`
	EventID     string    `json:"event_id" gorm:"index"`
	Stage       string    `json:"stage"`
	StartTime   time.Time `json:"start_time"`
	EndTime     time.Time `json:"end_time"`
	Duration    float64   `json:"duration"`
}

// LifeFocusEvent stores focus mode toggles
type LifeFocusEvent struct {
	gorm.Model
	LifeProbeID  uint      `json:"life_probe_id" gorm:"index"`
	EventID      string    `json:"event_id" gorm:"index"`
	IsFocused    bool      `json:"is_focused"`
	ChangeReason string    `json:"change_reason"`
	EventTime    time.Time `json:"event_time"`
}

// LifeScreenEvent stores lock/unlock events
type LifeScreenEvent struct {
	gorm.Model
	LifeProbeID uint      `json:"life_probe_id" gorm:"index"`
	EventID     string    `json:"event_id" gorm:"index"`
	Action      string    `json:"action"`
	EventTime   time.Time `json:"event_time"`
}

// Payload definitions --------------------------------------------------------

type HeartRatePayload struct {
	Value       float64   `json:"value"`
	Unit        string    `json:"unit"`
	MeasureTime time.Time `json:"measure_time"`
}

type StepSamplePayload struct {
	Value     float64   `json:"value"`
	StartTime time.Time `json:"start_time"`
	EndTime   time.Time `json:"end_time"`
}

type StepsDetailedPayload struct {
	StartPeriod     time.Time           `json:"start_period"`
	EndPeriod       time.Time           `json:"end_period"`
	Samples         []StepSamplePayload `json:"samples"`
	TotalCount      float64             `json:"total_count"`
	TodayTotalSteps *float64            `json:"today_total_steps"`
}

type SleepSegmentPayload struct {
	Stage     string    `json:"stage"`
	StartTime time.Time `json:"start_time"`
	EndTime   time.Time `json:"end_time"`
	Duration  float64   `json:"duration"`
}

type SleepDetailedPayload struct {
	IsSleepSessionFinished bool                  `json:"is_sleep_session_finished"`
	Segments               []SleepSegmentPayload `json:"segments"`
	TodayTotalSleepHours   *float64              `json:"today_total_sleep_hours"`
}

type FocusStatusPayload struct {
	IsFocused    bool   `json:"is_focused"`
	ChangeReason string `json:"change_reason"`
}

type ScreenEventPayload struct {
	Action    string    `json:"action"`
	EventTime time.Time `json:"event_time"`
}

// Summary & detail DTOs ------------------------------------------------------

type HeartRatePoint struct {
	Time  time.Time `json:"time"`
	Value float64   `json:"value"`
}

type StepSamplePoint struct {
	StartTime  time.Time `json:"start"`
	EndTime    time.Time `json:"end"`
	Value      float64   `json:"value"`
	SampleType string    `json:"sample_type"`
}

type FocusEventPoint struct {
	Time         time.Time `json:"time"`
	IsFocused    bool      `json:"is_focused"`
	ChangeReason string    `json:"change_reason"`
}

type ScreenEventPoint struct {
	Time   time.Time `json:"time"`
	Action string    `json:"action"`
}

type SleepSegmentPoint struct {
	Stage     string    `json:"stage"`
	StartTime time.Time `json:"start_time"`
	EndTime   time.Time `json:"end_time"`
	Duration  float64   `json:"duration"`
}

type SleepOverview struct {
	TotalDuration  float64            `json:"total_duration"`
	StageDurations map[string]float64 `json:"stage_durations"`
	StartTime      *time.Time         `json:"start_time"`
	EndTime        *time.Time         `json:"end_time"`
}

type DailyStepPoint struct {
	Day        time.Time `json:"day"`
	Total      float64   `json:"total"`
	SampleType string    `json:"sample_type"`
}

type LifeProbeSummary struct {
	ID              uint             `json:"id"`
	Name            string           `json:"name"`
	DeviceID        string           `json:"device_id"`
	Description     string           `json:"description"`
	Tags            string           `json:"tags"`
	AllowPublicView bool             `json:"allow_public_view"`
	BatteryLevel    *float64         `json:"battery_level"`
	LastSyncAt      *time.Time       `json:"last_sync_at"`
	LatestHeartRate *HeartRatePoint  `json:"latest_heart_rate"`
	FocusEvent      *FocusEventPoint `json:"focus_event"`
	StepsToday      float64          `json:"steps_today"`
	DailyTotals     []DailyStepPoint `json:"daily_totals"`
}

type LifeProbeDetails struct {
	Summary      *LifeProbeSummary   `json:"summary"`
	HeartRates   []HeartRatePoint    `json:"heart_rates"`
	StepSamples  []StepSamplePoint   `json:"step_samples"`
	FocusEvents  []FocusEventPoint   `json:"focus_events"`
	ScreenEvents []ScreenEventPoint  `json:"screen_events"`
	Sleep        []SleepSegmentPoint `json:"sleep_segments"`
	SleepStats   SleepOverview       `json:"sleep_overview"`
}

// CRUD helpers ----------------------------------------------------------------

func CreateLifeProbe(probe *LifeProbe) error {
	return DB.Create(probe).Error
}

func UpdateLifeProbe(probe *LifeProbe) error {
	return DB.Save(probe).Error
}

func DeleteLifeProbe(id uint) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("life_probe_id = ?", id).Delete(&LifeHeartRate{}).Error; err != nil {
			return err
		}
		if err := tx.Where("life_probe_id = ?", id).Delete(&LifeStepSample{}).Error; err != nil {
			return err
		}
		if err := tx.Where("life_probe_id = ?", id).Delete(&LifeStepDailyTotal{}).Error; err != nil {
			return err
		}
		if err := tx.Where("life_probe_id = ?", id).Delete(&LifeSleepSegment{}).Error; err != nil {
			return err
		}
		if err := tx.Where("life_probe_id = ?", id).Delete(&LifeFocusEvent{}).Error; err != nil {
			return err
		}
		if err := tx.Where("life_probe_id = ?", id).Delete(&LifeScreenEvent{}).Error; err != nil {
			return err
		}
		if err := tx.Where("life_probe_id = ?", id).Delete(&LifeLoggerEvent{}).Error; err != nil {
			return err
		}
		return tx.Delete(&LifeProbe{}, id).Error
	})
}

func GetLifeProbeByID(id uint) (*LifeProbe, error) {
	var probe LifeProbe
	if err := DB.First(&probe, id).Error; err != nil {
		return nil, err
	}
	return &probe, nil
}

func GetLifeProbeByDeviceID(deviceID string) (*LifeProbe, error) {
	var probe LifeProbe
	if err := DB.Where("device_id = ?", deviceID).First(&probe).Error; err != nil {
		return nil, err
	}
	return &probe, nil
}

func ListLifeProbes() ([]LifeProbe, error) {
	var probes []LifeProbe
	if err := DB.Order("created_at asc").Find(&probes).Error; err != nil {
		return nil, err
	}
	return probes, nil
}

func ListPublicLifeProbes() ([]LifeProbe, error) {
	var probes []LifeProbe
	if err := DB.Where("allow_public_view = ?", true).Order("created_at asc").Find(&probes).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return []LifeProbe{}, nil
		}
		return nil, err
	}
	return probes, nil
}

// Event persistence helpers ---------------------------------------------------

func CreateLifeLoggerEvent(db *gorm.DB, event *LifeLoggerEvent) error {
	return db.Create(event).Error
}

func UpdateProbeSyncInfo(db *gorm.DB, probeID uint, eventTime time.Time, battery *float64) error {
	updates := map[string]interface{}{
		"last_sync_at": eventTime,
	}
	if battery != nil {
		updates["battery_level"] = battery
	}
	return db.Model(&LifeProbe{}).Where("id = ?", probeID).Updates(updates).Error
}

func UpdateProbeHeartRate(db *gorm.DB, probeID uint, value float64, measureTime time.Time) error {
	updates := map[string]interface{}{
		"latest_heart_rate":    normalizeHeartRate(value),
		"latest_heart_rate_at": measureTime,
	}
	return db.Model(&LifeProbe{}).Where("id = ?", probeID).Updates(updates).Error
}

func UpdateProbeFocusStatus(db *gorm.DB, probeID uint, isFocused bool, reason string, eventTime time.Time) error {
	return db.Model(&LifeProbe{}).Where("id = ?", probeID).Updates(map[string]interface{}{
		"latest_focus_status": &isFocused,
		"latest_focus_reason": reason,
		"focus_updated_at":    eventTime,
	}).Error
}

func normalizeHeartRate(value float64) float64 {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return 0
	}
	return math.Round(value)
}

func RecordHeartRate(db *gorm.DB, probeID uint, eventID string, payload HeartRatePayload) error {
	record := LifeHeartRate{
		LifeProbeID: probeID,
		EventID:     eventID,
		MeasureTime: payload.MeasureTime,
		Value:       normalizeHeartRate(payload.Value),
		Unit:        payload.Unit,
	}
	return db.Create(&record).Error
}

func RecordStepSamples(db *gorm.DB, probeID uint, eventID, sampleType string, samples []StepSamplePayload) error {
	if len(samples) == 0 {
		return nil
	}

	records := make([]LifeStepSample, 0, len(samples))
	for _, sample := range samples {
		records = append(records, LifeStepSample{
			LifeProbeID: probeID,
			EventID:     eventID,
			SampleType:  sampleType,
			StartTime:   sample.StartTime,
			EndTime:     sample.EndTime,
			Value:       sample.Value,
		})
	}
	return db.Create(&records).Error
}

func incrementDailyTotal(db *gorm.DB, probeID uint, day time.Time, sampleType string, delta float64) error {
	if delta == 0 {
		return nil
	}

	record := LifeStepDailyTotal{
		LifeProbeID: probeID,
		Day:         day.UTC(),
		SampleType:  sampleType,
		Total:       delta,
	}

	return db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "life_probe_id"}, {Name: "day"}, {Name: "sample_type"}},
		DoUpdates: clause.Assignments(map[string]interface{}{"total": gorm.Expr("total + ?", delta)}),
	}).Create(&record).Error
}

func RecordDailyTotals(db *gorm.DB, probeID uint, sampleType string, samples []StepSamplePayload) error {
	for _, sample := range samples {
		for _, part := range splitStepSampleByDay(sample) {
			if err := incrementDailyTotal(db, probeID, part.Day, sampleType, part.Value); err != nil {
				return err
			}
		}
	}
	return nil
}

func RecordSleepSegments(db *gorm.DB, probeID uint, eventID string, segments []SleepSegmentPayload) error {
	if len(segments) == 0 {
		return nil
	}

	rows := make([]LifeSleepSegment, 0, len(segments))
	for _, seg := range segments {
		rows = append(rows, LifeSleepSegment{
			LifeProbeID: probeID,
			EventID:     eventID,
			Stage:       seg.Stage,
			StartTime:   seg.StartTime,
			EndTime:     seg.EndTime,
			Duration:    seg.Duration,
		})
	}
	return db.Create(&rows).Error
}

func RecordFocusEvent(db *gorm.DB, probeID uint, eventID string, payload FocusStatusPayload, eventTime time.Time) error {
	record := LifeFocusEvent{
		LifeProbeID:  probeID,
		EventID:      eventID,
		IsFocused:    payload.IsFocused,
		ChangeReason: payload.ChangeReason,
		EventTime:    eventTime,
	}
	return db.Create(&record).Error
}

func RecordScreenEvent(db *gorm.DB, probeID uint, eventID string, payload ScreenEventPayload) error {
	record := LifeScreenEvent{
		LifeProbeID: probeID,
		EventID:     eventID,
		Action:      payload.Action,
		EventTime:   payload.EventTime,
	}
	return db.Create(&record).Error
}

// Query helpers ---------------------------------------------------------------

func truncateToDay(t time.Time) time.Time {
	loc := t.Location()
	if loc == nil {
		loc = time.UTC
	}
	localTime := t.In(loc)
	localMidnight := time.Date(localTime.Year(), localTime.Month(), localTime.Day(), 0, 0, 0, 0, loc)
	return localMidnight.UTC()
}

type stepDayPart struct {
	Day   time.Time
	Value float64
}

func splitStepSampleByDay(sample StepSamplePayload) []stepDayPart {
	start := sample.StartTime
	end := sample.EndTime
	if !end.After(start) {
		return []stepDayPart{
			{
				Day:   truncateToDay(start),
				Value: sample.Value,
			},
		}
	}

	totalDuration := end.Sub(start).Seconds()
	if totalDuration <= 0 {
		return []stepDayPart{
			{
				Day:   truncateToDay(start),
				Value: sample.Value,
			},
		}
	}

	var parts []stepDayPart
	current := start
	for current.Before(end) {
		dayStart := truncateToDay(current)
		nextDay := dayStart.Add(24 * time.Hour)
		currentEnd := end
		if nextDay.Before(end) {
			currentEnd = nextDay
		}

		duration := currentEnd.Sub(current).Seconds()
		value := sample.Value * (duration / totalDuration)

		parts = append(parts, stepDayPart{
			Day:   dayStart,
			Value: value,
		})

		current = currentEnd
	}

	return parts
}

func getDailyTotals(probeID uint, days int, reference time.Time) ([]DailyStepPoint, error) {
	if days <= 0 {
		days = 7
	}
	if reference.IsZero() {
		reference = time.Now()
	}
	startDay := truncateToDay(reference.AddDate(0, 0, -(days - 1)))
	var rows []LifeStepDailyTotal
	if err := DB.Where("life_probe_id = ? AND day >= ?", probeID, startDay).
		Order("day asc").
		Find(&rows).Error; err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	points := make([]DailyStepPoint, 0, len(rows))
	for _, row := range rows {
		points = append(points, DailyStepPoint{
			Day:        row.Day,
			Total:      row.Total,
			SampleType: row.SampleType,
		})
	}
	return points, nil
}

func buildSleepOverview(segments []LifeSleepSegment) SleepOverview {
	if len(segments) == 0 {
		return SleepOverview{
			StageDurations: map[string]float64{},
		}
	}

	stageDurations := make(map[string]float64)
	var start *time.Time
	var end *time.Time
	var total float64

	for _, seg := range segments {
		stageDurations[seg.Stage] += seg.Duration
		total += seg.Duration
		if start == nil || seg.StartTime.Before(*start) {
			tmp := seg.StartTime
			start = &tmp
		}
		if end == nil || seg.EndTime.After(*end) {
			tmp := seg.EndTime
			end = &tmp
		}
	}

	return SleepOverview{
		TotalDuration:  total,
		StageDurations: stageDurations,
		StartTime:      start,
		EndTime:        end,
	}
}

func getLatestSleepSegments(probeID uint) ([]LifeSleepSegment, error) {
	var event LifeLoggerEvent
	if err := DB.Where("life_probe_id = ? AND data_type = ?", probeID, LifeDataTypeSleepDetailed).
		Order("timestamp desc").
		First(&event).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}

	var segments []LifeSleepSegment
	if err := DB.Where("life_probe_id = ? AND event_id = ?", probeID, event.EventID).
		Order("start_time asc").
		Find(&segments).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return []LifeSleepSegment{}, nil
		}
		return nil, err
	}
	return segments, nil
}

func BuildLifeProbeSummary(probe *LifeProbe, now time.Time, includeDailyTotals bool) (*LifeProbeSummary, error) {
	summary := &LifeProbeSummary{
		ID:              probe.ID,
		Name:            probe.Name,
		DeviceID:        probe.DeviceID,
		Description:     probe.Description,
		Tags:            probe.Tags,
		AllowPublicView: probe.AllowPublicView,
		BatteryLevel:    probe.BatteryLevel,
		LastSyncAt:      probe.LastSyncAt,
	}

	referenceTime := now
	if probe.LastSyncAt != nil {
		referenceTime = *probe.LastSyncAt
	}

	if probe.LatestHeartRateAt != nil {
		cpy := *probe.LatestHeartRateAt
		summary.LatestHeartRate = &HeartRatePoint{
			Time:  cpy,
			Value: probe.LatestHeartRate,
		}
	}

	if probe.LatestFocusStatus != nil && probe.FocusUpdatedAt != nil {
		cpy := *probe.FocusUpdatedAt
		summary.FocusEvent = &FocusEventPoint{
			Time:         cpy,
			IsFocused:    *probe.LatestFocusStatus,
			ChangeReason: probe.LatestFocusReason,
		}
	}

	day := truncateToDay(referenceTime)
	var total LifeStepDailyTotal
	if err := DB.Where("life_probe_id = ? AND day = ? AND sample_type = ?", probe.ID, day, LifeDataTypeStepsDetailed).
		First(&total).Error; err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
	} else {
		summary.StepsToday = total.Total
	}

	if includeDailyTotals {
		totals, err := getDailyTotals(probe.ID, 7, referenceTime)
		if err != nil {
			return nil, err
		}
		summary.DailyTotals = totals
	}

	return summary, nil
}

func GetLifeProbeDetails(probeID uint, start, end time.Time, dailyDays int) (*LifeProbeDetails, error) {
	probe, err := GetLifeProbeByID(probeID)
	if err != nil {
		return nil, err
	}

	if end.Before(start) {
		end = start.Add(24 * time.Hour)
	}

	summary, err := BuildLifeProbeSummary(probe, end, false)
	if err != nil {
		return nil, err
	}

	details := &LifeProbeDetails{
		Summary: summary,
	}

	var heartRows []LifeHeartRate
	if err := DB.Where("life_probe_id = ? AND measure_time BETWEEN ? AND ?", probeID, start, end).
		Order("measure_time asc").
		Find(&heartRows).Error; err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	for _, row := range heartRows {
		details.HeartRates = append(details.HeartRates, HeartRatePoint{
			Time:  row.MeasureTime,
			Value: row.Value,
		})
	}

	var stepRows []LifeStepSample
	if err := DB.Where("life_probe_id = ? AND start_time BETWEEN ? AND ?", probeID, start, end).
		Order("start_time asc").
		Find(&stepRows).Error; err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	for _, row := range stepRows {
		details.StepSamples = append(details.StepSamples, StepSamplePoint{
			StartTime:  row.StartTime,
			EndTime:    row.EndTime,
			Value:      row.Value,
			SampleType: row.SampleType,
		})
	}

	var focusRows []LifeFocusEvent
	if err := DB.Where("life_probe_id = ? AND event_time BETWEEN ? AND ?", probeID, start, end).
		Order("event_time asc").
		Find(&focusRows).Error; err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	for _, row := range focusRows {
		details.FocusEvents = append(details.FocusEvents, FocusEventPoint{
			Time:         row.EventTime,
			IsFocused:    row.IsFocused,
			ChangeReason: row.ChangeReason,
		})
	}

	var screenRows []LifeScreenEvent
	if err := DB.Where("life_probe_id = ? AND event_time BETWEEN ? AND ?", probeID, start, end).
		Order("event_time asc").
		Find(&screenRows).Error; err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	for _, row := range screenRows {
		details.ScreenEvents = append(details.ScreenEvents, ScreenEventPoint{
			Time:   row.EventTime,
			Action: row.Action,
		})
	}

	segments, err := getLatestSleepSegments(probeID)
	if err != nil {
		return nil, err
	}
	for _, seg := range segments {
		details.Sleep = append(details.Sleep, SleepSegmentPoint{
			Stage:     seg.Stage,
			StartTime: seg.StartTime,
			EndTime:   seg.EndTime,
			Duration:  seg.Duration,
		})
	}
	details.SleepStats = buildSleepOverview(segments)

	if dailyDays <= 0 {
		dailyDays = 7
	}
	reference := end
	if summary != nil && summary.LastSyncAt != nil {
		reference = *summary.LastSyncAt
	}
	if totals, err := getDailyTotals(probeID, dailyDays, reference); err == nil {
		details.Summary.DailyTotals = totals
	}

	return details, nil
}

// Cleanup --------------------------------------------------------------------

func DeleteLifeDataBefore(before time.Time) error {
	cutoffDay := truncateToDay(before)

	tables := []struct {
		model interface{}
		query string
		args  []interface{}
	}{
		{&LifeHeartRate{}, "measure_time < ?", []interface{}{before}},
		{&LifeStepSample{}, "end_time < ?", []interface{}{before}},
		{&LifeStepDailyTotal{}, "day < ?", []interface{}{cutoffDay}},
		{&LifeSleepSegment{}, "end_time < ?", []interface{}{before}},
		{&LifeFocusEvent{}, "event_time < ?", []interface{}{before}},
		{&LifeScreenEvent{}, "event_time < ?", []interface{}{before}},
		{&LifeLoggerEvent{}, "timestamp < ?", []interface{}{before}},
	}

	return DB.Transaction(func(tx *gorm.DB) error {
		for _, table := range tables {
			if err := tx.Where(table.query, table.args...).Delete(table.model).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func NormalizeLifeStepDailyTotals() error {
	const batchSize = 500
	var totals []LifeStepDailyTotal

	return DB.Model(&LifeStepDailyTotal{}).FindInBatches(&totals, batchSize, func(tx *gorm.DB, batch int) error {
		for _, row := range totals {
			newDay := truncateToDay(row.Day)
			if row.Day.Equal(newDay) {
				continue
			}
			if err := DB.Model(&LifeStepDailyTotal{}).Where("id = ?", row.ID).Update("day", newDay).Error; err != nil {
				return err
			}
		}
		return nil
	}).Error
}
