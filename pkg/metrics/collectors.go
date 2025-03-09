package metrics

import (
	"log"
	"runtime"
	"time"
)

// CollectorFunc 定义收集器函数类型
type CollectorFunc func(manager *MetricsManager)

// Collector 定义指标收集器接口
type Collector interface {
	// Start 启动收集器
	Start(interval time.Duration)
	// Stop 停止收集器
	Stop()
	// IsRunning 检查收集器是否正在运行
	IsRunning() bool
}

// MetricsCollector 基本指标收集器
type MetricsCollector struct {
	manager    *MetricsManager
	collectors []CollectorFunc
	ticker     *time.Ticker
	stopCh     chan struct{}
	isRunning  bool
}

// NewMetricsCollector 创建指标收集器
func NewMetricsCollector(manager *MetricsManager) *MetricsCollector {
	return &MetricsCollector{
		manager:    manager,
		collectors: make([]CollectorFunc, 0),
		stopCh:     make(chan struct{}),
	}
}

// AddCollector 添加收集器函数
func (c *MetricsCollector) AddCollector(collector CollectorFunc) {
	c.collectors = append(c.collectors, collector)
}

// Start 启动收集器
func (c *MetricsCollector) Start(interval time.Duration) {
	if c.isRunning {
		return
	}

	c.ticker = time.NewTicker(interval)
	c.isRunning = true

	go func() {
		// 首次立即收集一次
		c.collect()

		for {
			select {
			case <-c.ticker.C:
				c.collect()
			case <-c.stopCh:
				return
			}
		}
	}()

	log.Printf("指标收集器已启动，收集间隔: %v", interval)
}

// Stop 停止收集器
func (c *MetricsCollector) Stop() {
	if !c.isRunning {
		return
	}

	c.ticker.Stop()
	c.stopCh <- struct{}{}
	c.isRunning = false

	log.Printf("指标收集器已停止")
}

// IsRunning 检查收集器是否正在运行
func (c *MetricsCollector) IsRunning() bool {
	return c.isRunning
}

// collect 执行所有收集器
func (c *MetricsCollector) collect() {
	for _, collector := range c.collectors {
		collector(c.manager)
	}
}

// SystemStatsCollector 返回系统状态收集器
func SystemStatsCollector() CollectorFunc {
	// 注册系统指标
	return func(manager *MetricsManager) {
		// 确保指标已注册
		ensureSystemMetricsRegistered(manager)

		// 收集内存统计
		var memStats runtime.MemStats
		runtime.ReadMemStats(&memStats)

		// 设置系统统计数据
		manager.SetGauge("system_memory_alloc_bytes", float64(memStats.Alloc))
		manager.SetGauge("system_memory_total_alloc_bytes", float64(memStats.TotalAlloc))
		manager.SetGauge("system_memory_sys_bytes", float64(memStats.Sys))
		manager.SetGauge("system_memory_heap_objects", float64(memStats.HeapObjects))
		manager.SetGauge("system_gc_next_bytes", float64(memStats.NextGC))
		manager.SetGauge("system_gc_pause_total_ns", float64(memStats.PauseTotalNs))
		manager.SetGauge("system_goroutines", float64(runtime.NumGoroutine()))
		manager.SetGauge("system_cpu_threads", float64(runtime.GOMAXPROCS(0)))
	}
}

// ensureSystemMetricsRegistered 确保系统指标已注册
func ensureSystemMetricsRegistered(manager *MetricsManager) {
	// 内存分配大小
	if _, exists := manager.gaugeVecs["system_memory_alloc_bytes"]; !exists {
		manager.RegisterGauge("system_memory_alloc_bytes", "当前内存分配大小（字节）")
	}

	// 累计内存分配大小
	if _, exists := manager.gaugeVecs["system_memory_total_alloc_bytes"]; !exists {
		manager.RegisterGauge("system_memory_total_alloc_bytes", "累计内存分配大小（字节）")
	}

	// 系统使用的内存总量
	if _, exists := manager.gaugeVecs["system_memory_sys_bytes"]; !exists {
		manager.RegisterGauge("system_memory_sys_bytes", "系统使用的内存总量（字节）")
	}

	// 堆对象数量
	if _, exists := manager.gaugeVecs["system_memory_heap_objects"]; !exists {
		manager.RegisterGauge("system_memory_heap_objects", "堆对象数量")
	}

	// 下次GC触发阈值
	if _, exists := manager.gaugeVecs["system_gc_next_bytes"]; !exists {
		manager.RegisterGauge("system_gc_next_bytes", "下次GC触发阈值（字节）")
	}

	// GC暂停总时间
	if _, exists := manager.gaugeVecs["system_gc_pause_total_ns"]; !exists {
		manager.RegisterGauge("system_gc_pause_total_ns", "GC暂停总时间（纳秒）")
	}

	// goroutine数量
	if _, exists := manager.gaugeVecs["system_goroutines"]; !exists {
		manager.RegisterGauge("system_goroutines", "当前goroutine数量")
	}

	// CPU线程数
	if _, exists := manager.gaugeVecs["system_cpu_threads"]; !exists {
		manager.RegisterGauge("system_cpu_threads", "CPU线程数")
	}
}

// ConnectionStatsCollector 返回连接统计收集器
func ConnectionStatsCollector(getConnectionCount func() int) CollectorFunc {
	return func(manager *MetricsManager) {
		// 确保指标已注册
		if _, exists := manager.gaugeVecs["connection_count"]; !exists {
			manager.RegisterGauge("connection_count", "当前连接数")
		}

		// 设置连接数
		manager.SetGauge("connection_count", float64(getConnectionCount()))
	}
}

// MessageStatsCollector 返回消息统计收集器
func MessageStatsCollector(counts map[string]int) CollectorFunc {
	return func(manager *MetricsManager) {
		// 确保指标已注册
		if _, exists := manager.counterVecs["messages_total"]; !exists {
			manager.RegisterCounter("messages_total", "消息总数", "type", "service")
		}

		// 遍历消息计数
		for msgType, count := range counts {
			manager.IncrementCounter("messages_total", float64(count), msgType, "gateway")
		}
	}
}

// RedisStatsCollector 创建Redis统计收集器
func RedisStatsCollector(getStats func() map[string]float64) CollectorFunc {
	return func(manager *MetricsManager) {
		// 确保指标已注册
		if _, exists := manager.gaugeVecs["redis_operations"]; !exists {
			manager.RegisterGauge("redis_operations", "Redis操作统计", "operation")
		}

		// 收集Redis统计
		stats := getStats()
		for op, value := range stats {
			manager.SetGauge("redis_operations", value, op)
		}
	}
}

// DatabaseStatsCollector 创建数据库统计收集器
func DatabaseStatsCollector(getStats func() map[string]float64) CollectorFunc {
	return func(manager *MetricsManager) {
		// 确保指标已注册
		if _, exists := manager.gaugeVecs["database_operations"]; !exists {
			manager.RegisterGauge("database_operations", "数据库操作统计", "operation")
		}

		// 收集数据库统计
		stats := getStats()
		for op, value := range stats {
			manager.SetGauge("database_operations", value, op)
		}
	}
}
