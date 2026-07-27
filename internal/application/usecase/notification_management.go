package usecase

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"time"

	notificationdomain "gos/internal/domain/notification"
	platformparamdomain "gos/internal/domain/platformparam"
)

var notificationTemplatePlaceholderPattern = regexp.MustCompile(`\{([a-zA-Z0-9_]+)\}`)

var notificationBuiltinKeys = map[string]struct{}{
	"app_key":             {},
	"app_name":            {},
	"release_name":        {},
	"project_name":        {},
	"env":                 {},
	"env_code":            {},
	"branch":              {},
	"git_ref":             {},
	"image_version":       {},
	"image_tag":           {},
	"order_no":            {},
	"operation_type":      {},
	"source_order_no":     {},
	"executor_user_id":    {},
	"executor_name":       {},
	"release_status":      {},
	"release_stage":       {},
	"release_status_rich": {},
	"release_stage_rich":  {},
}

var notificationSystemKeys = map[string]struct{}{
	"release_detail_url": {},
}

type NotificationManager struct {
	repo         notificationdomain.Repository
	platformRepo platformparamdomain.Repository
	now          func() time.Time
}

type NotificationSourceOutput struct {
	ID                   string    `json:"id"`
	Name                 string    `json:"name"`
	SourceType           string    `json:"source_type"`
	WebhookURL           string    `json:"webhook_url"`
	HasVerificationParam bool      `json:"has_verification_param"`
	Keywords             string    `json:"keywords"`
	Enabled              bool      `json:"enabled"`
	Remark               string    `json:"remark"`
	CreatedBy            string    `json:"created_by"`
	UpdatedBy            string    `json:"updated_by"`
	CreatedAt            time.Time `json:"created_at"`
	UpdatedAt            time.Time `json:"updated_at"`
}

type NotificationSourceListOutput struct {
	Items []NotificationSourceOutput `json:"items"`
	Total int64                      `json:"total"`
}

type NotificationMarkdownTemplateConditionOutput struct {
	ParamKey      string `json:"param_key"`
	Operator      string `json:"operator"`
	ExpectedValue string `json:"expected_value"`
	MarkdownText  string `json:"markdown_text"`
	SortNo        int    `json:"sort_no"`
}

type NotificationMarkdownTemplateOutput struct {
	ID            string                                        `json:"id"`
	Name          string                                        `json:"name"`
	TitleTemplate string                                        `json:"title_template"`
	BodyTemplate  string                                        `json:"body_template"`
	Conditions    []NotificationMarkdownTemplateConditionOutput `json:"conditions"`
	Enabled       bool                                          `json:"enabled"`
	Remark        string                                        `json:"remark"`
	CreatedBy     string                                        `json:"created_by"`
	UpdatedBy     string                                        `json:"updated_by"`
	CreatedAt     time.Time                                     `json:"created_at"`
	UpdatedAt     time.Time                                     `json:"updated_at"`
}

type NotificationMarkdownTemplateListOutput struct {
	Items []NotificationMarkdownTemplateOutput `json:"items"`
	Total int64                                `json:"total"`
}

type NotificationHookOutput struct {
	ID                   string    `json:"id"`
	Name                 string    `json:"name"`
	SourceID             string    `json:"source_id"`
	SourceName           string    `json:"source_name"`
	SourceType           string    `json:"source_type"`
	MarkdownTemplateID   string    `json:"markdown_template_id"`
	MarkdownTemplateName string    `json:"markdown_template_name"`
	Enabled              bool      `json:"enabled"`
	Remark               string    `json:"remark"`
	CreatedBy            string    `json:"created_by"`
	UpdatedBy            string    `json:"updated_by"`
	CreatedAt            time.Time `json:"created_at"`
	UpdatedAt            time.Time `json:"updated_at"`
}

type NotificationHookListOutput struct {
	Items []NotificationHookOutput `json:"items"`
	Total int64                    `json:"total"`
}

type NotificationMarkdownTemplateConditionInput struct {
	ParamKey      string `json:"param_key"`
	Operator      string `json:"operator"`
	ExpectedValue string `json:"expected_value"`
	MarkdownText  string `json:"markdown_text"`
}

type CreateNotificationSourceInput struct {
	Name              string `json:"name"`
	SourceType        string `json:"source_type"`
	WebhookURL        string `json:"webhook_url"`
	VerificationParam string `json:"verification_param"`
	Keywords          string `json:"keywords"`
	Enabled           bool   `json:"enabled"`
	Remark            string `json:"remark"`
	CreatedBy         string `json:"created_by"`
}

type UpdateNotificationSourceInput struct {
	Name              string `json:"name"`
	SourceType        string `json:"source_type"`
	WebhookURL        string `json:"webhook_url"`
	VerificationParam string `json:"verification_param"`
	Keywords          string `json:"keywords"`
	Enabled           bool   `json:"enabled"`
	Remark            string `json:"remark"`
	UpdatedBy         string `json:"updated_by"`
}

type TestNotificationSourceWebhookInput struct {
	SourceID          string `json:"source_id"`
	Name              string `json:"name"`
	SourceType        string `json:"source_type"`
	WebhookURL        string `json:"webhook_url"`
	VerificationParam string `json:"verification_param"`
}

type NotificationSourceWebhookTestOutput struct {
	Success    bool   `json:"success"`
	Message    string `json:"message"`
	StatusCode int    `json:"status_code"`
}

type CreateNotificationMarkdownTemplateInput struct {
	Name          string                                       `json:"name"`
	TitleTemplate string                                       `json:"title_template"`
	BodyTemplate  string                                       `json:"body_template"`
	Conditions    []NotificationMarkdownTemplateConditionInput `json:"conditions"`
	Enabled       bool                                         `json:"enabled"`
	Remark        string                                       `json:"remark"`
	CreatedBy     string                                       `json:"created_by"`
}

type UpdateNotificationMarkdownTemplateInput struct {
	Name          string                                       `json:"name"`
	TitleTemplate string                                       `json:"title_template"`
	BodyTemplate  string                                       `json:"body_template"`
	Conditions    []NotificationMarkdownTemplateConditionInput `json:"conditions"`
	Enabled       bool                                         `json:"enabled"`
	Remark        string                                       `json:"remark"`
	UpdatedBy     string                                       `json:"updated_by"`
}

type CreateNotificationHookInput struct {
	Name               string `json:"name"`
	SourceID           string `json:"source_id"`
	MarkdownTemplateID string `json:"markdown_template_id"`
	Enabled            bool   `json:"enabled"`
	Remark             string `json:"remark"`
	CreatedBy          string `json:"created_by"`
}

type UpdateNotificationHookInput struct {
	Name               string `json:"name"`
	SourceID           string `json:"source_id"`
	MarkdownTemplateID string `json:"markdown_template_id"`
	Enabled            bool   `json:"enabled"`
	Remark             string `json:"remark"`
	UpdatedBy          string `json:"updated_by"`
}

// NewNotificationManager 创建并返回对应组件实例。
func NewNotificationManager(repo notificationdomain.Repository, platformRepo platformparamdomain.Repository) *NotificationManager {
	return &NotificationManager{
		repo:         repo,
		platformRepo: platformRepo,
		now: func() time.Time {
			return time.Now().UTC()
		},
	}
}

// ListSources 查询并返回列表数据。
func (uc *NotificationManager) ListSources(ctx context.Context, filter notificationdomain.SourceListFilter) (NotificationSourceListOutput, error) {
	if uc == nil || uc.repo == nil {
		return NotificationSourceListOutput{}, fmt.Errorf("%w: notification manager is not configured", ErrInvalidInput)
	}
	if filter.Type != "" && !filter.Type.Valid() {
		return NotificationSourceListOutput{}, fmt.Errorf("%w: invalid source_type", ErrInvalidInput)
	}
	items, total, err := uc.repo.ListSources(ctx, filter)
	if err != nil {
		return NotificationSourceListOutput{}, err
	}
	outputs := make([]NotificationSourceOutput, 0, len(items))
	for _, item := range items {
		outputs = append(outputs, toNotificationSourceOutput(item))
	}
	return NotificationSourceListOutput{Items: outputs, Total: total}, nil
}

// GetSource 查询并返回指定资源数据。
func (uc *NotificationManager) GetSource(ctx context.Context, id string) (NotificationSourceOutput, error) {
	if uc == nil || uc.repo == nil {
		return NotificationSourceOutput{}, fmt.Errorf("%w: notification manager is not configured", ErrInvalidInput)
	}
	item, err := uc.repo.GetSourceByID(ctx, strings.TrimSpace(id))
	if err != nil {
		return NotificationSourceOutput{}, err
	}
	return toNotificationSourceOutput(item), nil
}

// CreateSource 创建业务资源并返回处理结果。
func (uc *NotificationManager) CreateSource(ctx context.Context, input CreateNotificationSourceInput) (NotificationSourceOutput, error) {
	if uc == nil || uc.repo == nil {
		return NotificationSourceOutput{}, fmt.Errorf("%w: notification manager is not configured", ErrInvalidInput)
	}
	item, err := uc.normalizeSourceInput(input.Name, input.SourceType, input.WebhookURL, input.VerificationParam, input.Keywords, input.Enabled, input.Remark)
	if err != nil {
		return NotificationSourceOutput{}, err
	}
	now := uc.now()
	item.ID = generateID("ntfsrc")
	item.CreatedBy = strings.TrimSpace(input.CreatedBy)
	item.UpdatedBy = item.CreatedBy
	item.CreatedAt = now
	item.UpdatedAt = now
	created, err := uc.repo.CreateSource(ctx, item)
	if err != nil {
		return NotificationSourceOutput{}, err
	}
	return toNotificationSourceOutput(created), nil
}

// UpdateSource 更新业务资源并返回处理结果。
func (uc *NotificationManager) UpdateSource(ctx context.Context, id string, input UpdateNotificationSourceInput) (NotificationSourceOutput, error) {
	if uc == nil || uc.repo == nil {
		return NotificationSourceOutput{}, fmt.Errorf("%w: notification manager is not configured", ErrInvalidInput)
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return NotificationSourceOutput{}, ErrInvalidID
	}
	current, err := uc.repo.GetSourceByID(ctx, id)
	if err != nil {
		return NotificationSourceOutput{}, err
	}
	requestedType := notificationdomain.SourceType(strings.ToLower(strings.TrimSpace(input.SourceType)))
	verificationParam := input.VerificationParam
	if (requestedType == notificationdomain.SourceTypeDingTalk || requestedType == notificationdomain.SourceTypeFeishu) && strings.TrimSpace(verificationParam) == "" {
		verificationParam = current.VerificationParam
	}
	keywords := input.Keywords
	if requestedType == notificationdomain.SourceTypeDingTalk && strings.TrimSpace(keywords) == "" {
		keywords = current.Keywords
	}
	item, err := uc.normalizeSourceInput(input.Name, input.SourceType, input.WebhookURL, verificationParam, keywords, input.Enabled, input.Remark)
	if err != nil {
		return NotificationSourceOutput{}, err
	}
	item.ID = current.ID
	item.CreatedBy = current.CreatedBy
	item.CreatedAt = current.CreatedAt
	if item.SourceType != notificationdomain.SourceTypeDingTalk && item.SourceType != notificationdomain.SourceTypeFeishu {
		item.VerificationParam = ""
	}
	if item.SourceType != notificationdomain.SourceTypeDingTalk {
		item.Keywords = ""
	}
	item.UpdatedBy = strings.TrimSpace(input.UpdatedBy)
	item.UpdatedAt = uc.now()
	updated, err := uc.repo.UpdateSource(ctx, item)
	if err != nil {
		return NotificationSourceOutput{}, err
	}
	return toNotificationSourceOutput(updated), nil
}

// DeleteSource 删除业务资源并返回处理结果。
func (uc *NotificationManager) DeleteSource(ctx context.Context, id string) error {
	if uc == nil || uc.repo == nil {
		return fmt.Errorf("%w: notification manager is not configured", ErrInvalidInput)
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return ErrInvalidID
	}
	return uc.repo.DeleteSource(ctx, id)
}

// TestSourceWebhook 发送模拟通知，验证通知源 Webhook 是否可达。
func (uc *NotificationManager) TestSourceWebhook(ctx context.Context, input TestNotificationSourceWebhookInput) (NotificationSourceWebhookTestOutput, error) {
	if uc == nil {
		return NotificationSourceWebhookTestOutput{}, fmt.Errorf("%w: notification manager is not configured", ErrInvalidInput)
	}
	source, err := uc.normalizeSourceWebhookTestInput(ctx, input)
	if err != nil {
		return NotificationSourceWebhookTestOutput{}, err
	}
	title, body := uc.buildSourceWebhookTestContent(source)
	req, err := buildNotificationHookRequest(ctx, source, title, body)
	if err != nil {
		return NotificationSourceWebhookTestOutput{}, err
	}
	resp, err := sendTemplateWebhook(req)
	if err != nil {
		return NotificationSourceWebhookTestOutput{}, err
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		summary := strings.TrimSpace(string(respBody))
		if summary == "" {
			summary = http.StatusText(resp.StatusCode)
		}
		return NotificationSourceWebhookTestOutput{StatusCode: resp.StatusCode}, fmt.Errorf("%w: webhook returned %d: %s", ErrInvalidInput, resp.StatusCode, summary)
	}
	return NotificationSourceWebhookTestOutput{
		Success:    true,
		Message:    fmt.Sprintf("测试消息发送成功，HTTP %d", resp.StatusCode),
		StatusCode: resp.StatusCode,
	}, nil
}

// ListMarkdownTemplates 查询并返回列表数据。
func (uc *NotificationManager) ListMarkdownTemplates(ctx context.Context, filter notificationdomain.MarkdownTemplateListFilter) (NotificationMarkdownTemplateListOutput, error) {
	if uc == nil || uc.repo == nil {
		return NotificationMarkdownTemplateListOutput{}, fmt.Errorf("%w: notification manager is not configured", ErrInvalidInput)
	}
	items, total, err := uc.repo.ListMarkdownTemplates(ctx, filter)
	if err != nil {
		return NotificationMarkdownTemplateListOutput{}, err
	}
	outputs := make([]NotificationMarkdownTemplateOutput, 0, len(items))
	for _, item := range items {
		outputs = append(outputs, toNotificationMarkdownTemplateOutput(item))
	}
	return NotificationMarkdownTemplateListOutput{Items: outputs, Total: total}, nil
}

// GetMarkdownTemplate 查询并返回指定资源数据。
func (uc *NotificationManager) GetMarkdownTemplate(ctx context.Context, id string) (NotificationMarkdownTemplateOutput, error) {
	if uc == nil || uc.repo == nil {
		return NotificationMarkdownTemplateOutput{}, fmt.Errorf("%w: notification manager is not configured", ErrInvalidInput)
	}
	item, err := uc.repo.GetMarkdownTemplateByID(ctx, strings.TrimSpace(id))
	if err != nil {
		return NotificationMarkdownTemplateOutput{}, err
	}
	return toNotificationMarkdownTemplateOutput(item), nil
}

// CreateMarkdownTemplate 创建业务资源并返回处理结果。
func (uc *NotificationManager) CreateMarkdownTemplate(ctx context.Context, input CreateNotificationMarkdownTemplateInput) (NotificationMarkdownTemplateOutput, error) {
	if uc == nil || uc.repo == nil {
		return NotificationMarkdownTemplateOutput{}, fmt.Errorf("%w: notification manager is not configured", ErrInvalidInput)
	}
	item, err := uc.normalizeMarkdownTemplateInput(ctx, input.Name, input.TitleTemplate, input.BodyTemplate, input.Conditions, input.Enabled, input.Remark)
	if err != nil {
		return NotificationMarkdownTemplateOutput{}, err
	}
	now := uc.now()
	item.ID = generateID("ntfmd")
	item.CreatedBy = strings.TrimSpace(input.CreatedBy)
	item.UpdatedBy = item.CreatedBy
	item.CreatedAt = now
	item.UpdatedAt = now
	created, err := uc.repo.CreateMarkdownTemplate(ctx, item)
	if err != nil {
		return NotificationMarkdownTemplateOutput{}, err
	}
	return toNotificationMarkdownTemplateOutput(created), nil
}

// UpdateMarkdownTemplate 更新业务资源并返回处理结果。
func (uc *NotificationManager) UpdateMarkdownTemplate(ctx context.Context, id string, input UpdateNotificationMarkdownTemplateInput) (NotificationMarkdownTemplateOutput, error) {
	if uc == nil || uc.repo == nil {
		return NotificationMarkdownTemplateOutput{}, fmt.Errorf("%w: notification manager is not configured", ErrInvalidInput)
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return NotificationMarkdownTemplateOutput{}, ErrInvalidID
	}
	current, err := uc.repo.GetMarkdownTemplateByID(ctx, id)
	if err != nil {
		return NotificationMarkdownTemplateOutput{}, err
	}
	item, err := uc.normalizeMarkdownTemplateInput(ctx, input.Name, input.TitleTemplate, input.BodyTemplate, input.Conditions, input.Enabled, input.Remark)
	if err != nil {
		return NotificationMarkdownTemplateOutput{}, err
	}
	item.ID = current.ID
	item.CreatedBy = current.CreatedBy
	item.CreatedAt = current.CreatedAt
	item.UpdatedBy = strings.TrimSpace(input.UpdatedBy)
	item.UpdatedAt = uc.now()
	updated, err := uc.repo.UpdateMarkdownTemplate(ctx, item)
	if err != nil {
		return NotificationMarkdownTemplateOutput{}, err
	}
	return toNotificationMarkdownTemplateOutput(updated), nil
}

// DeleteMarkdownTemplate 删除业务资源并返回处理结果。
func (uc *NotificationManager) DeleteMarkdownTemplate(ctx context.Context, id string) error {
	if uc == nil || uc.repo == nil {
		return fmt.Errorf("%w: notification manager is not configured", ErrInvalidInput)
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return ErrInvalidID
	}
	return uc.repo.DeleteMarkdownTemplate(ctx, id)
}

// ListHooks 查询并返回列表数据。
func (uc *NotificationManager) ListHooks(ctx context.Context, filter notificationdomain.HookListFilter) (NotificationHookListOutput, error) {
	if uc == nil || uc.repo == nil {
		return NotificationHookListOutput{}, fmt.Errorf("%w: notification manager is not configured", ErrInvalidInput)
	}
	items, total, err := uc.repo.ListHooks(ctx, filter)
	if err != nil {
		return NotificationHookListOutput{}, err
	}
	outputs := make([]NotificationHookOutput, 0, len(items))
	for _, item := range items {
		outputs = append(outputs, toNotificationHookOutput(item))
	}
	return NotificationHookListOutput{Items: outputs, Total: total}, nil
}

// GetHook 查询并返回指定资源数据。
func (uc *NotificationManager) GetHook(ctx context.Context, id string) (NotificationHookOutput, error) {
	if uc == nil || uc.repo == nil {
		return NotificationHookOutput{}, fmt.Errorf("%w: notification manager is not configured", ErrInvalidInput)
	}
	item, err := uc.repo.GetHookByID(ctx, strings.TrimSpace(id))
	if err != nil {
		return NotificationHookOutput{}, err
	}
	return toNotificationHookOutput(item), nil
}

// CreateHook 创建业务资源并返回处理结果。
func (uc *NotificationManager) CreateHook(ctx context.Context, input CreateNotificationHookInput) (NotificationHookOutput, error) {
	if uc == nil || uc.repo == nil {
		return NotificationHookOutput{}, fmt.Errorf("%w: notification manager is not configured", ErrInvalidInput)
	}
	item, err := uc.normalizeHookInput(ctx, input.Name, input.SourceID, input.MarkdownTemplateID, input.Enabled, input.Remark)
	if err != nil {
		return NotificationHookOutput{}, err
	}
	now := uc.now()
	item.ID = generateID("ntfhk")
	item.CreatedBy = strings.TrimSpace(input.CreatedBy)
	item.UpdatedBy = item.CreatedBy
	item.CreatedAt = now
	item.UpdatedAt = now
	created, err := uc.repo.CreateHook(ctx, item)
	if err != nil {
		return NotificationHookOutput{}, err
	}
	return toNotificationHookOutput(created), nil
}

// UpdateHook 更新业务资源并返回处理结果。
func (uc *NotificationManager) UpdateHook(ctx context.Context, id string, input UpdateNotificationHookInput) (NotificationHookOutput, error) {
	if uc == nil || uc.repo == nil {
		return NotificationHookOutput{}, fmt.Errorf("%w: notification manager is not configured", ErrInvalidInput)
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return NotificationHookOutput{}, ErrInvalidID
	}
	current, err := uc.repo.GetHookByID(ctx, id)
	if err != nil {
		return NotificationHookOutput{}, err
	}
	item, err := uc.normalizeHookInput(ctx, input.Name, input.SourceID, input.MarkdownTemplateID, input.Enabled, input.Remark)
	if err != nil {
		return NotificationHookOutput{}, err
	}
	item.ID = current.ID
	item.CreatedBy = current.CreatedBy
	item.CreatedAt = current.CreatedAt
	item.UpdatedBy = strings.TrimSpace(input.UpdatedBy)
	item.UpdatedAt = uc.now()
	updated, err := uc.repo.UpdateHook(ctx, item)
	if err != nil {
		return NotificationHookOutput{}, err
	}
	return toNotificationHookOutput(updated), nil
}

// DeleteHook 删除业务资源并返回处理结果。
func (uc *NotificationManager) DeleteHook(ctx context.Context, id string) error {
	if uc == nil || uc.repo == nil {
		return fmt.Errorf("%w: notification manager is not configured", ErrInvalidInput)
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return ErrInvalidID
	}
	return uc.repo.DeleteHook(ctx, id)
}

// normalizeSourceInput 标准化输入值，保证后续逻辑使用统一格式。
func (uc *NotificationManager) normalizeSourceInput(name, sourceType, webhookURL, verificationParam, keywords string, enabled bool, remark string) (notificationdomain.Source, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return notificationdomain.Source{}, fmt.Errorf("%w: source name is required", ErrInvalidInput)
	}
	typeValue := notificationdomain.SourceType(strings.ToLower(strings.TrimSpace(sourceType)))
	if !typeValue.Valid() {
		return notificationdomain.Source{}, fmt.Errorf("%w: invalid source_type", ErrInvalidInput)
	}
	webhookURL = strings.TrimSpace(webhookURL)
	if webhookURL == "" {
		return notificationdomain.Source{}, fmt.Errorf("%w: webhook_url is required", ErrInvalidInput)
	}
	verificationParam = strings.TrimSpace(verificationParam)
	keywords = strings.TrimSpace(keywords)
	switch typeValue {
	case notificationdomain.SourceTypeDingTalk:
		if keywords == "" {
			keywords = strings.TrimSpace(keywords)
		}
	case notificationdomain.SourceTypeFeishu:
		if verificationParam == "" {
			return notificationdomain.Source{}, fmt.Errorf("%w: 飞书放行关键字不能为空", ErrInvalidInput)
		}
		keywords = ""
	default:
		verificationParam = ""
		keywords = ""
	}
	return notificationdomain.Source{
		Name:              name,
		SourceType:        typeValue,
		WebhookURL:        webhookURL,
		VerificationParam: verificationParam,
		Keywords:          keywords,
		Enabled:           enabled,
		Remark:            strings.TrimSpace(remark),
	}, nil
}

// normalizeSourceWebhookTestInput 标准化测试通知源输入。
func (uc *NotificationManager) normalizeSourceWebhookTestInput(ctx context.Context, input TestNotificationSourceWebhookInput) (notificationdomain.Source, error) {
	sourceID := strings.TrimSpace(input.SourceID)
	name := input.Name
	sourceType := input.SourceType
	webhookURL := input.WebhookURL
	verificationParam := input.VerificationParam
	keywords := ""
	if sourceID != "" {
		if uc.repo == nil {
			return notificationdomain.Source{}, fmt.Errorf("%w: notification repository is not configured", ErrInvalidInput)
		}
		current, err := uc.repo.GetSourceByID(ctx, sourceID)
		if err != nil {
			return notificationdomain.Source{}, err
		}
		if strings.TrimSpace(name) == "" {
			name = current.Name
		}
		if strings.TrimSpace(sourceType) == "" {
			sourceType = string(current.SourceType)
		}
		if strings.TrimSpace(webhookURL) == "" {
			webhookURL = current.WebhookURL
		}
		requestedType := notificationdomain.SourceType(strings.ToLower(strings.TrimSpace(sourceType)))
		if (requestedType == notificationdomain.SourceTypeDingTalk || requestedType == notificationdomain.SourceTypeFeishu) && strings.TrimSpace(verificationParam) == "" {
			verificationParam = current.VerificationParam
		}
		keywords = current.Keywords
	}
	return uc.normalizeSourceInput(name, sourceType, webhookURL, verificationParam, keywords, true, "")
}

// buildSourceWebhookTestContent 组装通知源测试消息内容。
func (uc *NotificationManager) buildSourceWebhookTestContent(source notificationdomain.Source) (string, string) {
	now := time.Now().UTC()
	if uc != nil && uc.now != nil {
		now = uc.now()
	}
	title := "Webhook 连通性测试"
	body := fmt.Sprintf(
		"## Webhook 连通性测试\n\n这是一条由 deploy_platform 发送的模拟通知，用于验证通知源 Webhook 是否可达。\n\n- 通知源：%s\n- 类型：%s\n- 时间：%s",
		strings.TrimSpace(firstNonEmpty(source.Name, "未命名通知源")),
		source.SourceType,
		now.Format("2006-01-02 15:04:05"),
	)
	return title, body
}

// normalizeMarkdownTemplateInput 标准化输入值，保证后续逻辑使用统一格式。
func (uc *NotificationManager) normalizeMarkdownTemplateInput(
	ctx context.Context,
	name, titleTemplate, bodyTemplate string,
	conditions []NotificationMarkdownTemplateConditionInput,
	enabled bool,
	remark string,
) (notificationdomain.MarkdownTemplate, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return notificationdomain.MarkdownTemplate{}, fmt.Errorf("%w: markdown template name is required", ErrInvalidInput)
	}
	titleTemplate = strings.TrimSpace(strings.ReplaceAll(titleTemplate, "\r\n", "\n"))
	bodyTemplate = strings.TrimSpace(strings.ReplaceAll(bodyTemplate, "\r\n", "\n"))
	if titleTemplate == "" && bodyTemplate == "" {
		return notificationdomain.MarkdownTemplate{}, fmt.Errorf("%w: title_template or body_template is required", ErrInvalidInput)
	}
	allowedKeys, err := uc.allowedMarkdownVariableKeys(ctx)
	if err != nil {
		return notificationdomain.MarkdownTemplate{}, err
	}
	if err := validateNotificationMarkdownPlaceholders(allowedKeys, titleTemplate, bodyTemplate); err != nil {
		return notificationdomain.MarkdownTemplate{}, err
	}
	normalizedConditions := make([]notificationdomain.MarkdownTemplateCondition, 0, len(conditions))
	for idx, item := range conditions {
		paramKey := strings.ToLower(strings.TrimSpace(item.ParamKey))
		if paramKey == "" {
			return notificationdomain.MarkdownTemplate{}, fmt.Errorf("%w: condition param_key is required", ErrInvalidInput)
		}
		if _, ok := allowedKeys[paramKey]; !ok {
			return notificationdomain.MarkdownTemplate{}, fmt.Errorf("%w: unsupported condition param_key %s", ErrInvalidInput, paramKey)
		}
		operator := notificationdomain.ConditionOperator(strings.ToLower(strings.TrimSpace(item.Operator)))
		if !operator.Valid() {
			return notificationdomain.MarkdownTemplate{}, fmt.Errorf("%w: invalid condition operator", ErrInvalidInput)
		}
		markdownText := strings.TrimSpace(strings.ReplaceAll(item.MarkdownText, "\r\n", "\n"))
		if markdownText == "" {
			return notificationdomain.MarkdownTemplate{}, fmt.Errorf("%w: condition markdown_text is required", ErrInvalidInput)
		}
		if err := validateNotificationMarkdownPlaceholders(allowedKeys, markdownText); err != nil {
			return notificationdomain.MarkdownTemplate{}, err
		}
		normalizedConditions = append(normalizedConditions, notificationdomain.MarkdownTemplateCondition{
			ParamKey:      paramKey,
			Operator:      operator,
			ExpectedValue: strings.TrimSpace(item.ExpectedValue),
			MarkdownText:  markdownText,
			SortNo:        idx + 1,
		})
	}
	return notificationdomain.MarkdownTemplate{
		Name:          name,
		TitleTemplate: titleTemplate,
		BodyTemplate:  bodyTemplate,
		Conditions:    normalizedConditions,
		Enabled:       enabled,
		Remark:        strings.TrimSpace(remark),
	}, nil
}

// normalizeHookInput 标准化输入值，保证后续逻辑使用统一格式。
func (uc *NotificationManager) normalizeHookInput(ctx context.Context, name, sourceID, markdownTemplateID string, enabled bool, remark string) (notificationdomain.Hook, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return notificationdomain.Hook{}, fmt.Errorf("%w: hook name is required", ErrInvalidInput)
	}
	sourceID = strings.TrimSpace(sourceID)
	if sourceID == "" {
		return notificationdomain.Hook{}, fmt.Errorf("%w: source_id is required", ErrInvalidInput)
	}
	markdownTemplateID = strings.TrimSpace(markdownTemplateID)
	if markdownTemplateID == "" {
		return notificationdomain.Hook{}, fmt.Errorf("%w: markdown_template_id is required", ErrInvalidInput)
	}
	source, err := uc.repo.GetSourceByID(ctx, sourceID)
	if err != nil {
		return notificationdomain.Hook{}, err
	}
	template, err := uc.repo.GetMarkdownTemplateByID(ctx, markdownTemplateID)
	if err != nil {
		return notificationdomain.Hook{}, err
	}
	return notificationdomain.Hook{
		Name:                 name,
		SourceID:             source.ID,
		SourceName:           source.Name,
		SourceType:           source.SourceType,
		MarkdownTemplateID:   template.ID,
		MarkdownTemplateName: template.Name,
		Enabled:              enabled,
		Remark:               strings.TrimSpace(remark),
	}, nil
}

// allowedMarkdownVariableKeys 封装当前模块的业务处理逻辑。
func (uc *NotificationManager) allowedMarkdownVariableKeys(ctx context.Context) (map[string]struct{}, error) {
	result := make(map[string]struct{}, len(notificationBuiltinKeys)+len(notificationSystemKeys)+16)
	for key := range notificationBuiltinKeys {
		result[key] = struct{}{}
	}
	for key := range notificationSystemKeys {
		result[key] = struct{}{}
	}
	if uc == nil || uc.platformRepo == nil {
		return result, nil
	}
	items, _, err := uc.platformRepo.List(ctx, platformparamdomain.ListFilter{Status: func() *platformparamdomain.Status { s := platformparamdomain.StatusEnabled; return &s }(), Page: 1, PageSize: 1000})
	if err != nil {
		return nil, err
	}
	for _, item := range items {
		key := strings.ToLower(strings.TrimSpace(item.ParamKey))
		if key != "" {
			result[key] = struct{}{}
		}
	}
	return result, nil
}

// validateNotificationMarkdownPlaceholders 封装当前模块的业务处理逻辑。
func validateNotificationMarkdownPlaceholders(allowedKeys map[string]struct{}, templates ...string) error {
	for _, tpl := range templates {
		matches := notificationTemplatePlaceholderPattern.FindAllStringSubmatch(tpl, -1)
		for _, match := range matches {
			if len(match) < 2 {
				continue
			}
			key := strings.ToLower(strings.TrimSpace(match[1]))
			if key == "" {
				continue
			}
			if _, ok := allowedKeys[key]; !ok {
				return fmt.Errorf("%w: unsupported markdown variable %s", ErrInvalidInput, key)
			}
		}
	}
	return nil
}

// renderNotificationMarkdownTemplate 封装当前模块的业务处理逻辑。
func renderNotificationMarkdownTemplate(values map[string]string, item notificationdomain.MarkdownTemplate) (string, string) {
	normalizedValues := normalizeNotificationVariables(values)
	title := renderHookString(normalizedValues, item.TitleTemplate)
	body := renderHookString(normalizedValues, item.BodyTemplate)
	if len(item.Conditions) == 0 {
		return strings.TrimSpace(title), strings.TrimSpace(body)
	}
	conditions := append([]notificationdomain.MarkdownTemplateCondition(nil), item.Conditions...)
	sort.SliceStable(conditions, func(i, j int) bool {
		return conditions[i].SortNo < conditions[j].SortNo
	})
	blocks := make([]string, 0, len(conditions)+1)
	if strings.TrimSpace(body) != "" {
		blocks = append(blocks, strings.TrimSpace(body))
	}
	for _, cond := range conditions {
		actualValue := normalizedValues[strings.ToLower(strings.TrimSpace(cond.ParamKey))]
		if !notificationConditionMatched(actualValue, cond.Operator, cond.ExpectedValue) {
			continue
		}
		block := strings.TrimSpace(renderHookString(normalizedValues, cond.MarkdownText))
		if block != "" {
			blocks = append(blocks, block)
		}
	}
	return strings.TrimSpace(title), strings.TrimSpace(strings.Join(blocks, "\n\n"))
}

// normalizeNotificationVariables 标准化输入值，保证后续逻辑使用统一格式。
func normalizeNotificationVariables(values map[string]string) map[string]string {
	if len(values) == 0 {
		return map[string]string{}
	}
	normalized := make(map[string]string, len(values))
	for key, value := range values {
		normalizedKey := strings.ToLower(strings.TrimSpace(key))
		if normalizedKey == "" {
			continue
		}
		normalized[normalizedKey] = strings.TrimSpace(value)
	}
	if strings.TrimSpace(normalized["release_stage_rich"]) == "" {
		if stage := strings.TrimSpace(normalized["release_stage"]); stage != "" {
			normalized["release_stage_rich"] = buildNotificationReleaseStageRichValue(stage)
		}
	}
	if strings.TrimSpace(normalized["release_status_rich"]) == "" {
		if status := strings.TrimSpace(normalized["release_status"]); status != "" {
			normalized["release_status_rich"] = buildNotificationReleaseStatusRichValue(status)
		}
	}
	return normalized
}

// notificationConditionMatched 封装当前模块的业务处理逻辑。
func notificationConditionMatched(actual string, operator notificationdomain.ConditionOperator, expected string) bool {
	actual = strings.TrimSpace(actual)
	expected = strings.TrimSpace(expected)
	switch operator {
	case notificationdomain.ConditionOperatorEquals:
		return actual == expected
	case notificationdomain.ConditionOperatorNotEquals:
		return actual != expected
	case notificationdomain.ConditionOperatorContains:
		if expected == "" {
			return false
		}
		return strings.Contains(actual, expected)
	case notificationdomain.ConditionOperatorNotContains:
		if expected == "" {
			return true
		}
		return !strings.Contains(actual, expected)
	case notificationdomain.ConditionOperatorIsEmpty:
		return actual == ""
	case notificationdomain.ConditionOperatorNotEmpty:
		return actual != ""
	default:
		return false
	}
}

// toNotificationSourceOutput 将领域对象转换为接口响应结构。
func toNotificationSourceOutput(item notificationdomain.Source) NotificationSourceOutput {
	return NotificationSourceOutput{
		ID:                   item.ID,
		Name:                 item.Name,
		SourceType:           string(item.SourceType),
		WebhookURL:           item.WebhookURL,
		HasVerificationParam: strings.TrimSpace(item.VerificationParam) != "",
		Keywords:             item.Keywords,
		Enabled:              item.Enabled,
		Remark:               item.Remark,
		CreatedBy:            item.CreatedBy,
		UpdatedBy:            item.UpdatedBy,
		CreatedAt:            item.CreatedAt,
		UpdatedAt:            item.UpdatedAt,
	}
}

// toNotificationMarkdownTemplateOutput 将领域对象转换为接口响应结构。
func toNotificationMarkdownTemplateOutput(item notificationdomain.MarkdownTemplate) NotificationMarkdownTemplateOutput {
	conditions := make([]NotificationMarkdownTemplateConditionOutput, 0, len(item.Conditions))
	for _, cond := range item.Conditions {
		conditions = append(conditions, NotificationMarkdownTemplateConditionOutput{
			ParamKey:      cond.ParamKey,
			Operator:      string(cond.Operator),
			ExpectedValue: cond.ExpectedValue,
			MarkdownText:  cond.MarkdownText,
			SortNo:        cond.SortNo,
		})
	}
	return NotificationMarkdownTemplateOutput{
		ID:            item.ID,
		Name:          item.Name,
		TitleTemplate: item.TitleTemplate,
		BodyTemplate:  item.BodyTemplate,
		Conditions:    conditions,
		Enabled:       item.Enabled,
		Remark:        item.Remark,
		CreatedBy:     item.CreatedBy,
		UpdatedBy:     item.UpdatedBy,
		CreatedAt:     item.CreatedAt,
		UpdatedAt:     item.UpdatedAt,
	}
}

// toNotificationHookOutput 将领域对象转换为接口响应结构。
func toNotificationHookOutput(item notificationdomain.Hook) NotificationHookOutput {
	return NotificationHookOutput{
		ID:                   item.ID,
		Name:                 item.Name,
		SourceID:             item.SourceID,
		SourceName:           item.SourceName,
		SourceType:           string(item.SourceType),
		MarkdownTemplateID:   item.MarkdownTemplateID,
		MarkdownTemplateName: item.MarkdownTemplateName,
		Enabled:              item.Enabled,
		Remark:               item.Remark,
		CreatedBy:            item.CreatedBy,
		UpdatedBy:            item.UpdatedBy,
		CreatedAt:            item.CreatedAt,
		UpdatedAt:            item.UpdatedAt,
	}
}
