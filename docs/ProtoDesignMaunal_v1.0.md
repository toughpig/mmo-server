# MMO游戏服务器通信协议设计规范 v1.0

## 1. 概述

本文档定义了MMO游戏服务器与客户端之间的通信协议，采用Protocol Buffers（protobuf）作为数据序列化方式，基于WebSocket实现实时双向通信。

## 2. 通信基础架构

### 2.1 传输协议

- 主要通信协议：WebSocket
- 数据格式：二进制（protobuf序列化）
- 传输加密：TLS/SSL（生产环境）

### 2.2 消息格式

每个消息由以下部分组成：

1. **消息ID**：4字节，大端序整数，用于标识消息类型
2. **消息体**：protobuf序列化后的二进制数据

所有消息都包含一个标准的消息头（MessageHeader），定义如下：

```protobuf
message MessageHeader {
  int32 msg_id = 1;       // 消息ID
  int64 timestamp = 2;    // 时间戳（毫秒）
  string session_id = 3;  // 会话ID
  int32 version = 4;      // 协议版本
  int32 result_code = 5;  // 结果代码，0表示成功
  string result_msg = 6;  // 结果消息
}
```

## 3. 消息ID分配规则

消息ID采用分段设计，便于管理和扩展：

- 1000-1999：登录相关消息
- 2000-2999：心跳相关消息
- 3000-3999：位置同步相关消息
- 4000-4999：聊天相关消息
- 5000-5999：战斗相关消息
- 6000-6999：背包和物品相关消息（预留）
- 7000-7999：任务相关消息（预留）
- 8000-8999：交易相关消息（预留）
- 9000-9999：系统通知和事件（预留）

## 4. 基础消息定义

### 4.1 登录相关

#### 4.1.1 登录请求 (1001)

```protobuf
message LoginRequest {
  MessageHeader header = 1;
  string username = 2;
  string password = 3;
  string device_id = 4;
  string client_version = 5;
}
```

#### 4.1.2 登录响应 (1002)

```protobuf
message LoginResponse {
  MessageHeader header = 1;
  string user_id = 2;
  string token = 3;
  UserInfo user_info = 4;
  ServerInfo server_info = 5;
}
```

### 4.2 心跳相关

#### 4.2.1 心跳请求 (2001)

```protobuf
message HeartbeatRequest {
  MessageHeader header = 1;
  int64 client_time = 2;
}
```

#### 4.2.2 心跳响应 (2002)

```protobuf
message HeartbeatResponse {
  MessageHeader header = 1;
  int64 server_time = 2;
}
```

### 4.3 位置同步相关

#### 4.3.1 位置同步请求 (3001)

```protobuf
message PositionSyncRequest {
  MessageHeader header = 1;
  string entity_id = 2;
  Position position = 3;
  Vector3 velocity = 4;
  float rotation = 5;
  int64 timestamp = 6;
}
```

#### 4.3.2 位置同步响应 (3002)

```protobuf
message PositionSyncResponse {
  MessageHeader header = 1;
  bool success = 2;
  repeated EntityPosition nearby_entities = 3;
}
```

### 4.4 聊天相关

#### 4.4.1 聊天请求 (4001)

```protobuf
message ChatRequest {
  MessageHeader header = 1;
  string sender_id = 2;
  string sender_name = 3;
  string content = 4;
  ChatType chat_type = 5;
  string target_id = 6; // 私聊时的目标ID
  string channel_id = 7; // 频道聊天时的频道ID
}
```

#### 4.4.2 聊天响应 (4002)

```protobuf
message ChatResponse {
  MessageHeader header = 1;
  bool delivered = 2;
  int64 message_id = 3;
  int64 timestamp = 4;
}
```

### 4.5 战斗相关

#### 4.5.1 战斗指令请求 (5001)

```protobuf
message CombatCommandRequest {
  MessageHeader header = 1;
  string attacker_id = 2;
  string target_id = 3;
  int32 skill_id = 4;
  Position position = 5;
  Vector3 direction = 6;
  repeated string affected_targets = 7;
  int64 timestamp = 8;
}
```

#### 4.5.2 战斗指令响应 (5002)

```protobuf
message CombatCommandResponse {
  MessageHeader header = 1;
  bool success = 2;
  repeated CombatResult results = 3;
}
```

## 5. 通信流程

### 5.1 连接与登录流程

1. 客户端与服务器建立WebSocket连接
2. 客户端发送登录请求（LoginRequest）
3. 服务器验证登录信息，返回登录响应（LoginResponse）
4. 若登录成功，客户端存储session_id并开始定期发送心跳请求

### 5.2 心跳保活流程

1. 客户端每30秒发送一次心跳请求（HeartbeatRequest）
2. 服务器收到心跳请求后返回心跳响应（HeartbeatResponse）
3. 若服务器在60秒内未收到客户端心跳，则断开连接
4. 若客户端在60秒内未收到服务器响应，则尝试重新连接

### 5.3 位置同步流程

1. 客户端以100ms的频率发送位置同步请求（PositionSyncRequest）
2. 服务器验证位置更新，返回位置同步响应（PositionSyncResponse）及附近实体信息
3. 服务器在必要时主动推送其他玩家的位置更新给客户端

### 5.4 聊天流程

1. 客户端发送聊天请求（ChatRequest）
2. 服务器验证聊天信息，返回聊天响应（ChatResponse）
3. 根据聊天类型，服务器将消息转发给相应的接收者

### 5.5 战斗流程

1. 客户端发送战斗指令请求（CombatCommandRequest）
2. 服务器验证战斗指令，计算战斗结果
3. 服务器返回战斗指令响应（CombatCommandResponse）
4. 服务器广播战斗结果给受影响的其他玩家

## 6. 数据类型定义

### 6.1 基础数据结构

#### 6.1.1 用户信息

```protobuf
message UserInfo {
  string user_id = 1;
  string username = 2;
  string nickname = 3;
  int32 level = 4;
  int64 exp = 5;
  int32 vip_level = 6;
  int64 last_login_time = 7;
  int64 created_time = 8;
}
```

#### 6.1.2 服务器信息

```protobuf
message ServerInfo {
  string server_id = 1;
  string server_name = 2;
  int32 server_status = 3;
  string server_address = 4;
  int32 server_port = 5;
  int32 online_count = 6;
  int32 max_online_count = 7;
}
```

#### 6.1.3 位置信息

```protobuf
message Position {
  float x = 1;
  float y = 2;
  float z = 3;
}

message Vector3 {
  float x = 1;
  float y = 2;
  float z = 3;
}

message EntityPosition {
  string entity_id = 1;
  string entity_type = 2;
  Position position = 3;
  float rotation = 4;
  Vector3 velocity = 5;
}
```

#### 6.1.4 战斗结果

```protobuf
message CombatResult {
  string target_id = 1;
  bool hit = 2;
  bool critical = 3;
  int32 damage = 4;
  int32 remaining_hp = 5;
  repeated BuffEffect buff_effects = 6;
}

message BuffEffect {
  int32 buff_id = 1;
  string buff_name = 2;
  int32 duration = 3;
  int32 stacks = 4;
}
```

### 6.2 枚举类型

#### 6.2.1 聊天类型

```protobuf
enum ChatType {
  WORLD = 0;
  PRIVATE = 1;
  TEAM = 2;
  GUILD = 3;
  SYSTEM = 4;
  CHANNEL = 5;
}
```

#### 6.2.2 消息类型

```protobuf
enum MessageType {
  UNKNOWN = 0;
  LOGIN_REQUEST = 1001;
  LOGIN_RESPONSE = 1002;
  HEARTBEAT_REQUEST = 2001;
  HEARTBEAT_RESPONSE = 2002;
  POSITION_SYNC_REQUEST = 3001;
  POSITION_SYNC_RESPONSE = 3002;
  CHAT_REQUEST = 4001;
  CHAT_RESPONSE = 4002;
  COMBAT_COMMAND_REQUEST = 5001;
  COMBAT_COMMAND_RESPONSE = 5002;
}
```

## 7. 错误码定义

| 错误码 | 描述 |
|-------|------|
| 0 | 成功 |
| 1001 | 用户名或密码错误 |
| 1002 | 账号已被禁用 |
| 1003 | 服务器维护中 |
| 1004 | 客户端版本过低 |
| 2001 | 会话已过期 |
| 3001 | 位置更新无效 |
| 4001 | 聊天内容含敏感词 |
| 5001 | 技能冷却中 |
| 5002 | 技能目标无效 |
| 9999 | 未知错误 |

## 8. 安全考虑

### 8.1 通信安全

- 所有生产环境通信采用WSS（WebSocket Secure）
- 密码等敏感信息在传输前应进行哈希处理
- 登录成功后，后续通信使用token进行身份验证

### 8.2 防作弊措施

- 服务器对位置更新进行合法性检查，防止瞬移作弊
- 战斗计算在服务器端进行，客户端仅发送指令
- 重要资源和状态变更需要服务器验证

### 8.3 流量控制

- 客户端发送请求频率有限制，防止洪水攻击
- 服务器对异常流量进行监控和限制

## 9. 版本控制

协议版本通过消息头中的version字段进行标识。

- 小版本更新（向后兼容）：只需要增加message定义中的字段
- 大版本更新（不兼容）：需要客户端和服务器同时更新

## 10. 扩展性设计

1. 消息ID预留足够空间，便于后续扩展
2. 关键数据结构预留扩展字段
3. 枚举类型设计考虑后续添加新值的可能性

## 11. 附录

### 11.1 消息流程图

[此处可添加关键交互的序列图]

### 11.2 版本历史

| 版本 | 日期 | 描述 |
|-----|------|-----|
| 1.0 | 2023-07-25 | 初始版本 | 