package main

import (
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"
)

var (
	port     = flag.Int("port", 3333, "P3P节点监听端口")
	dataDir  = flag.String("data", "./data", "数据存储目录")
)

func main() {
	flag.Parse()

	// 初始化日志
	log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)
	log.Printf("启动P3P节点，监听端口：%d\n", *port)

	// TODO: 初始化节点
	// TODO: 启动发现服务
	// TODO: 启动传输服务

	// 等待中断信号
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	log.Println("正在关闭P3P节点...")
} 