//go:build !cuda
// +build !cuda

package compute

import "errors"

// GPUComputer GPU计算器（非CUDA构建时的占位）
type GPUComputer struct {
	*CPUComputer // 嵌入CPU计算器作为回退
}

// NewGPUComputer 创建GPU计算器（无CUDA时回退到CPU）
func NewGPUComputer(batchSize, workers int) (*GPUComputer, error) {
	return nil, errors.New("GPU not available: build with 'cuda' tag")
}
