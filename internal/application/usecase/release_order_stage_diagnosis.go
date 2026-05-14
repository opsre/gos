package usecase

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	aidomain "gos/internal/domain/ai"
	domain "gos/internal/domain/release"
)

const defaultStageDiagnosisPromptVersion = "release_stage_diagnosis_v1"

const stageDiagnosisOutputSchema = `{
  "summary": "一句话说明最可能的失败原因",
  "severity": "error|warning|info",
  "confidence": 0.0,
  "root_causes": [{"category": "credential|network|script|dependency|environment|unknown", "title": "原因标题", "evidence": "依据", "confidence": 0.0}],
  "suggested_actions": [{"priority": "high|medium|low", "action": "处理动作", "owner_hint": "建议处理角色"}],
  "related_log_lines": [{"line_hint": "日志位置或关键词", "text": "相关日志"}],
  "needs_human_review": true
}`

const stageDiagnosisFollowUpOutputSchema = `{
  "answer": "直接回答用户追问，结合已有诊断和阶段日志给出可执行分析",
  "related_log_lines": [{"line_hint": "日志位置或关键词", "text": "相关日志"}],
  "suggested_actions": [{"priority": "high|medium|low", "action": "处理动作", "owner_hint": "建议处理角色"}],
  "needs_human_review": true
}`

type AIModelClientFactory interface {
	NewClient(config aidomain.ModelConfig) (AIModelClient, error)
}

type AIModelClient interface {
	DiagnoseStageLog(ctx context.Context, input AIChatInput) (json.RawMessage, error)
}

type StageDiagnosisInput struct {
	ForceRefresh bool
	CreatedBy    string
}

type AIChatInput struct {
	ReleaseOrder AIChatReleaseOrder       `json:"release_order"`
	Pipeline     AIChatPipeline           `json:"pipeline"`
	Log          AIChatLog                `json:"log"`
	Rules        AIChatRules              `json:"rules"`
	Diagnosis    *StageDiagnosisResult    `json:"diagnosis,omitempty"`
	Question     string                   `json:"question,omitempty"`
	Conversation []StageDiagnosisChatTurn `json:"conversation,omitempty"`
}

type AIChatReleaseOrder struct {
	ID              string `json:"id"`
	OrderNo         string `json:"order_no"`
	ApplicationName string `json:"application_name"`
	EnvCode         string `json:"env_code"`
	OperationType   string `json:"operation_type"`
	TriggerType     string `json:"trigger_type"`
}

type AIChatPipeline struct {
	Scope          string `json:"scope"`
	Provider       string `json:"provider"`
	ExecutionID    string `json:"execution_id"`
	StageID        string `json:"stage_id"`
	StageName      string `json:"stage_name"`
	StageStatus    string `json:"stage_status"`
	RawStatus      string `json:"raw_status"`
	DurationMillis int64  `json:"duration_millis"`
}

type AIChatLog struct {
	Hash       string `json:"hash"`
	TotalChars int    `json:"total_chars"`
	Truncated  bool   `json:"truncated"`
	Strategy   string `json:"strategy"`
	Content    string `json:"content"`
}

type AIChatRules struct {
	Language     string `json:"language"`
	OutputSchema string `json:"output_schema"`
}

type StageDiagnosisResult struct {
	Summary          string                    `json:"summary"`
	Severity         string                    `json:"severity"`
	Confidence       float64                   `json:"confidence"`
	RootCauses       []StageDiagnosisRootCause `json:"root_causes"`
	SuggestedActions []StageDiagnosisAction    `json:"suggested_actions"`
	RelatedLogLines  []StageDiagnosisLogLine   `json:"related_log_lines"`
	NeedsHumanReview bool                      `json:"needs_human_review"`
}

type StageDiagnosisRootCause struct {
	Category   string  `json:"category"`
	Title      string  `json:"title"`
	Evidence   string  `json:"evidence"`
	Confidence float64 `json:"confidence"`
}

type StageDiagnosisAction struct {
	Priority  string `json:"priority"`
	Action    string `json:"action"`
	OwnerHint string `json:"owner_hint"`
}

type StageDiagnosisLogLine struct {
	LineHint string `json:"line_hint"`
	Text     string `json:"text"`
}

type StageDiagnosisChatTurn struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type StageDiagnosisFollowUpInput struct {
	Question  string
	Messages  []StageDiagnosisFollowUpMessage
	CreatedBy string
}

type StageDiagnosisFollowUpMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type StageDiagnosisFollowUpOutput struct {
	Question         string                  `json:"question"`
	Answer           string                  `json:"answer"`
	RelatedLogLines  []StageDiagnosisLogLine `json:"related_log_lines"`
	SuggestedActions []StageDiagnosisAction  `json:"suggested_actions"`
	NeedsHumanReview bool                    `json:"needs_human_review"`
	CreatedAt        time.Time               `json:"created_at"`
}

type stageDiagnosisFollowUpResult struct {
	Answer           string                  `json:"answer"`
	RelatedLogLines  []StageDiagnosisLogLine `json:"related_log_lines"`
	SuggestedActions []StageDiagnosisAction  `json:"suggested_actions"`
	NeedsHumanReview bool                    `json:"needs_human_review"`
}

type StageDiagnosisOutput struct {
	ID              string               `json:"id"`
	ReleaseOrderID  string               `json:"release_order_id"`
	StageID         string               `json:"stage_id"`
	AIModelConfigID string               `json:"ai_model_config_id"`
	AIModelName     string               `json:"ai_model_name"`
	AIModel         string               `json:"ai_model"`
	Status          string               `json:"status"`
	Result          StageDiagnosisResult `json:"result"`
	ErrorMessage    string               `json:"error_message"`
	CreatedAt       time.Time            `json:"created_at"`
	FinishedAt      *time.Time           `json:"finished_at"`
}

func (uc *ReleaseOrderManager) DiagnosePipelineStage(
	ctx context.Context,
	orderID string,
	stageID string,
	input StageDiagnosisInput,
) (StageDiagnosisOutput, error) {
	if uc == nil || uc.aiModelRepo == nil {
		return StageDiagnosisOutput{}, fmt.Errorf("%w: ai model repository is not configured", ErrInvalidInput)
	}
	if uc.stageDiagnosisRepo == nil {
		return StageDiagnosisOutput{}, fmt.Errorf("%w: stage diagnosis repository is not configured", ErrInvalidInput)
	}
	if uc.aiClientFactory == nil {
		return StageDiagnosisOutput{}, fmt.Errorf("%w: ai client factory is not configured", ErrInvalidInput)
	}

	model, err := uc.aiModelRepo.GetDiagnosisModel(ctx)
	if err != nil {
		return StageDiagnosisOutput{}, err
	}
	if !model.Enabled || !model.HasAPIKey() {
		return StageDiagnosisOutput{}, fmt.Errorf("%w: diagnosis model is disabled or missing api key", ErrInvalidInput)
	}

	stage, stageLog, err := uc.GetPipelineStageLog(ctx, orderID, stageID)
	if err != nil {
		return StageDiagnosisOutput{}, err
	}
	order, err := uc.repo.GetByID(ctx, strings.TrimSpace(orderID))
	if err != nil {
		return StageDiagnosisOutput{}, err
	}

	sanitized := sanitizeStageDiagnosisLog(stageLog.Content)
	logHash := hashStageDiagnosisLog(sanitized)
	excerpt, truncated := extractStageDiagnosisLogExcerpt(sanitized, 60000, 1200)
	if !input.ForceRefresh {
		cached, cacheErr := uc.stageDiagnosisRepo.FindSuccessfulStageDiagnosisByCacheKey(ctx, aidomain.StageDiagnosisCacheKey{
			StageID:         stage.ID,
			LogHash:         logHash,
			AIModelConfigID: model.ID,
			PromptVersion:   defaultStageDiagnosisPromptVersion,
		})
		if cacheErr == nil {
			return toStageDiagnosisOutput(cached), nil
		}
		if cacheErr != nil && cacheErr != aidomain.ErrStageDiagnosisNotFound {
			return StageDiagnosisOutput{}, cacheErr
		}
	}

	chatInput := buildStageDiagnosisChatInput(order, stage, stageLog, logHash, sanitized, excerpt, truncated)
	client, err := uc.aiClientFactory.NewClient(model)
	if err != nil {
		return StageDiagnosisOutput{}, err
	}
	if client == nil {
		return StageDiagnosisOutput{}, fmt.Errorf("%w: ai client is not configured", ErrInvalidInput)
	}

	now := uc.now()
	rawResult, err := client.DiagnoseStageLog(ctx, chatInput)
	finishedAt := uc.now()
	item := aidomain.StageDiagnosis{
		ID:              generateID("diag"),
		ReleaseOrderID:  order.ID,
		StageID:         stage.ID,
		ExecutionID:     stage.ExecutionID,
		PipelineScope:   stage.PipelineScope,
		ExecutorType:    stage.ExecutorType,
		StageName:       stage.StageName,
		StageStatus:     string(stage.Status),
		AIModelConfigID: model.ID,
		AIModelName:     model.Name,
		AIModel:         model.Model,
		PromptVersion:   defaultStageDiagnosisPromptVersion,
		LogHash:         logHash,
		LogExcerpt:      excerpt,
		CreatedBy:       strings.TrimSpace(input.CreatedBy),
		CreatedAt:       now,
		FinishedAt:      &finishedAt,
	}
	if err != nil {
		item.Status = aidomain.StageDiagnosisStatusFailed
		item.ErrorMessage = err.Error()
		_ = uc.stageDiagnosisRepo.CreateStageDiagnosis(ctx, item)
		return StageDiagnosisOutput{}, err
	}
	_, normalizedResult, parseErr := normalizeStageDiagnosisResult(rawResult)
	if parseErr != nil {
		item.Status = aidomain.StageDiagnosisStatusFailed
		item.ErrorMessage = "AI 输出不是合法 JSON: " + parseErr.Error()
		_ = uc.stageDiagnosisRepo.CreateStageDiagnosis(ctx, item)
		return StageDiagnosisOutput{}, fmt.Errorf("%w: invalid ai diagnosis json", ErrInvalidInput)
	}
	item.Status = aidomain.StageDiagnosisStatusSuccess
	item.ResultJSON = string(normalizedResult)
	if err := uc.stageDiagnosisRepo.CreateStageDiagnosis(ctx, item); err != nil {
		return StageDiagnosisOutput{}, err
	}
	return toStageDiagnosisOutput(item), nil
}

func (uc *ReleaseOrderManager) GetLatestPipelineStageDiagnosis(
	ctx context.Context,
	orderID string,
	stageID string,
) (StageDiagnosisOutput, error) {
	if uc == nil || uc.stageDiagnosisRepo == nil {
		return StageDiagnosisOutput{}, fmt.Errorf("%w: stage diagnosis repository is not configured", ErrInvalidInput)
	}
	item, err := uc.stageDiagnosisRepo.FindLatestStageDiagnosis(ctx, strings.TrimSpace(orderID), strings.TrimSpace(stageID))
	if err != nil {
		return StageDiagnosisOutput{}, err
	}
	return toStageDiagnosisOutput(item), nil
}

func (uc *ReleaseOrderManager) GetPipelineStageDiagnosisByID(
	ctx context.Context,
	orderID string,
	diagnosisID string,
) (StageDiagnosisOutput, error) {
	if uc == nil || uc.stageDiagnosisRepo == nil {
		return StageDiagnosisOutput{}, fmt.Errorf("%w: stage diagnosis repository is not configured", ErrInvalidInput)
	}
	item, err := uc.stageDiagnosisRepo.GetStageDiagnosisByID(ctx, strings.TrimSpace(diagnosisID))
	if err != nil {
		return StageDiagnosisOutput{}, err
	}
	if strings.TrimSpace(item.ReleaseOrderID) != strings.TrimSpace(orderID) {
		return StageDiagnosisOutput{}, aidomain.ErrStageDiagnosisNotFound
	}
	return toStageDiagnosisOutput(item), nil
}

func (uc *ReleaseOrderManager) FollowUpPipelineStageDiagnosis(
	ctx context.Context,
	orderID string,
	stageID string,
	diagnosisID string,
	input StageDiagnosisFollowUpInput,
) (StageDiagnosisFollowUpOutput, error) {
	if uc == nil || uc.aiModelRepo == nil {
		return StageDiagnosisFollowUpOutput{}, fmt.Errorf("%w: ai model repository is not configured", ErrInvalidInput)
	}
	if uc.stageDiagnosisRepo == nil {
		return StageDiagnosisFollowUpOutput{}, fmt.Errorf("%w: stage diagnosis repository is not configured", ErrInvalidInput)
	}
	if uc.aiClientFactory == nil {
		return StageDiagnosisFollowUpOutput{}, fmt.Errorf("%w: ai client factory is not configured", ErrInvalidInput)
	}
	question := strings.TrimSpace(input.Question)
	if question == "" {
		return StageDiagnosisFollowUpOutput{}, fmt.Errorf("%w: follow-up question is required", ErrInvalidInput)
	}
	if len([]rune(question)) > 1000 {
		return StageDiagnosisFollowUpOutput{}, fmt.Errorf("%w: follow-up question is too long", ErrInvalidInput)
	}

	model, err := uc.aiModelRepo.GetDiagnosisModel(ctx)
	if err != nil {
		return StageDiagnosisFollowUpOutput{}, err
	}
	if !model.Enabled || !model.HasAPIKey() {
		return StageDiagnosisFollowUpOutput{}, fmt.Errorf("%w: diagnosis model is disabled or missing api key", ErrInvalidInput)
	}
	diagnosis, err := uc.stageDiagnosisRepo.GetStageDiagnosisByID(ctx, strings.TrimSpace(diagnosisID))
	if err != nil {
		return StageDiagnosisFollowUpOutput{}, err
	}
	if strings.TrimSpace(diagnosis.ReleaseOrderID) != strings.TrimSpace(orderID) ||
		strings.TrimSpace(diagnosis.StageID) != strings.TrimSpace(stageID) {
		return StageDiagnosisFollowUpOutput{}, aidomain.ErrStageDiagnosisNotFound
	}
	order, err := uc.repo.GetByID(ctx, strings.TrimSpace(orderID))
	if err != nil {
		return StageDiagnosisFollowUpOutput{}, err
	}
	stage, stageLog, err := uc.GetPipelineStageLog(ctx, order.ID, diagnosis.StageID)
	if err != nil {
		return StageDiagnosisFollowUpOutput{}, err
	}

	sanitized := sanitizeStageDiagnosisLog(stageLog.Content)
	logHash := hashStageDiagnosisLog(sanitized)
	excerpt, truncated := extractStageDiagnosisLogExcerpt(sanitized, 60000, 1200)
	diagnosisOutput := toStageDiagnosisOutput(diagnosis)
	chatInput := buildStageDiagnosisChatInput(order, stage, stageLog, logHash, sanitized, excerpt, truncated)
	chatInput.Rules.OutputSchema = stageDiagnosisFollowUpOutputSchema
	chatInput.Diagnosis = &diagnosisOutput.Result
	chatInput.Question = question
	chatInput.Conversation = normalizeStageDiagnosisConversation(input.Messages)

	client, err := uc.aiClientFactory.NewClient(model)
	if err != nil {
		return StageDiagnosisFollowUpOutput{}, err
	}
	if client == nil {
		return StageDiagnosisFollowUpOutput{}, fmt.Errorf("%w: ai client is not configured", ErrInvalidInput)
	}
	rawResult, err := client.DiagnoseStageLog(ctx, chatInput)
	if err != nil {
		return StageDiagnosisFollowUpOutput{}, err
	}
	result, err := normalizeStageDiagnosisFollowUpResult(rawResult)
	if err != nil {
		return StageDiagnosisFollowUpOutput{}, fmt.Errorf("%w: invalid ai follow-up json", ErrInvalidInput)
	}
	return StageDiagnosisFollowUpOutput{
		Question:         question,
		Answer:           result.Answer,
		RelatedLogLines:  result.RelatedLogLines,
		SuggestedActions: result.SuggestedActions,
		NeedsHumanReview: result.NeedsHumanReview,
		CreatedAt:        uc.now(),
	}, nil
}

func buildStageDiagnosisChatInput(
	order domain.ReleaseOrder,
	stage domain.ReleaseOrderPipelineStage,
	stageLog domain.ReleaseOrderPipelineStageLog,
	logHash string,
	sanitizedLog string,
	excerpt string,
	truncated bool,
) AIChatInput {
	return AIChatInput{
		ReleaseOrder: AIChatReleaseOrder{
			ID:              order.ID,
			OrderNo:         order.OrderNo,
			ApplicationName: order.ApplicationName,
			EnvCode:         order.EnvCode,
			OperationType:   string(order.OperationType),
			TriggerType:     string(order.TriggerType),
		},
		Pipeline: AIChatPipeline{
			Scope:          stage.PipelineScope,
			Provider:       stage.ExecutorType,
			ExecutionID:    stage.ExecutionID,
			StageID:        stage.ID,
			StageName:      firstNonEmpty(stageLog.StageName, stage.StageName),
			StageStatus:    string(stage.Status),
			RawStatus:      firstNonEmpty(stageLog.RawStatus, stage.RawStatus),
			DurationMillis: stage.DurationMillis,
		},
		Log: AIChatLog{
			Hash:       logHash,
			TotalChars: len([]rune(sanitizedLog)),
			Truncated:  truncated,
			Strategy:   "error_context_and_tail",
			Content:    excerpt,
		},
		Rules: AIChatRules{
			Language:     "zh-CN",
			OutputSchema: stageDiagnosisOutputSchema,
		},
	}
}

func sanitizeStageDiagnosisLog(raw string) string {
	lines := strings.Split(raw, "\n")
	for i, line := range lines {
		lower := strings.ToLower(line)
		if strings.Contains(lower, "password") ||
			strings.Contains(lower, "token") ||
			strings.Contains(lower, "secret") ||
			strings.Contains(lower, "authorization") ||
			strings.Contains(lower, "cookie") ||
			strings.Contains(lower, "api_key") ||
			strings.Contains(lower, "apikey") {
			lines[i] = maskSensitiveLogLine(line)
		}
	}
	return strings.Join(lines, "\n")
}

func maskSensitiveLogLine(line string) string {
	if idx := strings.Index(line, "="); idx >= 0 {
		return line[:idx+1] + "***"
	}
	if idx := strings.Index(line, ":"); idx >= 0 {
		return line[:idx+1] + " ***"
	}
	return "[REDACTED]"
}

func hashStageDiagnosisLog(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func extractStageDiagnosisLogExcerpt(raw string, maxChars int, maxTailLines int) (string, bool) {
	lines := strings.Split(raw, "\n")
	truncated := false
	if maxTailLines > 0 && len(lines) > maxTailLines {
		lines = lines[len(lines)-maxTailLines:]
		truncated = true
	}
	excerpt := strings.Join(lines, "\n")
	runes := []rune(excerpt)
	if maxChars > 0 && len(runes) > maxChars {
		excerpt = string(runes[len(runes)-maxChars:])
		truncated = true
	}
	return excerpt, truncated
}

func toStageDiagnosisOutput(item aidomain.StageDiagnosis) StageDiagnosisOutput {
	var parsed StageDiagnosisResult
	if strings.TrimSpace(item.ResultJSON) != "" {
		normalized, _, err := normalizeStageDiagnosisResult([]byte(item.ResultJSON))
		if err == nil {
			parsed = normalized
		}
	}
	return StageDiagnosisOutput{
		ID:              item.ID,
		ReleaseOrderID:  item.ReleaseOrderID,
		StageID:         item.StageID,
		AIModelConfigID: item.AIModelConfigID,
		AIModelName:     item.AIModelName,
		AIModel:         item.AIModel,
		Status:          string(item.Status),
		Result:          parsed,
		ErrorMessage:    item.ErrorMessage,
		CreatedAt:       item.CreatedAt,
		FinishedAt:      item.FinishedAt,
	}
}

func normalizeStageDiagnosisResult(raw json.RawMessage) (StageDiagnosisResult, json.RawMessage, error) {
	var root map[string]any
	if err := json.Unmarshal(raw, &root); err != nil {
		return StageDiagnosisResult{}, nil, err
	}
	var result StageDiagnosisResult
	if err := json.Unmarshal(raw, &result); err != nil {
		return StageDiagnosisResult{}, nil, err
	}

	diagnosis := objectValue(root, "diagnosis")
	result.Summary = firstNonEmpty(
		result.Summary,
		stringValue(root, "summary"),
		stringValue(root, "details"),
		stringValue(root, "detail"),
		stringValue(root, "error_message"),
		stringValue(root, "message"),
		stringValue(diagnosis, "summary"),
		stringValue(diagnosis, "details"),
		stringValue(diagnosis, "detail"),
		stringValue(diagnosis, "error_message"),
		stringValue(diagnosis, "message"),
	)
	result.Severity = firstNonEmpty(
		result.Severity,
		stringValue(root, "severity"),
		stringValue(diagnosis, "severity"),
		severityFromStatus(firstNonEmpty(stringValue(root, "stage_status"), stringValue(root, "status"), stringValue(diagnosis, "status"))),
	)
	if result.Severity == "" {
		result.Severity = "warning"
	}
	result.Confidence = firstPositiveFloat(
		result.Confidence,
		floatValue(root, "confidence"),
		floatValue(diagnosis, "confidence"),
	)
	if result.Confidence <= 0 {
		result.Confidence = 0.6
	}
	if len(result.RootCauses) == 0 {
		result.RootCauses = looseRootCauses(root, diagnosis)
	}
	if len(result.SuggestedActions) == 0 {
		result.SuggestedActions = looseSuggestedActions(root, diagnosis)
	}
	if len(result.RelatedLogLines) == 0 {
		result.RelatedLogLines = looseRelatedLogLines(root, diagnosis)
	}
	if len(result.RelatedLogLines) == 0 {
		result.RelatedLogLines = relatedLogLinesFromRootCauses(result.RootCauses)
	}
	if value, ok := boolValue(root, "needs_human_review"); ok {
		result.NeedsHumanReview = value
	} else if value, ok := boolValue(diagnosis, "needs_human_review"); ok {
		result.NeedsHumanReview = value
	} else {
		result.NeedsHumanReview = true
	}

	if result.Summary == "" && len(result.RootCauses) > 0 {
		result.Summary = result.RootCauses[0].Title
	}
	if result.Summary == "" && len(result.RelatedLogLines) > 0 {
		result.Summary = result.RelatedLogLines[0].Text
	}
	if result.Summary == "" {
		result.Summary = "AI 已返回诊断结果，但未提供明确结论"
		result.NeedsHumanReview = true
	}
	normalized, err := json.Marshal(result)
	if err != nil {
		return StageDiagnosisResult{}, nil, err
	}
	return result, normalized, nil
}

func normalizeStageDiagnosisFollowUpResult(raw json.RawMessage) (stageDiagnosisFollowUpResult, error) {
	var root map[string]any
	if err := json.Unmarshal(raw, &root); err != nil {
		return stageDiagnosisFollowUpResult{}, err
	}
	var result stageDiagnosisFollowUpResult
	if err := json.Unmarshal(raw, &result); err != nil {
		return stageDiagnosisFollowUpResult{}, err
	}
	diagnosis := objectValue(root, "diagnosis")
	result.Answer = firstNonEmpty(
		result.Answer,
		stringValue(root, "answer"),
		stringValue(root, "summary"),
		stringValue(root, "details"),
		stringValue(root, "message"),
		stringValue(diagnosis, "answer"),
		stringValue(diagnosis, "summary"),
		stringValue(diagnosis, "details"),
		stringValue(diagnosis, "message"),
	)
	if len(result.RelatedLogLines) == 0 {
		result.RelatedLogLines = looseRelatedLogLines(root, diagnosis)
	}
	if len(result.SuggestedActions) == 0 {
		result.SuggestedActions = looseSuggestedActions(root, diagnosis)
	}
	if value, ok := boolValue(root, "needs_human_review"); ok {
		result.NeedsHumanReview = value
	} else if value, ok := boolValue(diagnosis, "needs_human_review"); ok {
		result.NeedsHumanReview = value
	}
	if result.Answer == "" {
		return stageDiagnosisFollowUpResult{}, fmt.Errorf("answer is required")
	}
	return result, nil
}

func normalizeStageDiagnosisConversation(messages []StageDiagnosisFollowUpMessage) []StageDiagnosisChatTurn {
	if len(messages) == 0 {
		return nil
	}
	start := 0
	if len(messages) > 10 {
		start = len(messages) - 10
	}
	turns := make([]StageDiagnosisChatTurn, 0, len(messages)-start)
	for _, message := range messages[start:] {
		role := strings.ToLower(strings.TrimSpace(message.Role))
		if role != "user" && role != "assistant" {
			continue
		}
		content := strings.TrimSpace(message.Content)
		if content == "" {
			continue
		}
		runes := []rune(content)
		if len(runes) > 4000 {
			content = string(runes[:4000])
		}
		turns = append(turns, StageDiagnosisChatTurn{Role: role, Content: content})
	}
	return turns
}

func relatedLogLinesFromRootCauses(causes []StageDiagnosisRootCause) []StageDiagnosisLogLine {
	items := make([]StageDiagnosisLogLine, 0, len(causes))
	seen := make(map[string]struct{}, len(causes))
	for _, cause := range causes {
		text := strings.TrimSpace(cause.Evidence)
		if text == "" {
			continue
		}
		if _, ok := seen[text]; ok {
			continue
		}
		seen[text] = struct{}{}
		items = append(items, StageDiagnosisLogLine{
			LineHint: firstNonEmpty(cause.Title, cause.Category, "原因证据"),
			Text:     text,
		})
	}
	return items
}

func looseRootCauses(root map[string]any, diagnosis map[string]any) []StageDiagnosisRootCause {
	values := firstArrayValue(root, diagnosis, "root_causes", "possible_causes", "causes")
	items := make([]StageDiagnosisRootCause, 0, len(values))
	for _, value := range values {
		switch typed := value.(type) {
		case string:
			title := strings.TrimSpace(typed)
			if title != "" {
				items = append(items, StageDiagnosisRootCause{
					Category:   "pipeline",
					Title:      title,
					Evidence:   firstNonEmpty(stringValue(root, "error_message"), stringValue(diagnosis, "error_message")),
					Confidence: 0.6,
				})
			}
		case map[string]any:
			title := firstNonEmpty(stringValue(typed, "title"), stringValue(typed, "cause"), stringValue(typed, "reason"), stringValue(typed, "message"))
			if title != "" {
				items = append(items, StageDiagnosisRootCause{
					Category:   firstNonEmpty(stringValue(typed, "category"), "pipeline"),
					Title:      title,
					Evidence:   firstNonEmpty(stringValue(typed, "evidence"), stringValue(typed, "detail"), stringValue(typed, "details")),
					Confidence: firstPositiveFloat(floatValue(typed, "confidence"), 0.6),
				})
			}
		}
	}
	if len(items) == 0 {
		errorMessage := firstNonEmpty(stringValue(root, "error_message"), stringValue(diagnosis, "error_message"))
		if errorMessage != "" {
			items = append(items, StageDiagnosisRootCause{
				Category:   "pipeline",
				Title:      errorMessage,
				Evidence:   firstNonEmpty(stringValue(root, "details"), stringValue(diagnosis, "details")),
				Confidence: 0.6,
			})
		}
	}
	return items
}

func looseSuggestedActions(root map[string]any, diagnosis map[string]any) []StageDiagnosisAction {
	values := firstArrayValue(root, diagnosis, "suggested_actions", "actions", "suggestions")
	items := make([]StageDiagnosisAction, 0, len(values)+2)
	for _, value := range values {
		switch typed := value.(type) {
		case string:
			action := strings.TrimSpace(typed)
			if action != "" {
				items = append(items, StageDiagnosisAction{Priority: "high", Action: action, OwnerHint: "发布/运维"})
			}
		case map[string]any:
			action := firstNonEmpty(stringValue(typed, "action"), stringValue(typed, "suggestion"), stringValue(typed, "message"))
			if action != "" {
				items = append(items, StageDiagnosisAction{
					Priority:  firstNonEmpty(stringValue(typed, "priority"), "high"),
					Action:    action,
					OwnerHint: firstNonEmpty(stringValue(typed, "owner_hint"), stringValue(typed, "owner"), "发布/运维"),
				})
			}
		}
	}
	for _, action := range []string{
		stringValue(root, "suggestion"),
		stringValue(root, "suggested_action"),
		stringValue(diagnosis, "suggestion"),
		stringValue(diagnosis, "suggested_action"),
	} {
		action = strings.TrimSpace(action)
		if action != "" {
			items = append(items, StageDiagnosisAction{Priority: "high", Action: action, OwnerHint: "发布/运维"})
		}
	}
	return items
}

func looseRelatedLogLines(root map[string]any, diagnosis map[string]any) []StageDiagnosisLogLine {
	values := firstArrayValue(root, diagnosis, "related_log_lines", "log_lines")
	items := make([]StageDiagnosisLogLine, 0, len(values)+2)
	for _, value := range values {
		switch typed := value.(type) {
		case string:
			text := strings.TrimSpace(typed)
			if text != "" {
				items = append(items, StageDiagnosisLogLine{LineHint: "相关日志", Text: text})
			}
		case map[string]any:
			text := firstNonEmpty(stringValue(typed, "text"), stringValue(typed, "line"), stringValue(typed, "message"))
			if text != "" {
				items = append(items, StageDiagnosisLogLine{
					LineHint: firstNonEmpty(stringValue(typed, "line_hint"), stringValue(typed, "hint"), "相关日志"),
					Text:     text,
				})
			}
		}
	}
	for _, text := range []string{
		stringValue(root, "log_preview"),
		stringValue(root, "log_excerpt"),
		stringValue(diagnosis, "log_preview"),
		stringValue(diagnosis, "log_excerpt"),
	} {
		text = strings.TrimSpace(text)
		if text != "" {
			items = append(items, StageDiagnosisLogLine{LineHint: "相关日志", Text: text})
		}
	}
	return items
}

func firstArrayValue(root map[string]any, diagnosis map[string]any, keys ...string) []any {
	for _, scope := range []map[string]any{root, diagnosis} {
		for _, key := range keys {
			if values, ok := scope[key].([]any); ok {
				return values
			}
		}
	}
	return nil
}

func objectValue(values map[string]any, key string) map[string]any {
	if values == nil {
		return nil
	}
	if item, ok := values[key].(map[string]any); ok {
		return item
	}
	return nil
}

func stringValue(values map[string]any, key string) string {
	if values == nil {
		return ""
	}
	switch value := values[key].(type) {
	case string:
		return strings.TrimSpace(value)
	case json.Number:
		return strings.TrimSpace(value.String())
	case float64:
		return strings.TrimSpace(fmt.Sprintf("%g", value))
	case bool:
		if value {
			return "true"
		}
		return "false"
	default:
		return ""
	}
}

func floatValue(values map[string]any, key string) float64 {
	if values == nil {
		return 0
	}
	switch value := values[key].(type) {
	case float64:
		return value
	case int:
		return float64(value)
	case json.Number:
		parsed, _ := value.Float64()
		return parsed
	default:
		return 0
	}
}

func boolValue(values map[string]any, key string) (bool, bool) {
	if values == nil {
		return false, false
	}
	value, ok := values[key].(bool)
	return value, ok
}

func firstPositiveFloat(values ...float64) float64 {
	for _, value := range values {
		if value > 0 {
			return value
		}
	}
	return 0
}

func severityFromStatus(status string) string {
	status = strings.ToLower(strings.TrimSpace(status))
	if strings.Contains(status, "fail") || strings.Contains(status, "error") {
		return "error"
	}
	if strings.Contains(status, "warn") {
		return "warning"
	}
	return ""
}
