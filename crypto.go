package main

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
)

var encryptionKey []byte

// initEncryptionKey 初始化加密密钥
// 从用户目录的配置文件中读取或创建密钥
func initEncryptionKey() error {
	if encryptionKey != nil {
		return nil
	}

	// 获取密钥文件路径
	keyPath := getEncryptionKeyPath()

	// 尝试读取现有密钥
	if keyBytes, err := os.ReadFile(keyPath); err == nil {
		// 使用 SHA256 确保密钥长度为 32 字节
		hash := sha256.Sum256(keyBytes)
		encryptionKey = hash[:]
		return nil
	}

	// 创建新密钥
	keyBytes := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, keyBytes); err != nil {
		return fmt.Errorf("failed to generate encryption key: %w", err)
	}

	// 保存密钥
	if err := os.MkdirAll(filepath.Dir(keyPath), 0700); err != nil {
		return fmt.Errorf("failed to create key directory: %w", err)
	}

	if err := os.WriteFile(keyPath, keyBytes, 0600); err != nil {
		return fmt.Errorf("failed to save encryption key: %w", err)
	}

	encryptionKey = keyBytes
	return nil
}

// getEncryptionKeyPath 获取密钥文件路径
func getEncryptionKeyPath() string {
	var basePath string

	switch runtime.GOOS {
	case "darwin":
		basePath = filepath.Join(os.Getenv("HOME"), "Library", "Application Support")
	case "windows":
		basePath = os.Getenv("APPDATA")
	default: // linux, freebsd, etc.
		if xdgConfig := os.Getenv("XDG_CONFIG_HOME"); xdgConfig != "" {
			basePath = xdgConfig
		} else {
			basePath = filepath.Join(os.Getenv("HOME"), ".config")
		}
	}

	return filepath.Join(basePath, "goshell", ".encryption_key")
}

// encryptString 加密字符串
func encryptString(plaintext string) (string, error) {
	if encryptionKey == nil {
		if err := initEncryptionKey(); err != nil {
			return "", err
		}
	}

	if plaintext == "" {
		return "", nil
	}

	block, err := aes.NewCipher(encryptionKey)
	if err != nil {
		return "", fmt.Errorf("failed to create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("failed to create GCM: %w", err)
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err = io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("failed to generate nonce: %w", err)
	}

	ciphertext := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// decryptString 解密字符串
func decryptString(ciphertext string) (string, error) {
	if ciphertext == "" {
		return "", nil
	}

	if encryptionKey == nil {
		if err := initEncryptionKey(); err != nil {
			return "", err
		}
	}

	data, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil {
		return "", fmt.Errorf("failed to decode base64: %w", err)
	}

	block, err := aes.NewCipher(encryptionKey)
	if err != nil {
		return "", fmt.Errorf("failed to create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("failed to create GCM: %w", err)
	}

	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		return "", errors.New("ciphertext too short")
	}

	nonce, ciphertextBytes := data[:nonceSize], data[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertextBytes, nil)
	if err != nil {
		return "", fmt.Errorf("failed to decrypt: %w", err)
	}

	return string(plaintext), nil
}
