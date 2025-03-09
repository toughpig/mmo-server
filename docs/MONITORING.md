# 监控系统

本文档描述了MMO服务器框架的监控和指标收集系统。监控系统基于Prometheus实现，提供了对系统和业务指标的实时监控能力。

## 架构设计

MMO服务器框架的监控系统由以下组件构成：

1. **指标管理器（MetricsManager）**：核心组件，负责管理和注册各种指标
2. **指标收集器（MetricsCollector）**：定期收集系统和业务指标
3. **指标HTTP服务**：暴露Prometheus格式的指标数据
4. **Prometheus服务器**：抓取和存储指标数据（可选）
5. **Grafana仪表盘**：可视化指标数据（可选）

```
+---------------+    +---------------+    +---------------+
|   业务服务    |    |   网关服务    |    |  其他服务... |
+-------+-------+    +-------+-------+    +-------+-------+
        |                    |                    |
        v                    v                    v
+-------+--------------------+--------------------+-------+
|                     指标收集服务                       |
+------------------------+------------------------------+
                         |
                         v
              +----------+-----------+
              |   Prometheus服务器   |  (可选)
              +----------+-----------+
                         |
                         v
              +----------+-----------+
              |    Grafana仪表盘     |  (可选)
              +------------------------+
```

## 核心指标类型

监控系统支持四种基本指标类型：

1. **计数器（Counter）**：单调递增的计数器，如请求总数
2. **仪表盘（Gauge）**：可增可减的值，如当前连接数
3. **直方图（Histogram）**：对数据分布进行采样，如请求延迟
4. **摘要（Summary）**：类似直方图，但计算分位数，如95%的请求延迟

## 主要监控指标

### 系统指标

- `system_memory_alloc_bytes`：当前内存分配大小
- `system_memory_total_alloc_bytes`：累计内存分配大小
- `system_memory_sys_bytes`：系统使用的内存总量
- `system_memory_heap_objects`：堆对象数量
- `system_gc_next_bytes`：下次GC触发阈值
- `system_gc_pause_total_ns`：GC暂停总时间
- `system_goroutines`：当前goroutine数量
- `system_cpu_threads`：CPU线程数

### 连接指标

- `connection_count`：当前连接数
- `connection_rate`：每秒新建连接数
- `connection_errors`：连接错误数

### 消息指标

- `messages_total`：处理的消息总数
- `message_size_bytes`：消息大小分布
- `message_process_time`：消息处理时间分布

### Redis指标

- `redis_operations`：Redis操作统计
- `redis_errors`：Redis错误计数
- `redis_connection_pool`：连接池使用情况

### 数据库指标

- `database_operations`：数据库操作统计
- `database_query_time`：查询时间分布
- `database_errors`：数据库错误计数

## 使用指南

### 启动监控服务

使用提供的脚本启动监控服务：

```bash
# 确保脚本有执行权限
chmod +x scripts/start_metrics.sh
scripts/start_metrics.sh
```

默认情况下，监控服务会在端口9100上启动，并每10秒收集一次指标。

### 访问指标数据

指标数据以Prometheus格式暴露在`/metrics`端点，可以通过以下URL访问：

```
http://localhost:9100/metrics
```

### 停止监控服务

使用提供的脚本停止监控服务：

```bash
chmod +x scripts/stop_metrics.sh
scripts/stop_metrics.sh
```

### 配置选项

监控服务支持以下命令行选项：

- `-addr`：监听地址，默认为`:9100`
- `-interval`：指标收集间隔，默认为`10s`

例如，要使用不同的端口和收集间隔启动服务：

```bash
bin/metrics -addr=:9200 -interval=5s
```

## 集成Prometheus和Grafana

### Prometheus配置

将监控服务添加到Prometheus配置文件中：

```yaml
scrape_configs:
  - job_name: 'mmo_server'
    scrape_interval: 15s
    static_configs:
      - targets: ['localhost:9100']
```

### Grafana仪表盘

在项目的`config/grafana/`目录中提供了预配置的Grafana仪表盘：

1. `system_dashboard.json`：系统指标仪表盘
2. `service_dashboard.json`：服务指标仪表盘
3. `database_dashboard.json`：数据库指标仪表盘

将这些文件导入到Grafana中，即可获得可视化的监控界面。

## 扩展监控系统

### 添加自定义指标

要添加自定义业务指标，请遵循以下步骤：

1. 在服务代码中注入MetricsManager：

```go
import "mmo-server/pkg/metrics"

func NewService(metricsManager *metrics.MetricsManager) *Service {
    // 注册自定义指标
    metricsManager.RegisterCounter("my_business_events", "业务事件计数", "event_type")
    
    // ...
}
```

2. 在适当的地方更新指标：

```go
func (s *Service) handleEvent(eventType string) {
    // 处理事件...
    
    // 更新指标
    s.metricsManager.IncrementCounter("my_business_events", 1, eventType)
}
```

### 添加自定义收集器

创建自定义收集器函数：

```go
func MyCustomCollector(getMyStats func() map[string]float64) metrics.CollectorFunc {
    return func(manager *metrics.MetricsManager) {
        // 确保指标已注册
        if _, exists := manager.gaugeVecs["my_custom_metric"]; !exists {
            manager.RegisterGauge("my_custom_metric", "自定义指标", "label")
        }
        
        // 收集自定义统计
        stats := getMyStats()
        for key, value := range stats {
            manager.SetGauge("my_custom_metric", value, key)
        }
    }
}
```

将自定义收集器添加到收集器中：

```go
collector.AddCollector(MyCustomCollector(getMyStats))
```

## 故障排除

### 监控服务无法启动

检查以下几点：

1. 端口是否被占用：`netstat -tuln | grep 9100`
2. 查看日志文件：`cat logs/metrics.log`
3. 确保有足够的权限

### 指标未显示

1. 确认服务正在运行：`ps aux | grep metrics`
2. 检查端点是否可访问：`curl http://localhost:9100/metrics`
3. 查看指标注册是否成功

### 指标值异常

1. 检查指标收集逻辑是否正确
2. 确认指标类型（Counter vs Gauge）是否使用正确
3. 查看是否有并发访问导致的竞态条件

## 参考资料

- [Prometheus文档](https://prometheus.io/docs/introduction/overview/)
- [Grafana文档](https://grafana.com/docs/)
- [Go Prometheus客户端库](https://github.com/prometheus/client_golang) 