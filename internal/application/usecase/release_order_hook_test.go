package usecase

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	agentdomain "gos/internal/domain/agent"
	notificationdomain "gos/internal/domain/notification"
	domain "gos/internal/domain/release"
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
