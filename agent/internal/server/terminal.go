package server

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/user/server-ops-agent/pkg/logger"

	// 添加PTY支持
	"github.com/creack/pty"
)

// TerminalSession 表示一个终端会话
type TerminalSession struct {
	ID      string
	Cmd     *exec.Cmd
	Pty     *os.File // PTY文件句柄
	Stdin   io.WriteCloser
	Stdout  io.ReadCloser
	Stderr  io.ReadCloser
	Done    chan struct{}
	Lock    sync.Mutex
	IsAlive bool
}

// 存储活跃的终端会话
var terminalSessions = make(map[string]*TerminalSession)
var terminalSessionsLock sync.Mutex

// StartTerminalSession 启动一个新的终端会话
func StartTerminalSession(sessionID string, log *logger.Logger) (*TerminalSession, error) {
	log.Debug("启动终端会话: %s", sessionID)

	// 根据操作系统选择不同的shell
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.Command("powershell.exe")
	} else {
		// Linux/Unix默认使用bash，添加参数强制启用彩色输出
		cmd = exec.Command("/bin/bash")
	}

	// 设置环境变量
	cmd.Env = os.Environ()
	cmd.Env = append(cmd.Env, "TERM=xterm-256color")
	// 添加更多环境变量以强制启用彩色输出
	cmd.Env = append(cmd.Env, "FORCE_COLOR=true")
	cmd.Env = append(cmd.Env, "CLICOLOR=1")
	cmd.Env = append(cmd.Env, "CLICOLOR_FORCE=1")

	// 在Linux上设置命令别名来强制颜色输出
	if runtime.GOOS != "windows" {
		// 使用-c参数来执行初始化脚本，然后进入交互式shell
		initScript := `
# 颜色输出初始化
export TERM=xterm-256color
export FORCE_COLOR=true
export CLICOLOR=1
export CLICOLOR_FORCE=1

# 设置彩色提示符
export PS1='\[\033[01;32m\]\u@\h\[\033[00m\]:\[\033[01;34m\]\w\[\033[00m\]\$ '

# 适配不同Linux发行版的颜色别名
if [ -x "$(command -v dircolors)" ]; then
  eval "$(dircolors -b)"
fi

# 根据不同系统设置别名
case "$(uname -s)" in
  Linux*)
    # 检查ls是否支持--color选项
    if ls --color=auto >/dev/null 2>&1; then
      alias ls='ls --color=always'
    fi
    ;;
  Darwin*)
    # macOS
    export CLICOLOR=1
    export LSCOLORS=ExGxBxDxCxEgEdxbxgxcxd
    alias ls='ls -G'
    ;;
esac

# 通用别名
alias grep='grep --color=always'
alias egrep='egrep --color=always'
alias fgrep='fgrep --color=always'
alias diff='diff --color=always'

# 告知用户颜色已启用
echo -e "\033[32m终端颜色支持已启用\033[0m"

# 保持shell运行
exec bash
`
		cmd = exec.Command("/bin/bash", "-c", initScript)
	}

	// 会话结构
	session := &TerminalSession{
		ID:      sessionID,
		Cmd:     cmd,
		Done:    make(chan struct{}),
		IsAlive: true,
	}

	// 创建PTY (伪终端)
	if runtime.GOOS == "windows" {
		// Windows上使用标准管道，不支持真正的PTY
		stdin, err := cmd.StdinPipe()
		if err != nil {
			log.Error("获取标准输入失败: %v", err)
			return nil, err
		}

		stdout, err := cmd.StdoutPipe()
		if err != nil {
			log.Error("获取标准输出失败: %v", err)
			return nil, err
		}

		stderr, err := cmd.StderrPipe()
		if err != nil {
			log.Error("获取标准错误输出失败: %v", err)
			return nil, err
		}

		session.Stdin = stdin
		session.Stdout = stdout
		session.Stderr = stderr
	} else {
		// Linux/Mac上使用PTY
		ptmx, err := pty.Start(cmd)
		if err != nil {
			log.Error("创建PTY失败: %v", err)
			return nil, err
		}

		// 保存PTY
		session.Pty = ptmx
		session.Stdin = ptmx
		session.Stdout = ptmx
		session.Stderr = ptmx
	}

	// 启动命令
	if runtime.GOOS == "windows" {
		if err := cmd.Start(); err != nil {
			log.Error("启动终端进程失败: %v", err)
			return nil, err
		}
	}

	// 存储会话
	terminalSessionsLock.Lock()
	terminalSessions[sessionID] = session
	terminalSessionsLock.Unlock()

	// 监听命令结束
	go func() {
		err := cmd.Wait()
		session.Lock.Lock()
		session.IsAlive = false
		session.Lock.Unlock()
		close(session.Done)
		log.Debug("终端会话结束: %s, 错误: %v", sessionID, err)

		// 延迟删除会话，给客户端一些时间来处理会话结束
		time.Sleep(5 * time.Second)
		terminalSessionsLock.Lock()
		delete(terminalSessions, sessionID)
		terminalSessionsLock.Unlock()
	}()

	log.Debug("终端会话启动成功: %s", sessionID)
	return session, nil
}

// GetTerminalSession 获取终端会话
func GetTerminalSession(sessionID string) *TerminalSession {
	terminalSessionsLock.Lock()
	defer terminalSessionsLock.Unlock()
	return terminalSessions[sessionID]
}

// WriteToTerminal 向终端写入数据
func WriteToTerminal(sessionID string, data string, log *logger.Logger) error {
	session := GetTerminalSession(sessionID)
	if session == nil {
		return nil
	}

	session.Lock.Lock()
	defer session.Lock.Unlock()

	if !session.IsAlive {
		return nil
	}

	// 避免在Windows中换行符问题
	if runtime.GOOS == "windows" {
		data = strings.ReplaceAll(data, "\n", "\r\n")
	}

	_, err := session.Stdin.Write([]byte(data))
	if err != nil {
		log.Error("向终端写入数据失败: %v", err)
	}
	return err
}

// ResizeTerminal 调整终端大小
func ResizeTerminal(sessionID string, cols, rows uint16, log *logger.Logger) error {
	session := GetTerminalSession(sessionID)
	if session == nil {
		return nil
	}

	session.Lock.Lock()
	defer session.Lock.Unlock()

	if !session.IsAlive {
		return nil
	}

	// 调整PTY大小
	if runtime.GOOS != "windows" && session.Pty != nil {
		// 使用pty包提供的Setsize功能
		if err := pty.Setsize(session.Pty, &pty.Winsize{
			Rows: rows,
			Cols: cols,
		}); err != nil {
			log.Error("调整终端大小失败: %v", err)
			return err
		}
		return nil
	}

	// Windows无法直接调整终端大小
	if runtime.GOOS == "windows" {
		log.Debug("Windows不支持PTY终端大小调整")
	}

	return nil
}

// CloseTerminalSession 关闭终端会话
func CloseTerminalSession(sessionID string, log *logger.Logger) {
	session := GetTerminalSession(sessionID)
	if session == nil {
		return
	}

	terminalSessionsLock.Lock()
	delete(terminalSessions, sessionID)
	terminalSessionsLock.Unlock()

	session.Lock.Lock()
	defer session.Lock.Unlock()

	if !session.IsAlive {
		return
	}

	// 关闭会话
	session.IsAlive = false

	// 关闭PTY
	if session.Pty != nil {
		session.Pty.Close()
	}

	// 关闭标准输入输出
	if session.Stdin != nil {
		session.Stdin.Close()
	}

	// 终止进程
	if runtime.GOOS == "windows" {
		// Windows使用taskkill强制结束进程
		exec.Command("taskkill", "/F", "/T", "/PID", fmt.Sprintf("%d", session.Cmd.Process.Pid)).Run()
	} else {
		// Linux/macOS发送SIGKILL信号
		session.Cmd.Process.Kill()
	}
}

// GetTerminalWorkingDirectory 获取终端当前工作目录
func GetTerminalWorkingDirectory(sessionID string, log *logger.Logger) (string, error) {
	session := GetTerminalSession(sessionID)
	if session == nil {
		return "", fmt.Errorf("终端会话不存在: %s", sessionID)
	}

	session.Lock.Lock()
	defer session.Lock.Unlock()

	if !session.IsAlive {
		return "", fmt.Errorf("终端会话已关闭: %s", sessionID)
	}

	// 根据操作系统获取工作目录
	if runtime.GOOS == "windows" {
		// Windows系统：通过查询进程工作目录
		return getWindowsProcessWorkingDirectory(session.Cmd.Process.Pid, log)
	} else {
		// Linux/macOS系统：通过/proc/{pid}/cwd获取
		return getLinuxProcessWorkingDirectory(session.Cmd.Process.Pid, log)
	}
}

// getLinuxProcessWorkingDirectory 获取Linux/macOS进程的工作目录
func getLinuxProcessWorkingDirectory(pid int, log *logger.Logger) (string, error) {
	// 通过/proc/{pid}/cwd获取进程当前工作目录
	cwdPath := fmt.Sprintf("/proc/%d/cwd", pid)
	
	// 读取符号链接指向的真实路径
	realPath, err := os.Readlink(cwdPath)
	if err != nil {
		log.Error("读取进程工作目录失败 PID=%d: %v", pid, err)
		return "/", nil // 返回根目录作为默认值
	}
	
	log.Debug("获取到进程工作目录 PID=%d: %s", pid, realPath)
	return realPath, nil
}

// getWindowsProcessWorkingDirectory 获取Windows进程的工作目录
func getWindowsProcessWorkingDirectory(pid int, log *logger.Logger) (string, error) {
	// Windows下通过PowerShell命令获取进程工作目录
	cmd := exec.Command("powershell", "-Command", 
		fmt.Sprintf("(Get-Process -Id %d).Path | Split-Path", pid))
	
	output, err := cmd.Output()
	if err != nil {
		log.Error("获取Windows进程工作目录失败 PID=%d: %v", pid, err)
		return "C:\\", nil // 返回C盘根目录作为默认值
	}
	
	workingDir := strings.TrimSpace(string(output))
	if workingDir == "" {
		workingDir = "C:\\"
	}
	
	log.Debug("获取到Windows进程工作目录 PID=%d: %s", pid, workingDir)
	return workingDir, nil
}
