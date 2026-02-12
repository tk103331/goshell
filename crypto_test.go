package main

import (
	"testing"
)

// TestEncryptDecrypt 测试加密和解密功能
func TestEncryptDecrypt(t *testing.T) {
	testCases := []string{
		"simple_password",
		"complex!@#$%^&*()_+-=",
		"unicode_测试密码_😀",
		"very_long_password_that_has_many_characters_to_test_encryption_algorithm_1234567890",
		"", // 空字符串
	}

	for _, original := range testCases {
		t.Run(original, func(t *testing.T) {
			// 加密
			encrypted, err := encryptString(original)
			if err != nil {
				t.Fatalf("encryptString failed: %v", err)
			}

			// 验证加密后的字符串不为空（除非原字符串为空）
			if original != "" && encrypted == "" {
				t.Error("Encrypted string should not be empty for non-empty input")
			}

			// 加密后的字符串应该与原文不同
			if original != "" && encrypted == original {
				t.Error("Encrypted string should differ from original")
			}

			// 解密
			decrypted, err := decryptString(encrypted)
			if err != nil {
				t.Fatalf("decryptString failed: %v", err)
			}

			// 验证解密结果与原文相同
			if decrypted != original {
				t.Errorf("Decrypted string does not match original. got=%q, want=%q", decrypted, original)
			}
		})
	}
}

// TestDecryptInvalidCiphertext 测试解密无效密文
func TestDecryptInvalidCiphertext(t *testing.T) {
	invalidInputs := []string{
		"not_base64!",
		"invalid",
		"YWJj", // base64 但不是有效的加密数据
	}

	for _, invalid := range invalidInputs {
		t.Run(invalid, func(t *testing.T) {
			_, err := decryptString(invalid)
			if err == nil {
				t.Error("decryptString should fail with invalid ciphertext")
			}
		})
	}
}

// TestDecryptEmptyString 测试解密空字符串（应该成功返回空）
func TestDecryptEmptyString(t *testing.T) {
	decrypted, err := decryptString("")
	if err != nil {
		t.Fatalf("decryptString of empty string should succeed: %v", err)
	}
	if decrypted != "" {
		t.Errorf("decryptString of empty string should return empty, got: %q", decrypted)
	}
}

// TestSSHConfigDataPasswordEncryption 测试 SSH 配置数据密码加密
func TestSSHConfigDataPasswordEncryption(t *testing.T) {
	var err error
	config := &SSHConfigData{
		Name: "test_server",
		Host: "example.com",
		Port: 22,
		User: "testuser",
	}

	testPassword := "my_secure_password_123!"

	// 设置加密密码
	_, err = config.setPassword(testPassword)
	if err != nil {
		t.Fatalf("setPassword failed: %v", err)
	}

	// 验证存储的密码是加密后的（与原文不同）
	if config.Pswd == testPassword {
		t.Error("Stored password should be encrypted and differ from original")
	}

	// 获取解密后的密码
	decrypted, err := config.getPassword()
	if err != nil {
		t.Fatalf("getPassword failed: %v", err)
	}

	// 验证解密后的密码与原文相同
	if decrypted != testPassword {
		t.Errorf("Decrypted password does not match original. got=%q, want=%q", decrypted, testPassword)
	}
}

// TestK8SConfigDataTokenEncryption 测试 K8S 配置数据 token 加密
func TestK8SConfigDataTokenEncryption(t *testing.T) {
	var err error
	config := &K8SConfigData{
		Name:   "test_cluster",
		Server: "https://k8s.example.com",
	}

	testToken := "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIiwibmFtZSI6IkpvaG4gRG9lIiwiaWF0IjoxNTE2MjM5MDIyfQ"

	// 设置加密 token
	_, err = config.setToken(testToken)
	if err != nil {
		t.Fatalf("setToken failed: %v", err)
	}

	// 验证存储的 token 是加密后的（与原文不同）
	if config.Token == testToken {
		t.Error("Stored token should be encrypted and differ from original")
	}

	// 获取解密后的 token
	decrypted, err := config.getToken()
	if err != nil {
		t.Fatalf("getToken failed: %v", err)
	}

	// 验证解密后的 token 与原文相同
	if decrypted != testToken {
		t.Errorf("Decrypted token does not match original. got=%q, want=%q", decrypted, testToken)
	}
}

// TestEmptyPasswordHandling 测试空密码处理
func TestEmptyPasswordHandling(t *testing.T) {
	var err error
	config := &SSHConfigData{}

	// 设置空密码
	_, err = config.setPassword("")
	if err != nil {
		t.Fatalf("setPassword with empty string failed: %v", err)
	}

	// 验证空密码被正确处理
	if config.Pswd != "" {
		t.Error("Empty password should be stored as empty string")
	}

	// 获取解密后的密码
	decrypted, err := config.getPassword()
	if err != nil {
		t.Fatalf("getPassword failed: %v", err)
	}

	// 验证解密结果是空字符串
	if decrypted != "" {
		t.Errorf("Decrypted password should be empty. got=%q", decrypted)
	}
}
