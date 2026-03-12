package compute

import (
	"crypto/sha512"
	"strings"
	"sync"

	"boon/internal/bip44"
	"boon/internal/crypto"
	"golang.org/x/crypto/pbkdf2"
)

// CPUComputer CPU计算器
type CPUComputer struct {
	workers int
}

// NewCPUComputer 创建CPU计算器
func NewCPUComputer(workers int) *CPUComputer {
	if workers <= 0 {
		workers = 4
	}
	return &CPUComputer{workers: workers}
}

// Compute 计算助记词对应的TRON地址
func (c *CPUComputer) Compute(mnemonics [][]string) ([][]byte, error) {
	count := len(mnemonics)
	addresses := make([][]byte, count)

	var wg sync.WaitGroup
	sem := make(chan struct{}, c.workers)

	for i, words := range mnemonics {
		wg.Add(1)
		go func(idx int, w []string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			address := c.computeOne(w)
			addresses[idx] = address
		}(i, words)
	}

	wg.Wait()
	return addresses, nil
}

// computeOne 计算单个助记词对应的TRON地址
func (c *CPUComputer) computeOne(words []string) []byte {
	// 1. 拼接助记词
	mnemonic := strings.Join(words, " ")

	// 2. PBKDF2-HMAC-SHA512 计算种子
	seed := pbkdf2.Key([]byte(mnemonic), []byte("mnemonic"), 2048, 64, sha512.New)

	// 3. BIP44 派生 TRON 路径 m/44'/195'/0'/0/0
	deriver := bip44.NewDeriverFromSeed(seed)
	key, err := deriver.DeriveTRON()
	if err != nil {
		return nil
	}

	// 4. 获取公钥（未压缩，去掉04前缀）
	pubKeyBytes, err := bip44.GetPublicKeyBytes(key)
	if err != nil {
		return nil
	}

	// 5. Keccak256 并取前20字节
	return crypto.Keccak256Hash(pubKeyBytes)
}

// Close 关闭CPU计算器
func (c *CPUComputer) Close() error {
	return nil
}
