package redis

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/go-redis/redis/v8"
)

// PoolConfig 定义连接池配置
type PoolConfig struct {
	// Redis服务器地址，格式为 host:port
	Addresses []string
	// 密码，如果有的话
	Password string
	// 数据库编号
	DB int
	// 连接池最小空闲连接数
	MinIdleConns int
	// 连接池最大连接数
	PoolSize int
	// 连接最大空闲时间，超时后连接将被关闭
	IdleTimeout time.Duration
	// 读取超时时间
	ReadTimeout time.Duration
	// 写入超时时间
	WriteTimeout time.Duration
	// 连接超时时间
	DialTimeout time.Duration
	// 是否使用集群模式
	UseCluster bool
	// 最大重试次数
	MaxRetries int
	// 最小重试间隔
	MinRetryBackoff time.Duration
	// 最大重试间隔
	MaxRetryBackoff time.Duration
}

// DefaultConfig 返回默认配置
func DefaultConfig() *PoolConfig {
	return &PoolConfig{
		Addresses:       []string{"localhost:6379"},
		Password:        "",
		DB:              0,
		MinIdleConns:    10,
		PoolSize:        100,
		IdleTimeout:     5 * time.Minute,
		ReadTimeout:     3 * time.Second,
		WriteTimeout:    3 * time.Second,
		DialTimeout:     5 * time.Second,
		UseCluster:      false,
		MaxRetries:      3,
		MinRetryBackoff: 8 * time.Millisecond,
		MaxRetryBackoff: 512 * time.Millisecond,
	}
}

// Pool 是Redis连接池的接口定义
type Pool interface {
	// 获取Redis客户端
	GetClient() redis.Cmdable
	// 关闭连接池
	Close() error
	// 检查连接池健康状态
	HealthCheck(ctx context.Context) error
	// 获取集群健康信息
	GetClusterInfo(ctx context.Context) (map[string]string, error)
	// 返回连接池统计信息
	Stats() *PoolStats
}

// PoolStats 包含连接池统计信息
type PoolStats struct {
	TotalConnections       int
	IdleConnections        int
	StaleConnections       int
	ActiveConnections      int
	Hits                   int64
	Misses                 int64
	Timeouts               int64
	TotalCommandsProcessed int64
}

// ConnectionPool 实现Redis连接池
type ConnectionPool struct {
	client        redis.Cmdable
	config        *PoolConfig
	mu            sync.RWMutex
	stats         PoolStats
	clusterClient *redis.ClusterClient
	singleClient  *redis.Client
}

// NewConnectionPool 创建新的连接池
func NewConnectionPool(config *PoolConfig) (Pool, error) {
	if config == nil {
		config = DefaultConfig()
	}

	pool := &ConnectionPool{
		config: config,
		stats:  PoolStats{},
	}

	var err error
	if config.UseCluster {
		err = pool.initClusterClient()
	} else {
		err = pool.initSingleClient()
	}

	if err != nil {
		return nil, fmt.Errorf("初始化Redis连接池失败: %w", err)
	}

	// 验证连接是否可用
	ctx, cancel := context.WithTimeout(context.Background(), pool.config.DialTimeout)
	defer cancel()

	if err := pool.HealthCheck(ctx); err != nil {
		return nil, fmt.Errorf("Redis连接健康检查失败: %w", err)
	}

	log.Printf("Redis连接池已初始化, 地址: %v, 集群模式: %v", config.Addresses, config.UseCluster)
	return pool, nil
}

// initSingleClient 初始化单节点Redis客户端
func (p *ConnectionPool) initSingleClient() error {
	if len(p.config.Addresses) == 0 {
		return errors.New("未提供Redis地址")
	}

	p.singleClient = redis.NewClient(&redis.Options{
		Addr:            p.config.Addresses[0],
		Password:        p.config.Password,
		DB:              p.config.DB,
		MinIdleConns:    p.config.MinIdleConns,
		PoolSize:        p.config.PoolSize,
		IdleTimeout:     p.config.IdleTimeout,
		ReadTimeout:     p.config.ReadTimeout,
		WriteTimeout:    p.config.WriteTimeout,
		DialTimeout:     p.config.DialTimeout,
		MaxRetries:      p.config.MaxRetries,
		MinRetryBackoff: p.config.MinRetryBackoff,
		MaxRetryBackoff: p.config.MaxRetryBackoff,
	})

	p.client = p.singleClient
	return nil
}

// initClusterClient 初始化Redis集群客户端
func (p *ConnectionPool) initClusterClient() error {
	if len(p.config.Addresses) == 0 {
		return errors.New("未提供Redis集群地址")
	}

	p.clusterClient = redis.NewClusterClient(&redis.ClusterOptions{
		Addrs:           p.config.Addresses,
		Password:        p.config.Password,
		MinIdleConns:    p.config.MinIdleConns,
		PoolSize:        p.config.PoolSize,
		IdleTimeout:     p.config.IdleTimeout,
		ReadTimeout:     p.config.ReadTimeout,
		WriteTimeout:    p.config.WriteTimeout,
		DialTimeout:     p.config.DialTimeout,
		MaxRetries:      p.config.MaxRetries,
		MinRetryBackoff: p.config.MinRetryBackoff,
		MaxRetryBackoff: p.config.MaxRetryBackoff,
	})

	p.client = p.clusterClient
	return nil
}

// GetClient 返回Redis客户端
func (p *ConnectionPool) GetClient() redis.Cmdable {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.client
}

// Close 关闭连接池
func (p *ConnectionPool) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	var err error
	if p.config.UseCluster && p.clusterClient != nil {
		err = p.clusterClient.Close()
	} else if p.singleClient != nil {
		err = p.singleClient.Close()
	}

	if err != nil {
		return fmt.Errorf("关闭Redis连接池失败: %w", err)
	}

	log.Printf("Redis连接池已关闭")
	return nil
}

// HealthCheck 检查连接池健康状态
func (p *ConnectionPool) HealthCheck(ctx context.Context) error {
	client := p.GetClient()
	if client == nil {
		return errors.New("Redis客户端未初始化")
	}

	status := client.Ping(ctx)
	if status.Err() != nil {
		return fmt.Errorf("Redis健康检查失败: %w", status.Err())
	}

	result, err := status.Result()
	if err != nil {
		return fmt.Errorf("解析Redis健康检查结果失败: %w", err)
	}

	if result != "PONG" {
		return fmt.Errorf("Redis健康检查返回异常: %s", result)
	}

	return nil
}

// GetClusterInfo 获取集群信息
func (p *ConnectionPool) GetClusterInfo(ctx context.Context) (map[string]string, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if !p.config.UseCluster {
		return nil, errors.New("当前不是集群模式")
	}

	if p.clusterClient == nil {
		return nil, errors.New("集群客户端未初始化")
	}

	result := make(map[string]string)

	// 获取集群节点信息
	clusterNodes, err := p.clusterClient.ClusterNodes(ctx).Result()
	if err != nil {
		return nil, fmt.Errorf("获取集群节点失败: %w", err)
	}
	result["nodes"] = clusterNodes

	// 获取集群状态
	clusterInfo, err := p.clusterClient.ClusterInfo(ctx).Result()
	if err != nil {
		return nil, fmt.Errorf("获取集群信息失败: %w", err)
	}
	result["info"] = clusterInfo

	return result, nil
}

// Stats 返回连接池统计信息
func (p *ConnectionPool) Stats() *PoolStats {
	p.mu.RLock()
	defer p.mu.RUnlock()

	var redisPoolStats *redis.PoolStats

	if p.config.UseCluster && p.clusterClient != nil {
		redisPoolStats = p.clusterClient.PoolStats()
	} else if p.singleClient != nil {
		redisPoolStats = p.singleClient.PoolStats()
	}

	// 更新统计信息
	if redisPoolStats != nil {
		p.stats.TotalConnections = int(redisPoolStats.TotalConns)
		p.stats.IdleConnections = int(redisPoolStats.IdleConns)
		p.stats.StaleConnections = int(redisPoolStats.StaleConns)
		p.stats.ActiveConnections = int(redisPoolStats.TotalConns - redisPoolStats.IdleConns)
	}

	return &p.stats
}
