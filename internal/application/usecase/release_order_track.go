package usecase

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"path"
	"regexp"
	"strings"
	"time"

	pipelinedomain "gos/internal/domain/pipeline"
	domain "gos/internal/domain/release"
	"gos/internal/support/logx"
)

var queueURLPattern = regexp.MustCompile(`queue:\s*([^\s]+)`)
var buildURLPattern = regexp.MustCompile(`build:\s*([^\s]+)`)
var rawURLPattern = regexp.MustCompile(`https?://[^\s]+`)
var gosArtifactURLPattern = regexp.MustCompile(`(?m)\bGOS_ARTIFACT_URL\s*=\s*("[^"\r\n]+"|'[^'\r\n]+'|[^\s\r\n]+)`)

const maxArtifactLogScanBytes int64 = 8 << 20

type JenkinsReleaseStatusClient interface {
	GetQueueItem(ctx context.Context, queueURL string) (executableURL string, cancelled bool, why string, err error)
	GetBuildStatus(ctx context.Context, buildURL string) (building bool, result string, err error)
}

type JenkinsReleaseConsoleTextClient interface {
	GetBuildConsoleText(ctx context.Context, buildURL string, start int64) (content string, nextStart int64, moreData bool, err error)
}

type TrackReleaseExecutionOutput struct {
	RunningOrders int `json:"running_orders"`
	UpdatedOrders int `json:"updated_orders"`
	SkippedOrders int `json:"skipped_orders"`
	FailedOrders  int `json:"failed_orders"`
}

type TrackReleaseExecution struct {
	manager *ReleaseOrderManager
	jenkins JenkinsReleaseStatusClient
	now     func() time.Time
}

// NewTrackReleaseExecution 创建并返回对应组件实例。
func NewTrackReleaseExecution(
	manager *ReleaseOrderManager,
	jenkins JenkinsReleaseStatusClient,
) *TrackReleaseExecution {
	return &TrackReleaseExecution{
		manager: manager,
		jenkins: jenkins,
		now: func() time.Time {
			return time.Now().UTC()
		},
	}
}

// Execute 封装当前模块的业务处理逻辑。
func (uc *TrackReleaseExecution) Execute(ctx context.Context) (TrackReleaseExecutionOutput, error) {
	if uc == nil || uc.manager == nil {
		return TrackReleaseExecutionOutput{}, nil
	}
	if uc.jenkins == nil && (uc.manager.argocdRepo == nil || uc.manager.argocdFactory == nil) {
		return TrackReleaseExecutionOutput{}, nil
	}

	orders, err := uc.listRunningOrders(ctx)
	if err != nil {
		logx.Error("release_tracker", "list_trackable_orders_failed", err)
		return TrackReleaseExecutionOutput{}, err
	}
	logx.Info("release_tracker", "tick_start", logx.F("running_orders", len(orders)))

	output := TrackReleaseExecutionOutput{
		RunningOrders: len(orders),
	}

	for _, order := range orders {
		logx.Info("release_tracker", "sync_order_start",
			logx.F("order_id", order.ID),
			logx.F("order_no", order.OrderNo),
			logx.F("status", order.Status),
		)
		updated, skipped, runErr := uc.syncOrder(ctx, order)
		if runErr != nil {
			logx.Error("release_tracker", "sync_order_failed", runErr,
				logx.F("order_id", order.ID),
				logx.F("order_no", order.OrderNo),
			)
			output.FailedOrders++
			continue
		}
		if skipped {
			logx.Info("release_tracker", "sync_order_skipped",
				logx.F("order_id", order.ID),
				logx.F("order_no", order.OrderNo),
			)
			output.SkippedOrders++
			continue
		}
		if updated {
			logx.Info("release_tracker", "sync_order_updated",
				logx.F("order_id", order.ID),
				logx.F("order_no", order.OrderNo),
			)
			output.UpdatedOrders++
		}
	}
	logx.Info("release_tracker", "tick_finish",
		logx.F("running_orders", output.RunningOrders),
		logx.F("updated_orders", output.UpdatedOrders),
		logx.F("skipped_orders", output.SkippedOrders),
		logx.F("failed_orders", output.FailedOrders),
	)

	return output, nil
}

// listRunningOrders 查询并返回列表数据。
func (uc *TrackReleaseExecution) listRunningOrders(ctx context.Context) ([]domain.ReleaseOrder, error) {
	const pageSize = 100

	result := make([]domain.ReleaseOrder, 0)
	page := 1
	for {
		items, total, err := uc.manager.repo.ListTrackableOrders(ctx, page, pageSize)
		if err != nil {
			return nil, err
		}
		if len(items) == 0 {
			break
		}
		result = append(result, items...)
		if int64(page*pageSize) >= total {
			break
		}
		page++
	}
	return result, nil
}

// syncOrder 同步外部或内部状态数据。
func (uc *TrackReleaseExecution) syncOrder(ctx context.Context, order domain.ReleaseOrder) (bool, bool, error) {
	if order.Status == domain.OrderStatusCancelled {
		return uc.syncCancelledOrder(ctx, order)
	}
	if order.Status == domain.OrderStatusFailed {
		return uc.syncFailedOrder(ctx, order)
	}
	if err := uc.manager.touchExecutionLocks(ctx, order); err != nil {
		return false, false, err
	}
	executions, err := uc.manager.ListExecutions(ctx, order.ID)
	if err != nil {
		return false, false, err
	}
	if len(executions) == 0 {
		logx.Warn("release_tracker", "sync_order_without_executions",
			logx.F("order_id", order.ID),
			logx.F("order_no", order.OrderNo),
		)
		return false, true, nil
	}

	runningExecution := findExecutionByStatus(executions, domain.ExecutionStatusRunning)
	if runningExecution == nil {
		if order.Status == domain.OrderStatusBuilding &&
			findExecutionByScopeAndStatus(executions, domain.PipelineScopeCI, domain.ExecutionStatusSuccess) != nil &&
			findExecutionByScopeAndStatus(executions, domain.PipelineScopeCI, domain.ExecutionStatusPending) == nil &&
			findExecutionByScopeAndStatus(executions, domain.PipelineScopeCD, domain.ExecutionStatusPending) != nil {
			logx.Info("release_tracker", "building_order_waiting_deploy_transition",
				logx.F("order_id", order.ID),
				logx.F("order_no", order.OrderNo),
			)
			updated, syncErr := uc.syncNextStepAfterExecution(ctx, order)
			return updated, false, syncErr
		}
		pendingExecution := findExecutionByStatus(executions, domain.ExecutionStatusPending)
		if pendingExecution != nil {
			logx.Info("release_tracker", "start_pending_execution",
				logx.F("order_id", order.ID),
				logx.F("order_no", order.OrderNo),
				logx.F("execution_id", pendingExecution.ID),
				logx.F("pipeline_scope", pendingExecution.PipelineScope),
				logx.F("provider", pendingExecution.Provider),
			)
			orderParams, paramErr := uc.manager.ListParams(ctx, order.ID)
			if paramErr != nil {
				return false, false, paramErr
			}
			if err := uc.manager.startNextPendingExecution(ctx, order, executions, orderParams); err != nil {
				return false, false, err
			}
			return true, false, nil
		}
		logx.Info("release_tracker", "finalize_order_attempt",
			logx.F("order_id", order.ID),
			logx.F("order_no", order.OrderNo),
		)
		return uc.finalizeOrder(ctx, order, executions)
	}

	switch strings.ToLower(strings.TrimSpace(runningExecution.Provider)) {
	case string(pipelinedomain.ProviderJenkins):
		// continue below
	case string(pipelinedomain.ProviderArgoCD):
		return uc.syncArgoCDExecution(ctx, order, *runningExecution, executions)
	default:
		return uc.finalizeOrder(ctx, order, executions)
	}

	queueURL := strings.TrimSpace(runningExecution.QueueURL)
	buildURL := strings.TrimSpace(runningExecution.BuildURL)
	if queueURL == "" && buildURL == "" {
		logx.Warn("release_tracker", "jenkins_execution_missing_urls",
			logx.F("order_id", order.ID),
			logx.F("order_no", order.OrderNo),
			logx.F("execution_id", runningExecution.ID),
			logx.F("pipeline_scope", runningExecution.PipelineScope),
		)
		if uc.now().Sub(order.UpdatedAt) < 2*time.Minute {
			return false, true, nil
		}
		updated, finishErr := uc.finishStep(
			ctx,
			order.ID,
			scopeStepCode(runningExecution.PipelineScope, "pipeline_running"),
			domain.StepStatusFailed,
			"未记录 Jenkins 队列/构建地址，无法追踪执行结果",
		)
		return updated, false, finishErr
	}

	if buildURL == "" && queueURL != "" {
		logx.Info("release_tracker", "resolve_build_url_from_queue",
			logx.F("order_id", order.ID),
			logx.F("execution_id", runningExecution.ID),
			logx.F("queue_url", queueURL),
		)
		resolvedBuildURL, cancelled, why, queueErr := uc.jenkins.GetQueueItem(ctx, queueURL)
		if queueErr != nil {
			if isResourceNotFoundError(queueErr) {
				if uc.now().Sub(order.UpdatedAt) < 2*time.Minute {
					return false, true, nil
				}
				updated, finishErr := uc.finishStep(
					ctx,
					order.ID,
					scopeStepCode(runningExecution.PipelineScope, "pipeline_running"),
					domain.StepStatusFailed,
					"Jenkins 队列记录已过期，无法追踪结果",
				)
				return updated, false, finishErr
			}
			return false, false, queueErr
		}
		if cancelled {
			logx.Warn("release_tracker", "jenkins_queue_cancelled",
				logx.F("order_id", order.ID),
				logx.F("execution_id", runningExecution.ID),
				logx.F("queue_url", queueURL),
				logx.F("why", why),
			)
			now := uc.now()
			_, _ = uc.manager.repo.UpdateExecutionByScope(ctx, order.ID, runningExecution.PipelineScope, domain.ExecutionUpdateInput{
				Status:     domain.ExecutionStatusCancelled,
				QueueURL:   queueURL,
				StartedAt:  runningExecution.StartedAt,
				FinishedAt: &now,
				UpdatedAt:  now,
			})
			updated, finishErr := uc.finishStep(
				ctx,
				order.ID,
				scopeStepCode(runningExecution.PipelineScope, "pipeline_running"),
				domain.StepStatusFailed,
				"Jenkins 队列已取消: "+strings.TrimSpace(why),
			)
			return updated, false, finishErr
		}
		buildURL = strings.TrimSpace(resolvedBuildURL)
		if buildURL == "" {
			logx.Info("release_tracker", "jenkins_queue_waiting_build_url",
				logx.F("order_id", order.ID),
				logx.F("execution_id", runningExecution.ID),
				logx.F("queue_url", queueURL),
			)
			return false, false, nil
		}
		logx.Info("release_tracker", "jenkins_build_url_resolved",
			logx.F("order_id", order.ID),
			logx.F("execution_id", runningExecution.ID),
			logx.F("queue_url", queueURL),
			logx.F("build_url", buildURL),
		)
		now := uc.now()
		if _, err := uc.manager.repo.UpdateExecutionByScope(ctx, order.ID, runningExecution.PipelineScope, domain.ExecutionUpdateInput{
			Status:    domain.ExecutionStatusRunning,
			QueueURL:  queueURL,
			BuildURL:  buildURL,
			StartedAt: runningExecution.StartedAt,
			UpdatedAt: now,
		}); err != nil {
			return false, false, err
		}
	}

	if uc.manager.pipelineRepo != nil {
		if binding, bindingErr := uc.manager.resolveExecutionBinding(ctx, order, *runningExecution); bindingErr == nil {
			_, _ = uc.manager.refreshPipelineStages(ctx, order, *runningExecution, binding)
		}
	}

	building, result, statusErr := uc.jenkins.GetBuildStatus(ctx, buildURL)
	if statusErr != nil {
		if isResourceNotFoundError(statusErr) {
			if uc.now().Sub(order.UpdatedAt) < 2*time.Minute {
				return false, true, nil
			}
			updated, finishErr := uc.finishStep(
				ctx,
				order.ID,
				scopeStepCode(runningExecution.PipelineScope, "pipeline_running"),
				domain.StepStatusFailed,
				"Jenkins 构建记录不存在，无法追踪结果",
			)
			return updated, false, finishErr
		}
		return false, false, statusErr
	}
	if building {
		logx.Info("release_tracker", "jenkins_build_still_running",
			logx.F("order_id", order.ID),
			logx.F("execution_id", runningExecution.ID),
			logx.F("build_url", buildURL),
		)
		return false, false, nil
	}

	result = strings.ToUpper(strings.TrimSpace(result))
	if result == "" {
		return false, false, nil
	}

	switch result {
	case "SUCCESS":
		logx.Info("release_tracker", "jenkins_build_success",
			logx.F("order_id", order.ID),
			logx.F("execution_id", runningExecution.ID),
			logx.F("build_url", buildURL),
			logx.F("result", result),
		)
		now := uc.now()
		_, _ = uc.manager.repo.UpdateExecutionByScope(ctx, order.ID, runningExecution.PipelineScope, domain.ExecutionUpdateInput{
			Status:     domain.ExecutionStatusSuccess,
			QueueURL:   queueURL,
			BuildURL:   buildURL,
			StartedAt:  runningExecution.StartedAt,
			FinishedAt: &now,
			UpdatedAt:  now,
		})
		if err := uc.recordArtifactMetadataFromJenkinsLog(ctx, order, *runningExecution, buildURL); err != nil {
			logx.Warn("release_tracker", "record_artifact_metadata_failed",
				logx.F("order_id", order.ID),
				logx.F("order_no", order.OrderNo),
				logx.F("execution_id", runningExecution.ID),
				logx.F("pipeline_scope", runningExecution.PipelineScope),
				logx.F("error", err.Error()),
			)
		}
		updated1, err := uc.finishStep(ctx, order.ID, scopeStepCode(runningExecution.PipelineScope, "pipeline_running"), domain.StepStatusSuccess, messageWithBuildURL("Jenkins 构建成功", buildURL))
		if err != nil {
			return false, false, err
		}
		updated2, err := uc.finishStep(ctx, order.ID, scopeStepCode(runningExecution.PipelineScope, "pipeline_success"), domain.StepStatusSuccess, "Jenkins 结果: "+result)
		if err != nil {
			return false, false, err
		}
		updated3, err := uc.syncNextStepAfterExecution(ctx, order)
		if err != nil {
			return false, false, err
		}
		return updated1 || updated2 || updated3, false, nil
	case "FAILURE", "ABORTED", "UNSTABLE", "NOT_BUILT":
		logx.Warn("release_tracker", "jenkins_build_failed",
			logx.F("order_id", order.ID),
			logx.F("execution_id", runningExecution.ID),
			logx.F("build_url", buildURL),
			logx.F("result", result),
		)
		now := uc.now()
		nextStatus := domain.ExecutionStatusFailed
		if result == "ABORTED" {
			nextStatus = domain.ExecutionStatusCancelled
		}
		_, _ = uc.manager.repo.UpdateExecutionByScope(ctx, order.ID, runningExecution.PipelineScope, domain.ExecutionUpdateInput{
			Status:     nextStatus,
			QueueURL:   queueURL,
			BuildURL:   buildURL,
			StartedAt:  runningExecution.StartedAt,
			FinishedAt: &now,
			UpdatedAt:  now,
		})
		updated, err := uc.finishStep(ctx, order.ID, scopeStepCode(runningExecution.PipelineScope, "pipeline_running"), domain.StepStatusFailed, messageWithBuildURL("Jenkins 结果: "+result, buildURL))
		if err != nil {
			return false, false, err
		}
		updated2, err := uc.finishStep(ctx, order.ID, scopeStepCode(runningExecution.PipelineScope, "pipeline_success"), domain.StepStatusFailed, "Jenkins 结果: "+result)
		if err != nil {
			return false, false, err
		}
		updated3, err := uc.failRemainingExecutions(ctx, order, executions, runningExecution.PipelineScope, "前置阶段失败，未继续执行")
		if err != nil {
			return false, false, err
		}
		return updated || updated2 || updated3, false, nil
	default:
		return false, false, nil
	}
}

// recordArtifactMetadataFromJenkinsLog 从 Jenkins 构建日志中提取 GOS_ARTIFACT_URL 并记录制品元信息。
func (uc *TrackReleaseExecution) recordArtifactMetadataFromJenkinsLog(
	ctx context.Context,
	order domain.ReleaseOrder,
	execution domain.ReleaseOrderExecution,
	buildURL string,
) error {
	if uc == nil || uc.manager == nil || uc.jenkins == nil {
		return nil
	}
	logClient, ok := uc.jenkins.(JenkinsReleaseConsoleTextClient)
	if !ok {
		return nil
	}

	artifactURLs, err := uc.readGOSArtifactURLsFromBuildLog(ctx, logClient, buildURL)
	if err != nil {
		return err
	}
	if len(artifactURLs) == 0 {
		return nil
	}

	buildNumber := resolveBuildNumberFromJenkinsBuildURL(buildURL)
	for _, artifactURL := range artifactURLs {
		artifactName := resolveArtifactNameFromURL(artifactURL)
		if _, err := uc.manager.RecordArtifactMetadata(ctx, order.ID, RecordReleaseOrderArtifactMetadataInput{
			ExecutionID:      execution.ID,
			PipelineScope:    string(execution.PipelineScope),
			ArtifactName:     artifactName,
			ArtifactType:     resolveArtifactTypeFromName(artifactName),
			ArtifactVersion:  buildNumber,
			ArtifactURL:      artifactURL,
			BuildNumber:      buildNumber,
			AdditionalFields: map[string]any{"source": "jenkins_log", "build_url": buildURL},
		}); err != nil {
			return err
		}
	}
	return nil
}

func (uc *TrackReleaseExecution) readGOSArtifactURLsFromBuildLog(
	ctx context.Context,
	logClient JenkinsReleaseConsoleTextClient,
	buildURL string,
) ([]string, error) {
	start := int64(0)
	scanned := int64(0)
	tail := ""
	urls := make([]string, 0, 1)
	seen := map[string]struct{}{}
	for {
		content, nextStart, moreData, err := logClient.GetBuildConsoleText(ctx, buildURL, start)
		if err != nil {
			return nil, err
		}
		for _, value := range extractGOSArtifactURLs(tail + content) {
			if _, ok := seen[value]; ok {
				continue
			}
			seen[value] = struct{}{}
			urls = append(urls, value)
		}
		scanned += int64(len(content))
		tail = lastStringSuffix(tail+content, 256)
		if !moreData || nextStart <= start || scanned >= maxArtifactLogScanBytes {
			break
		}
		start = nextStart
	}
	return urls, nil
}

func extractGOSArtifactURLs(content string) []string {
	matches := gosArtifactURLPattern.FindAllStringSubmatch(content, -1)
	result := make([]string, 0, len(matches))
	for _, match := range matches {
		if len(match) < 2 {
			continue
		}
		value := strings.TrimSpace(match[1])
		value = strings.Trim(value, `"'`)
		if value != "" {
			result = append(result, value)
		}
	}
	return result
}

func resolveArtifactNameFromURL(raw string) string {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return ""
	}
	name := strings.TrimSpace(path.Base(parsed.Path))
	if name == "." || name == "/" {
		return ""
	}
	return name
}

func resolveArtifactTypeFromName(name string) string {
	lower := strings.ToLower(strings.TrimSpace(name))
	switch {
	case strings.HasSuffix(lower, ".tar.gz"):
		return "tar.gz"
	case strings.HasSuffix(lower, ".tgz"):
		return "tgz"
	case strings.HasSuffix(lower, ".zip"):
		return "zip"
	case strings.HasSuffix(lower, ".jar"):
		return "jar"
	case strings.HasSuffix(lower, ".war"):
		return "war"
	default:
		index := strings.LastIndex(lower, ".")
		if index >= 0 && index < len(lower)-1 {
			return lower[index+1:]
		}
		return ""
	}
}

func resolveBuildNumberFromJenkinsBuildURL(buildURL string) string {
	trimmed := strings.Trim(strings.TrimSpace(buildURL), "/")
	if trimmed == "" {
		return ""
	}
	index := strings.LastIndex(trimmed, "/")
	if index < 0 {
		return trimmed
	}
	return strings.TrimSpace(trimmed[index+1:])
}

func lastStringSuffix(value string, maxLen int) string {
	if maxLen <= 0 || len(value) <= maxLen {
		return value
	}
	return value[len(value)-maxLen:]
}

// syncArgoCDExecution 同步外部或内部状态数据。
func (uc *TrackReleaseExecution) syncArgoCDExecution(
	ctx context.Context,
	order domain.ReleaseOrder,
	runningExecution domain.ReleaseOrderExecution,
	executions []domain.ReleaseOrderExecution,
) (bool, bool, error) {
	if uc.manager.argocdRepo == nil || uc.manager.argocdFactory == nil {
		return false, false, fmt.Errorf("%w: argocd executor is not configured", ErrInvalidInput)
	}
	logx.Info("release_tracker", "sync_argocd_execution_start",
		logx.F("order_id", order.ID),
		logx.F("order_no", order.OrderNo),
		logx.F("execution_id", runningExecution.ID),
		logx.F("pipeline_scope", runningExecution.PipelineScope),
	)

	steps, err := uc.manager.ListSteps(ctx, order.ID)
	if err != nil {
		return false, false, err
	}
	syncStep := findStepByCode(steps, scopeStepCode(runningExecution.PipelineScope, "argocd_sync"))
	if syncStep == nil || syncStep.Status != domain.StepStatusSuccess {
		// ArgoCD Sync 还没真正触发前，不能拿当前应用的旧 Healthy 状态来判这次发布成功。
		// 同时 startArgoCDExecution 已经会把执行单元提早置为 running，避免 tracker 重复启动 CD。
		logx.Info("release_tracker", "sync_argocd_waiting_sync_step",
			logx.F("order_id", order.ID),
			logx.F("execution_id", runningExecution.ID),
			logx.F("sync_step_found", syncStep != nil),
			logx.F("sync_step_status", stepStatusValue(syncStep)),
		)
		return false, true, nil
	}

	binding, _, client, err := uc.manager.resolveArgoCDExecutionContext(ctx, order, runningExecution, nil)
	if err != nil {
		return false, false, err
	}
	gitopsType, gitopsErr := uc.manager.resolveOrderGitOpsType(ctx, order)
	if gitopsErr != nil {
		return false, false, gitopsErr
	}
	if _, refreshErr := uc.manager.refreshPipelineStages(ctx, order, runningExecution, binding); refreshErr != nil {
		// 阶段刷新失败不直接打断主状态同步，避免短暂接口抖动影响发布闭环。
	}

	appName, app, err := resolveArgoCDApplicationByRef(ctx, client, binding.ExternalRef, strings.TrimSpace(order.EnvCode), gitopsType)
	if err != nil {
		if errors.Is(err, ErrInvalidInput) {
			updated, finishErr := uc.finishStep(
				ctx,
				order.ID,
				scopeStepCode(runningExecution.PipelineScope, "health_check"),
				domain.StepStatusFailed,
				"ArgoCD 绑定未配置可用的 GitOps 子目录或 Application 标识",
			)
			return updated, false, finishErr
		}
		if isResourceNotFoundError(err) {
			if uc.now().Sub(order.UpdatedAt) < 2*time.Minute {
				return false, true, nil
			}
			updated, finishErr := uc.finishStep(
				ctx,
				order.ID,
				scopeStepCode(runningExecution.PipelineScope, "health_check"),
				domain.StepStatusFailed,
				"ArgoCD Application 不存在，无法继续追踪部署状态",
			)
			return updated, false, finishErr
		}
		return false, false, err
	}
	if appName == "" || app == nil {
		updated, finishErr := uc.finishStep(
			ctx,
			order.ID,
			scopeStepCode(runningExecution.PipelineScope, "health_check"),
			domain.StepStatusFailed,
			"ArgoCD 绑定未配置可用的 GitOps 子目录或 Application 标识",
		)
		return updated, false, finishErr
	}
	logx.Info("release_tracker", "sync_argocd_application",
		logx.F("order_id", order.ID),
		logx.F("execution_id", runningExecution.ID),
		logx.F("app_name", appName),
		logx.F("gitops_type", gitopsType),
		logx.F("sync_status", app.GetSyncStatus()),
		logx.F("health_status", app.GetHealthStatus()),
		logx.F("operation_phase", app.GetOperationPhase()),
	)

	syncStatus := strings.ToLower(strings.TrimSpace(app.GetSyncStatus()))
	healthStatus := strings.ToLower(strings.TrimSpace(app.GetHealthStatus()))
	operationPhase := strings.ToLower(strings.TrimSpace(app.GetOperationPhase()))

	switch {
	case (syncStatus == "synced" || syncStatus == "") && healthStatus == "healthy" && operationPhase != "running":
		logx.Info("release_tracker", "sync_argocd_success",
			logx.F("order_id", order.ID),
			logx.F("execution_id", runningExecution.ID),
			logx.F("app_name", appName),
			logx.F("sync_status", app.GetSyncStatus()),
			logx.F("health_status", app.GetHealthStatus()),
		)
		now := uc.now()
		_, _ = uc.manager.repo.UpdateExecutionByScope(ctx, order.ID, runningExecution.PipelineScope, domain.ExecutionUpdateInput{
			Status:        domain.ExecutionStatusSuccess,
			ExternalRunID: runningExecution.ExternalRunID,
			StartedAt:     runningExecution.StartedAt,
			FinishedAt:    &now,
			UpdatedAt:     now,
		})
		updated1, err := uc.finishStep(ctx, order.ID, scopeStepCode(runningExecution.PipelineScope, "health_check"), domain.StepStatusSuccess, fmt.Sprintf("ArgoCD 应用已同步，sync=%s，health=%s", firstNonEmpty(app.GetSyncStatus(), "Synced"), firstNonEmpty(app.GetHealthStatus(), "Healthy")))
		if err != nil {
			return false, false, err
		}
		updated2, err := uc.syncNextStepAfterExecution(ctx, order)
		if err != nil {
			return false, false, err
		}
		return updated1 || updated2, false, nil
	case operationPhase == "failed" || healthStatus == "degraded":
		logx.Warn("release_tracker", "sync_argocd_failed",
			logx.F("order_id", order.ID),
			logx.F("execution_id", runningExecution.ID),
			logx.F("app_name", appName),
			logx.F("sync_status", app.GetSyncStatus()),
			logx.F("health_status", app.GetHealthStatus()),
			logx.F("operation_phase", app.GetOperationPhase()),
		)
		now := uc.now()
		_, _ = uc.manager.repo.UpdateExecutionByScope(ctx, order.ID, runningExecution.PipelineScope, domain.ExecutionUpdateInput{
			Status:        domain.ExecutionStatusFailed,
			ExternalRunID: runningExecution.ExternalRunID,
			StartedAt:     runningExecution.StartedAt,
			FinishedAt:    &now,
			UpdatedAt:     now,
		})
		message := fmt.Sprintf("ArgoCD 同步失败，sync=%s，health=%s，phase=%s", firstNonEmpty(app.GetSyncStatus(), "Unknown"), firstNonEmpty(app.GetHealthStatus(), "Unknown"), firstNonEmpty(app.GetOperationPhase(), "Unknown"))
		updated, err := uc.finishStep(ctx, order.ID, scopeStepCode(runningExecution.PipelineScope, "health_check"), domain.StepStatusFailed, message)
		if err != nil {
			return false, false, err
		}
		updated2, err := uc.failRemainingExecutions(ctx, order, executions, runningExecution.PipelineScope, "ArgoCD 部署失败，未继续执行后续阶段")
		if err != nil {
			return false, false, err
		}
		return updated || updated2, false, nil
	default:
		logx.Info("release_tracker", "sync_argocd_running",
			logx.F("order_id", order.ID),
			logx.F("execution_id", runningExecution.ID),
			logx.F("app_name", appName),
			logx.F("sync_status", app.GetSyncStatus()),
			logx.F("health_status", app.GetHealthStatus()),
			logx.F("operation_phase", app.GetOperationPhase()),
		)
		return false, false, nil
	}
}

// stepStatusValue 封装当前模块的业务处理逻辑。
func stepStatusValue(step *domain.ReleaseOrderStep) string {
	if step == nil {
		return ""
	}
	return string(step.Status)
}

// findExecutionByStatus 封装当前模块的业务处理逻辑。
func findExecutionByStatus(items []domain.ReleaseOrderExecution, status domain.ExecutionStatus) *domain.ReleaseOrderExecution {
	for idx := range items {
		if items[idx].Status == status {
			return &items[idx]
		}
	}
	return nil
}

// syncNextStepAfterExecution 同步外部或内部状态数据。
func (uc *TrackReleaseExecution) syncNextStepAfterExecution(ctx context.Context, order domain.ReleaseOrder) (bool, error) {
	executions, err := uc.manager.ListExecutions(ctx, order.ID)
	if err != nil {
		return false, err
	}
	buildHookUpdated, buildHookFinished, buildHookBlocked, err := uc.manager.syncHooksAfterBuild(ctx, order, executions)
	if err != nil {
		return false, err
	}
	if buildHookBlocked {
		updated, failErr := uc.failRemainingExecutions(ctx, order, executions, domain.PipelineScopeCI, "构建完成 Hook 执行失败，未继续执行后续阶段")
		if failErr != nil {
			return false, failErr
		}
		return buildHookUpdated || updated, nil
	}
	if !buildHookFinished {
		return buildHookUpdated, nil
	}
	if order.Status == domain.OrderStatusBuilding {
		if findExecutionByScopeAndStatus(executions, domain.PipelineScopeCD, domain.ExecutionStatusPending) != nil {
			now := uc.now()
			if _, err := uc.manager.repo.UpdateStatus(
				ctx,
				order.ID,
				domain.OrderStatusBuiltWaitingDeploy,
				firstNonNilTime(order.StartedAt, &now),
				nil,
				now,
			); err != nil {
				return false, err
			}
			if err := uc.manager.releaseExecutionLocks(ctx, order.ID, domain.ExecutionLockStatusReleased); err != nil {
				return false, err
			}
			if err := uc.manager.markStep(
				ctx,
				order.ID,
				"global:release_finish",
				domain.StepStatusPending,
				"构建已完成，等待部署触发",
				nil,
				nil,
			); err != nil {
				return false, err
			}
			return true, nil
		}
	}
	if findExecutionByStatus(executions, domain.ExecutionStatusPending) != nil {
		orderParams, err := uc.manager.ListParams(ctx, order.ID)
		if err != nil {
			return false, err
		}
		if err := uc.manager.startNextPendingExecution(ctx, order, executions, orderParams); err != nil {
			return false, err
		}
		return true, nil
	}
	updated, _, err := uc.finalizeOrder(ctx, order, executions)
	return updated || buildHookUpdated, err
}

// finalizeOrder 封装当前模块的业务处理逻辑。
func (uc *TrackReleaseExecution) finalizeOrder(
	ctx context.Context,
	order domain.ReleaseOrder,
	executions []domain.ReleaseOrderExecution,
) (bool, bool, error) {
	now := uc.now()
	if len(executions) == 0 {
		return false, true, nil
	}

	stepUpdated, finished, orderStatus, message, err := uc.manager.syncHooksAfterRelease(ctx, order, executions)
	if err != nil {
		return false, false, err
	}
	if !finished {
		return stepUpdated, false, nil
	}

	stepStatus := domain.StepStatusSuccess
	if orderStatus != domain.OrderStatusSuccess {
		stepStatus = domain.StepStatusFailed
	}
	updated, err := uc.finishStep(ctx, order.ID, "global:release_finish", stepStatus, message)
	if err != nil {
		return false, false, err
	}
	updatedOrder, err := uc.manager.repo.UpdateStatus(ctx, order.ID, orderStatus, order.StartedAt, &now, now)
	if err != nil {
		return false, false, err
	}
	if orderStatus == domain.OrderStatusSuccess || orderStatus == domain.OrderStatusDeploySuccess {
		if stateErr := uc.manager.RecordAppReleaseState(ctx, updatedOrder.ID); stateErr != nil {
			logx.Error("release_order", "record_app_release_state_failed", stateErr,
				logx.F("order_id", updatedOrder.ID),
				logx.F("order_no", updatedOrder.OrderNo),
				logx.F("status", updatedOrder.Status),
			)
		}
	}
	if err := uc.manager.releaseExecutionLocks(ctx, order.ID, domain.ExecutionLockStatusReleased); err != nil {
		return false, false, err
	}
	return updated || stepUpdated, false, nil
}

// failRemainingExecutions 启动当前进程并完成依赖初始化。
func (uc *TrackReleaseExecution) failRemainingExecutions(
	ctx context.Context,
	order domain.ReleaseOrder,
	executions []domain.ReleaseOrderExecution,
	failedScope domain.PipelineScope,
	message string,
) (bool, error) {
	now := uc.now()
	updated := false
	for _, item := range executions {
		if item.PipelineScope == failedScope || item.Status != domain.ExecutionStatusPending {
			continue
		}
		if _, err := uc.manager.repo.UpdateExecutionByScope(ctx, order.ID, item.PipelineScope, domain.ExecutionUpdateInput{
			Status:     domain.ExecutionStatusSkipped,
			StartedAt:  &now,
			FinishedAt: &now,
			UpdatedAt:  now,
		}); err != nil {
			return false, err
		}
		for _, code := range executionStepCodes(item) {
			if ok, err := uc.finishStep(ctx, order.ID, code, domain.StepStatusFailed, message); err != nil {
				return false, err
			} else if ok {
				updated = true
			}
		}
	}
	if _, err := uc.finishStep(ctx, order.ID, "global:release_finish", domain.StepStatusFailed, message); err != nil {
		return false, err
	}
	if order.Status == domain.OrderStatusPending || order.Status == domain.OrderStatusQueued || order.Status == domain.OrderStatusDeploying || order.Status == domain.OrderStatusBuilding {
		if _, err := uc.manager.repo.UpdateStatus(ctx, order.ID, domain.OrderStatusRunning, firstNonNilTime(order.StartedAt, &now), nil, now); err != nil {
			return false, err
		}
	}
	return updated || true, nil
}

// syncCancelledOrder 同步外部或内部状态数据。
func (uc *TrackReleaseExecution) syncCancelledOrder(
	ctx context.Context,
	order domain.ReleaseOrder,
) (bool, bool, error) {
	executions, err := uc.manager.ListExecutions(ctx, order.ID)
	if err != nil {
		return false, false, err
	}
	steps, err := uc.manager.ListSteps(ctx, order.ID)
	if err != nil {
		return false, false, err
	}

	now := uc.now()
	updated := false
	for _, execution := range executions {
		if execution.Status.IsTerminal() {
			continue
		}
		if _, err := uc.manager.repo.UpdateExecutionByScope(ctx, order.ID, execution.PipelineScope, domain.ExecutionUpdateInput{
			Status:     domain.ExecutionStatusCancelled,
			QueueURL:   execution.QueueURL,
			BuildURL:   execution.BuildURL,
			StartedAt:  execution.StartedAt,
			FinishedAt: &now,
			UpdatedAt:  now,
		}); err != nil && !errors.Is(err, domain.ErrExecutionNotFound) {
			return false, false, err
		} else if err == nil {
			updated = true
		}
	}

	for _, step := range steps {
		if !shouldFinishStepOnCancel(step) {
			continue
		}
		if ok, err := uc.finishStep(ctx, order.ID, step.StepCode, domain.StepStatusFailed, "发布已取消"); err != nil {
			return false, false, err
		} else if ok {
			updated = true
		}
	}

	if order.FinishedAt == nil {
		if _, err := uc.manager.repo.UpdateStatus(ctx, order.ID, domain.OrderStatusCancelled, order.StartedAt, &now, now); err != nil {
			return false, false, err
		}
		updated = true
	}
	if err := uc.manager.releaseExecutionLocks(ctx, order.ID, domain.ExecutionLockStatusReleased); err != nil {
		return false, false, err
	}
	return updated, false, nil
}

// syncFailedOrder 同步外部或内部状态数据。
func (uc *TrackReleaseExecution) syncFailedOrder(
	ctx context.Context,
	order domain.ReleaseOrder,
) (bool, bool, error) {
	executions, err := uc.manager.ListExecutions(ctx, order.ID)
	if err != nil {
		return false, false, err
	}
	steps, err := uc.manager.ListSteps(ctx, order.ID)
	if err != nil {
		return false, false, err
	}

	now := uc.now()
	updated := false
	for _, execution := range executions {
		if execution.Status.IsTerminal() {
			continue
		}
		if _, err := uc.manager.repo.UpdateExecutionByScope(ctx, order.ID, execution.PipelineScope, domain.ExecutionUpdateInput{
			Status:     domain.ExecutionStatusFailed,
			QueueURL:   execution.QueueURL,
			BuildURL:   execution.BuildURL,
			StartedAt:  execution.StartedAt,
			FinishedAt: &now,
			UpdatedAt:  now,
		}); err != nil && !errors.Is(err, domain.ErrExecutionNotFound) {
			return false, false, err
		} else if err == nil {
			updated = true
		}
	}

	for _, step := range steps {
		if !shouldFinishStepOnFailure(step) {
			continue
		}
		if ok, err := uc.finishStep(ctx, order.ID, step.StepCode, domain.StepStatusFailed, "发布已失败，终止剩余执行"); err != nil {
			return false, false, err
		} else if ok {
			updated = true
		}
	}

	if order.FinishedAt == nil {
		if _, err := uc.manager.repo.UpdateStatus(ctx, order.ID, domain.OrderStatusFailed, order.StartedAt, &now, now); err != nil {
			return false, false, err
		}
		updated = true
	}

	normalizedOrder, err := uc.manager.repo.GetByID(ctx, order.ID)
	if err != nil {
		return false, false, err
	}
	normalizedExecutions, err := uc.manager.ListExecutions(ctx, order.ID)
	if err != nil {
		return false, false, err
	}
	finalizeUpdated, skipped, err := uc.finalizeOrder(ctx, normalizedOrder, normalizedExecutions)
	if err != nil {
		return false, false, err
	}
	return updated || finalizeUpdated, skipped, nil
}

// finishStep 封装当前模块的业务处理逻辑。
func (uc *TrackReleaseExecution) finishStep(
	ctx context.Context,
	orderID string,
	stepCode string,
	status domain.StepStatus,
	message string,
) (bool, error) {
	steps, err := uc.manager.ListSteps(ctx, orderID)
	if err != nil {
		return false, err
	}
	current := findStepByCode(steps, stepCode)
	if current == nil {
		return false, nil
	}
	switch current.Status {
	case domain.StepStatusSuccess, domain.StepStatusFailed:
		if current.Status != status && stepCode == "global:release_finish" && status == domain.StepStatusFailed {
			now := uc.now()
			startedAt := current.StartedAt
			if startedAt == nil {
				startedAt = &now
			}
			if err := uc.manager.markStep(ctx, orderID, stepCode, status, strings.TrimSpace(message), startedAt, &now); err != nil {
				return false, err
			}
			return true, nil
		}
		return false, nil
	case domain.StepStatusPending:
		_, _, err := uc.manager.StartStep(ctx, orderID, stepCode, "")
		if err != nil && !errors.Is(err, ErrInvalidInput) {
			return false, err
		}
	}

	steps, err = uc.manager.ListSteps(ctx, orderID)
	if err != nil {
		return false, err
	}
	current = findStepByCode(steps, stepCode)
	if current == nil || current.Status != domain.StepStatusRunning {
		return false, nil
	}

	_, _, err = uc.manager.FinishStep(ctx, orderID, stepCode, FinishReleaseOrderStepInput{
		Status:  status,
		Message: message,
	})
	if err != nil {
		if errors.Is(err, ErrInvalidInput) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// findStepByCode 封装当前模块的业务处理逻辑。
func findStepByCode(steps []domain.ReleaseOrderStep, stepCode string) *domain.ReleaseOrderStep {
	for index := range steps {
		if steps[index].StepCode == stepCode {
			return &steps[index]
		}
	}
	return nil
}

// extractQueueURLFromSteps 封装当前模块的业务处理逻辑。
func extractQueueURLFromSteps(steps []domain.ReleaseOrderStep) string {
	for _, step := range steps {
		if step.StepCode != "trigger_pipeline" {
			continue
		}
		if queueURL := extractQueueURL(step.Message); queueURL != "" {
			return queueURL
		}
	}
	for _, step := range steps {
		if queueURL := extractQueueURL(step.Message); queueURL != "" {
			return queueURL
		}
	}
	return ""
}

// extractBuildURLFromSteps 组装业务执行所需的输入数据。
func extractBuildURLFromSteps(steps []domain.ReleaseOrderStep) string {
	for _, step := range steps {
		if step.StepCode == "pipeline_running" || step.StepCode == "trigger_pipeline" {
			if buildURL := extractBuildURL(step.Message); buildURL != "" {
				return buildURL
			}
		}
	}
	for _, step := range steps {
		if buildURL := extractBuildURL(step.Message); buildURL != "" {
			return buildURL
		}
	}
	return ""
}

// extractQueueURL 封装当前模块的业务处理逻辑。
func extractQueueURL(message string) string {
	matches := queueURLPattern.FindStringSubmatch(strings.TrimSpace(message))
	if len(matches) < 2 {
		return ""
	}
	queueURL := strings.TrimSpace(matches[1])
	queueURL = strings.TrimRight(queueURL, "，,;；")
	return queueURL
}

// extractBuildURL 组装业务执行所需的输入数据。
func extractBuildURL(message string) string {
	matches := buildURLPattern.FindStringSubmatch(strings.TrimSpace(message))
	if len(matches) < 2 {
		for _, candidate := range rawURLPattern.FindAllString(strings.TrimSpace(message), -1) {
			buildURL := strings.TrimSpace(candidate)
			buildURL = strings.TrimRight(buildURL, "，,;；")
			lowerURL := strings.ToLower(buildURL)
			if strings.Contains(lowerURL, "/queue/item/") {
				continue
			}
			if strings.Contains(lowerURL, "/job/") {
				return buildURL
			}
		}
		return ""
	}
	buildURL := strings.TrimSpace(matches[1])
	buildURL = strings.TrimRight(buildURL, "，,;；")
	return buildURL
}

// messageWithBuildURL 组装业务执行所需的输入数据。
func messageWithBuildURL(message string, buildURL string) string {
	message = strings.TrimSpace(message)
	buildURL = strings.TrimSpace(buildURL)
	if buildURL == "" {
		return message
	}
	if extracted := extractBuildURL(message); extracted != "" {
		return message
	}
	message = strings.TrimRight(message, "，,;； ")
	if message == "" {
		return "build: " + buildURL
	}
	return message + "，build: " + buildURL
}

// isResourceNotFoundError 封装当前模块的业务处理逻辑。
func isResourceNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	text := strings.ToLower(err.Error())
	return strings.Contains(text, "status=404") || strings.Contains(text, "status=410")
}
