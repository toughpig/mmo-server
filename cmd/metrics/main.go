package main

import (
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"mmo-server/pkg/metrics"
)

var (
	// 命令行参数
	listenAddr = flag.String("addr", ":9100", "监控服务监听地址")
	interval   = flag.Duration("interval", 10*time.Second, "指标收集间隔")
)

func main() {
	flag.Parse()

	log.Printf("启动MMO服务器监控系统，监听地址: %s, 收集间隔: %v", *listenAddr, *interval)

	// 创建指标管理器
	manager := metrics.NewMetricsManager()

	// 创建收集器
	collector := metrics.NewMetricsCollector(manager)

	// 添加系统统计收集器
	collector.AddCollector(metrics.SystemStatsCollector())

	// 启动收集器
	collector.Start(*interval)

	// 启动HTTP服务
	if err := manager.StartServer(*listenAddr); err != nil {
		log.Fatalf("启动监控服务失败: %v", err)
	}

	// 等待中断信号
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	log.Println("正在停止监控服务...")

	// 停止收集器
	collector.Stop()

	// 停止HTTP服务
	if err := manager.StopServer(); err != nil {
		log.Printf("停止监控服务失败: %v", err)
	}

	log.Println("监控服务已停止")
}
