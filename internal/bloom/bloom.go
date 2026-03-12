package bloom

import (
	"bufio"
	"encoding/hex"
	"os"

	"github.com/bits-and-blooms/bloom/v3"
)

// Filter Bloom过滤器包装
type Filter struct {
	filter *bloom.BloomFilter
}

// NewFilter 创建Bloom过滤器
func NewFilter(expectedItems uint, falsePositiveRate float64) *Filter {
	return &Filter{
		filter: bloom.NewWithEstimates(expectedItems, falsePositiveRate),
	}
}

// LoadFromFile 从文件加载Bloom过滤器
// 文件格式：每行一个hex编码的20字节地址
func LoadFromFile(filename string) (*Filter, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	// 先统计行数
	lineCount := 0
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		if len(scanner.Bytes()) > 0 {
			lineCount++
		}
	}

	// 创建Bloom过滤器
	filter := NewFilter(uint(lineCount*2), 0.001)

	// 重新读取文件添加数据
	file.Seek(0, 0)
	scanner = bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if len(line) == 0 {
			continue
		}
		data, err := hex.DecodeString(line)
		if err != nil {
			continue
		}
		if len(data) == 20 {
			filter.Add(data)
		}
	}

	return filter, nil
}

// Add 添加数据到过滤器
func (f *Filter) Add(data []byte) {
	f.filter.Add(data)
}

// Contains 检查数据是否可能在过滤器中
func (f *Filter) Contains(data []byte) bool {
	return f.filter.Test(data)
}

// TestAndAdd 测试并添加
func (f *Filter) TestAndAdd(data []byte) bool {
	return f.filter.TestAndAdd(data)
}
