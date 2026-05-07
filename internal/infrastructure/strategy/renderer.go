package strategy

import (
	"fmt"
	"strconv"

	domain "gos/internal/domain/strategy"
)

type Renderer struct{}

func NewRenderer() *Renderer {
	return &Renderer{}
}

func (r *Renderer) Render(snapshot *domain.StrategySnapshot) (map[string]interface{}, error) {
	if snapshot == nil {
		return nil, nil
	}

	result := make(map[string]interface{})

	cfg := snapshot.Config

	strategyValues := make(map[string]interface{})
	strategyValues["type"] = string(snapshot.StrategyType)
	strategyValues["engine"] = string(snapshot.StrategyEngine)

	switch snapshot.StrategyType {
	case domain.StrategyTypeRollingUpdate:
		r.renderRollingUpdate(strategyValues, cfg)
	case domain.StrategyTypeCanary:
		r.renderCanary(strategyValues, cfg)
	case domain.StrategyTypeBlueGreen:
		r.renderBlueGreen(strategyValues, cfg)
	}

	result["strategy"] = strategyValues

	if cfg.MaxSurge != "" {
		if v, err := r.parseQuantity(cfg.MaxSurge); err == nil {
			result["maxSurge"] = v
		} else {
			result["maxSurge"] = cfg.MaxSurge
		}
	}
	if cfg.MaxUnavailable != "" {
		if v, err := r.parseQuantity(cfg.MaxUnavailable); err == nil {
			result["maxUnavailable"] = v
		} else {
			result["maxUnavailable"] = cfg.MaxUnavailable
		}
	}

	return result, nil
}

func (r *Renderer) renderRollingUpdate(vals map[string]interface{}, cfg domain.ResolvedStrategyConfig) {
	rollingUpdate := make(map[string]interface{})
	if cfg.MaxSurge != "" {
		rollingUpdate["maxSurge"] = cfg.MaxSurge
	}
	if cfg.MaxUnavailable != "" {
		rollingUpdate["maxUnavailable"] = cfg.MaxUnavailable
	}
	if cfg.TimeoutSeconds > 0 {
		rollingUpdate["timeoutSeconds"] = cfg.TimeoutSeconds
	}
	vals["rollingUpdate"] = rollingUpdate
}

func (r *Renderer) renderCanary(vals map[string]interface{}, cfg domain.ResolvedStrategyConfig) {
	canary := make(map[string]interface{})
	if cfg.MaxSurge != "" {
		canary["maxSurge"] = cfg.MaxSurge
	}
	if cfg.MaxUnavailable != "" {
		canary["maxUnavailable"] = cfg.MaxUnavailable
	}
	if cfg.TimeoutSeconds > 0 {
		canary["timeoutSeconds"] = cfg.TimeoutSeconds
	}
	if len(cfg.CanarySteps) > 0 {
		steps := make([]map[string]interface{}, 0, len(cfg.CanarySteps))
		for _, step := range cfg.CanarySteps {
			s := map[string]interface{}{
				"setWeight": step.SetWeight,
			}
			if step.PauseSeconds > 0 {
				s["pause"] = map[string]interface{}{
					"duration": fmt.Sprintf("%ds", step.PauseSeconds),
				}
			} else if step.PauseDuration != "" {
				s["pause"] = map[string]interface{}{
					"duration": step.PauseDuration,
				}
			}
			steps = append(steps, s)
		}
		canary["steps"] = steps
	}
	if cfg.AutoPromotion {
		canary["autoPromotionEnabled"] = true
	}
	vals["canary"] = canary
}

func (r *Renderer) renderBlueGreen(vals map[string]interface{}, cfg domain.ResolvedStrategyConfig) {
	blueGreen := make(map[string]interface{})
	if cfg.BlueGreen != nil {
		blueGreen["activeService"] = cfg.BlueGreen.ActiveService
		blueGreen["previewService"] = cfg.BlueGreen.PreviewService
		if cfg.BlueGreen.AutoPromotionSeconds > 0 {
			blueGreen["autoPromotionSeconds"] = cfg.BlueGreen.AutoPromotionSeconds
		}
	}
	if cfg.AutoPromotion {
		blueGreen["autoPromotionEnabled"] = true
	}
	vals["blueGreen"] = blueGreen
}

func (r *Renderer) parseQuantity(s string) (interface{}, error) {
	if s == "" {
		return nil, fmt.Errorf("empty")
	}
	if v, err := strconv.Atoi(s); err == nil {
		return v, nil
	}
	return nil, fmt.Errorf("not an integer")
}
