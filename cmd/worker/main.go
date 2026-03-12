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

	"boon/internal/worker"
)

var (
	schedulerURL = flag.String("scheduler", "http://localhost:8080", "调度服务器地址")
	workerID     = flag.String("id", "", "Worker ID（留空自动生成）")
	workers      = flag.Int("workers", runtime.NumCPU(), "并发计算线程数")
	batchSize    = flag.Int64("batch", 10000, "每批次枚举数量")
)

func main() {
	flag.Parse()

	// 生成Worker ID
	id := *workerID
	if id == "" {
		hostname, _ := os.Hostname()
		id = fmt.Sprintf("%s-%d", hostname, os.Getpid())
	}

	log.Printf("========================================")
	log.Printf("  Boon Worker v2 (紧凑协议)")
	log.Printf("========================================")
	log.Printf("  ID:           %s", id)
	log.Printf("  调度服务器:    %s", *schedulerURL)
	log.Printf("  计算线程:      %d", *workers)
	log.Printf("  批次大小:      %d", *batchSize)
	log.Printf("========================================")

	// 创建紧凑协议客户端
	client := worker.NewCompactClient(*schedulerURL, *batchSize)

	// 创建紧凑Worker
	w := worker.NewCompactWorker(id, client, *workers)

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
	startTime := time.Now()
	w.Start(ctx)

	// 等待完成
	<-ctx.Done()
	w.Stop()

	// 打印最终统计
	elapsed := time.Since(startTime)
	log.Printf("========================================")
	log.Printf("  Worker 已停止")
	log.Printf("  运行时间: %v", elapsed)
	log.Printf("========================================")
}
