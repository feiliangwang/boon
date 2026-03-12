# Boon Makefile

# Go 参数
GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
GOMOD=$(GOCMD) mod

# CUDA 参数
NVCC=nvcc
CUDA_ARCH=sm_75  # 根据你的GPU调整，如 sm_70, sm_75, sm_80 等
CUDA_INCLUDE=/usr/local/cuda/include
CUDA_LIB=/usr/local/cuda/lib64

# 项目结构
PROJECT_NAME=boon
CMD_DIR=cmd/boon
BUILD_DIR=build
COMPUTE_DIR=internal/compute

# 构建标志
LDFLAGS=-ldflags "-s -w"

.PHONY: all build clean test deps gpu cpu help

all: deps build

# 下载依赖
deps:
	$(GOMOD) download
	$(GOMOD) tidy

# CPU版本构建（默认）
build:
	$(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(PROJECT_NAME) ./$(CMD_DIR)

# GPU版本构建（需要CUDA环境）
gpu: deps
	@echo "构建GPU版本..."
	@mkdir -p $(BUILD_DIR)
	# 先编译CUDA代码
	$(NVCC) -c -o $(BUILD_DIR)/compute.o $(COMPUTE_DIR)/gpu.cu -arch=$(CUDA_ARCH) -I$(CUDA_INCLUDE)
	# 然后编译Go代码
	CGO_LDFLAGS="-L$(CUDA_LIB) -lcuda -lcudart" \
	$(GOBUILD) $(LDFLAGS) -tags cuda -o $(BUILD_DIR)/$(PROJECT_NAME)-gpu ./$(CMD_DIR)

# CPU版本
cpu: deps
	@echo "构建CPU版本..."
	$(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(PROJECT_NAME)-cpu ./$(CMD_DIR)

# 运行测试
test:
	$(GOTEST) -v ./...

# 清理
clean:
	$(GOCLEAN)
	rm -rf $(BUILD_DIR)

# 帮助
help:
	@echo "Boon - TRON助记词枚举工具"
	@echo ""
	@echo "使用方法:"
	@echo "  make build    - 构建CPU版本"
	@echo "  make gpu      - 构建GPU版本（需要CUDA）"
	@echo "  make cpu      - 显式构建CPU版本"
	@echo "  make test     - 运行测试"
	@echo "  make clean    - 清理构建文件"
	@echo ""
	@echo "运行示例:"
	@echo "  ./build/boon -mnemonic 'word1 word2 ? word4 ...' -bloom addresses.txt"
