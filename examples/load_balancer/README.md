# 服务发现和负载均衡示例

本示例演示了如何使用MMO服务器框架的服务发现和负载均衡功能。

## 功能概述

示例包含以下组件：

1. **服务注册中心** - 使用内存实现的服务注册表
2. **服务实例** - 示例服务器，支持动态注册和注销
3. **负载均衡客户端** - 使用不同负载均衡策略（随机、轮询、加权）的客户端

## 目录结构

```
load_balancer/
├── client/                # 客户端示例代码
│   └── client.go
├── server/                # 服务器示例代码
│   └── server.go
├── main.go                # 主程序（内置测试）
├── run_demo.sh            # 运行演示脚本
├── test.sh                # 测试脚本
└── README.md              # 本文档
```

## 运行演示

使用提供的脚本运行演示：

```bash
# 添加执行权限
chmod +x run_demo.sh
chmod +x test.sh

# 运行演示脚本
./run_demo.sh
```

你可以选择以下运行模式：

### 1. 内置测试

直接运行内置测试，模拟服务注册和客户端请求：

```bash
./load_balancer_demo -mode=test
```

### 2. 手动测试

按照以下步骤在多个终端窗口中启动服务器和客户端：

1. 启动服务器实例：

```bash
# 终端1
./server_demo -port=8081 -id=server1 -weight=1 -zone=zone1

# 终端2
./server_demo -port=8082 -id=server2 -weight=2 -zone=zone1

# 终端3
./server_demo -port=8083 -id=server3 -weight=3 -zone=zone2
```

2. 启动客户端（使用不同负载均衡策略）：

```bash
# 随机负载均衡
./client_demo -lb=random -count=12 -interval=500ms

# 轮询负载均衡
./client_demo -lb=round-robin -count=9 -interval=500ms

# 加权负载均衡
./client_demo -lb=weighted -count=20 -interval=500ms
```

## 服务器控制命令

在服务器终端中，你可以使用以下命令控制服务状态：

- `status` - 显示服务状态和权重
- `down` - 将服务标记为下线
- `up` - 将服务标记为上线
- `weight <值>` - 更改服务权重
- `exit` - 退出程序

## 测试

运行测试脚本以验证服务发现和负载均衡功能：

```bash
./test.sh
```

测试将运行内置的集成测试，验证以下功能：
- 服务注册和发现
- 不同负载均衡策略下的实例选择
- 服务状态变更的处理
- 服务监控和通知

## 扩展

该示例可以进一步扩展：

1. 实现基于外部系统的服务注册中心（如Consul、etcd）
2. 添加更多负载均衡策略（如最少连接数、响应时间）
3. 实现主动健康检查
4. 添加服务标签筛选功能
5. 实现DNS服务发现 