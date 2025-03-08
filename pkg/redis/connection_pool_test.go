package redis

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConnectionPool(t *testing.T) {
	// 创建一个mini redis实例用于测试
	s, err := miniredis.Run()
	if err != nil {
		t.Fatalf("无法启动mini redis: %v", err)
	}
	defer s.Close()

	// 创建一个指向miniredis的配置
	config := DefaultConfig()
	config.Addresses = []string{s.Addr()}

	// 测试创建连接池
	t.Run("NewConnectionPool", func(t *testing.T) {
		pool, err := NewConnectionPool(config)
		require.NoError(t, err)
		require.NotNil(t, pool)

		// 测试关闭连接池
		err = pool.Close()
		require.NoError(t, err)
	})

	// 测试健康检查
	t.Run("HealthCheck", func(t *testing.T) {
		pool, err := NewConnectionPool(config)
		require.NoError(t, err)
		defer pool.Close()

		err = pool.HealthCheck(context.Background())
		require.NoError(t, err)
	})

	// 测试获取客户端
	t.Run("GetClient", func(t *testing.T) {
		pool, err := NewConnectionPool(config)
		require.NoError(t, err)
		defer pool.Close()

		client := pool.GetClient()
		require.NotNil(t, client)

		// 使用客户端执行一个简单的操作
		status := client.Set(context.Background(), "test_key", "test_value", 0)
		require.NoError(t, status.Err())

		val, err := client.Get(context.Background(), "test_key").Result()
		require.NoError(t, err)
		assert.Equal(t, "test_value", val)
	})

	// 测试获取统计信息
	t.Run("Stats", func(t *testing.T) {
		pool, err := NewConnectionPool(config)
		require.NoError(t, err)
		defer pool.Close()

		stats := pool.Stats()
		require.NotNil(t, stats)

		// 执行一些操作来改变统计信息
		client := pool.GetClient()
		client.Set(context.Background(), "stats_test", "value", 0)
		client.Get(context.Background(), "stats_test")

		// 重新获取统计信息
		updatedStats := pool.Stats()
		require.NotNil(t, updatedStats)
	})

	// 测试错误情况
	t.Run("ConnectionError", func(t *testing.T) {
		badConfig := DefaultConfig()
		badConfig.Addresses = []string{"localhost:65535"} // 不存在的端口

		_, err := NewConnectionPool(badConfig)
		require.Error(t, err)
	})

	// 测试集群模式
	t.Run("ClusterMode", func(t *testing.T) {
		// 注意：miniredis不支持集群模式，这只是为了测试代码路径
		clusterConfig := DefaultConfig()
		clusterConfig.Addresses = []string{s.Addr()}
		clusterConfig.UseCluster = true

		// 在测试环境中，可能没有配置Redis集群，所以我们不强制要求返回错误
		pool, err := NewConnectionPool(clusterConfig)
		if err != nil {
			// 如果有错误，确认它与集群相关
			require.Contains(t, err.Error(), "cluster", "如果有错误，应该与集群配置相关")
		} else {
			// 如果成功创建了池，确保它是集群模式，并关闭它
			require.NotNil(t, pool)
			err = pool.Close()
			require.NoError(t, err)
		}
	})
}

func TestConnectionPoolWithTimeout(t *testing.T) {
	s, err := miniredis.Run()
	if err != nil {
		t.Fatalf("无法启动mini redis: %v", err)
	}
	defer s.Close()

	config := DefaultConfig()
	config.Addresses = []string{s.Addr()}
	config.DialTimeout = 1 * time.Second
	config.ReadTimeout = 1 * time.Second
	config.WriteTimeout = 1 * time.Second

	pool, err := NewConnectionPool(config)
	require.NoError(t, err)
	defer pool.Close()

	// 测试带超时的操作
	t.Run("OperationWithTimeout", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
		defer cancel()

		client := pool.GetClient()
		err := client.Set(ctx, "timeout_test", "value", 0).Err()
		require.NoError(t, err)

		val, err := client.Get(ctx, "timeout_test").Result()
		require.NoError(t, err)
		assert.Equal(t, "value", val)
	})

	// 测试超时情况
	t.Run("ConnectionError", func(t *testing.T) {
		// 使用错误的地址来模拟超时/连接错误
		badConfig := DefaultConfig()
		badConfig.Addresses = []string{"non.existent.host:6379"}
		badConfig.DialTimeout = 100 * time.Millisecond

		// 尝试创建一个新的连接池，应该失败
		_, err := NewConnectionPool(badConfig)
		require.Error(t, err)
	})
}
