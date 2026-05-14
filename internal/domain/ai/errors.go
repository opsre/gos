package ai

import "errors"

var (
	ErrModelConfigNotFound         = errors.New("ai model config not found")
	ErrModelConfigDuplicated       = errors.New("ai model config already exists")
	ErrDiagnosisModelNotConfigured = errors.New("ai diagnosis model is not configured")
	ErrDiagnosisModelInUse         = errors.New("ai model config is current diagnosis model")
	ErrStageDiagnosisNotFound      = errors.New("ai stage diagnosis not found")
)
