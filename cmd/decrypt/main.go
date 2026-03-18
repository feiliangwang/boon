// decrypt – 使用 ~/.boon/private.pem 解密从调度器 Web UI 复制的加密助记词
//
// 用法：
//
//	decrypt <base64密文>
//	echo '<base64密文>' | decrypt
//
// 私钥路径默认 ~/.boon/private.pem，可用 -key 覆盖。
package main

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
)

func main() {
	keyPath := flag.String("key", "", "私钥路径（默认 ~/.boon/private.pem）")
	flag.Parse()

	if *keyPath == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			fatal("获取 HOME 目录失败: %v", err)
		}
		*keyPath = home + "/.boon/private.pem"
	}

	// 读取私钥
	priv, err := loadPrivateKey(*keyPath)
	if err != nil {
		fatal("加载私钥失败: %v", err)
	}

	// 读取密文：优先命令行参数，否则读 stdin
	var cipherB64 string
	if flag.NArg() > 0 {
		cipherB64 = strings.TrimSpace(flag.Arg(0))
	} else {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			fatal("读取 stdin 失败: %v", err)
		}
		cipherB64 = strings.TrimSpace(string(data))
	}
	if cipherB64 == "" {
		fatal("用法: decrypt <base64密文>  或  echo '<密文>' | decrypt")
	}

	// base64 解码
	cipherBytes, err := base64.StdEncoding.DecodeString(cipherB64)
	if err != nil {
		fatal("base64 解码失败（请确认复制完整）: %v", err)
	}

	// RSA-OAEP-SHA256 解密
	plaintext, err := rsa.DecryptOAEP(sha256.New(), rand.Reader, priv, cipherBytes, nil)
	if err != nil {
		fatal("解密失败（私钥是否匹配？）: %v", err)
	}

	fmt.Println(string(plaintext))
}

func loadPrivateKey(path string) (*rsa.PrivateKey, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取文件失败: %w", err)
	}
	block, _ := pem.Decode(data)
	if block == nil {
		return nil, fmt.Errorf("无法解析 PEM 块")
	}
	// 支持 PKCS#1 和 PKCS#8 两种格式
	if block.Type == "RSA PRIVATE KEY" {
		return x509.ParsePKCS1PrivateKey(block.Bytes)
	}
	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("解析私钥失败: %w", err)
	}
	rsaKey, ok := key.(*rsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("不是 RSA 私钥")
	}
	return rsaKey, nil
}

func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "错误: "+format+"\n", args...)
	os.Exit(1)
}
