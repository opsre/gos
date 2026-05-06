package configstore

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	gitopsinfra "gos/internal/infrastructure/gitops"
)

type GitOpsStore struct {
	configPath string
}

// NewGitOpsStore 创建并返回对应组件实例。
func NewGitOpsStore(configPath string) *GitOpsStore {
	return &GitOpsStore{configPath: strings.TrimSpace(configPath)}
}

// SaveCommitMessageTemplate 将 GitOps 提交信息模版写回当前运行配置文件。
//
// 这里采用“只改一处键值”的策略：
// 1. 不要求调用方重写整份配置；
// 2. 未配置 gitops 节点时会自动补齐；
// 3. 传入空值时自动回退为默认模版，避免把系统写成不可用状态。
func (s *GitOpsStore) SaveCommitMessageTemplate(_ context.Context, template string) error {
	path := strings.TrimSpace(s.configPath)
	if path == "" {
		return fmt.Errorf("config path is required")
	}

	content, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read config file failed: %w", err)
	}

	var payload map[string]interface{}
	if err := json.Unmarshal(content, &payload); err != nil {
		return fmt.Errorf("decode config file failed: %w", err)
	}

	gitopsValue, ok := payload["gitops"]
	var gitopsNode map[string]interface{}
	if ok {
		if mapped, castOK := gitopsValue.(map[string]interface{}); castOK {
			gitopsNode = mapped
		}
	}
	if gitopsNode == nil {
		gitopsNode = make(map[string]interface{})
	}
	gitopsNode["commit_message_template"] = gitopsinfra.NormalizeCommitMessageTemplate(template)
	payload["gitops"] = gitopsNode

	updated, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return fmt.Errorf("encode config file failed: %w", err)
	}
	updated = append(updated, '\n')

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("prepare config directory failed: %w", err)
	}
	if err := os.WriteFile(path, updated, 0o644); err != nil {
		return fmt.Errorf("write config file failed: %w", err)
	}
	return nil
}
