package gbox

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"os"
)

// RsaAesClient 结构体
type RsaAesClient struct {
	prefix        string
	rsaPublicKey  *rsa.PublicKey
	rsaPrivateKey *rsa.PrivateKey // 用于解密
}

// NewRsaAesClient 创建客户端（仅加密）
func NewRsaAesClient(prefix string) *RsaAesClient {
	return &RsaAesClient{
		prefix: prefix,
	}
}

// NewRsaAesClientWithPrivateKey 创建支持解密的客户端
func NewRsaAesClientWithPrivateKey(prefix string, privateKeyPEM string) (*RsaAesClient, error) {
	client := &RsaAesClient{
		prefix: prefix,
	}

	privateKey, err := parseRSAPrivateKey(privateKeyPEM)
	if err != nil {
		return nil, fmt.Errorf("parse private key failed: %v", err)
	}
	client.rsaPrivateKey = privateKey

	client.rsaPublicKey = &privateKey.PublicKey

	return client, nil
}

// SetRSAPublicKey 设置RSA公钥（用于加密）
func (c *RsaAesClient) SetRSAPublicKey(publicKeyPEM string) error {
	publicKey, err := parseRSAPublicKey(publicKeyPEM)
	if err != nil {
		return err
	}
	c.rsaPublicKey = publicKey
	return nil
}

func (c *RsaAesClient) Encrypt(data any) (map[string]string, error) {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return nil, fmt.Errorf("JSON marshal failed: %v", err)
	}

	aesKey := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, aesKey); err != nil {
		return nil, fmt.Errorf("generate AES key failed: %v", err)
	}

	encryptedData, err := aesGCMEncrypt(jsonData, aesKey)
	if err != nil {
		return nil, fmt.Errorf("AES encrypt failed: %v", err)
	}

	if c.rsaPublicKey == nil {
		return nil, errors.New("RSA public key not set")
	}
	encryptedKey, err := rsaEncrypt(aesKey, c.rsaPublicKey)
	if err != nil {
		return nil, fmt.Errorf("RSA encrypt failed: %v", err)
	}

	result := map[string]string{
		"encrypted_key":  base64.StdEncoding.EncodeToString(encryptedKey),
		"encrypted_data": base64.StdEncoding.EncodeToString(encryptedData),
	}

	return result, nil
}

func (c *RsaAesClient) Decrypt(key string, data string) (map[string]any, error) {
	if c.rsaPrivateKey == nil {
		return nil, errors.New("RSA private key not set, cannot decrypt")
	}

	encryptedKey, err := base64.StdEncoding.DecodeString(key)
	if err != nil {
		return nil, fmt.Errorf("decode encrypted key failed: %v", err)
	}

	encryptedData, err := base64.StdEncoding.DecodeString(data)
	if err != nil {
		return nil, fmt.Errorf("decode encrypted data failed: %v", err)
	}

	aesKey, err := rsaDecrypt(encryptedKey, c.rsaPrivateKey)
	if err != nil {
		return nil, fmt.Errorf("RSA decrypt failed: %v", err)
	}

	plaintext, err := aesGCMDecrypt(encryptedData, aesKey)
	if err != nil {
		return nil, fmt.Errorf("AES decrypt failed: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(plaintext, &result); err != nil {
		return nil, fmt.Errorf("parse decrypted JSON failed: %v", err)
	}

	return result, nil
}

// aesGCMEncrypt AES-GCM加密
func aesGCMEncrypt(plaintext, key []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}

	ciphertext := gcm.Seal(nonce, nonce, plaintext, nil)
	return ciphertext, nil
}

// aesGCMDecrypt AES-GCM解密
func aesGCMDecrypt(ciphertext, key []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonceSize := gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return nil, errors.New("ciphertext too short")
	}

	nonce, ciphertext := ciphertext[:nonceSize], ciphertext[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, err
	}

	return plaintext, nil
}

// rsaEncrypt RSA加密（OAEP）
func rsaEncrypt(data []byte, publicKey *rsa.PublicKey) ([]byte, error) {
	hash := sha256.New()
	encryptedData, err := rsa.EncryptOAEP(hash, rand.Reader, publicKey, data, nil)
	if err != nil {
		return nil, err
	}
	return encryptedData, nil
}

// rsaDecrypt RSA解密（OAEP）
func rsaDecrypt(ciphertext []byte, privateKey *rsa.PrivateKey) ([]byte, error) {
	hash := sha256.New()
	plaintext, err := rsa.DecryptOAEP(hash, rand.Reader, privateKey, ciphertext, nil)
	if err != nil {
		return nil, err
	}
	return plaintext, nil
}

// parseRSAPublicKey 解析RSA公钥
func parseRSAPublicKey(publicKeyPEM string) (*rsa.PublicKey, error) {
	block, _ := pem.Decode([]byte(publicKeyPEM))
	if block == nil {
		return nil, errors.New("failed to parse PEM block containing the public key")
	}

	pub, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse public key: %v", err)
	}

	rsaPub, ok := pub.(*rsa.PublicKey)
	if !ok {
		return nil, errors.New("not an RSA public key")
	}

	return rsaPub, nil
}

// parseRSAPrivateKey 解析RSA私钥
func parseRSAPrivateKey(privateKeyPEM string) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode([]byte(privateKeyPEM))
	if block == nil {
		return nil, errors.New("failed to parse PEM block containing the private key")
	}

	privateKey, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err == nil {
		return privateKey, nil
	}

	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse private key: %v", err)
	}

	rsaKey, ok := key.(*rsa.PrivateKey)
	if !ok {
		return nil, errors.New("not an RSA private key")
	}

	return rsaKey, nil
}

// getRSAPublicKey 获取RSA公钥（向后兼容）
func (c *RsaAesClient) getRSAPublicKey() *rsa.PublicKey {
	return c.rsaPublicKey
}

// GenerateRSAKeyPair 生成RSA密钥对（辅助函数）
func GenerateRSAKeyPair(bits int) (string, string, error) {
	privateKey, err := rsa.GenerateKey(rand.Reader, bits)
	if err != nil {
		return "", "", err
	}

	publicKey := &privateKey.PublicKey
	publicKeyBytes, err := x509.MarshalPKIXPublicKey(publicKey)
	if err != nil {
		return "", "", err
	}
	publicKeyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: publicKeyBytes,
	})

	privateKeyBytes := x509.MarshalPKCS1PrivateKey(privateKey)
	privateKeyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: privateKeyBytes,
	})

	return string(publicKeyPEM), string(privateKeyPEM), nil
}

func LoadKeyFromFile(filename string) (string, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return "", err
	}
	return string(data), nil
}
