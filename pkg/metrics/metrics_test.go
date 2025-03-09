package metrics

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMetricsManager(t *testing.T) {
	// 创建指标管理器
	manager := NewMetricsManager()
	require.NotNil(t, manager)

	// 测试注册和使用计数器
	err := manager.RegisterCounter("test_counter", "测试计数器", "label1", "label2")
	require.NoError(t, err)

	// 增加计数器的值
	err = manager.IncrementCounter("test_counter", 5, "value1", "value2")
	require.NoError(t, err)

	// 测试注册和使用仪表盘
	err = manager.RegisterGauge("test_gauge", "测试仪表盘")
	require.NoError(t, err)

	// 设置仪表盘的值
	err = manager.SetGauge("test_gauge", 10)
	require.NoError(t, err)

	// 增加仪表盘的值
	err = manager.IncrementGauge("test_gauge", 5)
	require.NoError(t, err)

	// 减少仪表盘的值
	err = manager.DecrementGauge("test_gauge", 2)
	require.NoError(t, err)

	// 测试注册和使用直方图
	buckets := []float64{0.1, 0.5, 1, 2, 5}
	err = manager.RegisterHistogram("test_histogram", "测试直方图", buckets, "method")
	require.NoError(t, err)

	// 记录直方图观测值
	err = manager.ObserveHistogram("test_histogram", 1.5, "GET")
	require.NoError(t, err)

	// 测试注册和使用摘要
	objectives := map[float64]float64{0.5: 0.05, 0.9: 0.01, 0.99: 0.001}
	err = manager.RegisterSummary("test_summary", "测试摘要", objectives, "endpoint")
	require.NoError(t, err)

	// 记录摘要观测值
	err = manager.ObserveSummary("test_summary", 0.42, "/api/users")
	require.NoError(t, err)

	// 测试HTTP服务器
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/metrics" {
			// 这里只是简单测试，不实际检查metrics内容
			w.Write([]byte("metrics content"))
		} else {
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	// 测试MeasureDuration函数
	timer := manager.MeasureDuration("test_histogram", "POST")
	time.Sleep(10 * time.Millisecond)
	timer()

	// 测试重复注册错误
	err = manager.RegisterCounter("test_counter", "重复的计数器")
	assert.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "计数器已存在"))

	// 测试使用不存在的指标错误
	err = manager.IncrementCounter("nonexistent_counter", 1)
	assert.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "计数器不存在"))
}

func TestMetricsCollector(t *testing.T) {
	// 创建指标管理器
	manager := NewMetricsManager()
	require.NotNil(t, manager)

	// 创建收集器
	collector := NewMetricsCollector(manager)
	require.NotNil(t, collector)

	// 添加系统统计收集器
	collector.AddCollector(SystemStatsCollector())

	// 测试连接统计收集器
	getConnectionCount := func() int {
		return 42
	}
	collector.AddCollector(ConnectionStatsCollector(getConnectionCount))

	// 测试消息统计收集器
	messageCounts := map[string]int{
		"connect":    100,
		"disconnect": 50,
		"chat":       200,
	}
	collector.AddCollector(MessageStatsCollector(messageCounts))

	// 启动收集器（短时间）
	collector.Start(50 * time.Millisecond)
	assert.True(t, collector.IsRunning())

	// 等待至少一次收集完成
	time.Sleep(100 * time.Millisecond)

	// 停止收集器
	collector.Stop()
	assert.False(t, collector.IsRunning())
}
