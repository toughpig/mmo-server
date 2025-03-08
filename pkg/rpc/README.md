# RPC 框架

RPC 框架已完全迁移到基于 gRPC 的实现。

详细文档请参见 [gRPC 框架文档](../../README-GRPC.md)。

## 主要特性

- 高性能 gRPC 通信
- 服务注册和发现
- 上下文支持和超时控制
- 异步调用
- 直接服务访问接口
- 高效连接池管理

## 组件说明

- `grpc_client.go`: gRPC 客户端实现
- `grpc_server.go`: gRPC 服务器实现
- `conn_manager.go`: 连接管理器，负责连接池管理
- `factory.go`: 工厂模式实现，提供统一接口
- `rpc.go`: 核心接口和数据类型定义
- `call.go`: RPC 调用相关定义 