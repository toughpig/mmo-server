#!/bin/bash

set -e

# 颜色定义
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# 日志函数
log_info() {
  echo -e "${BLUE}[INFO]${NC} $1"
}

log_success() {
  echo -e "${GREEN}[SUCCESS]${NC} $1"
}

log_warn() {
  echo -e "${YELLOW}[WARNING]${NC} $1"
}

log_error() {
  echo -e "${RED}[ERROR]${NC} $1"
}

# 检查操作系统
check_os() {
  OS=$(uname -s)
  log_info "检测到操作系统: $OS"
  
  if [ "$OS" = "Linux" ]; then
    return 0
  elif [ "$OS" = "Darwin" ]; then
    return 0
  else
    log_error "不支持的操作系统: $OS"
    log_error "本测试脚本只支持Linux和macOS"
    return 1
  fi
}

# 检查并配置共享内存参数
setup_shm() {
  log_info "开始配置共享内存参数..."
  
  OS=$(uname -s)
  if [ "$OS" = "Linux" ]; then
    # 显示当前配置
    log_info "当前共享内存配置:"
    sysctl kernel.shmmax kernel.shmall
    
    # 检查/dev/shm大小
    df -h /dev/shm
    
    # 询问是否修改参数
    read -p "是否调整共享内存参数? (y/n): " adjust_params
    if [ "$adjust_params" = "y" ]; then
      sudo sysctl -w kernel.shmmax=536870912
      sudo sysctl -w kernel.shmall=131072
      sudo mount -o remount,size=512M /dev/shm
      log_success "已调整共享内存参数"
    fi
  elif [ "$OS" = "Darwin" ]; then
    # 显示当前配置
    log_info "当前共享内存配置:"
    sysctl kern.sysv.shmmax kern.sysv.shmall
    
    # 询问是否修改参数
    read -p "是否调整共享内存参数? (y/n): " adjust_params
    if [ "$adjust_params" = "y" ]; then
      sudo sysctl -w kern.sysv.shmmax=536870912
      sudo sysctl -w kern.sysv.shmall=131072
      sudo sysctl -w kern.sysv.shmseg=64
      log_success "已调整共享内存参数"
    fi
  fi
}

# 清理Socket文件
cleanup_socket_files() {
  log_info "清理遗留的Socket文件..."
  rm -f /tmp/mmo-rpc-*.sock /tmp/mmo-sync-*.sock
  log_success "Socket文件清理完成"
}

# 测试RPC服务器
test_rpc_server() {
  log_info "启动RPC服务器测试..."
  
  # 启动服务器
  log_info "启动RPC服务器 (30秒后自动退出)..."
  timeout 30s ./bin/rpc_test -mode=server -endpoint=/tmp/mmo-rpc-test.sock &
  SERVER_PID=$!
  
  # 等待服务器启动
  sleep 2
  
  # 检查服务器是否运行
  if ps -p $SERVER_PID > /dev/null; then
    log_success "RPC服务器启动成功 (PID: $SERVER_PID)"
    
    # 运行客户端
    log_info "运行RPC客户端测试..."
    ./bin/rpc_test -mode=client -endpoint=/tmp/mmo-rpc-test.sock
    CLIENT_RESULT=$?
    
    if [ $CLIENT_RESULT -eq 0 ]; then
      log_success "RPC客户端测试成功"
    else
      log_error "RPC客户端测试失败 (退出码: $CLIENT_RESULT)"
      kill -9 $SERVER_PID 2>/dev/null || true
      return 1
    fi
    
    # 运行压力测试
    log_info "运行RPC压力测试..."
    ./bin/rpc_test -mode=stress -endpoint=/tmp/mmo-rpc-test.sock -requests=20 -interval=5
    STRESS_RESULT=$?
    
    if [ $STRESS_RESULT -eq 0 ]; then
      log_success "RPC压力测试成功"
    else
      log_error "RPC压力测试失败 (退出码: $STRESS_RESULT)"
      kill -9 $SERVER_PID 2>/dev/null || true
      return 1
    fi
    
    # 运行并发测试
    log_info "运行RPC并发测试..."
    ./bin/rpc_test -mode=concurrent -endpoint=/tmp/mmo-rpc-test.sock -clients=5 -requests=10
    CONCURRENT_RESULT=$?
    
    if [ $CONCURRENT_RESULT -eq 0 ]; then
      log_success "RPC并发测试成功"
    else
      log_error "RPC并发测试失败 (退出码: $CONCURRENT_RESULT)"
      kill -9 $SERVER_PID 2>/dev/null || true
      return 1
    fi
    
    # 停止服务器
    log_info "停止RPC服务器..."
    kill -15 $SERVER_PID 2>/dev/null || true
    sleep 1
    
    # 确保服务器已停止
    if ps -p $SERVER_PID > /dev/null; then
      log_warn "RPC服务器仍在运行，强制终止..."
      kill -9 $SERVER_PID 2>/dev/null || true
    else
      log_success "RPC服务器已正常停止"
    fi
    
    return 0
  else
    log_error "RPC服务器启动失败"
    return 1
  fi
}

# 测试同步状态系统
test_sync_system() {
  log_info "启动玩家状态同步系统测试..."
  
  # 启动服务器
  log_info "启动同步服务器..."
  ./bin/sync_test -mode=server -endpoint=/tmp/mmo-sync-test.sock &
  SYNC_SERVER_PID=$!
  
  # 等待服务器启动
  sleep 2
  
  # 检查服务器是否运行
  if ps -p $SYNC_SERVER_PID > /dev/null; then
    log_success "同步服务器启动成功 (PID: $SYNC_SERVER_PID)"
    
    # 运行客户端
    log_info "运行同步客户端测试..."
    ./bin/sync_test -mode=client -endpoint=/tmp/mmo-sync-test.sock
    SYNC_CLIENT_RESULT=$?
    
    if [ $SYNC_CLIENT_RESULT -eq 0 ]; then
      log_success "同步客户端测试成功"
    else
      log_error "同步客户端测试失败 (退出码: $SYNC_CLIENT_RESULT)"
    fi
    
    # 停止服务器
    log_info "停止同步服务器..."
    kill -15 $SYNC_SERVER_PID 2>/dev/null || true
    sleep 1
    
    # 确保服务器已停止
    if ps -p $SYNC_SERVER_PID > /dev/null; then
      log_warn "同步服务器仍在运行，强制终止..."
      kill -9 $SYNC_SERVER_PID 2>/dev/null || true
    else
      log_success "同步服务器已正常停止"
    fi
    
    return $SYNC_CLIENT_RESULT
  else
    log_error "同步服务器启动失败"
    return 1
  fi
}

# 运行极简shmipc集成测试
run_minimal_shmipc_test() {
  log_info "运行极简shmipc集成测试..."
  
  # 创建临时文件
  TEMP_DIR=$(mktemp -d)
  SENDER_FILE="$TEMP_DIR/sender.go"
  RECEIVER_FILE="$TEMP_DIR/receiver.go"
  
  # 编写接收方代码
  cat > $RECEIVER_FILE <<EOF
package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/cloudwego/shmipc-go"
)

func main() {
	log.Println("启动shmipc接收方...")
	
	server, err := shmipc.NewServer("/tmp/shmipc-test.sock", nil)
	if err != nil {
		log.Fatalf("创建服务器失败: %v", err)
	}
	
	server.DisableIovectPayload()
	
	go func() {
		log.Println("等待连接...")
		for {
			session, err := server.AcceptSession()
			if err != nil {
				log.Printf("接受连接失败: %v", err)
				continue
			}
			
			log.Println("收到新连接，等待消息...")
			go func(s shmipc.Session) {
				defer s.Close()
				for {
					msg, err := s.Receive()
					if err != nil {
						log.Printf("接收消息失败: %v", err)
						return
					}
					
					content := string(msg.Bytes())
					log.Printf("收到消息: %s", content)
					
					// 回复
					resp := []byte("收到: " + content)
					if err := s.Send(resp); err != nil {
						log.Printf("发送回复失败: %v", err)
						return
					}
				}
			}(session)
		}
	}()
	
	// 等待信号
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh
	
	log.Println("关闭服务器...")
	server.Close()
}
EOF

  # 编写发送方代码
  cat > $SENDER_FILE <<EOF
package main

import (
	"log"
	"time"

	"github.com/cloudwego/shmipc-go"
)

func main() {
	log.Println("启动shmipc发送方...")
	
	// 给服务器一些启动时间
	time.Sleep(1 * time.Second)
	
	client, err := shmipc.NewClient("/tmp/shmipc-test.sock", nil)
	if err != nil {
		log.Fatalf("创建客户端失败: %v", err)
	}
	defer client.Close()
	client.DisableIovectPayload()
	
	session, err := client.NewSession(100)
	if err != nil {
		log.Fatalf("创建会话失败: %v", err)
	}
	defer session.Close()
	
	// 发送5个消息
	for i := 1; i <= 5; i++ {
		msg := []byte(fmt.Sprintf("测试消息 #%d", i))
		log.Printf("发送消息: %s", string(msg))
		
		if err := session.Send(msg); err != nil {
			log.Fatalf("发送消息失败: %v", err)
		}
		
		// 接收响应
		resp, err := session.Receive()
		if err != nil {
			log.Fatalf("接收响应失败: %v", err)
		}
		
		log.Printf("收到响应: %s", string(resp.Bytes()))
		time.Sleep(500 * time.Millisecond)
	}
	
	log.Println("测试完成")
}
EOF

  # 编译并运行测试
  log_info "编译简单shmipc测试程序..."
  
  # 检查是否安装了Go
  if ! command -v go &> /dev/null; then
    log_error "未找到Go编译器，跳过此测试"
    rm -rf $TEMP_DIR
    return 1
  fi
  
  OLD_DIR=$(pwd)
  cd $TEMP_DIR
  
  # 初始化模块
  go mod init shmipc_test
  go get github.com/cloudwego/shmipc-go
  
  # 编译
  go build -o receiver $RECEIVER_FILE
  go build -o sender $SENDER_FILE
  
  # 运行接收方
  log_info "启动接收方..."
  ./receiver &
  RECEIVER_PID=$!
  
  # 等待接收方启动
  sleep 2
  
  # 检查接收方是否运行
  if ps -p $RECEIVER_PID > /dev/null; then
    log_success "接收方启动成功 (PID: $RECEIVER_PID)"
    
    # 运行发送方
    log_info "启动发送方..."
    ./sender
    SENDER_RESULT=$?
    
    if [ $SENDER_RESULT -eq 0 ]; then
      log_success "简单shmipc测试成功"
    else
      log_error "简单shmipc测试失败 (退出码: $SENDER_RESULT)"
    fi
    
    # 停止接收方
    log_info "停止接收方..."
    kill -15 $RECEIVER_PID 2>/dev/null || true
    sleep 1
    
    # 确保接收方已停止
    if ps -p $RECEIVER_PID > /dev/null; then
      log_warn "接收方仍在运行，强制终止..."
      kill -9 $RECEIVER_PID 2>/dev/null || true
    else
      log_success "接收方已正常停止"
    fi
    
    cd $OLD_DIR
    rm -rf $TEMP_DIR
    
    return $SENDER_RESULT
  else
    log_error "接收方启动失败"
    cd $OLD_DIR
    rm -rf $TEMP_DIR
    return 1
  fi
}

# 主函数
main() {
  log_info "======== MMO服务器共享内存组件测试 ========"
  
  # 检查操作系统
  check_os || exit 1
  
  # 清理socket文件
  cleanup_socket_files
  
  # 配置共享内存参数
  setup_shm
  
  # 确认二进制文件已编译
  if [ ! -f "./bin/rpc_test" ] || [ ! -f "./bin/sync_test" ]; then
    log_info "编译测试二进制文件..."
    chmod +x build.sh && ./build.sh
  fi
  
  # 运行极简shmipc测试
  run_minimal_shmipc_test
  MINIMAL_RESULT=$?
  
  if [ $MINIMAL_RESULT -ne 0 ]; then
    log_warn "极简shmipc测试失败，这可能表明系统共享内存配置有问题"
    log_warn "建议检查系统配置后再继续"
    read -p "仍然继续测试其他组件? (y/n): " continue_testing
    if [ "$continue_testing" != "y" ]; then
      log_info "测试中止"
      exit 1
    fi
  fi
  
  # 测试RPC服务器
  test_rpc_server
  RPC_RESULT=$?
  
  if [ $RPC_RESULT -ne 0 ]; then
    log_error "RPC服务器测试失败"
  else
    log_success "RPC服务器测试成功"
    
    # 只有RPC测试成功才测试同步系统
    test_sync_system
    SYNC_RESULT=$?
    
    if [ $SYNC_RESULT -ne 0 ]; then
      log_error "玩家状态同步系统测试失败"
    else
      log_success "玩家状态同步系统测试成功"
    fi
  fi
  
  log_info "======== 测试完成 ========"
  
  # 总结测试结果
  echo ""
  echo "测试结果摘要:"
  echo "-------------------"
  
  if [ $MINIMAL_RESULT -eq 0 ]; then
    echo -e "极简shmipc测试: ${GREEN}通过${NC}"
  else
    echo -e "极简shmipc测试: ${RED}失败${NC}"
  fi
  
  if [ $RPC_RESULT -eq 0 ]; then
    echo -e "RPC服务器测试: ${GREEN}通过${NC}"
  else
    echo -e "RPC服务器测试: ${RED}失败${NC}"
  fi
  
  if [ $RPC_RESULT -eq 0 ]; then
    if [ $SYNC_RESULT -eq 0 ]; then
      echo -e "玩家状态同步测试: ${GREEN}通过${NC}"
    else
      echo -e "玩家状态同步测试: ${RED}失败${NC}"
    fi
  else
    echo -e "玩家状态同步测试: ${YELLOW}未执行${NC}"
  fi
  
  echo "-------------------"
  
  # 返回综合结果
  if [[ $MINIMAL_RESULT -eq 0 && $RPC_RESULT -eq 0 && $SYNC_RESULT -eq 0 ]]; then
    log_success "所有测试都通过了!"
    return 0
  else
    log_error "有些测试失败了，请检查上面的日志以获取详细信息"
    return 1
  fi
}

# 执行主函数
main "$@" 