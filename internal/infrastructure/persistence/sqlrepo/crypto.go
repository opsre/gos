package sqlrepo

import "gos/internal/support/secure"

// encryptStoredSecret 封装当前模块的业务处理逻辑。
func encryptStoredSecret(value string) (string, error) {
	return secure.EncryptString(value)
}

// decryptStoredSecret 封装当前模块的业务处理逻辑。
func decryptStoredSecret(value string) (string, error) {
	return secure.DecryptString(value)
}
