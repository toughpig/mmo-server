package main

import (
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/yourusername/mmo-server/pkg/config"
	"github.com/yourusername/mmo-server/pkg/network"
)

func main() {
	// 解析命令行参数
	configPath := flag.String("config", "", "配置文件路径")
	flag.Parse()

	// 加载配置
	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// 创建服务器
	server := network.NewServer(cfg)
	
	// 创建消息处理器
	network.NewMessageProcessor(server)

	// 处理系统信号
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// 启动服务器（异步）
	go func() {
		if err := server.Start(); err != nil {
			log.Fatalf("Server error: %v", err)
		}
	}()

	log.Println("MMO Server started. Press Ctrl+C to stop.")

	// 等待终止信号
	<-sigChan
	log.Println("Shutting down...")
	server.Stop()
	log.Println("Server shutdown complete.")
} 