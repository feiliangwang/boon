# Boon

TRON 助记词枚举与地址恢复工具。

## 功能

- 🔍 **助记词枚举** - 支持 `?` 占位符，自动枚举所有可能组合
- ✅ **BIP39验证** - 自动验证助记词合法性，过滤无效组合
- ⚡ **GPU加速** - 支持 CUDA 加速 PBKDF2 计算（可选）
- 🔐 **BIP44派生** - 标准 TRON 路径 `m/44'/195'/0'/0/0`
- 🌸 **Bloom过滤** - 高效地址匹配，从文件加载目标地址集

## 编译

### CPU 版本（默认）

```bash
make build
# 或
make cpu
```

### GPU 版本（需要 CUDA）

```bash
make gpu
```

**要求：**
- CUDA Toolkit 11.x+
- 兼容的 NVIDIA GPU

## 使用

```bash
./build/boon -mnemonic "word1 ? ? word4 word5 word6 word7 word8 word9 word10 word11 word12" -bloom addresses.txt
```

### 参数说明

| 参数 | 说明 | 默认值 |
|------|------|--------|
| `-mnemonic` | 助记词模板，未知词用 `?` 代替 | 必填 |
| `-bloom` | Bloom 过滤器文件路径（hex 编码地址） | 可选 |
| `-batch` | 批次大小 | 1000 |
| `-workers` | CPU 工作线程数 | CPU 核心数 |
| `-gpu` | 使用 GPU 加速 | true |
| `-v` | 详细输出 | false |

### Bloom 过滤器文件格式

每行一个地址（hex 编码，20 字节）：

```
a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2
b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3
...
```

## 示例

### 枚举 1 个未知词

```bash
./build/boon -mnemonic "abandon ability ? above abuse accident accuse account achieve acid acquire across" -bloom targets.txt -v
```

### 枚举 2 个未知词

```bash
./build/boon -mnemonic "? ability ? above abuse accident accuse account achieve acid acquire across" -bloom targets.txt
```

## 输出

匹配结果会：
1. 打印到控制台
2. 追加到 `matches.txt` 文件

格式：
```
助记词,地址
word1 word2 word3 ... word12,a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2
```

## 技术细节

### 计算流程

1. **枚举** - 根据 `?` 位置枚举所有 BIP39 词表组合
2. **验证** - 使用 BIP39 校验和验证助记词合法性
3. **批次** - 收集合还助记词到批次
4. **PBKDF2** - 计算 `PBKDF2-HMAC-SHA512(mnemonic, "mnemonic", 2048, 64)` 得到种子
5. **BIP44** - 从种子派生路径 `m/44'/195'/0'/0/0`
6. **Keccak256** - 对公钥计算 Keccak256，取前 20 字节
7. **Bloom** - 检查地址是否在目标集合中

### 性能估算

| 未知词数 | 组合数 | 预计时间（GPU） |
|----------|--------|-----------------|
| 1 | 2,048 | < 1秒 |
| 2 | 4,194,304 | ~数秒 |
| 3 | 8,589,934,592 | ~数分钟 |
| 4+ | 极大 | 不建议 |

## 许可证

MIT
