package usecase

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	pipelinedomain "gos/internal/domain/pipeline"
	scandomain "gos/internal/domain/pipelinescan"
)

func TestPipelineScanManagerScanPipelineStoresFindings(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	now := time.Unix(1_710_000_000, 0).UTC()
	pipeline := pipelinedomain.Pipeline{
		ID:          "pln-1",
		Provider:    pipelinedomain.ProviderJenkins,
		JobFullName: "folder/demo",
		JobName:     "demo",
		Status:      pipelinedomain.StatusActive,
	}
	pipelineRepo := &pipelineScanPipelineRepoFake{
		pipelines: map[string]pipelinedomain.Pipeline{pipeline.ID: pipeline},
	}
	scanRepo := &pipelineScanRepoFake{
		rules: []scandomain.Rule{
			{
				ID:         "psr-acl",
				RuleCode:   "artifact.oss.acl.required",
				RuleName:   "OSS 上传必须设置 ACL",
				Category:   scandomain.CategoryArtifact,
				Severity:   scandomain.SeverityWarning,
				Enabled:    true,
				ScopeJSON:  "{}",
				RuleDSL:    `{"matcher":{"type":"command_block","start_pattern":"ossutil\\s+cp","required_patterns":["--acl\\s+public-read"],"forbidden_patterns":["public-read-write"],"max_lines":8}}`,
				Message:    "发现 OSS 上传命令未设置对象 ACL",
				Suggestion: "增加 --acl public-read",
				CreatedAt:  now,
				UpdatedAt:  now,
			},
		},
	}
	jenkins := &pipelineScanJenkinsFake{
		scripts: map[string]pipelinedomain.JenkinsPipelineScript{
			"folder/demo": {
				Script: `pipeline {
  stages {
    stage('Upload OSS') {
      steps {
        sh """
ossutil cp "${WORKSPACE}/notarybusiness.zip" "oss://${OSS_BUCKET}/${OSS_DIR}/notarybusiness-${BUILD_NUMBER}.zip" --endpoint "${OSS_ENDPOINT}" -f
"""
      }
    }
  }
}`,
			},
		},
	}

	manager := NewPipelineScanManager(scanRepo, pipelineRepo, jenkins)
	manager.now = func() time.Time { return now }

	result, findings, err := manager.ScanPipeline(ctx, "pln-1")
	if err != nil {
		t.Fatalf("ScanPipeline failed: %v", err)
	}
	if result.ScanStatus != scandomain.ScanStatusWarning {
		t.Fatalf("ScanStatus = %q, want warning", result.ScanStatus)
	}
	if result.TotalFindings != 1 || result.WarningCount != 1 {
		t.Fatalf("result counts = %#v, want one warning", result)
	}
	if len(findings) != 1 {
		t.Fatalf("len(findings) = %d, want 1", len(findings))
	}
	if findings[0].ID == "" {
		t.Fatalf("finding ID was not assigned")
	}
	if findings[0].RuleCode != "artifact.oss.acl.required" {
		t.Fatalf("RuleCode = %q", findings[0].RuleCode)
	}

	savedResult, savedFindings, err := scanRepo.GetResultByPipelineID(ctx, "pln-1")
	if err != nil {
		t.Fatalf("saved result missing: %v", err)
	}
	if savedResult.ScriptHash != PipelineScriptHash(jenkins.scripts["folder/demo"].Script) {
		t.Fatalf("ScriptHash = %q", savedResult.ScriptHash)
	}
	if len(savedFindings) != 1 {
		t.Fatalf("len(savedFindings) = %d, want 1", len(savedFindings))
	}
}

func TestPipelineScanManagerCreateRuleGeneratesCodeFromRuleType(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	scanRepo := &pipelineScanRepoFake{}
	manager := NewPipelineScanManager(scanRepo, &pipelineScanPipelineRepoFake{}, &pipelineScanJenkinsFake{})

	rule, err := manager.CreateRule(ctx, CreatePipelineScanRuleInput{
		RuleType:   "artifact_oss_command_format_standard",
		RuleName:   "对象存储上传命令排版规范",
		Category:   scandomain.CategoryArtifact,
		Severity:   scandomain.SeverityWarning,
		Enabled:    true,
		ScopeJSON:  "{}",
		RuleDSL:    `{"matcher":{"type":"contains","pattern":"ossutil cp"}}`,
		Message:    "对象存储上传命令排版不符合规范",
		Suggestion: "请按完整命令模板调整 Jenkinsfile",
	})
	if err != nil {
		t.Fatalf("CreateRule failed: %v", err)
	}
	if rule.RuleCode != "artifact.oss.command_format.standard" {
		t.Fatalf("RuleCode = %q, want generated code", rule.RuleCode)
	}
}

func TestPipelineScanManagerCreateRuleGeneratesGOSArtifactURLCodeFromRuleType(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	scanRepo := &pipelineScanRepoFake{}
	manager := NewPipelineScanManager(scanRepo, &pipelineScanPipelineRepoFake{}, &pipelineScanJenkinsFake{})

	rule, err := manager.CreateRule(ctx, CreatePipelineScanRuleInput{
		RuleType:   "artifact_gos_artifact_url_standard",
		RuleName:   "GOS 制品地址输出规范",
		Category:   scandomain.CategoryArtifact,
		Severity:   scandomain.SeverityError,
		Enabled:    true,
		ScopeJSON:  "{}",
		RuleDSL:    `{"matcher":{"type":"regex","pattern":"(?m)\\bGOS_ARTIFACT_URL\\s*="}}`,
		Message:    "缺少 GOS_ARTIFACT_URL 制品地址输出",
		Suggestion: `OSS 上传成功后输出 echo "GOS_ARTIFACT_URL=..."`,
	})
	if err != nil {
		t.Fatalf("CreateRule failed: %v", err)
	}
	if rule.RuleCode != "artifact.gos.artifact_url.standard" {
		t.Fatalf("RuleCode = %q, want GOS artifact URL generated code", rule.RuleCode)
	}
	if PipelineScanRuleTypeFromCode(rule.RuleCode) != "artifact_gos_artifact_url_standard" {
		t.Fatalf("PipelineScanRuleTypeFromCode(%q) did not resolve generated type", rule.RuleCode)
	}
}

func TestPipelineScanManagerCreateRuleStoresTemplateValidationScopes(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	scanRepo := &pipelineScanRepoFake{}
	manager := NewPipelineScanManager(scanRepo, &pipelineScanPipelineRepoFake{}, &pipelineScanJenkinsFake{})

	rule, err := manager.CreateRule(ctx, CreatePipelineScanRuleInput{
		RuleType:                 "artifact_oss_command_format_standard",
		RuleName:                 "对象存储上传命令排版规范",
		Category:                 scandomain.CategoryArtifact,
		Severity:                 scandomain.SeverityWarning,
		Enabled:                  true,
		TemplateValidationScopes: []string{"ci", "cd"},
		ScopeJSON:                "{}",
		RuleDSL:                  `{"matcher":{"type":"contains","pattern":"ossutil cp"}}`,
		Message:                  "对象存储上传命令排版不符合规范",
		Suggestion:               "请按完整命令模板调整 Jenkinsfile",
	})
	if err != nil {
		t.Fatalf("CreateRule failed: %v", err)
	}
	if got := strings.Join(rule.TemplateValidationScopes, ","); got != "ci,cd" {
		t.Fatalf("TemplateValidationScopes = %q, want ci,cd", got)
	}
}

func TestPipelineScanManagerRuleMutationTriggersRescan(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	existing := scandomain.Rule{
		ID:         "psr-existing",
		RuleCode:   "artifact.oss.command_format.standard.psr-existing",
		RuleName:   "对象存储上传命令排版规范",
		Category:   scandomain.CategoryArtifact,
		Severity:   scandomain.SeverityWarning,
		Enabled:    true,
		ScopeJSON:  "{}",
		RuleDSL:    `{"matcher":{"type":"contains","pattern":"ossutil cp"}}`,
		Message:    "对象存储上传命令排版不符合规范",
		Suggestion: "请按完整命令模板调整 Jenkinsfile",
	}
	scanRepo := &pipelineScanRepoFake{rules: []scandomain.Rule{existing}}
	manager := NewPipelineScanManager(scanRepo, &pipelineScanPipelineRepoFake{}, &pipelineScanJenkinsFake{})
	rescanRequests := 0
	manager.ruleChangeRescan = func() {
		rescanRequests++
	}

	if _, err := manager.CreateRule(ctx, CreatePipelineScanRuleInput{
		RuleType:   "artifact_oss_pipeline_params_standard",
		RuleName:   "内置字段管线参数规范",
		Category:   scandomain.CategoryArtifact,
		Severity:   scandomain.SeverityWarning,
		Enabled:    true,
		ScopeJSON:  "{}",
		RuleDSL:    `{"matcher":{"type":"pipeline_parameters","required_parameters":["OSS_ENDPOINT"]}}`,
		Message:    "缺少管线参数",
		Suggestion: "补齐管线参数",
	}); err != nil {
		t.Fatalf("CreateRule failed: %v", err)
	}
	if _, err := manager.UpdateRule(ctx, existing.ID, UpdatePipelineScanRuleInput{
		RuleType:   "artifact_oss_command_format_standard",
		RuleName:   "对象存储上传命令排版规范更新",
		Category:   scandomain.CategoryArtifact,
		Severity:   scandomain.SeverityError,
		Enabled:    true,
		ScopeJSON:  "{}",
		RuleDSL:    `{"matcher":{"type":"contains","pattern":"ossutil cp"}}`,
		Message:    "对象存储上传命令排版不符合规范",
		Suggestion: "请按完整命令模板调整 Jenkinsfile",
	}); err != nil {
		t.Fatalf("UpdateRule failed: %v", err)
	}
	if err := manager.DeleteRule(ctx, existing.ID); err != nil {
		t.Fatalf("DeleteRule failed: %v", err)
	}

	if rescanRequests != 3 {
		t.Fatalf("rescanRequests = %d, want 3", rescanRequests)
	}
}

func TestPipelineScanManagerRejectsBuiltinRuleMutation(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	builtin := scandomain.Rule{
		ID:                       "psr-builtin-gos-artifact-url",
		RuleCode:                 "artifact.gos.artifact_url.standard",
		RuleName:                 "GOS 制品地址输出规范",
		Category:                 scandomain.CategoryArtifact,
		Severity:                 scandomain.SeverityWarning,
		Enabled:                  true,
		Builtin:                  true,
		TemplateValidationScopes: []string{"ci"},
		ScopeJSON:                "{}",
		RuleDSL:                  `{"matcher":{"type":"regex","pattern":"(?m)\\bGOS_ARTIFACT_URL\\s*="}}`,
		Message:                  "缺少 GOS_ARTIFACT_URL 制品地址输出",
		Suggestion:               `OSS 上传成功后输出 echo "GOS_ARTIFACT_URL=..."`,
	}
	scanRepo := &pipelineScanRepoFake{rules: []scandomain.Rule{builtin}}
	manager := NewPipelineScanManager(scanRepo, &pipelineScanPipelineRepoFake{}, &pipelineScanJenkinsFake{})
	rescanRequests := 0
	manager.ruleChangeRescan = func() {
		rescanRequests++
	}

	updated, err := manager.SetRuleEnabled(ctx, builtin.ID, false)
	if err != nil {
		t.Fatalf("SetRuleEnabled failed: %v", err)
	}
	if updated.Enabled {
		t.Fatalf("SetRuleEnabled Enabled = true, want false")
	}
	if updated.RuleName != builtin.RuleName || updated.RuleDSL != builtin.RuleDSL {
		t.Fatalf("SetRuleEnabled changed protected builtin fields: %#v", updated)
	}

	_, err = manager.UpdateRule(ctx, builtin.ID, UpdatePipelineScanRuleInput{
		RuleType:                 "artifact_gos_artifact_url_standard",
		RuleName:                 "GOS 制品地址输出规范更新",
		Category:                 scandomain.CategoryArtifact,
		Severity:                 scandomain.SeverityError,
		Enabled:                  false,
		TemplateValidationScopes: []string{"cd"},
		ScopeJSON:                "{}",
		RuleDSL:                  `{"matcher":{"type":"contains","pattern":"bad"}}`,
		Message:                  "bad",
		Suggestion:               "bad",
	})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("UpdateRule error = %v, want ErrInvalidInput", err)
	}
	if !strings.Contains(err.Error(), "内置管线规范不允许编辑") {
		t.Fatalf("UpdateRule error = %q, want builtin edit message", err.Error())
	}

	err = manager.DeleteRule(ctx, builtin.ID)
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("DeleteRule error = %v, want ErrInvalidInput", err)
	}
	if !strings.Contains(err.Error(), "内置管线规范不允许删除") {
		t.Fatalf("DeleteRule error = %q, want builtin delete message", err.Error())
	}
	if rescanRequests != 1 {
		t.Fatalf("rescanRequests = %d, want 1", rescanRequests)
	}
	stored, err := scanRepo.GetRuleByID(ctx, builtin.ID)
	if err != nil {
		t.Fatalf("builtin rule missing after rejected mutation: %v", err)
	}
	if stored.RuleName != builtin.RuleName || stored.Enabled || stored.RuleDSL != builtin.RuleDSL {
		t.Fatalf("builtin rule changed after rejected mutation: %#v", stored)
	}
}

func TestPipelineScanManagerCreateRuleGeneratesUniqueCodeWhenGeneratedTypeAlreadyExists(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	scanRepo := &pipelineScanRepoFake{}
	manager := NewPipelineScanManager(scanRepo, &pipelineScanPipelineRepoFake{}, &pipelineScanJenkinsFake{})

	first, err := manager.CreateRule(ctx, CreatePipelineScanRuleInput{
		RuleType:   "artifact_oss_command_format_standard",
		RuleName:   "对象存储上传命令排版规范",
		Category:   scandomain.CategoryArtifact,
		Severity:   scandomain.SeverityWarning,
		Enabled:    true,
		ScopeJSON:  "{}",
		RuleDSL:    `{"matcher":{"type":"contains","pattern":"ossutil cp"}}`,
		Message:    "对象存储上传命令排版不符合规范",
		Suggestion: "请按完整命令模板调整 Jenkinsfile",
	})
	if err != nil {
		t.Fatalf("first CreateRule failed: %v", err)
	}

	second, err := manager.CreateRule(ctx, CreatePipelineScanRuleInput{
		RuleType:   "artifact_oss_command_format_standard",
		RuleName:   "OA 桶上传命令排版规范",
		Category:   scandomain.CategoryArtifact,
		Severity:   scandomain.SeverityWarning,
		Enabled:    true,
		ScopeJSON:  "{}",
		RuleDSL:    `{"matcher":{"type":"contains","pattern":"ossutil cp"}}`,
		Message:    "OA 桶上传命令排版不符合规范",
		Suggestion: "请按完整命令模板调整 Jenkinsfile",
	})
	if err != nil {
		t.Fatalf("second CreateRule failed: %v", err)
	}
	if second.RuleCode == first.RuleCode {
		t.Fatalf("second RuleCode = %q, want a unique generated code", second.RuleCode)
	}
	if !strings.HasPrefix(second.RuleCode, "artifact.oss.command_format.standard.") {
		t.Fatalf("second RuleCode = %q, want generated code with standard prefix", second.RuleCode)
	}
	if PipelineScanRuleTypeFromCode(second.RuleCode) != "artifact_oss_command_format_standard" {
		t.Fatalf("PipelineScanRuleTypeFromCode(%q) did not resolve generated type", second.RuleCode)
	}
}

func TestPipelineScanManagerUpdateRulePreservesGeneratedInstanceCode(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	existing := scandomain.Rule{
		ID:         "psr-existing",
		RuleCode:   "artifact.oss.command_format.standard.psr-existing",
		RuleName:   "OA 桶上传命令排版规范",
		Category:   scandomain.CategoryArtifact,
		Severity:   scandomain.SeverityWarning,
		Enabled:    true,
		ScopeJSON:  "{}",
		RuleDSL:    `{"matcher":{"type":"contains","pattern":"ossutil cp"}}`,
		Message:    "OA 桶上传命令排版不符合规范",
		Suggestion: "请按完整命令模板调整 Jenkinsfile",
	}
	scanRepo := &pipelineScanRepoFake{rules: []scandomain.Rule{existing}}
	manager := NewPipelineScanManager(scanRepo, &pipelineScanPipelineRepoFake{}, &pipelineScanJenkinsFake{})

	updated, err := manager.UpdateRule(ctx, existing.ID, UpdatePipelineScanRuleInput{
		RuleType:   "artifact_oss_command_format_standard",
		RuleName:   "OA 桶上传命令排版规范更新",
		Category:   scandomain.CategoryArtifact,
		Severity:   scandomain.SeverityError,
		Enabled:    true,
		ScopeJSON:  "{}",
		RuleDSL:    `{"matcher":{"type":"contains","pattern":"ossutil cp"}}`,
		Message:    "OA 桶上传命令排版不符合规范",
		Suggestion: "请按完整命令模板调整 Jenkinsfile",
	})
	if err != nil {
		t.Fatalf("UpdateRule failed: %v", err)
	}
	if updated.RuleCode != existing.RuleCode {
		t.Fatalf("RuleCode = %q, want preserved %q", updated.RuleCode, existing.RuleCode)
	}
}

type pipelineScanRepoFake struct {
	rules    []scandomain.Rule
	result   scandomain.Result
	findings []scandomain.Finding
}

func (r *pipelineScanRepoFake) InitSchema(context.Context) error { return nil }

func (r *pipelineScanRepoFake) CreateRule(_ context.Context, item scandomain.Rule) error {
	for _, rule := range r.rules {
		if rule.ID == item.ID || rule.RuleCode == item.RuleCode {
			return scandomain.ErrRuleDuplicated
		}
	}
	r.rules = append(r.rules, item)
	return nil
}

func (r *pipelineScanRepoFake) ListRules(context.Context, scandomain.RuleListFilter) ([]scandomain.Rule, int64, error) {
	return append([]scandomain.Rule(nil), r.rules...), int64(len(r.rules)), nil
}

func (r *pipelineScanRepoFake) ListEnabledRules(context.Context) ([]scandomain.Rule, error) {
	items := make([]scandomain.Rule, 0, len(r.rules))
	for _, rule := range r.rules {
		if rule.Enabled {
			items = append(items, rule)
		}
	}
	return items, nil
}

func (r *pipelineScanRepoFake) GetRuleByID(_ context.Context, id string) (scandomain.Rule, error) {
	for _, rule := range r.rules {
		if rule.ID == id {
			return rule, nil
		}
	}
	return scandomain.Rule{}, scandomain.ErrRuleNotFound
}

func (r *pipelineScanRepoFake) UpdateRule(_ context.Context, id string, input scandomain.RuleUpdateInput) (scandomain.Rule, error) {
	for i, rule := range r.rules {
		if rule.ID == id {
			r.rules[i].RuleCode = input.RuleCode
			r.rules[i].RuleName = input.RuleName
			r.rules[i].Category = input.Category
			r.rules[i].Severity = input.Severity
			r.rules[i].Enabled = input.Enabled
			r.rules[i].TemplateValidationScopes = append([]string(nil), input.TemplateValidationScopes...)
			r.rules[i].ScopeJSON = input.ScopeJSON
			r.rules[i].RuleDSL = input.RuleDSL
			r.rules[i].Message = input.Message
			r.rules[i].Suggestion = input.Suggestion
			return r.rules[i], nil
		}
	}
	return scandomain.Rule{}, scandomain.ErrRuleNotFound
}

func (r *pipelineScanRepoFake) DeleteRule(_ context.Context, id string) error {
	for i, rule := range r.rules {
		if rule.ID == id {
			r.rules = append(r.rules[:i], r.rules[i+1:]...)
			return nil
		}
	}
	return scandomain.ErrRuleNotFound
}

func (r *pipelineScanRepoFake) SaveScan(_ context.Context, result scandomain.Result, findings []scandomain.Finding) error {
	r.result = result
	r.findings = append([]scandomain.Finding(nil), findings...)
	return nil
}

func (r *pipelineScanRepoFake) GetResultByPipelineID(_ context.Context, pipelineID string) (scandomain.Result, []scandomain.Finding, error) {
	if r.result.PipelineID != pipelineID {
		return scandomain.Result{}, nil, scandomain.ErrResultNotFound
	}
	return r.result, append([]scandomain.Finding(nil), r.findings...), nil
}

func (r *pipelineScanRepoFake) ListResults(context.Context, scandomain.ResultListFilter) ([]scandomain.Result, int64, error) {
	if r.result.ID == "" {
		return nil, 0, nil
	}
	return []scandomain.Result{r.result}, 1, nil
}

type pipelineScanPipelineRepoFake struct {
	pipelines map[string]pipelinedomain.Pipeline
}

func (r *pipelineScanPipelineRepoFake) InitSchema(context.Context) error { return nil }
func (r *pipelineScanPipelineRepoFake) UpsertPipelines(context.Context, []pipelinedomain.Pipeline) (int, int, error) {
	return 0, 0, nil
}
func (r *pipelineScanPipelineRepoFake) MarkMissingPipelinesInactive(context.Context, pipelinedomain.Provider, []string, time.Time) (int, error) {
	return 0, nil
}
func (r *pipelineScanPipelineRepoFake) ListPipelines(context.Context, pipelinedomain.PipelineListFilter) ([]pipelinedomain.Pipeline, int64, error) {
	items := make([]pipelinedomain.Pipeline, 0, len(r.pipelines))
	for _, item := range r.pipelines {
		items = append(items, item)
	}
	return items, int64(len(items)), nil
}
func (r *pipelineScanPipelineRepoFake) GetPipelineByID(_ context.Context, id string) (pipelinedomain.Pipeline, error) {
	item, ok := r.pipelines[id]
	if !ok {
		return pipelinedomain.Pipeline{}, pipelinedomain.ErrPipelineNotFound
	}
	return item, nil
}
func (r *pipelineScanPipelineRepoFake) MarkPipelineVerified(context.Context, string, time.Time, time.Time) (pipelinedomain.Pipeline, error) {
	return pipelinedomain.Pipeline{}, nil
}
func (r *pipelineScanPipelineRepoFake) CreateBinding(context.Context, pipelinedomain.PipelineBinding) error {
	return nil
}
func (r *pipelineScanPipelineRepoFake) ListBindingsByApplication(context.Context, pipelinedomain.BindingListFilter) ([]pipelinedomain.PipelineBinding, int64, error) {
	return nil, 0, nil
}
func (r *pipelineScanPipelineRepoFake) GetBindingByID(context.Context, string) (pipelinedomain.PipelineBinding, error) {
	return pipelinedomain.PipelineBinding{}, pipelinedomain.ErrBindingNotFound
}
func (r *pipelineScanPipelineRepoFake) UpdateBinding(context.Context, string, pipelinedomain.BindingUpdateInput, time.Time) (pipelinedomain.PipelineBinding, error) {
	return pipelinedomain.PipelineBinding{}, nil
}
func (r *pipelineScanPipelineRepoFake) DeleteBinding(context.Context, string) error { return nil }

type pipelineScanJenkinsFake struct {
	scripts map[string]pipelinedomain.JenkinsPipelineScript
}

func (j *pipelineScanJenkinsFake) ListJobs(context.Context) ([]pipelinedomain.JenkinsJob, error) {
	return nil, nil
}
func (j *pipelineScanJenkinsFake) GetJob(context.Context, string) (pipelinedomain.JenkinsJob, error) {
	return pipelinedomain.JenkinsJob{}, nil
}
func (j *pipelineScanJenkinsFake) GetPipelineScript(_ context.Context, fullName string) (pipelinedomain.JenkinsPipelineScript, error) {
	return j.scripts[fullName], nil
}
func (j *pipelineScanJenkinsFake) GetPipelineConfigXML(context.Context, string) (string, error) {
	return "", nil
}
func (j *pipelineScanJenkinsFake) BuildJobURL(fullName string) string { return fullName }
