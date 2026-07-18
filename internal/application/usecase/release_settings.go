package usecase

import (
	"context"
	"fmt"
	"strings"
)

type ReleaseSettingsStore interface {
	LoadEnvOptions(ctx context.Context) ([]string, error)
	SaveEnvOptions(ctx context.Context, values []string) error
	LoadEnvConfigs(ctx context.Context) ([]ReleaseEnvironmentConfig, error)
	SaveEnvConfigs(ctx context.Context, values []ReleaseEnvironmentConfig) error
	LoadDefaultEnvCode(ctx context.Context) (string, error)
	SaveDefaultEnvCode(ctx context.Context, value string) error
	LoadConcurrencySettings(ctx context.Context) (ReleaseConcurrencySettingsOutput, error)
	SaveConcurrencySettings(ctx context.Context, input ReleaseConcurrencySettingsInput) error
	LoadGitOpsConfig(ctx context.Context) (ReleaseGitOpsConfigOutput, error)
	SaveGitOpsConfig(ctx context.Context, input ReleaseGitOpsConfigInput) error
}

type ReleaseConcurrencyLockScope string

const (
	ReleaseConcurrencyLockScopeApplication      ReleaseConcurrencyLockScope = "application"
	ReleaseConcurrencyLockScopeApplicationEnv   ReleaseConcurrencyLockScope = "application_env"
	ReleaseConcurrencyLockScopeGitOpsRepoBranch ReleaseConcurrencyLockScope = "gitops_repo_branch"
)

// Valid 封装当前模块的业务处理逻辑。
func (s ReleaseConcurrencyLockScope) Valid() bool {
	switch s {
	case ReleaseConcurrencyLockScopeApplication, ReleaseConcurrencyLockScopeApplicationEnv, ReleaseConcurrencyLockScopeGitOpsRepoBranch:
		return true
	default:
		return false
	}
}

type ReleaseConcurrencyConflictStrategy string

const (
	ReleaseConcurrencyConflictStrategyReject ReleaseConcurrencyConflictStrategy = "reject"
	ReleaseConcurrencyConflictStrategyQueue  ReleaseConcurrencyConflictStrategy = "queue"
)

// Valid 封装当前模块的业务处理逻辑。
func (s ReleaseConcurrencyConflictStrategy) Valid() bool {
	switch s {
	case ReleaseConcurrencyConflictStrategyReject, ReleaseConcurrencyConflictStrategyQueue:
		return true
	default:
		return false
	}
}

type ReleaseConcurrencySettingsOutput struct {
	Enabled          bool                               `json:"enabled"`
	LockScope        ReleaseConcurrencyLockScope        `json:"lock_scope"`
	ConflictStrategy ReleaseConcurrencyConflictStrategy `json:"conflict_strategy"`
	LockTimeoutSec   int                                `json:"lock_timeout_sec"`
}

type ReleaseConcurrencySettingsInput = ReleaseConcurrencySettingsOutput

type ReleaseGitOpsConfigOutput struct {
	HelmScanPath      string `json:"helm_scan_path"`
	KustomizeScanPath string `json:"kustomize_scan_path"`
}

type ReleaseGitOpsConfigInput = ReleaseGitOpsConfigOutput

type ReleaseEnvironmentConfig struct {
	Code        string `json:"code"`
	Description string `json:"description"`
}

type ReleaseSettingsOutput struct {
	EnvOptions     []string                         `json:"env_options"`
	EnvConfigs     []ReleaseEnvironmentConfig       `json:"env_configs"`
	DefaultEnvCode string                           `json:"default_env_code"`
	Concurrency    ReleaseConcurrencySettingsOutput `json:"concurrency"`
	GitOpsConfig   ReleaseGitOpsConfigOutput        `json:"gitops_config"`
}

type QueryReleaseSettings struct {
	store ReleaseSettingsStore
}

// NewQueryReleaseSettings 创建并返回对应组件实例。
func NewQueryReleaseSettings(store ReleaseSettingsStore) *QueryReleaseSettings {
	return &QueryReleaseSettings{store: store}
}

// Execute 封装当前模块的业务处理逻辑。
func (uc *QueryReleaseSettings) Execute(ctx context.Context) (ReleaseSettingsOutput, error) {
	if uc == nil || uc.store == nil {
		return ReleaseSettingsOutput{}, fmt.Errorf("%w: release settings are not configured", ErrInvalidInput)
	}
	envConfigs, err := uc.store.LoadEnvConfigs(ctx)
	if err != nil {
		return ReleaseSettingsOutput{}, err
	}
	envConfigs = normalizeReleaseEnvConfigs(envConfigs)
	options := releaseEnvOptionsFromConfigs(envConfigs)
	if len(options) == 0 {
		loadedOptions, err := uc.store.LoadEnvOptions(ctx)
		if err != nil {
			return ReleaseSettingsOutput{}, err
		}
		options = normalizeReleaseEnvOptions(loadedOptions)
		envConfigs = releaseEnvConfigsFromOptions(options)
	}
	defaultEnv, err := uc.store.LoadDefaultEnvCode(ctx)
	if err != nil {
		return ReleaseSettingsOutput{}, err
	}
	concurrency, err := uc.store.LoadConcurrencySettings(ctx)
	if err != nil {
		return ReleaseSettingsOutput{}, err
	}
	gitopsConfig, err := uc.store.LoadGitOpsConfig(ctx)
	if err != nil {
		return ReleaseSettingsOutput{}, err
	}
	return ReleaseSettingsOutput{
		EnvOptions:     options,
		EnvConfigs:     envConfigs,
		DefaultEnvCode: defaultEnv,
		Concurrency:    normalizeConcurrencySettings(concurrency),
		GitOpsConfig:   normalizeGitOpsConfig(gitopsConfig),
	}, nil
}

type UpdateReleaseSettingsInput struct {
	EnvOptions     []string
	EnvConfigs     []ReleaseEnvironmentConfig
	DefaultEnvCode string
	Concurrency    ReleaseConcurrencySettingsInput
	GitOpsConfig   ReleaseGitOpsConfigInput
}

type UpdateReleaseSettings struct {
	store  ReleaseSettingsStore
	reader *QueryReleaseSettings
}

// NewUpdateReleaseSettings 创建并返回对应组件实例。
func NewUpdateReleaseSettings(store ReleaseSettingsStore, reader *QueryReleaseSettings) *UpdateReleaseSettings {
	return &UpdateReleaseSettings{store: store, reader: reader}
}

// Execute 封装当前模块的业务处理逻辑。
func (uc *UpdateReleaseSettings) Execute(ctx context.Context, input UpdateReleaseSettingsInput) (ReleaseSettingsOutput, error) {
	if uc == nil || uc.store == nil || uc.reader == nil {
		return ReleaseSettingsOutput{}, fmt.Errorf("%w: release settings are not configured", ErrInvalidInput)
	}
	envConfigs := normalizeReleaseEnvConfigs(input.EnvConfigs)
	if len(envConfigs) == 0 {
		envConfigs = releaseEnvConfigsFromOptions(input.EnvOptions)
	}
	options := releaseEnvOptionsFromConfigs(envConfigs)
	if false && len(options) == 0 {
		return ReleaseSettingsOutput{}, fmt.Errorf("%w: 至少需要配置一个发布环境", ErrInvalidInput)
	}
	if err := uc.store.SaveEnvConfigs(ctx, envConfigs); err != nil {
		return ReleaseSettingsOutput{}, err
	}
	if err := uc.store.SaveDefaultEnvCode(ctx, strings.TrimSpace(input.DefaultEnvCode)); err != nil {
		return ReleaseSettingsOutput{}, err
	}
	if err := uc.store.SaveConcurrencySettings(ctx, normalizeConcurrencySettings(input.Concurrency)); err != nil {
		return ReleaseSettingsOutput{}, err
	}
	if err := uc.store.SaveGitOpsConfig(ctx, normalizeGitOpsConfig(input.GitOpsConfig)); err != nil {
		return ReleaseSettingsOutput{}, err
	}
	return uc.reader.Execute(ctx)
}

func normalizeReleaseEnvConfigs(values []ReleaseEnvironmentConfig) []ReleaseEnvironmentConfig {
	result := make([]ReleaseEnvironmentConfig, 0, len(values))
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
		result = append(result, ReleaseEnvironmentConfig{
			Code:        code,
			Description: strings.TrimSpace(item.Description),
		})
	}
	return result
}

func releaseEnvConfigsFromOptions(values []string) []ReleaseEnvironmentConfig {
	options := normalizeReleaseEnvOptions(values)
	result := make([]ReleaseEnvironmentConfig, 0, len(options))
	for _, item := range options {
		result = append(result, ReleaseEnvironmentConfig{
			Code: item,
		})
	}
	return result
}

func releaseEnvOptionsFromConfigs(values []ReleaseEnvironmentConfig) []string {
	result := make([]string, 0, len(values))
	for _, item := range normalizeReleaseEnvConfigs(values) {
		result = append(result, item.Code)
	}
	return result
}

// normalizeReleaseEnvOptions 标准化输入值，保证后续逻辑使用统一格式。
func normalizeReleaseEnvOptions(values []string) []string {
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

// normalizeConcurrencySettings 标准化输入值，保证后续逻辑使用统一格式。
func normalizeConcurrencySettings(input ReleaseConcurrencySettingsInput) ReleaseConcurrencySettingsOutput {
	scope := ReleaseConcurrencyLockScope(strings.TrimSpace(string(input.LockScope)))
	if !scope.Valid() {
		scope = ReleaseConcurrencyLockScopeApplicationEnv
	}

	strategy := ReleaseConcurrencyConflictStrategy(strings.TrimSpace(string(input.ConflictStrategy)))
	if !strategy.Valid() {
		strategy = ReleaseConcurrencyConflictStrategyReject
	}

	timeout := input.LockTimeoutSec
	if timeout <= 0 {
		timeout = 1800
	}
	if timeout < 30 {
		timeout = 30
	}
	if timeout > 86400 {
		timeout = 86400
	}

	return ReleaseConcurrencySettingsOutput{
		Enabled:          input.Enabled,
		LockScope:        scope,
		ConflictStrategy: strategy,
		LockTimeoutSec:   timeout,
	}
}

const (
	defaultHelmScanPath      = "apps/helm"
	defaultKustomizeScanPath = "apps/{app_key}/overlays/{env}"
)

// normalizeGitOpsConfig 标准化输入值，保证后续逻辑使用统一格式。
func normalizeGitOpsConfig(input ReleaseGitOpsConfigInput) ReleaseGitOpsConfigOutput {
	helmPath := strings.TrimSpace(input.HelmScanPath)
	if helmPath == "" {
		helmPath = defaultHelmScanPath
	}
	kustomizePath := strings.TrimSpace(input.KustomizeScanPath)
	if kustomizePath == "" {
		kustomizePath = defaultKustomizeScanPath
	}
	return ReleaseGitOpsConfigOutput{
		HelmScanPath:      strings.TrimRight(helmPath, "/"),
		KustomizeScanPath: strings.TrimRight(kustomizePath, "/"),
	}
}
