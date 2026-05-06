package usecase

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	platformparamdomain "gos/internal/domain/platformparam"
	domain "gos/internal/domain/release"
)

type ReleaseOrderValueProgressStatus string

const (
	ReleaseOrderValueProgressPending  ReleaseOrderValueProgressStatus = "pending"
	ReleaseOrderValueProgressRunning  ReleaseOrderValueProgressStatus = "running"
	ReleaseOrderValueProgressResolved ReleaseOrderValueProgressStatus = "resolved"
	ReleaseOrderValueProgressFailed   ReleaseOrderValueProgressStatus = "failed"
	ReleaseOrderValueProgressSkipped  ReleaseOrderValueProgressStatus = "skipped"
)

type ReleaseOrderValueProgressItem struct {
	PipelineScope     domain.PipelineScope
	ParamKey          string
	ParamName         string
	ExecutorParamName string
	Required          bool
	Status            ReleaseOrderValueProgressStatus
	Value             string
	ValueSource       string
	Message           string
	UpdatedAt         *time.Time
	SortNo            int
}

// ListValueProgress 按发布模板里已选中的平台标准 Key 生成一份“取值进度”视图。
//
// 设计目标：
// 1. 让用户看到“当前模板里映射过的字段”现在取到了什么值；
// 2. 当值不是发布单创建时立即确定，而是在执行过程中才产生时，也能实时反馈；
// 3. 不额外持久化一张表，尽量基于模板参数、参数快照和执行态实时计算。
func (uc *ReleaseOrderManager) ListValueProgress(
	ctx context.Context,
	orderID string,
) ([]ReleaseOrderValueProgressItem, error) {
	orderID = strings.TrimSpace(orderID)
	if orderID == "" {
		return nil, ErrInvalidID
	}

	order, err := uc.repo.GetByID(ctx, orderID)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(order.TemplateID) == "" {
		return []ReleaseOrderValueProgressItem{}, nil
	}

	_, templateBindings, templateParams, _, _, err := uc.repo.GetTemplateByID(ctx, order.TemplateID)
	if err != nil {
		return nil, err
	}

	orderParams, err := uc.repo.ListParams(ctx, order.ID)
	if err != nil {
		return nil, err
	}
	executions, err := uc.repo.ListExecutions(ctx, order.ID)
	if err != nil {
		return nil, err
	}

	paramsByScopeKey := indexReleaseOrderParams(orderParams)
	executionByScope := make(map[domain.PipelineScope]domain.ReleaseOrderExecution, len(executions))
	for _, item := range executions {
		executionByScope[item.PipelineScope] = item
	}

	items := make([]ReleaseOrderValueProgressItem, 0, len(templateParams)+4)
	for _, param := range templateParams {
		items = append(items, resolveReleaseOrderValueProgressItem(order, param, paramsByScopeKey, executionByScope))
	}

	builtinItems, err := uc.buildBuiltinValueProgress(
		ctx,
		order,
		templateBindings,
		items,
		paramsByScopeKey,
		executionByScope,
	)
	if err != nil {
		return nil, err
	}
	items = append(items, builtinItems...)

	if err := uc.applyRollbackSnapshotValueProgress(ctx, order, items); err != nil {
		return nil, err
	}

	sort.SliceStable(items, func(i, j int) bool {
		if items[i].PipelineScope != items[j].PipelineScope {
			return strings.Compare(string(items[i].PipelineScope), string(items[j].PipelineScope)) < 0
		}
		if items[i].SortNo != items[j].SortNo {
			return items[i].SortNo < items[j].SortNo
		}
		return strings.Compare(items[i].ParamKey, items[j].ParamKey) < 0
	})
	return items, nil
}

// applyRollbackSnapshotValueProgress 封装当前模块的业务处理逻辑。
func (uc *ReleaseOrderManager) applyRollbackSnapshotValueProgress(
	ctx context.Context,
	order domain.ReleaseOrder,
	items []ReleaseOrderValueProgressItem,
) error {
	if uc == nil || uc.repo == nil {
		return nil
	}
	if !strings.EqualFold(strings.TrimSpace(string(order.OperationType)), string(domain.OperationTypeRollback)) {
		return nil
	}
	sourceOrderID := strings.TrimSpace(order.SourceOrderID)
	if sourceOrderID == "" {
		return nil
	}
	snapshot, err := uc.repo.GetDeploySnapshotByOrderID(ctx, sourceOrderID)
	if err != nil {
		if errors.Is(err, domain.ErrDeploySnapshotNotFound) {
			return nil
		}
		return err
	}
	_, imageVersion, err := decodeHelmDeploySnapshot(snapshot)
	if err != nil {
		return nil
	}
	imageVersion = strings.TrimSpace(imageVersion)
	if imageVersion == "" {
		return nil
	}
	for idx := range items {
		if items[idx].PipelineScope != domain.PipelineScopeCD {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(items[idx].ParamKey))
		if key != "image_version" && key != "image_tag" {
			continue
		}
		if items[idx].Status == ReleaseOrderValueProgressResolved && strings.TrimSpace(items[idx].Value) != "" {
			continue
		}
		items[idx].Value = imageVersion
		items[idx].ValueSource = "deploy_snapshot"
		items[idx].Status = ReleaseOrderValueProgressResolved
		items[idx].Message = "已从来源成功单部署快照取值"
		items[idx].UpdatedAt = timePointer(snapshot.CreatedAt)
	}
	return nil
}

// resolveReleaseOrderValueProgressItem 解析上下文数据，得到后续流程需要的结果。
func resolveReleaseOrderValueProgressItem(
	order domain.ReleaseOrder,
	param domain.ReleaseTemplateParam,
	paramsByScopeKey map[string]indexedReleaseParam,
	executionByScope map[domain.PipelineScope]domain.ReleaseOrderExecution,
) ReleaseOrderValueProgressItem {
	progress := ReleaseOrderValueProgressItem{
		PipelineScope:     param.PipelineScope,
		ParamKey:          strings.TrimSpace(param.ParamKey),
		ParamName:         firstNonEmpty(strings.TrimSpace(param.ParamName), strings.TrimSpace(param.ParamKey)),
		ExecutorParamName: strings.TrimSpace(param.ExecutorParamName),
		Required:          param.Required,
		Status:            ReleaseOrderValueProgressPending,
		SortNo:            param.SortNo,
	}

	if snapshot, ok := findIndexedReleaseParam(paramsByScopeKey, param.PipelineScope, progress.ParamKey); ok {
		progress.Value = strings.TrimSpace(snapshot.ParamValue)
		progress.ValueSource = strings.TrimSpace(string(snapshot.ValueSource))
		progress.UpdatedAt = timePointer(snapshot.CreatedAt)
		progress.Status = ReleaseOrderValueProgressResolved
		progress.Message = buildResolvedMessage(progress.ValueSource)
		return progress
	}

	execution, hasExecution := executionByScope[param.PipelineScope]
	if derived, ok := deriveReleaseProgressValue(order, progress, execution); ok {
		progress.Value = derived.Value
		progress.ValueSource = derived.Source
		progress.UpdatedAt = derived.UpdatedAt
		progress.Status = ReleaseOrderValueProgressResolved
		progress.Message = derived.Message
		return progress
	}

	progress.Status, progress.Message = derivePendingProgressState(order, progress.Required, hasExecution, execution)
	if hasExecution {
		progress.UpdatedAt = timePointer(execution.UpdatedAt)
	}
	return progress
}

// buildBuiltinValueProgress 会把系统内置字段自动纳入“取值进度”。
//
// 设计原因：
// 1. 内置字段属于平台治理能力，不应该依赖用户是否在模板里显式勾选；
// 2. 只要当前模板启用了某个执行单元，该执行单元就应该能看到内置字段的取值状态；
// 3. 当 CD 走 ArgoCD 时，CD 视图里的内置字段优先展示“沿用 CI”的结果，帮助用户理解最终部署值来自哪里。
func (uc *ReleaseOrderManager) buildBuiltinValueProgress(
	ctx context.Context,
	order domain.ReleaseOrder,
	bindings []domain.ReleaseTemplateBinding,
	baseItems []ReleaseOrderValueProgressItem,
	paramsByScopeKey map[string]indexedReleaseParam,
	executionByScope map[domain.PipelineScope]domain.ReleaseOrderExecution,
) ([]ReleaseOrderValueProgressItem, error) {
	if uc.platformRepo == nil {
		return nil, nil
	}

	builtinDicts, err := uc.listEnabledBuiltinPlatformParams(ctx)
	if err != nil {
		return nil, err
	}
	if len(builtinDicts) == 0 {
		return nil, nil
	}

	baseByScopeKey := make(map[string]ReleaseOrderValueProgressItem, len(baseItems))
	for _, item := range baseItems {
		baseByScopeKey[buildValueProgressScopeKey(item.PipelineScope, item.ParamKey)] = item
	}

	generated := make([]ReleaseOrderValueProgressItem, 0, len(bindings)*max(1, len(builtinDicts)))
	usesArgoCDCD := templateUsesArgoCDForCD(bindings)
	appKey := ""
	if uc.appRepo != nil && strings.TrimSpace(order.ApplicationID) != "" {
		if appRecord, err := uc.appRepo.GetByID(ctx, strings.TrimSpace(order.ApplicationID)); err == nil {
			appKey = strings.TrimSpace(appRecord.Key)
		}
	}
	for _, binding := range bindings {
		for _, dict := range builtinDicts {
			key := strings.ToLower(strings.TrimSpace(dict.ParamKey))
			if key == "" {
				continue
			}
			scopeKey := buildValueProgressScopeKey(binding.PipelineScope, key)
			if _, exists := baseByScopeKey[scopeKey]; exists {
				continue
			}

			progress, ok := buildBuiltinProgressItem(
				order,
				appKey,
				binding.PipelineScope,
				dict,
				baseByScopeKey,
				paramsByScopeKey,
				executionByScope,
				usesArgoCDCD,
			)
			if !ok {
				continue
			}
			generated = append(generated, progress)
			baseByScopeKey[scopeKey] = progress
		}
	}
	return generated, nil
}

// listEnabledBuiltinPlatformParams 查询并返回列表数据。
func (uc *ReleaseOrderManager) listEnabledBuiltinPlatformParams(
	ctx context.Context,
) (map[string]platformparamdomain.PlatformParamDict, error) {
	builtin := true
	status := platformparamdomain.StatusEnabled
	items, _, err := uc.platformRepo.List(ctx, platformparamdomain.ListFilter{
		Builtin:  &builtin,
		Status:   &status,
		Page:     1,
		PageSize: 500,
	})
	if err != nil {
		return nil, err
	}
	result := make(map[string]platformparamdomain.PlatformParamDict, len(items))
	for _, item := range items {
		key := strings.ToLower(strings.TrimSpace(item.ParamKey))
		if key == "" {
			continue
		}
		result[key] = item
	}
	return result, nil
}

// templateUsesArgoCDForCD 封装当前模块的业务处理逻辑。
func templateUsesArgoCDForCD(bindings []domain.ReleaseTemplateBinding) bool {
	for _, item := range bindings {
		if item.PipelineScope != domain.PipelineScopeCD {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(item.Provider), "argocd") {
			return true
		}
	}
	return false
}

// buildMirroredBuiltinMessage 组装业务执行所需的输入数据。
func buildMirroredBuiltinMessage(item ReleaseOrderValueProgressItem) string {
	switch item.Status {
	case ReleaseOrderValueProgressResolved:
		return "当前 CD 会复用 CI 中已解析出的内置字段值"
	case ReleaseOrderValueProgressRunning:
		return "当前 CD 正等待 CI 先产出内置字段值"
	case ReleaseOrderValueProgressFailed:
		return "CI 内置字段取值失败，CD 暂时无法继续复用"
	case ReleaseOrderValueProgressSkipped:
		return "CI 未产出该内置字段值，CD 本次不会复用"
	default:
		return "当前 CD 会沿用 CI 的内置字段值，等待 CI 执行单元先完成取值"
	}
}

// buildBuiltinProgressItem 组装业务执行所需的输入数据。
func buildBuiltinProgressItem(
	order domain.ReleaseOrder,
	appKey string,
	scope domain.PipelineScope,
	dict platformparamdomain.PlatformParamDict,
	baseByScopeKey map[string]ReleaseOrderValueProgressItem,
	paramsByScopeKey map[string]indexedReleaseParam,
	executionByScope map[domain.PipelineScope]domain.ReleaseOrderExecution,
	usesArgoCDCD bool,
) (ReleaseOrderValueProgressItem, bool) {
	paramKey := strings.TrimSpace(dict.ParamKey)
	if paramKey == "" {
		return ReleaseOrderValueProgressItem{}, false
	}
	if strings.EqualFold(paramKey, "app_key") && strings.TrimSpace(appKey) != "" {
		progress := ReleaseOrderValueProgressItem{
			PipelineScope:     scope,
			ParamKey:          paramKey,
			ParamName:         firstNonEmpty(strings.TrimSpace(dict.Name), paramKey),
			ExecutorParamName: "系统内置",
			Required:          dict.Required,
			Status:            ReleaseOrderValueProgressResolved,
			Value:             strings.TrimSpace(appKey),
			ValueSource:       "application_key",
			Message:           "已从应用标识取值",
			UpdatedAt:         timePointer(order.UpdatedAt),
			SortNo:            normalizeBuiltinProgressSortNo(dict.ParamKey),
		}
		return progress, true
	}

	if scope == domain.PipelineScopeCD && usesArgoCDCD {
		if ciProgress, hasCIProgress := baseByScopeKey[buildValueProgressScopeKey(domain.PipelineScopeCI, paramKey)]; hasCIProgress {
			progress := ReleaseOrderValueProgressItem{
				PipelineScope:     domain.PipelineScopeCD,
				ParamKey:          paramKey,
				ParamName:         firstNonEmpty(strings.TrimSpace(dict.Name), paramKey),
				ExecutorParamName: "沿用 CI",
				Required:          true,
				Status:            ciProgress.Status,
				Value:             ciProgress.Value,
				ValueSource:       ciProgress.ValueSource,
				Message:           buildMirroredBuiltinMessage(ciProgress),
				UpdatedAt:         ciProgress.UpdatedAt,
				SortNo:            normalizeBuiltinProgressSortNo(dict.ParamKey),
			}
			if progress.UpdatedAt == nil {
				if execution, ok := executionByScope[domain.PipelineScopeCD]; ok {
					progress.UpdatedAt = timePointer(execution.UpdatedAt)
				}
			}
			return progress, true
		}
	}

	progress := resolveReleaseOrderValueProgressItem(order, domain.ReleaseTemplateParam{
		PipelineScope:     scope,
		ParamKey:          paramKey,
		ParamName:         firstNonEmpty(strings.TrimSpace(dict.Name), paramKey),
		ExecutorParamName: "系统内置",
		Required:          dict.Required,
		SortNo:            normalizeBuiltinProgressSortNo(dict.ParamKey),
	}, paramsByScopeKey, executionByScope)
	if progress.Message == "" {
		progress.Message = "系统内置字段，平台会自动跟踪其取值状态"
	}
	return progress, true
}

// normalizeBuiltinProgressSortNo 标准化输入值，保证后续逻辑使用统一格式。
func normalizeBuiltinProgressSortNo(paramKey string) int {
	key := strings.ToLower(strings.TrimSpace(paramKey))
	if key == "app_key" {
		return 8990
	}
	if key == "image_version" {
		return 9000
	}
	return 9100
}

// max 封装当前模块的业务处理逻辑。
func max(a int, b int) int {
	if a > b {
		return a
	}
	return b
}

type indexedReleaseParam struct {
	ParamValue  string
	ValueSource domain.ValueSource
	CreatedAt   time.Time
}

// indexReleaseOrderParams 封装当前模块的业务处理逻辑。
func indexReleaseOrderParams(items []domain.ReleaseOrderParam) map[string]indexedReleaseParam {
	result := make(map[string]indexedReleaseParam, len(items))
	for _, item := range items {
		key := buildValueProgressScopeKey(item.PipelineScope, item.ParamKey)
		value := strings.TrimSpace(item.ParamValue)
		if key == "" || value == "" {
			continue
		}
		result[key] = indexedReleaseParam{
			ParamValue:  value,
			ValueSource: item.ValueSource,
			CreatedAt:   item.CreatedAt,
		}
	}
	return result
}

// findIndexedReleaseParam 封装当前模块的业务处理逻辑。
func findIndexedReleaseParam(
	items map[string]indexedReleaseParam,
	scope domain.PipelineScope,
	paramKey string,
) (indexedReleaseParam, bool) {
	item, ok := items[buildValueProgressScopeKey(scope, paramKey)]
	return item, ok
}

// buildValueProgressScopeKey 组装业务执行所需的输入数据。
func buildValueProgressScopeKey(scope domain.PipelineScope, paramKey string) string {
	return strings.ToLower(strings.TrimSpace(string(scope))) + "::" + strings.ToLower(strings.TrimSpace(paramKey))
}

type derivedProgressValue struct {
	Value     string
	Source    string
	Message   string
	UpdatedAt *time.Time
}

// deriveReleaseProgressValue 封装当前模块的业务处理逻辑。
func deriveReleaseProgressValue(
	order domain.ReleaseOrder,
	item ReleaseOrderValueProgressItem,
	execution domain.ReleaseOrderExecution,
) (derivedProgressValue, bool) {
	paramKey := strings.ToLower(strings.TrimSpace(item.ParamKey))
	switch paramKey {
	case "image_version":
		if strings.EqualFold(strings.TrimSpace(execution.Provider), "jenkins") {
			buildNumber := parseJenkinsBuildNumber(execution.BuildURL)
			if buildNumber != "" {
				return derivedProgressValue{
					Value:     buildNumber,
					Source:    "jenkins_build_number",
					Message:   "已从 Jenkins 构建号 BUILD_NUMBER 自动取值",
					UpdatedAt: timePointer(execution.UpdatedAt),
				}, true
			}
		}
		if value := strings.TrimSpace(order.ImageTag); value != "" {
			return derivedProgressValue{
				Value:     value,
				Source:    "release_order_summary",
				Message:   "已从发布单摘要字段取值",
				UpdatedAt: timePointer(order.UpdatedAt),
			}, true
		}
	case "image_tag":
		if value := strings.TrimSpace(order.ImageTag); value != "" {
			return derivedProgressValue{
				Value:     value,
				Source:    "release_order_summary",
				Message:   "已从发布单摘要字段取值",
				UpdatedAt: timePointer(order.UpdatedAt),
			}, true
		}
	case "env", "env_code":
		if value := strings.TrimSpace(order.EnvCode); value != "" {
			return derivedProgressValue{
				Value:     value,
				Source:    "release_order_summary",
				Message:   "已从发布单摘要字段取值",
				UpdatedAt: timePointer(order.UpdatedAt),
			}, true
		}
	case "branch", "git_ref":
		if value := strings.TrimSpace(order.GitRef); value != "" {
			return derivedProgressValue{
				Value:     value,
				Source:    "release_order_summary",
				Message:   "已从发布单摘要字段取值",
				UpdatedAt: timePointer(order.UpdatedAt),
			}, true
		}
	case "project_name":
		if value := strings.TrimSpace(order.SonService); value != "" {
			return derivedProgressValue{
				Value:     value,
				Source:    "release_order_summary",
				Message:   "已从发布单摘要字段取值",
				UpdatedAt: timePointer(order.UpdatedAt),
			}, true
		}
	}
	return derivedProgressValue{}, false
}

// parseJenkinsBuildNumber 解析输入内容并返回结构化结果。
func parseJenkinsBuildNumber(buildURL string) string {
	text := strings.Trim(strings.TrimSpace(buildURL), "/")
	if text == "" {
		return ""
	}
	parts := strings.Split(text, "/")
	if len(parts) == 0 {
		return ""
	}
	last := strings.TrimSpace(parts[len(parts)-1])
	for _, ch := range last {
		if ch < '0' || ch > '9' {
			return ""
		}
	}
	return last
}

// buildResolvedMessage 解析上下文数据，得到后续流程需要的结果。
func buildResolvedMessage(source string) string {
	switch strings.ToLower(strings.TrimSpace(source)) {
	case "release_input":
		return "已从发布单填写值中取值"
	case "fixed":
		return "已从模板固定值中取值"
	case "application":
		return "已从应用默认值中取值"
	case "environment":
		return "已从环境默认值中取值"
	case "jenkins_build_number":
		return "已从 Jenkins 构建号 BUILD_NUMBER 自动取值"
	default:
		if strings.TrimSpace(source) == "" {
			return "已取值"
		}
		return fmt.Sprintf("已从 %s 取值", source)
	}
}

// derivePendingProgressState 封装当前模块的业务处理逻辑。
func derivePendingProgressState(
	order domain.ReleaseOrder,
	required bool,
	hasExecution bool,
	execution domain.ReleaseOrderExecution,
) (ReleaseOrderValueProgressStatus, string) {
	if !hasExecution {
		if order.Status.IsTerminal() {
			if required {
				return ReleaseOrderValueProgressFailed, "执行已结束，但字段仍未取到值"
			}
			return ReleaseOrderValueProgressSkipped, "该字段未参与本次执行"
		}
		return ReleaseOrderValueProgressPending, "等待执行单元开始解析字段"
	}

	switch execution.Status {
	case domain.ExecutionStatusRunning:
		return ReleaseOrderValueProgressRunning, "正在等待执行器返回字段值"
	case domain.ExecutionStatusSuccess:
		if required {
			return ReleaseOrderValueProgressFailed, "执行已完成，但字段仍未取到值"
		}
		return ReleaseOrderValueProgressSkipped, "执行已完成，该字段未产生取值"
	case domain.ExecutionStatusFailed, domain.ExecutionStatusCancelled:
		return ReleaseOrderValueProgressFailed, "执行失败，字段未能成功取值"
	case domain.ExecutionStatusSkipped:
		return ReleaseOrderValueProgressSkipped, "该执行单元已跳过"
	default:
		return ReleaseOrderValueProgressPending, "等待执行器开始处理字段"
	}
}

// timePointer 封装当前模块的业务处理逻辑。
func timePointer(value time.Time) *time.Time {
	if value.IsZero() {
		return nil
	}
	copyValue := value
	return &copyValue
}
