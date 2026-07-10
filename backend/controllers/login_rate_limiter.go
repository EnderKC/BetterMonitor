package controllers

import (
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

const (
	loginMaxFailures        = 5
	loginLockout            = 15 * time.Minute
	loginAttemptStaleWindow = 30 * time.Minute // 未锁定条目静默超过此时长后清理
	loginCleanupInterval    = 10 * time.Minute
)

type loginAttempt struct {
	failures  int
	lockedAt  time.Time
	updatedAt time.Time
}

var (
	loginAttempts    = make(map[string]*loginAttempt)
	loginAttemptsMu  sync.Mutex
	loginCleanupOnce sync.Once
)

// startLoginAttemptCleaner 启动后台清理，回收过期/陈旧的登录尝试记录，防止 map 无限增长（仅执行一次）
func startLoginAttemptCleaner() {
	loginCleanupOnce.Do(func() {
		go func() {
			ticker := time.NewTicker(loginCleanupInterval)
			defer ticker.Stop()
			for range ticker.C {
				cleanupLoginAttempts()
			}
		}()
	})
}

// cleanupLoginAttempts 清理已过锁定期或长时间未活动的登录尝试记录
func cleanupLoginAttempts() {
	now := time.Now()
	loginAttemptsMu.Lock()
	defer loginAttemptsMu.Unlock()
	for key, attempt := range loginAttempts {
		if !attempt.lockedAt.IsZero() {
			// 已锁定：锁定期已过则回收
			if now.Sub(attempt.lockedAt) >= loginLockout {
				delete(loginAttempts, key)
			}
			continue
		}
		// 未锁定：长时间无新的失败尝试则回收
		if now.Sub(attempt.updatedAt) >= loginAttemptStaleWindow {
			delete(loginAttempts, key)
		}
	}
}

func loginAttemptKey(c *gin.Context, username string) string {
	return strings.TrimSpace(c.ClientIP()) + "|" + strings.ToLower(strings.TrimSpace(username))
}

func checkLoginAllowed(c *gin.Context, username string) bool {
	startLoginAttemptCleaner()

	key := loginAttemptKey(c, username)
	loginAttemptsMu.Lock()
	defer loginAttemptsMu.Unlock()

	attempt, ok := loginAttempts[key]
	if !ok {
		return true
	}
	if !attempt.lockedAt.IsZero() {
		if time.Since(attempt.lockedAt) < loginLockout {
			return false
		}
		delete(loginAttempts, key)
		return true
	}
	return true
}

func recordLoginFailure(c *gin.Context, username string) {
	key := loginAttemptKey(c, username)
	loginAttemptsMu.Lock()
	defer loginAttemptsMu.Unlock()

	attempt, ok := loginAttempts[key]
	if !ok {
		attempt = &loginAttempt{}
		loginAttempts[key] = attempt
	}
	attempt.failures++
	attempt.updatedAt = time.Now()
	if attempt.failures >= loginMaxFailures {
		attempt.lockedAt = time.Now()
	}
}

func clearLoginFailures(c *gin.Context, username string) {
	key := loginAttemptKey(c, username)
	loginAttemptsMu.Lock()
	defer loginAttemptsMu.Unlock()
	delete(loginAttempts, key)
}

func abortLoginRateLimited(c *gin.Context) {
	c.JSON(http.StatusTooManyRequests, gin.H{
		"error": "登录失败次数过多，请稍后再试",
	})
}
