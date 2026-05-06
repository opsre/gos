package secure

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
	"strings"
	"sync"
)

const encryptedPrefix = "enc:v1:"

var (
	defaultPassphrase = "gos-local-dev-encryption-key"
	keyMu             sync.RWMutex
	activeKey         = deriveKey(defaultPassphrase)
)

// SetSecretKey 封装当前模块的业务处理逻辑。
func SetSecretKey(passphrase string) {
	keyMu.Lock()
	defer keyMu.Unlock()
	value := strings.TrimSpace(passphrase)
	if value == "" {
		value = defaultPassphrase
	}
	activeKey = deriveKey(value)
}

// EncryptString 封装当前模块的业务处理逻辑。
func EncryptString(value string) (string, error) {
	plain := strings.TrimSpace(value)
	if plain == "" {
		return "", nil
	}
	if strings.HasPrefix(plain, encryptedPrefix) {
		return plain, nil
	}
	block, err := aes.NewCipher(currentKey())
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	ciphertext := gcm.Seal(nil, nonce, []byte(plain), nil)
	payload := append(nonce, ciphertext...)
	return encryptedPrefix + base64.StdEncoding.EncodeToString(payload), nil
}

// DecryptString 封装当前模块的业务处理逻辑。
func DecryptString(value string) (string, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "", nil
	}
	if !strings.HasPrefix(trimmed, encryptedPrefix) {
		return trimmed, nil
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(trimmed, encryptedPrefix))
	if err != nil {
		return "", fmt.Errorf("decode encrypted value: %w", err)
	}
	block, err := aes.NewCipher(currentKey())
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	if len(raw) < gcm.NonceSize() {
		return "", fmt.Errorf("invalid encrypted payload")
	}
	nonce := raw[:gcm.NonceSize()]
	ciphertext := raw[gcm.NonceSize():]
	plain, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", fmt.Errorf("decrypt encrypted value: %w", err)
	}
	return string(plain), nil
}

// MaskString 封装当前模块的业务处理逻辑。
func MaskString(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}
	if len(trimmed) <= 8 {
		return strings.Repeat("*", len(trimmed))
	}
	return trimmed[:2] + strings.Repeat("*", len(trimmed)-4) + trimmed[len(trimmed)-2:]
}

// deriveKey 封装当前模块的业务处理逻辑。
func deriveKey(passphrase string) []byte {
	sum := sha256.Sum256([]byte(passphrase))
	key := make([]byte, len(sum))
	copy(key, sum[:])
	return key
}

// currentKey 封装当前模块的业务处理逻辑。
func currentKey() []byte {
	keyMu.RLock()
	defer keyMu.RUnlock()
	key := make([]byte, len(activeKey))
	copy(key, activeKey)
	return key
}
