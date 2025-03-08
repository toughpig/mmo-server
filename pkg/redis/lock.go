package redis

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/go-redis/redis/v8"
)

// LockConfig 定义锁配置
type LockConfig struct {
	// 默认锁超时时间
	DefaultTimeout time.Duration
	// 重试间隔
	RetryInterval time.Duration
	// 最大重试次数
	MaxRetries int
	// 是否自动续期
	AutoRenew bool
	// 续期间隔（小于超时时间的1/3）
	RenewInterval time.Duration
	// 锁前缀
	KeyPrefix string
}

// DefaultLockConfig 返回默认锁配置
func DefaultLockConfig() *LockConfig {
	return &LockConfig{
		DefaultTimeout: 30 * time.Second,
		RetryInterval:  100 * time.Millisecond,
		MaxRetries:     3,
		AutoRenew:      true,
		RenewInterval:  10 * time.Second,
		KeyPrefix:      "lock:",
	}
}

// Lock 分布式锁接口
type Lock interface {
	// 获取锁（阻塞直到获取成功或上下文取消）
	Acquire(ctx context.Context, key string) (string, error)
	// 尝试获取锁（非阻塞，立即返回）
	TryAcquire(ctx context.Context, key string, timeout time.Duration) (string, bool, error)
	// 释放锁
	Release(ctx context.Context, key string, token string) (bool, error)
	// 刷新锁（延长锁的过期时间）
	Refresh(ctx context.Context, key string, token string, timeout time.Duration) (bool, error)
	// 检查锁状态
	IsLocked(ctx context.Context, key string, token string) (bool, error)
}

// RedisLock 实现Redis分布式锁
type RedisLock struct {
	pool   Pool
	config *LockConfig
	// 追踪当前持有的锁，用于自动续期
	activeLocks     map[string]*lockInfo
	activeLocksMu   sync.RWMutex
	renewalStopChan chan string
}

// lockInfo 包含锁的详细信息
type lockInfo struct {
	key       string
	token     string
	renewChan chan struct{}
	timeout   time.Duration
}

// NewLock 创建新的Redis分布式锁
func NewLock(pool Pool, config *LockConfig) (Lock, error) {
	if pool == nil {
		return nil, errors.New("Redis连接池未初始化")
	}

	if config == nil {
		config = DefaultLockConfig()
	}

	// 验证配置
	if config.RenewInterval >= config.DefaultTimeout/3 {
		config.RenewInterval = config.DefaultTimeout / 3
		log.Printf("调整续期间隔为 %v (锁超时时间的1/3)", config.RenewInterval)
	}

	lock := &RedisLock{
		pool:            pool,
		config:          config,
		activeLocks:     make(map[string]*lockInfo),
		renewalStopChan: make(chan string, 100),
	}

	// 启动停止续期的处理循环
	go lock.handleStopRenewals()

	return lock, nil
}

// Acquire 获取锁（阻塞直到获取成功或上下文取消）
func (l *RedisLock) Acquire(ctx context.Context, key string) (string, error) {
	// 使用默认超时
	timeout := l.config.DefaultTimeout

	for {
		token, acquired, err := l.TryAcquire(ctx, key, timeout)
		if err != nil {
			return "", err
		}

		if acquired {
			return token, nil
		}

		// 检查上下文是否已取消
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(l.config.RetryInterval):
			// 重试
		}
	}
}

// TryAcquire 尝试获取锁（非阻塞，立即返回）
func (l *RedisLock) TryAcquire(ctx context.Context, key string, timeout time.Duration) (string, bool, error) {
	// 生成随机令牌
	token, err := l.generateToken()
	if err != nil {
		return "", false, fmt.Errorf("生成锁令牌失败: %w", err)
	}

	// 使用默认超时（如果未指定）
	if timeout <= 0 {
		timeout = l.config.DefaultTimeout
	}

	lockKey := l.formatKey(key)
	client := l.pool.GetClient()

	// 尝试设置锁，使用NX选项确保键不存在时才设置
	success, err := client.SetNX(ctx, lockKey, token, timeout).Result()
	if err != nil {
		return "", false, fmt.Errorf("获取锁失败: %w", err)
	}

	if !success {
		return "", false, nil
	}

	log.Printf("获取锁成功: %s, 令牌: %s, 超时: %v", key, token, timeout)

	// 如果启用自动续期，启动续期协程
	if l.config.AutoRenew {
		l.startAutoRenew(key, token, timeout)
	}

	return token, true, nil
}

// Release 释放锁
func (l *RedisLock) Release(ctx context.Context, key string, token string) (bool, error) {
	lockKey := l.formatKey(key)
	client := l.pool.GetClient()

	// 使用Lua脚本确保只释放自己的锁
	// 1. 获取当前锁的令牌
	// 2. 如果令牌匹配，删除锁
	// 3. 返回是否成功删除
	script := `
		if redis.call("GET", KEYS[1]) == ARGV[1] then
			return redis.call("DEL", KEYS[1])
		else
			return 0
		end
	`

	result, err := client.Eval(ctx, script, []string{lockKey}, token).Int64()
	if err != nil {
		return false, fmt.Errorf("释放锁失败: %w", err)
	}

	success := result == 1

	// 如果释放成功且启用了自动续期，停止续期
	if success && l.config.AutoRenew {
		l.stopAutoRenew(key)
	}

	if success {
		log.Printf("释放锁成功: %s, 令牌: %s", key, token)
	} else {
		log.Printf("释放锁失败（可能已过期或由其他客户端持有）: %s, 令牌: %s", key, token)
	}

	return success, nil
}

// Refresh 刷新锁（延长锁的过期时间）
func (l *RedisLock) Refresh(ctx context.Context, key string, token string, timeout time.Duration) (bool, error) {
	lockKey := l.formatKey(key)
	client := l.pool.GetClient()

	// 使用Lua脚本确保只刷新自己的锁
	// 1. 获取当前锁的令牌
	// 2. 如果令牌匹配，刷新过期时间
	// 3. 返回是否成功刷新
	script := `
		if redis.call("GET", KEYS[1]) == ARGV[1] then
			return redis.call("PEXPIRE", KEYS[1], ARGV[2])
		else
			return 0
		end
	`

	// 使用默认超时（如果未指定）
	if timeout <= 0 {
		timeout = l.config.DefaultTimeout
	}

	// 转换为毫秒
	timeoutMs := int64(timeout / time.Millisecond)

	result, err := client.Eval(ctx, script, []string{lockKey}, token, timeoutMs).Int64()
	if err != nil {
		return false, fmt.Errorf("刷新锁失败: %w", err)
	}

	success := result == 1

	if success {
		log.Printf("刷新锁成功: %s, 令牌: %s, 新超时: %v", key, token, timeout)
	} else {
		log.Printf("刷新锁失败（可能已过期或由其他客户端持有）: %s, 令牌: %s", key, token)
	}

	// 更新活跃锁信息
	if success && l.config.AutoRenew {
		l.activeLocksMu.Lock()
		if info, exists := l.activeLocks[key]; exists && info.token == token {
			info.timeout = timeout
		}
		l.activeLocksMu.Unlock()
	}

	return success, nil
}

// IsLocked 检查锁状态
func (l *RedisLock) IsLocked(ctx context.Context, key string, token string) (bool, error) {
	lockKey := l.formatKey(key)
	client := l.pool.GetClient()

	// 获取当前锁的令牌
	currentToken, err := client.Get(ctx, lockKey).Result()
	if err != nil {
		if err == redis.Nil {
			// 键不存在，未锁定
			return false, nil
		}
		return false, fmt.Errorf("检查锁状态失败: %w", err)
	}

	// 如果提供了令牌，检查是否匹配
	if token != "" {
		return currentToken == token, nil
	}

	// 如果未提供令牌，只要键存在就表示已锁定
	return true, nil
}

// formatKey 格式化锁键
func (l *RedisLock) formatKey(key string) string {
	return l.config.KeyPrefix + key
}

// generateToken 生成随机令牌
func (l *RedisLock) generateToken() (string, error) {
	b := make([]byte, 16)
	_, err := rand.Read(b)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// startAutoRenew 启动自动续期
func (l *RedisLock) startAutoRenew(key string, token string, timeout time.Duration) {
	l.activeLocksMu.Lock()
	defer l.activeLocksMu.Unlock()

	// 创建锁信息
	info := &lockInfo{
		key:       key,
		token:     token,
		renewChan: make(chan struct{}),
		timeout:   timeout,
	}

	// 存储锁信息
	l.activeLocks[key] = info

	// 启动续期协程
	go func() {
		// 计算续期间隔，为超时时间的1/3
		interval := l.config.RenewInterval
		if interval <= 0 || interval >= timeout/3 {
			interval = timeout / 3
		}

		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				// 执行续期
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				success, err := l.Refresh(ctx, key, token, timeout)
				cancel()

				if err != nil {
					log.Printf("自动续期失败: %s, 令牌: %s, 错误: %v", key, token, err)
				} else if !success {
					log.Printf("自动续期失败（可能已过期或由其他客户端持有）: %s, 令牌: %s", key, token)
					l.stopAutoRenew(key)
					return
				}

			case <-info.renewChan:
				// 收到停止信号
				log.Printf("停止自动续期: %s, 令牌: %s", key, token)
				return
			}
		}
	}()
}

// stopAutoRenew 停止自动续期
func (l *RedisLock) stopAutoRenew(key string) {
	// 发送停止信号到处理协程
	l.renewalStopChan <- key
}

// handleStopRenewals 处理停止续期请求
func (l *RedisLock) handleStopRenewals() {
	for key := range l.renewalStopChan {
		l.activeLocksMu.Lock()
		if info, exists := l.activeLocks[key]; exists {
			close(info.renewChan)
			delete(l.activeLocks, key)
		}
		l.activeLocksMu.Unlock()
	}
}
