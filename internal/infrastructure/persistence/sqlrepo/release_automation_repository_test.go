package sqlrepo

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	domain "gos/internal/domain/releaseautomation"

	_ "modernc.org/sqlite"
)

func newTestReleaseAutomationRepository(t *testing.T) (*ReleaseAutomationRepository, *sql.DB) {
	t.Helper()

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("sql.Open failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	repo := NewReleaseAutomationRepository(db, "sqlite")
	if err := repo.InitSchema(context.Background()); err != nil {
		t.Fatalf("InitSchema failed: %v", err)
	}
	return repo, db
}

func newTestReleaseAutomation(id string, appID string, envCode string, gitRef string, createdAt time.Time) domain.Automation {
	return domain.Automation{
		ID:              id,
		Name:            "生产自动发布 " + id,
		ApplicationID:   appID,
		ApplicationName: "fusion-source-web",
		TemplateID:      "rt-1",
		TemplateName:    "生产发布模板",
		EnvCode:         envCode,
		GitRef:          gitRef,
		DispatchMode:    domain.DispatchModeBuildDeploy,
		Enabled:         true,
		Params: []domain.Param{
			{PipelineScope: "ci", ParamKey: "git_ref", ExecutorParamName: "GIT_REF", ParamValue: gitRef, ValueSource: "release_input"},
		},
		Remark:        "测试",
		LastSeenSHA:   "sha-1",
		CreatorUserID: "usr-1",
		CreatorName:   "张三",
		CreatedAt:     createdAt,
		UpdatedAt:     createdAt,
	}
}

func TestReleaseAutomationRepositoryCRUD(t *testing.T) {
	t.Parallel()

	repo, _ := newTestReleaseAutomationRepository(t)
	ctx := context.Background()
	now := time.Unix(1700000000, 0).UTC()

	item := newTestReleaseAutomation("rauto-1", "app-1", "prod", "main", now)
	if err := repo.Create(ctx, item); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	got, err := repo.GetByID(ctx, item.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if got.Name != item.Name || got.DispatchMode != domain.DispatchModeBuildDeploy {
		t.Fatalf("got = %+v", got)
	}
	if len(got.Params) != 1 || got.Params[0].ExecutorParamName != "GIT_REF" {
		t.Fatalf("params not round-tripped: %+v", got.Params)
	}
	if got.CreatorUserID != "usr-1" || got.CreatorName != "张三" {
		t.Fatalf("creator not round-tripped: %q / %q", got.CreatorUserID, got.CreatorName)
	}
	if got.LastCheckedAt != nil {
		t.Fatalf("LastCheckedAt = %v, want nil before the first poll", got.LastCheckedAt)
	}

	// 列表过滤：关键字命中应用名/分支名，enabled 与 application_id 生效。
	items, total, err := repo.List(ctx, domain.ListFilter{Keyword: "main", Page: 1, PageSize: 20})
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if total != 1 || len(items) != 1 {
		t.Fatalf("List keyword main: total=%d len=%d", total, len(items))
	}
	enabled := false
	if _, total, err = repo.List(ctx, domain.ListFilter{Enabled: &enabled}); err != nil {
		t.Fatalf("List enabled=false failed: %v", err)
	} else if total != 0 {
		t.Fatalf("List enabled=false total = %d, want 0", total)
	}
	if _, total, err = repo.List(ctx, domain.ListFilter{ApplicationID: "app-2"}); err != nil {
		t.Fatalf("List by application failed: %v", err)
	} else if total != 0 {
		t.Fatalf("List app-2 total = %d, want 0", total)
	}

	// 更新不动运行态列：last_seen_sha 仍由轮询写回。
	updated := got
	updated.Name = "改个名字"
	updated.DispatchMode = domain.DispatchModeExecute
	updated.UpdatedAt = now.Add(time.Minute)
	if err := repo.Update(ctx, updated); err != nil {
		t.Fatalf("Update failed: %v", err)
	}
	reloaded, err := repo.GetByID(ctx, item.ID)
	if err != nil {
		t.Fatalf("GetByID after update failed: %v", err)
	}
	if reloaded.Name != "改个名字" || reloaded.DispatchMode != domain.DispatchModeExecute {
		t.Fatalf("update not applied: %+v", reloaded)
	}
	if reloaded.LastSeenSHA != "sha-1" {
		t.Fatalf("LastSeenSHA = %q, want the runtime value kept", reloaded.LastSeenSHA)
	}

	if err := repo.Delete(ctx, item.ID); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}
	if _, err := repo.GetByID(ctx, item.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("GetByID after delete err = %v, want ErrNotFound", err)
	}
	if err := repo.Delete(ctx, item.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("second Delete err = %v, want ErrNotFound", err)
	}
}

func TestReleaseAutomationRepositoryUniqueTarget(t *testing.T) {
	t.Parallel()

	repo, _ := newTestReleaseAutomationRepository(t)
	ctx := context.Background()
	now := time.Unix(1700000000, 0).UTC()

	if err := repo.Create(ctx, newTestReleaseAutomation("rauto-1", "app-1", "prod", "main", now)); err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	// 同应用 + 同环境 + 同分支：唯一键必须拒绝。
	duplicate := newTestReleaseAutomation("rauto-2", "app-1", "prod", "main", now)
	err := repo.Create(ctx, duplicate)
	if !errors.Is(err, domain.ErrDuplicated) {
		t.Fatalf("duplicate Create err = %v, want ErrDuplicated", err)
	}
	// 换环境就允许。
	if err := repo.Create(ctx, newTestReleaseAutomation("rauto-3", "app-1", "staging", "main", now)); err != nil {
		t.Fatalf("Create with another env failed: %v", err)
	}

	// 更新同样受唯一键约束：把 staging 那条改回 prod/main 必须失败。
	other, err := repo.GetByID(ctx, "rauto-3")
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	other.EnvCode = "prod"
	if err := repo.Update(ctx, other); !errors.Is(err, domain.ErrDuplicated) {
		t.Fatalf("conflicting Update err = %v, want ErrDuplicated", err)
	}

	updated, err := repo.GetByID(ctx, "rauto-1")
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if err := repo.Update(ctx, updated); err != nil {
		t.Fatalf("self Update failed: %v", err)
	}
}

func TestReleaseAutomationRepositoryCommitTriggerIsConditional(t *testing.T) {
	t.Parallel()

	repo, _ := newTestReleaseAutomationRepository(t)
	ctx := context.Background()
	now := time.Unix(1700000000, 0).UTC()

	item := newTestReleaseAutomation("rauto-1", "app-1", "prod", "main", now)
	item.LastSeenSHA = "sha-1"
	if err := repo.Create(ctx, item); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// 过期的期望值：另一个副本已经推进过基线，本次必须不生效。
	committed, err := repo.CommitTrigger(ctx, item.ID, "sha-0", "sha-2", "sha-2", "ro-1", now.Add(time.Minute), "")
	if err != nil {
		t.Fatalf("CommitTrigger failed: %v", err)
	}
	if committed {
		t.Fatalf("CommitTrigger with a stale expectation committed, want false")
	}
	unchanged, err := repo.GetByID(ctx, item.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if unchanged.LastSeenSHA != "sha-1" || unchanged.LastOrderID != "" || unchanged.LastCheckedAt != nil {
		t.Fatalf("stale CAS changed the row: %+v", unchanged)
	}

	// 期望值匹配：推进基线并记录触发痕迹。
	committed, err = repo.CommitTrigger(ctx, item.ID, "sha-1", "sha-2", "sha-2", "ro-1", now.Add(2*time.Minute), "")
	if err != nil {
		t.Fatalf("CommitTrigger failed: %v", err)
	}
	if !committed {
		t.Fatalf("CommitTrigger with the matching expectation did not commit")
	}
	advanced, err := repo.GetByID(ctx, item.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if advanced.LastSeenSHA != "sha-2" || advanced.LastTriggeredSHA != "sha-2" || advanced.LastOrderID != "ro-1" {
		t.Fatalf("advanced row = %+v", advanced)
	}
	if advanced.LastCheckedAt == nil || !advanced.LastCheckedAt.Equal(now.Add(2*time.Minute)) {
		t.Fatalf("LastCheckedAt = %v", advanced.LastCheckedAt)
	}
	if advanced.LastError != "" {
		t.Fatalf("LastError = %q, want empty", advanced.LastError)
	}

	// 第二次用同一个期望值：基线已变，必须失败（这就是防重复建单的那道闸）。
	committed, err = repo.CommitTrigger(ctx, item.ID, "sha-1", "sha-3", "sha-3", "ro-2", now.Add(3*time.Minute), "")
	if err != nil {
		t.Fatalf("CommitTrigger failed: %v", err)
	}
	if committed {
		t.Fatalf("replayed CAS committed, want false")
	}
}

func TestReleaseAutomationRepositoryResetBaselineIsGuarded(t *testing.T) {
	t.Parallel()

	repo, _ := newTestReleaseAutomationRepository(t)
	ctx := context.Background()
	now := time.Unix(1700000000, 0).UTC()

	item := newTestReleaseAutomation("rauto-1", "app-1", "prod", "main", now)
	item.LastSeenSHA = "sha-1"
	item.LastTriggeredSHA = "sha-1"
	item.LastOrderID = "ro-1"
	if err := repo.Create(ctx, item); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// 换了分支：条件匹配，重设基线并清掉上一次的触发痕迹。
	if err := repo.Update(ctx, func() domain.Automation {
		updated := item
		updated.GitRef = "release/2026-09-19"
		updated.UpdatedAt = now.Add(time.Minute)
		return updated
	}()); err != nil {
		t.Fatalf("Update failed: %v", err)
	}
	committed, err := repo.ResetBaseline(ctx, item.ID, "app-1", "prod", "release/2026-09-19", "sha-1", "sha-9", now.Add(time.Minute))
	if err != nil {
		t.Fatalf("ResetBaseline failed: %v", err)
	}
	if !committed {
		t.Fatalf("ResetBaseline did not commit on a matching row")
	}
	reset, err := repo.GetByID(ctx, item.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if reset.LastSeenSHA != "sha-9" {
		t.Fatalf("LastSeenSHA = %q, want sha-9", reset.LastSeenSHA)
	}
	if reset.LastTriggeredSHA != "" || reset.LastOrderID != "" {
		t.Fatalf("stale trigger trace = %q / %q, want empty", reset.LastTriggeredSHA, reset.LastOrderID)
	}

	// 期望值过期（轮询已经推进过）：不得覆盖。
	committed, err = repo.ResetBaseline(ctx, item.ID, "app-1", "prod", "release/2026-09-19", "sha-1", "sha-10", now.Add(2*time.Minute))
	if err != nil {
		t.Fatalf("ResetBaseline failed: %v", err)
	}
	if committed {
		t.Fatalf("ResetBaseline with a stale expectation committed, want false")
	}
	// 身份不匹配（例如另一个编辑者已改回）：同样不得写入。
	committed, err = repo.ResetBaseline(ctx, item.ID, "app-1", "prod", "main", "sha-9", "sha-11", now.Add(3*time.Minute))
	if err != nil {
		t.Fatalf("ResetBaseline failed: %v", err)
	}
	if committed {
		t.Fatalf("ResetBaseline with a mismatching identity committed, want false")
	}
	unchanged, err := repo.GetByID(ctx, item.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if unchanged.LastSeenSHA != "sha-9" {
		t.Fatalf("LastSeenSHA = %q, want sha-9 kept", unchanged.LastSeenSHA)
	}
}

func TestReleaseAutomationRepositoryUpdateCheckStateKeepsBaseline(t *testing.T) {
	t.Parallel()

	repo, _ := newTestReleaseAutomationRepository(t)
	ctx := context.Background()
	now := time.Unix(1700000000, 0).UTC()

	item := newTestReleaseAutomation("rauto-1", "app-1", "prod", "main", now)
	if err := repo.Create(ctx, item); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	checkedAt := now.Add(time.Minute)
	if err := repo.UpdateCheckState(ctx, item.ID, checkedAt, "分支不存在"); err != nil {
		t.Fatalf("UpdateCheckState failed: %v", err)
	}
	got, err := repo.GetByID(ctx, item.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if got.LastError != "分支不存在" {
		t.Fatalf("LastError = %q", got.LastError)
	}
	if got.LastCheckedAt == nil || !got.LastCheckedAt.Equal(checkedAt) {
		t.Fatalf("LastCheckedAt = %v, want %v", got.LastCheckedAt, checkedAt)
	}
	if got.LastSeenSHA != "sha-1" {
		t.Fatalf("LastSeenSHA = %q, want the baseline untouched", got.LastSeenSHA)
	}

	// 恢复正常后错误必须被清空。
	if err := repo.UpdateCheckState(ctx, item.ID, checkedAt.Add(time.Minute), ""); err != nil {
		t.Fatalf("UpdateCheckState failed: %v", err)
	}
	got, err = repo.GetByID(ctx, item.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if got.LastError != "" {
		t.Fatalf("LastError = %q, want empty", got.LastError)
	}
}

func TestReleaseAutomationRepositoryListEnabledSkipsDisabledAndCapsLimit(t *testing.T) {
	t.Parallel()

	repo, _ := newTestReleaseAutomationRepository(t)
	ctx := context.Background()
	now := time.Unix(1700000000, 0).UTC()

	enabled := newTestReleaseAutomation("rauto-1", "app-1", "prod", "main", now)
	disabled := newTestReleaseAutomation("rauto-2", "app-2", "prod", "main", now)
	disabled.Enabled = false
	for _, item := range []domain.Automation{enabled, disabled} {
		if err := repo.Create(ctx, item); err != nil {
			t.Fatalf("Create failed: %v", err)
		}
	}

	items, err := repo.ListEnabled(ctx, 10)
	if err != nil {
		t.Fatalf("ListEnabled failed: %v", err)
	}
	if len(items) != 1 || items[0].ID != enabled.ID {
		t.Fatalf("ListEnabled returned %+v, want only the enabled config", items)
	}

	// 从未检查过的配置排在前面：给 enabled 补一次检查时间后它必须排到后面。
	if err := repo.UpdateCheckState(ctx, "rauto-1", now.Add(time.Minute), ""); err != nil {
		t.Fatalf("UpdateCheckState failed: %v", err)
	}
	fresh := newTestReleaseAutomation("rauto-3", "app-3", "prod", "main", now.Add(time.Second))
	if err := repo.Create(ctx, fresh); err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	items, err = repo.ListEnabled(ctx, 1)
	if err != nil {
		t.Fatalf("ListEnabled failed: %v", err)
	}
	if len(items) != 1 || items[0].ID != "rauto-3" {
		t.Fatalf("ListEnabled limit=1 returned %+v, want the never checked config first", items)
	}
}
