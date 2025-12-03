package controllers

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/user/server-ops-backend/models"
	"gorm.io/gorm"
)

type lifeProbeRequest struct {
	Name            string `json:"name"`
	DeviceID        string `json:"device_id"`
	Description     string `json:"description"`
	Tags            string `json:"tags"`
	AllowPublicView *bool  `json:"allow_public_view"`
}

// CreateLifeProbe handles creation of a new life probe device.
func CreateLifeProbe(c *gin.Context) {
	var req lifeProbeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的请求数据"})
		return
	}

	if req.Name == "" || req.DeviceID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "名称和设备ID不能为空"})
		return
	}

	probe := models.LifeProbe{
		Name:            req.Name,
		DeviceID:        req.DeviceID,
		Description:     req.Description,
		Tags:            req.Tags,
		AllowPublicView: true,
	}
	if req.AllowPublicView != nil {
		probe.AllowPublicView = *req.AllowPublicView
	}

	if err := models.CreateLifeProbe(&probe); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "创建生命探针失败: " + err.Error()})
		return
	}

	go notifyLifeProbeListChanged()

	c.JSON(http.StatusCreated, gin.H{"life_probe": probe})
}

// ListLifeProbes returns all probes with summary data (authenticated).
func ListLifeProbes(c *gin.Context) {
	probes, err := models.ListLifeProbes()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取生命探针列表失败: " + err.Error()})
		return
	}

	now := time.Now()
	summaries := make([]*models.LifeProbeSummary, 0, len(probes))
	for i := range probes {
		summary, err := models.BuildLifeProbeSummary(&probes[i], now, false)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "构建生命探针摘要失败: " + err.Error()})
			return
		}
		summaries = append(summaries, summary)
	}

	c.JSON(http.StatusOK, gin.H{"life_probes": summaries})
}

// GetLifeProbe returns the basic probe info for editing.
func GetLifeProbe(c *gin.Context) {
	id, err := parseUintParam(c, "id")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的探针ID"})
		return
	}

	probe, err := models.GetLifeProbeByID(uint(id))
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, gorm.ErrRecordNotFound) {
			status = http.StatusNotFound
		}
		c.JSON(status, gin.H{"error": "生命探针不存在"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"life_probe": probe})
}

// UpdateLifeProbe updates metadata for a probe.
func UpdateLifeProbe(c *gin.Context) {
	id, err := parseUintParam(c, "id")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的探针ID"})
		return
	}

	probe, err := models.GetLifeProbeByID(uint(id))
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, gorm.ErrRecordNotFound) {
			status = http.StatusNotFound
		}
		c.JSON(status, gin.H{"error": "生命探针不存在"})
		return
	}

	var req lifeProbeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的请求数据"})
		return
	}

	if req.Name != "" {
		probe.Name = req.Name
	}
	if req.DeviceID != "" {
		probe.DeviceID = req.DeviceID
	}
	probe.Description = req.Description
	probe.Tags = req.Tags
	if req.AllowPublicView != nil {
		probe.AllowPublicView = *req.AllowPublicView
	}

	if err := models.UpdateLifeProbe(probe); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "更新生命探针失败: " + err.Error()})
		return
	}

	go notifyLifeProbeListChanged()

	c.JSON(http.StatusOK, gin.H{"life_probe": probe})
}

// DeleteLifeProbe removes a probe and historical data.
func DeleteLifeProbe(c *gin.Context) {
	id, err := parseUintParam(c, "id")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的探针ID"})
		return
	}

	if err := models.DeleteLifeProbe(uint(id)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "删除生命探针失败: " + err.Error()})
		return
	}

	go cleanupLifeProbeConnections(uint(id))
	go notifyLifeProbeListChanged()

	c.JSON(http.StatusOK, gin.H{"message": "生命探针已删除"})
}

// GetLifeProbeDetails returns detailed metrics for a probe (authenticated).
func GetLifeProbeDetails(c *gin.Context) {
	handleLifeProbeDetailsRequest(c, false)
}

// GetPublicLifeProbeDetails returns detailed metrics for public probes.
func GetPublicLifeProbeDetails(c *gin.Context) {
	handleLifeProbeDetailsRequest(c, true)
}

func handleLifeProbeDetailsRequest(c *gin.Context, public bool) {
	id, err := parseUintParam(c, "id")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的探针ID"})
		return
	}

	probe, err := models.GetLifeProbeByID(uint(id))
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, gorm.ErrRecordNotFound) {
			status = http.StatusNotFound
		}
		c.JSON(status, gin.H{"error": "生命探针不存在"})
		return
	}

	if public && !probe.AllowPublicView {
		c.JSON(http.StatusForbidden, gin.H{"error": "该生命探针未公开"})
		return
	}

	start, end := parseLifeTimeRange(c)
	dailyDays := parseIntDefault(c, "daily_days", 7)

	details, err := models.GetLifeProbeDetails(uint(id), start, end, dailyDays)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取生命探针详情失败: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, details)
}

func parseLifeTimeRange(c *gin.Context) (time.Time, time.Time) {
	const defaultHours = 24

	startStr := c.Query("start_time")
	endStr := c.Query("end_time")
	hours := parseIntDefault(c, "hours", defaultHours)

	now := time.Now()
	var start, end time.Time
	var err error

	if endStr != "" {
		end, err = time.Parse(time.RFC3339, endStr)
		if err != nil {
			end = now
		}
	} else {
		end = now
	}

	if startStr != "" {
		start, err = time.Parse(time.RFC3339, startStr)
		if err != nil {
			start = end.Add(-time.Duration(hours) * time.Hour)
		}
	} else {
		start = end.Add(-time.Duration(hours) * time.Hour)
	}

	return start, end
}

func parseIntDefault(c *gin.Context, key string, defaultVal int) int {
	if val := c.Query(key); val != "" {
		if parsed, err := strconv.Atoi(val); err == nil {
			return parsed
		}
	}
	return defaultVal
}

// GetPublicLifeProbes returns summaries for probes marked as public.
func GetPublicLifeProbes(c *gin.Context) {
	probes, err := models.ListPublicLifeProbes()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取生命探针列表失败: " + err.Error()})
		return
	}

	now := time.Now()
	summaries := make([]*models.LifeProbeSummary, 0, len(probes))
	for i := range probes {
		summary, err := models.BuildLifeProbeSummary(&probes[i], now, false)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "构建生命探针摘要失败: " + err.Error()})
			return
		}
		summaries = append(summaries, summary)
	}

	c.JSON(http.StatusOK, gin.H{"life_probes": summaries})
}

// ---------------- LifeLogger ingestion --------------------

type lifeLoggerRequest struct {
	EventID      string          `json:"event_id"`
	Timestamp    string          `json:"timestamp"`
	DeviceID     string          `json:"device_id"`
	BatteryLevel *float64        `json:"battery_level"`
	DataType     string          `json:"data_type"`
	Payload      json.RawMessage `json:"payload"`
}

// IngestLifeLoggerEvent receives raw payloads from the LifeLogger client.
func IngestLifeLoggerEvent(c *gin.Context) {
	bodyBytes, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "读取请求数据失败"})
		return
	}
	if len(bodyBytes) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求体不能为空"})
		return
	}

	var pingReq struct {
		Ping      string `json:"ping"`
		DeviceID  string `json:"device_id"`
		Timestamp string `json:"timestamp"`
	}
	if err := json.Unmarshal(bodyBytes, &pingReq); err == nil && pingReq.Ping == "test" {
		if pingReq.DeviceID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "设备ID不能为空"})
			return
		}

		probe, err := models.GetLifeProbeByDeviceID(pingReq.DeviceID)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				c.JSON(http.StatusNotFound, gin.H{"error": "生命探针不存在"})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": "查询生命探针失败"})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"success":           true,
			"message":           "pong",
			"device_id":         probe.DeviceID,
			"probe_name":        probe.Name,
			"allow_public_view": probe.AllowPublicView,
			"received_at":       time.Now().UTC(),
		})
		return
	}

	var req lifeLoggerRequest
	if err := json.Unmarshal(bodyBytes, &req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的数据格式"})
		return
	}

	if req.EventID == "" || req.DeviceID == "" || req.DataType == "" || len(req.Payload) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "缺少必须字段"})
		return
	}

	if !isSupportedLifeDataType(req.DataType) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "不支持的数据类型"})
		return
	}

	eventTime, err := time.Parse(time.RFC3339, req.Timestamp)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "timestamp 字段格式错误"})
		return
	}

	probe, err := models.GetLifeProbeByDeviceID(req.DeviceID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "未找到对应的生命探针"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询生命探针失败"})
		return
	}

	var (
		heartPayload  *models.HeartRatePayload
		stepsPayload  *models.StepsDetailedPayload
		sleepPayload  *models.SleepDetailedPayload
		focusPayload  *models.FocusStatusPayload
		screenPayload *models.ScreenEventPayload
	)

	switch req.DataType {
	case models.LifeDataTypeHeartRate:
		var payload models.HeartRatePayload
		if err := json.Unmarshal(req.Payload, &payload); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "心率数据格式错误"})
			return
		}
		heartPayload = &payload
	case models.LifeDataTypeStepsDetailed, models.LifeDataTypeEnergy:
		var payload models.StepsDetailedPayload
		if err := json.Unmarshal(req.Payload, &payload); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "步数数据格式错误"})
			return
		}
		stepsPayload = &payload
	case models.LifeDataTypeSleepDetailed:
		var payload models.SleepDetailedPayload
		if err := json.Unmarshal(req.Payload, &payload); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "睡眠数据格式错误"})
			return
		}
		sleepPayload = &payload
	case models.LifeDataTypeFocusStatus:
		var payload models.FocusStatusPayload
		if err := json.Unmarshal(req.Payload, &payload); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "专注模式数据格式错误"})
			return
		}
		focusPayload = &payload
	case models.LifeDataTypeScreenEvent:
		var payload models.ScreenEventPayload
		if err := json.Unmarshal(req.Payload, &payload); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "屏幕事件数据格式错误"})
			return
		}
		if payload.EventTime.IsZero() {
			payload.EventTime = eventTime
		}
		screenPayload = &payload
	}

	if err := models.DB.Transaction(func(tx *gorm.DB) error {
		event := models.LifeLoggerEvent{
			LifeProbeID:  probe.ID,
			EventID:      req.EventID,
			DeviceID:     req.DeviceID,
			DataType:     req.DataType,
			Timestamp:    eventTime,
			BatteryLevel: req.BatteryLevel,
			Payload:      req.Payload,
		}

		if err := models.CreateLifeLoggerEvent(tx, &event); err != nil {
			if errors.Is(err, gorm.ErrDuplicatedKey) {
				return nil
			}
			// SQLite 报错信息
			if err.Error() == "UNIQUE constraint failed: life_logger_events.event_id" {
				return nil
			}
			return err
		}

		if err := models.UpdateProbeSyncInfo(tx, probe.ID, eventTime, req.BatteryLevel); err != nil {
			return err
		}

		switch req.DataType {
		case models.LifeDataTypeHeartRate:
			if heartPayload == nil {
				return errors.New("心率数据为空")
			}
			if err := models.RecordHeartRate(tx, probe.ID, req.EventID, *heartPayload); err != nil {
				return err
			}
			if err := models.UpdateProbeHeartRate(tx, probe.ID, heartPayload.Value, heartPayload.MeasureTime); err != nil {
				return err
			}
		case models.LifeDataTypeStepsDetailed, models.LifeDataTypeEnergy:
			if stepsPayload == nil {
				return errors.New("步数数据为空")
			}
			if err := models.RecordStepSamples(tx, probe.ID, req.EventID, req.DataType, stepsPayload.Samples); err != nil {
				return err
			}
			if err := models.RecordDailyTotals(tx, probe.ID, req.DataType, stepsPayload.Samples); err != nil {
				return err
			}
		case models.LifeDataTypeSleepDetailed:
			if sleepPayload == nil {
				return errors.New("睡眠数据为空")
			}
			if err := models.RecordSleepSegments(tx, probe.ID, req.EventID, sleepPayload.Segments); err != nil {
				return err
			}
		case models.LifeDataTypeFocusStatus:
			if focusPayload == nil {
				return errors.New("专注模式数据为空")
			}
			if err := models.RecordFocusEvent(tx, probe.ID, req.EventID, *focusPayload, eventTime); err != nil {
				return err
			}
			if err := models.UpdateProbeFocusStatus(tx, probe.ID, focusPayload.IsFocused, focusPayload.ChangeReason, eventTime); err != nil {
				return err
			}
		case models.LifeDataTypeScreenEvent:
			if screenPayload == nil {
				return errors.New("屏幕事件数据为空")
			}
			if err := models.RecordScreenEvent(tx, probe.ID, req.EventID, *screenPayload); err != nil {
				return err
			}
		}

		return nil
	}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "保存数据失败: " + err.Error()})
		return
	}

	go notifyLifeProbeDataChanged(probe.ID)

	c.JSON(http.StatusOK, gin.H{"success": true})
}

func isSupportedLifeDataType(dataType string) bool {
	switch dataType {
	case models.LifeDataTypeHeartRate,
		models.LifeDataTypeStepsDetailed,
		models.LifeDataTypeEnergy,
		models.LifeDataTypeSleepDetailed,
		models.LifeDataTypeFocusStatus,
		models.LifeDataTypeScreenEvent:
		return true
	default:
		return false
	}
}
