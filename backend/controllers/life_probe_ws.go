package controllers

import (
	"errors"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/user/server-ops-backend/models"
	"github.com/user/server-ops-backend/services"
	"gorm.io/gorm"
)

var lifeProbePublicListConns = &publicConnSet{}
var lifeProbePrivateListConns = &publicConnSet{}
var lifeProbeDetailConns = NewSubscriberRegistry[uint, *lifeDetailConn]()

type lifeDetailConn struct {
	conn       *SafeConn
	hours      int
	dailyDays  int
	maskDevice bool // 是否需要脱敏设备ID（公开访问时为true）
}

func buildLifeProbeSummaries(includeAll bool) ([]*models.LifeProbeSummary, error) {
	var (
		probes []models.LifeProbe
		err    error
	)

	if includeAll {
		probes, err = models.ListLifeProbes()
	} else {
		probes, err = models.ListPublicLifeProbes()
	}
	if err != nil {
		return nil, err
	}

	now := time.Now()
	summaries := make([]*models.LifeProbeSummary, 0, len(probes))
	for i := range probes {
		summary, err := models.BuildLifeProbeSummary(&probes[i], now, true)
		if err != nil {
			return nil, err
		}
		// 公开访问时对设备ID进行脱敏
		if !includeAll {
			summary.DeviceID = maskDeviceID(summary.DeviceID)
		}
		summaries = append(summaries, summary)
	}
	return summaries, nil
}

func lifeProbeListPayload(includeAll bool) (map[string]interface{}, error) {
	summaries, err := buildLifeProbeSummaries(includeAll)
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{
		"type":        "life_probe_list",
		"life_probes": summaries,
	}, nil
}

// LifeProbeListWebSocketHandler 推送生命探针列表
func LifeProbeListWebSocketHandler(c *gin.Context) {
	includeAll, ok := authorizeOptionalBrowserTicket(c, services.WSTicketClaims{
		Purpose: services.WSTicketLifeProbeList,
	})
	if !ok {
		return
	}

	// 列表接口始终允许访问（不受系统开关限制）
	// 系统开关 AllowPublicLifeProbeAccess 只控制详情接口的公开访问

	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Printf("升级生命探针列表WebSocket失败: %v", err)
		return
	}
	safeConn := &SafeConn{Conn: conn}
	defer safeConn.Close()

	targetSet := lifeProbePublicListConns
	if includeAll {
		targetSet = lifeProbePrivateListConns
	}
	targetSet.add(safeConn)
	defer targetSet.remove(safeConn)

	sendList := func() error {
		payload, err := lifeProbeListPayload(includeAll)
		if err != nil {
			return err
		}
		return safeConn.WriteJSON(payload)
	}

	if err := sendList(); err != nil {
		log.Printf("发送生命探针列表失败: %v", err)
		return
	}

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if err := sendList(); err != nil {
				log.Printf("刷新生命探针列表失败: %v", err)
				return
			}
		default:
			if err := safeConn.Conn.SetReadDeadline(time.Now().Add(35 * time.Second)); err != nil {
				log.Printf("设置生命探针列表读超时失败: %v", err)
				return
			}
			if _, _, err := safeConn.ReadMessage(); err != nil {
				if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
					log.Printf("生命探针列表WebSocket关闭: %v", err)
				}
				return
			}
		}
	}
}

func parseLifeDetailRequestedRange(c *gin.Context) (int, int) {
	hours := 24
	if val := c.DefaultQuery("hours", "24"); val != "" {
		if parsed, err := strconv.Atoi(val); err == nil && parsed > 0 {
			hours = parsed
		}
	}
	dailyDays := 7
	if val := c.DefaultQuery("daily_days", "7"); val != "" {
		if parsed, err := strconv.Atoi(val); err == nil && parsed > 0 {
			dailyDays = parsed
		}
	}
	return hours, dailyDays
}

func getLifeProbeDetailsPayload(probeID uint, hours, dailyDays int, maskDevice bool) (map[string]interface{}, error) {
	if hours <= 0 {
		hours = 24
	}
	end := time.Now()
	start := end.Add(-time.Duration(hours) * time.Hour)

	details, err := models.GetLifeProbeDetails(probeID, start, end, dailyDays)
	if err != nil {
		return nil, err
	}

	// 公开访问时对设备ID进行脱敏
	if maskDevice && details.Summary.DeviceID != "" {
		details.Summary.DeviceID = maskDeviceID(details.Summary.DeviceID)
	}

	return map[string]interface{}{
		"type":          "life_probe_detail",
		"life_probe_id": probeID,
		"details":       details,
	}, nil
}

func registerLifeProbeDetailConn(probeID uint, conn *lifeDetailConn) {
	lifeProbeDetailConns.Add(probeID, conn)
}

func unregisterLifeProbeDetailConn(probeID uint, conn *lifeDetailConn) {
	lifeProbeDetailConns.Remove(probeID, conn)
}

func closeLifeProbeDetailConnSet(probeID uint) {
	for _, item := range lifeProbeDetailConns.RemoveAll(probeID) {
		_ = item.conn.WriteControl(websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseNormalClosure, "life probe removed"),
			time.Now().Add(time.Second))
		_ = item.conn.Close()
	}
}

// LifeProbeDetailWebSocketHandler 推送单个生命探针详情
func LifeProbeDetailWebSocketHandler(c *gin.Context) {
	if rejectLegacyWebSocketAuth(c) {
		return
	}
	idParam := c.Param("id")
	probeID64, err := strconv.ParseUint(idParam, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的生命探针ID"})
		return
	}
	probeID := uint(probeID64)

	includeAll, ok := authorizeOptionalBrowserTicket(c, services.WSTicketClaims{
		Purpose:     services.WSTicketLifeProbeDetail,
		LifeProbeID: probeID,
	})
	if !ok {
		return
	}

	// 公开访问时检查系统设置（fail-closed: 读取失败时拒绝访问）
	if !includeAll {
		settings, err := models.GetSettings()
		if err != nil {
			log.Printf("[生命探针WS] GetSettings失败(拒绝匿名详情访问): %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "系统设置读取失败"})
			return
		}
		if !settings.AllowPublicLifeProbeAccess {
			c.JSON(http.StatusForbidden, gin.H{"error": "系统未开启公开访问生命探针详情功能"})
			return
		}
	}

	probe, err := models.GetLifeProbeByID(probeID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "生命探针不存在"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "查询生命探针失败"})
		}
		return
	}

	if !includeAll && !probe.AllowPublicView {
		c.JSON(http.StatusForbidden, gin.H{"error": "该生命探针未公开"})
		return
	}

	hours, dailyDays := parseLifeDetailRequestedRange(c)

	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Printf("升级生命探针详情WebSocket失败: %v", err)
		return
	}
	safeConn := &SafeConn{Conn: conn}
	defer safeConn.Close()

	connInfo := &lifeDetailConn{
		conn:       safeConn,
		hours:      hours,
		dailyDays:  dailyDays,
		maskDevice: !includeAll, // 公开访问时需要脱敏
	}
	registerLifeProbeDetailConn(probeID, connInfo)
	defer unregisterLifeProbeDetailConn(probeID, connInfo)

	sendDetail := func() error {
		payload, err := getLifeProbeDetailsPayload(probeID, connInfo.hours, connInfo.dailyDays, connInfo.maskDevice)
		if err != nil {
			return err
		}
		return safeConn.WriteJSON(payload)
	}

	if err := sendDetail(); err != nil {
		log.Printf("发送生命探针详情失败: %v", err)
		return
	}

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if err := sendDetail(); err != nil {
				log.Printf("刷新生命探针详情失败: %v", err)
				return
			}
		default:
			if err := safeConn.Conn.SetReadDeadline(time.Now().Add(35 * time.Second)); err != nil {
				log.Printf("设置生命探针详情读超时失败: %v", err)
				return
			}
			if _, _, err := safeConn.ReadMessage(); err != nil {
				if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
					log.Printf("生命探针详情WebSocket关闭: %v", err)
				}
				return
			}
		}
	}
}

func broadcastLifeProbeList(includeAll bool) {
	var target *publicConnSet
	if includeAll {
		target = lifeProbePrivateListConns
	} else {
		target = lifeProbePublicListConns
	}

	if target == nil || target.len() == 0 {
		return
	}

	payload, err := lifeProbeListPayload(includeAll)
	if err != nil {
		log.Printf("构建生命探针列表数据失败: %v", err)
		return
	}
	target.broadcast(payload)
}

func broadcastLifeProbeDetail(probeID uint) {
	for _, connInfo := range lifeProbeDetailConns.Snapshot(probeID) {
		payload, err := getLifeProbeDetailsPayload(probeID, connInfo.hours, connInfo.dailyDays, connInfo.maskDevice)
		if err != nil {
			log.Printf("构建生命探针详情失败: %v", err)
			continue
		}
		if err := connInfo.conn.WriteJSON(payload); err != nil {
			log.Printf("广播生命探针详情失败: %v", err)
		}
	}
}

func notifyLifeProbeListChanged() {
	go broadcastLifeProbeList(false)
	go broadcastLifeProbeList(true)
}

func notifyLifeProbeDataChanged(probeID uint) {
	notifyLifeProbeListChanged()
	go broadcastLifeProbeDetail(probeID)
}

func cleanupLifeProbeConnections(probeID uint) {
	closeLifeProbeDetailConnSet(probeID)
}
