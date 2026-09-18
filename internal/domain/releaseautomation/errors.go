package releaseautomation

import "errors"

var (
	ErrNotFound = errors.New("release automation not found")
	// ErrDuplicated 命中唯一键 uk_release_automation_app_env_ref：
	// 同应用 + 同环境 + 同分支只允许存在一条配置。
	ErrDuplicated = errors.New("release automation already exists for the same application, environment and branch")
)
