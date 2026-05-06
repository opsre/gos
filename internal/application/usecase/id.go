package usecase

import (
	"crypto/rand"
	"fmt"
	"time"
)

// generateID 封装当前模块的业务处理逻辑。
func generateID(prefix string) string {
	b := make([]byte, 12)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("%s-%d", prefix, time.Now().UTC().UnixNano())
	}
	return fmt.Sprintf("%s-%x", prefix, b)
}
