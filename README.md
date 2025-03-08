# MMO服务器框架

这是一个使用Golang开发的MMO游戏服务器框架，实现了最小通信架构。

## 技术栈

- Golang: 主要开发语言
- Protobuf: 用于定义通信协议
- shmipc-go: 高性能进程间通信
- Redis: 用于缓存和消息队列
- PostgreSQL: 持久化数据存储

## 项目结构

- `proto/`: 协议定义文件(.proto)
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
- `internal/`: 内部包
  - `models/`: 数据模型
  - `utils/`: 工具函数
- `cmd/`: 各种可执行程序
  - `gateway/`: 网关服务入口
  - `server/`: 主服务器入口
  - `protocol_test/`: 协议测试工具
  - `secure_ws_client/`: 安全WebSocket客户端
  - `load_test/`: 负载测试工具
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

## 当前实现状态

该项目目前处于开发阶段，已实现以下功能：

1. 基于AES-GCM的安全加密通信层
2. 多协议路由和消息转换框架
3. 安全WebSocket服务器
4. 负载测试工具

有关详细的实现状态，请参阅[实现状态文档](docs/IMPLEMENTATION_STATUS.md)。

## 通信协议

详细通信协议见 [协议设计手册](docs/ProtoDesignMaunal_v1.0.md)

## 贡献指南

欢迎提交Pull Request或Issue。在提交代码前，请确保：

1. 所有测试都通过
2. 代码格式符合Go标准
3. 新功能包含适当的测试和文档

## 许可证

[MIT License](LICENSE) 