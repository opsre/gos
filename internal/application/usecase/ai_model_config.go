package usecase

import (
	"context"
	"fmt"
	"strings"
	"time"

	aidomain "gos/internal/domain/ai"
	"gos/internal/support/secure"
)

type AIModelConfigManager struct {
	repo aidomain.ModelConfigRepository
	now  func() time.Time
}

type AIModelConfigInput struct {
	Name        string
	Provider    string
	BaseURL     string
	Model       string
	APIKey      *string
	Temperature float64
	MaxTokens   int
	TimeoutSec  int
	Enabled     bool
	CreatedBy   string
}

type AIModelConfigOutput struct {
	ID               string    `json:"id"`
	Name             string    `json:"name"`
	Provider         string    `json:"provider"`
	BaseURL          string    `json:"base_url"`
	Model            string    `json:"model"`
	APIKeyConfigured bool      `json:"api_key_configured"`
	Temperature      float64   `json:"temperature"`
	MaxTokens        int       `json:"max_tokens"`
	TimeoutSec       int       `json:"timeout_sec"`
	Enabled          bool      `json:"enabled"`
	IsDiagnosisModel bool      `json:"is_diagnosis_model"`
	CreatedBy        string    `json:"created_by"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

func NewAIModelConfigManager(repo aidomain.ModelConfigRepository) *AIModelConfigManager {
	return &AIModelConfigManager{
		repo: repo,
		now: func() time.Time {
			return time.Now().UTC()
		},
	}
}

func (m *AIModelConfigManager) Create(ctx context.Context, input AIModelConfigInput) (AIModelConfigOutput, error) {
	if m == nil || m.repo == nil {
		return AIModelConfigOutput{}, fmt.Errorf("%w: ai model repository is not configured", ErrInvalidInput)
	}
	normalized, err := normalizeAIModelConfigInput(input)
	if err != nil {
		return AIModelConfigOutput{}, err
	}
	apiKeyCipher, err := encryptOptionalAPIKey(normalized.APIKey, "")
	if err != nil {
		return AIModelConfigOutput{}, err
	}
	now := m.now()
	item := aidomain.ModelConfig{
		ID:           generateID("aimc"),
		Name:         normalized.Name,
		Provider:     aidomain.ModelProvider(normalized.Provider),
		BaseURL:      normalized.BaseURL,
		Model:        normalized.Model,
		APIKeyCipher: apiKeyCipher,
		Temperature:  normalized.Temperature,
		MaxTokens:    normalized.MaxTokens,
		TimeoutSec:   normalized.TimeoutSec,
		Enabled:      normalized.Enabled,
		CreatedBy:    strings.TrimSpace(input.CreatedBy),
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if err := m.repo.CreateModelConfig(ctx, item); err != nil {
		return AIModelConfigOutput{}, err
	}
	return toAIModelConfigOutput(item), nil
}

func (m *AIModelConfigManager) List(ctx context.Context) ([]AIModelConfigOutput, error) {
	if m == nil || m.repo == nil {
		return nil, fmt.Errorf("%w: ai model repository is not configured", ErrInvalidInput)
	}
	items, err := m.repo.ListModelConfigs(ctx)
	if err != nil {
		return nil, err
	}
	output := make([]AIModelConfigOutput, 0, len(items))
	for _, item := range items {
		output = append(output, toAIModelConfigOutput(item))
	}
	return output, nil
}

func (m *AIModelConfigManager) Get(ctx context.Context, id string) (AIModelConfigOutput, error) {
	item, err := m.GetDomainConfig(ctx, id)
	if err != nil {
		return AIModelConfigOutput{}, err
	}
	return toAIModelConfigOutput(item), nil
}

func (m *AIModelConfigManager) GetDomainConfig(ctx context.Context, id string) (aidomain.ModelConfig, error) {
	if m == nil || m.repo == nil {
		return aidomain.ModelConfig{}, fmt.Errorf("%w: ai model repository is not configured", ErrInvalidInput)
	}
	item, err := m.repo.GetModelConfigByID(ctx, strings.TrimSpace(id))
	if err != nil {
		return aidomain.ModelConfig{}, err
	}
	return item, nil
}

func (m *AIModelConfigManager) Update(ctx context.Context, id string, input AIModelConfigInput) (AIModelConfigOutput, error) {
	if m == nil || m.repo == nil {
		return AIModelConfigOutput{}, fmt.Errorf("%w: ai model repository is not configured", ErrInvalidInput)
	}
	trimmedID := strings.TrimSpace(id)
	if trimmedID == "" {
		return AIModelConfigOutput{}, ErrInvalidID
	}
	existing, err := m.repo.GetModelConfigByID(ctx, trimmedID)
	if err != nil {
		return AIModelConfigOutput{}, err
	}
	normalized, err := normalizeAIModelConfigInput(input)
	if err != nil {
		return AIModelConfigOutput{}, err
	}
	apiKeyCipher, err := encryptOptionalAPIKey(normalized.APIKey, existing.APIKeyCipher)
	if err != nil {
		return AIModelConfigOutput{}, err
	}
	updated, err := m.repo.UpdateModelConfig(ctx, trimmedID, aidomain.ModelConfigUpdateInput{
		Name:         normalized.Name,
		Provider:     aidomain.ModelProvider(normalized.Provider),
		BaseURL:      normalized.BaseURL,
		Model:        normalized.Model,
		APIKeyCipher: apiKeyCipher,
		Temperature:  normalized.Temperature,
		MaxTokens:    normalized.MaxTokens,
		TimeoutSec:   normalized.TimeoutSec,
		Enabled:      normalized.Enabled,
		UpdatedAt:    m.now(),
	})
	if err != nil {
		return AIModelConfigOutput{}, err
	}
	return toAIModelConfigOutput(updated), nil
}

func (m *AIModelConfigManager) Delete(ctx context.Context, id string) error {
	if m == nil || m.repo == nil {
		return fmt.Errorf("%w: ai model repository is not configured", ErrInvalidInput)
	}
	return m.repo.DeleteModelConfig(ctx, strings.TrimSpace(id))
}

func (m *AIModelConfigManager) SetDiagnosisModel(ctx context.Context, id string) (AIModelConfigOutput, error) {
	if m == nil || m.repo == nil {
		return AIModelConfigOutput{}, fmt.Errorf("%w: ai model repository is not configured", ErrInvalidInput)
	}
	item, err := m.repo.GetModelConfigByID(ctx, strings.TrimSpace(id))
	if err != nil {
		return AIModelConfigOutput{}, err
	}
	if !item.Enabled {
		return AIModelConfigOutput{}, fmt.Errorf("%w: disabled ai model cannot be diagnosis model", ErrInvalidInput)
	}
	if !item.HasAPIKey() {
		return AIModelConfigOutput{}, fmt.Errorf("%w: ai model api key is required", ErrInvalidInput)
	}
	selected, err := m.repo.SetDiagnosisModel(ctx, item.ID, m.now())
	if err != nil {
		return AIModelConfigOutput{}, err
	}
	return toAIModelConfigOutput(selected), nil
}

func (m *AIModelConfigManager) UnsetDiagnosisModel(ctx context.Context, id string) (AIModelConfigOutput, error) {
	if m == nil || m.repo == nil {
		return AIModelConfigOutput{}, fmt.Errorf("%w: ai model repository is not configured", ErrInvalidInput)
	}
	item, err := m.repo.GetModelConfigByID(ctx, strings.TrimSpace(id))
	if err != nil {
		return AIModelConfigOutput{}, err
	}
	if !item.IsDiagnosisModel {
		return AIModelConfigOutput{}, fmt.Errorf("%w: ai model is not current diagnosis model", ErrInvalidInput)
	}
	cleared, err := m.repo.UnsetDiagnosisModel(ctx, item.ID, m.now())
	if err != nil {
		return AIModelConfigOutput{}, err
	}
	return toAIModelConfigOutput(cleared), nil
}

func normalizeAIModelConfigInput(input AIModelConfigInput) (AIModelConfigInput, error) {
	input.Name = strings.TrimSpace(input.Name)
	input.Provider = strings.TrimSpace(input.Provider)
	input.BaseURL = strings.TrimRight(strings.TrimSpace(input.BaseURL), "/")
	input.Model = strings.TrimSpace(input.Model)
	if input.Name == "" || input.Provider == "" || input.BaseURL == "" || input.Model == "" {
		return AIModelConfigInput{}, fmt.Errorf("%w: name, provider, base_url and model are required", ErrInvalidInput)
	}
	provider := aidomain.ModelProvider(input.Provider)
	if !provider.Valid() {
		return AIModelConfigInput{}, fmt.Errorf("%w: unsupported ai model provider", ErrInvalidInput)
	}
	if input.Temperature < 0 {
		input.Temperature = 0
	}
	if input.Temperature > 2 {
		input.Temperature = 2
	}
	if input.MaxTokens <= 0 {
		input.MaxTokens = 2048
	}
	if input.TimeoutSec <= 0 {
		input.TimeoutSec = 60
	}
	return input, nil
}

func encryptOptionalAPIKey(apiKey *string, existingCipher string) (string, error) {
	if apiKey == nil {
		return strings.TrimSpace(existingCipher), nil
	}
	trimmed := strings.TrimSpace(*apiKey)
	if trimmed == "" {
		return "", nil
	}
	return secure.EncryptString(trimmed)
}

func toAIModelConfigOutput(item aidomain.ModelConfig) AIModelConfigOutput {
	return AIModelConfigOutput{
		ID:               item.ID,
		Name:             item.Name,
		Provider:         string(item.Provider),
		BaseURL:          item.BaseURL,
		Model:            item.Model,
		APIKeyConfigured: item.HasAPIKey(),
		Temperature:      item.Temperature,
		MaxTokens:        item.MaxTokens,
		TimeoutSec:       item.TimeoutSec,
		Enabled:          item.Enabled,
		IsDiagnosisModel: item.IsDiagnosisModel,
		CreatedBy:        item.CreatedBy,
		CreatedAt:        item.CreatedAt,
		UpdatedAt:        item.UpdatedAt,
	}
}
