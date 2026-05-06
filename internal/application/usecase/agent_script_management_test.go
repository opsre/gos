package usecase

import (
	"context"
	"database/sql"
	"testing"
	"time"

	agentdomain "gos/internal/domain/agent"
	"gos/internal/infrastructure/persistence/sqlrepo"

	_ "modernc.org/sqlite"
)

// TestAgentScriptUpdateSyncsBoundTasks 更新业务资源并返回处理结果。
func TestAgentScriptUpdateSyncsBoundTasks(t *testing.T) {
	t.Parallel()

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("sql.Open failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	repo := sqlrepo.NewAgentRepository(db, "sqlite")
	ctx := context.Background()
	if err := repo.InitSchema(ctx); err != nil {
		t.Fatalf("InitSchema failed: %v", err)
	}

	now := time.Now().UTC()
	scriptManager := NewAgentScriptManager(repo)
	scriptManager.now = func() time.Time { return now }
	taskManager := NewAgentTaskManager(repo)
	taskManager.now = func() time.Time { return now }

	instance, err := repo.CreateInstance(ctx, agentdomain.Instance{
		ID:             "agt-script-sync",
		AgentCode:      "agent-script-sync",
		Name:           "Agent Script Sync",
		WorkDir:        "/tmp/agent-script-sync",
		Token:          "token-script-sync",
		Status:         agentdomain.StatusActive,
		LastTaskStatus: agentdomain.LastTaskStatusUnknown,
		CreatedAt:      now,
		UpdatedAt:      now,
	})
	if err != nil {
		t.Fatalf("CreateInstance failed: %v", err)
	}

	createdScript, err := scriptManager.Create(ctx, CreateAgentScriptInput{
		Name:       "部署脚本",
		TaskType:   string(agentdomain.TaskTypeShellScript),
		ShellType:  "sh",
		ScriptText: "echo old",
		CreatedBy:  "tester",
	})
	if err != nil {
		t.Fatalf("Create script failed: %v", err)
	}

	temporaryTask, err := taskManager.Create(ctx, CreateAgentTaskInput{
		Name:      "模板任务",
		TaskMode:  string(agentdomain.TaskModeTemporary),
		ScriptID:  createdScript.ID,
		CreatedBy: "tester",
	})
	if err != nil {
		t.Fatalf("Create temporary task failed: %v", err)
	}
	residentTask, err := taskManager.Create(ctx, CreateAgentTaskInput{
		AgentID:   instance.ID,
		Name:      "常驻任务",
		TaskMode:  string(agentdomain.TaskModeResident),
		ScriptID:  createdScript.ID,
		CreatedBy: "tester",
	})
	if err != nil {
		t.Fatalf("Create resident task failed: %v", err)
	}

	updatedScript, err := scriptManager.Update(ctx, createdScript.ID, UpdateAgentScriptInput{
		Name:       "部署脚本-新版",
		TaskType:   string(agentdomain.TaskTypeScriptFile),
		ShellType:  "bash",
		ScriptPath: "deploy.sh",
		ScriptText: "echo new",
		UpdatedBy:  "tester",
	})
	if err != nil {
		t.Fatalf("Update script failed: %v", err)
	}
	if updatedScript.TaskType != string(agentdomain.TaskTypeScriptFile) {
		t.Fatalf("updated script task_type = %q, want %q", updatedScript.TaskType, agentdomain.TaskTypeScriptFile)
	}

	reloadedTemporary, err := repo.GetTaskByID(ctx, temporaryTask.ID)
	if err != nil {
		t.Fatalf("GetTaskByID temporary failed: %v", err)
	}
	if reloadedTemporary.ScriptName != "部署脚本-新版" {
		t.Fatalf("temporary task script_name = %q, want updated name", reloadedTemporary.ScriptName)
	}
	if reloadedTemporary.TaskType != string(agentdomain.TaskTypeScriptFile) {
		t.Fatalf("temporary task task_type = %q, want %q", reloadedTemporary.TaskType, agentdomain.TaskTypeScriptFile)
	}
	if reloadedTemporary.ShellType != "bash" {
		t.Fatalf("temporary task shell_type = %q, want bash", reloadedTemporary.ShellType)
	}
	if reloadedTemporary.ScriptPath != "deploy.sh" {
		t.Fatalf("temporary task script_path = %q, want deploy.sh", reloadedTemporary.ScriptPath)
	}
	if reloadedTemporary.ScriptText != "echo new" {
		t.Fatalf("temporary task script_text = %q, want updated content", reloadedTemporary.ScriptText)
	}

	reloadedResident, err := repo.GetTaskByID(ctx, residentTask.ID)
	if err != nil {
		t.Fatalf("GetTaskByID resident failed: %v", err)
	}
	if reloadedResident.ScriptName != "部署脚本-新版" {
		t.Fatalf("resident task script_name = %q, want updated name", reloadedResident.ScriptName)
	}
	if reloadedResident.ScriptText != "echo new" {
		t.Fatalf("resident task script_text = %q, want updated content", reloadedResident.ScriptText)
	}
}

// TestAgentTaskManagerUsesLatestManagedScriptForStaleTaskSnapshots 封装当前模块的业务处理逻辑。
func TestAgentTaskManagerUsesLatestManagedScriptForStaleTaskSnapshots(t *testing.T) {
	t.Parallel()

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("sql.Open failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	repo := sqlrepo.NewAgentRepository(db, "sqlite")
	ctx := context.Background()
	if err := repo.InitSchema(ctx); err != nil {
		t.Fatalf("InitSchema failed: %v", err)
	}

	now := time.Now().UTC()
	manager := NewAgentTaskManager(repo)
	manager.now = func() time.Time { return now }

	agents := []agentdomain.Instance{
		{
			ID:             "agt-sync-1",
			AgentCode:      "agent-sync-1",
			Name:           "Agent Sync 1",
			WorkDir:        "/tmp/agent-sync-1",
			Token:          "token-sync-1",
			Status:         agentdomain.StatusActive,
			LastTaskStatus: agentdomain.LastTaskStatusUnknown,
			CreatedAt:      now,
			UpdatedAt:      now,
		},
		{
			ID:             "agt-sync-2",
			AgentCode:      "agent-sync-2",
			Name:           "Agent Sync 2",
			WorkDir:        "/tmp/agent-sync-2",
			Token:          "token-sync-2",
			Status:         agentdomain.StatusActive,
			LastTaskStatus: agentdomain.LastTaskStatusUnknown,
			CreatedAt:      now,
			UpdatedAt:      now,
		},
	}
	for _, item := range agents {
		if _, err := repo.CreateInstance(ctx, item); err != nil {
			t.Fatalf("CreateInstance failed: %v", err)
		}
	}

	script, err := repo.CreateScript(ctx, agentdomain.Script{
		ID:         "agtscr-stale",
		Name:       "旧脚本",
		TaskType:   string(agentdomain.TaskTypeShellScript),
		ShellType:  "sh",
		ScriptText: "echo old",
		CreatedBy:  "tester",
		UpdatedBy:  "tester",
		CreatedAt:  now,
		UpdatedAt:  now,
	})
	if err != nil {
		t.Fatalf("CreateScript failed: %v", err)
	}

	sourceTask, err := repo.CreateTask(ctx, agentdomain.Task{
		ID:             "agtask-stale-source",
		Name:           "过期模板任务",
		TaskMode:       agentdomain.TaskModeTemporary,
		TaskType:       string(agentdomain.TaskTypeShellScript),
		ShellType:      "sh",
		WorkDir:        "/tmp/template",
		ScriptID:       script.ID,
		ScriptName:     "旧脚本",
		ScriptText:     "echo old",
		TimeoutSec:     60,
		Status:         agentdomain.TaskStatusDraft,
		TargetAgentIDs: []string{agents[0].ID, agents[1].ID},
		CreatedBy:      "tester",
		CreatedAt:      now,
		UpdatedAt:      now,
	})
	if err != nil {
		t.Fatalf("CreateTask failed: %v", err)
	}

	_, err = repo.UpdateScript(ctx, agentdomain.Script{
		ID:         script.ID,
		Name:       "新脚本",
		TaskType:   string(agentdomain.TaskTypeScriptFile),
		ShellType:  "bash",
		ScriptPath: "deploy.sh",
		ScriptText: "echo latest",
		CreatedBy:  script.CreatedBy,
		UpdatedBy:  "tester",
		CreatedAt:  script.CreatedAt,
		UpdatedAt:  now.Add(time.Minute),
	})
	if err != nil {
		t.Fatalf("UpdateScript failed: %v", err)
	}

	got, err := manager.Get(ctx, sourceTask.ID)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if got.ScriptName != "新脚本" || got.ScriptText != "echo latest" {
		t.Fatalf("Get returned stale script snapshot: %#v", got)
	}

	updatedSource, err := manager.Execute(ctx, sourceTask.ID, ExecuteAgentTaskInput{})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	children, _, err := repo.ListTasks(ctx, agentdomain.TaskListFilter{
		SourceTaskID:    sourceTask.ID,
		DispatchBatchID: updatedSource.DispatchBatchID,
		Page:            1,
		PageSize:        10,
	})
	if err != nil {
		t.Fatalf("ListTasks children failed: %v", err)
	}
	if len(children) != 2 {
		t.Fatalf("len(children) = %d, want 2", len(children))
	}
	for _, child := range children {
		if child.ScriptName != "新脚本" {
			t.Fatalf("child %s script_name = %q, want 新脚本", child.ID, child.ScriptName)
		}
		if child.ScriptPath != "deploy.sh" {
			t.Fatalf("child %s script_path = %q, want deploy.sh", child.ID, child.ScriptPath)
		}
		if child.ScriptText != "echo latest" {
			t.Fatalf("child %s script_text = %q, want echo latest", child.ID, child.ScriptText)
		}
	}
}
