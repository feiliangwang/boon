package mnemonic

import (
	"strings"

	"github.com/tyler-smith/go-bip39"
)

// WordList BIP39 英文助记词列表
var WordList = bip39.GetWordList()

// WordCount 助记词总数
var WordCount = len(WordList)

// Validator 助记词验证器
type Validator struct{}

// NewValidator 创建验证器
func NewValidator() *Validator {
	return &Validator{}
}

// Validate 验证助记词是否合法
func (v *Validator) Validate(words []string) bool {
	mnemonic := strings.Join(words, " ")
	return bip39.IsMnemonicValid(mnemonic)
}

// ValidateMnemonic 验证助记词字符串是否合法
func (v *Validator) ValidateMnemonic(mnemonic string) bool {
	return bip39.IsMnemonicValid(mnemonic)
}

// GetUnknownIndices 获取未知词的位置索引
func GetUnknownIndices(words []string) []int {
	indices := make([]int, 0)
	for i, word := range words {
		if word == "?" {
			indices = append(indices, i)
		}
	}
	return indices
}

// ReplaceWords 替换指定位置的词
func ReplaceWords(words []string, indices []int, replacements []string) []string {
	result := make([]string, len(words))
	copy(result, words)
	for i, idx := range indices {
		if i < len(replacements) {
			result[idx] = replacements[i]
		}
	}
	return result
}
