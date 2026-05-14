package usecase

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	aidomain "gos/internal/domain/ai"
	"gos/internal/support/secure"
)

func TestAIModelConfigManagerEncryptsAndPreservesAPIKey(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	now := time.Unix(1_710_000_000, 0).UTC()
	repo := newAIModelConfigRepoFake()
	manager := NewAIModelConfigManager(repo)
	manager.now = func() time.Time { return now }

	created, err := manager.Create(ctx, AIModelConfigInput{
		Name:        "诊断模型",
		Provider:    "openai_compatible",
		BaseURL:     "https://api.example.com/v1",
		Model:       "chat-diagnosis",
		APIKey:      stringPtr("sk-test-secret"),
		Temperature: 0.2,
		MaxTokens:   2048,
		TimeoutSec:  60,
		Enabled:     true,
		CreatedBy:   "usr-1",
	})
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	if !created.APIKeyConfigured {
		t.Fatalf("APIKeyConfigured = false, want true")
	}
	stored := repo.items[created.ID]
	if stored.APIKeyCipher == "" || stored.APIKeyCipher == "sk-test-secret" {
		t.Fatalf("APIKeyCipher = %q, want encrypted value", stored.APIKeyCipher)
	}
	plain, err := secure.DecryptString(stored.APIKeyCipher)
	if err != nil {
		t.Fatalf("DecryptString failed: %v", err)
	}
	if plain != "sk-test-secret" {
		t.Fatalf("decrypted API key = %q, want original secret", plain)
	}

	updated, err := manager.Update(ctx, created.ID, AIModelConfigInput{
		Name:        "诊断模型更新",
		Provider:    "openai_compatible",
		BaseURL:     "https://api.changed.example/v1",
		Model:       "chat-diagnosis-v2",
		APIKey:      nil,
		Temperature: 0.1,
		MaxTokens:   1024,
		TimeoutSec:  30,
		Enabled:     true,
	})
	if err != nil {
		t.Fatalf("Update failed: %v", err)
	}
	if !updated.APIKeyConfigured {
		t.Fatalf("updated.APIKeyConfigured = false, want true")
	}
	if repo.items[created.ID].APIKeyCipher != stored.APIKeyCipher {
		t.Fatalf("API key was not preserved: got %q want %q", repo.items[created.ID].APIKeyCipher, stored.APIKeyCipher)
	}
}

func TestAIModelConfigManagerSetDiagnosisModelValidatesEnabledAndAPIKey(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	now := time.Unix(1_710_000_000, 0).UTC()
	repo := newAIModelConfigRepoFake()
	manager := NewAIModelConfigManager(repo)
	manager.now = func() time.Time { return now }

	disabled, err := manager.Create(ctx, AIModelConfigInput{
		Name:      "disabled",
		Provider:  "openai_compatible",
		BaseURL:   "https://api.example.com/v1",
		Model:     "chat",
		APIKey:    stringPtr("sk-disabled"),
		Enabled:   false,
		CreatedBy: "usr-1",
	})
	if err != nil {
		t.Fatalf("Create disabled failed: %v", err)
	}
	if _, err := manager.SetDiagnosisModel(ctx, disabled.ID); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("SetDiagnosisModel(disabled) err = %v, want ErrInvalidInput", err)
	}

	noKey, err := manager.Create(ctx, AIModelConfigInput{
		Name:      "no key",
		Provider:  "openai_compatible",
		BaseURL:   "https://api.example.com/v1",
		Model:     "chat",
		Enabled:   true,
		CreatedBy: "usr-1",
	})
	if err != nil {
		t.Fatalf("Create noKey failed: %v", err)
	}
	if _, err := manager.SetDiagnosisModel(ctx, noKey.ID); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("SetDiagnosisModel(noKey) err = %v, want ErrInvalidInput", err)
	}

	enabled, err := manager.Create(ctx, AIModelConfigInput{
		Name:      "enabled",
		Provider:  "openai_compatible",
		BaseURL:   "https://api.example.com/v1",
		Model:     "chat",
		APIKey:    stringPtr("sk-enabled"),
		Enabled:   true,
		CreatedBy: "usr-1",
	})
	if err != nil {
		t.Fatalf("Create enabled failed: %v", err)
	}
	selected, err := manager.SetDiagnosisModel(ctx, enabled.ID)
	if err != nil {
		t.Fatalf("SetDiagnosisModel(enabled) failed: %v", err)
	}
	if selected.ID != enabled.ID || !selected.IsDiagnosisModel {
		t.Fatalf("selected = %#v, want enabled diagnosis model", selected)
	}
}

func TestAIModelConfigManagerUnsetDiagnosisModelClearsCurrentModel(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	now := time.Unix(1_710_000_000, 0).UTC()
	repo := newAIModelConfigRepoFake()
	manager := NewAIModelConfigManager(repo)
	manager.now = func() time.Time { return now }

	created, err := manager.Create(ctx, AIModelConfigInput{
		Name:      "enabled",
		Provider:  "openai_compatible",
		BaseURL:   "https://api.example.com/v1",
		Model:     "chat",
		APIKey:    stringPtr("sk-enabled"),
		Enabled:   true,
		CreatedBy: "usr-1",
	})
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	if _, err := manager.SetDiagnosisModel(ctx, created.ID); err != nil {
		t.Fatalf("SetDiagnosisModel failed: %v", err)
	}

	cleared, err := manager.UnsetDiagnosisModel(ctx, created.ID)
	if err != nil {
		t.Fatalf("UnsetDiagnosisModel failed: %v", err)
	}
	if cleared.ID != created.ID || cleared.IsDiagnosisModel {
		t.Fatalf("cleared = %#v, want same model with IsDiagnosisModel=false", cleared)
	}
	if _, err := repo.GetDiagnosisModel(ctx); !errors.Is(err, aidomain.ErrDiagnosisModelNotConfigured) {
		t.Fatalf("GetDiagnosisModel err = %v, want ErrDiagnosisModelNotConfigured", err)
	}
}

func TestAIModelConfigManagerUnsetDiagnosisModelRejectsNonCurrentModel(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	repo := newAIModelConfigRepoFake()
	manager := NewAIModelConfigManager(repo)

	created, err := manager.Create(ctx, AIModelConfigInput{
		Name:      "not current",
		Provider:  "openai_compatible",
		BaseURL:   "https://api.example.com/v1",
		Model:     "chat",
		APIKey:    stringPtr("sk-enabled"),
		Enabled:   true,
		CreatedBy: "usr-1",
	})
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	if _, err := manager.UnsetDiagnosisModel(ctx, created.ID); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("UnsetDiagnosisModel err = %v, want ErrInvalidInput", err)
	}
}

func stringPtr(value string) *string {
	return &value
}

type aiModelConfigRepoFake struct {
	items map[string]aidomain.ModelConfig
}

func newAIModelConfigRepoFake() *aiModelConfigRepoFake {
	return &aiModelConfigRepoFake{items: make(map[string]aidomain.ModelConfig)}
}

func (r *aiModelConfigRepoFake) InitSchema(context.Context) error { return nil }

func (r *aiModelConfigRepoFake) CreateModelConfig(_ context.Context, item aidomain.ModelConfig) error {
	if strings.TrimSpace(item.ID) == "" {
		return aidomain.ErrModelConfigNotFound
	}
	r.items[item.ID] = item
	return nil
}

func (r *aiModelConfigRepoFake) ListModelConfigs(context.Context) ([]aidomain.ModelConfig, error) {
	items := make([]aidomain.ModelConfig, 0, len(r.items))
	for _, item := range r.items {
		items = append(items, item)
	}
	return items, nil
}

func (r *aiModelConfigRepoFake) GetModelConfigByID(_ context.Context, id string) (aidomain.ModelConfig, error) {
	item, ok := r.items[id]
	if !ok {
		return aidomain.ModelConfig{}, aidomain.ErrModelConfigNotFound
	}
	return item, nil
}

func (r *aiModelConfigRepoFake) UpdateModelConfig(_ context.Context, id string, input aidomain.ModelConfigUpdateInput) (aidomain.ModelConfig, error) {
	item, ok := r.items[id]
	if !ok {
		return aidomain.ModelConfig{}, aidomain.ErrModelConfigNotFound
	}
	item.Name = input.Name
	item.Provider = input.Provider
	item.BaseURL = input.BaseURL
	item.Model = input.Model
	item.APIKeyCipher = input.APIKeyCipher
	item.Temperature = input.Temperature
	item.MaxTokens = input.MaxTokens
	item.TimeoutSec = input.TimeoutSec
	item.Enabled = input.Enabled
	item.UpdatedAt = input.UpdatedAt
	r.items[id] = item
	return item, nil
}

func (r *aiModelConfigRepoFake) DeleteModelConfig(_ context.Context, id string) error {
	delete(r.items, id)
	return nil
}

func (r *aiModelConfigRepoFake) SetDiagnosisModel(_ context.Context, id string, updatedAt time.Time) (aidomain.ModelConfig, error) {
	item, ok := r.items[id]
	if !ok {
		return aidomain.ModelConfig{}, aidomain.ErrModelConfigNotFound
	}
	for key, value := range r.items {
		value.IsDiagnosisModel = false
		value.UpdatedAt = updatedAt
		r.items[key] = value
	}
	item.IsDiagnosisModel = true
	item.UpdatedAt = updatedAt
	r.items[id] = item
	return item, nil
}

func (r *aiModelConfigRepoFake) UnsetDiagnosisModel(_ context.Context, id string, updatedAt time.Time) (aidomain.ModelConfig, error) {
	item, ok := r.items[id]
	if !ok {
		return aidomain.ModelConfig{}, aidomain.ErrModelConfigNotFound
	}
	item.IsDiagnosisModel = false
	item.UpdatedAt = updatedAt
	r.items[id] = item
	return item, nil
}

func (r *aiModelConfigRepoFake) GetDiagnosisModel(context.Context) (aidomain.ModelConfig, error) {
	for _, item := range r.items {
		if item.IsDiagnosisModel && item.Enabled {
			return item, nil
		}
	}
	return aidomain.ModelConfig{}, aidomain.ErrDiagnosisModelNotConfigured
}
