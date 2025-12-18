package controllers

import (
	"fmt"
	"time"

	"github.com/user/server-ops-backend/models"
)

// MonitorPayload 表示从Agent或HTTP上报的监控数据
type MonitorPayload struct {
	CPUUsage       float64 `json:"cpu_usage"`
	MemoryUsed     uint64  `json:"memory_used"`
	MemoryTotal    uint64  `json:"memory_total"`
	DiskUsed       uint64  `json:"disk_used"`
	DiskTotal      uint64  `json:"disk_total"`
	NetworkIn      float64 `json:"network_in"`
	NetworkOut     float64 `json:"network_out"`
	LoadAvg1       float64 `json:"load_avg_1"`
	LoadAvg5       float64 `json:"load_avg_5"`
	LoadAvg15      float64 `json:"load_avg_15"`
	SwapUsed       uint64  `json:"swap_used"`
	SwapTotal      uint64  `json:"swap_total"`
	BootTime       uint64  `json:"boot_time"`
	Latency        float64 `json:"latency"`
	PacketLoss     float64 `json:"packet_loss"`
	Processes      int     `json:"processes"`
	TCPConnections int     `json:"tcp_connections"`
	UDPConnections int     `json:"udp_connections"`
}

// persistMonitorPayload 保存监控数据并更新服务器统计信息
func persistMonitorPayload(server *models.Server, payload *MonitorPayload) (*models.ServerMonitor, error) {
	if server == nil || payload == nil {
		return nil, fmt.Errorf("invalid monitor payload")
	}

	now := time.Now()
	record := models.ServerMonitor{
		ServerID:       server.ID,
		Timestamp:      now,
		CPUUsage:       payload.CPUUsage,
		MemoryUsed:     payload.MemoryUsed,
		MemoryTotal:    payload.MemoryTotal,
		DiskUsed:       payload.DiskUsed,
		DiskTotal:      payload.DiskTotal,
		NetworkIn:      payload.NetworkIn,
		NetworkOut:     payload.NetworkOut,
		LoadAvg1:       payload.LoadAvg1,
		LoadAvg5:       payload.LoadAvg5,
		LoadAvg15:      payload.LoadAvg15,
		SwapUsed:       payload.SwapUsed,
		SwapTotal:      payload.SwapTotal,
		BootTime:       payload.BootTime,
		Latency:        payload.Latency,
		PacketLoss:     payload.PacketLoss,
		Processes:      payload.Processes,
		TCPConnections: payload.TCPConnections,
		UDPConnections: payload.UDPConnections,
	}

	if err := models.AddMonitorData(&record); err != nil {
		return nil, err
	}

	// 更新服务器累计流量和网络质量
	server.NetworkInTotal += uint64(payload.NetworkIn)
	server.NetworkOutTotal += uint64(payload.NetworkOut)
	server.Latency = payload.Latency
	server.PacketLoss = payload.PacketLoss
	server.Status = "online"
	server.Online = true
	server.LastHeartbeat = now

	updates := map[string]interface{}{
		"network_in_total":  server.NetworkInTotal,
		"network_out_total": server.NetworkOutTotal,
		"latency":           server.Latency,
		"packet_loss":       server.PacketLoss,
		"last_heartbeat":    server.LastHeartbeat,
		"online":            server.Online,
		"status":            server.Status,
	}

	if err := models.DB.Model(&models.Server{}).Where("id = ?", server.ID).Updates(updates).Error; err != nil {
		return nil, err
	}

	return &record, nil
}
