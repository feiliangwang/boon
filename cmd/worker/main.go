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
	workers      = flag.Int("workers", runtime.NumCPU(), "并发计算worker数量")
	prefetch     = flag.Int("prefetch", 0, "预取任务数量（0=自动，默认为workers*2）")
	pollInterval = flag.Duration("poll", 50*time.Millisecond, "任务轮询间隔")
)

func main() {
	flag.Parse()

	// 生成Worker ID
	id := *workerID
	if id == "" {
		hostname, _ := os.Hostname()
		id = fmt.Sprintf("%s-%d", hostname, os.Getpid())
	}

	prefetchSize := *prefetch
	if prefetchSize <= 0 {
		prefetchSize = *workers * 2 // 默认预取worker数量的2倍
	}

	log.Printf("========================================")
	log.Printf("  Boon Worker 启动")
	log.Printf("========================================")
	log.Printf("  ID:           %s", id)
	log.Printf("  调度服务器:    %s", *schedulerURL)
	log.Printf("  计算线程:      %d", *workers)
	log.Printf("  预取数量:      %d", prefetchSize)
	log.Printf("  轮询间隔:      %v", *pollInterval)
	log.Printf("========================================")

	// 创建HTTP客户端
	client := worker.NewHTTPClient(*schedulerURL)

	// 创建计算器（CPU版本）
	computer := compute.NewCPUComputer(*workers)
	defer computer.Close()

	// 创建Worker
	w := worker.NewWorker(id, client, computer, *workers)
	w.SetPollInterval(*pollInterval)
	w.SetPrefetchSize(prefetchSize)

	// 处理信号
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-sigCh
		log.Printf("收到信号: %v，正在关闭...", sig)
		cancel()
	}()

	// 启动Worker
	w.Start(ctx)

	// 等待完成
	<-ctx.Done()
	w.Stop()

	// 打印最终统计
	stats := w.GetStats()
	log.Printf("========================================")
	log.Printf("  Worker 已停止")
	log.Printf("  总拉取: %d", stats["tasks_fetched"])
	log.Printf("  总计算: %d", stats["tasks_computed"])
	log.Printf("  总提交: %d", stats["tasks_submitted"])
	log.Printf("========================================")
}
