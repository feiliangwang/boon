package main

import (
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"boon/internal/bloom"
	"boon/internal/mnemonic"
	"boon/internal/protocol"
)

var (
	mnemonicTemplate = flag.String("mnemonic", "", "助记词模板，未知词用?代替")
	bloomFile        = flag.String("bloom", "", "Bloom过滤器文件路径")
	batchSize        = flag.Int("batch", 1000, "批次大小")
	port             = flag.Int("port", 8080, "HTTP服务端口")
	outputFile       = flag.String("o", "matches.txt", "匹配结果输出文件")
)

// SchedulerServer 调度服务器
type SchedulerServer struct {
	taskQueue   chan *protocol.Task
	taskMap     map[int]*protocol.Task
	taskMapMu   sync.RWMutex
	bloomFilter *bloom.Filter

	taskID    int
	taskIDMu  sync.Mutex

	matchFile *os.File
	matchMu   sync.Mutex

	stats struct {
		sync.Mutex
		totalTasks     int64
		completedTasks int64
		matches        int64
		startTime      time.Time
	}
}

func main() {
	flag.Parse()

	if *mnemonicTemplate == "" {
		log.Fatal("请提供助记词模板，使用 -mnemonic 参数")
	}

	words := strings.Fields(*mnemonicTemplate)
	if len(words) != 12 {
		log.Fatalf("助记词必须是12个，当前: %d", len(words))
	}

	// 加载Bloom过滤器
	var bloomFilter *bloom.Filter
	if *bloomFile != "" {
		log.Printf("加载Bloom过滤器: %s", *bloomFile)
		var err error
		bloomFilter, err = bloom.LoadFromFile(*bloomFile)
		if err != nil {
			log.Fatalf("加载Bloom过滤器失败: %v", err)
		}
		log.Println("Bloom过滤器加载完成")
	}

	// 创建服务器
	server := &SchedulerServer{
		taskQueue:   make(chan *protocol.Task, 1000),
		taskMap:     make(map[int]*protocol.Task),
		bloomFilter: bloomFilter,
	}
	server.stats.startTime = time.Now()

	// 打开匹配文件
	f, err := os.OpenFile(*outputFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Fatalf("无法打开输出文件: %v", err)
	}
	server.matchFile = f

	// 启动枚举
	enumerator := mnemonic.NewEnumerator(words, *batchSize)
	go server.runEnumerator(enumerator)

	// 启动统计
	go server.printStats()

	// 启动HTTP服务
	http.HandleFunc("/api/task/fetch", server.handleFetchTask)
	http.HandleFunc("/api/task/submit", server.handleSubmitResult)
	http.HandleFunc("/api/stats", server.handleStats)

	addr := fmt.Sprintf(":%d", *port)
	log.Printf("调度服务器启动: %s", addr)
	log.Fatal(http.ListenAndServe(addr, nil))
}

// runEnumerator 运行枚举器
func (s *SchedulerServer) runEnumerator(enum *mnemonic.Enumerator) {
	batchChan := enum.BatchEnumerate()
	for batch := range batchChan {
		s.taskIDMu.Lock()
		s.taskID++
		taskID := s.taskID
		s.taskIDMu.Unlock()

		task := &protocol.Task{
			ID:        taskID,
			Mnemonics: batch,
		}

		s.taskMapMu.Lock()
		s.taskMap[taskID] = task
		s.taskMapMu.Unlock()

		s.stats.Lock()
		s.stats.totalTasks++
		s.stats.Unlock()

		s.taskQueue <- task
	}
}

// handleFetchTask 处理任务获取请求
func (s *SchedulerServer) handleFetchTask(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", 405)
		return
	}

	select {
	case task := <-s.taskQueue:
		json.NewEncoder(w).Encode(protocol.TaskResponse{
			Task:  task,
			Count: len(s.taskQueue),
		})
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}

// handleSubmitResult 处理结果提交
func (s *SchedulerServer) handleSubmitResult(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", 405)
		return
	}

	var req struct {
		WorkerID  string   `json:"worker_id"`
		TaskID    int      `json:"task_id"`
		Addresses []string `json:"addresses"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}

	// 获取原始任务
	s.taskMapMu.RLock()
	task, exists := s.taskMap[req.TaskID]
	s.taskMapMu.RUnlock()

	if !exists {
		http.Error(w, "Task not found", 404)
		return
	}

	// 检查匹配
	matches := 0
	for i, addrHex := range req.Addresses {
		if addrHex == "" {
			continue
		}

		addr, err := hex.DecodeString(addrHex)
		if err != nil {
			continue
		}

		if s.bloomFilter == nil || s.bloomFilter.Contains(addr) {
			matches++
			mnemonic := strings.Join(task.Mnemonics[i], " ")
			s.saveMatch(mnemonic, addr)
			log.Printf("========== 找到匹配 ==========")
			log.Printf("Worker: %s", req.WorkerID)
			log.Printf("助记词: %s", mnemonic)
			log.Printf("地址: %x", addr)
			log.Printf("==============================")
		}
	}

	// 清理任务
	s.taskMapMu.Lock()
	delete(s.taskMap, req.TaskID)
	s.taskMapMu.Unlock()

	s.stats.Lock()
	s.stats.completedTasks++
	s.stats.matches += int64(matches)
	s.stats.Unlock()

	json.NewEncoder(w).Encode(protocol.ResultResponse{
		Success: true,
		Matches: matches,
	})
}

// handleStats 处理统计请求
func (s *SchedulerServer) handleStats(w http.ResponseWriter, r *http.Request) {
	s.stats.Lock()
	elapsed := time.Since(s.stats.startTime)
	stats := map[string]interface{}{
		"total_tasks":     s.stats.totalTasks,
		"completed_tasks": s.stats.completedTasks,
		"pending_tasks":   len(s.taskQueue),
		"matches":         s.stats.matches,
		"elapsed":         elapsed.String(),
		"rate":            float64(s.stats.completedTasks) / elapsed.Seconds(),
	}
	s.stats.Unlock()

	json.NewEncoder(w).Encode(stats)
}

// saveMatch 保存匹配结果
func (s *SchedulerServer) saveMatch(mnemonic string, addr []byte) {
	s.matchMu.Lock()
	defer s.matchMu.Unlock()
	fmt.Fprintf(s.matchFile, "%s,%x\n", mnemonic, addr)
}

// printStats 打印统计信息
func (s *SchedulerServer) printStats() {
	ticker := time.NewTicker(10 * time.Second)
	for range ticker.C {
		s.stats.Lock()
		elapsed := time.Since(s.stats.startTime)
		rate := float64(s.stats.completedTasks) / elapsed.Seconds()
		log.Printf("统计: 任务=%d/%d 匹配=%d 速率=%.2f/s",
			s.stats.completedTasks, s.stats.totalTasks, s.stats.matches, rate)
		s.stats.Unlock()
	}
}
