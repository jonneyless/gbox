package gbox

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"

	"golang.org/x/crypto/bcrypt"
)

// Bcrypt 加密成本（通常为 10-14，越高越安全但越慢）
const bcryptCost = 10

// HashPassword 使用 bcrypt 对密码进行哈希加密
func HashPassword(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), bcryptCost)
	return string(bytes), err
}

// CheckPasswordHash 验证密码是否与哈希匹配
func CheckPasswordHash(password, hash string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}

func EncryptTo64Hex(data string) string {
	hash := sha256.Sum256([]byte(data))
	return hex.EncodeToString(hash[:])
}

func GenerateHMACSHA256(message string, key string) string {
	h := hmac.New(sha256.New, []byte(key))
	h.Write([]byte(message))
	signature := h.Sum(nil)
	return hex.EncodeToString(signature)
}
