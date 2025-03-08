# MMO游戏网关服务

这是MMO游戏服务器框架的网关服务，提供了高性能的WebSocket和QUIC连接支持，以及会话管理和心跳检测功能。

## 功能特性

- WebSocket服务器：支持1000+并发连接
- QUIC协议支持（简化实现，未来完善）
- 会话管理：追踪和管理客户端会话
- 心跳检测：自动断开不活跃的连接
- 性能指标：通过HTTP接口提供实时指标

## 配置说明

网关服务通过配置文件配置，默认位置为`config/gateway.json`。主要配置项包括：

```json
{
  "session": {
    "heartbeat_ttl_sec": 60,       // 心跳超时时间（秒）
    "check_interval_sec": 10       // 心跳检查间隔（秒）
  },
  "websocket": {
    "enabled": true,               // 是否启用WebSocket
    "host": "0.0.0.0",             // 监听地址
    "port": 8081,                  // 监听端口
    "path": "/ws",                 // WebSocket路径
    "max_connections": 1000        // 最大连接数
  },
  "quic": {
    "enabled": true,               // 是否启用QUIC
    "host": "0.0.0.0",             // 监听地址
    "port": 8082,                  // 监听端口
    "max_connections": 1000        // 最大连接数
  },
  "metrics": {
    "enabled": true,               // 是否启用指标监控
    "host": "0.0.0.0",             // 监听地址
    "port": 9090,                  // 监听端口
    "path": "/metrics"             // 指标路径
  }
}
```

## 编译和运行

### 编译网关服务

```bash
go build -o bin/gateway cmd/gateway/main.go
```

### 运行网关服务

```bash
./bin/gateway
```

或者指定配置文件路径：

```bash
./bin/gateway -config path/to/config.json
```

## 测试工具

项目提供了两个测试工具：

### WebSocket客户端

简单的WebSocket客户端，用于测试连接和消息发送：

```bash
./bin/ws_client -addr localhost:8081 -path /ws -messages 10 -interval 1000
```

参数说明：
- `-addr`: 服务器地址和端口
- `-path`: WebSocket路径
- `-messages`: 发送的消息数量
- `-interval`: 消息间隔（毫秒）

### 负载测试工具

用于测试网关的并发性能：

```bash
./bin/load_test -conn 100 -msg 10 -interval 100
```

参数说明：
- `-conn`: 并发连接数
- `-msg`: 每个连接发送的消息数量
- `-interval`: 消息间隔（毫秒）
- `-rampup`: 建立所有连接的时间（秒）

## 监控指标

网关提供了HTTP接口用于监控：

- 指标接口：`http://localhost:9090/metrics`
- 健康检查：`http://localhost:9090/health`

## 架构设计

网关服务由以下主要组件组成：

1. **会话管理器**：管理所有客户端连接的会话，处理心跳超时，提供会话查询功能。
2. **WebSocket服务器**：处理WebSocket连接，提供升级HTTP连接和消息收发功能。
3. **QUIC服务器**：处理QUIC连接，提供基于UDP的可靠消息传输（简化实现）。
4. **指标服务器**：提供HTTP接口，用于监控网关的运行状态和性能指标。

所有连接和消息处理都使用Go语言的goroutine并发模型，能够高效地处理大量并发连接。 