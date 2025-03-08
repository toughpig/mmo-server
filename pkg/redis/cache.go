package redis

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/go-redis/redis/v8"
)

// CacheConfig 定义缓存配置
type CacheConfig struct {
	// 默认过期时间
	DefaultTTL time.Duration
	// 是否压缩数据（未实现）
	Compress bool
	// 压缩阈值，大于此值才会压缩（未实现）
	CompressMinSize int
	// 是否启用JSON序列化
	EnableJSON bool
	// 统计信息收集间隔
	StatsCollectionInterval time.Duration
}

// DefaultCacheConfig 返回默认缓存配置
func DefaultCacheConfig() *CacheConfig {
	return &CacheConfig{
		DefaultTTL:              1 * time.Hour,
		Compress:                false,
		CompressMinSize:         1024, // 1KB
		EnableJSON:              true,
		StatsCollectionInterval: 5 * time.Minute,
	}
}

// CacheStats 包含缓存统计信息
type CacheStats struct {
	Hits              int64
	Misses            int64
	Errors            int64
	TotalItems        int64
	ExpiredItems      int64
	EvictedItems      int64
	TotalMemoryBytes  int64
	LastCollectedTime time.Time
}

// Cache 是Redis缓存层的接口定义
type Cache interface {
	// 获取单个对象
	Get(ctx context.Context, key string, dest interface{}) error
	// 设置单个对象
	Set(ctx context.Context, key string, value interface{}, ttl time.Duration) error
	// 删除单个对象
	Delete(ctx context.Context, key string) error
	// 检查键是否存在
	Exists(ctx context.Context, key string) (bool, error)
	// 获取多个对象
	MGet(ctx context.Context, keys []string) ([]interface{}, error)
	// 设置多个对象
	MSet(ctx context.Context, items map[string]interface{}) error
	// 递增值
	Incr(ctx context.Context, key string) (int64, error)
	// 设置过期时间
	Expire(ctx context.Context, key string, ttl time.Duration) error
	// 获取所有匹配模式的键
	Keys(ctx context.Context, pattern string) ([]string, error)
	// 获取缓存统计信息
	Stats() *CacheStats
}

// RedisCache 实现Redis缓存层
type RedisCache struct {
	pool   Pool
	config *CacheConfig
	stats  CacheStats
}

// NewCache 创建新的Redis缓存
func NewCache(pool Pool, config *CacheConfig) (Cache, error) {
	if pool == nil {
		return nil, errors.New("Redis连接池未初始化")
	}

	if config == nil {
		config = DefaultCacheConfig()
	}

	cache := &RedisCache{
		pool:   pool,
		config: config,
	}

	// 验证Redis连接
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	client := pool.GetClient()
	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("Redis连接验证失败: %w", err)
	}

	log.Printf("Redis缓存已初始化，默认TTL: %v, JSON模式: %v",
		config.DefaultTTL, config.EnableJSON)

	return cache, nil
}

// Get 从缓存获取对象
func (c *RedisCache) Get(ctx context.Context, key string, dest interface{}) error {
	client := c.pool.GetClient()

	val, err := client.Get(ctx, key).Result()
	if err != nil {
		if err == redis.Nil {
			c.stats.Misses++
			return fmt.Errorf("缓存未命中: %s", key)
		}
		c.stats.Errors++
		return fmt.Errorf("从Redis获取数据失败: %w", err)
	}

	c.stats.Hits++

	// 如果启用了JSON模式，尝试将字符串解析为JSON
	if c.config.EnableJSON && dest != nil {
		if err := json.Unmarshal([]byte(val), dest); err != nil {
			c.stats.Errors++
			return fmt.Errorf("解析JSON数据失败: %w", err)
		}
		return nil
	}

	// 如果没有启用JSON模式或dest为nil，则直接返回字符串
	// 注意：这里需要确保dest是一个字符串指针
	if strPtr, ok := dest.(*string); ok {
		*strPtr = val
		return nil
	}

	return fmt.Errorf("无法将Redis值转换为目标类型")
}

// Set 设置缓存对象
func (c *RedisCache) Set(ctx context.Context, key string, value interface{}, ttl time.Duration) error {
	client := c.pool.GetClient()

	// 如果ttl为0，使用默认TTL
	if ttl == 0 {
		ttl = c.config.DefaultTTL
	}

	var val string
	var err error

	// 如果启用了JSON模式，将对象序列化为JSON
	if c.config.EnableJSON {
		var jsonBytes []byte
		jsonBytes, err = json.Marshal(value)
		if err != nil {
			c.stats.Errors++
			return fmt.Errorf("序列化对象为JSON失败: %w", err)
		}
		val = string(jsonBytes)
	} else {
		// 如果没有启用JSON模式，尝试直接将值转换为字符串
		switch v := value.(type) {
		case string:
			val = v
		case []byte:
			val = string(v)
		default:
			val = fmt.Sprintf("%v", v)
		}
	}

	// 设置值到Redis
	err = client.Set(ctx, key, val, ttl).Err()
	if err != nil {
		c.stats.Errors++
		return fmt.Errorf("设置缓存失败: %w", err)
	}

	return nil
}

// Delete 删除缓存对象
func (c *RedisCache) Delete(ctx context.Context, key string) error {
	client := c.pool.GetClient()

	result, err := client.Del(ctx, key).Result()
	if err != nil {
		c.stats.Errors++
		return fmt.Errorf("删除缓存失败: %w", err)
	}

	// 如果结果为0，表示键不存在
	if result == 0 {
		return fmt.Errorf("缓存键不存在: %s", key)
	}

	return nil
}

// Exists 检查键是否存在
func (c *RedisCache) Exists(ctx context.Context, key string) (bool, error) {
	client := c.pool.GetClient()

	result, err := client.Exists(ctx, key).Result()
	if err != nil {
		c.stats.Errors++
		return false, fmt.Errorf("检查缓存键失败: %w", err)
	}

	return result > 0, nil
}

// MGet 获取多个对象
func (c *RedisCache) MGet(ctx context.Context, keys []string) ([]interface{}, error) {
	if len(keys) == 0 {
		return []interface{}{}, nil
	}

	client := c.pool.GetClient()

	vals, err := client.MGet(ctx, keys...).Result()
	if err != nil {
		c.stats.Errors++
		return nil, fmt.Errorf("批量获取缓存失败: %w", err)
	}

	// 统计命中和未命中次数
	for _, val := range vals {
		if val == nil {
			c.stats.Misses++
		} else {
			c.stats.Hits++
		}
	}

	return vals, nil
}

// MSet 设置多个对象
func (c *RedisCache) MSet(ctx context.Context, items map[string]interface{}) error {
	if len(items) == 0 {
		return nil
	}

	client := c.pool.GetClient()

	// 将map转换为slice，格式为[key1, val1, key2, val2, ...]
	pairs := make([]interface{}, 0, len(items)*2)
	for k, v := range items {
		var val interface{}

		// 如果启用了JSON模式，将对象序列化为JSON
		if c.config.EnableJSON {
			jsonBytes, err := json.Marshal(v)
			if err != nil {
				c.stats.Errors++
				return fmt.Errorf("序列化对象为JSON失败: %w", err)
			}
			val = string(jsonBytes)
		} else {
			// 尝试直接将值转换为字符串
			switch vt := v.(type) {
			case string:
				val = vt
			case []byte:
				val = string(vt)
			default:
				val = fmt.Sprintf("%v", vt)
			}
		}

		pairs = append(pairs, k, val)
	}

	// 批量设置值到Redis
	err := client.MSet(ctx, pairs...).Err()
	if err != nil {
		c.stats.Errors++
		return fmt.Errorf("批量设置缓存失败: %w", err)
	}

	return nil
}

// Incr 递增值
func (c *RedisCache) Incr(ctx context.Context, key string) (int64, error) {
	client := c.pool.GetClient()

	val, err := client.Incr(ctx, key).Result()
	if err != nil {
		c.stats.Errors++
		return 0, fmt.Errorf("递增值失败: %w", err)
	}

	return val, nil
}

// Expire 设置过期时间
func (c *RedisCache) Expire(ctx context.Context, key string, ttl time.Duration) error {
	client := c.pool.GetClient()

	success, err := client.Expire(ctx, key, ttl).Result()
	if err != nil {
		c.stats.Errors++
		return fmt.Errorf("设置过期时间失败: %w", err)
	}

	if !success {
		return fmt.Errorf("键不存在，无法设置过期时间: %s", key)
	}

	return nil
}

// Keys 获取所有匹配模式的键
func (c *RedisCache) Keys(ctx context.Context, pattern string) ([]string, error) {
	client := c.pool.GetClient()

	keys, err := client.Keys(ctx, pattern).Result()
	if err != nil {
		c.stats.Errors++
		return nil, fmt.Errorf("查询键失败: %w", err)
	}

	return keys, nil
}

// Stats 返回缓存统计信息
func (c *RedisCache) Stats() *CacheStats {
	// 这里可以添加更多统计信息的收集逻辑
	return &c.stats
}
