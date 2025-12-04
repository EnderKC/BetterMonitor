package controllers

import (
	"errors"
	"log"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/user/server-ops-backend/models"
	"gorm.io/gorm"
)

var lifeProbePublicListConns = &publicConnSet{}
var lifeProbePrivateListConns = &publicConnSet{}
var lifeProbeDetailConns sync.Map // map[uint]*lifeDetailConnSet

type lifeDetailConn struct {
	conn      *SafeConn
	hours     int
	dailyDays int
}

type lifeDetailConnSet struct {
	mu    sync.Mutex
	conns map[*lifeDetailConn]struct{}
}

func (s *lifeDetailConnSet) add(conn *lifeDetailConn) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.conns == nil {
		s.conns = make(map[*lifeDetailConn]struct{})
	}
	s.conns[conn] = struct{}{}
}

func (s *lifeDetailConnSet) remove(conn *lifeDetailConn) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.conns, conn)
}

func (s *lifeDetailConnSet) len() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.conns)
}

func (s *lifeDetailConnSet) snapshot() []*lifeDetailConn {
	s.mu.Lock()
	defer s.mu.Unlock()
	list := make([]*lifeDetailConn, 0, len(s.conns))
	for conn := range s.conns {
		list = append(list, conn)
	}
	return list
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
	token := c.Query("token")
	includeAll := false
	if token != "" {
		if claims, err := verifyJWTFromQuery(token); err == nil && claims != nil {
			includeAll = true
		}
	}

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

func getLifeProbeDetailsPayload(probeID uint, hours, dailyDays int) (map[string]interface{}, error) {
	if hours <= 0 {
		hours = 24
	}
	end := time.Now()
	start := end.Add(-time.Duration(hours) * time.Hour)

	details, err := models.GetLifeProbeDetails(probeID, start, end, dailyDays)
	if err != nil {
		return nil, err
	}

	return map[string]interface{}{
		"type":          "life_probe_detail",
		"life_probe_id": probeID,
		"details":       details,
	}, nil
}

func registerLifeProbeDetailConn(probeID uint, conn *lifeDetailConn) {
	value, _ := lifeProbeDetailConns.LoadOrStore(probeID, &lifeDetailConnSet{})
	set := value.(*lifeDetailConnSet)
	set.add(conn)
}

func unregisterLifeProbeDetailConn(probeID uint, conn *lifeDetailConn) {
	if value, ok := lifeProbeDetailConns.Load(probeID); ok {
		if set, _ := value.(*lifeDetailConnSet); set != nil {
			set.remove(conn)
			if set.len() == 0 {
				lifeProbeDetailConns.Delete(probeID)
			}
		}
	}
}

func closeLifeProbeDetailConnSet(probeID uint) {
	if value, ok := lifeProbeDetailConns.LoadAndDelete(probeID); ok {
		if set, _ := value.(*lifeDetailConnSet); set != nil {
			conns := set.snapshot()
			for _, item := range conns {
				_ = item.conn.WriteControl(websocket.CloseMessage,
					websocket.FormatCloseMessage(websocket.CloseNormalClosure, "life probe removed"),
					time.Now().Add(time.Second))
				_ = item.conn.Close()
			}
		}
	}
}

// LifeProbeDetailWebSocketHandler 推送单个生命探针详情
func LifeProbeDetailWebSocketHandler(c *gin.Context) {
	idParam := c.Param("id")
	probeID64, err := strconv.ParseUint(idParam, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的生命探针ID"})
		return
	}
	probeID := uint(probeID64)

	token := c.Query("token")
	includeAll := false
	if token != "" {
		if claims, err := verifyJWTFromQuery(token); err == nil && claims != nil {
			includeAll = true
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
		conn:      safeConn,
		hours:     hours,
		dailyDays: dailyDays,
	}
	registerLifeProbeDetailConn(probeID, connInfo)
	defer unregisterLifeProbeDetailConn(probeID, connInfo)

	sendDetail := func() error {
		payload, err := getLifeProbeDetailsPayload(probeID, connInfo.hours, connInfo.dailyDays)
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
	value, ok := lifeProbeDetailConns.Load(probeID)
	if !ok {
		return
	}

	set, _ := value.(*lifeDetailConnSet)
	if set == nil || set.len() == 0 {
		lifeProbeDetailConns.Delete(probeID)
		return
	}

	conns := set.snapshot()
	for _, connInfo := range conns {
		payload, err := getLifeProbeDetailsPayload(probeID, connInfo.hours, connInfo.dailyDays)
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
