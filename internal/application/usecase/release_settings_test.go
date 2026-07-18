package usecase

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestQueryReleaseSettingsExposesEnvConfigsForDescriptions(t *testing.T) {
	t.Parallel()

	reader := NewQueryReleaseSettings(releaseSettingsStoreStub{
		envOptions: []string{"dev", "prod"},
		defaultEnv: "prod",
		concurrency: ReleaseConcurrencySettingsOutput{
			Enabled:          true,
			LockScope:        ReleaseConcurrencyLockScopeApplicationEnv,
			ConflictStrategy: ReleaseConcurrencyConflictStrategyReject,
			LockTimeoutSec:   1800,
		},
		gitopsConfig: ReleaseGitOpsConfigOutput{
			HelmScanPath:      "apps/helm",
			KustomizeScanPath: "apps/{app_key}/overlays/{env}",
		},
	})

	output, err := reader.Execute(context.Background())
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	payload, err := json.Marshal(output)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	if !strings.Contains(string(payload), `"env_configs":[{"code":"dev","description":""},{"code":"prod","description":""}]`) {
		t.Fatalf("release settings json = %s, want legacy env_options converted without invented descriptions", payload)
	}
}

func TestQueryReleaseSettingsKeepsConfiguredEnvDescriptions(t *testing.T) {
	t.Parallel()

	reader := NewQueryReleaseSettings(releaseSettingsStoreStub{
		envConfigs: []ReleaseEnvironmentConfig{
			{Code: "prod", Description: "生产灰度审批后发布"},
		},
		concurrency: ReleaseConcurrencySettingsOutput{
			Enabled:          true,
			LockScope:        ReleaseConcurrencyLockScopeApplicationEnv,
			ConflictStrategy: ReleaseConcurrencyConflictStrategyReject,
			LockTimeoutSec:   1800,
		},
		gitopsConfig: ReleaseGitOpsConfigOutput{
			HelmScanPath:      "apps/helm",
			KustomizeScanPath: "apps/{app_key}/overlays/{env}",
		},
	})

	output, err := reader.Execute(context.Background())
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if len(output.EnvConfigs) != 1 || output.EnvConfigs[0].Description != "生产灰度审批后发布" {
		t.Fatalf("env configs = %#v, want configured description kept", output.EnvConfigs)
	}
}

type releaseSettingsStoreStub struct {
	envConfigs   []ReleaseEnvironmentConfig
	envOptions   []string
	defaultEnv   string
	concurrency  ReleaseConcurrencySettingsOutput
	gitopsConfig ReleaseGitOpsConfigOutput
}

func (s releaseSettingsStoreStub) LoadEnvOptions(context.Context) ([]string, error) {
	return append([]string(nil), s.envOptions...), nil
}

func (s releaseSettingsStoreStub) SaveEnvOptions(context.Context, []string) error {
	return nil
}

func (s releaseSettingsStoreStub) LoadEnvConfigs(context.Context) ([]ReleaseEnvironmentConfig, error) {
	if len(s.envConfigs) > 0 {
		return append([]ReleaseEnvironmentConfig(nil), s.envConfigs...), nil
	}
	return releaseEnvConfigsFromOptions(s.envOptions), nil
}

func (s releaseSettingsStoreStub) SaveEnvConfigs(context.Context, []ReleaseEnvironmentConfig) error {
	return nil
}

func (s releaseSettingsStoreStub) LoadDefaultEnvCode(context.Context) (string, error) {
	return s.defaultEnv, nil
}

func (s releaseSettingsStoreStub) SaveDefaultEnvCode(context.Context, string) error {
	return nil
}

func (s releaseSettingsStoreStub) LoadConcurrencySettings(context.Context) (ReleaseConcurrencySettingsOutput, error) {
	return s.concurrency, nil
}

func (s releaseSettingsStoreStub) SaveConcurrencySettings(context.Context, ReleaseConcurrencySettingsInput) error {
	return nil
}

func (s releaseSettingsStoreStub) LoadGitOpsConfig(context.Context) (ReleaseGitOpsConfigOutput, error) {
	return s.gitopsConfig, nil
}

func (s releaseSettingsStoreStub) SaveGitOpsConfig(context.Context, ReleaseGitOpsConfigInput) error {
	return nil
}
