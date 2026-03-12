package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"boon/internal/compute"
	"boon/internal/worker"
)

var (
	schedulerURL = flag.String("scheduler", "http://localhost:8080", "调度服务器地址")
	workerID     = flag.String("id", "", "Worker ID（留空自动生成）")
	workers      = flag.Int("workers", runtime.NumCPU(), "并发worker数量")
	pollInterval = flag.Duration("poll", 100*time.Millisecond, "任务轮询间隔")
)

func main() {
	flag.Parse()

	// 生成Worker ID
	id := *workerID
	if id == "" {
		hostname, _ := os.Hostname()
		id = fmt.Sprintf("%s-%d", hostname, os.Getpid())
	}

	log.Printf("Worker 启动: %s", id)
	log.Printf("调度服务器: %s", *schedulerURL)
	log.Printf("并发数: %d", *workers)

	// 创建HTTP客户端
	client := worker.NewHTTPClient(*schedulerURL)

	// 创建计算器（CPU版本）
	computer := compute.NewCPUComputer(*workers)
	defer computer.Close()

	// 创建Worker
	w := worker.NewWorker(id, client, computer, *workers)
	w.SetPollInterval(*pollInterval)

	// 处理信号
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigCh
		log.Println("收到停止信号，正在关闭...")
		cancel()
	}()

	// 启动Worker
	w.Start(ctx)

	// 等待完成
	<-ctx.Done()
	w.Stop()

	log.Println("Worker 已停止")
}
