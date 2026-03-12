package mnemonic

import (
	"sync"
)

// Enumerator 助记词枚举器
type Enumerator struct {
	words      []string
	indices    []int
	validator  *Validator
	batchSize  int
	batchChan  chan []string
	workerChan chan struct{}
	wg         sync.WaitGroup
}

// NewEnumerator 创建枚举器
func NewEnumerator(template []string, batchSize int) *Enumerator {
	return &Enumerator{
		words:      template,
		indices:    GetUnknownIndices(template),
		validator:  NewValidator(),
		batchSize:  batchSize,
		batchChan:  make(chan []string, 100),
		workerChan: make(chan struct{}, 4), // 4个并发worker
	}
}

// Enumerate 开始枚举
func (e *Enumerator) Enumerate() <-chan []string {
	resultChan := make(chan []string, 1000)

	go func() {
		defer close(resultChan)

		unknownCount := len(e.indices)
		if unknownCount == 0 {
			// 没有未知词，直接验证
			if e.validator.Validate(e.words) {
				resultChan <- e.words
			}
			return
		}

		// 计算总组合数
		totalCombinations := 1
		for i := 0; i < unknownCount; i++ {
			totalCombinations *= WordCount
		}

		// 枚举所有组合
		indices := make([]int, unknownCount)
		for count := 0; count < totalCombinations; count++ {
			// 生成当前组合的词
			replacements := make([]string, unknownCount)
			temp := count
			for i := 0; i < unknownCount; i++ {
				indices[i] = temp % WordCount
				replacements[i] = WordList[indices[i]]
				temp /= WordCount
			}

			// 替换未知词
			candidate := ReplaceWords(e.words, e.indices, replacements)

			// 验证助记词
			if e.validator.Validate(candidate) {
				e.workerChan <- struct{}{}
				e.wg.Add(1)
				go func(c []string) {
					defer func() {
						<-e.workerChan
						e.wg.Done()
					}()
					resultChan <- c
				}(candidate)
			}
		}

		e.wg.Wait()
	}()

	return resultChan
}

// BatchEnumerate 批次枚举，返回批次通道
func (e *Enumerator) BatchEnumerate() <-chan [][]string {
	batchChan := make(chan [][]string, 10)

	go func() {
		defer close(batchChan)

		resultChan := e.Enumerate()
		batch := make([][]string, 0, e.batchSize)

		for mnemonic := range resultChan {
			batch = append(batch, mnemonic)
			if len(batch) >= e.batchSize {
				batchChan <- batch
				batch = make([][]string, 0, e.batchSize)
			}
		}

		// 发送剩余的批次
		if len(batch) > 0 {
			batchChan <- batch
		}
	}()

	return batchChan
}
