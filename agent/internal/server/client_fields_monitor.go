//go:build monitor_only

package server

// clientOpsFields 监控版无操作类字段
type clientOpsFields struct{}

// initOpsFields 监控版无需初始化操作类字段
func (c *Client) initOpsFields() {}
