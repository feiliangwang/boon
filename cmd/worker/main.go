package main

import (
	"bytes"
	"context"
	"encoding/hex"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"syscall"
	"time"

	bip39 "github.com/tyler-smith/go-bip39"

	"boon/internal/bloom"
	"boon/internal/compute"
	"boon/internal/mnemonic"
	"boon/internal/protocol"
	"boon/internal/worker"
)

var (
	schedulerURL = flag.String("scheduler", "http://localhost:8080", "调度服务器地址")
	workerID     = flag.String("id", "", "Worker ID（留空自动生成）")
	workers      = flag.Int("workers", runtime.NumCPU(), "并发计算线程数")
	bloomFile    = flag.String("bloom", "account.bin.bloom", "Bloom过滤器文件（本地加载）")
	useGPU       = flag.Bool("gpu", false, "使用GPU加速（需要CUDA构建）")
	benchN       = flag.Int("bench", 0, "测速模式：随机生成N个助记词测算计算速度（0=禁用）")
	verifyN      = flag.Int("verify", 0, "验证模式：随机生成N个助记词对比GPU与CPU结果（0=禁用）")
	benchFullN   = flag.Int64("bench-full", 0, "全链路测速：模拟完整枚举→验证→计算流程，扫描N个索引（0=禁用）")
)
func main() {
	flag.Parse()

	// 独立模式：测速
	if *benchN > 0 {
		runBench(*benchN, *useGPU, *workers)
		return
	}

	// 独立模式：验证
	if *verifyN > 0 {
		runVerify(*verifyN, *workers)
		return
	}

	// 独立模式：全链路测速
	if *benchFullN > 0 {
		runBenchFull(*benchFullN, *useGPU, *workers)
		return
	}

	// 正常 Worker 模式
	id := *workerID
	if id == "" {
		hostname, _ := os.Hostname()
		id = fmt.Sprintf("%s-%d", hostname, os.Getpid())
	}

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

	log.Printf("========================================")
	log.Printf("  Boon Worker v2 (紧凑协议)")
	log.Printf("========================================")
	log.Printf("  ID:           %s", id)
	log.Printf("  调度服务器:    %s", *schedulerURL)
	log.Printf("  计算线程:      %d", *workers)
	log.Printf("  Bloom过滤:     %s", boolStr(bloomFilter != nil, "已加载", "未加载"))
	log.Printf("  GPU加速:       %s", boolStr(*useGPU, "启用", "禁用"))
	log.Printf("========================================")

	var seedComp compute.SeedComputer
	gpuMode := false
	if *useGPU {
		gpu, err := compute.NewGPUComputer()
		if err != nil {
			log.Printf("GPU初始化失败，回退到CPU: %v", err)
			seedComp = compute.NewCPUComputer()
		} else {
			log.Printf("GPU计算器初始化成功")
			seedComp = gpu
			gpuMode = true
		}
	} else {
		seedComp = compute.NewCPUComputer()
	}

	client := worker.NewCompactClient(*schedulerURL)
	// GPU 模式：workers=1（避免并发调用 CUDA）；CPU 模式：使用指定并发数
	effectiveWorkers := *workers
	if gpuMode {
		effectiveWorkers = 1
	}
	w := worker.NewCompactWorkerWithComputer(id, client, effectiveWorkers, seedComp)
	if gpuMode {
		// GPU 最优批次大小 65536，单次大批量调用效率最高
		w.SetBatchSize(65536)
		// 多 CPU 核并行枚举，与 GPU 计算流水线重叠
		w.SetEnumWorkers(runtime.NumCPU())
	}
	w.SetBloomFilter(bloomFilter)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigCh
		log.Printf("收到信号: %v，正在关闭...", sig)
		cancel()
	}()

	startTime := time.Now()
	w.Start(ctx)
	<-ctx.Done()
	w.Stop()

	elapsed := time.Since(startTime)
	log.Printf("========================================")
	log.Printf("  Worker 已停止，运行时间: %v", elapsed)
	log.Printf("========================================")
}

// ================================================================
// 测速模式
// ================================================================

func runBench(total int, gpu bool, cpuWorkers int) {
	// 生成助记词
	fmt.Printf("生成 %d 个随机助记词...\n", total)
	mnemonics, err := genMnemonics(total)
	if err != nil {
		log.Fatalf("生成助记词失败: %v", err)
	}

	// 选择计算引擎
	var comp compute.SeedComputer
	compName := "CPU"
	if gpu {
		g, err := compute.NewGPUComputer()
		if err != nil {
			log.Fatalf("GPU初始化失败: %v\n提示: 需要 CUDA 构建且有可用 GPU", err)
		}
		comp = g
		compName = "GPU"
	} else {
		comp = compute.NewCPUComputer()
		compName = fmt.Sprintf("CPU(单线程)")
	}

	fmt.Printf("\n计算引擎: %s\n", compName)
	fmt.Printf("助记词总数: %d\n\n", total)

	batchSizes := []int{1, 8, 16, 32, 64, 128, 256, 512, 1024, 2048}

	// 表头
	fmt.Printf("%-10s  %-12s  %-10s  %-14s\n", "批次大小", "已计算", "耗时", "速度(个/s)")
	fmt.Printf("%-10s  %-12s  %-10s  %-14s\n",
		"----------", "------------", "----------", "--------------")

	// 热身（第一次 CUDA kernel launch 较慢）
	warmupN := 32
	if warmupN > total {
		warmupN = total
	}
	comp.Compute(mnemonics[:warmupN])

	for _, bs := range batchSizes {
		if bs > total {
			break
		}
		computed := 0
		start := time.Now()
		for i := 0; i+bs <= total; i += bs {
			comp.Compute(mnemonics[i : i+bs])
			computed += bs
		}
		elapsed := time.Since(start)
		speed := float64(computed) / elapsed.Seconds()
		fmt.Printf("%-10d  %-12d  %-10s  %-.0f\n",
			bs, computed, fmtDuration(elapsed), speed)
	}

	fmt.Println()
}

// ================================================================
// 验证模式
// ================================================================

func runVerify(total int, cpuWorkers int) {
	gpuComp, err := compute.NewGPUComputer()
	if err != nil {
		log.Fatalf("GPU初始化失败: %v\n提示: 需要 CUDA 构建且有可用 GPU", err)
	}
	cpuComp := compute.NewCPUComputer()

	fmt.Printf("生成 %d 个随机助记词...\n", total)
	mnemonics, err := genMnemonics(total)
	if err != nil {
		log.Fatalf("生成助记词失败: %v", err)
	}

	// 用不同批次大小分段验证，确保边界情况也测到
	batchSizes := buildVerifyBatches(total)

	fmt.Printf("\n验证 GPU(compact) vs CPU(compact) 地址一致性\n")
	fmt.Printf("助记词总数: %d\n\n", total)

	totalChecked := 0
	totalFail := 0
	offset := 0

	fmt.Printf("%-10s  %-8s  %-8s  %-8s  %s\n", "批次大小", "已校验", "通过", "失败", "状态")
	fmt.Printf("%-10s  %-8s  %-8s  %-8s  %s\n",
		"----------", "--------", "--------", "--------", "------")

	for _, bs := range batchSizes {
		end := offset + bs
		if end > total {
			end = total
		}
		batch := mnemonics[offset:end]
		n := len(batch)
		if n == 0 {
			break
		}

		gpuAddrs := gpuComp.Compute(batch)
		cpuAddrs := cpuComp.Compute(batch)

		fail := 0
		for i := range batch {
			if !bytes.Equal(gpuAddrs[i], cpuAddrs[i]) {
				fail++
				fmt.Printf("  MISMATCH [%d] mnemonic: %q\n", offset+i, batch[i])
				fmt.Printf("    GPU: %s\n", hex.EncodeToString(gpuAddrs[i]))
				fmt.Printf("    CPU: %s\n", hex.EncodeToString(cpuAddrs[i]))
			}
		}

		totalChecked += n
		totalFail += fail
		offset = end

		status := "OK"
		if fail > 0 {
			status = "FAIL"
		}
		fmt.Printf("%-10d  %-8d  %-8d  %-8d  %s\n",
			bs, totalChecked, totalChecked-totalFail, totalFail, status)

		if offset >= total {
			break
		}
	}

	fmt.Println()
	if totalFail == 0 {
		fmt.Printf("✓ 全部通过  %d/%d 个地址与CPU一致\n", totalChecked, totalChecked)
	} else {
		fmt.Printf("✗ 验证失败  %d/%d 个不一致\n", totalFail, totalChecked)
		os.Exit(1)
	}
}

// ================================================================
// 全链路测速模式
// ================================================================

func runBenchFull(total int64, gpu bool, cpuWorkers int) {
	knownWords := []string{"abandon", "abandon", "abandon", "abandon", "abandon", "abandon", "abandon", "abandon", "", "", "", ""}

	var seedComp compute.SeedComputer
	compName := "CPU"
	if gpu {
		g, err := compute.NewGPUComputer()
		if err != nil {
			log.Fatalf("GPU初始化失败: %v\n提示: 需要 CUDA 构建且有可用 GPU", err)
		}
		seedComp = g
		compName = "GPU"
	} else {
		seedComp = compute.NewCPUComputer()
	}

	fmt.Printf("全链路性能分析 (bench-full)\n")
	fmt.Printf("计算引擎: %s", compName)
	if !gpu {
		fmt.Printf(" (%d线程)", cpuWorkers)
	}
	fmt.Printf("\n模板: 8个已知词 + 4个未知词 (positions 8-11)\n")
	fmt.Printf("索引范围: [0, %d)\n\n", total)

	// ── 阶段1：纯枚举+BIP39校验（CPU，单线程）─────────────────────
	fmt.Printf("── 阶段1: 纯枚举+BIP39校验 ──\n")
	var validMnemonics []string
	{
		enum := worker.NewLocalEnumerator(&worker.TaskTemplate{
			JobID: 1, Words: append([]string(nil), knownWords...), UnknownPos: []int{8, 9, 10, 11},
		})
		validator := mnemonic.NewValidator()
		start := time.Now()
		for idx := int64(0); idx < total; idx++ {
			words, ok := enum.EnumerateAt(idx, validator)
			if ok {
				validMnemonics = append(validMnemonics, strings.Join(words, " "))
			}
		}
		elapsed := time.Since(start)
		validCount := len(validMnemonics)
		passRate := float64(validCount) / float64(total) * 100
		fmt.Printf("  总索引: %d  有效助记词: %d (%.1f%%)\n", total, validCount, passRate)
		fmt.Printf("  耗时: %s  速度: %.0f 索引/s\n\n", fmtDuration(elapsed), float64(total)/elapsed.Seconds())
	}

	// ── 阶段2：纯计算（GPU/CPU，不含枚举）────────────────────────
	fmt.Printf("── 阶段2: 纯计算（跳过枚举，直接送入%s）──\n", compName)
	{
		batchSize := 65536
		if !gpu {
			batchSize = 500
		}
		// 热身
		warmupN := batchSize
		if warmupN > len(validMnemonics) {
			warmupN = len(validMnemonics)
		}
		seedComp.Compute(validMnemonics[:warmupN])

		computed := 0
		start := time.Now()
		for i := 0; i < len(validMnemonics); i += batchSize {
			end := i + batchSize
			if end > len(validMnemonics) {
				end = len(validMnemonics)
			}
			seedComp.Compute(validMnemonics[i:end])
			computed += end - i
		}
		elapsed := time.Since(start)
		fmt.Printf("  计算助记词: %d  批次大小: %d\n", computed, batchSize)
		fmt.Printf("  耗时: %s  速度: %.0f 助记词/s\n\n", fmtDuration(elapsed), float64(computed)/elapsed.Seconds())
	}

	// ── 阶段3：完整流水线（枚举→校验→计算串联）───────────────────
	fmt.Printf("── 阶段3: 完整流水线 ──\n")
	{
		effectiveWorkers := cpuWorkers
		if gpu {
			effectiveWorkers = 1
		}
		batchSizes := []int64{500, 2048, 8192, 32768, 65536, 131072, 262144}
		if gpu {
			batchSizes = []int64{65536}
		}

		// 热身
		{
			warmupTask := &protocol.CompactTask{TaskID: 0, JobID: 1, StartIdx: 0, EndIdx: 2048}
			warmupEnum := worker.NewLocalEnumerator(&worker.TaskTemplate{
				JobID: 1, Words: append([]string(nil), knownWords...), UnknownPos: []int{8, 9, 10, 11},
			})
			cc := compute.NewCompactComputer(effectiveWorkers, seedComp)
			cc.SetBatchSize(2048)
			if gpu {
				cc.SetEnumWorkers(runtime.NumCPU())
			}
			cc.ComputeRange(warmupEnum, warmupTask, nil)
		}

		task := &protocol.CompactTask{TaskID: 1, JobID: 1, StartIdx: 0, EndIdx: total}

		fmt.Printf("  %-12s  %-10s  %-14s  %s\n", "批次大小", "耗时", "速度(索引/s)", "喂入GPU(助记词/s)")
		fmt.Printf("  %-12s  %-10s  %-14s  %s\n", "------------", "----------", "--------------", "-----------------")

		for _, bs := range batchSizes {
			enum := worker.NewLocalEnumerator(&worker.TaskTemplate{
				JobID: 1, Words: append([]string(nil), knownWords...), UnknownPos: []int{8, 9, 10, 11},
			})
			cc := compute.NewCompactComputer(effectiveWorkers, seedComp)
			cc.SetBatchSize(bs)
			if gpu {
				cc.SetEnumWorkers(runtime.NumCPU())
			}

			start := time.Now()
			result := cc.ComputeRange(enum, task, nil)
			elapsed := time.Since(start)
			_ = result

			idxSpeed := float64(total) / elapsed.Seconds()
			validPerSec := idxSpeed * float64(len(validMnemonics)) / float64(total)
			fmt.Printf("  %-12d  %-10s  %-14.0f  %.0f\n",
				bs, fmtDuration(elapsed), idxSpeed, validPerSec)
		}
		fmt.Println()
	}
}

// buildVerifyBatches 返回覆盖 total 个元素的批次序列：
// 先用小批次（边界测试），再用大批次。
func buildVerifyBatches(total int) []int {
	sizes := []int{1, 1, 2, 4, 8, 16, 32, 64, 128, 256, 512, 1024}
	var result []int
	remaining := total
	for _, s := range sizes {
		if remaining <= 0 {
			break
		}
		if s > remaining {
			s = remaining
		}
		result = append(result, s)
		remaining -= s
	}
	// 剩余部分用 512 块
	for remaining > 0 {
		s := 512
		if s > remaining {
			s = remaining
		}
		result = append(result, s)
		remaining -= s
	}
	return result
}

// ================================================================
// 工具函数
// ================================================================

func genMnemonics(n int) ([]string, error) {
	out := make([]string, n)
	for i := range out {
		entropy, err := bip39.NewEntropy(128) // 12 words
		if err != nil {
			return nil, err
		}
		mn, err := bip39.NewMnemonic(entropy)
		if err != nil {
			return nil, err
		}
		out[i] = mn
	}
	return out, nil
}

func fmtDuration(d time.Duration) string {
	if d < time.Millisecond {
		return fmt.Sprintf("%dµs", d.Microseconds())
	}
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	return fmt.Sprintf("%.2fs", d.Seconds())
}

func boolStr(cond bool, trueStr, falseStr string) string {
	if cond {
		return trueStr
	}
	return falseStr
}
