//go:build monitor_only

package handler

// appOpsFields 监控版无终端处理相关字段
type appOpsFields struct{}

// InitTerminalHandling 监控版无需终端处理
func (a *App) InitTerminalHandling() {}
