package crypto

import (
	"golang.org/x/crypto/sha3"
)

// Keccak256 计算Keccak256哈希
func Keccak256(data []byte) []byte {
	h := sha3.NewLegacyKeccak256()
	h.Write(data)
	return h.Sum(nil)
}

// Keccak256Hash 计算Keccak256并返回前20字节
func Keccak256Hash(data []byte) []byte {
	hash := Keccak256(data)
	return hash[:20]
}
