package db

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/jackc/pgconn"
	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/pgxpool"
)

// DBConfig 定义数据库连接池配置
type DBConfig struct {
	// 连接URL，格式：postgres://username:password@host:port/database
	ConnectionURL string
	// 最大连接数
	MaxConnections int32
	// 最小连接数
	MinConnections int32
	// 最大连接空闲时间
	MaxConnIdleTime time.Duration
	// 连接最大寿命
	MaxConnLifetime time.Duration
	// 健康检查周期
	HealthCheckPeriod time.Duration
	// 连接超时时间
	ConnectTimeout time.Duration
	// 查询超时时间
	QueryTimeout time.Duration
}

// DefaultDBConfig 返回默认数据库配置
func DefaultDBConfig() *DBConfig {
	return &DBConfig{
		ConnectionURL:     "postgres://postgres:postgres@localhost:5432/postgres",
		MaxConnections:    20,
		MinConnections:    5,
		MaxConnIdleTime:   5 * time.Minute,
		MaxConnLifetime:   1 * time.Hour,
		HealthCheckPeriod: 1 * time.Minute,
		ConnectTimeout:    10 * time.Second,
		QueryTimeout:      30 * time.Second,
	}
}

// PoolStats 连接池统计信息
type PoolStats struct {
	TotalConnections      int64
	IdleConnections       int64
	ActiveConnections     int64
	MaxConnections        int32
	AcquireCount          int64
	AcquireDurationAvgMs  float64
	QueriesExecuted       int64
	AvgQueryDurationMs    float64
	LastHealthCheckTime   time.Time
	LastHealthCheckStatus string
}

// Pool 是数据库连接池接口
type Pool interface {
	// 获取数据库连接
	Acquire(ctx context.Context) (*pgxpool.Conn, error)
	// 执行查询并返回结果
	Query(ctx context.Context, sql string, args ...interface{}) (pgx.Rows, error)
	// 执行查询并返回单行结果
	QueryRow(ctx context.Context, sql string, args ...interface{}) pgx.Row
	// 执行命令并返回影响的行数
	Exec(ctx context.Context, sql string, args ...interface{}) (pgconn.CommandTag, error)
	// 开始事务
	Begin(ctx context.Context) (pgx.Tx, error)
	// 执行健康检查
	HealthCheck(ctx context.Context) error
	// 获取统计信息
	Stats() *PoolStats
	// 关闭连接池
	Close()
}

// pgConnPool 实现数据库连接池
type pgConnPool struct {
	pool     *pgxpool.Pool
	config   *DBConfig
	mu       sync.RWMutex
	stats    PoolStats
	queryLog *sync.Map // 记录查询性能
}

// NewConnectionPool 创建数据库连接池
func NewConnectionPool(config *DBConfig) (Pool, error) {
	if config == nil {
		config = DefaultDBConfig()
	}

	// 创建pgx连接池配置
	poolConfig, err := pgxpool.ParseConfig(config.ConnectionURL)
	if err != nil {
		return nil, fmt.Errorf("解析连接URL失败: %w", err)
	}

	// 应用自定义配置
	poolConfig.MaxConns = config.MaxConnections
	poolConfig.MinConns = config.MinConnections
	poolConfig.MaxConnIdleTime = config.MaxConnIdleTime
	poolConfig.MaxConnLifetime = config.MaxConnLifetime
	poolConfig.HealthCheckPeriod = config.HealthCheckPeriod
	poolConfig.ConnConfig.ConnectTimeout = config.ConnectTimeout

	// 创建连接池
	pool, err := pgxpool.ConnectConfig(context.Background(), poolConfig)
	if err != nil {
		return nil, fmt.Errorf("连接数据库失败: %w", err)
	}

	dbPool := &pgConnPool{
		pool:     pool,
		config:   config,
		queryLog: &sync.Map{},
	}

	// 执行初始健康检查
	ctx, cancel := context.WithTimeout(context.Background(), config.ConnectTimeout)
	defer cancel()

	if err := dbPool.HealthCheck(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("数据库健康检查失败: %w", err)
	}

	log.Printf("数据库连接池已初始化，最大连接数: %d", config.MaxConnections)
	return dbPool, nil
}

// Acquire 获取数据库连接
func (p *pgConnPool) Acquire(ctx context.Context) (*pgxpool.Conn, error) {
	// 如果提供了查询超时，使用它创建新的上下文
	if p.config.QueryTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, p.config.QueryTimeout)
		defer cancel()
	}

	conn, err := p.pool.Acquire(ctx)
	if err != nil {
		return nil, fmt.Errorf("获取数据库连接失败: %w", err)
	}

	return conn, nil
}

// Query 执行查询并返回结果
func (p *pgConnPool) Query(ctx context.Context, sql string, args ...interface{}) (pgx.Rows, error) {
	startTime := time.Now()

	// 如果提供了查询超时，使用它创建新的上下文
	if p.config.QueryTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, p.config.QueryTimeout)
		defer cancel()
	}

	rows, err := p.pool.Query(ctx, sql, args...)

	// 记录查询统计信息
	duration := time.Since(startTime)
	p.recordQueryStats(sql, duration)

	if err != nil {
		return nil, fmt.Errorf("执行查询失败: %w", err)
	}

	return rows, nil
}

// QueryRow 执行查询并返回单行结果
func (p *pgConnPool) QueryRow(ctx context.Context, sql string, args ...interface{}) pgx.Row {
	startTime := time.Now()

	// 如果提供了查询超时，使用它创建新的上下文
	if p.config.QueryTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, p.config.QueryTimeout)
		defer cancel()
	}

	row := p.pool.QueryRow(ctx, sql, args...)

	// 记录查询统计信息
	duration := time.Since(startTime)
	p.recordQueryStats(sql, duration)

	return row
}

// Exec 执行命令并返回影响的行数
func (p *pgConnPool) Exec(ctx context.Context, sql string, args ...interface{}) (pgconn.CommandTag, error) {
	startTime := time.Now()

	// 如果提供了查询超时，使用它创建新的上下文
	if p.config.QueryTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, p.config.QueryTimeout)
		defer cancel()
	}

	tag, err := p.pool.Exec(ctx, sql, args...)

	// 记录查询统计信息
	duration := time.Since(startTime)
	p.recordQueryStats(sql, duration)

	if err != nil {
		return nil, fmt.Errorf("执行命令失败: %w", err)
	}

	return tag, nil
}

// Begin 开始事务
func (p *pgConnPool) Begin(ctx context.Context) (pgx.Tx, error) {
	// 如果提供了查询超时，使用它创建新的上下文
	if p.config.QueryTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, p.config.QueryTimeout)
		defer cancel()
	}

	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("开始事务失败: %w", err)
	}

	return tx, nil
}

// HealthCheck 执行健康检查
func (p *pgConnPool) HealthCheck(ctx context.Context) error {
	// 如果提供了查询超时，使用它创建新的上下文
	if p.config.QueryTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, p.config.QueryTimeout)
		defer cancel()
	}

	// 执行简单查询检查连接健康
	var result int
	err := p.pool.QueryRow(ctx, "SELECT 1").Scan(&result)

	// 更新健康检查状态
	p.mu.Lock()
	p.stats.LastHealthCheckTime = time.Now()
	if err != nil {
		p.stats.LastHealthCheckStatus = fmt.Sprintf("失败: %v", err)
		p.mu.Unlock()
		return fmt.Errorf("数据库健康检查失败: %w", err)
	} else {
		p.stats.LastHealthCheckStatus = "成功"
		p.mu.Unlock()
	}

	return nil
}

// Stats 获取统计信息
func (p *pgConnPool) Stats() *PoolStats {
	p.mu.Lock()
	defer p.mu.Unlock()

	// 获取pgx连接池统计信息
	poolStats := p.pool.Stat()

	// 更新统计信息
	p.stats.TotalConnections = int64(poolStats.TotalConns())
	p.stats.IdleConnections = int64(poolStats.IdleConns())
	p.stats.ActiveConnections = int64(poolStats.AcquiredConns())
	p.stats.MaxConnections = poolStats.MaxConns()
	p.stats.AcquireCount = poolStats.AcquireCount()
	p.stats.AcquireDurationAvgMs = float64(poolStats.AcquireDuration().Milliseconds()) / float64(max(1, poolStats.AcquireCount()))

	return &p.stats
}

// Close 关闭连接池
func (p *pgConnPool) Close() {
	p.pool.Close()
	log.Println("数据库连接池已关闭")
}

// recordQueryStats 记录查询统计信息
func (p *pgConnPool) recordQueryStats(sql string, duration time.Duration) {
	p.mu.Lock()
	defer p.mu.Unlock()

	// 增加查询计数
	p.stats.QueriesExecuted++

	// 更新平均查询时间
	currentAvg := p.stats.AvgQueryDurationMs
	currentCount := float64(p.stats.QueriesExecuted - 1)
	newDurationMs := float64(duration.Milliseconds())

	// 计算新的平均值
	if currentCount > 0 {
		p.stats.AvgQueryDurationMs = (currentAvg*currentCount + newDurationMs) / float64(p.stats.QueriesExecuted)
	} else {
		p.stats.AvgQueryDurationMs = newDurationMs
	}

	// 可选：在queryLog中记录查询性能
	if queryInfo, exists := p.queryLog.Load(sql); exists {
		info := queryInfo.(struct {
			Count     int64
			TotalTime time.Duration
		})
		info.Count++
		info.TotalTime += duration
		p.queryLog.Store(sql, info)
	} else {
		p.queryLog.Store(sql, struct {
			Count     int64
			TotalTime time.Duration
		}{
			Count:     1,
			TotalTime: duration,
		})
	}
}

// max 返回两个int64中的较大值
func max(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}
