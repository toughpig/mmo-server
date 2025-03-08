# 多协议路由（Multi-Protocol Routing）

本文档描述MMO服务器框架中的多协议路由功能，该功能允许将来自不同协议（WebSocket、QUIC）的消息转换为统一的内部格式，并路由到相应的服务。

## 架构概述

多协议路由系统由以下核心组件组成：

1. **协议转换器（ProtocolConverter）**：将不同协议的消息转换为内部统一格式
2. **消息路由器（MessageRouter）**：根据消息类型将消息路由到不同的服务
3. **服务发现（ServiceDiscovery）**：管理服务实例信息
4. **负载均衡器（LoadBalancer）**：在多个服务实例之间进行负载均衡
5. **处理器注册表（HandlerRegistry）**：管理消息处理器

```
[客户端] 
    |
    | WebSocket/QUIC 连接
    v
[网关服务] -----> [协议转换] -----> [消息路由] -----> [服务发现]
                                     |              |
                                     v              v
                               [负载均衡] <----- [服务实例]
```

## 内部消息格式

为了统一不同协议的消息处理，我们定义了一种内部消息格式（`Message`），包含以下字段：

- **Version**：协议版本号
- **Flags**：消息标志（加密、压缩等）
- **ServiceType**：服务类型（聊天、游戏等）
- **MessageType**：消息类型（登录、心跳等）
- **SequenceID**：序列号，用于可靠传输
- **Timestamp**：时间戳
- **PayloadLength**：负载长度
- **SourceService**：源服务名
- **DestinationService**：目标服务名
- **CorrelationID**：关联ID，用于追踪请求-响应
- **SessionID**：会话ID
- **Payload**：消息负载

## 支持的协议和格式

### 连接类型（ConnectionType）

- **WebSocket**：基于HTTP的双向通信协议
- **QUIC**：基于UDP的多路复用传输协议
- **Internal**：内部服务间通信

### 消息格式（FormatType）

- **Binary**：二进制格式，高效但不易调试
- **JSON**：人类可读的文本格式，适合调试
- **Text**：纯文本格式，通常是简单的字符串
- **Protobuf**：Protocol Buffers二进制格式，高效且紧凑

## 协议转换流程

### 客户端到服务器

1. 客户端通过WebSocket或QUIC发送消息
2. 网关接收消息并识别连接类型和格式
3. 协议转换器将消息转换为内部格式
4. 消息路由器将消息路由到对应的服务
5. 服务处理消息并返回响应
6. 协议转换器将内部格式的响应转换回原始格式
7. 网关将响应发送回客户端

### 消息路由

1. 根据消息的ServiceType找到对应的服务名
2. 通过服务发现找到服务的实例列表
3. 使用负载均衡器选择一个服务实例
4. 将消息发送到选中的服务实例
5. 等待服务响应，或者异步发送不等待响应

## 使用示例

### 初始化网关处理器

```go
handler, err := gateway.NewGatewayHandler(config)
if err != nil {
    log.Fatalf("Failed to create gateway handler: %v", err)
}

// 启动网关
if err := handler.Start(); err != nil {
    log.Fatalf("Failed to start gateway: %v", err)
}
```

### 注册消息处理器

```go
registry := protocol.NewHandlerRegistry()
registry.RegisterHandler(protocol.ServiceTypeChat, 1, func(msg *protocol.Message) (*protocol.Message, error) {
    // 处理聊天消息
    response := msg.Clone()
    response.SourceService, response.DestinationService = msg.DestinationService, msg.SourceService
    response.Payload = []byte(`{"status":"ok"}`)
    return response, nil
})
```

### 客户端发送消息示例（WebSocket）

```javascript
// JSON格式
const message = {
    service_type: 2, // 聊天服务
    message_type: 1, // 聊天消息
    flags: 0,
    payload: {
        content: "Hello, World!",
        from: "user1",
        to: "all"
    },
    session_id: "session-123"
};

websocket.send(JSON.stringify(message));
```

## 安全考虑

- 所有外部连接都应使用TLS/DTLS加密
- 敏感数据应使用应用层加密（AES-GCM）
- 应实现消息认证和验证机制
- 使用会话追踪和限流防止滥用

## 性能优化

- 二进制格式比JSON更高效，适用于高性能场景
- 使用连接池减少连接建立开销
- 实现消息批处理减少网络往返
- 对热点服务进行分片和扩展

## 测试工具

我们提供了测试工具来验证多协议路由功能：

- `cmd/protocol_test/main.go`: 测试协议转换和消息路由
- `tools/ws_client.go`: WebSocket客户端测试工具
- `tools/secure_ws_client.go`: 安全WebSocket客户端测试工具

## 下一步计划

- 实现QUIC协议服务器的完整功能
- 添加消息压缩支持
- 优化大规模消息路由的性能
- 增加监控和指标收集 