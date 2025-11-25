package models

import (
	"log"
	"time"

	"gorm.io/gorm"
)

// Server 服务器模型
type Server struct {
	gorm.Model
	Name            string    `json:"name" gorm:"not null"`                   // 服务器名称
	IP              string    `json:"ip"`                                     // 服务器IP
	PublicIP        string    `json:"public_ip" gorm:"type:varchar(45)"`      // 公网IP
	OS              string    `json:"os"`                                     // 操作系统
	Arch            string    `json:"arch"`                                   // 架构
	CPUCores        int       `json:"cpu_cores"`                              // CPU核心数
	CPUModel        string    `json:"cpu_model"`                              // CPU型号
	MemoryTotal     int64     `json:"memory_total"`                           // 总内存(KB)
	DiskTotal       int64     `json:"disk_total"`                             // 总磁盘空间(KB)
	LastHeartbeat   time.Time `json:"last_heartbeat"`                         // 最后心跳时间
	Online          bool      `json:"online" gorm:"default:false"`            // 是否在线
	SecretKey       string    `json:"secret_key" gorm:"type:varchar(64)"`     // 密钥
	RegisterToken   string    `json:"-" gorm:"type:varchar(64)"`              // 注册令牌
	UserID          uint      `json:"user_id" gorm:"default:0"`               // 所属用户ID
	Tags            string    `json:"tags" gorm:"type:varchar(255)"`          // 标签，用逗号分隔
	Description     string    `json:"description" gorm:"type:text"`           // 描述
	AllowPublicView bool      `json:"allow_public_view" gorm:"default:false"` // 是否允许公开查看
	Status          string    `json:"status" gorm:"default:'offline'"`        // 服务器状态
	SystemInfo      string    `json:"system_info" gorm:"type:text"`           // 系统信息 JSON
	AgentVersion    string    `json:"agent_version" gorm:"type:varchar(64)"`  // Agent版本
	CountryCode     string    `json:"country_code" gorm:"type:varchar(10)"`   // 国家代码
	NetworkInTotal  uint64    `json:"network_in_total" gorm:"default:0"`      // 总入网流量
	NetworkOutTotal uint64    `json:"network_out_total" gorm:"default:0"`     // 总出网流量
	Latency         float64   `json:"latency" gorm:"default:0"`               // 延迟(ms)
	PacketLoss      float64   `json:"packet_loss" gorm:"default:0"`           // 丢包率(%)
	// Monitor 统计信息使用一对多关系
	Monitors []ServerMonitor `json:"-"`
}

// ServerMonitor 服务器监控数据模型
type ServerMonitor struct {
	gorm.Model
	ServerID    uint      `gorm:"index:idx_server_timestamp" json:"server_id"`
	Timestamp   time.Time `gorm:"index:idx_server_timestamp" json:"timestamp"`
	CPUUsage    float64   `json:"cpu_usage"`    // CPU使用率百分比
	MemoryUsed  uint64    `json:"memory_used"`  // 内存使用量(bytes)
	MemoryTotal uint64    `json:"memory_total"` // 内存总量(bytes)
	SwapUsed    uint64    `json:"swap_used"`    // Swap使用量(bytes)
	SwapTotal   uint64    `json:"swap_total"`   // Swap总量(bytes)
	DiskUsed    uint64    `json:"disk_used"`    // 磁盘使用量(bytes)
	DiskTotal   uint64    `json:"disk_total"`   // 磁盘总量(bytes)
	NetworkIn   float64   `json:"network_in"`   // 网络入流量(bytes/s)
	NetworkOut  float64   `json:"network_out"`  // 网络出流量(bytes/s)
	LoadAvg1    float64   `json:"load_avg_1"`   // 1分钟平均负载
	LoadAvg5    float64   `json:"load_avg_5"`   // 5分钟平均负载
	LoadAvg15   float64   `json:"load_avg_15"`  // 15分钟平均负载
	BootTime    uint64    `json:"boot_time"`    // 系统启动时间戳
	Latency     float64   `json:"latency"`      // 延迟(ms)
	PacketLoss  float64   `json:"packet_loss"`  // 丢包率(%)
}

// ServerMonitorData 服务器监控数据
type ServerMonitorData struct {
	gorm.Model
	ServerID    uint    `json:"server_id" gorm:"index"` // 服务器ID
	CPUUsage    float64 `json:"cpu_usage"`              // CPU使用率
	MemoryUsage float64 `json:"memory_usage"`           // 内存使用率
	DiskUsage   float64 `json:"disk_usage"`             // 磁盘使用率
	NetRecv     int64   `json:"net_recv"`               // 网络接收(Bytes)
	NetSent     int64   `json:"net_sent"`               // 网络发送(Bytes)
	LoadAvg1    float64 `json:"load_avg1"`              // 1分钟负载
	LoadAvg5    float64 `json:"load_avg5"`              // 5分钟负载
	LoadAvg15   float64 `json:"load_avg15"`             // 15分钟负载
	Processes   int     `json:"processes"`              // 进程数
	Timestamp   int64   `json:"timestamp"`              // 时间戳
}

// GetServer 根据ID获取服务器信息
func GetServer(id interface{}) (*Server, error) {
	var server Server
	if err := DB.First(&server, id).Error; err != nil {
		return nil, err
	}
	return &server, nil
}

// GetAllServers 获取所有服务器
func GetAllServers(userID uint) ([]Server, error) {
	var servers []Server
	query := DB

	// 如果指定了用户ID，则只获取该用户的服务器
	if userID > 0 {
		query = query.Where("user_id = ?", userID)
	}

	if err := query.Find(&servers).Error; err != nil {
		return nil, err
	}

	return servers, nil
}

// CheckServerOnlineStatus 检查服务器在线状态
func CheckServerOnlineStatus() {
	var servers []Server
	if err := DB.Find(&servers).Error; err != nil {
		return
	}

	for _, server := range servers {
		// 如果最后心跳时间超过1分钟，则标记为离线
		if time.Since(server.LastHeartbeat) > time.Minute {
			DB.Model(&server).Update("online", false)
		}
	}
}

// SaveServerMonitorData 保存服务器监控数据
func SaveServerMonitorData(data *ServerMonitorData) error {
	return DB.Create(data).Error
}

// GetServerRecentMonitorData 获取服务器最近的监控数据
func GetServerRecentMonitorData(serverID uint, limit int) ([]ServerMonitorData, error) {
	var data []ServerMonitorData

	if err := DB.Where("server_id = ?", serverID).
		Order("created_at desc").
		Limit(limit).
		Find(&data).Error; err != nil {
		return nil, err
	}

	return data, nil
}

// GetServerMonitorDataByTimeRange 根据时间范围获取服务器监控数据
func GetServerMonitorDataByTimeRange(serverID uint, start, end time.Time) ([]ServerMonitorData, error) {
	var data []ServerMonitorData

	if err := DB.Where("server_id = ? AND created_at BETWEEN ? AND ?", serverID, start, end).
		Order("created_at asc").
		Find(&data).Error; err != nil {
		return nil, err
	}

	return data, nil
}

// DeleteServerMonitorDataBefore 删除指定时间之前的监控数据
func DeleteServerMonitorDataBefore(before time.Time) error {
	// 删除ServerMonitor表中timestamp小于指定时间的记录
	result := DB.Where("timestamp < ?", before).Delete(&ServerMonitor{})
	if result.Error != nil {
		return result.Error
	}

	log.Printf("成功删除 %d 条过期监控数据", result.RowsAffected)
	return nil
}

// 更新服务器的心跳时间和在线状态
func UpdateServerHeartbeat(serverID uint) error {
	return DB.Model(&Server{}).Where("id = ?", serverID).
		Updates(map[string]interface{}{
			"last_heartbeat": time.Now(),
			"online":         true,
		}).Error
}

// GetServerByID 通过ID获取服务器
func GetServerByID(id uint) (*Server, error) {
	var server Server
	result := DB.First(&server, id)
	if result.Error != nil {
		return nil, result.Error
	}

	// 检查服务器的在线状态
	CheckServerStatus(&server)
	log.Println("服务器", server)

	return &server, nil
}

// CheckServerStatus 检查服务器的在线状态
// 如果最后心跳时间超过15秒，则将状态设置为离线
func CheckServerStatus(server *Server) {
	// 定义心跳超时时间为15秒
	const heartbeatTimeout = 15 * time.Second

	// 检查最后心跳时间是否超过超时时间
	timeSinceLastHeartbeat := time.Since(server.LastHeartbeat)

	// 记录日志方便调试
	log.Printf("服务器 %d (%s) 状态检查: 当前状态=%t, 上次心跳=%v, 距今=%v",
		server.ID, server.Name, server.Online,
		server.LastHeartbeat.Format(time.RFC3339),
		timeSinceLastHeartbeat)

	// 如果超过超时时间且当前状态为在线，则更新为离线
	if timeSinceLastHeartbeat > heartbeatTimeout {
		if server.Online {
			server.Online = false
			server.Status = "offline"
			// 只在数据库中更新状态，不更新其他字段
			result := DB.Model(server).Updates(map[string]interface{}{
				"online": false,
				"status": "offline",
			})
			if result.Error != nil {
				log.Printf("更新服务器 %d 状态为离线失败: %v", server.ID, result.Error)
			} else {
				log.Printf("服务器 %d 状态已更新为离线", server.ID)
			}
		}
	} else if !server.Online && timeSinceLastHeartbeat <= heartbeatTimeout {
		// 如果心跳在超时窗口内，但状态是离线，则更新为在线
		server.Online = true
		server.Status = "online"
		result := DB.Model(server).Updates(map[string]interface{}{
			"online": true,
			"status": "online",
		})
		if result.Error != nil {
			log.Printf("更新服务器 %d 状态为在线失败: %v", server.ID, result.Error)
		} else {
			log.Printf("服务器 %d 状态已更新为在线", server.ID)
		}
	}
}

// CreateServer 创建服务器
func CreateServer(server *Server) error {
	return DB.Create(server).Error
}

// UpdateServer 更新服务器信息
func UpdateServer(server *Server) error {
	return DB.Save(server).Error
}

// DeleteServer 删除服务器
func DeleteServer(id uint) error {
	// 删除服务器的同时删除相关监控数据
	if err := DB.Where("server_id = ?", id).Delete(&ServerMonitor{}).Error; err != nil {
		return err
	}
	return DB.Delete(&Server{}, id).Error
}

// UpdateServerStatus 更新服务器状态
func UpdateServerStatus(id uint, status string) error {
	return DB.Model(&Server{}).Where("id = ?", id).Updates(map[string]interface{}{
		"status":         status,
		"last_heartbeat": time.Now(),
		"online":         status == "online",
	}).Error
}

// UpdateServerStatusOnly 只更新服务器状态，不更新心跳时间
func UpdateServerStatusOnly(id uint, status string) error {
	return DB.Model(&Server{}).Where("id = ?", id).Updates(map[string]interface{}{
		"status": status,
		"online": status == "online",
	}).Error
}

// UpdateServerHeartbeatAndStatus 更新服务器心跳时间和状态
func UpdateServerHeartbeatAndStatus(id uint, status string) error {
	return DB.Model(&Server{}).Where("id = ?", id).Updates(map[string]interface{}{
		"status":         status,
		"last_heartbeat": time.Now(),
		"online":         status == "online",
	}).Error
}

// UpdateServerAgentVersion 更新服务器的Agent版本
func UpdateServerAgentVersion(id uint, version string) error {
	return DB.Model(&Server{}).Where("id = ?", id).
		Update("agent_version", version).Error
}

// AddMonitorData 添加监控数据
func AddMonitorData(data *ServerMonitor) error {
	return DB.Create(data).Error
}

// GetServerMonitorData 获取服务器监控数据
func GetServerMonitorData(serverID uint, startTime, endTime time.Time) ([]ServerMonitor, error) {
	var data []ServerMonitor

	// 记录查询参数，便于调试
	log.Printf("[DEBUG] 查询服务器ID=%d的监控数据，时间范围: %v 到 %v",
		serverID, startTime.Format(time.RFC3339), endTime.Format(time.RFC3339))

	// 构建查询条件
	query := DB.Where("server_id = ?", serverID)

	// 添加时间范围条件
	if !startTime.IsZero() && !endTime.IsZero() {
		query = query.Where("timestamp BETWEEN ? AND ?", startTime, endTime)
	} else if !startTime.IsZero() {
		query = query.Where("timestamp >= ?", startTime)
	} else if !endTime.IsZero() {
		query = query.Where("timestamp <= ?", endTime)
	}

	// 按时间升序排序
	query = query.Order("timestamp")

	// 执行查询
	result := query.Find(&data)

	// 记录查询结果
	if result.Error != nil {
		log.Printf("[ERROR] 查询服务器监控数据失败: %v", result.Error)
	} else {
		log.Printf("[DEBUG] 查询到 %d 条监控数据记录", len(data))

		// 如果数据为空，检查是否有任何监控数据存在
		if len(data) == 0 {
			var count int64
			DB.Model(&ServerMonitor{}).Where("server_id = ?", serverID).Count(&count)
			log.Printf("[DEBUG] 服务器ID=%d总共有 %d 条监控数据记录", serverID, count)
		}
	}

	return data, result.Error
}

// GetLatestMonitorData 获取最新的监控数据
func GetLatestMonitorData(serverID uint, limit int) ([]ServerMonitor, error) {
	var data []ServerMonitor
	result := DB.Where("server_id = ?", serverID).Order("timestamp desc").Limit(limit).Find(&data)
	return data, result.Error
}
