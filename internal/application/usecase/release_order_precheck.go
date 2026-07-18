package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	argocddomain "gos/internal/domain/argocdapp"
	pipelineparamdomain "gos/internal/domain/executorparam"
	pipelinedomain "gos/internal/domain/pipeline"
	domain "gos/internal/domain/release"
)

type ReleaseOrderPrecheckItemStatus string

const (
	ReleaseOrderPrecheckItemStatusPass    ReleaseOrderPrecheckItemStatus = "pass"
	ReleaseOrderPrecheckItemStatusWarn    ReleaseOrderPrecheckItemStatus = "warn"
	ReleaseOrderPrecheckItemStatusBlocked ReleaseOrderPrecheckItemStatus = "blocked"
)

type ReleaseOrderPrecheckItem struct {
	Key     string                         `json:"key"`
	Name    string                         `json:"name"`
	Status  ReleaseOrderPrecheckItemStatus `json:"status"`
	Message string                         `json:"message"`
}

type ReleaseOrderPrecheckOutput struct {
	OrderID          string                     `json:"order_id"`
	OrderNo          string                     `json:"order_no"`
	Executable       bool                       `json:"executable"`
	WaitingForLock   bool                       `json:"waiting_for_lock"`
	AheadCount       int                        `json:"ahead_count"`
	LockEnabled      bool                       `json:"lock_enabled"`
	LockScope        string                     `json:"lock_scope"`
	ConflictStrategy string                     `json:"conflict_strategy"`
	LockKey          string                     `json:"lock_key"`
	ConflictOrderNo  string                     `json:"conflict_order_no"`
	ConflictMessage  string                     `json:"conflict_message"`
	Items            []ReleaseOrderPrecheckItem `json:"items"`
}

type releaseDispatchGuard struct {
	Settings       ReleaseConcurrencySettingsOutput
	LockScope      domain.ExecutionLockScope
	LockKey        string
	ConflictLock   *domain.ReleaseExecutionLock
	ConflictOrder  *domain.ReleaseOrder
	WaitingForLock bool
	AheadCount     int
	Message        string
}

type ReleaseOrderDispatchAction string

const (
	ReleaseOrderDispatchActionExecute ReleaseOrderDispatchAction = "execute"
	ReleaseOrderDispatchActionBuild   ReleaseOrderDispatchAction = "build"
	ReleaseOrderDispatchActionDeploy  ReleaseOrderDispatchAction = "deploy"
)

// PrecheckExecute 检查业务状态并返回校验结果。
func (uc *ReleaseOrderManager) PrecheckExecute(ctx context.Context, id string) (ReleaseOrderPrecheckOutput, error) {
	return uc.precheckOrderDispatch(ctx, id, ReleaseOrderDispatchActionExecute)
}

// PrecheckBuild 组装业务执行所需的输入数据。
func (uc *ReleaseOrderManager) PrecheckBuild(ctx context.Context, id string) (ReleaseOrderPrecheckOutput, error) {
	return uc.precheckOrderDispatch(ctx, id, ReleaseOrderDispatchActionBuild)
}

// PrecheckDeploy 检查业务状态并返回校验结果。
func (uc *ReleaseOrderManager) PrecheckDeploy(ctx context.Context, id string) (ReleaseOrderPrecheckOutput, error) {
	return uc.precheckOrderDispatch(ctx, id, ReleaseOrderDispatchActionDeploy)
}

// precheckOrderDispatch 检查业务状态并返回校验结果。
func (uc *ReleaseOrderManager) precheckOrderDispatch(
	ctx context.Context,
	id string,
	action ReleaseOrderDispatchAction,
) (ReleaseOrderPrecheckOutput, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return ReleaseOrderPrecheckOutput{}, ErrInvalidID
	}
	order, err := uc.repo.GetByID(ctx, id)
	if err != nil {
		return ReleaseOrderPrecheckOutput{}, err
	}
	executions, err := uc.repo.ListExecutions(ctx, order.ID)
	if err != nil {
		return ReleaseOrderPrecheckOutput{}, err
	}
	params, err := uc.repo.ListParams(ctx, order.ID)
	if err != nil {
		return ReleaseOrderPrecheckOutput{}, err
	}
	return uc.buildOrderPrecheck(ctx, order, executions, params, action)
}

// buildOrderPrecheck 组装业务执行所需的输入数据。
func (uc *ReleaseOrderManager) buildOrderPrecheck(
	ctx context.Context,
	order domain.ReleaseOrder,
	executions []domain.ReleaseOrderExecution,
	params []domain.ReleaseOrderParam,
	action ReleaseOrderDispatchAction,
) (ReleaseOrderPrecheckOutput, error) {
	output := ReleaseOrderPrecheckOutput{
		OrderID:    order.ID,
		OrderNo:    order.OrderNo,
		Executable: true,
		Items:      make([]ReleaseOrderPrecheckItem, 0, 4),
	}

	statusItem, executionItem, pendingExecution := uc.resolveDispatchPrecheckItems(order, executions, action)
	if statusItem.Status == ReleaseOrderPrecheckItemStatusBlocked {
		output.Executable = false
	}
	output.Items = append(output.Items, statusItem)

	if executionItem.Status == ReleaseOrderPrecheckItemStatusBlocked {
		output.Executable = false
	}
	output.Items = append(output.Items, executionItem)

	templateBindings, templateParams, templateLoaded, templateItem, err := uc.buildTemplateCompliancePrecheckItem(ctx, order)
	if err != nil {
		return ReleaseOrderPrecheckOutput{}, err
	}
	if templateItem.Key != "" {
		if templateItem.Status == ReleaseOrderPrecheckItemStatusBlocked {
			output.Executable = false
		}
		output.Items = append(output.Items, templateItem)
	}
	if templateLoaded {
		paramMappingItem, err := uc.buildTemplateParamMappingPrecheckItem(ctx, templateBindings, templateParams, executions, params, action)
		if err != nil {
			return ReleaseOrderPrecheckOutput{}, err
		}
		if paramMappingItem.Status == ReleaseOrderPrecheckItemStatusBlocked {
			output.Executable = false
		}
		output.Items = append(output.Items, paramMappingItem)
	}

	if pendingExecution != nil {
		if referenceItem, ok, err := uc.buildExecutionReferencePrecheckItem(ctx, *pendingExecution); err != nil {
			return ReleaseOrderPrecheckOutput{}, err
		} else if ok {
			if referenceItem.Status == ReleaseOrderPrecheckItemStatusBlocked {
				output.Executable = false
			}
			output.Items = append(output.Items, referenceItem)
		}
		guard, err := uc.evaluateDispatchGuard(ctx, order, *pendingExecution, params)
		if err != nil {
			return ReleaseOrderPrecheckOutput{}, err
		}
		output.LockEnabled = guard.Settings.Enabled
		output.LockScope = string(guard.Settings.LockScope)
		output.ConflictStrategy = string(guard.Settings.ConflictStrategy)
		output.LockKey = guard.LockKey
		switch {
		case guard.ConflictLock != nil:
			output.ConflictOrderNo = strings.TrimSpace(guard.ConflictLock.ReleaseOrderNo)
			output.ConflictMessage = strings.TrimSpace(guard.Message)
		case guard.ConflictOrder != nil:
			output.ConflictOrderNo = strings.TrimSpace(guard.ConflictOrder.OrderNo)
			output.ConflictMessage = strings.TrimSpace(guard.Message)
		}
		output.AheadCount = guard.AheadCount
		if guard.Settings.Enabled || guard.ConflictLock != nil || guard.ConflictOrder != nil {
			itemName := "并发发布"
			if !guard.Settings.Enabled && guard.ConflictOrder != nil {
				itemName = "执行顺序"
			}
			item := ReleaseOrderPrecheckItem{
				Key:     "concurrency_lock",
				Name:    itemName,
				Status:  ReleaseOrderPrecheckItemStatusPass,
				Message: "未检测到执行互斥冲突",
			}
			if guard.Settings.Enabled {
				item.Message = fmt.Sprintf("并发控制已启用，当前按 %s 加锁", guard.Settings.LockScope)
			}
			switch {
			case (guard.ConflictLock != nil || guard.ConflictOrder != nil) && guard.WaitingForLock:
				item.Status = ReleaseOrderPrecheckItemStatusWarn
				item.Message = guard.Message
				output.WaitingForLock = true
			case guard.ConflictOrder != nil:
				item.Status = ReleaseOrderPrecheckItemStatusBlocked
				item.Message = guard.Message
				output.Executable = false
			case guard.ConflictLock != nil && guard.Settings.ConflictStrategy == ReleaseConcurrencyConflictStrategyReject:
				item.Status = ReleaseOrderPrecheckItemStatusBlocked
				item.Message = guard.Message
				output.Executable = false
			case guard.ConflictLock != nil && guard.Settings.ConflictStrategy == ReleaseConcurrencyConflictStrategyQueue:
				item.Status = ReleaseOrderPrecheckItemStatusWarn
				item.Message = guard.Message
				output.WaitingForLock = true
			}
			output.Items = append(output.Items, item)
		}
	}

	return output, nil
}

func (uc *ReleaseOrderManager) buildTemplateCompliancePrecheckItem(
	ctx context.Context,
	order domain.ReleaseOrder,
) ([]domain.ReleaseTemplateBinding, []domain.ReleaseTemplateParam, bool, ReleaseOrderPrecheckItem, error) {
	templateID := strings.TrimSpace(order.TemplateID)
	item := ReleaseOrderPrecheckItem{
		Key:     "template_compliance",
		Name:    "发布模板规范",
		Status:  ReleaseOrderPrecheckItemStatusPass,
		Message: "发布模板未检测到管线规范违规",
	}
	if templateID == "" {
		return nil, nil, false, ReleaseOrderPrecheckItem{}, nil
	}
	template, bindings, params, _, _, err := uc.repo.GetTemplateByID(ctx, templateID)
	if err != nil {
		if errors.Is(err, domain.ErrTemplateNotFound) {
			return nil, nil, false, ReleaseOrderPrecheckItem{}, nil
		}
		return nil, nil, false, ReleaseOrderPrecheckItem{}, err
	}
	if template.Status != domain.TemplateStatusActive {
		item.Status = ReleaseOrderPrecheckItemStatusBlocked
		item.Message = fmt.Sprintf("发布模板 %s 已禁用，请先更新发布单模板", firstNonEmpty(template.Name, templateID))
		return bindings, params, true, item, nil
	}
	compliance := evaluateReleaseTemplateCompliance(ctx, uc.pipelineScanRepo, bindings)
	switch compliance.Status {
	case domain.TemplateComplianceStatusViolated:
		item.Status = ReleaseOrderPrecheckItemStatusBlocked
		item.Message = fmt.Sprintf("发布模板 %s 违反管线规范，%s", firstNonEmpty(template.Name, templateID), firstNonEmpty(compliance.Summary, "请先修复模板绑定管线"))
	case domain.TemplateComplianceStatusUnknown:
		item.Status = ReleaseOrderPrecheckItemStatusWarn
		item.Message = firstNonEmpty(compliance.Summary, "发布模板规范状态暂不可用")
	default:
		item.Message = fmt.Sprintf("发布模板 %s 未检测到管线规范违规", firstNonEmpty(template.Name, templateID))
	}
	return bindings, params, true, item, nil
}

func (uc *ReleaseOrderManager) buildTemplateParamMappingPrecheckItem(
	ctx context.Context,
	bindings []domain.ReleaseTemplateBinding,
	templateParams []domain.ReleaseTemplateParam,
	executions []domain.ReleaseOrderExecution,
	orderParams []domain.ReleaseOrderParam,
	action ReleaseOrderDispatchAction,
) (ReleaseOrderPrecheckItem, error) {
	item := ReleaseOrderPrecheckItem{
		Key:     "template_param_mapping",
		Name:    "CI/CD 参数映射",
		Status:  ReleaseOrderPrecheckItemStatusPass,
		Message: "待执行 CI/CD 参数映射完整",
	}
	scopes := precheckParamMappingScopes(executions, action)
	if len(scopes) == 0 {
		item.Message = "当前没有待执行的 CI/CD 单元"
		return item, nil
	}

	bindingByScope := make(map[domain.PipelineScope]domain.ReleaseTemplateBinding, len(bindings))
	for _, binding := range bindings {
		if binding.Enabled {
			bindingByScope[binding.PipelineScope] = binding
		}
	}

	liveParamsByScope, liveParamErrors := uc.loadLiveJenkinsParamSnapshots(ctx, bindingByScope, scopes)
	orderParamValues := indexReleaseOrderParamValues(orderParams)
	missing := make([]string, 0)
	missing = append(missing, liveParamErrors...)
	for _, param := range templateParams {
		if !scopes[param.PipelineScope] {
			continue
		}
		binding := bindingByScope[param.PipelineScope]
		if strings.ToLower(strings.TrimSpace(binding.Provider)) != string(pipelinedomain.ProviderJenkins) {
			continue
		}
		selectedValue := lookupReleaseOrderParamValue(orderParamValues, param.PipelineScope, param.ExecutorParamName, param.ParamKey)
		missing = append(missing, uc.validateSingleTemplateParamMapping(ctx, param, liveParamsByScope[param.PipelineScope], selectedValue)...)
	}

	if len(missing) > 0 {
		item.Status = ReleaseOrderPrecheckItemStatusBlocked
		item.Message = "参数映射不完整：" + strings.Join(missing, "；")
	}
	return item, nil
}

func (uc *ReleaseOrderManager) validateSingleTemplateParamMapping(
	ctx context.Context,
	param domain.ReleaseTemplateParam,
	liveParams map[string]pipelineparamdomain.JenkinsParamSnapshot,
	selectedValue string,
) []string {
	scopeLabel := strings.ToUpper(string(param.PipelineScope))
	paramLabel := executorParamNameOrKey(param.ExecutorParamName, firstNonEmpty(param.ParamName, param.ParamKey, param.ID))
	missing := make([]string, 0, 3)
	if strings.TrimSpace(param.ExecutorParamName) == "" {
		missing = append(missing, fmt.Sprintf("%s 参数 %s 缺少管线参数名称", scopeLabel, paramLabel))
	}
	if strings.TrimSpace(param.ParamKey) == "" {
		missing = append(missing, fmt.Sprintf("%s 参数 %s 未映射平台字段", scopeLabel, paramLabel))
	}
	if param.ValueSource != "" && !param.ValueSource.Valid() {
		missing = append(missing, fmt.Sprintf("%s 参数 %s 取值方式无效", scopeLabel, paramLabel))
	}
	switch param.ValueSource {
	case domain.TemplateParamValueSourceFixed:
		if strings.TrimSpace(param.FixedValue) == "" {
			missing = append(missing, fmt.Sprintf("%s 参数 %s 固定值为空", scopeLabel, paramLabel))
		}
	case domain.TemplateParamValueSourceCIParam, domain.TemplateParamValueSourceBuiltin:
		if strings.TrimSpace(param.SourceParamKey) == "" {
			missing = append(missing, fmt.Sprintf("%s 参数 %s 缺少来源字段", scopeLabel, paramLabel))
		}
	}
	var paramDef pipelineparamdomain.ExecutorParamDef
	hasParamDef := false
	if uc.paramRepo != nil && strings.TrimSpace(param.ExecutorParamDefID) != "" {
		var err error
		paramDef, err = uc.paramRepo.GetByID(ctx, strings.TrimSpace(param.ExecutorParamDefID))
		if err != nil {
			missing = append(missing, fmt.Sprintf("%s 参数 %s 对应的管线参数定义不存在", scopeLabel, paramLabel))
			return missing
		}
		hasParamDef = true
		if paramDef.Status != "" && paramDef.Status != pipelineparamdomain.StatusActive {
			missing = append(missing, fmt.Sprintf("%s 参数 %s 对应的管线参数定义已失效", scopeLabel, paramLabel))
		}
		if strings.TrimSpace(paramDef.ExecutorParamName) == "" {
			missing = append(missing, fmt.Sprintf("%s 参数 %s 当前管线参数名称为空", scopeLabel, paramLabel))
		}
		if strings.TrimSpace(paramDef.ParamKey) == "" {
			missing = append(missing, fmt.Sprintf("%s 参数 %s 当前未映射平台字段", scopeLabel, paramLabel))
		}
	}
	if liveParams != nil {
		paramName := strings.ToLower(strings.TrimSpace(param.ExecutorParamName))
		liveParam, ok := liveParams[paramName]
		if !ok && paramName != "" {
			missing = append(missing, fmt.Sprintf("%s 参数 %s 在 Jenkins 真实管线中不存在", scopeLabel, paramLabel))
		} else if ok && hasParamDef {
			missing = append(missing, compareJenkinsChoiceCandidates(scopeLabel, paramLabel, paramDef, liveParam, selectedValue)...)
		}
	}
	return missing
}

func (uc *ReleaseOrderManager) loadLiveJenkinsParamSnapshots(
	ctx context.Context,
	bindingByScope map[domain.PipelineScope]domain.ReleaseTemplateBinding,
	scopes map[domain.PipelineScope]bool,
) (map[domain.PipelineScope]map[string]pipelineparamdomain.JenkinsParamSnapshot, []string) {
	reader, ok := uc.jenkins.(JenkinsReleaseParamReader)
	if !ok || reader == nil || uc.pipelineRepo == nil {
		return nil, nil
	}
	result := make(map[domain.PipelineScope]map[string]pipelineparamdomain.JenkinsParamSnapshot)
	failures := make([]string, 0)
	for scope := range scopes {
		binding := bindingByScope[scope]
		if strings.ToLower(strings.TrimSpace(binding.Provider)) != string(pipelinedomain.ProviderJenkins) {
			continue
		}
		scopeLabel := strings.ToUpper(string(scope))
		pipelineID := strings.TrimSpace(binding.PipelineID)
		if pipelineID == "" {
			failures = append(failures, fmt.Sprintf("%s 管线未保存 Jenkins 管线 ID，无法校验真实参数", scopeLabel))
			continue
		}
		pipeline, err := uc.pipelineRepo.GetPipelineByID(ctx, pipelineID)
		if err != nil {
			failures = append(failures, fmt.Sprintf("%s 管线 %s 不存在，无法校验真实参数", scopeLabel, pipelineID))
			continue
		}
		fullName := strings.TrimSpace(pipeline.JobFullName)
		if fullName == "" {
			failures = append(failures, fmt.Sprintf("%s 管线 %s 未保存 Jenkins Job 名称，无法校验真实参数", scopeLabel, pipelineID))
			continue
		}
		jobSet, err := reader.GetJobParamSet(ctx, fullName)
		if err != nil {
			failures = append(failures, fmt.Sprintf("%s 管线 %s 读取 Jenkins 真实参数失败", scopeLabel, firstNonEmpty(pipeline.JobName, fullName)))
			continue
		}
		index := make(map[string]pipelineparamdomain.JenkinsParamSnapshot, len(jobSet.Params))
		for _, item := range jobSet.Params {
			name := strings.ToLower(strings.TrimSpace(item.Name))
			if name == "" {
				continue
			}
			index[name] = item
		}
		result[scope] = index
	}
	return result, failures
}

func compareJenkinsChoiceCandidates(
	scopeLabel string,
	paramLabel string,
	paramDef pipelineparamdomain.ExecutorParamDef,
	liveParam pipelineparamdomain.JenkinsParamSnapshot,
	selectedValue string,
) []string {
	localChoices := extractChoiceCandidates(paramDef.RawMeta)
	liveChoices := extractChoiceCandidates(liveParam.RawMeta)
	localIsChoice := paramDef.ParamType == pipelineparamdomain.ParamTypeChoice || len(localChoices) > 0
	liveIsChoice := liveParam.ParamType == pipelineparamdomain.ParamTypeChoice || len(liveChoices) > 0
	if !localIsChoice && !liveIsChoice {
		return nil
	}
	if isJenkinsGitParameterRawMeta(paramDef.RawMeta) || isJenkinsGitParameterRawMeta(liveParam.RawMeta) {
		return validateSelectedJenkinsChoiceCandidate(scopeLabel, paramLabel, selectedValue, liveChoices)
	}
	if sameStringSlice(localChoices, liveChoices) {
		return nil
	}
	return []string{
		fmt.Sprintf(
			"%s 参数 %s 候选值与 Jenkins 真实管线不一致，平台=%s，Jenkins=%s",
			scopeLabel,
			paramLabel,
			formatChoiceCandidates(localChoices),
			formatChoiceCandidates(liveChoices),
		),
	}
}

func validateSelectedJenkinsChoiceCandidate(
	scopeLabel string,
	paramLabel string,
	selectedValue string,
	liveChoices []string,
) []string {
	selectedValue = strings.TrimSpace(selectedValue)
	if selectedValue == "" || len(liveChoices) == 0 {
		return nil
	}
	for _, choice := range liveChoices {
		if selectedValue == strings.TrimSpace(choice) {
			return nil
		}
	}
	return []string{
		fmt.Sprintf(
			"%s 参数 %s 取值 %s 不在 Jenkins 真实候选值中，Jenkins=%s",
			scopeLabel,
			paramLabel,
			selectedValue,
			formatChoiceCandidates(liveChoices),
		),
	}
}

func isJenkinsGitParameterRawMeta(rawMeta string) bool {
	rawMeta = strings.TrimSpace(rawMeta)
	if rawMeta == "" {
		return false
	}
	var fields map[string]any
	if err := json.Unmarshal([]byte(rawMeta), &fields); err != nil {
		return false
	}
	className := strings.ToLower(strings.TrimSpace(fmt.Sprint(fields["_class"])))
	typeName := strings.ToLower(strings.TrimSpace(fmt.Sprint(fields["type"])))
	return strings.Contains(className, "gitparameterdefinition") || strings.Contains(typeName, "gitparameterdefinition")
}

func extractChoiceCandidates(rawMeta string) []string {
	rawMeta = strings.TrimSpace(rawMeta)
	if rawMeta == "" {
		return nil
	}
	var value any
	if err := json.Unmarshal([]byte(rawMeta), &value); err != nil {
		return nil
	}
	return normalizeChoiceCandidateValues(value)
}

func normalizeChoiceCandidateValues(value any) []string {
	switch typed := value.(type) {
	case []any:
		result := make([]string, 0, len(typed))
		for _, item := range typed {
			result = append(result, normalizeChoiceCandidateValues(item)...)
		}
		return compactChoiceCandidates(result)
	case map[string]any:
		for _, key := range []string{"choices", "choiceList", "values", "value", "items", "list"} {
			if nested, ok := typed[key]; ok {
				if result := normalizeChoiceCandidateValues(nested); len(result) > 0 {
					return result
				}
			}
		}
	case string:
		delimiter := ","
		if strings.Contains(typed, "\n") {
			delimiter = "\n"
		}
		parts := strings.Split(typed, delimiter)
		return compactChoiceCandidates(parts)
	case float64:
		return []string{strings.TrimSpace(fmt.Sprintf("%v", typed))}
	case bool:
		if typed {
			return []string{"true"}
		}
		return []string{"false"}
	}
	return nil
}

func compactChoiceCandidates(values []string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		result = append(result, trimmed)
	}
	return result
}

func sameStringSlice(left []string, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for idx := range left {
		if left[idx] != right[idx] {
			return false
		}
	}
	return true
}

func formatChoiceCandidates(values []string) string {
	if len(values) == 0 {
		return "[]"
	}
	return "[" + strings.Join(values, ",") + "]"
}

func indexReleaseOrderParamValues(params []domain.ReleaseOrderParam) map[string]string {
	result := make(map[string]string, len(params)*2)
	for _, param := range params {
		value := strings.TrimSpace(param.ParamValue)
		if name := strings.ToLower(strings.TrimSpace(param.ExecutorParamName)); name != "" {
			result[releaseOrderParamValueKey(param.PipelineScope, name)] = value
		}
		if key := strings.ToLower(strings.TrimSpace(param.ParamKey)); key != "" {
			result[releaseOrderParamValueKey(param.PipelineScope, key)] = value
		}
	}
	return result
}

func lookupReleaseOrderParamValue(
	values map[string]string,
	scope domain.PipelineScope,
	executorParamName string,
	paramKey string,
) string {
	if len(values) == 0 {
		return ""
	}
	if name := strings.ToLower(strings.TrimSpace(executorParamName)); name != "" {
		if value, ok := values[releaseOrderParamValueKey(scope, name)]; ok {
			return value
		}
	}
	if key := strings.ToLower(strings.TrimSpace(paramKey)); key != "" {
		if value, ok := values[releaseOrderParamValueKey(scope, key)]; ok {
			return value
		}
	}
	return ""
}

func releaseOrderParamValueKey(scope domain.PipelineScope, key string) string {
	return string(scope) + "\x00" + key
}

func precheckParamMappingScopes(
	executions []domain.ReleaseOrderExecution,
	action ReleaseOrderDispatchAction,
) map[domain.PipelineScope]bool {
	result := make(map[domain.PipelineScope]bool)
	for _, execution := range executions {
		if execution.Status != domain.ExecutionStatusPending {
			continue
		}
		switch action {
		case ReleaseOrderDispatchActionBuild:
			if execution.PipelineScope == domain.PipelineScopeCI {
				result[domain.PipelineScopeCI] = true
			}
		case ReleaseOrderDispatchActionDeploy:
			if execution.PipelineScope == domain.PipelineScopeCD {
				result[domain.PipelineScopeCD] = true
			}
		default:
			if execution.PipelineScope.Valid() {
				result[execution.PipelineScope] = true
			}
		}
	}
	return result
}

// resolveDispatchPrecheckItems 解析上下文数据，得到后续流程需要的结果。
func (uc *ReleaseOrderManager) resolveDispatchPrecheckItems(
	order domain.ReleaseOrder,
	executions []domain.ReleaseOrderExecution,
	action ReleaseOrderDispatchAction,
) (ReleaseOrderPrecheckItem, ReleaseOrderPrecheckItem, *domain.ReleaseOrderExecution) {
	statusItem := ReleaseOrderPrecheckItem{
		Key:     "order_status",
		Name:    "发布单状态",
		Status:  ReleaseOrderPrecheckItemStatusPass,
		Message: "发布单处于可执行状态",
	}
	executionItem := ReleaseOrderPrecheckItem{
		Key:     "execution_units",
		Name:    "执行单元",
		Status:  ReleaseOrderPrecheckItemStatusPass,
		Message: fmt.Sprintf("已配置 %d 个执行单元", len(executions)),
	}

	switch action {
	case ReleaseOrderDispatchActionBuild:
		hasCI := hasExecutionForScope(executions, domain.PipelineScopeCI)
		hasCD := hasExecutionForScope(executions, domain.PipelineScopeCD)
		switch {
		case order.OperationType != domain.OperationTypeDeploy:
			statusItem.Status = ReleaseOrderPrecheckItemStatusBlocked
			statusItem.Message = "仅普通发布单支持先构建后部署"
		case !hasCI || !hasCD:
			statusItem.Status = ReleaseOrderPrecheckItemStatusBlocked
			statusItem.Message = "当前发布单未同时配置 CI / CD 执行单元，无法分段构建"
		default:
			switch order.Status {
			case domain.OrderStatusPending:
				statusItem.Message = "发布单处于待执行状态，可进入构建阶段"
			case domain.OrderStatusApproved:
				statusItem.Message = "发布单已审批通过，可进入构建阶段"
			case domain.OrderStatusBuilding:
				statusItem.Status = ReleaseOrderPrecheckItemStatusBlocked
				statusItem.Message = "构建已发起，正在等待构建结果"
			case domain.OrderStatusBuiltWaitingDeploy:
				statusItem.Status = ReleaseOrderPrecheckItemStatusBlocked
				statusItem.Message = "构建已完成，请使用部署操作继续执行"
			case domain.OrderStatusPendingApproval:
				statusItem.Status = ReleaseOrderPrecheckItemStatusBlocked
				statusItem.Message = "发布单待审批，审批通过后才允许构建"
			case domain.OrderStatusApproving:
				statusItem.Status = ReleaseOrderPrecheckItemStatusBlocked
				statusItem.Message = "发布单审批中，审批完成后才允许构建"
			case domain.OrderStatusRejected:
				statusItem.Status = ReleaseOrderPrecheckItemStatusBlocked
				statusItem.Message = "发布单审批已拒绝，无法继续触发构建"
			case domain.OrderStatusQueued, domain.OrderStatusRunning, domain.OrderStatusDeploying:
				statusItem.Status = ReleaseOrderPrecheckItemStatusBlocked
				statusItem.Message = "发布单已进入执行中，无法重复触发构建"
			default:
				statusItem.Status = ReleaseOrderPrecheckItemStatusBlocked
				statusItem.Message = "当前发布单不是可构建状态，无法再次触发构建"
			}
		}
		target := findExecutionByScopeAndStatus(executions, domain.PipelineScopeCI, domain.ExecutionStatusPending)
		if target == nil {
			executionItem.Status = ReleaseOrderPrecheckItemStatusBlocked
			executionItem.Message = "未找到可执行的 CI 构建单元"
		} else {
			executionItem.Message = "已选定 CI 构建单元，将只触发构建阶段"
		}
		return statusItem, executionItem, target
	case ReleaseOrderDispatchActionDeploy:
		switch order.Status {
		case domain.OrderStatusBuiltWaitingDeploy:
			statusItem.Message = "构建已完成，可进入部署阶段"
		case domain.OrderStatusBuilding:
			statusItem.Status = ReleaseOrderPrecheckItemStatusBlocked
			statusItem.Message = "当前仍在构建中，构建完成后才允许部署"
		case domain.OrderStatusPending, domain.OrderStatusApproved:
			statusItem.Status = ReleaseOrderPrecheckItemStatusBlocked
			statusItem.Message = "请先完成构建，再执行部署"
		case domain.OrderStatusPendingApproval:
			statusItem.Status = ReleaseOrderPrecheckItemStatusBlocked
			statusItem.Message = "发布单待审批，审批通过并完成构建后才允许部署"
		case domain.OrderStatusApproving:
			statusItem.Status = ReleaseOrderPrecheckItemStatusBlocked
			statusItem.Message = "发布单审批中，审批完成并构建成功后才允许部署"
		case domain.OrderStatusRejected:
			statusItem.Status = ReleaseOrderPrecheckItemStatusBlocked
			statusItem.Message = "发布单审批已拒绝，无法继续部署"
		case domain.OrderStatusQueued, domain.OrderStatusRunning, domain.OrderStatusDeploying:
			statusItem.Status = ReleaseOrderPrecheckItemStatusBlocked
			statusItem.Message = "部署已发起，无法重复触发"
		default:
			statusItem.Status = ReleaseOrderPrecheckItemStatusBlocked
			statusItem.Message = "当前发布单不是可部署状态，无法再次触发部署"
		}
		target := findExecutionByScopeAndStatus(executions, domain.PipelineScopeCD, domain.ExecutionStatusPending)
		if target == nil {
			executionItem.Status = ReleaseOrderPrecheckItemStatusBlocked
			executionItem.Message = "未找到可执行的 CD 部署单元"
		} else {
			executionItem.Message = "已选定 CD 部署单元，将只触发部署阶段"
		}
		return statusItem, executionItem, target
	default:
		switch order.Status {
		case domain.OrderStatusPending:
			statusItem.Message = "发布单处于待执行状态"
		case domain.OrderStatusApproved:
			statusItem.Message = "发布单已审批通过，可进入执行阶段"
		case domain.OrderStatusQueued:
			statusItem.Status = ReleaseOrderPrecheckItemStatusWarn
			statusItem.Message = "发布单已进入等待队列"
		case domain.OrderStatusRunning:
			statusItem.Message = "发布单已进入调度中"
		case domain.OrderStatusPendingApproval:
			statusItem.Status = ReleaseOrderPrecheckItemStatusBlocked
			statusItem.Message = "发布单待审批，审批通过后才允许触发"
		case domain.OrderStatusApproving:
			statusItem.Status = ReleaseOrderPrecheckItemStatusBlocked
			statusItem.Message = "发布单审批中，审批完成后才允许触发"
		case domain.OrderStatusRejected:
			statusItem.Status = ReleaseOrderPrecheckItemStatusBlocked
			statusItem.Message = "发布单审批已拒绝，无法继续触发"
		case domain.OrderStatusBuilding:
			statusItem.Status = ReleaseOrderPrecheckItemStatusBlocked
			statusItem.Message = "当前发布单正在构建中，无法再次触发完整发布"
		case domain.OrderStatusBuiltWaitingDeploy:
			statusItem.Status = ReleaseOrderPrecheckItemStatusBlocked
			statusItem.Message = "当前发布单已完成构建，请改用部署操作继续执行"
		case domain.OrderStatusDeploying:
			statusItem.Status = ReleaseOrderPrecheckItemStatusBlocked
			statusItem.Message = "发布单已进入发布中，无法再次触发"
		default:
			statusItem.Status = ReleaseOrderPrecheckItemStatusBlocked
			statusItem.Message = "当前发布单不是可执行状态，无法再次触发"
		}
		target := findExecutionByStatus(executions, domain.ExecutionStatusPending)
		if len(executions) == 0 || target == nil {
			executionItem.Status = ReleaseOrderPrecheckItemStatusBlocked
			executionItem.Message = "未找到可执行的待执行单元"
		}
		return statusItem, executionItem, target
	}
}

// buildExecutionReferencePrecheckItem 组装业务执行所需的输入数据。
func (uc *ReleaseOrderManager) buildExecutionReferencePrecheckItem(
	ctx context.Context,
	execution domain.ReleaseOrderExecution,
) (ReleaseOrderPrecheckItem, bool, error) {
	if uc == nil || uc.pipelineRepo == nil || strings.TrimSpace(execution.BindingID) == "" {
		return ReleaseOrderPrecheckItem{}, false, nil
	}
	item := ReleaseOrderPrecheckItem{
		Key:     "execution_reference",
		Name:    "模板绑定",
		Status:  ReleaseOrderPrecheckItemStatusPass,
		Message: "模板绑定引用正常",
	}

	binding, err := uc.pipelineRepo.GetBindingByID(ctx, execution.BindingID)
	if err == nil {
		if binding.Status == pipelinedomain.StatusInactive {
			if strings.TrimSpace(execution.PipelineID) != "" {
				item.Status = ReleaseOrderPrecheckItemStatusWarn
				item.Message = fmt.Sprintf("模板引用的绑定 %s 已失效，将回退到快照管线 %s 继续执行，建议尽快更新模板绑定", firstNonEmpty(binding.Name, execution.BindingName, execution.BindingID), execution.PipelineID)
			} else {
				item.Status = ReleaseOrderPrecheckItemStatusBlocked
				item.Message = fmt.Sprintf("模板引用的绑定 %s 已失效，且未保存可回退的管线 ID，请先更新模板绑定", firstNonEmpty(binding.Name, execution.BindingName, execution.BindingID))
			}
		}
		return item, true, nil
	}
	if !errors.Is(err, pipelinedomain.ErrBindingNotFound) {
		return ReleaseOrderPrecheckItem{}, false, err
	}
	if strings.TrimSpace(execution.PipelineID) != "" {
		pipeline, pipelineErr := uc.pipelineRepo.GetPipelineByID(ctx, execution.PipelineID)
		if pipelineErr == nil {
			if activeErr := ensureActivePipelineRecord(pipeline, "快照管线"); activeErr == nil {
				item.Status = ReleaseOrderPrecheckItemStatusWarn
				item.Message = fmt.Sprintf("模板引用的绑定 %s 已失效，将回退到快照管线 %s 继续执行，建议尽快更新模板绑定", firstNonEmpty(execution.BindingName, execution.BindingID), execution.PipelineID)
				return item, true, nil
			}
			item.Status = ReleaseOrderPrecheckItemStatusBlocked
			item.Message = fmt.Sprintf("模板引用的绑定 %s 已失效，且快照管线 %s 不可用，请先更新模板绑定", firstNonEmpty(execution.BindingName, execution.BindingID), execution.PipelineID)
			return item, true, nil
		}
		if !errors.Is(pipelineErr, pipelinedomain.ErrPipelineNotFound) {
			return ReleaseOrderPrecheckItem{}, false, pipelineErr
		}
	}
	item.Status = ReleaseOrderPrecheckItemStatusBlocked
	item.Message = fmt.Sprintf("模板引用的绑定 %s 已失效，且未找到可回退的快照管线，请先更新模板绑定", firstNonEmpty(execution.BindingName, execution.BindingID))
	return item, true, nil
}

// evaluateDispatchGuard 封装当前模块的业务处理逻辑。
func (uc *ReleaseOrderManager) evaluateDispatchGuard(
	ctx context.Context,
	order domain.ReleaseOrder,
	execution domain.ReleaseOrderExecution,
	params []domain.ReleaseOrderParam,
) (releaseDispatchGuard, error) {
	settings, err := uc.loadReleaseConcurrencySettings(ctx)
	if err != nil {
		return releaseDispatchGuard{}, err
	}
	guard := releaseDispatchGuard{Settings: settings}

	// An active execution lock is the authoritative queue owner. Check it before
	// comparing queued orders so a follower cannot make the lock owner wait for
	// itself after the tracker refreshes the order state.
	if settings.Enabled {
		lockScope, lockKey, lockErr := uc.buildExecutionLockIdentity(ctx, order, execution, params, settings)
		if lockErr != nil {
			return releaseDispatchGuard{}, lockErr
		}
		guard.LockScope = lockScope
		guard.LockKey = lockKey

		lock, findLockErr := uc.repo.FindActiveExecutionLock(ctx, lockKey, "", uc.now())
		if findLockErr != nil && !errors.Is(findLockErr, domain.ErrExecutionLockNotFound) {
			return releaseDispatchGuard{}, findLockErr
		}
		if findLockErr == nil {
			if strings.TrimSpace(lock.ReleaseOrderID) == strings.TrimSpace(order.ID) {
				return guard, nil
			}
			guard.ConflictLock = &lock
			if uc.shouldQueueInConcurrentBatch(ctx, order, lock) {
				guard.WaitingForLock = true
				guard.Message = fmt.Sprintf("当前批次的同应用同环境发布单 %s 正在执行，已进入顺序等待队列", firstNonEmpty(lock.ReleaseOrderNo, lock.ReleaseOrderID))
				return guard, nil
			}
			if settings.ConflictStrategy == ReleaseConcurrencyConflictStrategyQueue {
				guard.WaitingForLock = true
				guard.Message = fmt.Sprintf("当前目标已被发布单 %s 占用，已进入排队等待", firstNonEmpty(lock.ReleaseOrderNo, lock.ReleaseOrderID))
				return guard, nil
			}
			guard.Message = fmt.Sprintf("当前目标已被发布单 %s 占用，请稍后再试", firstNonEmpty(lock.ReleaseOrderNo, lock.ReleaseOrderID))
			return guard, nil
		}
	}

	conflictOrder, err := uc.repo.FindActiveOrderByApplicationEnv(ctx, order.ApplicationID, order.EnvCode, order.ID)
	if err != nil && !errors.Is(err, domain.ErrOrderNotFound) {
		return releaseDispatchGuard{}, err
	}
	if err == nil {
		guard.ConflictOrder = &conflictOrder
		aheadCount, countErr := uc.repo.CountActiveOrdersByApplicationEnv(ctx, order.ApplicationID, order.EnvCode, order.ID)
		if countErr != nil {
			return releaseDispatchGuard{}, countErr
		}
		if aheadCount <= 0 {
			aheadCount = 1
		}
		guard.AheadCount = aheadCount
		switch {
		case shouldQueueBehindConcurrentOrder(order, conflictOrder):
			guard.WaitingForLock = true
			guard.Message = fmt.Sprintf("当前批次的同应用同环境发布单 %s 正在执行，已进入顺序等待队列", firstNonEmpty(conflictOrder.OrderNo, conflictOrder.ID))
		case settings.ConflictStrategy == ReleaseConcurrencyConflictStrategyQueue:
			guard.WaitingForLock = true
			guard.Message = fmt.Sprintf("当前应用在环境 %s 前面还有 %d 单，已进入排队等待", firstNonEmpty(strings.TrimSpace(order.EnvCode), "-"), aheadCount)
		default:
			guard.Message = fmt.Sprintf("当前应用在环境 %s 前面还有 %d 单，请等待先前执行单结束后再点击发布", firstNonEmpty(strings.TrimSpace(order.EnvCode), "-"), aheadCount)
		}
		return guard, nil
	}

	if !settings.Enabled {
		return guard, nil
	}
	return guard, nil
}

// ensureExecutionLock 校验前置条件，不满足时写入对应错误响应。
func (uc *ReleaseOrderManager) ensureExecutionLock(
	ctx context.Context,
	order domain.ReleaseOrder,
	execution domain.ReleaseOrderExecution,
	params []domain.ReleaseOrderParam,
) (releaseDispatchGuard, bool, error) {
	guard, err := uc.evaluateDispatchGuard(ctx, order, execution, params)
	if err != nil {
		return releaseDispatchGuard{}, false, err
	}
	if guard.ConflictOrder != nil {
		if guard.WaitingForLock {
			return guard, false, nil
		}
		return guard, false, fmt.Errorf("%w: %s", ErrConcurrentReleaseBlocked, guard.Message)
	}
	if !guard.Settings.Enabled {
		return guard, true, nil
	}
	if guard.ConflictLock != nil {
		if guard.WaitingForLock {
			return guard, false, nil
		}
		if guard.Settings.ConflictStrategy == ReleaseConcurrencyConflictStrategyQueue {
			return guard, false, nil
		}
		return guard, false, fmt.Errorf("%w: %s", ErrConcurrentReleaseBlocked, guard.Message)
	}
	lock := domain.ReleaseExecutionLock{
		ID:             generateID("rlk"),
		LockScope:      guard.LockScope,
		LockKey:        guard.LockKey,
		ApplicationID:  order.ApplicationID,
		EnvCode:        order.EnvCode,
		ReleaseOrderID: order.ID,
		ReleaseOrderNo: order.OrderNo,
		Status:         domain.ExecutionLockStatusActive,
		OwnerType:      "release_order",
		CreatedAt:      uc.now(),
	}
	expiredAt := uc.now().Add(time.Duration(guard.Settings.LockTimeoutSec) * time.Second)
	lock.ExpiredAt = &expiredAt
	resolvedLock, acquired, err := uc.repo.AcquireExecutionLock(ctx, lock, uc.now())
	if err != nil {
		return releaseDispatchGuard{}, false, err
	}
	if !acquired {
		guard.ConflictLock = &resolvedLock
		if uc.shouldQueueInConcurrentBatch(ctx, order, resolvedLock) {
			guard.WaitingForLock = true
			guard.Message = fmt.Sprintf("当前批次的同应用同环境发布单 %s 正在执行，已进入顺序等待队列", firstNonEmpty(resolvedLock.ReleaseOrderNo, resolvedLock.ReleaseOrderID))
			return guard, false, nil
		}
		if guard.Settings.ConflictStrategy == ReleaseConcurrencyConflictStrategyQueue {
			guard.WaitingForLock = true
			guard.Message = fmt.Sprintf("当前目标已被发布单 %s 占用，已进入排队等待", firstNonEmpty(resolvedLock.ReleaseOrderNo, resolvedLock.ReleaseOrderID))
			return guard, false, nil
		}
		guard.Message = fmt.Sprintf("当前目标已被发布单 %s 占用，请稍后再试", firstNonEmpty(resolvedLock.ReleaseOrderNo, resolvedLock.ReleaseOrderID))
		return guard, false, fmt.Errorf("%w: %s", ErrConcurrentReleaseBlocked, guard.Message)
	}
	return guard, true, nil
}

// touchExecutionLocks 将领域对象转换为接口响应结构。
func (uc *ReleaseOrderManager) touchExecutionLocks(ctx context.Context, order domain.ReleaseOrder) error {
	settings, err := uc.loadReleaseConcurrencySettings(ctx)
	if err != nil || !settings.Enabled {
		return err
	}
	expires := uc.now().Add(time.Duration(settings.LockTimeoutSec) * time.Second)
	return uc.repo.TouchExecutionLocksByOrderID(ctx, order.ID, expires)
}

// releaseExecutionLocks 封装当前模块的业务处理逻辑。
func (uc *ReleaseOrderManager) releaseExecutionLocks(ctx context.Context, orderID string, status domain.ExecutionLockStatus) error {
	if uc == nil || uc.repo == nil || strings.TrimSpace(orderID) == "" {
		return nil
	}
	return uc.repo.ReleaseExecutionLocksByOrderID(ctx, orderID, status, uc.now())
}

// loadReleaseConcurrencySettings 封装当前模块的业务处理逻辑。
func (uc *ReleaseOrderManager) loadReleaseConcurrencySettings(ctx context.Context) (ReleaseConcurrencySettingsOutput, error) {
	if uc == nil || uc.releaseSettings == nil {
		return normalizeConcurrencySettings(ReleaseConcurrencySettingsOutput{}), nil
	}
	settings, err := uc.releaseSettings.LoadConcurrencySettings(ctx)
	if err != nil {
		return ReleaseConcurrencySettingsOutput{}, err
	}
	return normalizeConcurrencySettings(settings), nil
}

// buildExecutionLockIdentity 组装业务执行所需的输入数据。
func (uc *ReleaseOrderManager) buildExecutionLockIdentity(
	ctx context.Context,
	order domain.ReleaseOrder,
	execution domain.ReleaseOrderExecution,
	params []domain.ReleaseOrderParam,
	settings ReleaseConcurrencySettingsOutput,
) (domain.ExecutionLockScope, string, error) {
	scope := domain.ExecutionLockScope(settings.LockScope)
	switch settings.LockScope {
	case ReleaseConcurrencyLockScopeApplication:
		return scope, fmt.Sprintf("app:%s", strings.TrimSpace(order.ApplicationID)), nil
	case ReleaseConcurrencyLockScopeGitOpsRepoBranch:
		if isArgoCDExecution(execution) {
			if key, err := uc.buildGitOpsRepoBranchLockKey(ctx, order, execution, params); err == nil && strings.TrimSpace(key) != "" {
				return scope, key, nil
			}
		}
		fallthrough
	case ReleaseConcurrencyLockScopeApplicationEnv:
		return domain.ExecutionLockScopeApplicationEnv, fmt.Sprintf("app:%s:env:%s", strings.TrimSpace(order.ApplicationID), strings.TrimSpace(order.EnvCode)), nil
	default:
		return domain.ExecutionLockScopeApplicationEnv, fmt.Sprintf("app:%s:env:%s", strings.TrimSpace(order.ApplicationID), strings.TrimSpace(order.EnvCode)), nil
	}
}

// buildGitOpsRepoBranchLockKey 组装业务执行所需的输入数据。
func (uc *ReleaseOrderManager) buildGitOpsRepoBranchLockKey(
	ctx context.Context,
	order domain.ReleaseOrder,
	execution domain.ReleaseOrderExecution,
	params []domain.ReleaseOrderParam,
) (string, error) {
	snapshot, err := uc.repo.GetDeploySnapshotByOrderID(ctx, order.ID)
	if err == nil && strings.TrimSpace(snapshot.RepoURL) != "" {
		branch := uc.resolveGitOpsBranchByApplication(
			ctx,
			order.ApplicationID,
			firstNonEmpty(strings.TrimSpace(snapshot.EnvCode), strings.TrimSpace(order.EnvCode)),
			argocddomain.Instance{},
			strings.TrimSpace(snapshot.Branch),
		)
		return fmt.Sprintf("repo:%s:branch:%s", strings.TrimSpace(snapshot.RepoURL), branch), nil
	}
	if err != nil && !errors.Is(err, domain.ErrDeploySnapshotNotFound) {
		return "", err
	}
	template, _, _, _, _, err := uc.repo.GetTemplateByID(ctx, strings.TrimSpace(order.TemplateID))
	if err != nil {
		return "", err
	}
	binding, argocdInstance, client, err := uc.resolveArgoCDExecutionContext(ctx, order, execution, params)
	if err != nil {
		return "", err
	}
	environment := uc.resolveArgoCDEnvironment(order, params)
	appName, app, err := resolveArgoCDApplicationByRef(ctx, client, binding.ExternalRef, environment, normalizeTemplateGitOpsType(template.GitOpsType, true))
	_ = appName
	if err != nil {
		return "", err
	}
	repoURL := strings.TrimSpace(app.GetRepoURL())
	branch := uc.resolveGitOpsTargetBranch(ctx, order, params, argocdInstance, app)
	if repoURL == "" {
		return "", fmt.Errorf("argocd application repo url is empty")
	}
	return fmt.Sprintf("repo:%s:branch:%s", repoURL, branch), nil
}
