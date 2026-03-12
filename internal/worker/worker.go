package worker

import (
	"boon/internal/compute"
	"boon/internal/protocol"
	"context"
	"log"
	"sync"
	"time"
)

// Worker 计算节点
type Worker struct {
	id       string
	computer compute.SeedComputer
	client   SchedulerClient

	pollInterval time.Duration
	workers      int

	mu       sync.Mutex
	running  bool
	stopCh   chan struct{}
	wg       sync.WaitGroup
}

// SchedulerClient 调度器客户端接口
type SchedulerClient interface {
	FetchTask(workerID string) (*protocol.Task, error)
	SubmitResult(workerID string, result *protocol.Result) error
}

// NewWorker 创建计算节点
func NewWorker(id string, client SchedulerClient, computer compute.SeedComputer, workers int) *Worker {
	return &Worker{
		id:           id,
		computer:     computer,
		client:       client,
		pollInterval: 100 * time.Millisecond,
		workers:      workers,
		stopCh:       make(chan struct{}),
	}
}

// Start 启动worker
func (w *Worker) Start(ctx context.Context) {
	w.mu.Lock()
	if w.running {
		w.mu.Unlock()
		return
	}
	w.running = true
	w.mu.Unlock()

	for i := 0; i < w.workers; i++ {
		w.wg.Add(1)
		go w.runWorker(ctx)
	}
}

// Stop 停止worker
func (w *Worker) Stop() {
	w.mu.Lock()
	if !w.running {
		w.mu.Unlock()
		return
	}
	w.running = false
	close(w.stopCh)
	w.mu.Unlock()

	w.wg.Wait()
}

// SetPollInterval 设置轮询间隔
func (w *Worker) SetPollInterval(d time.Duration) {
	w.pollInterval = d
}

// runWorker 运行单个worker协程
func (w *Worker) runWorker(ctx context.Context) {
	defer w.wg.Done()

	for {
		select {
		case <-ctx.Done():
			return
		case <-w.stopCh:
			return
		default:
		}

		// 获取任务
		task, err := w.client.FetchTask(w.id)
		if err != nil {
			log.Printf("获取任务失败: %v", err)
			time.Sleep(time.Second)
			continue
		}

		if task == nil {
			// 没有任务，等待
			time.Sleep(w.pollInterval)
			continue
		}

		// 计算地址
		addresses, err := w.computer.Compute(task.Mnemonics)
		if err != nil {
			log.Printf("计算失败 [task=%d]: %v", task.ID, err)
			continue
		}

		// 提交结果
		result := &protocol.Result{
			TaskID:    task.ID,
			Addresses: addresses,
		}

		if err := w.client.SubmitResult(w.id, result); err != nil {
			log.Printf("提交结果失败 [task=%d]: %v", task.ID, err)
		}
	}
}
