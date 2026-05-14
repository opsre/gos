package ai

import (
	"strings"
	"time"
)

type ModelProvider string

const (
	ProviderOpenAICompatible ModelProvider = "openai_compatible"
)

func (p ModelProvider) Valid() bool {
	switch p {
	case ProviderOpenAICompatible:
		return true
	default:
		return false
	}
}

type ModelConfig struct {
	ID               string
	Name             string
	Provider         ModelProvider
	BaseURL          string
	Model            string
	APIKeyCipher     string
	Temperature      float64
	MaxTokens        int
	TimeoutSec       int
	Enabled          bool
	IsDiagnosisModel bool
	CreatedBy        string
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

func (m ModelConfig) HasAPIKey() bool {
	return strings.TrimSpace(m.APIKeyCipher) != ""
}

type ModelConfigUpdateInput struct {
	Name         string
	Provider     ModelProvider
	BaseURL      string
	Model        string
	APIKeyCipher string
	Temperature  float64
	MaxTokens    int
	TimeoutSec   int
	Enabled      bool
	UpdatedAt    time.Time
}
