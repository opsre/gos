package usecase

import (
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
