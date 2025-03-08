# 服务发现与负载均衡

本文档介绍了MMO服务器框架中的服务发现和负载均衡功能，帮助您高效地管理分布式服务。

## 概述

随着MMO游戏规模的增长，服务需要水平扩展以处理增加的负载。服务发现和负载均衡模块解决了在分布式环境中管理和访问服务的问题。该模块提供以下核心功能：

- **服务注册与发现**：自动注册和发现服务实例
- **负载均衡**：在多个服务实例之间智能分配请求
- **健康检查**：监控服务实例的状态并移除不健康的实例
- **服务监控**：通过观察者模式监控服务实例的变化

## 架构设计

服务发现和负载均衡模块由以下主要组件组成：

### 1. 服务注册表 (ServiceRegistry)

服务注册表是所有服务实例信息的中央存储库。它提供了以下核心功能：

- 注册新的服务实例
- 注销现有服务实例
- 查询特定服务的实例列表
- 监听服务实例的变更

框架提供了一个基于内存的注册表实现 (`InMemoryRegistry`)，适用于单机或小规模部署。对于生产环境，您可以实现外部注册表的适配器（如Consul、etcd或ZooKeeper）。

### 2. 负载均衡器 (LoadBalancer)

负载均衡器负责在多个服务实例之间分配请求，支持多种负载均衡策略：

- **随机选择**：随机选择一个服务实例
- **轮询选择**：按顺序轮流选择服务实例
- **加权选择**：根据实例权重进行选择，权重越高被选中的概率越大

负载均衡器通过服务注册表获取实例信息，然后应用相应的选择算法来确定请求目标。

### 3. 服务实例 (ServiceInstance)

服务实例代表一个正在运行的服务程序，包含以下信息：

- 唯一标识符
- 服务名称
- 连接地址（如 host:port）
- 标签（用于服务筛选）
- 权重（用于加权负载均衡）
- 状态（active, draining, down）

## 使用方法

### 创建服务注册表

```go
// 创建内存注册表
registry := rpc.NewInMemoryRegistry()
```

### 创建负载均衡器

```go
// 创建负载均衡器
loadBalancer := rpc.NewLoadBalancer(registry)

// 为不同服务设置不同的负载均衡策略
loadBalancer.SetSelector("player-service", &rpc.RoundRobinSelector{})
loadBalancer.SetSelector("chat-service", &rpc.WeightedSelector{})
```

### 注册服务实例

```go
// 创建服务实例
instance := &rpc.ServiceInstance{
    ID:       "instance-1",
    Name:     "player-service",
    Endpoint: "localhost:8081",
    Tags:     map[string]string{"zone": "zone-1"},
    Status:   "active",
    Weight:   1,
}

// 注册服务实例
err := registry.Register(context.Background(), instance)
if err != nil {
    log.Printf("注册服务实例失败: %v", err)
}
```

### 获取服务实例

```go
// 获取服务实例
instance, err := loadBalancer.GetInstance(context.Background(), "player-service")
if err != nil {
    log.Printf("获取服务实例失败: %v", err)
} else {
    log.Printf("选择实例: %s 在 %s", instance.ID, instance.Endpoint)
}
```

### 获取gRPC客户端

```go
// 获取服务的gRPC客户端
client, err := loadBalancer.GetClient(context.Background(), "player-service")
if err != nil {
    log.Printf("获取客户端失败: %v", err)
    return
}

// 使用客户端进行RPC调用
result, err := client.Call(context.Background(), "UpdatePosition", req)
```

### 监控服务变化

```go
// 监听服务变化
ch, err := registry.WatchService(context.Background(), "player-service")
if err != nil {
    log.Printf("监控服务失败: %v", err)
    return
}

// 处理服务变化事件
for instances := range ch {
    log.Printf("服务实例变化: 当前有 %d 个活跃实例", len(instances))
    for _, inst := range instances {
        log.Printf("  - %s 在 %s", inst.ID, inst.Endpoint)
    }
}
```

## 高级用法

### 自定义服务选择器

您可以通过实现 `InstanceSelector` 接口来创建自定义的服务选择策略：

```go
// 自定义选择器
type MyCustomSelector struct {
    // 自定义字段
}

// 实现Select方法
func (s *MyCustomSelector) Select(instances []*rpc.ServiceInstance) (*rpc.ServiceInstance, error) {
    // 自定义选择逻辑
    // ...
    return selectedInstance, nil
}

// 设置自定义选择器
loadBalancer.SetSelector("my-service", &MyCustomSelector{})
```

### 外部服务注册表集成

对于生产环境，您可以实现 `ServiceRegistry` 接口来集成外部服务注册系统：

```go
// Consul服务注册表实现
type ConsulRegistry struct {
    client *consulapi.Client
    // 其他字段
}

// 实现ServiceRegistry接口的方法
func (r *ConsulRegistry) Register(ctx context.Context, instance *rpc.ServiceInstance) error {
    // 实现Consul注册逻辑
    // ...
}

// 创建Consul注册表
consulRegistry := NewConsulRegistry("localhost:8500")

// 使用Consul注册表创建负载均衡器
loadBalancer := rpc.NewLoadBalancer(consulRegistry)
```

## 最佳实践

1. **服务粒度**：将您的应用拆分为适当粒度的微服务，每个服务都有明确的职责。

2. **标签使用**：使用标签来标识服务实例的特性，如区域、版本、部署环境等，以便进行更细粒度的服务发现。

3. **健康检查**：定期检查服务实例的健康状态，并自动移除不健康的实例。

4. **缓存结果**：在客户端缓存服务实例列表，以减少对服务注册表的请求，提高性能。

5. **故障转移**：实现请求重试和故障转移逻辑，以提高系统的弹性。

## 示例应用

参考 `examples/load_balancer/main.go` 查看完整示例。该示例演示了如何：

- 创建和配置服务注册表和负载均衡器
- 注册多个服务实例
- 监控服务变化
- 模拟客户端请求和负载均衡 