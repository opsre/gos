package usecase

import (
	"context"
	"errors"
	"fmt"
	"strings"

	scandomain "gos/internal/domain/pipelinescan"
	releasedomain "gos/internal/domain/release"
)

type releaseTemplateComplianceResult struct {
	Status   releasedomain.TemplateComplianceStatus
	Summary  string
	Findings []releasedomain.ReleaseTemplateComplianceFinding
}

func evaluateReleaseTemplateCompliance(
	ctx context.Context,
	scanRepo scandomain.Repository,
	bindings []releasedomain.ReleaseTemplateBinding,
) releaseTemplateComplianceResult {
	if scanRepo == nil {
		return releaseTemplateComplianceResult{Status: releasedomain.TemplateComplianceStatusUnknown}
	}

	rules, err := scanRepo.ListEnabledRules(ctx)
	if err != nil {
		return releaseTemplateComplianceResult{
			Status:  releasedomain.TemplateComplianceStatusUnknown,
			Summary: "模板规范状态暂不可用",
		}
	}
	ruleIndex := buildTemplateValidationRuleIndex(rules)
	if len(ruleIndex.byScope) == 0 {
		return releaseTemplateComplianceResult{Status: releasedomain.TemplateComplianceStatusCompliant}
	}

	result := releaseTemplateComplianceResult{Status: releasedomain.TemplateComplianceStatusCompliant}
	missingScanCount := 0
	for _, binding := range bindings {
		if !binding.Enabled {
			continue
		}
		scope := binding.PipelineScope
		scopedRules := ruleIndex.byScope[scope]
		if len(scopedRules.byID) == 0 && len(scopedRules.byCode) == 0 {
			continue
		}
		if strings.ToLower(strings.TrimSpace(binding.Provider)) != "jenkins" {
			continue
		}
		pipelineID := strings.TrimSpace(binding.PipelineID)
		if pipelineID == "" {
			continue
		}
		scanResult, findings, err := scanRepo.GetResultByPipelineID(ctx, pipelineID)
		if err != nil {
			if errors.Is(err, scandomain.ErrResultNotFound) {
				missingScanCount++
				continue
			}
			return releaseTemplateComplianceResult{
				Status:  releasedomain.TemplateComplianceStatusUnknown,
				Summary: "模板规范状态暂不可用",
			}
		}
		for _, finding := range findings {
			if finding.Status != "" && finding.Status != scandomain.FindingStatusOpen {
				continue
			}
			rule, ok := scopedRules.byID[strings.TrimSpace(finding.RuleID)]
			if !ok {
				rule, ok = scopedRules.byCode[strings.TrimSpace(finding.RuleCode)]
			}
			if !ok {
				continue
			}
			result.Findings = append(result.Findings, releasedomain.ReleaseTemplateComplianceFinding{
				PipelineScope: scope,
				PipelineID:    pipelineID,
				PipelineName:  firstNonEmpty(scanResult.PipelineName, binding.BindingName),
				RuleID:        rule.ID,
				RuleCode:      rule.RuleCode,
				RuleName:      firstNonEmpty(finding.RuleName, rule.RuleName),
				Severity:      string(finding.Severity),
				LineNo:        finding.LineNo,
				Message:       firstNonEmpty(finding.Message, rule.Message),
				Suggestion:    firstNonEmpty(finding.Suggestion, rule.Suggestion),
			})
		}
	}

	if len(result.Findings) > 0 {
		result.Status = releasedomain.TemplateComplianceStatusViolated
		result.Summary = summarizeReleaseTemplateComplianceFindings(result.Findings)
		return result
	}
	if missingScanCount > 0 {
		result.Status = releasedomain.TemplateComplianceStatusUnknown
		result.Summary = "部分模板管线未完成规范扫描"
	}
	return result
}

type templateValidationRuleIndex struct {
	byScope map[releasedomain.PipelineScope]scopedTemplateValidationRules
}

type scopedTemplateValidationRules struct {
	byID   map[string]scandomain.Rule
	byCode map[string]scandomain.Rule
}

func buildTemplateValidationRuleIndex(rules []scandomain.Rule) templateValidationRuleIndex {
	index := templateValidationRuleIndex{byScope: make(map[releasedomain.PipelineScope]scopedTemplateValidationRules)}
	for _, rule := range rules {
		if !rule.Enabled {
			continue
		}
		for _, rawScope := range rule.TemplateValidationScopes {
			scope := releasedomain.PipelineScope(strings.ToLower(strings.TrimSpace(rawScope)))
			if !scope.Valid() {
				continue
			}
			scoped := index.byScope[scope]
			if scoped.byID == nil {
				scoped.byID = make(map[string]scandomain.Rule)
				scoped.byCode = make(map[string]scandomain.Rule)
			}
			if rule.ID != "" {
				scoped.byID[rule.ID] = rule
			}
			if rule.RuleCode != "" {
				scoped.byCode[rule.RuleCode] = rule
			}
			index.byScope[scope] = scoped
		}
	}
	return index
}

func summarizeReleaseTemplateComplianceFindings(findings []releasedomain.ReleaseTemplateComplianceFinding) string {
	counts := map[releasedomain.PipelineScope]int{}
	for _, finding := range findings {
		counts[finding.PipelineScope]++
	}
	parts := make([]string, 0, 2)
	for _, scope := range []releasedomain.PipelineScope{releasedomain.PipelineScopeCI, releasedomain.PipelineScopeCD} {
		if counts[scope] > 0 {
			parts = append(parts, fmt.Sprintf("%s %d项违规", strings.ToUpper(string(scope)), counts[scope]))
		}
	}
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, "，")
}
