package usecase

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	scandomain "gos/internal/domain/pipelinescan"
	releasedomain "gos/internal/domain/release"
)

func TestEvaluateReleaseTemplateComplianceUsesConfiguredScope(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	now := time.Unix(1_710_000_000, 0).UTC()
	scanRepo := &pipelineScanRepoFake{
		rules: []scandomain.Rule{
			{
				ID:                       "psr-ci",
				RuleCode:                 "artifact.oss.command_format.standard",
				RuleName:                 "OSS 上传命令规范",
				Category:                 scandomain.CategoryArtifact,
				Severity:                 scandomain.SeverityWarning,
				Enabled:                  true,
				TemplateValidationScopes: []string{"ci"},
				ScopeJSON:                "{}",
				RuleDSL:                  `{"matcher":{"type":"contains","pattern":"ossutil cp"}}`,
				Message:                  "OSS 上传命令不符合规范",
				CreatedAt:                now,
				UpdatedAt:                now,
			},
		},
		result: scandomain.Result{
			ID:           "psres-ci",
			PipelineID:   "pipeline-ci",
			PipelineName: "folder/ci",
			ScanStatus:   scandomain.ScanStatusWarning,
		},
		findings: []scandomain.Finding{
			{
				ID:         "psf-ci",
				PipelineID: "pipeline-ci",
				RuleID:     "psr-ci",
				RuleCode:   "artifact.oss.command_format.standard",
				RuleName:   "OSS 上传命令规范",
				Severity:   scandomain.SeverityWarning,
				Message:    "OSS 上传命令不符合规范",
				Status:     scandomain.FindingStatusOpen,
			},
		},
	}

	result := evaluateReleaseTemplateCompliance(ctx, scanRepo, []releasedomain.ReleaseTemplateBinding{
		{
			PipelineScope: releasedomain.PipelineScopeCI,
			Provider:      "jenkins",
			PipelineID:    "pipeline-ci",
			Enabled:       true,
		},
	})
	if result.Status != releasedomain.TemplateComplianceStatusViolated {
		t.Fatalf("Status = %q, want violated", result.Status)
	}
	if !strings.Contains(result.Summary, "CI 1项违规") {
		t.Fatalf("Summary = %q, want CI violation count", result.Summary)
	}
	if len(result.Findings) != 1 || result.Findings[0].PipelineScope != releasedomain.PipelineScopeCI {
		t.Fatalf("Findings = %#v, want one CI finding", result.Findings)
	}

	result = evaluateReleaseTemplateCompliance(ctx, scanRepo, []releasedomain.ReleaseTemplateBinding{
		{
			PipelineScope: releasedomain.PipelineScopeCD,
			Provider:      "jenkins",
			PipelineID:    "pipeline-ci",
			Enabled:       true,
		},
	})
	if result.Status != releasedomain.TemplateComplianceStatusCompliant {
		t.Fatalf("Status = %q, want compliant for unscoped CD binding", result.Status)
	}
}

func TestReleaseOrderRejectsViolatedTemplate(t *testing.T) {
	t.Parallel()

	manager, releaseRepo := newReleaseOrderManagerForCancelTest(t)
	ctx := context.Background()
	now := time.Unix(1_710_000_000, 0).UTC()
	template := releasedomain.ReleaseTemplate{
		ID:              "rt-violated",
		Name:            "违规模板",
		ApplicationID:   "app-1",
		ApplicationName: "App 1",
		BindingID:       "app-1",
		BindingName:     "CI",
		BindingType:     "ci",
		Status:          releasedomain.TemplateStatusActive,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	bindings := []releasedomain.ReleaseTemplateBinding{
		{
			ID:            "rtb-ci",
			TemplateID:    template.ID,
			PipelineScope: releasedomain.PipelineScopeCI,
			BindingID:     "binding-ci",
			BindingName:   "CI",
			Provider:      "jenkins",
			PipelineID:    "pipeline-ci",
			Enabled:       true,
			SortNo:        10,
			CreatedAt:     now,
			UpdatedAt:     now,
		},
	}
	if err := releaseRepo.CreateTemplate(ctx, template, bindings, nil, nil, nil); err != nil {
		t.Fatalf("CreateTemplate failed: %v", err)
	}
	manager.SetPipelineScanRepository(&pipelineScanRepoFake{
		rules: []scandomain.Rule{
			{
				ID:                       "psr-ci",
				RuleCode:                 "artifact.oss.command_format.standard",
				RuleName:                 "OSS 上传命令规范",
				Severity:                 scandomain.SeverityWarning,
				Enabled:                  true,
				TemplateValidationScopes: []string{"ci"},
			},
		},
		result: scandomain.Result{
			ID:           "psres-ci",
			PipelineID:   "pipeline-ci",
			PipelineName: "folder/ci",
			ScanStatus:   scandomain.ScanStatusWarning,
		},
		findings: []scandomain.Finding{
			{
				ID:         "psf-ci",
				PipelineID: "pipeline-ci",
				RuleID:     "psr-ci",
				RuleCode:   "artifact.oss.command_format.standard",
				RuleName:   "OSS 上传命令规范",
				Severity:   scandomain.SeverityWarning,
				Message:    "OSS 上传命令不符合规范",
				Status:     scandomain.FindingStatusOpen,
			},
		},
	})

	_, _, _, _, err := manager.resolveTemplateForCreate(ctx, "app-1", template.ID)
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("resolveTemplateForCreate error = %v, want ErrInvalidInput", err)
	}
	if !strings.Contains(err.Error(), "模板违反管线规范") {
		t.Fatalf("resolveTemplateForCreate error = %v, want compliance message", err)
	}
}
