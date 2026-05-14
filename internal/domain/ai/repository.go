package ai

import (
	"context"
	"time"
)

type ModelConfigRepository interface {
	InitSchema(ctx context.Context) error
	CreateModelConfig(ctx context.Context, item ModelConfig) error
	ListModelConfigs(ctx context.Context) ([]ModelConfig, error)
	GetModelConfigByID(ctx context.Context, id string) (ModelConfig, error)
	UpdateModelConfig(ctx context.Context, id string, input ModelConfigUpdateInput) (ModelConfig, error)
	DeleteModelConfig(ctx context.Context, id string) error
	SetDiagnosisModel(ctx context.Context, id string, updatedAt time.Time) (ModelConfig, error)
	UnsetDiagnosisModel(ctx context.Context, id string, updatedAt time.Time) (ModelConfig, error)
	GetDiagnosisModel(ctx context.Context) (ModelConfig, error)
}
