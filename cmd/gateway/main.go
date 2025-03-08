package main

import (
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/toughpig/mmo-server/pkg/gateway"
)

func main() {
	// 解析命令行参数
	configPath := flag.String("config", "", "配置文件路径")
	flag.Parse()

	// 加载配置
	config, err := gateway.LoadGatewayConfig(*configPath)
	if err != nil {
		log.Fatalf("Failed to load gateway config: %v", err)
	}

	// 创建网关处理器
	gw, err := gateway.NewGatewayHandler(config)
	if err != nil {
		log.Fatalf("Failed to create gateway: %v", err)
	}

	// 处理系统信号
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// 启动网关
	if err := gw.Start(); err != nil {
		log.Fatalf("Failed to start gateway: %v", err)
	}

	log.Println("Gateway running. Press Ctrl+C to stop.")

	// 等待终止信号
	<-sigChan
	log.Println("Shutting down...")

	// 停止网关
	if err := gw.Stop(); err != nil {
		log.Fatalf("Error stopping gateway: %v", err)
	}

	log.Println("Gateway shutdown complete.")
}
