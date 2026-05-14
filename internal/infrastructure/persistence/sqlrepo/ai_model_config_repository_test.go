package sqlrepo

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	aidomain "gos/internal/domain/ai"

	_ "modernc.org/sqlite"
)

func TestAIModelConfigRepositoryStoresConfigsAndSelectsDiagnosisModel(t *testing.T) {
	t.Parallel()

	repo := newTestAIModelConfigRepository(t)
	ctx := context.Background()
	now := time.Unix(1_710_000_000, 0).UTC()

	first := aidomain.ModelConfig{
		ID:           "aimc-1",
		Name:         "primary",
		Provider:     aidomain.ProviderOpenAICompatible,
		BaseURL:      "https://api.one.example/v1",
		Model:        "chat-one",
		APIKeyCipher: "enc:v1:first",
		Temperature:  0.2,
		MaxTokens:    2048,
		TimeoutSec:   60,
		Enabled:      true,
		CreatedBy:    "usr-1",
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	second := first
	second.ID = "aimc-2"
	second.Name = "secondary"
	second.BaseURL = "https://api.two.example/v1"
	second.Model = "chat-two"
	second.APIKeyCipher = "enc:v1:second"

	if err := repo.CreateModelConfig(ctx, first); err != nil {
		t.Fatalf("CreateModelConfig(first) failed: %v", err)
	}
	if err := repo.CreateModelConfig(ctx, second); err != nil {
		t.Fatalf("CreateModelConfig(second) failed: %v", err)
	}

	selected, err := repo.SetDiagnosisModel(ctx, second.ID, now.Add(time.Minute))
	if err != nil {
		t.Fatalf("SetDiagnosisModel(second) failed: %v", err)
	}
	if !selected.IsDiagnosisModel {
		t.Fatalf("selected.IsDiagnosisModel = false, want true")
	}

	diagnosis, err := repo.GetDiagnosisModel(ctx)
	if err != nil {
		t.Fatalf("GetDiagnosisModel failed: %v", err)
	}
	if diagnosis.ID != second.ID {
		t.Fatalf("diagnosis model ID = %q, want %q", diagnosis.ID, second.ID)
	}

	items, err := repo.ListModelConfigs(ctx)
	if err != nil {
		t.Fatalf("ListModelConfigs failed: %v", err)
	}
	var diagnosisCount int
	for _, item := range items {
		if item.IsDiagnosisModel {
			diagnosisCount++
		}
	}
	if diagnosisCount != 1 {
		t.Fatalf("diagnosisCount = %d, want 1; items = %#v", diagnosisCount, items)
	}

	selected, err = repo.SetDiagnosisModel(ctx, first.ID, now.Add(2*time.Minute))
	if err != nil {
		t.Fatalf("SetDiagnosisModel(first) failed: %v", err)
	}
	if selected.ID != first.ID || !selected.IsDiagnosisModel {
		t.Fatalf("selected after switch = %#v", selected)
	}

	diagnosis, err = repo.GetDiagnosisModel(ctx)
	if err != nil {
		t.Fatalf("GetDiagnosisModel after switch failed: %v", err)
	}
	if diagnosis.ID != first.ID {
		t.Fatalf("diagnosis model after switch = %q, want %q", diagnosis.ID, first.ID)
	}

	cleared, err := repo.UnsetDiagnosisModel(ctx, first.ID, now.Add(3*time.Minute))
	if err != nil {
		t.Fatalf("UnsetDiagnosisModel(first) failed: %v", err)
	}
	if cleared.IsDiagnosisModel {
		t.Fatalf("cleared.IsDiagnosisModel = true, want false")
	}
	if _, err := repo.GetDiagnosisModel(ctx); !errors.Is(err, aidomain.ErrDiagnosisModelNotConfigured) {
		t.Fatalf("GetDiagnosisModel after unset err = %v, want ErrDiagnosisModelNotConfigured", err)
	}
}

func newTestAIModelConfigRepository(t *testing.T) *AIModelConfigRepository {
	t.Helper()

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite failed: %v", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	t.Cleanup(func() {
		_ = db.Close()
	})

	repo := NewAIModelConfigRepository(db, "sqlite")
	if err := repo.InitSchema(context.Background()); err != nil {
		t.Fatalf("InitSchema failed: %v", err)
	}
	return repo
}
