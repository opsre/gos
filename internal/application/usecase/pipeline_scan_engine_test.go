package usecase

import (
	"fmt"
	"strings"
	"testing"

	scandomain "gos/internal/domain/pipelinescan"
)

func TestPipelineScanEngineCommandBlockFindsMissingACL(t *testing.T) {
	t.Parallel()

	script := `
pipeline {
  stages {
    stage('upload') {
      steps {
        sh """
ossutil cp "${WORKSPACE}/app.zip" \
"oss://${OSS_BUCKET}/${OSS_DIR}/app-${BUILD_NUMBER}.zip" \
--endpoint "${OSS_ENDPOINT}" \
--access-key-id "${OSS_ACCESS_KEY_ID}" \
--access-key-secret "${OSS_ACCESS_KEY_SECRET}" -f
"""
      }
    }
  }
}`

	rules := []scandomain.Rule{
		{
			ID:         "rule-acl",
			RuleCode:   "artifact.oss.acl.required",
			RuleName:   "OSS 上传必须设置 ACL",
			Category:   scandomain.CategoryArtifact,
			Severity:   scandomain.SeverityWarning,
			Enabled:    true,
			RuleDSL:    `{"matcher":{"type":"command_block","start_pattern":"ossutil\\s+cp","required_patterns":["--acl\\s+(public-read|private)"],"max_lines":20}}`,
			Message:    "发现 OSS 上传命令未设置对象 ACL",
			Suggestion: "在 ossutil cp 命令中增加 --acl public-read 或 --acl private",
		},
	}

	findings, err := NewPipelineScanEngine().ScanScript("pln-test", "demo", script, rules)
	if err != nil {
		t.Fatalf("ScanScript returned error: %v", err)
	}
	if len(findings) != 1 {
		t.Fatalf("len(findings) = %d, want 1: %#v", len(findings), findings)
	}
	if findings[0].RuleCode != "artifact.oss.acl.required" {
		t.Fatalf("RuleCode = %q", findings[0].RuleCode)
	}
	if findings[0].Severity != scandomain.SeverityWarning {
		t.Fatalf("Severity = %q", findings[0].Severity)
	}
	if findings[0].LineNo != 7 {
		t.Fatalf("LineNo = %d, want 7", findings[0].LineNo)
	}
	if !strings.Contains(findings[0].MatchedText, "ossutil cp") {
		t.Fatalf("MatchedText = %q, want ossutil cp block", findings[0].MatchedText)
	}
}

func TestPipelineScanEngineCommandFormatChecksOrderedLines(t *testing.T) {
	t.Parallel()

	script := `
sh """
ossutil cp "${WORKSPACE}/notarybusiness.zip" \
"oss://${OSS_BUCKET}/${OSS_DIR}/notarybusiness-${BUILD_NUMBER}.zip" \
--access-key-secret "${OSS_ACCESS_KEY_SECRET}" \
--endpoint "${OSS_ENDPOINT}" \
--access-key-id "${OSS_ACCESS_KEY_ID}" \
--acl public-read \
-f
"""`

	rules := []scandomain.Rule{
		{
			ID:       "rule-format",
			RuleCode: "artifact.oss.command.format",
			RuleName: "OSS 上传命令格式必须符合平台规范",
			Category: scandomain.CategoryArtifact,
			Severity: scandomain.SeverityWarning,
			Enabled:  true,
			RuleDSL: `{
				"matcher": {
					"type": "command_format",
					"start_pattern": "ossutil\\s+cp",
					"format": {
						"mode": "ordered_lines",
						"allow_extra_lines": false,
						"ignore_indent": true,
						"max_lines": 10,
						"lines": [
							{"name":"upload_command","pattern":"^\\s*ossutil\\s+cp\\s+.+\\\\$"},
							{"name":"target_object","pattern":"^\\s*\\\"oss://\\$\\{OSS_BUCKET\\}/\\$\\{OSS_DIR\\}/.+\\\"\\s+\\\\$"},
							{"name":"endpoint","pattern":"^\\s*--endpoint\\s+\\\"?\\$\\{OSS_ENDPOINT\\}\\\"?\\s+\\\\$"},
							{"name":"access_key_id","pattern":"^\\s*--access-key-id\\s+\\\"?\\$\\{OSS_ACCESS_KEY_ID\\}\\\"?\\s+\\\\$"},
							{"name":"access_key_secret","pattern":"^\\s*--access-key-secret\\s+\\\"?\\$\\{OSS_ACCESS_KEY_SECRET\\}\\\"?\\s+\\\\$"},
							{"name":"acl","pattern":"^\\s*--acl\\s+(public-read|private)\\s+\\\\$"},
							{"name":"force","pattern":"^\\s*-f\\s*$"}
						]
					}
				}
			}`,
			Message:    "OSS 上传命令格式不符合平台规范",
			Suggestion: "请按规范顺序编排：cp、目标对象、endpoint、access-key-id、access-key-secret、acl、-f",
		},
	}

	findings, err := NewPipelineScanEngine().ScanScript("pln-test", "demo", script, rules)
	if err != nil {
		t.Fatalf("ScanScript returned error: %v", err)
	}
	if len(findings) != 1 {
		t.Fatalf("len(findings) = %d, want 1: %#v", len(findings), findings)
	}
	if !strings.Contains(findings[0].DetailsJSON, "endpoint") {
		t.Fatalf("DetailsJSON = %q, want endpoint mismatch detail", findings[0].DetailsJSON)
	}
}

func TestPipelineScanEngineCommandFormatRequireBlock(t *testing.T) {
	t.Parallel()

	script := `
pipeline {
  stages {
    stage('build') {
      steps {
        sh 'mvn clean package'
      }
    }
  }
}`

	baseRuleDSL := `{
		"matcher": {
			"type": "command_format",
			"start_pattern": "ossutil\\s+cp",
			"format": {
				"require_block": %t,
				"allow_extra_lines": false,
				"max_lines": 8,
				"lines": [
					{"name":"upload_command","pattern":"^\\s*ossutil\\s+cp\\s+.+\\\\$"},
					{"name":"acl","pattern":"^\\s*--acl\\s+public-read\\s+-f$"}
				]
			}
		}
	}`

	t.Run("disabled keeps missing command compliant", func(t *testing.T) {
		t.Parallel()
		rules := []scandomain.Rule{
			{
				ID:         "rule-format",
				RuleCode:   "artifact.oss.command.format",
				RuleName:   "OSS 上传命令格式必须符合平台规范",
				Category:   scandomain.CategoryArtifact,
				Severity:   scandomain.SeverityWarning,
				Enabled:    true,
				RuleDSL:    fmt.Sprintf(baseRuleDSL, false),
				Message:    "OSS 上传命令格式不符合平台规范",
				Suggestion: "请按规范顺序编排上传命令",
			},
		}

		findings, err := NewPipelineScanEngine().ScanScript("pln-test", "demo", script, rules)
		if err != nil {
			t.Fatalf("ScanScript returned error: %v", err)
		}
		if len(findings) != 0 {
			t.Fatalf("len(findings) = %d, want 0: %#v", len(findings), findings)
		}
	})

	t.Run("enabled reports missing command", func(t *testing.T) {
		t.Parallel()
		rules := []scandomain.Rule{
			{
				ID:         "rule-format",
				RuleCode:   "artifact.oss.command.format",
				RuleName:   "OSS 上传命令格式必须符合平台规范",
				Category:   scandomain.CategoryArtifact,
				Severity:   scandomain.SeverityWarning,
				Enabled:    true,
				RuleDSL:    fmt.Sprintf(baseRuleDSL, true),
				Message:    "OSS 上传命令格式不符合平台规范",
				Suggestion: "请按规范顺序编排上传命令",
			},
		}

		findings, err := NewPipelineScanEngine().ScanScript("pln-test", "demo", script, rules)
		if err != nil {
			t.Fatalf("ScanScript returned error: %v", err)
		}
		if len(findings) != 1 {
			t.Fatalf("len(findings) = %d, want 1: %#v", len(findings), findings)
		}
		if findings[0].LineNo != 0 {
			t.Fatalf("LineNo = %d, want 0 for missing command", findings[0].LineNo)
		}
		if findings[0].Message != "管线中未找到该完整命令" {
			t.Fatalf("Message = %q, want missing command message", findings[0].Message)
		}
		if !strings.Contains(findings[0].DetailsJSON, "未找到完整命令") {
			t.Fatalf("DetailsJSON = %q, want missing command detail", findings[0].DetailsJSON)
		}
	})
}

func TestPipelineScanEnginePipelineParametersFindsMissingOSSFields(t *testing.T) {
	t.Parallel()

	script := `
pipeline {
  agent any
  parameters {
    string(name: 'OSS_ENDPOINT', defaultValue: '', description: 'Endpoint')
    string(name: 'OSS_BUCKET', defaultValue: '', description: 'Bucket')
    string(name: 'OSS_DIR', defaultValue: '', description: 'Dir')
  }
}`

	rules := []scandomain.Rule{
		{
			ID:       "rule-params",
			RuleCode: "artifact.oss.pipeline_params.standard",
			RuleName: "OSS 管线参数必须完整",
			Category: scandomain.CategoryArtifact,
			Severity: scandomain.SeverityWarning,
			Enabled:  true,
			RuleDSL: `{
				"matcher": {
					"type": "pipeline_parameters",
					"required_parameters": [
						"OSS_ENDPOINT",
						"OSS_BUCKET",
						"OSS_DIR",
						"OSS_ACL",
						"OSS_ACCESS_KEY_ID",
						"OSS_ACCESS_KEY_SECRET"
					]
				}
			}`,
			Message:    "OSS 管线参数缺失",
			Suggestion: "请在 Jenkins 参数中补齐对象存储内置字段",
		},
	}

	findings, err := NewPipelineScanEngine().ScanScript("pln-test", "demo", script, rules)
	if err != nil {
		t.Fatalf("ScanScript returned error: %v", err)
	}
	if len(findings) != 1 {
		t.Fatalf("len(findings) = %d, want 1: %#v", len(findings), findings)
	}
	if !strings.Contains(findings[0].DetailsJSON, "OSS_ACL") {
		t.Fatalf("DetailsJSON = %q, want missing OSS_ACL detail", findings[0].DetailsJSON)
	}
	if !strings.Contains(findings[0].DetailsJSON, "OSS_ACCESS_KEY_SECRET") {
		t.Fatalf("DetailsJSON = %q, want missing OSS_ACCESS_KEY_SECRET detail", findings[0].DetailsJSON)
	}
}

func TestPipelineScanEngineRegexRequiresGOSArtifactURLOutput(t *testing.T) {
	t.Parallel()

	rules := []scandomain.Rule{
		{
			ID:         "rule-gos-artifact-url",
			RuleCode:   "artifact.gos.artifact_url.standard",
			RuleName:   "GOS 制品地址输出规范",
			Category:   scandomain.CategoryArtifact,
			Severity:   scandomain.SeverityError,
			Enabled:    true,
			RuleDSL:    `{"matcher":{"type":"regex","pattern":"(?m)\\bGOS_ARTIFACT_URL\\s*="}}`,
			Message:    "缺少 GOS_ARTIFACT_URL 制品地址输出",
			Suggestion: `OSS 上传成功后输出 echo "GOS_ARTIFACT_URL=..."`,
		},
	}

	missingScript := `
stage('Upload OSS') {
  steps {
    sh 'ossutil cp app.jar oss://bucket/app.jar -f'
  }
}`
	findings, err := NewPipelineScanEngine().ScanScript("pln-test", "demo", missingScript, rules)
	if err != nil {
		t.Fatalf("ScanScript returned error: %v", err)
	}
	if len(findings) != 1 {
		t.Fatalf("len(findings) = %d, want 1: %#v", len(findings), findings)
	}
	if findings[0].Message != "缺少 GOS_ARTIFACT_URL 制品地址输出" {
		t.Fatalf("Message = %q", findings[0].Message)
	}

	compliantScript := `
stage('Upload OSS') {
  steps {
    sh '''
ossutil cp app.jar oss://bucket/app.jar -f
echo "GOS_ARTIFACT_URL=https://example.com/app.jar"
'''
  }
}`
	findings, err = NewPipelineScanEngine().ScanScript("pln-test", "demo", compliantScript, rules)
	if err != nil {
		t.Fatalf("ScanScript returned error: %v", err)
	}
	if len(findings) != 0 {
		t.Fatalf("len(findings) = %d, want 0: %#v", len(findings), findings)
	}
}
