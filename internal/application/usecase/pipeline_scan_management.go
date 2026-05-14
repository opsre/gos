package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	pipelinedomain "gos/internal/domain/pipeline"
	scandomain "gos/internal/domain/pipelinescan"
)

var pipelineScanRuleCodePattern = regexp.MustCompile(`^[a-z][a-z0-9_.-]*$`)

type pipelineScanRuleTypeDefinition struct {
	Type string
	Code string
}

var pipelineScanRuleTypeDefinitions = []pipelineScanRuleTypeDefinition{
	{Type: "artifact_oss_command_format_standard", Code: "artifact.oss.command_format.standard"},
	{Type: "artifact_oss_pipeline_params_standard", Code: "artifact.oss.pipeline_params.standard"},
	{Type: "artifact_gos_artifact_url_standard", Code: "artifact.gos.artifact_url.standard"},
}

type PipelineScanManager struct {
	scanRepo              scandomain.Repository
	pipelineRepo          pipelinedomain.Repository
	jenkins               JenkinsPipelineClient
	engine                *PipelineScanEngine
	now                   func() time.Time
	ruleChangeRescan      func()
	ruleChangeScanMu      sync.Mutex
	ruleChangeScanRunning bool
	ruleChangeScanPending bool
	ruleChangeScanTimeout time.Duration
}

type CreatePipelineScanRuleInput struct {
	RuleType                 string              `json:"rule_type"`
	RuleCode                 string              `json:"rule_code"`
	RuleName                 string              `json:"rule_name"`
	Category                 scandomain.Category `json:"category"`
	Severity                 scandomain.Severity `json:"severity"`
	Enabled                  bool                `json:"enabled"`
	TemplateValidationScopes []string            `json:"template_validation_scopes"`
	ScopeJSON                string              `json:"scope_json"`
	RuleDSL                  string              `json:"rule_dsl_json"`
	Message                  string              `json:"message"`
	Suggestion               string              `json:"suggestion"`
}

type UpdatePipelineScanRuleInput struct {
	RuleType                 string              `json:"rule_type"`
	RuleCode                 string              `json:"rule_code"`
	RuleName                 string              `json:"rule_name"`
	Category                 scandomain.Category `json:"category"`
	Severity                 scandomain.Severity `json:"severity"`
	Enabled                  bool                `json:"enabled"`
	TemplateValidationScopes []string            `json:"template_validation_scopes"`
	ScopeJSON                string              `json:"scope_json"`
	RuleDSL                  string              `json:"rule_dsl_json"`
	Message                  string              `json:"message"`
	Suggestion               string              `json:"suggestion"`
}

type ScanPipelinesOutput struct {
	Total   int `json:"total"`
	Scanned int `json:"scanned"`
	Skipped int `json:"skipped"`
	Failed  int `json:"failed"`
}

func NewPipelineScanManager(
	scanRepo scandomain.Repository,
	pipelineRepo pipelinedomain.Repository,
	jenkins JenkinsPipelineClient,
) *PipelineScanManager {
	manager := &PipelineScanManager{
		scanRepo:              scanRepo,
		pipelineRepo:          pipelineRepo,
		jenkins:               jenkins,
		engine:                NewPipelineScanEngine(),
		ruleChangeScanTimeout: 30 * time.Minute,
		now: func() time.Time {
			return time.Now().UTC()
		},
	}
	manager.ruleChangeRescan = manager.scheduleRuleChangeRescan
	return manager
}

func (uc *PipelineScanManager) CreateRule(ctx context.Context, input CreatePipelineScanRuleInput) (scandomain.Rule, error) {
	clean, err := normalizeCreatePipelineScanRuleInput(input)
	if err != nil {
		return scandomain.Rule{}, err
	}
	now := uc.now()
	ruleID := generateID("psr")
	item := scandomain.Rule{
		ID:                       ruleID,
		RuleCode:                 clean.RuleCode,
		RuleName:                 clean.RuleName,
		Category:                 clean.Category,
		Severity:                 clean.Severity,
		Enabled:                  clean.Enabled,
		TemplateValidationScopes: append([]string(nil), clean.TemplateValidationScopes...),
		ScopeJSON:                clean.ScopeJSON,
		RuleDSL:                  clean.RuleDSL,
		Message:                  clean.Message,
		Suggestion:               clean.Suggestion,
		CreatedAt:                now,
		UpdatedAt:                now,
	}
	if err := uc.scanRepo.CreateRule(ctx, item); err != nil {
		if isBackendGeneratedPipelineScanRuleCode(input.RuleType, input.RuleCode) && errors.Is(err, scandomain.ErrRuleDuplicated) {
			item.RuleCode = generatedPipelineScanRuleInstanceCode(clean.RuleCode, item.ID)
			if retryErr := uc.scanRepo.CreateRule(ctx, item); retryErr != nil {
				return scandomain.Rule{}, retryErr
			}
			created, getErr := uc.scanRepo.GetRuleByID(ctx, item.ID)
			if getErr != nil {
				return scandomain.Rule{}, getErr
			}
			uc.requestRuleChangeRescan()
			return created, nil
		}
		return scandomain.Rule{}, err
	}
	created, err := uc.scanRepo.GetRuleByID(ctx, item.ID)
	if err != nil {
		return scandomain.Rule{}, err
	}
	uc.requestRuleChangeRescan()
	return created, nil
}

func (uc *PipelineScanManager) ListRules(ctx context.Context, filter scandomain.RuleListFilter) ([]scandomain.Rule, int64, error) {
	filter.Keyword = strings.TrimSpace(filter.Keyword)
	if filter.Category != "" && !filter.Category.Valid() {
		return nil, 0, ErrInvalidInput
	}
	if filter.Severity != "" && !filter.Severity.Valid() {
		return nil, 0, ErrInvalidInput
	}
	filter.Page, filter.PageSize = normalizePipelineScanPagination(filter.Page, filter.PageSize)
	return uc.scanRepo.ListRules(ctx, filter)
}

func (uc *PipelineScanManager) GetRule(ctx context.Context, id string) (scandomain.Rule, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return scandomain.Rule{}, ErrInvalidID
	}
	return uc.scanRepo.GetRuleByID(ctx, id)
}

func (uc *PipelineScanManager) UpdateRule(ctx context.Context, id string, input UpdatePipelineScanRuleInput) (scandomain.Rule, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return scandomain.Rule{}, ErrInvalidID
	}
	current, err := uc.scanRepo.GetRuleByID(ctx, id)
	if err != nil {
		return scandomain.Rule{}, err
	}
	if current.Builtin {
		return scandomain.Rule{}, fmt.Errorf("%w: 内置管线规范不允许编辑", ErrInvalidInput)
	}
	clean, err := normalizeUpdatePipelineScanRuleInput(input, current.RuleCode)
	if err != nil {
		return scandomain.Rule{}, err
	}
	updated, err := uc.scanRepo.UpdateRule(ctx, id, clean)
	if err != nil {
		return scandomain.Rule{}, err
	}
	uc.requestRuleChangeRescan()
	return updated, nil
}

func (uc *PipelineScanManager) SetRuleEnabled(ctx context.Context, id string, enabled bool) (scandomain.Rule, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return scandomain.Rule{}, ErrInvalidID
	}
	current, err := uc.scanRepo.GetRuleByID(ctx, id)
	if err != nil {
		return scandomain.Rule{}, err
	}
	if current.Enabled == enabled {
		return current, nil
	}
	updated, err := uc.scanRepo.UpdateRule(ctx, id, scandomain.RuleUpdateInput{
		RuleCode:                 current.RuleCode,
		RuleName:                 current.RuleName,
		Category:                 current.Category,
		Severity:                 current.Severity,
		Enabled:                  enabled,
		TemplateValidationScopes: append([]string(nil), current.TemplateValidationScopes...),
		ScopeJSON:                current.ScopeJSON,
		RuleDSL:                  current.RuleDSL,
		Message:                  current.Message,
		Suggestion:               current.Suggestion,
	})
	if err != nil {
		return scandomain.Rule{}, err
	}
	uc.requestRuleChangeRescan()
	return updated, nil
}

func (uc *PipelineScanManager) DeleteRule(ctx context.Context, id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return ErrInvalidID
	}
	current, err := uc.scanRepo.GetRuleByID(ctx, id)
	if err != nil {
		return err
	}
	if current.Builtin {
		return fmt.Errorf("%w: 内置管线规范不允许删除", ErrInvalidInput)
	}
	if err := uc.scanRepo.DeleteRule(ctx, id); err != nil {
		return err
	}
	uc.requestRuleChangeRescan()
	return nil
}

func (uc *PipelineScanManager) requestRuleChangeRescan() {
	if uc == nil || uc.ruleChangeRescan == nil {
		return
	}
	uc.ruleChangeRescan()
}

func (uc *PipelineScanManager) scheduleRuleChangeRescan() {
	uc.ruleChangeScanMu.Lock()
	if uc.ruleChangeScanRunning {
		uc.ruleChangeScanPending = true
		uc.ruleChangeScanMu.Unlock()
		return
	}
	uc.ruleChangeScanRunning = true
	uc.ruleChangeScanMu.Unlock()

	go uc.runRuleChangeRescanLoop()
}

func (uc *PipelineScanManager) runRuleChangeRescanLoop() {
	for {
		timeout := uc.ruleChangeScanTimeout
		if timeout <= 0 {
			timeout = 30 * time.Minute
		}
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		_, _ = uc.ScanActiveJenkinsPipelines(ctx)
		cancel()

		uc.ruleChangeScanMu.Lock()
		if !uc.ruleChangeScanPending {
			uc.ruleChangeScanRunning = false
			uc.ruleChangeScanMu.Unlock()
			return
		}
		uc.ruleChangeScanPending = false
		uc.ruleChangeScanMu.Unlock()
	}
}

func (uc *PipelineScanManager) ScanPipeline(ctx context.Context, id string) (scandomain.Result, []scandomain.Finding, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return scandomain.Result{}, nil, ErrInvalidID
	}

	pipeline, err := uc.pipelineRepo.GetPipelineByID(ctx, id)
	if err != nil {
		return scandomain.Result{}, nil, err
	}
	if pipeline.Provider != pipelinedomain.ProviderJenkins {
		return scandomain.Result{}, nil, fmt.Errorf("%w: only jenkins pipeline scan is supported", ErrInvalidProvider)
	}
	if err := ensureActivePipelineRecord(pipeline, "当前管线"); err != nil {
		return scandomain.Result{}, nil, err
	}
	if strings.TrimSpace(pipeline.JobFullName) == "" {
		return scandomain.Result{}, nil, fmt.Errorf("%w: jenkins job full name is empty", ErrInvalidInput)
	}

	script, err := uc.jenkins.GetPipelineScript(ctx, pipeline.JobFullName)
	if err != nil {
		return scandomain.Result{}, nil, err
	}
	rules, err := uc.scanRepo.ListEnabledRules(ctx)
	if err != nil {
		return scandomain.Result{}, nil, err
	}

	now := uc.now()
	status := scandomain.ScanStatusCompliant
	findings := make([]scandomain.Finding, 0)
	if strings.TrimSpace(script.Script) == "" {
		status = scandomain.ScanStatusUnknown
	} else {
		findings, err = uc.engine.ScanScript(pipeline.ID, pipeline.JobFullName, script.Script, rules)
		if err != nil {
			return scandomain.Result{}, nil, err
		}
		for i := range findings {
			findings[i].ID = generateID("psf")
			findings[i].PipelineID = pipeline.ID
			if findings[i].Status == "" {
				findings[i].Status = scandomain.FindingStatusOpen
			}
			findings[i].CreatedAt = now
			findings[i].UpdatedAt = now
		}
		status = scanStatusFromFindings(findings)
	}

	result := scandomain.Result{
		ID:            generateID("psres"),
		PipelineID:    pipeline.ID,
		PipelineName:  pipeline.JobFullName,
		ScanStatus:    status,
		TotalFindings: len(findings),
		ScriptHash:    PipelineScriptHash(script.Script),
		LastScannedAt: now,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	result.ErrorCount, result.WarningCount, result.InfoCount = countPipelineScanFindings(findings)

	if err := uc.scanRepo.SaveScan(ctx, result, findings); err != nil {
		return scandomain.Result{}, nil, err
	}
	return result, findings, nil
}

func (uc *PipelineScanManager) ScanActiveJenkinsPipelines(ctx context.Context) (ScanPipelinesOutput, error) {
	const pageSize = 100
	output := ScanPipelinesOutput{}
	for page := 1; ; page++ {
		items, _, err := uc.pipelineRepo.ListPipelines(ctx, pipelinedomain.PipelineListFilter{
			Provider: pipelinedomain.ProviderJenkins,
			Status:   pipelinedomain.StatusActive,
			Page:     page,
			PageSize: pageSize,
		})
		if err != nil {
			return output, err
		}
		if len(items) == 0 {
			break
		}
		output.Total += len(items)
		for _, item := range items {
			if _, _, err := uc.ScanPipeline(ctx, item.ID); err != nil {
				output.Failed++
				continue
			}
			output.Scanned++
		}
		if len(items) < pageSize {
			break
		}
	}
	return output, nil
}

func (uc *PipelineScanManager) GetPipelineResult(ctx context.Context, pipelineID string) (scandomain.Result, []scandomain.Finding, error) {
	pipelineID = strings.TrimSpace(pipelineID)
	if pipelineID == "" {
		return scandomain.Result{}, nil, ErrInvalidID
	}
	return uc.scanRepo.GetResultByPipelineID(ctx, pipelineID)
}

func (uc *PipelineScanManager) ListResults(ctx context.Context, filter scandomain.ResultListFilter) ([]scandomain.Result, int64, error) {
	filter.PipelineName = strings.TrimSpace(filter.PipelineName)
	if filter.ScanStatus != "" {
		switch filter.ScanStatus {
		case scandomain.ScanStatusCompliant, scandomain.ScanStatusWarning, scandomain.ScanStatusFailed, scandomain.ScanStatusUnknown:
		default:
			return nil, 0, ErrInvalidStatus
		}
	}
	filter.Page, filter.PageSize = normalizePipelineScanPagination(filter.Page, filter.PageSize)
	return uc.scanRepo.ListResults(ctx, filter)
}

func normalizeCreatePipelineScanRuleInput(input CreatePipelineScanRuleInput) (scandomain.Rule, error) {
	ruleCode, err := resolvePipelineScanRuleCode(input.RuleType, input.RuleCode)
	if err != nil {
		return scandomain.Rule{}, err
	}
	return normalizePipelineScanRuleInput(scandomain.RuleUpdateInput{
		RuleCode:                 ruleCode,
		RuleName:                 input.RuleName,
		Category:                 input.Category,
		Severity:                 input.Severity,
		Enabled:                  input.Enabled,
		TemplateValidationScopes: input.TemplateValidationScopes,
		ScopeJSON:                input.ScopeJSON,
		RuleDSL:                  input.RuleDSL,
		Message:                  input.Message,
		Suggestion:               input.Suggestion,
	})
}

func normalizeUpdatePipelineScanRuleInput(input UpdatePipelineScanRuleInput, currentRuleCode string) (scandomain.RuleUpdateInput, error) {
	ruleCode, err := resolvePipelineScanRuleCodeForUpdate(input.RuleType, input.RuleCode, currentRuleCode)
	if err != nil {
		return scandomain.RuleUpdateInput{}, err
	}
	rule, err := normalizePipelineScanRuleInput(scandomain.RuleUpdateInput{
		RuleCode:                 ruleCode,
		RuleName:                 input.RuleName,
		Category:                 input.Category,
		Severity:                 input.Severity,
		Enabled:                  input.Enabled,
		TemplateValidationScopes: input.TemplateValidationScopes,
		ScopeJSON:                input.ScopeJSON,
		RuleDSL:                  input.RuleDSL,
		Message:                  input.Message,
		Suggestion:               input.Suggestion,
	})
	if err != nil {
		return scandomain.RuleUpdateInput{}, err
	}
	return scandomain.RuleUpdateInput{
		RuleCode:                 rule.RuleCode,
		RuleName:                 rule.RuleName,
		Category:                 rule.Category,
		Severity:                 rule.Severity,
		Enabled:                  rule.Enabled,
		TemplateValidationScopes: append([]string(nil), rule.TemplateValidationScopes...),
		ScopeJSON:                rule.ScopeJSON,
		RuleDSL:                  rule.RuleDSL,
		Message:                  rule.Message,
		Suggestion:               rule.Suggestion,
	}, nil
}

func resolvePipelineScanRuleCode(ruleType string, fallbackCode string) (string, error) {
	normalizedType := strings.TrimSpace(ruleType)
	if normalizedType == "" {
		return strings.TrimSpace(fallbackCode), nil
	}
	for _, definition := range pipelineScanRuleTypeDefinitions {
		if definition.Type == normalizedType {
			return definition.Code, nil
		}
	}
	return "", fmt.Errorf("%w: rule_type is invalid", ErrInvalidInput)
}

func resolvePipelineScanRuleCodeForUpdate(ruleType string, fallbackCode string, currentRuleCode string) (string, error) {
	normalizedType := strings.TrimSpace(ruleType)
	explicitCode := strings.TrimSpace(fallbackCode)
	if normalizedType == "" {
		if explicitCode != "" {
			return explicitCode, nil
		}
		return strings.TrimSpace(currentRuleCode), nil
	}
	if explicitCode != "" {
		return explicitCode, nil
	}
	baseCode, err := resolvePipelineScanRuleCode(normalizedType, "")
	if err != nil {
		return "", err
	}
	currentCode := strings.TrimSpace(currentRuleCode)
	if currentCode == baseCode || strings.HasPrefix(currentCode, baseCode+".") {
		return currentCode, nil
	}
	return baseCode, nil
}

func PipelineScanRuleTypeFromCode(ruleCode string) string {
	normalizedCode := strings.TrimSpace(ruleCode)
	for _, definition := range pipelineScanRuleTypeDefinitions {
		if definition.Code == normalizedCode || strings.HasPrefix(normalizedCode, definition.Code+".") {
			return definition.Type
		}
	}
	return ""
}

func isBackendGeneratedPipelineScanRuleCode(ruleType string, ruleCode string) bool {
	return strings.TrimSpace(ruleType) != "" && strings.TrimSpace(ruleCode) == ""
}

func generatedPipelineScanRuleInstanceCode(baseCode string, ruleID string) string {
	suffix := strings.TrimSpace(strings.ToLower(ruleID))
	suffix = strings.TrimPrefix(suffix, "psr-")
	suffix = regexp.MustCompile(`[^a-z0-9_-]+`).ReplaceAllString(suffix, "-")
	suffix = strings.Trim(suffix, "-_")
	if suffix == "" {
		suffix = "generated"
	}
	return strings.TrimSpace(baseCode) + "." + suffix
}

func normalizePipelineScanRuleInput(input scandomain.RuleUpdateInput) (scandomain.Rule, error) {
	ruleCode := strings.TrimSpace(input.RuleCode)
	if !pipelineScanRuleCodePattern.MatchString(ruleCode) {
		return scandomain.Rule{}, fmt.Errorf("%w: rule_code is invalid", ErrInvalidInput)
	}
	ruleName := strings.TrimSpace(input.RuleName)
	if ruleName == "" {
		return scandomain.Rule{}, fmt.Errorf("%w: rule_name is required", ErrInvalidInput)
	}
	category := input.Category
	if category == "" {
		category = scandomain.CategoryCustom
	}
	if !category.Valid() {
		return scandomain.Rule{}, fmt.Errorf("%w: category is invalid", ErrInvalidInput)
	}
	severity := input.Severity
	if severity == "" {
		severity = scandomain.SeverityWarning
	}
	if !severity.Valid() {
		return scandomain.Rule{}, fmt.Errorf("%w: severity is invalid", ErrInvalidInput)
	}

	scopeJSON := strings.TrimSpace(input.ScopeJSON)
	if scopeJSON == "" {
		scopeJSON = "{}"
	}
	if !json.Valid([]byte(scopeJSON)) {
		return scandomain.Rule{}, fmt.Errorf("%w: scope_json is invalid", ErrInvalidInput)
	}

	ruleDSL := strings.TrimSpace(input.RuleDSL)
	if _, err := parsePipelineRuleDSL(ruleDSL); err != nil {
		return scandomain.Rule{}, fmt.Errorf("%w: rule_dsl_json is invalid: %v", ErrInvalidInput, err)
	}

	message := strings.TrimSpace(input.Message)
	if message == "" {
		message = "管线脚本不符合规范规则"
	}

	templateValidationScopes, err := normalizePipelineScanTemplateValidationScopes(input.TemplateValidationScopes)
	if err != nil {
		return scandomain.Rule{}, err
	}

	return scandomain.Rule{
		RuleCode:                 ruleCode,
		RuleName:                 ruleName,
		Category:                 category,
		Severity:                 severity,
		Enabled:                  input.Enabled,
		TemplateValidationScopes: templateValidationScopes,
		ScopeJSON:                scopeJSON,
		RuleDSL:                  ruleDSL,
		Message:                  message,
		Suggestion:               strings.TrimSpace(input.Suggestion),
	}, nil
}

func normalizePipelineScanTemplateValidationScopes(values []string) ([]string, error) {
	result := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, raw := range values {
		value := strings.ToLower(strings.TrimSpace(raw))
		if value == "" {
			continue
		}
		if value != "ci" && value != "cd" {
			return nil, fmt.Errorf("%w: template_validation_scopes is invalid", ErrInvalidInput)
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result, nil
}

func scanStatusFromFindings(findings []scandomain.Finding) scandomain.ScanStatus {
	errors, warnings, _ := countPipelineScanFindings(findings)
	switch {
	case errors > 0:
		return scandomain.ScanStatusFailed
	case warnings > 0:
		return scandomain.ScanStatusWarning
	default:
		return scandomain.ScanStatusCompliant
	}
}

func countPipelineScanFindings(findings []scandomain.Finding) (int, int, int) {
	errorCount := 0
	warningCount := 0
	infoCount := 0
	for _, finding := range findings {
		switch finding.Severity {
		case scandomain.SeverityError:
			errorCount++
		case scandomain.SeverityWarning:
			warningCount++
		case scandomain.SeverityInfo:
			infoCount++
		}
	}
	return errorCount, warningCount, infoCount
}

func normalizePipelineScanPagination(page, pageSize int) (int, int) {
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	return page, pageSize
}
