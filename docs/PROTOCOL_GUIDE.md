# 协议使用指南

本文档提供了MMO服务器框架支持的各种网络协议的详细使用说明，帮助开发人员理解如何与服务器进行通信。

## 支持的协议

MMO服务器框架目前支持以下网络协议：

1. **WebSocket**：用于Web客户端和服务器之间的双向实时通信
2. **QUIC**：低延迟、多路复用的UDP协议，适用于游戏状态同步
3. **HTTP/REST**：用于传统的请求-响应交互
4. **内部RPC**：用于服务间通信

## 消息格式

所有协议支持统一的消息格式，可以使用以下几种序列化方式：

### 1. 二进制格式

二进制消息格式为：
- 1字节标志位（用于表示消息特性）
- 2字节服务类型（表示处理消息的服务）
- 2字节消息类型（表示具体的消息操作）
- 剩余字节为负载数据

```
+--------+-----------------+-----------------+--------------------+
| 标志位 |    服务类型     |    消息类型     |      负载数据      |
| (1字节)|    (2字节)      |    (2字节)      |     (变长)         |
+--------+-----------------+-----------------+--------------------+
```

### 2. JSON格式

JSON消息格式如下：

```json
{
  "service_type": 1,
  "message_type": 2,
  "flags": 0,
  "session_id": "550e8400-e29b-41d4-a716-446655440000",
  "payload": {
    "key1": "value1",
    "key2": 42,
    "key3": [1, 2, 3]
  }
}
```

### 3. Protocol Buffers格式

使用Protobuf时，需要先定义消息格式：

```protobuf
syntax = "proto3";
package mmo;

message GenericMessage {
  uint32 service_type = 1;
  uint32 message_type = 2;
  uint32 flags = 3;
  string session_id = 4;
  bytes payload = 5;
}
```

## WebSocket通信

### 连接建立

WebSocket连接URL：`ws://服务器地址:端口/ws`
安全WebSocket连接URL：`wss://服务器地址:端口/ws`

### 认证流程

1. 客户端连接到WebSocket端点
2. 服务器发送挑战消息
3. 客户端响应挑战（可能包含用户凭据）
4. 服务器验证并确认连接

示例认证消息（JSON格式）：

```json
{
  "service_type": 1,
  "message_type": 1,
  "flags": 0,
  "payload": {
    "username": "player1",
    "token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..."
  }
}
```

### 心跳机制

为保持连接活跃，客户端应定期（通常每30秒）发送心跳消息：

```json
{
  "service_type": 0,
  "message_type": 0,
  "flags": 1
}
```

服务器将回复相同的消息确认心跳。

## QUIC通信

### 连接建立

QUIC连接地址：`quic://服务器地址:端口`

### 密钥交换

QUIC协议内置了TLS 1.3安全层，但我们的框架还实现了应用层的额外加密：

1. 客户端生成临时ECDH密钥对
2. 客户端发送公钥到服务器
3. 服务器生成会话密钥并用客户端公钥加密
4. 服务器返回加密的会话密钥
5. 客户端解密获得会话密钥

此后的所有消息都使用该会话密钥进行AES-GCM加密。

### 流的使用

QUIC支持多个并发流：

- 控制流：用于认证和系统消息
- 数据流：用于游戏状态同步
- 聊天流：用于玩家间通信

每个流都是全双工的，客户端和服务器可以同时发送和接收数据。

## HTTP/REST APIs

RESTful API端点根据功能进行组织：

- `/api/auth/*` - 认证相关
- `/api/players/*` - 玩家数据
- `/api/items/*` - 物品和库存
- `/api/worlds/*` - 世界和地图数据

所有REST API都返回标准JSON响应：

```json
{
  "success": true,
  "data": { ... },
  "error": null
}
```

或出错时：

```json
{
  "success": false,
  "data": null,
  "error": {
    "code": 404,
    "message": "Player not found"
  }
}
```

## 错误处理

所有协议都使用一致的错误代码系统：

| 错误码范围 | 错误类型 |
|---------|--------|
| 1-99 | 系统错误 |
| 100-199 | 认证和权限错误 |
| 200-299 | 输入验证错误 |
| 300-399 | 业务逻辑错误 |
| 400-499 | 数据库错误 |
| 500-599 | 网络和连接错误 |

## 性能考虑

- 当消息频率高时（如位置更新），使用二进制格式减少带宽
- 对于复杂但不频繁的消息（如物品交易），使用JSON格式提高可读性
- 考虑使用消息批处理减少网络往返
- 大型数据传输（如地图数据）应考虑分块传输

## 安全最佳实践

1. 所有敏感通信都应使用TLS/SSL
2. 认证令牌有过期时间并定期轮换
3. 敏感数据在传输前加密
4. 实现速率限制防止滥用

## 调试工具

1. **网络监视器**：在`tool/network_monitor`中提供，可以捕获和解析协议消息
2. **协议测试工具**：在`tool/protocol_test`中提供，可以手动构造和发送各种格式的消息
3. **性能分析器**：在`tool/perf_analyzer`中提供，可以测试不同消息格式和大小的性能

## 示例代码

客户端连接示例（JavaScript）：

```javascript
// WebSocket连接示例
const ws = new WebSocket('ws://game-server.example.com:8080/ws');

ws.onopen = () => {
  // 发送认证消息
  const authMessage = {
    service_type: 1,
    message_type: 1,
    payload: {
      username: 'player1',
      token: 'your-auth-token'
    }
  };
  ws.send(JSON.stringify(authMessage));
};

ws.onmessage = (event) => {
  const message = JSON.parse(event.data);
  console.log('收到消息:', message);
  
  // 处理不同类型的消息
  if (message.service_type === 2 && message.message_type === 3) {
    // 处理游戏状态更新
  } else if (message.service_type === 3 && message.message_type === 1) {
    // 处理聊天消息
  }
};
```

服务端处理示例（Go）：

```go
// 注册消息处理器
router.RegisterHandler(1, 1, func(msg *protocol.Message) (*protocol.Message, error) {
  // 处理认证消息
  username := msg.Payload["username"].(string)
  token := msg.Payload["token"].(string)
  
  // 验证令牌...
  
  return &protocol.Message{
    ServiceType: 1,
    MessageType: 2,
    Payload: map[string]interface{}{
      "status": "authenticated",
      "player_id": "12345"
    },
  }, nil
})
```

## 进一步阅读

- [WebSocket协议规范](https://datatracker.ietf.org/doc/html/rfc6455)
- [QUIC协议规范](https://datatracker.ietf.org/doc/html/rfc9000)
- [Protocol Buffers文档](https://protobuf.dev/)
- [MMO服务器框架内部文档](./IMPLEMENTATION_STATUS.md) 