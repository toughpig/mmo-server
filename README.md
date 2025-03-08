# MMO服务器框架

这是一个使用Golang开发的MMO游戏服务器框架，实现了高性能通信和状态同步功能。

## 技术栈

- Golang: 主要开发语言
- Protobuf: 用于定义通信协议
- shmipc-go: 高性能进程间通信
- Redis: 用于缓存和消息队列
- PostgreSQL: 持久化数据存储
- AES-GCM: 安全加密通信
- Envoy: 边缘代理服务

## 项目结构

- `proto_define/`: 协议定义文件(.proto)和生成的Go代码
- `server/`: 服务器主要代码
- `pkg/`: 公共包
  - `db/`: 数据库相关代码
  - `redis/`: Redis相关代码
  - `network/`: 网络通信相关代码
  - `game/`: 游戏逻辑相关代码
  - `gateway/`: 网关服务组件
  - `protocol/`: 协议转换和路由
  - `security/`: 安全加密组件
  - `config/`: 配置管理
  - `rpc/`: 基于shmipc-go的RPC框架
  - `sync/`: 玩家状态同步模块
  - `aoi/`: 兴趣区域系统
- `internal/`: 内部包
  - `models/`: 数据模型
  - `utils/`: 工具函数
- `cmd/`: 各种可执行程序
  - `gateway/`: 网关服务入口
  - `server/`: 主服务器入口
  - `protocol_test/`: 协议测试工具
  - `secure_ws_client/`: 安全WebSocket客户端
  - `load_test/`: 负载测试工具
  - `rpc_test/`: RPC框架测试工具
  - `sync_test/`: 状态同步测试工具
  - `aoi_test/`: AOI系统测试工具
- `config/`: 配置文件
- `docs/`: 文档
- `tools/`: 辅助工具
- `bin/`: 构建输出目录

## 构建指南

运行以下命令构建所有组件：

```bash
chmod +x build.sh
./build.sh
```

构建完成后，可执行文件将存储在`bin/`目录中。

## 测试指南

运行以下命令执行所有测试：

```bash
chmod +x test.sh
./test.sh
```

测试结果会保存在`test_results.log`文件中，覆盖率报告会生成为`coverage.html`。

### RPC框架测试

RPC框架有专门的测试工具，可以通过以下命令运行：

```bash
# 运行RPC框架示例测试
./bin/rpc_test -mode=example

# 运行服务器模式
./bin/rpc_test -mode=server -endpoint=/tmp/my-test-socket.sock

# 运行客户端模式
./bin/rpc_test -mode=client -endpoint=/tmp/my-test-socket.sock
```

### 玩家状态同步测试

状态同步系统也有专门的测试工具：

```bash
# 运行状态同步示例测试
./bin/sync_test -mode=example
```

### AOI系统测试

兴趣区域系统测试工具：

```bash
# 运行AOI系统性能测试
./bin/aoi_test
```

## 当前实现状态

该项目目前已实现以下功能：

1. **网关服务**
   - 基于AES-GCM的安全加密通信层
   - 多协议路由和消息转换框架
   - 安全WebSocket服务器
   - 会话管理和认证

2. **逻辑服务进程**
   - 基于shmipc-go的高性能RPC框架
   - 玩家状态同步模块
   - 基础AOI（兴趣区域）系统

正在开发的功能（第三阶段）：

1. **数据库模块**
   - Redis连接池（支持集群模式）
   - 数据库异步访问层（使用go-sql-driver）
   - 缓存同步机制（Write-Back策略）

有关详细的实现状态，请参阅[实现状态文档](docs/IMPLEMENTATION_STATUS.md)。

## 通信协议

详细通信协议见 [协议设计手册](docs/ProtoDesignMaunal_v1.0.md)

## RPC框架

框架基于shmipc-go实现了高性能的进程间通信机制，支持：

- 服务自动注册和发现
- 双向通信
- 上下文支持（超时和取消）
- 异步调用
- 协议无关（支持Protobuf和其他序列化格式）

更多详情请参阅 [RPC框架文档](pkg/rpc/README.md)

## 贡献指南

欢迎提交Pull Request或Issue。在提交代码前，请确保：

1. 所有测试都通过
2. 代码格式符合Go标准
3. 新功能包含适当的测试和文档

## 许可证

[MIT License](LICENSE) 