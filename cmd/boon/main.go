package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"

	"boon/internal/bloom"
	"boon/internal/compute"
	"boon/internal/mnemonic"
)

var (
	mnemonicTemplate = flag.String("mnemonic", "", "助记词模板，未知词用?代替，如: word1 word2 ? word4 ...")
	bloomFile        = flag.String("bloom", "", "Bloom过滤器文件路径")
	batchSize        = flag.Int("batch", 1000, "批次大小")
	workers          = flag.Int("workers", runtime.NumCPU(), "CPU工作线程数")
	outputFile       = flag.String("o", "matches.txt", "匹配结果输出文件")
	verbose          = flag.Bool("v", false, "详细输出")
)

func main() {
	flag.Parse()

	// 1. 解析助记词模板
	if *mnemonicTemplate == "" {
		log.Fatal("请提供助记词模板，使用 -mnemonic 参数")
	}

	words := strings.Fields(*mnemonicTemplate)
	if len(words) != 12 {
		log.Fatalf("助记词必须是12个，当前: %d", len(words))
	}

	// 2. 加载Bloom过滤器
	var bloomFilter *bloom.Filter
	if *bloomFile != "" {
		log.Printf("加载Bloom过滤器: %s", *bloomFile)
		var err error
		bloomFilter, err = bloom.LoadFromFile(*bloomFile)
		if err != nil {
			log.Fatalf("加载Bloom过滤器失败: %v", err)
		}
		log.Println("Bloom过滤器加载完成")
	} else {
		log.Println("警告: 未指定Bloom过滤器文件，将输出所有计算结果")
	}

	// 3. 创建计算器
	computer := compute.NewCPUComputer(*workers)
	defer computer.Close()

	// 4. 创建枚举器
	enumerator := mnemonic.NewEnumerator(words, *batchSize)

	// 5. 统计信息
	var (
		totalProcessed int64
		totalMatches   int64
		statsMu        sync.Mutex
		startTime      = time.Now()
	)

	// 打印统计
	go func() {
		ticker := time.NewTicker(10 * time.Second)
		for range ticker.C {
			elapsed := time.Since(startTime)
			statsMu.Lock()
			processed := totalProcessed
			matches := totalMatches
			statsMu.Unlock()

			rate := float64(processed) / elapsed.Seconds()
			log.Printf("统计: 处理=%d 匹配=%d 速率=%.2f/s",
				processed, matches, rate)
		}
	}()

	// 6. 开始处理
	log.Println("开始枚举助记词...")
	batchChan := enumerator.BatchEnumerate()

	for batch := range batchChan {
		if *verbose {
			log.Printf("处理批次: %d 个助记词", len(batch))
		}

		// 调用计算器获取地址
		addresses, err := computer.Compute(batch)
		if err != nil {
			log.Printf("计算失败: %v", err)
			continue
		}

		// Bloom过滤测试
		for i, address := range addresses {
			if address == nil {
				continue
			}

			statsMu.Lock()
			totalProcessed++
			statsMu.Unlock()

			// 如果没有bloom过滤器，输出所有结果
			if bloomFilter == nil {
				mnemonicStr := strings.Join(batch[i], " ")
				log.Printf("助记词: %s", mnemonicStr)
				log.Printf("地址: %x", address)
				continue
			}

			// Bloom过滤检查
			if bloomFilter.Contains(address) {
				statsMu.Lock()
				totalMatches++
				statsMu.Unlock()

				// 找到匹配
				mnemonicStr := strings.Join(batch[i], " ")
				log.Printf("========== 找到匹配 ==========")
				log.Printf("助记词: %s", mnemonicStr)
				log.Printf("地址: %x", address)
				log.Printf("==============================")

				// 保存到文件
				saveMatch(*outputFile, mnemonicStr, address)
			}
		}
	}

	// 最终统计
	elapsed := time.Since(startTime)
	statsMu.Lock()
	log.Printf("完成！总计: 处理=%d 匹配=%d 耗时=%v",
		totalProcessed, totalMatches, elapsed)
	statsMu.Unlock()
}

// saveMatch 保存匹配结果到文件
func saveMatch(filename, mnemonic string, address []byte) {
	f, err := os.OpenFile(filename, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Printf("无法打开输出文件: %v", err)
		return
	}
	defer f.Close()

	fmt.Fprintf(f, "%s,%x\n", mnemonic, address)
}
