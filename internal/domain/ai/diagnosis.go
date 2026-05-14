package ai

import (
	"context"
	"time"
)

type StageDiagnosisStatus string

const (
	StageDiagnosisStatusSuccess StageDiagnosisStatus = "success"
	StageDiagnosisStatusFailed  StageDiagnosisStatus = "failed"
)

type StageDiagnosis struct {
	ID              string
	ReleaseOrderID  string
	StageID         string
	ExecutionID     string
	PipelineScope   string
	ExecutorType    string
	StageName       string
	StageStatus     string
	AIModelConfigID string
	AIModelName     string
	AIModel         string
	PromptVersion   string
	LogHash         string
	LogExcerpt      string
	Status          StageDiagnosisStatus
	ResultJSON      string
	ErrorMessage    string
	CreatedBy       string
	CreatedAt       time.Time
	FinishedAt      *time.Time
}

type StageDiagnosisCacheKey struct {
	StageID         string
	LogHash         string
	AIModelConfigID string
	PromptVersion   string
}

type StageDiagnosisRepository interface {
	InitSchema(ctx context.Context) error
	CreateStageDiagnosis(ctx context.Context, item StageDiagnosis) error
	FindSuccessfulStageDiagnosisByCacheKey(ctx context.Context, cache StageDiagnosisCacheKey) (StageDiagnosis, error)
	FindLatestStageDiagnosis(ctx context.Context, releaseOrderID string, stageID string) (StageDiagnosis, error)
	GetStageDiagnosisByID(ctx context.Context, id string) (StageDiagnosis, error)
}
