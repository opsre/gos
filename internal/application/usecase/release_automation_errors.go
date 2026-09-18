package usecase

import (
	"errors"
	"strings"
)

// ErrAutomationGitUnreachable 表示保存发布自动化前的 git 校验没通过：
// 分支不可读、没有匹配凭证、读取超时等。HTTP 层据此返回 400 并展示原因，
// 且配置不会落库。
var ErrAutomationGitUnreachable = errors.New("release automation git check failed")

// releaseAutomationGitError 只携带可读原因：页面要直接展示「Git 凭证被拒绝」
// 「分支不存在」这类文案，包一层英文前缀反而会污染提示。
type releaseAutomationGitError struct {
	reason string
}

func newReleaseAutomationGitError(reason string) error {
	trimmed := strings.TrimSpace(reason)
	if trimmed == "" {
		trimmed = "读取分支 HEAD 失败"
	}
	return &releaseAutomationGitError{reason: trimmed}
}

func (e *releaseAutomationGitError) Error() string {
	return e.reason
}

// Unwrap 让调用方可以用 errors.Is(err, ErrAutomationGitUnreachable) 判定错误类别，
// 同时 Error() 仍然只返回可读原因。
func (e *releaseAutomationGitError) Unwrap() error {
	return ErrAutomationGitUnreachable
}

// readableGitCheckReason 把 git 读取错误压成一行可读原因。GitCommitManager 返回的
// 已经是中文可读文案，这里只做兜底，避免空消息变成空提示。
func readableGitCheckReason(err error) string {
	if err == nil {
		return ""
	}
	reason := strings.TrimSpace(err.Error())
	if reason == "" {
		return "读取分支 HEAD 失败"
	}
	return reason
}
