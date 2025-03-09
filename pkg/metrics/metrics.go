package metrics

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// 定义指标类型常量
const (
	// 计数器类型的指标
	CounterMetric = "counter"
	// 仪表盘类型的指标（值可上可下）
	GaugeMetric = "gauge"
	// 直方图类型的指标（分布统计）
	HistogramMetric = "histogram"
	// 摘要类型的指标（分位数统计）
	SummaryMetric = "summary"
)

// MetricsManager 监控指标管理器
type MetricsManager struct {
	registry    *prometheus.Registry
	counterVecs map[string]*prometheus.CounterVec
	gaugeVecs   map[string]*prometheus.GaugeVec
	histograms  map[string]*prometheus.HistogramVec
	summaries   map[string]*prometheus.SummaryVec
	server      *http.Server
	mu          sync.RWMutex
}

// NewMetricsManager 创建新的指标管理器
func NewMetricsManager() *MetricsManager {
	return &MetricsManager{
		registry:    prometheus.NewRegistry(),
		counterVecs: make(map[string]*prometheus.CounterVec),
		gaugeVecs:   make(map[string]*prometheus.GaugeVec),
		histograms:  make(map[string]*prometheus.HistogramVec),
		summaries:   make(map[string]*prometheus.SummaryVec),
	}
}

// RegisterCounter 注册计数器类型指标
func (m *MetricsManager) RegisterCounter(name, help string, labelNames ...string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.counterVecs[name]; exists {
		return fmt.Errorf("计数器已存在: %s", name)
	}

	counterVec := promauto.With(m.registry).NewCounterVec(
		prometheus.CounterOpts{
			Name: name,
			Help: help,
		},
		labelNames,
	)

	m.counterVecs[name] = counterVec
	return nil
}

// IncrementCounter 增加计数器值
func (m *MetricsManager) IncrementCounter(name string, value float64, labelValues ...string) error {
	m.mu.RLock()
	counterVec, exists := m.counterVecs[name]
	m.mu.RUnlock()

	if !exists {
		return fmt.Errorf("计数器不存在: %s", name)
	}

	counterVec.WithLabelValues(labelValues...).Add(value)
	return nil
}

// RegisterGauge 注册仪表盘类型指标
func (m *MetricsManager) RegisterGauge(name, help string, labelNames ...string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.gaugeVecs[name]; exists {
		return fmt.Errorf("仪表盘已存在: %s", name)
	}

	gaugeVec := promauto.With(m.registry).NewGaugeVec(
		prometheus.GaugeOpts{
			Name: name,
			Help: help,
		},
		labelNames,
	)

	m.gaugeVecs[name] = gaugeVec
	return nil
}

// SetGauge 设置仪表盘值
func (m *MetricsManager) SetGauge(name string, value float64, labelValues ...string) error {
	m.mu.RLock()
	gaugeVec, exists := m.gaugeVecs[name]
	m.mu.RUnlock()

	if !exists {
		return fmt.Errorf("仪表盘不存在: %s", name)
	}

	gaugeVec.WithLabelValues(labelValues...).Set(value)
	return nil
}

// IncrementGauge 增加仪表盘值
func (m *MetricsManager) IncrementGauge(name string, value float64, labelValues ...string) error {
	m.mu.RLock()
	gaugeVec, exists := m.gaugeVecs[name]
	m.mu.RUnlock()

	if !exists {
		return fmt.Errorf("仪表盘不存在: %s", name)
	}

	gaugeVec.WithLabelValues(labelValues...).Add(value)
	return nil
}

// DecrementGauge 减少仪表盘值
func (m *MetricsManager) DecrementGauge(name string, value float64, labelValues ...string) error {
	m.mu.RLock()
	gaugeVec, exists := m.gaugeVecs[name]
	m.mu.RUnlock()

	if !exists {
		return fmt.Errorf("仪表盘不存在: %s", name)
	}

	gaugeVec.WithLabelValues(labelValues...).Sub(value)
	return nil
}

// RegisterHistogram 注册直方图类型指标
func (m *MetricsManager) RegisterHistogram(name, help string, buckets []float64, labelNames ...string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.histograms[name]; exists {
		return fmt.Errorf("直方图已存在: %s", name)
	}

	histogramVec := promauto.With(m.registry).NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    name,
			Help:    help,
			Buckets: buckets,
		},
		labelNames,
	)

	m.histograms[name] = histogramVec
	return nil
}

// ObserveHistogram 记录直方图观测值
func (m *MetricsManager) ObserveHistogram(name string, value float64, labelValues ...string) error {
	m.mu.RLock()
	histogramVec, exists := m.histograms[name]
	m.mu.RUnlock()

	if !exists {
		return fmt.Errorf("直方图不存在: %s", name)
	}

	histogramVec.WithLabelValues(labelValues...).Observe(value)
	return nil
}

// RegisterSummary 注册摘要类型指标
func (m *MetricsManager) RegisterSummary(name, help string, objectives map[float64]float64, labelNames ...string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.summaries[name]; exists {
		return fmt.Errorf("摘要已存在: %s", name)
	}

	summaryVec := promauto.With(m.registry).NewSummaryVec(
		prometheus.SummaryOpts{
			Name:       name,
			Help:       help,
			Objectives: objectives,
		},
		labelNames,
	)

	m.summaries[name] = summaryVec
	return nil
}

// ObserveSummary 记录摘要观测值
func (m *MetricsManager) ObserveSummary(name string, value float64, labelValues ...string) error {
	m.mu.RLock()
	summaryVec, exists := m.summaries[name]
	m.mu.RUnlock()

	if !exists {
		return fmt.Errorf("摘要不存在: %s", name)
	}

	summaryVec.WithLabelValues(labelValues...).Observe(value)
	return nil
}

// StartServer 启动HTTP服务器暴露指标
func (m *MetricsManager) StartServer(address string) error {
	if m.server != nil {
		return fmt.Errorf("服务器已在运行")
	}

	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{}))

	m.server = &http.Server{
		Addr:    address,
		Handler: mux,
	}

	go func() {
		log.Printf("启动指标服务器，监听地址: %s", address)
		if err := m.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("指标服务器错误: %v", err)
		}
	}()

	return nil
}

// StopServer 停止HTTP服务器
func (m *MetricsManager) StopServer() error {
	if m.server == nil {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	log.Printf("正在停止指标服务器...")
	return m.server.Shutdown(ctx)
}

// MeasureDuration 测量函数执行时间并记录到指标中
func (m *MetricsManager) MeasureDuration(metricName string, labelValues ...string) func() {
	startTime := time.Now()
	return func() {
		duration := time.Since(startTime).Seconds()
		if err := m.ObserveHistogram(metricName, duration, labelValues...); err != nil {
			log.Printf("记录指标失败 %s: %v", metricName, err)
		}
	}
}
