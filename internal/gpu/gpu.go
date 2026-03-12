//go:build cuda
// +build cuda

package gpu

import (
	"fmt"
	"unsafe"
)

/*
#cgo CFLAGS: -I/usr/local/cuda/include
#cgo LDFLAGS: -L/usr/local/cuda/lib64 -lcuda -lcudart
#include <cuda_runtime.h>
#include <stdlib.h>

extern void pbkdf2_cuda_batch(const char** mnemonics, int count, unsigned char* seeds_out);
*/
import "C"

// GPUClient GPU客户端
type GPUClient struct {
	initialized bool
	batchSize   int
}

// NewGPUClient 创建GPU客户端
func NewGPUClient(batchSize int) (*GPUClient, error) {
	var deviceCount C.int
	err := C.cudaGetDeviceCount(&deviceCount)
	if err != C.cudaSuccess {
		return nil, fmt.Errorf("no CUDA device available")
	}

	return &GPUClient{
		initialized: true,
		batchSize:   batchSize,
	}, nil
}

// IsAvailable 检查GPU是否可用
func IsAvailable() bool {
	var deviceCount C.int
	err := C.cudaGetDeviceCount(&deviceCount)
	return err == C.cudaSuccess && deviceCount > 0
}

// ComputeSeedBatch 批量计算种子（GPU版本）
func (g *GPUClient) ComputeSeedBatch(mnemonics [][]string) ([][]byte, error) {
	return nil, fmt.Errorf("GPU mode requires CUDA build tag")
}

// Close 关闭GPU客户端
func (g *GPUClient) Close() error {
	C.cudaDeviceReset()
	g.initialized = false
	return nil
}
