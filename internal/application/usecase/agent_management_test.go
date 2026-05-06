package usecase

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"

	agentdomain "gos/internal/domain/agent"
	"gos/internal/infrastructure/persistence/sqlrepo"

	_ "modernc.org/sqlite"
)

// TestAgentManagerDeleteBlocksActiveTasks 删除业务资源并返回处理结果。
func TestAgentManagerDeleteBlocksActiveTasks(t *testing.T) {
	t.Parallel()

	manager, repo := newTestAgentManager(t)
	ctx := context.Background()
	now := time.Now().UTC()

	instance := agentdomain.Instance{
		ID:             "agt-delete-blocked",
		AgentCode:      "agent-delete-blocked",
		Name:           "Delete Blocked",
		WorkDir:        "/tmp/delete-blocked",
		Token:          "token-delete-blocked",
		Status:         agentdomain.StatusActive,
		LastTaskStatus: agentdomain.LastTaskStatusUnknown,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if _, err := repo.CreateInstance(ctx, instance); err != nil {
		t.Fatalf("CreateInstance failed: %v", err)
	}
	if _, err := repo.CreateTask(ctx, agentdomain.Task{
		ID:         "agtask-delete-running",
		AgentID:    instance.ID,
		AgentCode:  instance.AgentCode,
		Name:       "running-task",
		TaskMode:   agentdomain.TaskModeTemporary,
		TaskType:   string(agentdomain.TaskTypeShellScript),
		ShellType:  "sh",
		WorkDir:    instance.WorkDir,
		ScriptText: "echo running",
		Variables:  map[string]string{},
		TimeoutSec: 30,
		Status:     agentdomain.TaskStatusRunning,
		CreatedBy:  "test",
		CreatedAt:  now,
		UpdatedAt:  now,
	}); err != nil {
		t.Fatalf("CreateTask failed: %v", err)
	}

	err := manager.Delete(ctx, instance.ID)
	if !errors.Is(err, agentdomain.ErrInstanceDeleteBlocked) {
		t.Fatalf("Delete error = %v, want %v", err, agentdomain.ErrInstanceDeleteBlocked)
	}
}

// TestAgentManagerDeleteCleansAgentBindings 删除业务资源并返回处理结果。
func TestAgentManagerDeleteCleansAgentBindings(t *testing.T) {
	t.Parallel()

	manager, repo := newTestAgentManager(t)
	ctx := context.Background()
	now := time.Now().UTC()

	toDelete := agentdomain.Instance{
		ID:             "agt-delete-target",
		AgentCode:      "agent-delete-target",
		Name:           "Delete Target",
		WorkDir:        "/tmp/delete-target",
		Token:          "token-delete-target",
		Status:         agentdomain.StatusActive,
		LastTaskStatus: agentdomain.LastTaskStatusUnknown,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if _, err := repo.CreateInstance(ctx, toDelete); err != nil {
		t.Fatalf("CreateInstance target failed: %v", err)
	}
	keep := agentdomain.Instance{
		ID:             "agt-keep-target",
		AgentCode:      "agent-keep-target",
		Name:           "Keep Target",
		WorkDir:        "/tmp/keep-target",
		Token:          "token-keep-target",
		Status:         agentdomain.StatusActive,
		LastTaskStatus: agentdomain.LastTaskStatusUnknown,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if _, err := repo.CreateInstance(ctx, keep); err != nil {
		t.Fatalf("CreateInstance keep failed: %v", err)
	}

	if _, err := repo.CreateTask(ctx, agentdomain.Task{
		ID:         "resident-delete",
		AgentID:    toDelete.ID,
		AgentCode:  toDelete.AgentCode,
		Name:       "resident-delete",
		TaskMode:   agentdomain.TaskModeResident,
		TaskType:   string(agentdomain.TaskTypeShellScript),
		ShellType:  "sh",
		WorkDir:    toDelete.WorkDir,
		ScriptText: "echo resident",
		Variables:  map[string]string{},
		TimeoutSec: 30,
		Status:     agentdomain.TaskStatusCancelled,
		CreatedBy:  "test",
		CreatedAt:  now,
		UpdatedAt:  now,
	}); err != nil {
		t.Fatalf("CreateTask resident failed: %v", err)
	}
	if _, err := repo.CreateTask(ctx, agentdomain.Task{
		ID:            "temporary-history",
		AgentID:       toDelete.ID,
		AgentCode:     toDelete.AgentCode,
		Name:          "temporary-history",
		TaskMode:      agentdomain.TaskModeTemporary,
		TaskType:      string(agentdomain.TaskTypeShellScript),
		ShellType:     "sh",
		WorkDir:       toDelete.WorkDir,
		ScriptText:    "echo history",
		Variables:     map[string]string{},
		TimeoutSec:    30,
		Status:        agentdomain.TaskStatusSuccess,
		LastRunStatus: agentdomain.TaskStatusSuccess,
		CreatedBy:     "test",
		CreatedAt:     now,
		UpdatedAt:     now,
	}); err != nil {
		t.Fatalf("CreateTask temporary history failed: %v", err)
	}
	if _, err := repo.CreateTask(ctx, agentdomain.Task{
		ID:         "temporary-draft",
		AgentID:    toDelete.ID,
		AgentCode:  toDelete.AgentCode,
		Name:       "temporary-draft",
		TaskMode:   agentdomain.TaskModeTemporary,
		TaskType:   string(agentdomain.TaskTypeShellScript),
		ShellType:  "sh",
		WorkDir:    toDelete.WorkDir,
		ScriptText: "echo draft",
		Variables:  map[string]string{},
		TimeoutSec: 30,
		Status:     agentdomain.TaskStatusDraft,
		CreatedBy:  "test",
		CreatedAt:  now,
		UpdatedAt:  now,
	}); err != nil {
		t.Fatalf("CreateTask temporary draft failed: %v", err)
	}
	if _, err := repo.CreateTask(ctx, agentdomain.Task{
		ID:             "temporary-template",
		AgentID:        "",
		AgentCode:      "",
		TargetAgentIDs: []string{toDelete.ID, keep.ID},
		Name:           "temporary-template",
		TaskMode:       agentdomain.TaskModeTemporary,
		TaskType:       string(agentdomain.TaskTypeShellScript),
		ShellType:      "sh",
		WorkDir:        "/tmp/template",
		ScriptText:     "echo template",
		Variables:      map[string]string{},
		TimeoutSec:     30,
		Status:         agentdomain.TaskStatusDraft,
		CreatedBy:      "test",
		CreatedAt:      now,
		UpdatedAt:      now,
	}); err != nil {
		t.Fatalf("CreateTask template failed: %v", err)
	}

	if err := manager.Delete(ctx, toDelete.ID); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	if _, err := repo.GetInstanceByID(ctx, toDelete.ID); !errors.Is(err, agentdomain.ErrInstanceNotFound) {
		t.Fatalf("GetInstanceByID after delete error = %v, want %v", err, agentdomain.ErrInstanceNotFound)
	}
	if _, err := repo.GetTaskByID(ctx, "resident-delete"); !errors.Is(err, agentdomain.ErrTaskNotFound) {
		t.Fatalf("resident task error = %v, want %v", err, agentdomain.ErrTaskNotFound)
	}

	historyTask, err := repo.GetTaskByID(ctx, "temporary-history")
	if err != nil {
		t.Fatalf("GetTaskByID temporary-history failed: %v", err)
	}
	if historyTask.AgentID != "" {
		t.Fatalf("history task agent_id = %q, want empty", historyTask.AgentID)
	}
	if historyTask.AgentCode != toDelete.AgentCode {
		t.Fatalf("history task agent_code = %q, want %q", historyTask.AgentCode, toDelete.AgentCode)
	}
	if historyTask.Status != agentdomain.TaskStatusSuccess {
		t.Fatalf("history task status = %s, want %s", historyTask.Status, agentdomain.TaskStatusSuccess)
	}

	draftTask, err := repo.GetTaskByID(ctx, "temporary-draft")
	if err != nil {
		t.Fatalf("GetTaskByID temporary-draft failed: %v", err)
	}
	if draftTask.AgentID != "" {
		t.Fatalf("draft task agent_id = %q, want empty", draftTask.AgentID)
	}
	if draftTask.Status != agentdomain.TaskStatusCancelled {
		t.Fatalf("draft task status = %s, want %s", draftTask.Status, agentdomain.TaskStatusCancelled)
	}
	if !strings.Contains(draftTask.LastRunSummary, "原绑定 Agent 已删除") {
		t.Fatalf("draft task last_run_summary = %q, want delete hint", draftTask.LastRunSummary)
	}
	if !strings.Contains(draftTask.FailureReason, "原绑定 Agent 已删除") {
		t.Fatalf("draft task failure_reason = %q, want delete hint", draftTask.FailureReason)
	}

	templateTask, err := repo.GetTaskByID(ctx, "temporary-template")
	if err != nil {
		t.Fatalf("GetTaskByID temporary-template failed: %v", err)
	}
	if len(templateTask.TargetAgentIDs) != 1 || templateTask.TargetAgentIDs[0] != keep.ID {
		t.Fatalf("template target_agent_ids = %v, want [%s]", templateTask.TargetAgentIDs, keep.ID)
	}
}

// newTestAgentManager 封装当前模块的业务处理逻辑。
func newTestAgentManager(t *testing.T) (*AgentManager, *sqlrepo.AgentRepository) {
	t.Helper()

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("sql.Open failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	repo := sqlrepo.NewAgentRepository(db, "sqlite")
	if err := repo.InitSchema(context.Background()); err != nil {
		t.Fatalf("InitSchema failed: %v", err)
	}
	return NewAgentManager(repo), repo
}
