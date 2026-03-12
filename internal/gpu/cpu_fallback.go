//go:build !cuda
// +build !cuda

package gpu

import (
	"crypto/sha512"
	"strings"
	"sync"

	"golang.org/x/crypto/pbkdf2"
)

// CPUFallback CPU后备实现
type CPUFallback struct {
	batchSize int
	workers   int
}

// NewCPUFallback 创建CPU后备
func NewCPUFallback(batchSize, workers int) *CPUFallback {
	if workers <= 0 {
		workers = 4
	}
	return &CPUFallback{
		batchSize: batchSize,
		workers:   workers,
	}
}

// ComputeSeedBatch 批量计算种子（CPU版本）
func (c *CPUFallback) ComputeSeedBatch(mnemonics [][]string) ([][]byte, error) {
	count := len(mnemonics)
	seeds := make([][]byte, count)

	var wg sync.WaitGroup
	sem := make(chan struct{}, c.workers)

	for i, words := range mnemonics {
		wg.Add(1)
		go func(idx int, w []string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			mnemonic := strings.Join(w, " ")
			// PBKDF2-HMAC-SHA512, 2048次迭代
			seed := pbkdf2.Key([]byte(mnemonic), []byte("mnemonic"), 2048, 64, sha512.New)
			seeds[idx] = seed
		}(i, words)
	}

	wg.Wait()
	return seeds, nil
}

// IsAvailable CPU始终可用
func IsAvailable() bool {
	return true
}
