# Metrics 模块使用指南

本文档提供了MMO服务器框架中Metrics监控系统的详细使用说明，包括配置、集成和自定义指标的添加方法。

## 1. 简介

Metrics模块基于Prometheus实现，用于收集和监控MMO服务器的各项性能指标和状态数据。该模块支持多种类型的指标，包括计数器(Counter)、仪表盘(Gauge)、直方图(Histogram)和摘要(Summary)，可以帮助开发人员和运维人员实时了解服务器的运行状况。

### 1.1 主要功能

- 系统资源监控（CPU、内存、磁盘、网络）
- 连接数和消息吞吐量统计
- Redis缓存性能监控
- 数据库操作监控
- 自定义业务指标支持
- Grafana仪表盘可视化

### 1.2 架构概览

Metrics模块由以下几个部分组成：

1. **MetricsManager**: 核心组件，负责创建和管理各类指标
2. **Collectors**: 各种指标收集器的实现，负责收集特定类型的数据
3. **HTTP服务**: 用于向Prometheus暴露监控数据的HTTP接口
4. **配置管理**: 控制监控系统的行为和参数

## 2. 快速开始

### 2.1 安装和配置

1. 确保项目依赖包含Prometheus客户端库：

```go
import (
    "github.com/prometheus/client_golang/prometheus"
    "github.com/prometheus/client_golang/prometheus/promhttp"
)
```

2. 在配置文件中添加Metrics相关配置：

```json
{
    "metrics": {
        "enabled": true,
        "listen_address": ":9090",
        "path": "/metrics",
        "collection_interval": 15,
        "enable_go_metrics": true,
        "enable_process_metrics": true
    }
}
```

### 2.2 启动Metrics服务

在主程序中初始化和启动Metrics服务：

```go
import "mmo-server/pkg/metrics"

func main() {
    // 创建Metrics管理器
    metricsManager, err := metrics.NewMetricsManager(config.Metrics)
    if err != nil {
        log.Fatalf("Failed to create metrics manager: %v", err)
    }
    
    // 启动HTTP服务暴露metrics
    go metricsManager.ServeHTTP()
    
    // 注册自定义收集器
    metricsManager.RegisterCollectors()
    
    // 其他服务初始化...
}
```

### 2.3 查看监控数据

启动服务后，可以通过以下URL访问监控数据：

```
http://your-server-address:9090/metrics
```

## 3. 内置指标

Metrics模块预置了多种指标，用于监控服务器的核心功能：

### 3.1 系统指标

| 指标名称 | 类型 | 描述 |
|---------|------|------|
| `mmo_system_cpu_usage` | Gauge | CPU使用率（百分比） |
| `mmo_system_memory_usage` | Gauge | 内存使用量（字节） |
| `mmo_system_disk_usage` | Gauge | 磁盘使用量（字节） |
| `mmo_system_network_tx_bytes` | Counter | 网络发送字节数 |
| `mmo_system_network_rx_bytes` | Counter | 网络接收字节数 |

### 3.2 连接指标

| 指标名称 | 类型 | 描述 |
|---------|------|------|
| `mmo_connections_active` | Gauge | 当前活跃连接数 |
| `mmo_connections_total` | Counter | 累计连接总数 |
| `mmo_connections_error` | Counter | 连接错误数 |
| `mmo_connections_duration` | Histogram | 连接持续时间分布 |

### 3.3 消息指标

| 指标名称 | 类型 | 描述 |
|---------|------|------|
| `mmo_messages_received_total` | Counter | 接收消息总数 |
| `mmo_messages_sent_total` | Counter | 发送消息总数 |
| `mmo_messages_errors` | Counter | 消息处理错误数 |
| `mmo_messages_processing_time` | Histogram | 消息处理时间分布 |
| `mmo_messages_size_bytes` | Histogram | 消息大小分布 |

### 3.4 缓存指标

| 指标名称 | 类型 | 描述 |
|---------|------|------|
| `mmo_cache_hits_total` | Counter | 缓存命中总数 |
| `mmo_cache_misses_total` | Counter | 缓存未命中总数 |
| `mmo_cache_operations_total` | Counter | 缓存操作总数 |
| `mmo_cache_operation_errors` | Counter | 缓存操作错误数 |
| `mmo_cache_operation_duration` | Histogram | 缓存操作耗时分布 |

### 3.5 数据库指标

| 指标名称 | 类型 | 描述 |
|---------|------|------|
| `mmo_db_queries_total` | Counter | 数据库查询总数 |
| `mmo_db_query_errors` | Counter | 数据库查询错误数 |
| `mmo_db_query_duration` | Histogram | 数据库查询耗时分布 |
| `mmo_db_connections_active` | Gauge | 活跃数据库连接数 |
| `mmo_db_connection_errors` | Counter | 数据库连接错误数 |

## 4. 自定义指标

除了内置指标外，开发人员还可以根据业务需求添加自定义指标。

### 4.1 创建自定义指标

```go
import (
    "mmo-server/pkg/metrics"
)

// 在您的服务中创建指标
func InitCustomMetrics(metricsManager *metrics.MetricsManager) {
    // 创建一个计数器
    myCounter := metricsManager.NewCounter(
        "my_custom_counter",
        "A custom counter for demonstration",
        []string{"label1", "label2"},
    )
    
    // 创建一个仪表盘
    myGauge := metricsManager.NewGauge(
        "my_custom_gauge",
        "A custom gauge for demonstration",
        []string{"label1"},
    )
    
    // 创建一个直方图
    myHistogram := metricsManager.NewHistogram(
        "my_custom_histogram",
        "A custom histogram for demonstration",
        []string{"label1"},
        []float64{0.01, 0.05, 0.1, 0.5, 1, 2, 5, 10},
    )
    
    // 保存指标引用以便后续使用
    // ...
}
```

### 4.2 更新指标值

创建指标后，您可以在代码中更新这些指标的值：

```go
// 增加计数器
myCounter.With(prometheus.Labels{"label1": "value1", "label2": "value2"}).Inc()

// 设置仪表盘值
myGauge.With(prometheus.Labels{"label1": "value1"}).Set(42.0)

// 观察直方图值
myHistogram.With(prometheus.Labels{"label1": "value1"}).Observe(0.42)
```

### 4.3 创建自定义收集器

对于复杂的指标收集需求，可以创建自定义收集器：

```go
type MyCustomCollector struct {
    someMetric *prometheus.GaugeVec
    // 其他指标...
}

func NewMyCustomCollector() *MyCustomCollector {
    return &MyCustomCollector{
        someMetric: prometheus.NewGaugeVec(
            prometheus.GaugeOpts{
                Name: "mmo_my_custom_metric",
                Help: "Description of my custom metric",
            },
            []string{"label1", "label2"},
        ),
    }
}

// Describe 实现Collector接口
func (c *MyCustomCollector) Describe(ch chan<- *prometheus.Desc) {
    c.someMetric.Describe(ch)
}

// Collect 实现Collector接口
func (c *MyCustomCollector) Collect(ch chan<- prometheus.Metric) {
    // 获取最新数据
    value := getCurrentValue()
    
    // 更新指标
    c.someMetric.With(prometheus.Labels{
        "label1": "value1",
        "label2": "value2",
    }).Set(value)
    
    // 收集指标
    c.someMetric.Collect(ch)
}

// 注册收集器
func RegisterMyCustomCollector(metricsManager *metrics.MetricsManager) {
    collector := NewMyCustomCollector()
    metricsManager.RegisterCollector(collector)
}
```

## 5. 与Prometheus集成

### 5.1 Prometheus配置

要将MMO服务器的指标集成到Prometheus中，请在Prometheus配置文件中添加以下内容：

```yaml
scrape_configs:
  - job_name: 'mmo-server'
    scrape_interval: 15s
    static_configs:
      - targets: ['your-server-address:9090']
    metrics_path: /metrics
```

### 5.2 标签使用建议

为了更好地组织和查询监控数据，建议在指标中使用以下标签：

- `server`: 服务器实例标识符
- `service`: 服务名称（如gateway、game、chat等）
- `environment`: 环境（如dev、test、prod）
- `region`: 部署区域

使用示例：

```go
counter.With(prometheus.Labels{
    "server": "server-01",
    "service": "gateway",
    "environment": "prod",
    "region": "us-east",
}).Inc()
```

## 6. 与Grafana集成

### 6.1 配置Grafana数据源

1. 在Grafana中添加Prometheus数据源
2. 设置URL指向Prometheus服务器地址
3. 保存并测试连接

### 6.2 导入仪表盘

MMO服务器框架提供了预配置的Grafana仪表盘，可以在`./config/grafana/dashboards/`目录中找到：

- `mmo_overview.json`: 系统总览仪表盘
- `mmo_connections.json`: 连接监控仪表盘
- `mmo_messages.json`: 消息监控仪表盘
- `mmo_cache.json`: 缓存监控仪表盘
- `mmo_database.json`: 数据库监控仪表盘

导入步骤：
1. 在Grafana中点击"+"按钮，选择"Import"
2. 上传JSON文件或粘贴JSON内容
3. 选择适当的数据源
4. 点击"Import"完成导入

## 7. 常见问题

### 7.1 指标命名冲突

**问题**: 创建的自定义指标与内置指标名称冲突。

**解决方案**: 自定义指标建议使用自己的前缀，例如`mmo_myservice_*`，避免与内置的`mmo_*`指标冲突。

### 7.2 内存占用过高

**问题**: Metrics模块占用内存过高。

**解决方案**:
- 减少高基数标签的使用（如用户ID、会话ID等）
- 适当提高收集间隔
- 只收集必要的指标
- 使用适当的汇总手段代替原始数据

### 7.3 指标未显示

**问题**: 配置了指标但在Prometheus中看不到。

**解决方案**:
- 确认Metrics服务是否正常运行，可通过直接访问`/metrics`端点验证
- 检查Prometheus配置是否正确
- 检查网络连接和防火墙设置
- 查看Prometheus目标页面中的错误信息

## 8. 最佳实践

1. **保持指标名称简洁明了**：使用有意义的名称，遵循`mmo_subsystem_metric_unit`格式

2. **合理使用标签**：标签能提供额外维度，但过多标签会增加内存占用

3. **选择合适的指标类型**：
   - 计数器(Counter)：适用于累计值，如请求数、错误数
   - 仪表盘(Gauge)：适用于可变值，如内存使用、连接数
   - 直方图(Histogram)：适用于分布统计，如请求持续时间
   - 摘要(Summary)：适用于需要计算分位数的场景

4. **定期审查指标**：删除不再使用的指标，避免资源浪费

5. **设置告警**：为关键指标配置合理的告警阈值，及时发现问题

## 9. 参考资料

- [Prometheus文档](https://prometheus.io/docs/introduction/overview/)
- [Grafana文档](https://grafana.com/docs/)
- [pkg/metrics包文档](../pkg/metrics/README.md)
- [Prometheus Go客户端库](https://github.com/prometheus/client_golang) 