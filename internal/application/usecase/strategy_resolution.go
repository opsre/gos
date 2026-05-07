package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	domain "gos/internal/domain/release"
	strategy "gos/internal/domain/strategy"
)

type StrategyResolutionService struct {
	strategyRepo strategy.Repository
	now          func() time.Time
}

func NewStrategyResolutionService(repo strategy.Repository) *StrategyResolutionService {
	return &StrategyResolutionService{strategyRepo: repo, now: time.Now}
}

type StrategyResolveInput struct {
	ApplicationID string
	EnvCode       string
	GitOpsType    domain.GitOpsType
}

type ResolvedConfig struct {
	Template    strategy.ReleaseStrategyTemplate
	Binding     strategy.ApplicationEnvStrategyBinding
	Runtime     *strategy.ApplicationEnvRuntimeBinding
	Snapshot    strategy.StrategySnapshot
	SnapshotJSON string
}

func (s *StrategyResolutionService) Resolve(ctx context.Context, input StrategyResolveInput) (*ResolvedConfig, error) {
	input.ApplicationID = strings.TrimSpace(input.ApplicationID)
	input.EnvCode = strings.TrimSpace(input.EnvCode)
	if input.ApplicationID == "" || input.EnvCode == "" {
		return nil, fmt.Errorf("%w: application_id and env_code are required", ErrInvalidInput)
	}

	binding, err := s.strategyRepo.GetStrategyBindingByAppEnv(ctx, input.ApplicationID, input.EnvCode)
	if err != nil {
		if errors.Is(err, strategy.ErrStrategyBindingNotFound) {
			return nil, nil
		}
		return nil, err
	}

	template, err := s.strategyRepo.GetTemplateByID(ctx, binding.StrategyTemplateID)
	if err != nil {
		return nil, err
	}
	if template.Status != strategy.TemplateStatusActive {
		return nil, fmt.Errorf("%w: strategy template %s is not active", ErrInvalidInput, template.Name)
	}

	var runtime *strategy.ApplicationEnvRuntimeBinding
	runtimeVal, err := s.strategyRepo.GetRuntimeBindingByAppEnv(ctx, input.ApplicationID, input.EnvCode)
	if err != nil && !errors.Is(err, strategy.ErrRuntimeBindingNotFound) {
		return nil, err
	}
	if err == nil {
		runtime = &runtimeVal
	}

	namespace := "default"
	workloadName := ""
	if runtime != nil {
		namespace = firstNonEmpty(strings.TrimSpace(runtime.Namespace), "default")
		workloadName = strings.TrimSpace(runtime.WorkloadName)
	}

	engine := template.StrategyEngine
	if engine == "" {
		engine = strategy.StrategyEngineK8sNative
	}

	var tmplCfg strategy.StrategyTemplateConfig
	if template.StrategyConfig != "" {
		if err := json.Unmarshal([]byte(template.StrategyConfig), &tmplCfg); err != nil {
			return nil, fmt.Errorf("failed to parse strategy template config: %w", err)
		}
	}

	var overrideCfg strategy.StrategyTemplateConfig
	if binding.OverridesConfig != "" {
		if err := json.Unmarshal([]byte(binding.OverridesConfig), &overrideCfg); err != nil {
			return nil, fmt.Errorf("failed to parse strategy binding overrides: %w", err)
		}
	}

	tmplCfg = mergeStrategyConfig(tmplCfg, overrideCfg)

	config := strategy.ResolvedStrategyConfig{
		MaxSurge:       tmplCfg.MaxSurge,
		MaxUnavailable: tmplCfg.MaxUnavailable,
		TimeoutSeconds: tmplCfg.TimeoutSeconds,
		AutoPromotion:  tmplCfg.AutoPromotion,
		AutoRollback:   tmplCfg.AutoRollback,
		BlueGreen:      tmplCfg.BlueGreen,
	}
	if len(tmplCfg.CanarySteps) > 0 {
		config.CanarySteps = append([]strategy.CanaryStep(nil), tmplCfg.CanarySteps...)
	}

	snapshot := strategy.StrategySnapshot{
		StrategyName:   template.Name,
		StrategyType:   template.StrategyType,
		StrategyEngine: engine,
		TemplateID:     template.ID,
		EnvCode:        input.EnvCode,
		ApplicationID:  input.ApplicationID,
		Namespace:      namespace,
		WorkloadName:   workloadName,
		Config:         config,
	}

	snapshotJSON, err := json.Marshal(snapshot)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal strategy snapshot: %w", err)
	}

	return &ResolvedConfig{
		Template:     template,
		Binding:      binding,
		Runtime:      runtime,
		Snapshot:     snapshot,
		SnapshotJSON: string(snapshotJSON),
	}, nil
}

func (s *StrategyResolutionService) Precheck(_ context.Context, resolved *ResolvedConfig) error {
	if resolved == nil {
		return nil
	}

	if resolved.Runtime == nil {
		return fmt.Errorf("%w: no runtime binding configured for application %s env %s, cannot apply strategy",
			strategy.ErrPrecheckFailed, resolved.Snapshot.ApplicationID, resolved.Snapshot.EnvCode)
	}

	engine := resolved.Snapshot.StrategyEngine

	switch resolved.Snapshot.StrategyType {
	case strategy.StrategyTypeRollingUpdate:
	case strategy.StrategyTypeCanary:
		if engine == strategy.StrategyEngineK8sNative && len(resolved.Snapshot.Config.CanarySteps) > 0 {
			return fmt.Errorf("%w: k8s_native engine does not support canary steps, use argo_rollouts",
				strategy.ErrPrecheckFailed)
		}
	case strategy.StrategyTypeBlueGreen:
		if engine == strategy.StrategyEngineK8sNative {
			return fmt.Errorf("%w: k8s_native engine does not support blue_green strategy, use argo_rollouts",
				strategy.ErrPrecheckFailed)
		}
		if resolved.Snapshot.Config.BlueGreen != nil &&
			(strings.TrimSpace(resolved.Snapshot.Config.BlueGreen.ActiveService) == "" ||
				strings.TrimSpace(resolved.Snapshot.Config.BlueGreen.PreviewService) == "") {
			return fmt.Errorf("%w: blue_green config requires active_service and preview_service",
				strategy.ErrPrecheckFailed)
		}
	}

	return nil
}

func mergeStrategyConfig(base, override strategy.StrategyTemplateConfig) strategy.StrategyTemplateConfig {
	if override.MaxSurge != "" {
		base.MaxSurge = override.MaxSurge
	}
	if override.MaxUnavailable != "" {
		base.MaxUnavailable = override.MaxUnavailable
	}
	if override.TimeoutSeconds > 0 {
		base.TimeoutSeconds = override.TimeoutSeconds
	}
	if len(override.CanarySteps) > 0 {
		base.CanarySteps = override.CanarySteps
	}
	if override.BlueGreen != nil {
		base.BlueGreen = override.BlueGreen
	}
	if override.AutoPromotion {
		base.AutoPromotion = override.AutoPromotion
	}
	if override.AutoRollback {
		base.AutoRollback = override.AutoRollback
	}
	if override.RollbackEnabled {
		base.RollbackEnabled = override.RollbackEnabled
	}
	return base
}
