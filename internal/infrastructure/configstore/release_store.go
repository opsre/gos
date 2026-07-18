package configstore

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gos/internal/application/usecase"
)

var defaultReleaseEnvOptions = []string{"dev", "test", "prod"}

var defaultReleaseConcurrencySettings = usecase.ReleaseConcurrencySettingsOutput{
	Enabled:          false,
	LockScope:        usecase.ReleaseConcurrencyLockScopeApplicationEnv,
	ConflictStrategy: usecase.ReleaseConcurrencyConflictStrategyReject,
	LockTimeoutSec:   1800,
}

type ReleaseStore struct {
	configPath string
}

// NewReleaseStore 创建并返回对应组件实例。
func NewReleaseStore(configPath string) *ReleaseStore {
	return &ReleaseStore{configPath: strings.TrimSpace(configPath)}
}

// LoadEnvOptions 封装当前模块的业务处理逻辑。
func (s *ReleaseStore) LoadEnvOptions(ctx context.Context) ([]string, error) {
	configs, err := s.LoadEnvConfigs(ctx)
	if err != nil {
		return nil, err
	}
	if len(configs) > 0 {
		return releaseEnvOptionsFromConfigs(configs), nil
	}
	path := strings.TrimSpace(s.configPath)
	if path == "" {
		return cloneStringList(defaultReleaseEnvOptions), nil
	}

	content, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cloneStringList(defaultReleaseEnvOptions), nil
		}
		return nil, fmt.Errorf("read config file failed: %w", err)
	}

	var payload map[string]interface{}
	if err := json.Unmarshal(content, &payload); err != nil {
		return nil, fmt.Errorf("decode config file failed: %w", err)
	}

	node := readMapNode(payload, "release")
	if _, ok := node["env_options"]; ok {
		return normalizeStringListFromAny(node["env_options"]), nil
	}
	options := normalizeStringListFromAny(node["env_options"])
	if len(options) == 0 {
		return cloneStringList(defaultReleaseEnvOptions), nil
	}
	return options, nil
}

// LoadEnvConfigs 封装当前模块的业务处理逻辑。
func (s *ReleaseStore) LoadEnvConfigs(_ context.Context) ([]usecase.ReleaseEnvironmentConfig, error) {
	path := strings.TrimSpace(s.configPath)
	if path == "" {
		return releaseEnvConfigsFromOptions(defaultReleaseEnvOptions), nil
	}

	content, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return releaseEnvConfigsFromOptions(defaultReleaseEnvOptions), nil
		}
		return nil, fmt.Errorf("read config file failed: %w", err)
	}

	var payload map[string]interface{}
	if err := json.Unmarshal(content, &payload); err != nil {
		return nil, fmt.Errorf("decode config file failed: %w", err)
	}

	node := readMapNode(payload, "release")
	if _, ok := node["env_configs"]; ok {
		return normalizeEnvConfigsFromAny(node["env_configs"]), nil
	}
	if _, ok := node["env_options"]; ok {
		return releaseEnvConfigsFromOptions(normalizeStringListFromAny(node["env_options"])), nil
	}
	return releaseEnvConfigsFromOptions(defaultReleaseEnvOptions), nil
}

// LoadDefaultEnvCode 封装当前模块的业务处理逻辑。
func (s *ReleaseStore) LoadDefaultEnvCode(_ context.Context) (string, error) {
	path := strings.TrimSpace(s.configPath)
	if path == "" {
		return "", nil
	}

	content, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", fmt.Errorf("read config file failed: %w", err)
	}

	var payload map[string]interface{}
	if err := json.Unmarshal(content, &payload); err != nil {
		return "", fmt.Errorf("decode config file failed: %w", err)
	}

	node := readMapNode(payload, "release")
	if value, ok := node["default_env_code"].(string); ok {
		return strings.TrimSpace(value), nil
	}
	return "", nil
}

// LoadConcurrencySettings 封装当前模块的业务处理逻辑。
func (s *ReleaseStore) LoadConcurrencySettings(_ context.Context) (usecase.ReleaseConcurrencySettingsOutput, error) {
	path := strings.TrimSpace(s.configPath)
	if path == "" {
		return defaultReleaseConcurrencySettings, nil
	}

	content, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return defaultReleaseConcurrencySettings, nil
		}
		return usecase.ReleaseConcurrencySettingsOutput{}, fmt.Errorf("read config file failed: %w", err)
	}

	var payload map[string]interface{}
	if err := json.Unmarshal(content, &payload); err != nil {
		return usecase.ReleaseConcurrencySettingsOutput{}, fmt.Errorf("decode config file failed: %w", err)
	}

	releaseNode := readMapNode(payload, "release")
	concurrencyNode := readMapNode(releaseNode, "concurrency")

	settings := defaultReleaseConcurrencySettings
	settings.Enabled = boolFromAny(concurrencyNode["enabled"])
	if value := strings.TrimSpace(fmt.Sprint(concurrencyNode["lock_scope"])); value != "" {
		settings.LockScope = usecase.ReleaseConcurrencyLockScope(value)
	}
	if value := strings.TrimSpace(fmt.Sprint(concurrencyNode["conflict_strategy"])); value != "" {
		settings.ConflictStrategy = usecase.ReleaseConcurrencyConflictStrategy(value)
	}
	if value := intFromAny(concurrencyNode["lock_timeout_sec"]); value > 0 {
		settings.LockTimeoutSec = value
	}
	return usecase.ReleaseConcurrencySettingsOutput{
		Enabled:          settings.Enabled,
		LockScope:        settings.LockScope,
		ConflictStrategy: settings.ConflictStrategy,
		LockTimeoutSec:   settings.LockTimeoutSec,
	}, nil
}

// SaveEnvOptions 封装当前模块的业务处理逻辑。
func (s *ReleaseStore) SaveEnvOptions(_ context.Context, values []string) error {
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

	options := normalizeStringList(values)
	if options == nil {
		options = []string{}
	}
	configs := releaseEnvConfigsFromOptions(options)

	releaseNode := readMapNode(payload, "release")
	releaseNode["env_options"] = options
	releaseNode["env_configs"] = configs
	payload["release"] = releaseNode

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

// SaveEnvConfigs 封装当前模块的业务处理逻辑。
func (s *ReleaseStore) SaveEnvConfigs(_ context.Context, values []usecase.ReleaseEnvironmentConfig) error {
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

	configs := normalizeEnvConfigs(values)
	releaseNode := readMapNode(payload, "release")
	releaseNode["env_configs"] = configs
	releaseNode["env_options"] = releaseEnvOptionsFromConfigs(configs)
	payload["release"] = releaseNode

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

// SaveDefaultEnvCode 封装当前模块的业务处理逻辑。
func (s *ReleaseStore) SaveDefaultEnvCode(_ context.Context, value string) error {
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

	releaseNode := readMapNode(payload, "release")
	releaseNode["default_env_code"] = strings.TrimSpace(value)
	payload["release"] = releaseNode

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

// SaveConcurrencySettings 封装当前模块的业务处理逻辑。
func (s *ReleaseStore) SaveConcurrencySettings(_ context.Context, input usecase.ReleaseConcurrencySettingsInput) error {
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

	releaseNode := readMapNode(payload, "release")
	releaseNode["concurrency"] = map[string]interface{}{
		"enabled":           input.Enabled,
		"lock_scope":        strings.TrimSpace(string(input.LockScope)),
		"conflict_strategy": strings.TrimSpace(string(input.ConflictStrategy)),
		"lock_timeout_sec":  input.LockTimeoutSec,
	}
	payload["release"] = releaseNode

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

// LoadGitOpsConfig 封装当前模块的业务处理逻辑。
func (s *ReleaseStore) LoadGitOpsConfig(_ context.Context) (usecase.ReleaseGitOpsConfigOutput, error) {
	path := strings.TrimSpace(s.configPath)
	if path == "" {
		return defaultReleaseGitOpsConfig(), nil
	}

	content, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return defaultReleaseGitOpsConfig(), nil
		}
		return usecase.ReleaseGitOpsConfigOutput{}, fmt.Errorf("read config file failed: %w", err)
	}

	var payload map[string]interface{}
	if err := json.Unmarshal(content, &payload); err != nil {
		return usecase.ReleaseGitOpsConfigOutput{}, fmt.Errorf("decode config file failed: %w", err)
	}

	releaseNode := readMapNode(payload, "release")
	gitopsNode := readMapNode(releaseNode, "gitops_config")

	result := defaultReleaseGitOpsConfig()
	if value := strings.TrimSpace(fmt.Sprint(gitopsNode["helm_scan_path"])); value != "" {
		result.HelmScanPath = value
	}
	if value := strings.TrimSpace(fmt.Sprint(gitopsNode["kustomize_scan_path"])); value != "" {
		result.KustomizeScanPath = value
	}
	return result, nil
}

// SaveGitOpsConfig 封装当前模块的业务处理逻辑。
func (s *ReleaseStore) SaveGitOpsConfig(_ context.Context, input usecase.ReleaseGitOpsConfigInput) error {
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

	releaseNode := readMapNode(payload, "release")
	releaseNode["gitops_config"] = map[string]interface{}{
		"helm_scan_path":      strings.TrimSpace(input.HelmScanPath),
		"kustomize_scan_path": strings.TrimSpace(input.KustomizeScanPath),
	}
	payload["release"] = releaseNode

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

// defaultReleaseGitOpsConfig 封装当前模块的业务处理逻辑。
func defaultReleaseGitOpsConfig() usecase.ReleaseGitOpsConfigOutput {
	return usecase.ReleaseGitOpsConfigOutput{
		HelmScanPath:      "apps/helm",
		KustomizeScanPath: "apps/{app_key}/overlays/{env}",
	}
}

// readMapNode 封装当前模块的业务处理逻辑。
func readMapNode(payload map[string]interface{}, key string) map[string]interface{} {
	if payload == nil {
		return map[string]interface{}{}
	}
	if node, ok := payload[key].(map[string]interface{}); ok && node != nil {
		return node
	}
	return map[string]interface{}{}
}

// boolFromAny 封装当前模块的业务处理逻辑。
func boolFromAny(raw interface{}) bool {
	switch value := raw.(type) {
	case bool:
		return value
	case string:
		return strings.EqualFold(strings.TrimSpace(value), "true")
	default:
		return false
	}
}

// intFromAny 封装当前模块的业务处理逻辑。
func intFromAny(raw interface{}) int {
	switch value := raw.(type) {
	case float64:
		return int(value)
	case int:
		return value
	case int32:
		return int(value)
	case int64:
		return int(value)
	case string:
		text := strings.TrimSpace(value)
		if text == "" {
			return 0
		}
		var result int
		_, _ = fmt.Sscanf(text, "%d", &result)
		return result
	default:
		return 0
	}
}

// normalizeStringListFromAny 标准化输入值，保证后续逻辑使用统一格式。
func normalizeStringListFromAny(raw interface{}) []string {
	items, ok := raw.([]interface{})
	if !ok {
		return nil
	}
	values := make([]string, 0, len(items))
	for _, item := range items {
		values = append(values, strings.TrimSpace(fmt.Sprint(item)))
	}
	return normalizeStringList(values)
}

func normalizeEnvConfigsFromAny(raw interface{}) []usecase.ReleaseEnvironmentConfig {
	items, ok := raw.([]interface{})
	if !ok {
		return nil
	}
	values := make([]usecase.ReleaseEnvironmentConfig, 0, len(items))
	for _, item := range items {
		node, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		values = append(values, usecase.ReleaseEnvironmentConfig{
			Code:        stringFromAny(node["code"]),
			Description: stringFromAny(node["description"]),
		})
	}
	return normalizeEnvConfigs(values)
}

func stringFromAny(raw interface{}) string {
	if raw == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(raw))
}

func normalizeEnvConfigs(values []usecase.ReleaseEnvironmentConfig) []usecase.ReleaseEnvironmentConfig {
	result := make([]usecase.ReleaseEnvironmentConfig, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, item := range values {
		code := strings.TrimSpace(item.Code)
		if code == "" {
			continue
		}
		if _, exists := seen[code]; exists {
			continue
		}
		seen[code] = struct{}{}
		result = append(result, usecase.ReleaseEnvironmentConfig{
			Code:        code,
			Description: strings.TrimSpace(item.Description),
		})
	}
	return result
}

func releaseEnvConfigsFromOptions(values []string) []usecase.ReleaseEnvironmentConfig {
	options := normalizeStringList(values)
	result := make([]usecase.ReleaseEnvironmentConfig, 0, len(options))
	for _, item := range options {
		result = append(result, usecase.ReleaseEnvironmentConfig{Code: item})
	}
	return result
}

func releaseEnvOptionsFromConfigs(values []usecase.ReleaseEnvironmentConfig) []string {
	result := make([]string, 0, len(values))
	for _, item := range normalizeEnvConfigs(values) {
		result = append(result, item.Code)
	}
	return result
}

// normalizeStringList 标准化输入值，保证后续逻辑使用统一格式。
func normalizeStringList(values []string) []string {
	result := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, item := range values {
		value := strings.TrimSpace(item)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

// cloneStringList 查询并返回列表数据。
func cloneStringList(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	result := make([]string, len(values))
	copy(result, values)
	return result
}
