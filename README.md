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
- `internal/`: 内部包
  - `models/`: 数据模型
  - `utils/`: 工具函数
- `cmd/server/`: 服务器入口
- `config/`: 配置文件
- `docs/`: 文档

## 通信协议

详细通信协议见 `docs/ProtoDesignMaunal_v1.0.md` 