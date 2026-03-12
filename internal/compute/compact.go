package compute

import (
	"crypto/sha512"
	"strings"
	"sync"

	"boon/internal/bip44"
	"boon/internal/crypto"
	"boon/internal/mnemonic"
	"boon/internal/protocol"
	"golang.org/x/crypto/pbkdf2"
)

// Enumerator 枚举器接口
type Enumerator interface {
	EnumerateAt(idx int64, validator *mnemonic.Validator) ([]string, bool)
}

// CompactComputer 紧凑计算器
type CompactComputer struct {
	workers int
}

// NewCompactComputer 创建紧凑计算器
func NewCompactComputer(workers int) *CompactComputer {
	if workers <= 0 {
		workers = 4
	}
	return &CompactComputer{workers: workers}
}

// ComputeRange 计算位置范围内的匹配
func (c *CompactComputer) ComputeRange(
	enum Enumerator,
	task *protocol.CompactTask,
	bloomFilter func([]byte) bool,
) *protocol.CompactResult {
	result := &protocol.CompactResult{
		TaskID:  task.TaskID,
		Matches: make([]protocol.MatchData, 0),
	}

	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, c.workers)

	// 按批次并行处理
	batchSize := int64(500)
	for start := task.StartIdx; start < task.EndIdx; start += batchSize {
		end := start + batchSize
		if end > task.EndIdx {
			end = task.EndIdx
		}

		wg.Add(1)
		go func(s, e int64) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			matches := c.processBatch(enum, s, e, bloomFilter)
			if len(matches) > 0 {
				mu.Lock()
				result.Matches = append(result.Matches, matches...)
				mu.Unlock()
			}
		}(start, end)
	}

	wg.Wait()
	return result
}

// processBatch 处理一个批次
func (c *CompactComputer) processBatch(
	enum Enumerator,
	start, end int64,
	bloomFilter func([]byte) bool,
) []protocol.MatchData {
	matches := make([]protocol.MatchData, 0)
	validator := mnemonic.NewValidator()

	for idx := start; idx < end; idx++ {
		// 枚举位置
		words, valid := enum.EnumerateAt(idx, validator)
		if !valid {
			continue
		}

		// 计算地址
		address := c.computeAddress(words)
		if address == nil {
			continue
		}

		// Bloom过滤（如果有）
		if bloomFilter != nil && !bloomFilter(address) {
			continue
		}

		// 匹配
		matches = append(matches, protocol.MatchData{
			Index:   idx,
			Address: address,
		})
	}

	return matches
}

// computeAddress 计算地址（20 bytes）
func (c *CompactComputer) computeAddress(words []string) []byte {
	mnemonicStr := strings.Join(words, " ")
	seed := pbkdf2.Key([]byte(mnemonicStr), []byte("mnemonic"), 2048, 64, sha512.New)

	deriver := bip44.NewDeriverFromSeed(seed)
	key, err := deriver.DeriveTRON()
	if err != nil {
		return nil
	}

	pubKeyBytes, err := bip44.GetPublicKeyBytes(key)
	if err != nil {
		return nil
	}

	return crypto.Keccak256Hash(pubKeyBytes)
}
