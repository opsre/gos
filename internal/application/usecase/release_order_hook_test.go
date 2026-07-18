package usecase

import (
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	agentdomain "gos/internal/domain/agent"
	appdomain "gos/internal/domain/application"
	notificationdomain "gos/internal/domain/notification"
	domain "gos/internal/domain/release"
	"gos/internal/infrastructure/persistence/sqlrepo"
)

// TestShouldTriggerTemplateHook 封装当前模块的业务处理逻辑。
func TestShouldTriggerTemplateHook(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		condition domain.TemplateHookTriggerCondition
		status    domain.OrderStatus
		want      bool
	}{
		{
			name:      "on_success with success",
			condition: domain.TemplateHookTriggerOnSuccess,
			status:    domain.OrderStatusSuccess,
			want:      true,
		},
		{
			name:      "on_success with failed",
			condition: domain.TemplateHookTriggerOnSuccess,
			status:    domain.OrderStatusFailed,
			want:      false,
		},
		{
			name:      "on_failed with failed",
			condition: domain.TemplateHookTriggerOnFailed,
			status:    domain.OrderStatusFailed,
			want:      true,
		},
		{
			name:      "on_failed with cancelled",
			condition: domain.TemplateHookTriggerOnFailed,
			status:    domain.OrderStatusCancelled,
			want:      true,
		},
		{
			name:      "on_failed with success",
			condition: domain.TemplateHookTriggerOnFailed,
			status:    domain.OrderStatusSuccess,
			want:      false,
		},
		{
			name:      "always with success",
			condition: domain.TemplateHookTriggerAlways,
			status:    domain.OrderStatusSuccess,
			want:      true,
		},
		{
			name:      "always with failed",
			condition: domain.TemplateHookTriggerAlways,
			status:    domain.OrderStatusFailed,
			want:      true,
		},
		{
			name:      "always with cancelled",
			condition: domain.TemplateHookTriggerAlways,
			status:    domain.OrderStatusCancelled,
			want:      true,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := shouldTriggerTemplateHook(tc.condition, tc.status)
			if got != tc.want {
				t.Fatalf("shouldTriggerTemplateHook(%q, %q) = %v, want %v", tc.condition, tc.status, got, tc.want)
			}
		})
	}
}

// TestHookMatchesOrderEnv 封装当前模块的业务处理逻辑。
func TestHookMatchesOrderEnv(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		envCodes  []string
		orderEnv  string
		wantMatch bool
	}{
		{
			name:      "empty hook envs means all envs",
			envCodes:  nil,
			orderEnv:  "prod",
			wantMatch: true,
		},
		{
			name:      "single env match",
			envCodes:  []string{"prod"},
			orderEnv:  "prod",
			wantMatch: true,
		},
		{
			name:      "case insensitive match",
			envCodes:  []string{"Prod"},
			orderEnv:  "prod",
			wantMatch: true,
		},
		{
			name:      "env not matched",
			envCodes:  []string{"prod"},
			orderEnv:  "dev",
			wantMatch: false,
		},
		{
			name:      "blank order env does not match filtered hook",
			envCodes:  []string{"prod"},
			orderEnv:  "",
			wantMatch: false,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := hookMatchesOrderEnv(tc.envCodes, tc.orderEnv); got != tc.wantMatch {
				t.Fatalf("hookMatchesOrderEnv(%v, %q) = %v, want %v", tc.envCodes, tc.orderEnv, got, tc.wantMatch)
			}
		})
	}
}

// TestBuildTemplateHookEnvSkipMessage 组装业务执行所需的输入数据。
func TestBuildTemplateHookEnvSkipMessage(t *testing.T) {
	t.Parallel()

	got := buildTemplateHookEnvSkipMessage([]string{"prod", "pre"}, "dev")
	want := "当前环境 dev 未命中 Hook 执行环境（prod / pre），已跳过"
	if got != want {
		t.Fatalf("buildTemplateHookEnvSkipMessage mismatch: got %q want %q", got, want)
	}
}

// TestParseHookTaskBatchIdentity 解析输入内容并返回结构化结果。
func TestParseHookTaskBatchIdentity(t *testing.T) {
	t.Parallel()

	message := buildHookTaskBatchProgressMessage(
		domain.ReleaseTemplateHook{Name: "发布后校验", TargetName: "发布后校验"},
		agentdomain.Task{ID: "agtask-source", Name: "发布后校验"},
		[]agentdomain.Task{
			{ID: "agtask-1", Status: agentdomain.TaskStatusPending},
			{ID: "agtask-2", Status: agentdomain.TaskStatusRunning},
		},
		"agbatch-1",
	)
	sourceTaskID, batchID := parseHookTaskBatchIdentity(message)
	if sourceTaskID != "agtask-source" {
		t.Fatalf("sourceTaskID = %q, want %q", sourceTaskID, "agtask-source")
	}
	if batchID != "agbatch-1" {
		t.Fatalf("batchID = %q, want %q", batchID, "agbatch-1")
	}
}

// TestParseHookTaskIDFromTerminalMessage 解析输入内容并返回结构化结果。
func TestParseHookTaskIDFromTerminalMessage(t *testing.T) {
	t.Parallel()

	message := buildHookTaskTerminalMessage(
		domain.ReleaseTemplateHook{Name: "发布后校验", TargetName: "发布后校验"},
		agentdomain.Task{ID: "agtask-123", Name: "发布后校验", LastRunSummary: "执行完成"},
		"执行成功",
	)
	if got := parseHookTaskID(message); got != "agtask-123" {
		t.Fatalf("parseHookTaskID(%q) = %q, want %q", message, got, "agtask-123")
	}
}

// TestParseHookExecuteStage 解析输入内容并返回结构化结果。
func TestParseHookExecuteStage(t *testing.T) {
	t.Parallel()

	if got := parseHookExecuteStage("hook:build_complete:webhook_notification:3"); got != domain.TemplateHookExecuteStageBuildComplete {
		t.Fatalf("parseHookExecuteStage(new code) = %q, want %q", got, domain.TemplateHookExecuteStageBuildComplete)
	}
	if got := parseHookExecuteStage("hook:webhook_notification:3"); got != domain.TemplateHookExecuteStagePostRelease {
		t.Fatalf("parseHookExecuteStage(legacy code) = %q, want %q", got, domain.TemplateHookExecuteStagePostRelease)
	}
}

// TestMergeAgentTaskVariablesOverridesReleaseVariables 封装当前模块的业务处理逻辑。
func TestMergeAgentTaskVariablesOverridesReleaseVariables(t *testing.T) {
	t.Parallel()

	values := map[string]string{
		"env":          "prod",
		"artifact_url": "https://release.example.com/a.jar",
		"image_tag":    "100",
	}

	mergeAgentTaskVariables(values, map[string]string{
		"artifact_url": "https://agent.example.com/b.jar",
		" custom_key ": " custom-value ",
		"   ":          "ignored",
	})

	if got := values["artifact_url"]; got != "https://agent.example.com/b.jar" {
		t.Fatalf("artifact_url = %q, want agent variable override", got)
	}
	if got := values["custom_key"]; got != "custom-value" {
		t.Fatalf("custom_key = %q, want trimmed custom value", got)
	}
	if got := values["env"]; got != "prod" {
		t.Fatalf("env = %q, want untouched release variable", got)
	}
}

// TestBuildHookTaskVariablesIncludesReleaseName 通知 Hook 变量应包含发布名称。
func TestBuildHookTaskVariablesIncludesReleaseName(t *testing.T) {
	t.Parallel()

	manager, _ := newReleaseOrderManagerForCancelTest(t)
	values, err := manager.buildHookTaskVariables(context.Background(), domain.ReleaseOrder{
		ID:          "ro-hook-release-name",
		OrderNo:     "RO-HOOK-RELEASE-NAME",
		ReleaseName: "南部新城智慧文旅正式发布",
		EnvCode:     "prod",
		Status:      domain.OrderStatusSuccess,
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}, nil, domain.ReleaseTemplateHook{}, domain.TemplateHookExecuteStagePostRelease)
	if err != nil {
		t.Fatalf("buildHookTaskVariables failed: %v", err)
	}
	if got := values["release_name"]; got != "南部新城智慧文旅正式发布" {
		t.Fatalf("release_name = %q, want %q", got, "南部新城智慧文旅正式发布")
	}
}

// TestBuildHookTaskVariablesUsesCIOnlyForGOSArtifactURL Hook 变量中的 GOS 制品地址只能来自 CI 单元。
func TestBuildHookTaskVariablesUsesCIOnlyForGOSArtifactURL(t *testing.T) {
	t.Parallel()

	manager, repo := newReleaseOrderManagerForCancelTest(t)
	ctx := context.Background()
	now := time.Now().UTC()
	order := testReleaseOrder("ro-hook-gos-artifact", "RO-HOOK-GOS-ARTIFACT", domain.OrderStatusSuccess, now)
	params := []domain.ReleaseOrderParam{
		{
			ID:             "rop-hook-gos-artifact-cd",
			ReleaseOrderID: order.ID,
			PipelineScope:  domain.PipelineScopeCD,
			ParamKey:       "gos_artifact_url",
			ParamValue:     "https://cd.example.com/should-not-use.jar",
			CreatedAt:      now,
		},
		{
			ID:             "rop-hook-gos-artifact-ci",
			ReleaseOrderID: order.ID,
			PipelineScope:  domain.PipelineScopeCI,
			ParamKey:       "gos_artifact_url",
			ParamValue:     "https://ci.example.com/app.jar",
			CreatedAt:      now,
		},
	}
	if err := repo.Create(ctx, order, nil, params, nil); err != nil {
		t.Fatalf("Create release order failed: %v", err)
	}

	values, err := manager.buildHookTaskVariables(ctx, order, nil, domain.ReleaseTemplateHook{}, domain.TemplateHookExecuteStagePostRelease)
	if err != nil {
		t.Fatalf("buildHookTaskVariables failed: %v", err)
	}
	if got := values["gos_artifact_url"]; got != "https://ci.example.com/app.jar" {
		t.Fatalf("gos_artifact_url = %q, want CI value", got)
	}
}

// TestBuildHookTaskVariablesUsesApplicationForGOSArtifactPath Hook 变量中的 GOS 制品路径来自 App 基础信息。
func TestBuildHookTaskVariablesUsesApplicationForGOSArtifactPath(t *testing.T) {
	t.Parallel()

	manager, repo := newReleaseOrderManagerForCancelTest(t)
	manager.appRepo = applicationRepositoryStub{
		app: appdomain.Application{
			ID:                "app-1",
			Key:               "app-1",
			ArtifactDirectory: "release/pay-center",
		},
	}
	ctx := context.Background()
	now := time.Now().UTC()
	order := testReleaseOrder("ro-hook-gos-artifact-path", "RO-HOOK-GOS-ARTIFACT-PATH", domain.OrderStatusSuccess, now)
	order.ApplicationID = "app-1"
	params := []domain.ReleaseOrderParam{
		{
			ID:             "rop-hook-gos-artifact-path-cd",
			ReleaseOrderID: order.ID,
			PipelineScope:  domain.PipelineScopeCD,
			ParamKey:       "gos_artifact_path",
			ParamValue:     "release/from-cd-param",
			CreatedAt:      now,
		},
	}
	if err := repo.Create(ctx, order, nil, params, nil); err != nil {
		t.Fatalf("Create release order failed: %v", err)
	}

	values, err := manager.buildHookTaskVariables(ctx, order, nil, domain.ReleaseTemplateHook{}, domain.TemplateHookExecuteStagePostRelease)
	if err != nil {
		t.Fatalf("buildHookTaskVariables failed: %v", err)
	}
	if got := values["gos_artifact_path"]; got != "release/pay-center" {
		t.Fatalf("gos_artifact_path = %q, want app artifact directory", got)
	}
}

// TestSyncHooksAfterReleaseKeepsFinishStepRunningWhileAgentHookRunning 确保发布后 Agent Hook 未结束前不提前成功整体进度。
func TestSyncHooksAfterReleaseKeepsFinishStepRunningWhileAgentHookRunning(t *testing.T) {
	t.Parallel()

	manager, repo := newReleaseOrderManagerForCancelTest(t)
	ctx := context.Background()
	now := time.Now().UTC()
	startedAt := now.Add(-2 * time.Minute)
	manager.now = func() time.Time { return now }

	agentDB, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("sql.Open agent failed: %v", err)
	}
	t.Cleanup(func() { _ = agentDB.Close() })
	agentRepo := sqlrepo.NewAgentRepository(agentDB, "sqlite")
	if err := agentRepo.InitSchema(ctx); err != nil {
		t.Fatalf("agent InitSchema failed: %v", err)
	}
	manager.agentRepo = agentRepo

	template := domain.ReleaseTemplate{
		ID:              "rt-agent-hook-running",
		Name:            "Agent Hook Template",
		ApplicationID:   "app-1",
		ApplicationName: "App 1",
		BindingID:       "app-1",
		BindingName:     "App 1",
		BindingType:     "application",
		Status:          domain.TemplateStatusActive,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	hooks := []domain.ReleaseTemplateHook{
		{
			ID:               "hook-agent-running",
			TemplateID:       template.ID,
			HookType:         domain.TemplateHookTypeAgentTask,
			Name:             "发布后 Agent 校验",
			ExecuteStage:     domain.TemplateHookExecuteStagePostRelease,
			TriggerCondition: domain.TemplateHookTriggerOnSuccess,
			FailurePolicy:    domain.TemplateHookFailurePolicyBlockRelease,
			TargetID:         "agtask-source-running",
			TargetName:       "发布后 Agent 校验",
			SortNo:           1,
			CreatedAt:        now,
			UpdatedAt:        now,
		},
	}
	if err := repo.CreateTemplate(ctx, template, nil, nil, nil, hooks); err != nil {
		t.Fatalf("CreateTemplate failed: %v", err)
	}

	sourceTask := agentdomain.Task{
		ID:         "agtask-source-running",
		Name:       "发布后 Agent 校验",
		TaskMode:   agentdomain.TaskModeTemporary,
		TaskType:   string(agentdomain.TaskTypeShellScript),
		ShellType:  "sh",
		WorkDir:    "/tmp",
		ScriptText: "echo check",
		Variables:  map[string]string{},
		TimeoutSec: 30,
		Status:     agentdomain.TaskStatusDraft,
		CreatedBy:  "tester",
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	if _, err := agentRepo.CreateTask(ctx, sourceTask); err != nil {
		t.Fatalf("CreateTask source failed: %v", err)
	}
	runningTask := agentdomain.Task{
		ID:              "agtask-running-1",
		AgentID:         "agent-1",
		AgentCode:       "agent-1",
		SourceTaskID:    sourceTask.ID,
		DispatchBatchID: "agbatch-running",
		Name:            "发布后 Agent 校验",
		TaskMode:        agentdomain.TaskModeTemporary,
		TaskType:        string(agentdomain.TaskTypeShellScript),
		ShellType:       "sh",
		WorkDir:         "/tmp",
		ScriptText:      "echo check",
		Variables:       map[string]string{},
		TimeoutSec:      30,
		Status:          agentdomain.TaskStatusRunning,
		StartedAt:       &startedAt,
		CreatedBy:       "tester",
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	if _, err := agentRepo.CreateTask(ctx, runningTask); err != nil {
		t.Fatalf("CreateTask running failed: %v", err)
	}

	order := testReleaseOrder("ro-agent-hook-running", "RO-AGENT-HOOK-RUNNING", domain.OrderStatusRunning, now)
	order.TemplateID = template.ID
	order.TemplateName = template.Name
	order.StartedAt = &startedAt
	executions := []domain.ReleaseOrderExecution{
		testReleaseExecution(order.ID, "exec-ci", domain.PipelineScopeCI, domain.ExecutionStatusSuccess, now),
		testReleaseExecution(order.ID, "exec-cd", domain.PipelineScopeCD, domain.ExecutionStatusSuccess, now),
	}
	steps := defaultReleaseOrderSteps(order.ID, executions, now, "", hooks, order.EnvCode)
	for idx := range steps {
		switch steps[idx].StepCode {
		case "hook:post_release:agent_task:1":
			steps[idx].Status = domain.StepStatusRunning
			steps[idx].StartedAt = &startedAt
			steps[idx].Message = buildHookTaskBatchProgressMessage(hooks[0], sourceTask, []agentdomain.Task{runningTask}, runningTask.DispatchBatchID)
		case "global:release_finish":
			steps[idx].Status = domain.StepStatusPending
		default:
			steps[idx].Status = domain.StepStatusSuccess
			steps[idx].StartedAt = &startedAt
			steps[idx].FinishedAt = &now
		}
	}
	if err := repo.Create(ctx, order, executions, nil, steps); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	_, finished, status, _, err := manager.syncHooksAfterRelease(ctx, order, executions)
	if err != nil {
		t.Fatalf("syncHooksAfterRelease failed: %v", err)
	}
	if finished {
		t.Fatal("finished = true, want false while agent hook is running")
	}
	if status != domain.OrderStatusRunning {
		t.Fatalf("status = %s, want %s", status, domain.OrderStatusRunning)
	}

	storedSteps, err := repo.ListSteps(ctx, order.ID)
	if err != nil {
		t.Fatalf("ListSteps failed: %v", err)
	}
	finishStep := findStepByCode(storedSteps, "global:release_finish")
	if finishStep == nil {
		t.Fatal("finish step = nil")
	}
	if finishStep.Status == domain.StepStatusSuccess {
		t.Fatalf("finish step status = %s, want not success while agent hook is running", finishStep.Status)
	}
	if finishStep.Status != domain.StepStatusRunning {
		t.Fatalf("finish step status = %s, want %s", finishStep.Status, domain.StepStatusRunning)
	}
}

// TestFinalizeOrderOverridesPrematureSuccessFinishStepWhenAgentHookFailed 确保历史误标成功的整体进度可被阻塞 Hook 失败纠正。
func TestFinalizeOrderOverridesPrematureSuccessFinishStepWhenAgentHookFailed(t *testing.T) {
	t.Parallel()

	manager, repo := newReleaseOrderManagerForCancelTest(t)
	ctx := context.Background()
	now := time.Now().UTC()
	startedAt := now.Add(-2 * time.Minute)
	manager.now = func() time.Time { return now }

	template := domain.ReleaseTemplate{
		ID:              "rt-agent-hook-failed",
		Name:            "Agent Hook Failed Template",
		ApplicationID:   "app-1",
		ApplicationName: "App 1",
		BindingID:       "app-1",
		BindingName:     "App 1",
		BindingType:     "application",
		Status:          domain.TemplateStatusActive,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	hooks := []domain.ReleaseTemplateHook{
		{
			ID:               "hook-agent-failed",
			TemplateID:       template.ID,
			HookType:         domain.TemplateHookTypeAgentTask,
			Name:             "发布后 Agent 校验",
			ExecuteStage:     domain.TemplateHookExecuteStagePostRelease,
			TriggerCondition: domain.TemplateHookTriggerOnSuccess,
			FailurePolicy:    domain.TemplateHookFailurePolicyBlockRelease,
			TargetID:         "agtask-source-failed",
			TargetName:       "发布后 Agent 校验",
			SortNo:           1,
			CreatedAt:        now,
			UpdatedAt:        now,
		},
	}
	if err := repo.CreateTemplate(ctx, template, nil, nil, nil, hooks); err != nil {
		t.Fatalf("CreateTemplate failed: %v", err)
	}

	order := testReleaseOrder("ro-agent-hook-failed", "RO-AGENT-HOOK-FAILED", domain.OrderStatusRunning, now)
	order.TemplateID = template.ID
	order.TemplateName = template.Name
	order.StartedAt = &startedAt
	executions := []domain.ReleaseOrderExecution{
		testReleaseExecution(order.ID, "exec-ci", domain.PipelineScopeCI, domain.ExecutionStatusSuccess, now),
		testReleaseExecution(order.ID, "exec-cd", domain.PipelineScopeCD, domain.ExecutionStatusSuccess, now),
	}
	steps := defaultReleaseOrderSteps(order.ID, executions, now, "", hooks, order.EnvCode)
	for idx := range steps {
		switch steps[idx].StepCode {
		case "hook:post_release:agent_task:1":
			steps[idx].Status = domain.StepStatusFailed
			steps[idx].StartedAt = &startedAt
			steps[idx].FinishedAt = &now
			steps[idx].Message = "执行失败：发布后 Agent 校验"
		case "global:release_finish":
			steps[idx].Status = domain.StepStatusSuccess
			steps[idx].StartedAt = &startedAt
			steps[idx].FinishedAt = &now
			steps[idx].Message = "主发布流程完成"
		default:
			steps[idx].Status = domain.StepStatusSuccess
			steps[idx].StartedAt = &startedAt
			steps[idx].FinishedAt = &now
		}
	}
	if err := repo.Create(ctx, order, executions, nil, steps); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	tracker := NewTrackReleaseExecution(manager, nil)
	tracker.now = func() time.Time { return now }
	prematureSuccess := order
	prematureSuccess.Status = domain.OrderStatusSuccess
	trackable, err := tracker.isRealtimeTrackableOrder(ctx, prematureSuccess)
	if err != nil {
		t.Fatalf("isRealtimeTrackableOrder failed: %v", err)
	}
	if !trackable {
		t.Fatal("premature success with failed blocking hook must remain trackable")
	}
	if _, _, err := tracker.finalizeOrder(ctx, order, executions); err != nil {
		t.Fatalf("finalizeOrder failed: %v", err)
	}

	stored, err := repo.GetByID(ctx, order.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if stored.Status != domain.OrderStatusFailed {
		t.Fatalf("stored status = %s, want %s", stored.Status, domain.OrderStatusFailed)
	}
	storedSteps, err := repo.ListSteps(ctx, order.ID)
	if err != nil {
		t.Fatalf("ListSteps failed: %v", err)
	}
	finishStep := findStepByCode(storedSteps, "global:release_finish")
	if finishStep == nil {
		t.Fatal("finish step = nil")
	}
	if finishStep.Status != domain.StepStatusFailed {
		t.Fatalf("finish step status = %s, want %s", finishStep.Status, domain.StepStatusFailed)
	}
}

func TestPrematureSuccessRepairSkipsWarnOnlyFailedHook(t *testing.T) {
	t.Parallel()

	manager, repo := newReleaseOrderManagerForCancelTest(t)
	ctx := context.Background()
	now := time.Date(2026, 7, 17, 16, 0, 0, 0, time.UTC)
	template := domain.ReleaseTemplate{
		ID:              "rt-agent-hook-warn-only",
		Name:            "Warn Only Hook Template",
		ApplicationID:   "app-1",
		ApplicationName: "App 1",
		BindingID:       "app-1",
		BindingName:     "App 1",
		BindingType:     "application",
		Status:          domain.TemplateStatusActive,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	hook := domain.ReleaseTemplateHook{
		ID:               "hook-agent-warn-only",
		TemplateID:       template.ID,
		HookType:         domain.TemplateHookTypeAgentTask,
		Name:             "非阻塞 Agent 校验",
		ExecuteStage:     domain.TemplateHookExecuteStagePostRelease,
		TriggerCondition: domain.TemplateHookTriggerOnSuccess,
		FailurePolicy:    domain.TemplateHookFailurePolicyWarnOnly,
		TargetID:         "agtask-warn-only",
		TargetName:       "非阻塞 Agent 校验",
		SortNo:           1,
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	if err := repo.CreateTemplate(ctx, template, nil, nil, nil, []domain.ReleaseTemplateHook{hook}); err != nil {
		t.Fatalf("CreateTemplate failed: %v", err)
	}
	order := testReleaseOrder("ro-agent-hook-warn-only", "RO-AGENT-HOOK-WARN-ONLY", domain.OrderStatusSuccess, now)
	order.TemplateID = template.ID
	order.TemplateName = template.Name
	step := testReleaseStep(order.ID, "step-agent-hook-warn-only", domain.StepScopeGlobal, "hook:post_release:agent_task:1", domain.StepStatusFailed, 1, now)
	if err := repo.Create(ctx, order, nil, nil, []domain.ReleaseOrderStep{step}); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	tracker := NewTrackReleaseExecution(manager, nil)
	trackable, err := tracker.isRealtimeTrackableOrder(ctx, order)
	if err != nil {
		t.Fatalf("isRealtimeTrackableOrder failed: %v", err)
	}
	if trackable {
		t.Fatal("warn-only failed hook must not reopen an already successful order")
	}
}

// TestDeriveOrderStatusFromStepsDoesNotLetFinishSuccessBypassRunningHook 校验整体成功不能绕过未完成 Hook。
func TestDeriveOrderStatusFromStepsDoesNotLetFinishSuccessBypassRunningHook(t *testing.T) {
	t.Parallel()

	status, shouldUpdate := deriveOrderStatusFromSteps([]domain.ReleaseOrderStep{
		{
			StepScope: domain.StepScopeGlobal,
			StepCode:  "hook:post_release:agent_task:1",
			Status:    domain.StepStatusRunning,
		},
		{
			StepScope: domain.StepScopeGlobal,
			StepCode:  "global:release_finish",
			Status:    domain.StepStatusSuccess,
		},
	})
	if status != domain.OrderStatusRunning {
		t.Fatalf("status = %s, want %s", status, domain.OrderStatusRunning)
	}
	if shouldUpdate {
		t.Fatal("shouldUpdate = true, want false while hook is running")
	}
}

// TestEvaluateMainReleaseStatus 启动当前进程并完成依赖初始化。
func TestEvaluateMainReleaseStatus(t *testing.T) {
	t.Parallel()

	status, message, done := evaluateMainReleaseStatus([]domain.ReleaseOrderExecution{
		{Status: domain.ExecutionStatusSuccess},
		{Status: domain.ExecutionStatusSuccess},
	})
	if !done || status != domain.OrderStatusSuccess || message != "发布完成" {
		t.Fatalf("success case mismatch: done=%v status=%s message=%s", done, status, message)
	}

	status, message, done = evaluateMainReleaseStatus([]domain.ReleaseOrderExecution{
		{Status: domain.ExecutionStatusSuccess},
		{Status: domain.ExecutionStatusFailed},
	})
	if !done || status != domain.OrderStatusFailed || message != "存在失败执行单元" {
		t.Fatalf("failed case mismatch: done=%v status=%s message=%s", done, status, message)
	}

	status, message, done = evaluateMainReleaseStatus([]domain.ReleaseOrderExecution{
		{Status: domain.ExecutionStatusCancelled},
	})
	if !done || status != domain.OrderStatusCancelled || message != "存在已取消执行单元" {
		t.Fatalf("cancelled case mismatch: done=%v status=%s message=%s", done, status, message)
	}

	status, message, done = evaluateMainReleaseStatus([]domain.ReleaseOrderExecution{
		{Status: domain.ExecutionStatusRunning},
	})
	if done || status != domain.OrderStatusRunning || message != "" {
		t.Fatalf("running case mismatch: done=%v status=%s message=%s", done, status, message)
	}
}

// TestDeriveHookReleaseStatus 封装当前模块的业务处理逻辑。
func TestDeriveHookReleaseStatus(t *testing.T) {
	t.Parallel()

	order := domain.ReleaseOrder{Status: domain.OrderStatusBuilding}
	executions := []domain.ReleaseOrderExecution{
		{PipelineScope: domain.PipelineScopeCI, Status: domain.ExecutionStatusSuccess},
		{PipelineScope: domain.PipelineScopeCD, Status: domain.ExecutionStatusPending},
	}
	if got := deriveHookReleaseStatus(order, executions, domain.TemplateHookExecuteStageBuildComplete); got != string(domain.OrderStatusSuccess) {
		t.Fatalf("deriveHookReleaseStatus(build_complete) = %q, want %q", got, domain.OrderStatusSuccess)
	}

	failedExecutions := []domain.ReleaseOrderExecution{
		{PipelineScope: domain.PipelineScopeCI, Status: domain.ExecutionStatusFailed},
	}
	if got := deriveHookReleaseStatus(order, failedExecutions, domain.TemplateHookExecuteStageBuildComplete); got != string(domain.OrderStatusFailed) {
		t.Fatalf("deriveHookReleaseStatus(build_failed) = %q, want %q", got, domain.OrderStatusFailed)
	}

	finishedExecutions := []domain.ReleaseOrderExecution{
		{PipelineScope: domain.PipelineScopeCI, Status: domain.ExecutionStatusSuccess},
		{PipelineScope: domain.PipelineScopeCD, Status: domain.ExecutionStatusSuccess},
	}
	if got := deriveHookReleaseStatus(domain.ReleaseOrder{Status: domain.OrderStatusSuccess}, finishedExecutions, domain.TemplateHookExecuteStagePostRelease); got != string(domain.OrderStatusSuccess) {
		t.Fatalf("deriveHookReleaseStatus(post_release) = %q, want %q", got, domain.OrderStatusSuccess)
	}
}

// TestBuildNotificationRichValues 组装业务执行所需的输入数据。
func TestBuildNotificationRichValues(t *testing.T) {
	t.Parallel()

	if got := buildNotificationReleaseStageRichValue("build_complete"); got != "🟠 构建完成" {
		t.Fatalf("buildNotificationReleaseStageRichValue(build_complete) = %q", got)
	}
	if got := buildNotificationReleaseStageRichValue("post_release"); got != "🔵 发布完成" {
		t.Fatalf("buildNotificationReleaseStageRichValue(post_release) = %q", got)
	}
	if got := buildNotificationReleaseStatusRichValue("success"); got != "🟢 成功" {
		t.Fatalf("buildNotificationReleaseStatusRichValue(success) = %q", got)
	}
	if got := buildNotificationReleaseStatusRichValue("failed"); got != "🔴 失败" {
		t.Fatalf("buildNotificationReleaseStatusRichValue(failed) = %q", got)
	}
	if got := buildNotificationReleaseStatusRichValue("built_waiting_deploy"); got != "🟠 已构建待部署" {
		t.Fatalf("buildNotificationReleaseStatusRichValue(built_waiting_deploy) = %q", got)
	}
}

// TestEnforceNotificationCoreVariables 封装当前模块的业务处理逻辑。
func TestEnforceNotificationCoreVariables(t *testing.T) {
	t.Parallel()

	order := domain.ReleaseOrder{Status: domain.OrderStatusSuccess}
	executions := []domain.ReleaseOrderExecution{
		{PipelineScope: domain.PipelineScopeCI, Status: domain.ExecutionStatusSuccess},
		{PipelineScope: domain.PipelineScopeCD, Status: domain.ExecutionStatusSuccess},
	}
	values := map[string]string{
		"app_name": "gateway",
	}

	enforceNotificationCoreVariables(order, executions, domain.TemplateHookExecuteStagePostRelease, values)

	if got := values["release_stage"]; got != "post_release" {
		t.Fatalf("release_stage = %q, want %q", got, "post_release")
	}
	if got := values["release_stage_rich"]; got != "🔵 发布完成" {
		t.Fatalf("release_stage_rich = %q, want %q", got, "🔵 发布完成")
	}
	if got := values["release_status"]; got != "success" {
		t.Fatalf("release_status = %q, want %q", got, "success")
	}
	if got := values["release_status_rich"]; got != "🟢 成功" {
		t.Fatalf("release_status_rich = %q, want %q", got, "🟢 成功")
	}
}

// TestContainsUnresolvedNotificationCorePlaceholder 解析上下文数据，得到后续流程需要的结果。
func TestContainsUnresolvedNotificationCorePlaceholder(t *testing.T) {
	t.Parallel()

	if !containsUnresolvedNotificationCorePlaceholder("阶段：{release_stage_rich}") {
		t.Fatal("expected unresolved release_stage_rich placeholder to be detected")
	}
	if !containsUnresolvedNotificationCorePlaceholder("结果：{Release_Status_Rich}") {
		t.Fatal("expected case-insensitive unresolved release_status_rich placeholder to be detected")
	}
	if containsUnresolvedNotificationCorePlaceholder("阶段：🔵 发布完成") {
		t.Fatal("did not expect plain rendered text to be detected as unresolved placeholder")
	}
}

// TestSendTemplateWebhookTimeout 封装当前模块的业务处理逻辑。
func TestSendTemplateWebhookTimeout(t *testing.T) {
	t.Parallel()

	previousTimeout := templateWebhookHTTPTimeout
	templateWebhookHTTPTimeout = 50 * time.Millisecond
	t.Cleanup(func() {
		templateWebhookHTTPTimeout = previousTimeout
	})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	t.Cleanup(server.Close)

	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, server.URL, strings.NewReader(`{}`))
	if err != nil {
		t.Fatalf("NewRequestWithContext failed: %v", err)
	}

	startedAt := time.Now()
	_, err = sendTemplateWebhook(req)
	if err == nil {
		t.Fatal("sendTemplateWebhook error = nil, want timeout error")
	}
	if elapsed := time.Since(startedAt); elapsed >= 180*time.Millisecond {
		t.Fatalf("sendTemplateWebhook elapsed = %s, want timeout before server responds", elapsed)
	}
}

// TestBuildNotificationHookRequestAddsDingTalkSignature 组装业务执行所需的输入数据。
func TestBuildNotificationHookRequestAddsDingTalkSignature(t *testing.T) {
	t.Parallel()

	req, err := buildNotificationHookRequest(context.Background(), notificationdomain.Source{
		SourceType:        notificationdomain.SourceTypeDingTalk,
		WebhookURL:        "https://oapi.dingtalk.com/robot/send?access_token=test-token",
		VerificationParam: "ding-secret",
	}, "title", "body")
	if err != nil {
		t.Fatalf("buildNotificationHookRequest failed: %v", err)
	}

	parsedURL, err := url.Parse(req.URL.String())
	if err != nil {
		t.Fatalf("url.Parse failed: %v", err)
	}
	query := parsedURL.Query()
	if query.Get("access_token") != "test-token" {
		t.Fatalf("access_token = %q, want %q", query.Get("access_token"), "test-token")
	}
	if query.Get("timestamp") == "" {
		t.Fatal("timestamp = empty, want signed timestamp")
	}
	if query.Get("sign") == "" {
		t.Fatal("sign = empty, want signed signature")
	}
}

// TestBuildNotificationHookRequestBuildsFeishuCardWithKeywordTitle 组装业务执行所需的输入数据。
func TestBuildNotificationHookRequestBuildsFeishuCardWithKeywordTitle(t *testing.T) {
	t.Parallel()

	req, err := buildNotificationHookRequest(context.Background(), notificationdomain.Source{
		SourceType:        notificationdomain.SourceType("feishu"),
		WebhookURL:        "https://open.feishu.cn/open-apis/bot/v2/hook/test-token",
		VerificationParam: "GOS放行",
	}, "发布完成", "**body**")
	if err != nil {
		t.Fatalf("buildNotificationHookRequest failed: %v", err)
	}

	bodyBytes, err := io.ReadAll(req.Body)
	if err != nil {
		t.Fatalf("ReadAll request body failed: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(bodyBytes, &payload); err != nil {
		t.Fatalf("Unmarshal request body failed: %v", err)
	}
	if got := payload["msg_type"]; got != "interactive" {
		t.Fatalf("msg_type = %v, want interactive", got)
	}
	card, ok := payload["card"].(map[string]any)
	if !ok {
		t.Fatalf("card payload type = %T, want object", payload["card"])
	}
	header, ok := card["header"].(map[string]any)
	if !ok {
		t.Fatalf("header payload type = %T, want object", card["header"])
	}
	titleNode, ok := header["title"].(map[string]any)
	if !ok {
		t.Fatalf("header.title payload type = %T, want object", header["title"])
	}
	titleContent, _ := titleNode["content"].(string)
	if !strings.Contains(titleContent, "GOS放行") || !strings.Contains(titleContent, "发布完成") {
		t.Fatalf("feishu title = %q, want keyword and original title", titleContent)
	}
	elements, ok := card["elements"].([]any)
	if !ok || len(elements) != 1 {
		t.Fatalf("elements = %#v, want one markdown element", card["elements"])
	}
	markdownElement, ok := elements[0].(map[string]any)
	if !ok {
		t.Fatalf("element payload type = %T, want object", elements[0])
	}
	if got := markdownElement["content"]; got != "**body**" {
		t.Fatalf("markdown content = %v, want **body**", got)
	}
}
