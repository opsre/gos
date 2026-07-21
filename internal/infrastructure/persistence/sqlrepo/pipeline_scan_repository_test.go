package sqlrepo

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	scandomain "gos/internal/domain/pipelinescan"

	_ "modernc.org/sqlite"
)

func TestPipelineScanRepositoryStoresRulesAndResults(t *testing.T) {
	t.Parallel()

	repo := newTestPipelineScanRepository(t)
	ctx := context.Background()
	now := time.Unix(1_710_000_000, 0).UTC()

	rule := scandomain.Rule{
		ID:                       "psr-acl",
		RuleCode:                 "artifact.oss.acl.required",
		RuleName:                 "OSS 上传必须设置 ACL",
		Category:                 scandomain.CategoryArtifact,
		Severity:                 scandomain.SeverityWarning,
		Enabled:                  true,
		TemplateValidationScopes: []string{"ci", "cd"},
		ScopeJSON:                "{}",
		RuleDSL:                  `{"matcher":{"type":"contains","pattern":"ossutil cp"}}`,
		Message:                  "发现 OSS 上传命令未设置对象 ACL",
		Suggestion:               "增加 --acl public-read",
		CreatedAt:                now,
		UpdatedAt:                now,
	}
	if err := repo.CreateRule(ctx, rule); err != nil {
		t.Fatalf("CreateRule failed: %v", err)
	}

	enabled, err := repo.ListEnabledRules(ctx)
	if err != nil {
		t.Fatalf("ListEnabledRules failed: %v", err)
	}
	var gotCustomRule scandomain.Rule
	for _, item := range enabled {
		if item.RuleCode == rule.RuleCode {
			gotCustomRule = item
			break
		}
	}
	if gotCustomRule.RuleCode != rule.RuleCode {
		t.Fatalf("enabled rules = %#v, want %s", enabled, rule.RuleCode)
	}
	if len(gotCustomRule.TemplateValidationScopes) != 2 || gotCustomRule.TemplateValidationScopes[0] != "ci" || gotCustomRule.TemplateValidationScopes[1] != "cd" {
		t.Fatalf("TemplateValidationScopes = %#v, want ci/cd", gotCustomRule.TemplateValidationScopes)
	}

	result := scandomain.Result{
		ID:            "psres-1",
		PipelineID:    "pln-1",
		PipelineName:  "demo",
		ScanStatus:    scandomain.ScanStatusWarning,
		TotalFindings: 1,
		WarningCount:  1,
		ScriptHash:    "hash-1",
		LastScannedAt: now,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	findings := []scandomain.Finding{
		{
			ID:          "psf-1",
			PipelineID:  "pln-1",
			RuleID:      rule.ID,
			RuleCode:    rule.RuleCode,
			RuleName:    rule.RuleName,
			Severity:    scandomain.SeverityWarning,
			LineNo:      10,
			MatchedText: "ossutil cp app.zip",
			Message:     rule.Message,
			Suggestion:  rule.Suggestion,
			DetailsJSON: `{"missing_patterns":["--acl"]}`,
			Status:      scandomain.FindingStatusOpen,
			CreatedAt:   now,
			UpdatedAt:   now,
		},
	}
	if err := repo.SaveScan(ctx, result, findings); err != nil {
		t.Fatalf("SaveScan failed: %v", err)
	}

	gotResult, gotFindings, err := repo.GetResultByPipelineID(ctx, "pln-1")
	if err != nil {
		t.Fatalf("GetResultByPipelineID failed: %v", err)
	}
	if gotResult.ScanStatus != scandomain.ScanStatusWarning {
		t.Fatalf("ScanStatus = %q, want warning", gotResult.ScanStatus)
	}
	if len(gotFindings) != 1 {
		t.Fatalf("len(gotFindings) = %d, want 1", len(gotFindings))
	}
	if gotFindings[0].LineNo != 10 || gotFindings[0].RuleCode != rule.RuleCode {
		t.Fatalf("finding = %#v", gotFindings[0])
	}
}

func TestPipelineScanRepositorySeedsBuiltinGOSArtifactURLRule(t *testing.T) {
	t.Parallel()

	repo := newTestPipelineScanRepository(t)
	ctx := context.Background()

	rules, _, err := repo.ListRules(ctx, scandomain.RuleListFilter{Page: 1, PageSize: 20})
	if err != nil {
		t.Fatalf("ListRules failed: %v", err)
	}
	var got scandomain.Rule
	for _, item := range rules {
		if item.RuleCode == "artifact.gos.artifact_url.standard" {
			got = item
			break
		}
	}
	if got.RuleCode == "" {
		t.Fatalf("builtin GOS artifact URL rule not seeded: %#v", rules)
	}
	if !got.Builtin {
		t.Fatalf("Builtin = false, want true")
	}
	if got.RuleName != "GOS 制品地址输出规范" {
		t.Fatalf("RuleName = %q, want GOS 制品地址输出规范", got.RuleName)
	}
	if got.Enabled {
		t.Fatalf("Enabled = true, want false for a fresh deployment")
	}
	if len(got.TemplateValidationScopes) != 1 || got.TemplateValidationScopes[0] != "ci" {
		t.Fatalf("TemplateValidationScopes = %#v, want ci", got.TemplateValidationScopes)
	}
	if got.Category != scandomain.CategoryArtifact || got.Severity != scandomain.SeverityWarning {
		t.Fatalf("category/severity = %q/%q, want artifact/warning", got.Category, got.Severity)
	}
	if got.RuleDSL == "" || got.Message == "" || got.Suggestion == "" {
		t.Fatalf("seeded rule content is incomplete: %#v", got)
	}
}

func TestPipelineScanRepositoryRecordsAutomaticV131MigrationOnce(t *testing.T) {
	t.Parallel()

	repo := newTestPipelineScanRepository(t)
	ctx := context.Background()
	const version = "deploy_platform_v1_3_1_pipeline_scan_rules"

	assertMigrationCount := func(want int) {
		t.Helper()
		var got int
		if err := repo.db.QueryRowContext(
			ctx,
			`SELECT COUNT(*) FROM gos_schema_migration WHERE version = ?;`,
			version,
		).Scan(&got); err != nil {
			t.Fatalf("query migration record failed: %v", err)
		}
		if got != want {
			t.Fatalf("migration record count = %d, want %d", got, want)
		}
	}

	assertMigrationCount(1)
	if err := repo.InitSchema(ctx); err != nil {
		t.Fatalf("second InitSchema failed: %v", err)
	}
	assertMigrationCount(1)
}

func TestPipelineScanRepositoryPreservesBuiltinEnabledOnReseed(t *testing.T) {
	t.Parallel()

	repo := newTestPipelineScanRepository(t)
	ctx := context.Background()

	rules, _, err := repo.ListRules(ctx, scandomain.RuleListFilter{Page: 1, PageSize: 20})
	if err != nil {
		t.Fatalf("ListRules failed: %v", err)
	}
	var builtin scandomain.Rule
	for _, item := range rules {
		if item.RuleCode == "artifact.gos.artifact_url.standard" {
			builtin = item
			break
		}
	}
	if builtin.ID == "" {
		t.Fatalf("builtin GOS artifact URL rule not seeded: %#v", rules)
	}
	_, err = repo.UpdateRule(ctx, builtin.ID, scandomain.RuleUpdateInput{
		RuleCode:                 builtin.RuleCode,
		RuleName:                 builtin.RuleName,
		Category:                 builtin.Category,
		Severity:                 builtin.Severity,
		Enabled:                  true,
		TemplateValidationScopes: append([]string(nil), builtin.TemplateValidationScopes...),
		ScopeJSON:                builtin.ScopeJSON,
		RuleDSL:                  builtin.RuleDSL,
		Message:                  builtin.Message,
		Suggestion:               builtin.Suggestion,
	})
	if err != nil {
		t.Fatalf("UpdateRule failed: %v", err)
	}
	if err := repo.InitSchema(ctx); err != nil {
		t.Fatalf("InitSchema reseed failed: %v", err)
	}
	got, err := repo.GetRuleByID(ctx, builtin.ID)
	if err != nil {
		t.Fatalf("GetRuleByID failed: %v", err)
	}
	if !got.Enabled {
		t.Fatalf("Enabled = false after reseed, want preserved true")
	}
	if !got.Builtin || got.RuleName != "GOS 制品地址输出规范" {
		t.Fatalf("builtin metadata was not preserved after reseed: %#v", got)
	}
}

func TestPipelineScanRepositoryMigratesLegacyBuiltinRuleID(t *testing.T) {
	t.Parallel()

	repo := newTestPipelineScanRepository(t)
	ctx := context.Background()
	const (
		builtinID = "psr-builtin-gos-artifact-url"
		legacyID  = "psr-legacy-gos-artifact-url"
	)

	builtin, err := repo.GetRuleByID(ctx, builtinID)
	if err != nil {
		t.Fatalf("GetRuleByID failed: %v", err)
	}
	if _, err := repo.UpdateRule(ctx, builtin.ID, scandomain.RuleUpdateInput{
		RuleCode:                 builtin.RuleCode,
		RuleName:                 builtin.RuleName,
		Category:                 builtin.Category,
		Severity:                 builtin.Severity,
		Enabled:                  false,
		TemplateValidationScopes: append([]string(nil), builtin.TemplateValidationScopes...),
		ScopeJSON:                builtin.ScopeJSON,
		RuleDSL:                  builtin.RuleDSL,
		Message:                  builtin.Message,
		Suggestion:               builtin.Suggestion,
	}); err != nil {
		t.Fatalf("disable builtin rule failed: %v", err)
	}

	now := time.Unix(1_710_000_100, 0).UTC()
	if err := repo.SaveScan(ctx, scandomain.Result{
		ID:            "psres-legacy-rule-id",
		PipelineID:    "pln-legacy-rule-id",
		PipelineName:  "legacy-rule-id",
		ScanStatus:    scandomain.ScanStatusWarning,
		TotalFindings: 1,
		WarningCount:  1,
		ScriptHash:    "legacy-rule-id",
		LastScannedAt: now,
		CreatedAt:     now,
		UpdatedAt:     now,
	}, []scandomain.Finding{{
		ID:          "psf-legacy-rule-id",
		PipelineID:  "pln-legacy-rule-id",
		RuleID:      builtinID,
		RuleCode:    builtin.RuleCode,
		RuleName:    builtin.RuleName,
		Severity:    builtin.Severity,
		Message:     builtin.Message,
		Suggestion:  builtin.Suggestion,
		DetailsJSON: `{}`,
		Status:      scandomain.FindingStatusOpen,
		CreatedAt:   now,
		UpdatedAt:   now,
	}}); err != nil {
		t.Fatalf("SaveScan failed: %v", err)
	}
	if _, err := repo.db.ExecContext(ctx, `UPDATE pipeline_scan_findings SET rule_id = ? WHERE rule_id = ?;`, legacyID, builtinID); err != nil {
		t.Fatalf("prepare legacy finding failed: %v", err)
	}
	if _, err := repo.db.ExecContext(ctx, `UPDATE pipeline_scan_rules SET id = ? WHERE id = ?;`, legacyID, builtinID); err != nil {
		t.Fatalf("prepare legacy rule failed: %v", err)
	}

	if err := repo.InitSchema(ctx); err != nil {
		t.Fatalf("InitSchema migration failed: %v", err)
	}
	migrated, err := repo.GetRuleByID(ctx, builtinID)
	if err != nil {
		t.Fatalf("canonical builtin rule missing after migration: %v", err)
	}
	if migrated.Enabled {
		t.Fatal("builtin enabled state was reset during ID migration")
	}
	if _, err := repo.GetRuleByID(ctx, legacyID); !errors.Is(err, scandomain.ErrRuleNotFound) {
		t.Fatalf("legacy rule lookup error = %v, want ErrRuleNotFound", err)
	}
	_, findings, err := repo.GetResultByPipelineID(ctx, "pln-legacy-rule-id")
	if err != nil {
		t.Fatalf("GetResultByPipelineID failed: %v", err)
	}
	if len(findings) != 1 || findings[0].RuleID != builtinID {
		t.Fatalf("migrated findings = %#v, want rule ID %q", findings, builtinID)
	}
}

func newTestPipelineScanRepository(t *testing.T) *PipelineScanRepository {
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

	repo := NewPipelineScanRepository(db, "sqlite")
	if err := repo.InitSchema(context.Background()); err != nil {
		t.Fatalf("InitSchema failed: %v", err)
	}
	return repo
}
