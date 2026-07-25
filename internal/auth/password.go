package auth

import (
	"fmt"

	"golang.org/x/crypto/bcrypt"
)

// HashPassword 使用 bcrypt 生成新密码哈希。
// 输入：password 是非空原始密码。
// 输出：返回 bcrypt 哈希；输入无效或生成失败时返回错误。
// 副作用：读取系统安全随机源。
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
// 输入：password 是待验证密码，passwordHash 是已有 bcrypt 哈希。
// 输出：匹配返回 true，不匹配返回 false，哈希无效时返回错误。
// 副作用：无。
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
