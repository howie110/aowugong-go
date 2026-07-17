package auth

import (
	"fmt"

	"golang.org/x/crypto/bcrypt"
)

// HashPassword 使用 bcrypt 生成新密码哈希。
func HashPassword(password string) (string, error) {
	// 1. 拒绝空密码，避免生成不可用的账户凭据。
	if password == "" {
		return "", ErrInvalidInput
	}

	// 2. 使用 bcrypt 默认成本生成兼容生产数据的哈希。
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", fmt.Errorf("生成 bcrypt 密码哈希: %w", err)
	}
	return string(hash), nil
}

// VerifyPassword 验证原始密码与 bcrypt 哈希是否匹配。
func VerifyPassword(password, passwordHash string) (bool, error) {
	// 1. 使用 bcrypt 比对已有生产密码哈希。
	err := bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(password))
	if err == nil {
		return true, nil
	}
	if err == bcrypt.ErrMismatchedHashAndPassword {
		return false, nil
	}
	return false, fmt.Errorf("校验 bcrypt 密码哈希: %w", err)
}
