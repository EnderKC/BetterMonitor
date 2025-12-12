package models

import (
	"encoding/json"
	"errors"
	"math"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// 生命探针支持的数据类型
const (
	LifeDataTypeHeartRate     = "heart_rate"
	LifeDataTypeStepsDetailed = "steps_detailed"
	LifeDataTypeSleepDetailed = "sleep_detailed"
	// 已废弃的数据类型（不再接受新数据）：
	// - "energy_detailed": 能量消耗
	// - "focus_status": 专注模式
	// - "screen_event": 屏幕事件
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
	LifeProbeID uint      `json:"life_probe_id" gorm:"index;uniqueIndex:idx_heart_probe_time"`
	EventID     string    `json:"event_id" gorm:"index"`
	MeasureTime time.Time `json:"measure_time" gorm:"index;uniqueIndex:idx_heart_probe_time"`
	Value       float64   `json:"value"`
	Unit        string    `json:"unit"`
}

// LifeStepSample stores each segmented step/energy value
type LifeStepSample struct {
	gorm.Model
	LifeProbeID uint      `json:"life_probe_id" gorm:"index;uniqueIndex:idx_step_unique"`
	EventID     string    `json:"event_id" gorm:"index"`
	SampleType  string    `json:"sample_type" gorm:"index;uniqueIndex:idx_step_unique"`
	StartTime   time.Time `json:"start_time" gorm:"index;uniqueIndex:idx_step_unique"`
	EndTime     time.Time `json:"end_time" gorm:"index;uniqueIndex:idx_step_unique"`
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
	LifeProbeID uint      `json:"life_probe_id" gorm:"index;uniqueIndex:idx_sleep_unique"`
	EventID     string    `json:"event_id" gorm:"index"`
	Stage       string    `json:"stage" gorm:"uniqueIndex:idx_sleep_unique"`
	StartTime   time.Time `json:"start_time" gorm:"uniqueIndex:idx_sleep_unique"`
	EndTime     time.Time `json:"end_time" gorm:"uniqueIndex:idx_sleep_unique"`
	Duration    float64   `json:"duration"`
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

// SleepDetailedPayload represents sleep session data from client
type SleepDetailedPayload struct {
	IsSleepSessionFinished bool                  `json:"is_sleep_session_finished"`
	Segments               []SleepSegmentPayload `json:"segments"` // 可能为空，需正确处理
	TodayTotalSleepHours   *float64              `json:"today_total_sleep_hours"`
}

// Summary & detail DTOs ------------------------------------------------------

type HeartRatePoint struct {
	Time  time.Time `json:"time"`
	Value float64   `json:"value"`
}

// StepSamplePoint represents a step/energy sample with time range
type StepSamplePoint struct {
	StartTime  time.Time `json:"start"`
	EndTime    time.Time `json:"end"`
	Value      float64   `json:"value"`
	SampleType string    `json:"sample_type"`
}

// SleepSegmentPoint represents a sleep stage segment
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

// LifeProbeSummary contains probe info with latest metrics
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
	StepsToday      float64          `json:"steps_today"`
	SleepDuration   *float64         `json:"sleep_duration"`
	DailyTotals     []DailyStepPoint `json:"daily_totals"`
}

// LifeProbeDetails contains all time-series data for a probe
type LifeProbeDetails struct {
	Summary     *LifeProbeSummary   `json:"summary"`
	HeartRates  []HeartRatePoint    `json:"heart_rates"`
	StepSamples []StepSamplePoint   `json:"step_samples"`
	Sleep       []SleepSegmentPoint `json:"sleep_segments"`
	SleepStats  SleepOverview       `json:"sleep_overview"`
}

// CRUD helpers ----------------------------------------------------------------

func CreateLifeProbe(probe *LifeProbe) error {
	return DB.Create(probe).Error
}

func UpdateLifeProbe(probe *LifeProbe) error {
	return DB.Save(probe).Error
}

// DeleteLifeProbe removes probe and all associated data
func DeleteLifeProbe(id uint) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		// 删除所有关联的数据
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
		if err := tx.Where("life_probe_id = ?", id).Delete(&LifeLoggerEvent{}).Error; err != nil {
			return err
		}
		// 最后删除探针本身
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
	return db.Model(&LifeProbe{}).
		Where("id = ? AND (last_sync_at IS NULL OR last_sync_at <= ?)", probeID, eventTime).
		Updates(updates).Error
}

// UpdateProbeHeartRate updates latest heart rate in probe record
func UpdateProbeHeartRate(db *gorm.DB, probeID uint, value float64, measureTime time.Time) error {
	updates := map[string]interface{}{
		"latest_heart_rate":    normalizeHeartRate(value),
		"latest_heart_rate_at": measureTime,
	}
	return db.Model(&LifeProbe{}).
		Where("id = ? AND (latest_heart_rate_at IS NULL OR latest_heart_rate_at <= ?)", probeID, measureTime).
		Updates(updates).Error
}

// normalizeHeartRate cleans up invalid heart rate values
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
	return db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "life_probe_id"}, {Name: "measure_time"}},
		DoNothing: true,
	}).Create(&record).Error
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
	return db.Clauses(clause.OnConflict{
		Columns: []clause.Column{
			{Name: "life_probe_id"},
			{Name: "sample_type"},
			{Name: "start_time"},
			{Name: "end_time"},
		},
		DoNothing: true,
	}).Create(&records).Error
}

func incrementDailyTotal(db *gorm.DB, probeID uint, day time.Time, sampleType string, delta float64) error {
	if delta == 0 {
		return nil
	}

	record := LifeStepDailyTotal{
		LifeProbeID: probeID,
		Day:         truncateToDay(day),
		SampleType:  sampleType,
		Total:       delta,
	}

	return db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "life_probe_id"}, {Name: "day"}, {Name: "sample_type"}},
		DoUpdates: clause.Assignments(map[string]interface{}{"total": gorm.Expr("total + ?", delta)}),
	}).Create(&record).Error
}

func setDailyTotal(db *gorm.DB, probeID uint, day time.Time, sampleType string, total float64) error {
	record := LifeStepDailyTotal{
		LifeProbeID: probeID,
		Day:         truncateToDay(day),
		SampleType:  sampleType,
		Total:       total,
	}

	return db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "life_probe_id"}, {Name: "day"}, {Name: "sample_type"}},
		DoUpdates: clause.Assignments(map[string]interface{}{"total": total}),
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

func OverrideDailyTotal(db *gorm.DB, probeID uint, sampleType string, reference time.Time, total float64) error {
	return setDailyTotal(db, probeID, reference, sampleType, total)
}

// RecordSleepSegments inserts sleep segments, handling empty segments gracefully
func RecordSleepSegments(db *gorm.DB, probeID uint, eventID string, segments []SleepSegmentPayload) error {
	if len(segments) == 0 {
		// 客户端可能无法获取睡眠阶段数据，这是合法的
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
	return db.Clauses(clause.OnConflict{
		Columns: []clause.Column{
			{Name: "life_probe_id"},
			{Name: "stage"},
			{Name: "start_time"},
			{Name: "end_time"},
		},
		DoUpdates: clause.Assignments(map[string]interface{}{
			"event_id": eventID,
			"duration": gorm.Expr("excluded.duration"),
		}),
	}).Create(&rows).Error
}

// Query helpers ---------------------------------------------------------------

func truncateToDay(t time.Time) time.Time {
	utc := t.UTC()
	return time.Date(utc.Year(), utc.Month(), utc.Day(), 0, 0, 0, 0, time.UTC)
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
	reference = reference.UTC()
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
		if start == nil || seg.StartTime.Before(*start) {
			tmp := seg.StartTime
			start = &tmp
		}
		if end == nil || seg.EndTime.After(*end) {
			tmp := seg.EndTime
			end = &tmp
		}

		// `in_bed` 表示“在床但未入睡”的阶段。如果把它和睡眠阶段一起累加，
		// 总时长会比真实睡眠时间长很多（示例中达到 17 小时）。因此统计时忽略
		// 该阶段，只累计浅睡/深睡/REM/清醒等数据。
		if seg.Stage == "in_bed" {
			continue
		}

		stageDurations[seg.Stage] += seg.Duration
		total += seg.Duration
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

// BuildLifeProbeSummary constructs summary with latest metrics
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
	referenceTime = referenceTime.UTC()

	// 最新心率
	if probe.LatestHeartRateAt != nil {
		cpy := *probe.LatestHeartRateAt
		summary.LatestHeartRate = &HeartRatePoint{
			Time:  cpy,
			Value: probe.LatestHeartRate,
		}
	}

	// 今日步数
	day := truncateToDay(referenceTime)
	nextDay := day.Add(24 * time.Hour)
	var total LifeStepDailyTotal
	if err := DB.Where("life_probe_id = ? AND sample_type = ? AND day >= ? AND day < ?",
		probe.ID, LifeDataTypeStepsDetailed, day, nextDay).
		Order("day desc").
		First(&total).Error; err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}

		// 退而求其次：从样本表累加今日步数
		var sum float64
		if err := DB.Model(&LifeStepSample{}).
			Select("COALESCE(SUM(value), 0)").
			Where("life_probe_id = ? AND sample_type = ? AND start_time >= ? AND start_time < ?",
				probe.ID, LifeDataTypeStepsDetailed, day, nextDay).
			Scan(&sum).Error; err == nil {
			summary.StepsToday = sum
		}
	} else {
		summary.StepsToday = total.Total
	}

	// 最新睡眠时长
	if segments, err := getLatestSleepSegments(probe.ID); err == nil && len(segments) > 0 {
		overview := buildSleepOverview(segments)
		summary.SleepDuration = &overview.TotalDuration
	}

	// 可选：包含每日汇总数据
	if includeDailyTotals {
		totals, err := getDailyTotals(probe.ID, 7, referenceTime)
		if err != nil {
			return nil, err
		}
		summary.DailyTotals = totals
	}

	return summary, nil
}

// GetLifeProbeDetails retrieves all time-series data for a probe within time range
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

	// 心率数据
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

	// 步数样本
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

	// 最新睡眠数据
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

	// 每日汇总
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

// DeleteLifeDataBefore removes old life probe data (for retention policies)
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
