#!/bin/bash

# 设置颜色输出
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# 编译主程序、服务器和客户端
echo -e "${GREEN}正在编译程序...${NC}"
go build -o load_balancer_demo main.go
cd server && go build -o ../server_demo && cd ..
cd client && go build -o ../client_demo && cd ..

# 打印使用说明
echo -e "${YELLOW}=== 服务发现和负载均衡演示 ===${NC}"
echo -e "${BLUE}可以通过以下方式运行演示:${NC}"
echo ""

echo -e "${GREEN}1. 运行内置测试${NC}"
echo -e "${YELLOW}./load_balancer_demo -mode=test${NC}"
echo "  这将运行内置测试，模拟服务注册和客户端请求"
echo ""

echo -e "${GREEN}2. 启动服务器实例${NC}"
echo -e "终端1: ${YELLOW}./server_demo -port=8081 -id=server1 -weight=1 -zone=zone1${NC}"
echo -e "终端2: ${YELLOW}./server_demo -port=8082 -id=server2 -weight=2 -zone=zone1${NC}"
echo -e "终端3: ${YELLOW}./server_demo -port=8083 -id=server3 -weight=3 -zone=zone2${NC}"
echo ""

echo -e "${GREEN}3. 启动客户端${NC}"
echo -e "终端4: ${YELLOW}./client_demo -lb=random -count=12 -interval=500ms${NC}"
echo -e "终端5: ${YELLOW}./client_demo -lb=round-robin -count=9 -interval=500ms${NC}"
echo -e "终端6: ${YELLOW}./client_demo -lb=weighted -count=20 -interval=500ms${NC}"
echo ""

echo -e "${BLUE}提示:${NC}"
echo "1. 在服务器终端中，您可以使用以下命令:"
echo "   - status: 显示服务状态"
echo "   - down: 将服务标记为下线"
echo "   - up: 将服务标记为上线"
echo "   - weight <值>: 更改服务权重"
echo "   - exit: 退出程序"
echo ""
echo "2. 观察不同负载均衡算法下的请求分配情况"
echo "3. 尝试下线某个服务，观察请求自动重新分配"
echo ""

# 确认是否自动启动演示
echo -e "${YELLOW}是否启动内置测试? [y/N]${NC}"
read -r auto_start

if [[ "$auto_start" == "y" || "$auto_start" == "Y" ]]; then
    echo -e "${GREEN}启动内置测试...${NC}"
    ./load_balancer_demo -mode=test
else
    echo -e "${BLUE}请按照上述说明手动启动演示。${NC}"
fi 