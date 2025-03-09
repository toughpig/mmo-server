# Envoy集成指南

本文档描述了如何将Envoy集成到MMO服务器框架中作为边缘代理，提供TLS终止、负载均衡、请求路由和安全功能。

## Envoy简介

Envoy是一个开源的边缘和服务代理，专为云原生应用程序设计。它具有以下特点：

- 透明的HTTP/2、gRPC和WebSocket代理
- 高级负载均衡
- TLS终止
- 可观察性和监控
- 丰富的路由和筛选器链
- 可扩展的API

在MMO服务器架构中，Envoy作为边缘代理，为所有客户端连接提供单一入口点，保护内部服务并优化网络流量。

## 架构设计

```
客户端 ----> [ Envoy边缘代理 ] ----> [ 网关服务器 ] ----> [ 内部服务 ]
               TLS终止              协议转换           业务逻辑
               负载均衡              会话管理
               路由                 安全验证
               限流
```

### Envoy的职责

1. **TLS终止** - 处理HTTPS和WSS连接，卸载加密处理
2. **请求路由** - 根据路径将请求路由到适当的后端服务
3. **负载均衡** - 在多个网关服务器实例之间分配负载
4. **访问控制** - 实现IP白名单、黑名单和请求限流
5. **监控和指标** - 收集请求统计和性能指标

## 配置说明

Envoy配置文件位于`config/envoy/envoy.yaml`，包含以下主要部分：

### 管理接口

```yaml
admin:
  access_log_path: /var/log/envoy/admin.log
  address:
    socket_address:
      address: 0.0.0.0
      port_value: 9901
```

管理接口提供运行时统计信息和配置更新功能，生产环境建议限制访问。

### 监听器配置

Envoy配置了两个主要监听器：

1. **HTTP监听器** (端口8080)
   - 处理未加密的HTTP和WebSocket流量
   - 路由到对应的后端服务

2. **HTTPS监听器** (端口8443)
   - 处理加密的HTTPS和WSS流量
   - 终止TLS连接
   - 路由到对应的后端服务

### 集群配置

配置了三个后端服务集群：

1. **websocket_service** - 处理WebSocket连接
2. **api_service** - 处理API请求
3. **default_service** - 处理其他请求

每个集群可以包含多个节点，实现负载均衡。

## 实施指南

### 安装Envoy

在Ubuntu系统上：

```bash
sudo apt-get update && sudo apt-get install -y apt-transport-https ca-certificates curl gnupg-agent software-properties-common
curl -sL 'https://deb.dl.getenvoy.io/public/gpg.8115BA8E629CC074.key' | sudo gpg --dearmor -o /usr/share/keyrings/getenvoy-keyring.gpg
echo "deb [arch=amd64 signed-by=/usr/share/keyrings/getenvoy-keyring.gpg] https://deb.dl.getenvoy.io/public/deb/ubuntu $(lsb_release -cs) main" | sudo tee /etc/apt/sources.list.d/getenvoy.list
sudo apt-get update && sudo apt-get install -y getenvoy-envoy
```

### 生成TLS证书

对于开发环境，可以生成自签名证书：

```bash
mkdir -p /etc/ssl/certs /etc/ssl/private
openssl req -x509 -nodes -days 365 -newkey rsa:2048 -keyout /etc/ssl/private/server.key -out /etc/ssl/certs/server.crt
```

生产环境应使用受信任的CA颁发的证书。

### 启动和停止Envoy

使用提供的脚本：

```bash
# 启动Envoy
chmod +x scripts/start_envoy.sh
scripts/start_envoy.sh

# 停止Envoy
chmod +x scripts/stop_envoy.sh
scripts/stop_envoy.sh
```

### 调整网关服务器配置

修改`config/gateway.json`配置，使网关服务器监听Envoy将转发的端口：

```json
{
  "websocket": {
    "address": ":8081"
  }
}
```

## 监控和指标

Envoy提供丰富的监控和指标收集功能：

1. **访问日志** - 配置在`access_log_path`
2. **管理界面** - 访问`http://localhost:9901/stats`查看统计信息
3. **Prometheus集成** - 通过`/stats/prometheus`端点暴露指标

## 高级功能

### 请求限流

在HTTP过滤器中添加`envoy.filters.http.rate_limit`可以实现请求限流：

```yaml
http_filters:
- name: envoy.filters.http.rate_limit
  typed_config:
    "@type": type.googleapis.com/envoy.extensions.filters.http.ratelimit.v3.RateLimit
    domain: gateway_ratelimit
    stage: 0
    rate_limit_service:
      grpc_service:
        envoy_grpc:
          cluster_name: rate_limit_service
```

### 故障注入

用于测试系统在网络故障条件下的行为：

```yaml
http_filters:
- name: envoy.filters.http.fault
  typed_config:
    "@type": type.googleapis.com/envoy.extensions.filters.http.fault.v3.HTTPFault
    delay:
      percentage:
        numerator: 10
        denominator: HUNDRED
      fixed_delay: 5s
```

## 故障排除

1. **查看Envoy日志**
   ```bash
   tail -f logs/envoy/envoy.log
   ```

2. **检查Envoy状态**
   ```bash
   curl -s http://localhost:9901/server_info | jq
   ```

3. **查看集群健康状态**
   ```bash
   curl -s http://localhost:9901/clusters | jq
   ```

4. **检查路由配置**
   ```bash
   curl -s http://localhost:9901/config_dump | jq '.configs[] | select(.["@type"] | contains("route"))'
   ```

## 生产环境考虑

1. **安全配置** - 限制管理接口访问，配置适当的TLS参数
2. **资源限制** - 为Envoy设置适当的内存和CPU限制
3. **监控集成** - 集成Prometheus和Grafana进行监控
4. **配置管理** - 使用版本控制管理Envoy配置文件
5. **滚动更新** - 实施无停机更新策略

## 参考资料

- [Envoy官方文档](https://www.envoyproxy.io/docs/envoy/latest/)
- [Envoy GitHub仓库](https://github.com/envoyproxy/envoy)
- [Envoy配置参考](https://www.envoyproxy.io/docs/envoy/latest/configuration/configuration) 