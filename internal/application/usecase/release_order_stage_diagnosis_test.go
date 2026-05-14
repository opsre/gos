package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	aidomain "gos/internal/domain/ai"
	pipelinedomain "gos/internal/domain/pipeline"
	domain "gos/internal/domain/release"
)

func TestDiagnosePipelineStageRequiresConfiguredDiagnosisModel(t *testing.T) {
	t.Parallel()

	manager, _ := newReleaseOrderManagerForStageDiagnosisTest(t)
	manager.aiModelRepo = newAIStageDiagnosisModelRepoFake()
	manager.stageDiagnosisRepo = newAIStageDiagnosisRepoFake()
	manager.aiClientFactory = &stageDiagnosisClientFactoryFake{}

	_, err := manager.DiagnosePipelineStage(context.Background(), "ro-missing", "stage-missing", StageDiagnosisInput{})
	if !errors.Is(err, aidomain.ErrDiagnosisModelNotConfigured) {
		t.Fatalf("DiagnosePipelineStage err = %v, want ErrDiagnosisModelNotConfigured", err)
	}
}

func TestDiagnosePipelineStageSanitizesLogAndStoresResult(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	manager, repo := newReleaseOrderManagerForStageDiagnosisTest(t)
	now := time.Unix(1_710_000_000, 0).UTC()
	manager.now = func() time.Time { return now }

	order := testReleaseOrder("ro-ai", "RO-AI", domain.OrderStatusDeployFailed, now)
	execution := testReleaseExecution(order.ID, "exec-ci", domain.PipelineScopeCI, domain.ExecutionStatusFailed, now)
	execution.BuildURL = "https://jenkins.example/job/demo/1/"
	if err := repo.Create(ctx, order, []domain.ReleaseOrderExecution{execution}, nil, nil); err != nil {
		t.Fatalf("Create release order failed: %v", err)
	}
	stage := domain.ReleaseOrderPipelineStage{
		ID:             "stage-build",
		ReleaseOrderID: order.ID,
		ExecutionID:    execution.ID,
		PipelineScope:  "ci",
		ExecutorType:   "jenkins",
		StageKey:       "Build",
		StageName:      "Build",
		Status:         domain.PipelineStageStatusFailed,
		RawStatus:      "FAILED",
		SortNo:         1,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if err := repo.ReplacePipelineStages(ctx, order.ID, []domain.ReleaseOrderPipelineStage{stage}); err != nil {
		t.Fatalf("ReplacePipelineStages failed: %v", err)
	}

	modelRepo := newAIStageDiagnosisModelRepoFake()
	modelRepo.items["aimc-1"] = aidomain.ModelConfig{
		ID:               "aimc-1",
		Name:             "diagnosis",
		Provider:         aidomain.ProviderOpenAICompatible,
		BaseURL:          "https://api.example.com/v1",
		Model:            "chat",
		APIKeyCipher:     "enc:v1:key",
		Enabled:          true,
		IsDiagnosisModel: true,
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	diagnosisRepo := newAIStageDiagnosisRepoFake()
	client := &stageDiagnosisClientFake{
		response: json.RawMessage(`{"summary":"构建失败，凭据已脱敏","severity":"error","confidence":0.9,"root_causes":[],"suggested_actions":[],"related_log_lines":[],"needs_human_review":true}`),
	}
	manager.aiModelRepo = modelRepo
	manager.stageDiagnosisRepo = diagnosisRepo
	manager.aiClientFactory = &stageDiagnosisClientFactoryFake{client: client}
	manager.jenkins = &stageDiagnosisJenkinsFake{
		stageLog: domain.ReleaseOrderPipelineStageLog{
			StageName: "Build",
			RawStatus: "FAILED",
			Content:   "npm install failed\npassword=super-secret\nAuthorization: Bearer token-value\nERROR dependency timeout",
			FetchedAt: now,
		},
	}

	output, err := manager.DiagnosePipelineStage(ctx, order.ID, stage.ID, StageDiagnosisInput{CreatedBy: "usr-1"})
	if err != nil {
		t.Fatalf("DiagnosePipelineStage failed: %v", err)
	}
	if output.Status != string(aidomain.StageDiagnosisStatusSuccess) {
		t.Fatalf("Status = %q, want success", output.Status)
	}
	if output.Result.Summary == "" {
		t.Fatalf("diagnosis summary is empty: %#v", output.Result)
	}
	if len(diagnosisRepo.items) != 1 {
		t.Fatalf("saved diagnosis count = %d, want 1", len(diagnosisRepo.items))
	}
	saved := diagnosisRepo.items[0]
	if strings.Contains(saved.LogExcerpt, "super-secret") || strings.Contains(saved.LogExcerpt, "token-value") {
		t.Fatalf("LogExcerpt was not sanitized: %q", saved.LogExcerpt)
	}
	if client.lastInput.Log.Hash == "" || client.lastInput.Log.Content == "" {
		t.Fatalf("client log metadata = %#v, want hash and sanitized content", client.lastInput.Log)
	}
}

func TestDiagnosePipelineStageNormalizesLooseAIResult(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	manager, repo := newReleaseOrderManagerForStageDiagnosisTest(t)
	now := time.Unix(1_710_000_000, 0).UTC()
	manager.now = func() time.Time { return now }

	order := testReleaseOrder("ro-loose", "RO-LOOSE", domain.OrderStatusDeployFailed, now)
	execution := testReleaseExecution(order.ID, "exec-ci", domain.PipelineScopeCI, domain.ExecutionStatusFailed, now)
	execution.BuildURL = "https://jenkins.example/job/demo/1/"
	if err := repo.Create(ctx, order, []domain.ReleaseOrderExecution{execution}, nil, nil); err != nil {
		t.Fatalf("Create release order failed: %v", err)
	}
	stage := domain.ReleaseOrderPipelineStage{
		ID:             "stage-oss",
		ReleaseOrderID: order.ID,
		ExecutionID:    execution.ID,
		PipelineScope:  "ci",
		ExecutorType:   "jenkins",
		StageKey:       "Upload OSS",
		StageName:      "Upload OSS",
		Status:         domain.PipelineStageStatusFailed,
		RawStatus:      "FAILED",
		SortNo:         1,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if err := repo.ReplacePipelineStages(ctx, order.ID, []domain.ReleaseOrderPipelineStage{stage}); err != nil {
		t.Fatalf("ReplacePipelineStages failed: %v", err)
	}

	modelRepo := newAIStageDiagnosisModelRepoFake()
	modelRepo.items["aimc-1"] = aidomain.ModelConfig{
		ID:               "aimc-1",
		Name:             "diagnosis",
		Provider:         aidomain.ProviderOpenAICompatible,
		BaseURL:          "https://api.example.com/v1",
		Model:            "chat",
		APIKeyCipher:     "enc:v1:key",
		Enabled:          true,
		IsDiagnosisModel: true,
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	diagnosisRepo := newAIStageDiagnosisRepoFake()
	client := &stageDiagnosisClientFake{
		response: json.RawMessage(`{
			"diagnosis": {
				"details": "ossutil cp 命令缺少必需的 --access-key-id 参数。",
				"error_message": "Flag --access-key-id requires argument!",
				"suggestion": "请检查 Jenkins 环境变量或构建脚本中的 OSS 凭证配置。"
			},
			"possible_causes": [
				"ossutil 命令行缺少 --access-key-id 参数",
				"环境变量 ACCESS_KEY_ID 未设置"
			],
			"log_preview": "Flag --access-key-id requires argument!"
		}`),
	}
	manager.aiModelRepo = modelRepo
	manager.stageDiagnosisRepo = diagnosisRepo
	manager.aiClientFactory = &stageDiagnosisClientFactoryFake{client: client}
	manager.jenkins = &stageDiagnosisJenkinsFake{
		stageLog: domain.ReleaseOrderPipelineStageLog{
			StageName: "Upload OSS",
			RawStatus: "FAILED",
			Content:   "ossutil cp app.zip oss://bucket/path\nFlag --access-key-id requires argument!",
			FetchedAt: now,
		},
	}

	output, err := manager.DiagnosePipelineStage(ctx, order.ID, stage.ID, StageDiagnosisInput{})
	if err != nil {
		t.Fatalf("DiagnosePipelineStage failed: %v", err)
	}
	if output.Result.Summary == "" {
		t.Fatalf("Summary is empty after normalization: %#v", output.Result)
	}
	if len(output.Result.RootCauses) != 2 {
		t.Fatalf("RootCauses length = %d, want 2: %#v", len(output.Result.RootCauses), output.Result.RootCauses)
	}
	if len(output.Result.SuggestedActions) != 1 {
		t.Fatalf("SuggestedActions length = %d, want 1: %#v", len(output.Result.SuggestedActions), output.Result.SuggestedActions)
	}
	if len(output.Result.RelatedLogLines) != 1 {
		t.Fatalf("RelatedLogLines length = %d, want 1: %#v", len(output.Result.RelatedLogLines), output.Result.RelatedLogLines)
	}
	if !strings.Contains(diagnosisRepo.items[0].ResultJSON, `"root_causes"`) {
		t.Fatalf("saved ResultJSON was not normalized: %s", diagnosisRepo.items[0].ResultJSON)
	}
}

func TestToStageDiagnosisOutputNormalizesLegacyStoredResult(t *testing.T) {
	t.Parallel()

	output := toStageDiagnosisOutput(aidomain.StageDiagnosis{
		ID:         "diag-legacy",
		ResultJSON: `{"diagnosis":{"details":"命令缺少 OSS access key","suggestion":"补齐 Jenkins OSS 凭证"},"possible_causes":["--access-key-id 为空"],"log_preview":"Flag --access-key-id requires argument!"}`,
		Status:     aidomain.StageDiagnosisStatusSuccess,
		CreatedAt:  time.Unix(1_710_000_000, 0).UTC(),
	})
	if output.Result.Summary == "" {
		t.Fatalf("legacy Summary is empty: %#v", output.Result)
	}
	if len(output.Result.RootCauses) != 1 {
		t.Fatalf("legacy RootCauses length = %d, want 1", len(output.Result.RootCauses))
	}
	if len(output.Result.SuggestedActions) != 1 {
		t.Fatalf("legacy SuggestedActions length = %d, want 1", len(output.Result.SuggestedActions))
	}
	if len(output.Result.RelatedLogLines) != 1 {
		t.Fatalf("legacy RelatedLogLines length = %d, want 1", len(output.Result.RelatedLogLines))
	}
}

func TestFollowUpPipelineStageDiagnosisUsesExistingDiagnosisAndHistory(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	manager, repo := newReleaseOrderManagerForStageDiagnosisTest(t)
	now := time.Unix(1_710_000_000, 0).UTC()
	manager.now = func() time.Time { return now }

	order := testReleaseOrder("ro-follow", "RO-FOLLOW", domain.OrderStatusDeployFailed, now)
	execution := testReleaseExecution(order.ID, "exec-ci", domain.PipelineScopeCI, domain.ExecutionStatusFailed, now)
	execution.BuildURL = "https://jenkins.example/job/demo/1/"
	if err := repo.Create(ctx, order, []domain.ReleaseOrderExecution{execution}, nil, nil); err != nil {
		t.Fatalf("Create release order failed: %v", err)
	}
	stage := domain.ReleaseOrderPipelineStage{
		ID:             "stage-follow",
		ReleaseOrderID: order.ID,
		ExecutionID:    execution.ID,
		PipelineScope:  "ci",
		ExecutorType:   "jenkins",
		StageKey:       "Upload OSS",
		StageName:      "Upload OSS",
		Status:         domain.PipelineStageStatusFailed,
		RawStatus:      "FAILED",
		SortNo:         1,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if err := repo.ReplacePipelineStages(ctx, order.ID, []domain.ReleaseOrderPipelineStage{stage}); err != nil {
		t.Fatalf("ReplacePipelineStages failed: %v", err)
	}

	modelRepo := newAIStageDiagnosisModelRepoFake()
	modelRepo.items["aimc-1"] = aidomain.ModelConfig{
		ID:               "aimc-1",
		Name:             "diagnosis",
		Provider:         aidomain.ProviderOpenAICompatible,
		BaseURL:          "https://api.example.com/v1",
		Model:            "chat",
		APIKeyCipher:     "enc:v1:key",
		Enabled:          true,
		IsDiagnosisModel: true,
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	diagnosisRepo := newAIStageDiagnosisRepoFake()
	diagnosisRepo.items = append(diagnosisRepo.items, aidomain.StageDiagnosis{
		ID:              "diag-follow",
		ReleaseOrderID:  order.ID,
		StageID:         stage.ID,
		ExecutionID:     execution.ID,
		PipelineScope:   "ci",
		ExecutorType:    "jenkins",
		StageName:       "Upload OSS",
		StageStatus:     string(domain.PipelineStageStatusFailed),
		AIModelConfigID: "aimc-1",
		AIModelName:     "diagnosis",
		AIModel:         "chat",
		Status:          aidomain.StageDiagnosisStatusSuccess,
		ResultJSON:      `{"summary":"OSS 上传失败","severity":"error","confidence":0.9,"root_causes":[{"category":"credential","title":"OSS AK 缺失","evidence":"Flag --access-key-id requires argument!","confidence":0.9}],"suggested_actions":[],"related_log_lines":[],"needs_human_review":true}`,
		CreatedAt:       now,
		FinishedAt:      &now,
	})
	client := &stageDiagnosisClientFake{
		response: json.RawMessage(`{"answer":"最短修复步骤：检查 Jenkins 凭证注入，补齐 OSS access key 后重跑。","related_log_lines":[{"line_hint":"错误行","text":"Flag --access-key-id requires argument!"}],"suggested_actions":[{"priority":"high","action":"补齐 OSS access key","owner_hint":"发布/运维"}],"needs_human_review":true}`),
	}
	manager.aiModelRepo = modelRepo
	manager.stageDiagnosisRepo = diagnosisRepo
	manager.aiClientFactory = &stageDiagnosisClientFactoryFake{client: client}
	manager.jenkins = &stageDiagnosisJenkinsFake{
		stageLog: domain.ReleaseOrderPipelineStageLog{
			StageName: "Upload OSS",
			RawStatus: "FAILED",
			Content:   "ossutil cp app.zip oss://bucket/path\nFlag --access-key-id requires argument!",
			FetchedAt: now,
		},
	}

	output, err := manager.FollowUpPipelineStageDiagnosis(ctx, order.ID, stage.ID, "diag-follow", StageDiagnosisFollowUpInput{
		Question: "给我最短修复步骤",
		Messages: []StageDiagnosisFollowUpMessage{
			{Role: "user", Content: "解释这个错误"},
			{Role: "assistant", Content: "OSS access key 缺失。"},
		},
	})
	if err != nil {
		t.Fatalf("FollowUpPipelineStageDiagnosis failed: %v", err)
	}
	if !strings.Contains(output.Answer, "最短修复步骤") {
		t.Fatalf("Answer = %q, want follow-up answer", output.Answer)
	}
	if len(output.RelatedLogLines) != 1 {
		t.Fatalf("RelatedLogLines length = %d, want 1", len(output.RelatedLogLines))
	}
	if client.lastInput.Question != "给我最短修复步骤" {
		t.Fatalf("client question = %q, want prompt", client.lastInput.Question)
	}
	if len(client.lastInput.Conversation) != 2 {
		t.Fatalf("client history length = %d, want 2", len(client.lastInput.Conversation))
	}
	if client.lastInput.Diagnosis == nil || client.lastInput.Diagnosis.Summary == "" {
		t.Fatalf("client diagnosis context = %#v, want existing diagnosis", client.lastInput.Diagnosis)
	}
}

func TestDiagnosePipelineStageReturnsCachedResultForSameLogAndModel(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	manager, repo := newReleaseOrderManagerForStageDiagnosisTest(t)
	now := time.Unix(1_710_000_000, 0).UTC()
	manager.now = func() time.Time { return now }

	order := testReleaseOrder("ro-cache", "RO-CACHE", domain.OrderStatusDeployFailed, now)
	execution := testReleaseExecution(order.ID, "exec-ci", domain.PipelineScopeCI, domain.ExecutionStatusFailed, now)
	execution.BuildURL = "https://jenkins.example/job/demo/1/"
	if err := repo.Create(ctx, order, []domain.ReleaseOrderExecution{execution}, nil, nil); err != nil {
		t.Fatalf("Create release order failed: %v", err)
	}
	stage := domain.ReleaseOrderPipelineStage{
		ID:             "stage-cache",
		ReleaseOrderID: order.ID,
		ExecutionID:    execution.ID,
		PipelineScope:  "ci",
		ExecutorType:   "jenkins",
		StageKey:       "Build",
		StageName:      "Build",
		Status:         domain.PipelineStageStatusFailed,
		RawStatus:      "FAILED",
		SortNo:         1,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if err := repo.ReplacePipelineStages(ctx, order.ID, []domain.ReleaseOrderPipelineStage{stage}); err != nil {
		t.Fatalf("ReplacePipelineStages failed: %v", err)
	}

	modelRepo := newAIStageDiagnosisModelRepoFake()
	modelRepo.items["aimc-1"] = aidomain.ModelConfig{
		ID:               "aimc-1",
		Name:             "diagnosis",
		Provider:         aidomain.ProviderOpenAICompatible,
		BaseURL:          "https://api.example.com/v1",
		Model:            "chat",
		APIKeyCipher:     "enc:v1:key",
		Enabled:          true,
		IsDiagnosisModel: true,
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	diagnosisRepo := newAIStageDiagnosisRepoFake()
	client := &stageDiagnosisClientFake{
		response: json.RawMessage(`{"summary":"第一次诊断","severity":"error","confidence":0.8,"root_causes":[],"suggested_actions":[],"related_log_lines":[],"needs_human_review":true}`),
	}
	manager.aiModelRepo = modelRepo
	manager.stageDiagnosisRepo = diagnosisRepo
	manager.aiClientFactory = &stageDiagnosisClientFactoryFake{client: client}
	manager.jenkins = &stageDiagnosisJenkinsFake{
		stageLog: domain.ReleaseOrderPipelineStageLog{
			StageName: "Build",
			RawStatus: "FAILED",
			Content:   "ERROR dependency timeout",
			FetchedAt: now,
		},
	}

	first, err := manager.DiagnosePipelineStage(ctx, order.ID, stage.ID, StageDiagnosisInput{})
	if err != nil {
		t.Fatalf("first DiagnosePipelineStage failed: %v", err)
	}
	second, err := manager.DiagnosePipelineStage(ctx, order.ID, stage.ID, StageDiagnosisInput{})
	if err != nil {
		t.Fatalf("second DiagnosePipelineStage failed: %v", err)
	}
	if first.ID != second.ID {
		t.Fatalf("cached diagnosis ID = %q, want first ID %q", second.ID, first.ID)
	}
	if client.calls != 1 {
		t.Fatalf("client calls = %d, want 1 due cache hit", client.calls)
	}
}

func newReleaseOrderManagerForStageDiagnosisTest(t *testing.T) (*ReleaseOrderManager, releaseStageDiagnosisReleaseRepo) {
	t.Helper()
	manager, repo := newReleaseOrderManagerForCancelTest(t)
	return manager, repo
}

type releaseStageDiagnosisReleaseRepo interface {
	Create(context.Context, domain.ReleaseOrder, []domain.ReleaseOrderExecution, []domain.ReleaseOrderParam, []domain.ReleaseOrderStep) error
	ReplacePipelineStages(context.Context, string, []domain.ReleaseOrderPipelineStage) error
}

type stageDiagnosisJenkinsFake struct {
	stageLog domain.ReleaseOrderPipelineStageLog
}

func (j *stageDiagnosisJenkinsFake) TriggerBuild(context.Context, string, map[string]string) (string, error) {
	return "", nil
}
func (j *stageDiagnosisJenkinsFake) GetQueueItem(context.Context, string) (string, bool, string, error) {
	return "", false, "", nil
}
func (j *stageDiagnosisJenkinsFake) AbortQueueItem(context.Context, string) error { return nil }
func (j *stageDiagnosisJenkinsFake) AbortBuild(context.Context, string) error     { return nil }
func (j *stageDiagnosisJenkinsFake) GetBuildStages(context.Context, string) ([]domain.ReleaseOrderPipelineStage, error) {
	return nil, nil
}
func (j *stageDiagnosisJenkinsFake) GetBuildStageLog(context.Context, string, string) (domain.ReleaseOrderPipelineStageLog, error) {
	return j.stageLog, nil
}

type stageDiagnosisClientFactoryFake struct {
	client *stageDiagnosisClientFake
}

func (f *stageDiagnosisClientFactoryFake) NewClient(aidomain.ModelConfig) (AIModelClient, error) {
	return f.client, nil
}

type stageDiagnosisClientFake struct {
	response  json.RawMessage
	lastInput AIChatInput
	calls     int
}

func (c *stageDiagnosisClientFake) DiagnoseStageLog(_ context.Context, input AIChatInput) (json.RawMessage, error) {
	c.calls++
	c.lastInput = input
	return c.response, nil
}

type aiStageDiagnosisModelRepoFake struct {
	*aiModelConfigRepoFake
}

func newAIStageDiagnosisModelRepoFake() *aiStageDiagnosisModelRepoFake {
	return &aiStageDiagnosisModelRepoFake{aiModelConfigRepoFake: newAIModelConfigRepoFake()}
}

type aiStageDiagnosisRepoFake struct {
	items []aidomain.StageDiagnosis
}

func newAIStageDiagnosisRepoFake() *aiStageDiagnosisRepoFake {
	return &aiStageDiagnosisRepoFake{}
}

func (r *aiStageDiagnosisRepoFake) InitSchema(context.Context) error { return nil }

func (r *aiStageDiagnosisRepoFake) CreateStageDiagnosis(_ context.Context, item aidomain.StageDiagnosis) error {
	r.items = append(r.items, item)
	return nil
}

func (r *aiStageDiagnosisRepoFake) FindSuccessfulStageDiagnosisByCacheKey(_ context.Context, cache aidomain.StageDiagnosisCacheKey) (aidomain.StageDiagnosis, error) {
	for _, item := range r.items {
		if item.StageID == cache.StageID &&
			item.LogHash == cache.LogHash &&
			item.AIModelConfigID == cache.AIModelConfigID &&
			item.PromptVersion == cache.PromptVersion &&
			item.Status == aidomain.StageDiagnosisStatusSuccess {
			return item, nil
		}
	}
	return aidomain.StageDiagnosis{}, aidomain.ErrStageDiagnosisNotFound
}

func (r *aiStageDiagnosisRepoFake) GetStageDiagnosisByID(_ context.Context, id string) (aidomain.StageDiagnosis, error) {
	for _, item := range r.items {
		if item.ID == id {
			return item, nil
		}
	}
	return aidomain.StageDiagnosis{}, aidomain.ErrStageDiagnosisNotFound
}

func (r *aiStageDiagnosisRepoFake) FindLatestStageDiagnosis(_ context.Context, releaseOrderID string, stageID string) (aidomain.StageDiagnosis, error) {
	for i := len(r.items) - 1; i >= 0; i-- {
		item := r.items[i]
		if item.ReleaseOrderID == releaseOrderID && item.StageID == stageID {
			return item, nil
		}
	}
	return aidomain.StageDiagnosis{}, aidomain.ErrStageDiagnosisNotFound
}

var _ = pipelinedomain.ProviderJenkins
