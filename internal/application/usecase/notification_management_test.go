package usecase

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	notificationdomain "gos/internal/domain/notification"
)

// TestNormalizeNotificationVariablesBuildsRichFallbacks 标准化输入值，保证后续逻辑使用统一格式。
func TestNormalizeNotificationVariablesBuildsRichFallbacks(t *testing.T) {
	t.Parallel()

	normalized := normalizeNotificationVariables(map[string]string{
		"release_stage":  "post_release",
		"release_status": "success",
	})

	if got := normalized["release_stage_rich"]; got != "🔵 发布完成" {
		t.Fatalf("release_stage_rich = %q, want %q", got, "🔵 发布完成")
	}
	if got := normalized["release_status_rich"]; got != "🟢 成功" {
		t.Fatalf("release_status_rich = %q, want %q", got, "🟢 成功")
	}
}

// TestAllowedMarkdownVariableKeysIncludesReleaseName 内置通知变量应包含发布名称。
func TestAllowedMarkdownVariableKeysIncludesReleaseName(t *testing.T) {
	t.Parallel()

	manager := &NotificationManager{}
	keys, err := manager.allowedMarkdownVariableKeys(context.Background())
	if err != nil {
		t.Fatalf("allowedMarkdownVariableKeys failed: %v", err)
	}
	if _, ok := keys["release_name"]; !ok {
		t.Fatal("release_name is not an allowed notification markdown variable")
	}
	if err := validateNotificationMarkdownPlaceholders(keys, "[{env}] {release_name}", "发布名称：{release_name}"); err != nil {
		t.Fatalf("validateNotificationMarkdownPlaceholders release_name failed: %v", err)
	}
}

// TestNormalizeSourceInputKeepsFeishuKeyword 标准化输入值，保证后续逻辑使用统一格式。
func TestNormalizeSourceInputKeepsFeishuKeyword(t *testing.T) {
	t.Parallel()

	manager := &NotificationManager{}
	source, err := manager.normalizeSourceInput("飞书发布群", "feishu", "https://open.feishu.cn/open-apis/bot/v2/hook/test-token", "GOS放行", true, "")
	if err != nil {
		t.Fatalf("normalizeSourceInput failed: %v", err)
	}
	if source.SourceType != notificationdomain.SourceType("feishu") {
		t.Fatalf("SourceType = %q, want feishu", source.SourceType)
	}
	if source.VerificationParam != "GOS放行" {
		t.Fatalf("VerificationParam = %q, want keyword", source.VerificationParam)
	}
}

// TestTestSourceWebhookSendsFeishuSimulationMessage 发送模拟消息测试通知源连通性。
func TestTestSourceWebhookSendsFeishuSimulationMessage(t *testing.T) {
	t.Parallel()

	var payload map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", r.Method)
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("ReadAll body failed: %v", err)
		}
		if err := json.Unmarshal(body, &payload); err != nil {
			t.Fatalf("Unmarshal body failed: %v", err)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	t.Cleanup(server.Close)

	manager := &NotificationManager{}
	output, err := manager.TestSourceWebhook(context.Background(), TestNotificationSourceWebhookInput{
		Name:              "飞书发布群",
		SourceType:        "feishu",
		WebhookURL:        server.URL,
		VerificationParam: "GOS放行",
	})
	if err != nil {
		t.Fatalf("TestSourceWebhook failed: %v", err)
	}
	if !output.Success {
		t.Fatal("Success = false, want true")
	}
	if output.StatusCode != http.StatusOK {
		t.Fatalf("StatusCode = %d, want %d", output.StatusCode, http.StatusOK)
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
	if !strings.Contains(titleContent, "GOS放行") || !strings.Contains(titleContent, "Webhook 连通性测试") {
		t.Fatalf("feishu title = %q, want keyword and test title", titleContent)
	}
}

// TestCreateSourceAllowsWeComWebhook 企业微信群机器人 Webhook 通知源应允许新建。
func TestCreateSourceAllowsWeComWebhook(t *testing.T) {
	t.Parallel()

	repo := &notificationSourceCreateRepo{}
	manager := NewNotificationManager(repo, nil)
	output, err := manager.CreateSource(context.Background(), CreateNotificationSourceInput{
		Name:              "企业微信群机器人",
		SourceType:        "wecom",
		WebhookURL:        "https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=test-key",
		VerificationParam: "should-be-cleared",
		Enabled:           true,
		CreatedBy:         "tester",
	})
	if err != nil {
		t.Fatalf("CreateSource failed: %v", err)
	}
	if output.SourceType != string(notificationdomain.SourceTypeWeCom) {
		t.Fatalf("SourceType = %q, want %q", output.SourceType, notificationdomain.SourceTypeWeCom)
	}
	if repo.created.VerificationParam != "" {
		t.Fatalf("VerificationParam = %q, want empty for wecom", repo.created.VerificationParam)
	}
}

// TestRenderNotificationMarkdownTemplateUsesRichFallbacks 封装当前模块的业务处理逻辑。
func TestRenderNotificationMarkdownTemplateUsesRichFallbacks(t *testing.T) {
	t.Parallel()

	title, body := renderNotificationMarkdownTemplate(map[string]string{
		"env":            "dev",
		"app_name":       "gateway",
		"release_stage":  "post_release",
		"release_status": "success",
	}, notificationdomain.MarkdownTemplate{
		TitleTemplate: "[{env}] {app_name} {release_status_rich}",
		BodyTemplate:  "> 阶段：{release_stage_rich}\n> 结果：{release_status_rich}",
	})

	if strings.Contains(title, "{release_status_rich}") {
		t.Fatalf("title still contains release_status_rich placeholder: %q", title)
	}
	if strings.Contains(body, "{release_stage_rich}") || strings.Contains(body, "{release_status_rich}") {
		t.Fatalf("body still contains rich placeholders: %q", body)
	}
}

type notificationSourceCreateRepo struct {
	created notificationdomain.Source
}

func (r *notificationSourceCreateRepo) InitSchema(context.Context) error {
	return nil
}

func (r *notificationSourceCreateRepo) CreateSource(_ context.Context, item notificationdomain.Source) (notificationdomain.Source, error) {
	r.created = item
	return item, nil
}

func (r *notificationSourceCreateRepo) UpdateSource(context.Context, notificationdomain.Source) (notificationdomain.Source, error) {
	return notificationdomain.Source{}, nil
}

func (r *notificationSourceCreateRepo) GetSourceByID(context.Context, string) (notificationdomain.Source, error) {
	return notificationdomain.Source{}, nil
}

func (r *notificationSourceCreateRepo) ListSources(context.Context, notificationdomain.SourceListFilter) ([]notificationdomain.Source, int64, error) {
	return nil, 0, nil
}

func (r *notificationSourceCreateRepo) DeleteSource(context.Context, string) error {
	return nil
}

func (r *notificationSourceCreateRepo) CreateMarkdownTemplate(context.Context, notificationdomain.MarkdownTemplate) (notificationdomain.MarkdownTemplate, error) {
	return notificationdomain.MarkdownTemplate{}, nil
}

func (r *notificationSourceCreateRepo) UpdateMarkdownTemplate(context.Context, notificationdomain.MarkdownTemplate) (notificationdomain.MarkdownTemplate, error) {
	return notificationdomain.MarkdownTemplate{}, nil
}

func (r *notificationSourceCreateRepo) GetMarkdownTemplateByID(context.Context, string) (notificationdomain.MarkdownTemplate, error) {
	return notificationdomain.MarkdownTemplate{}, nil
}

func (r *notificationSourceCreateRepo) ListMarkdownTemplates(context.Context, notificationdomain.MarkdownTemplateListFilter) ([]notificationdomain.MarkdownTemplate, int64, error) {
	return nil, 0, nil
}

func (r *notificationSourceCreateRepo) DeleteMarkdownTemplate(context.Context, string) error {
	return nil
}

func (r *notificationSourceCreateRepo) CreateHook(context.Context, notificationdomain.Hook) (notificationdomain.Hook, error) {
	return notificationdomain.Hook{}, nil
}

func (r *notificationSourceCreateRepo) UpdateHook(context.Context, notificationdomain.Hook) (notificationdomain.Hook, error) {
	return notificationdomain.Hook{}, nil
}

func (r *notificationSourceCreateRepo) GetHookByID(context.Context, string) (notificationdomain.Hook, error) {
	return notificationdomain.Hook{}, nil
}

func (r *notificationSourceCreateRepo) ListHooks(context.Context, notificationdomain.HookListFilter) ([]notificationdomain.Hook, int64, error) {
	return nil, 0, nil
}

func (r *notificationSourceCreateRepo) DeleteHook(context.Context, string) error {
	return nil
}
