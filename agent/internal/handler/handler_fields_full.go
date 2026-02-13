//go:build !monitor_only

package handler

// appOpsFields 全功能版的终端处理相关字段
type appOpsFields struct {
	terminalHandler *TerminalHandler
}

// InitTerminalHandling 初始化终端处理（全功能版）
func (a *App) InitTerminalHandling() {
	a.terminalHandler = NewTerminalHandler(a.log)
	InitTerminalHandling(a.client, a.log)
	a.log.Info("终端处理初始化完成")
}
