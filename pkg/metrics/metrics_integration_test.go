package metrics

import (
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMetricsIntegration 测试Metrics系统的完整集成
func TestMetricsIntegration(t *testing.T) {
	// 创建Metrics管理器
	manager := NewMetricsManager()
	require.NotNil(t, manager)

	// 配置测试端口
	port := 19090
	address := fmt.Sprintf(":%d", port)

	// 启动HTTP服务器
	err := manager.StartServer(address)
	require.NoError(t, err)
	defer manager.StopServer()

	// 给服务器一些时间启动
	time.Sleep(100 * time.Millisecond)

	// 注册一个计数器
	err = manager.RegisterCounter("test_counter", "Test counter for integration test", "label1", "label2")
	require.NoError(t, err)

	// 注册一个仪表盘
	err = manager.RegisterGauge("test_gauge", "Test gauge for integration test", "label1")
	require.NoError(t, err)

	// 注册一个直方图
	err = manager.RegisterHistogram("test_histogram", "Test histogram for integration test",
		[]float64{0.01, 0.1, 0.5, 1, 5}, "label1")
	require.NoError(t, err)

	// 更新指标值
	err = manager.IncrementCounter("test_counter", 1, "value1", "value2")
	require.NoError(t, err)

	err = manager.SetGauge("test_gauge", 42.5, "value1")
	require.NoError(t, err)

	err = manager.ObserveHistogram("test_histogram", 0.75, "value1")
	require.NoError(t, err)

	// 验证指标暴露HTTP端点
	resp, err := http.Get(fmt.Sprintf("http://localhost:%d/metrics", port))
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// 读取响应体
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	// 检查指标是否在输出中
	bodyStr := string(body)
	assert.Contains(t, bodyStr, "test_counter")
	assert.Contains(t, bodyStr, "test_gauge")
	assert.Contains(t, bodyStr, "test_histogram")
}

// TestMetricsCollectors 测试指标收集器
func TestMetricsCollectors(t *testing.T) {
	// 创建Metrics管理器
	manager := NewMetricsManager()
	require.NotNil(t, manager)

	// 创建指标收集器
	collector := NewMetricsCollector(manager)
	require.NotNil(t, collector)

	// 添加一个简单的收集器函数
	var collectorCalled bool
	collector.AddCollector(func(m *MetricsManager) {
		collectorCalled = true
		// 注册并设置一个测试仪表盘
		if err := m.RegisterGauge("test_collector_gauge", "Gauge set by collector", "instance"); err == nil {
			m.SetGauge("test_collector_gauge", 100, "test_instance")
		}
	})

	// 测试启动和收集
	collector.Start(100 * time.Millisecond)
	require.True(t, collector.IsRunning())

	// 等待收集器运行
	time.Sleep(200 * time.Millisecond)

	// 确认收集器被调用
	assert.True(t, collectorCalled)

	// 停止收集器
	collector.Stop()
	require.False(t, collector.IsRunning())
}

// TestSystemStatsCollector 测试系统指标收集器
func TestSystemStatsCollector(t *testing.T) {
	// 创建Metrics管理器
	manager := NewMetricsManager()
	require.NotNil(t, manager)

	// 创建指标收集器
	collector := NewMetricsCollector(manager)
	require.NotNil(t, collector)

	// 添加系统指标收集器
	collector.AddCollector(SystemStatsCollector())

	// 启动收集器
	collector.Start(100 * time.Millisecond)
	require.True(t, collector.IsRunning())
	defer collector.Stop()

	// 等待收集一次
	time.Sleep(200 * time.Millisecond)

	// 验证系统指标已注册
	registry := prometheus.DefaultRegisterer.(*prometheus.Registry)
	metrics, err := registry.Gather()
	require.NoError(t, err)

	// 检查是否有系统指标
	foundSystemMetric := false
	for _, metric := range metrics {
		if metric.GetName() == "go_goroutines" ||
			metric.GetName() == "go_memstats_alloc_bytes" {
			foundSystemMetric = true
			break
		}
	}

	assert.True(t, foundSystemMetric, "没有找到系统指标")
}

// TestConnectionStatsCollector 测试连接指标收集器
func TestConnectionStatsCollector(t *testing.T) {
	// 创建Metrics管理器
	manager := NewMetricsManager()
	require.NotNil(t, manager)

	// 模拟连接计数函数
	connections := 5
	getConnectionCount := func() int {
		return connections
	}

	// 添加连接指标收集器
	collector := NewMetricsCollector(manager)
	collector.AddCollector(ConnectionStatsCollector(getConnectionCount))

	// 启动收集器
	collector.Start(100 * time.Millisecond)
	defer collector.Stop()

	// 等待收集一次
	time.Sleep(200 * time.Millisecond)

	// 配置测试端口
	port := 19091
	address := fmt.Sprintf(":%d", port)

	// 启动HTTP服务器验证指标
	err := manager.StartServer(address)
	require.NoError(t, err)
	defer manager.StopServer()

	time.Sleep(100 * time.Millisecond)

	// 验证指标暴露HTTP端点
	resp, err := http.Get(fmt.Sprintf("http://localhost:%d/metrics", port))
	require.NoError(t, err)
	defer resp.Body.Close()

	// 读取响应体
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	// 检查连接指标是否在输出中
	bodyStr := string(body)
	assert.Contains(t, bodyStr, "connection_count")
}
