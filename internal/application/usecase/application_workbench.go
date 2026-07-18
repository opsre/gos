package usecase

import (
	"context"
	"strings"

	appdomain "gos/internal/domain/application"
	releasedomain "gos/internal/domain/release"
)

// ApplicationWorkbench 将应用工作台所需的只读数据收敛为一次服务端查询。
// 它刻意直接使用仓储读取，避免工作台刷新触发发布单的状态协调或外部调用。
type ApplicationWorkbench struct {
	appRepo     appdomain.Repository
	releaseRepo releasedomain.Repository
}

type ApplicationWorkbenchInput struct {
	ApplicationFilter       appdomain.ListFilter
	OverviewApplicationIDs  []string
	ReleaseApplicationIDs   []string
	ReleaseApplicationScope []releasedomain.ApplicationEnvScope
	TemplateApplicationIDs  []string
	IncludeReleaseData      bool
	IncludeTemplateData     bool
}

type ApplicationWorkbenchOutput struct {
	Applications           []appdomain.Application
	Page                   int
	PageSize               int
	Total                  int64
	OverviewApplicationIDs []string
	Templates              []releasedomain.ReleaseTemplate
	RecentReleaseOrders    []releasedomain.ReleaseOrder
	ReleaseStateSummaries  []releasedomain.AppReleaseStateSummary
	OverviewReleaseOrders  []releasedomain.ReleaseOrder
}

func NewApplicationWorkbench(appRepo appdomain.Repository, releaseRepo releasedomain.Repository) *ApplicationWorkbench {
	return &ApplicationWorkbench{appRepo: appRepo, releaseRepo: releaseRepo}
}

// Load 返回工作台首屏及轮询所需快照。查询数量与应用数量无关：应用分页、模板分页
// 和发布单读取均在服务端批量完成，客户端不再发起 N+1 请求。
func (uc *ApplicationWorkbench) Load(ctx context.Context, input ApplicationWorkbenchInput) (ApplicationWorkbenchOutput, error) {
	filter := input.ApplicationFilter
	filter.Keyword = strings.TrimSpace(filter.Keyword)
	filter.Key = strings.TrimSpace(filter.Key)
	filter.Name = strings.TrimSpace(filter.Name)
	filter.ProjectID = strings.TrimSpace(filter.ProjectID)
	if filter.Status != "" && !filter.Status.Valid() {
		return ApplicationWorkbenchOutput{}, ErrInvalidStatus
	}
	if filter.Page <= 0 {
		filter.Page = 1
	}
	if filter.PageSize <= 0 {
		filter.PageSize = 20
	}
	if filter.PageSize > 100 {
		filter.PageSize = 100
	}

	apps, total, err := uc.appRepo.List(ctx, filter)
	if err != nil {
		return ApplicationWorkbenchOutput{}, err
	}
	output := ApplicationWorkbenchOutput{
		Applications: apps,
		Page:         filter.Page,
		PageSize:     filter.PageSize,
		Total:        total,
	}

	overviewIDs, err := uc.listApplicationIDs(ctx, input.OverviewApplicationIDs)
	if err != nil {
		return ApplicationWorkbenchOutput{}, err
	}
	output.OverviewApplicationIDs = overviewIDs

	if input.IncludeTemplateData {
		templates, templateErr := uc.listActiveTemplates(ctx, input.TemplateApplicationIDs)
		if templateErr != nil {
			return ApplicationWorkbenchOutput{}, templateErr
		}
		output.Templates = templates
	}
	if !input.IncludeReleaseData {
		return output, nil
	}

	pageApplicationIDs := applicationIDs(apps)
	if len(pageApplicationIDs) > 0 {
		recentApplicationIDs, recentScopes := releaseFilterForApplications(
			pageApplicationIDs,
			input.ReleaseApplicationIDs,
			input.ReleaseApplicationScope,
		)
		if len(recentApplicationIDs) > 0 || len(recentScopes) > 0 {
			recent, _, recentErr := uc.releaseRepo.List(ctx, releasedomain.ListFilter{
				ApplicationIDs:              recentApplicationIDs,
				VisibleApplicationEnvScopes: recentScopes,
				Page:                        1,
				PageSize:                    maxWorkbenchRecentPageSize(len(pageApplicationIDs)),
			})
			if recentErr != nil {
				return ApplicationWorkbenchOutput{}, recentErr
			}
			output.RecentReleaseOrders = keepRecentOrdersPerApplication(recent, pageApplicationIDs, 12)

			states, stateErr := uc.releaseRepo.ListCurrentAppReleaseStateSummaries(ctx, pageApplicationIDs)
			if stateErr != nil {
				return ApplicationWorkbenchOutput{}, stateErr
			}
			output.ReleaseStateSummaries = filterVisibleStateSummaries(states, input.ReleaseApplicationIDs, input.ReleaseApplicationScope)
		}
	}

	if len(overviewIDs) == 0 {
		return output, nil
	}
	overviewFilterIDs, overviewScopes := releaseFilterForApplications(
		overviewIDs,
		input.ReleaseApplicationIDs,
		input.ReleaseApplicationScope,
	)
	if len(overviewFilterIDs) == 0 && len(overviewScopes) == 0 {
		return output, nil
	}
	overviewOrders, _, overviewErr := uc.releaseRepo.List(ctx, releasedomain.ListFilter{
		ApplicationIDs:              overviewFilterIDs,
		VisibleApplicationEnvScopes: overviewScopes,
		Page:                        1,
		PageSize:                    5000,
	})
	if overviewErr != nil {
		return ApplicationWorkbenchOutput{}, overviewErr
	}
	output.OverviewReleaseOrders = overviewOrders
	return output, nil
}

func (uc *ApplicationWorkbench) listApplicationIDs(ctx context.Context, visibleIDs []string) ([]string, error) {
	const pageSize = 100
	result := make([]string, 0)
	for page := 1; ; page++ {
		apps, total, err := uc.appRepo.List(ctx, appdomain.ListFilter{
			ApplicationIDs: visibleIDs,
			Page:           page,
			PageSize:       pageSize,
		})
		if err != nil {
			return nil, err
		}
		result = append(result, applicationIDs(apps)...)
		if len(result) >= int(total) || len(apps) < pageSize {
			return result, nil
		}
	}
}

func (uc *ApplicationWorkbench) listActiveTemplates(ctx context.Context, applicationIDs []string) ([]releasedomain.ReleaseTemplate, error) {
	const pageSize = 500
	result := make([]releasedomain.ReleaseTemplate, 0)
	for page := 1; ; page++ {
		items, total, err := uc.releaseRepo.ListTemplates(ctx, releasedomain.TemplateListFilter{
			ApplicationIDs: applicationIDs,
			Status:         releasedomain.TemplateStatusActive,
			Page:           page,
			PageSize:       pageSize,
		})
		if err != nil {
			return nil, err
		}
		result = append(result, items...)
		if len(result) >= int(total) || len(items) < pageSize {
			return result, nil
		}
	}
}

func applicationIDs(apps []appdomain.Application) []string {
	ids := make([]string, 0, len(apps))
	for _, app := range apps {
		if id := strings.TrimSpace(app.ID); id != "" {
			ids = append(ids, id)
		}
	}
	return ids
}

func intersectIDs(required []string, visible []string) []string {
	if len(visible) == 0 {
		return required
	}
	allowed := make(map[string]struct{}, len(visible))
	for _, id := range visible {
		if value := strings.TrimSpace(id); value != "" {
			allowed[value] = struct{}{}
		}
	}
	result := make([]string, 0, len(required))
	for _, id := range required {
		if _, ok := allowed[strings.TrimSpace(id)]; ok {
			result = append(result, id)
		}
	}
	return result
}

func releaseFilterForApplications(
	applicationIDs []string,
	visibleApplicationIDs []string,
	scopes []releasedomain.ApplicationEnvScope,
) ([]string, []releasedomain.ApplicationEnvScope) {
	filteredScopes := make([]releasedomain.ApplicationEnvScope, 0, len(scopes))
	targets := make(map[string]struct{}, len(applicationIDs))
	for _, applicationID := range applicationIDs {
		if value := strings.TrimSpace(applicationID); value != "" {
			targets[value] = struct{}{}
		}
	}
	for _, scope := range scopes {
		if _, ok := targets[strings.TrimSpace(scope.ApplicationID)]; ok {
			filteredScopes = append(filteredScopes, scope)
		}
	}
	return intersectIDs(applicationIDs, visibleApplicationIDs), filteredScopes
}

func maxWorkbenchRecentPageSize(applicationCount int) int {
	value := applicationCount * 36
	if value < 120 {
		return 120
	}
	if value > 2000 {
		return 2000
	}
	return value
}

func keepRecentOrdersPerApplication(items []releasedomain.ReleaseOrder, applicationIDs []string, limit int) []releasedomain.ReleaseOrder {
	allowed := make(map[string]struct{}, len(applicationIDs))
	for _, id := range applicationIDs {
		allowed[strings.TrimSpace(id)] = struct{}{}
	}
	counts := make(map[string]int, len(applicationIDs))
	result := make([]releasedomain.ReleaseOrder, 0, len(items))
	for _, item := range items {
		applicationID := strings.TrimSpace(item.ApplicationID)
		if _, ok := allowed[applicationID]; !ok || counts[applicationID] >= limit {
			continue
		}
		counts[applicationID]++
		result = append(result, item)
	}
	return result
}

func filterVisibleStateSummaries(items []releasedomain.AppReleaseStateSummary, applicationIDs []string, scopes []releasedomain.ApplicationEnvScope) []releasedomain.AppReleaseStateSummary {
	if len(applicationIDs) == 0 && len(scopes) == 0 {
		return items
	}
	allowedApplications := make(map[string]struct{}, len(applicationIDs))
	for _, id := range applicationIDs {
		if value := strings.TrimSpace(id); value != "" {
			allowedApplications[value] = struct{}{}
		}
	}
	allowedScopes := make(map[string]struct{}, len(scopes))
	for _, scope := range scopes {
		key := strings.TrimSpace(scope.ApplicationID) + "::" + strings.TrimSpace(scope.EnvCode)
		allowedScopes[key] = struct{}{}
	}
	result := make([]releasedomain.AppReleaseStateSummary, 0, len(items))
	for _, item := range items {
		applicationID := strings.TrimSpace(item.ApplicationID)
		if _, ok := allowedApplications[applicationID]; ok {
			result = append(result, item)
			continue
		}
		if _, ok := allowedScopes[applicationID+"::"+strings.TrimSpace(item.EnvCode)]; ok {
			result = append(result, item)
		}
	}
	return result
}
