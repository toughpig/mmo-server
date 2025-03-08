package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"time"

	"github.com/gorilla/websocket"
)

func main() {
	// 命令行参数
	addr := flag.String("addr", "localhost:8081", "服务器地址")
	path := flag.String("path", "/ws", "WebSocket路径")
	messages := flag.Int("messages", 10, "发送的消息数量")
	interval := flag.Int("interval", 1000, "消息间隔(毫秒)")
	flag.Parse()

	// 建立WebSocket连接
	url := fmt.Sprintf("ws://%s%s", *addr, *path)
	log.Printf("正在连接到 %s", url)

	c, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		log.Fatalf("连接失败: %v", err)
	}
	defer c.Close()

	// 设置中断处理
	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt)

	// 发送/接收通道
	done := make(chan struct{})

	// 接收协程
	go func() {
		defer close(done)
		for {
			_, message, err := c.ReadMessage()
			if err != nil {
				log.Println("接收错误:", err)
				return
			}
			log.Printf("收到: %s", message)
		}
	}()

	// 发送指定数量的消息
	log.Printf("将发送 %d 条消息，间隔 %d ms", *messages, *interval)
	for i := 0; i < *messages; i++ {
		select {
		case <-done:
			return
		case <-interrupt:
			log.Println("中断，正在关闭连接")
			err := c.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
			if err != nil {
				log.Println("关闭错误:", err)
			}
			select {
			case <-done:
			case <-time.After(time.Second):
			}
			return
		default:
			message := fmt.Sprintf("Echo测试消息 #%d", i+1)
			log.Printf("发送: %s", message)
			err := c.WriteMessage(websocket.TextMessage, []byte(message))
			if err != nil {
				log.Println("发送错误:", err)
				return
			}
			time.Sleep(time.Duration(*interval) * time.Millisecond)
		}
	}

	// 等待一会儿，确保所有响应都收到
	time.Sleep(time.Second)

	// 关闭连接
	log.Println("测试完成，正在关闭连接")
	err = c.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
	if err != nil {
		log.Println("关闭错误:", err)
	}
	<-done
}
