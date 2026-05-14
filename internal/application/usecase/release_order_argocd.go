package usecase

import (
	"context"
	"encoding/json"
	"fmt"
	"path"
	"regexp"
	"strings"
	"time"

	appdomain "gos/internal/domain/application"
	argocddomain "gos/internal/domain/argocdapp"
	gitopsdomain "gos/internal/domain/gitops"
	pipelinedomain "gos/internal/domain/pipeline"
	domain "gos/internal/domain/release"
	"gos/internal/support/logx"
)

var gitopsTemplatePlaceholderPattern = regexp.MustCompile(`\{([a-zA-Z0-9_]+)\}`)

type helmDeploySnapshotPayload struct {
	ImageVersion string                   `json:"image_version"`
	Rules        []helmDeploySnapshotRule `json:"rules"`
}

type helmDeploySnapshotRule struct {
	FilePath   string `json:"file_path"`
	TargetPath string `json:"target_path"`
	Value      string `json:"value"`
}

// startArgoCDExecution 封装当前模块的业务处理逻辑。
func (uc *ReleaseOrderManager) startArgoCDExecution(
	ctx context.Context,
	order domain.ReleaseOrder,
	execution domain.ReleaseOrderExecution,
	orderParams []domain.ReleaseOrderParam,
	executions []domain.ReleaseOrderExecution,
) error {
	logx.Info("argocd_cd", "start_execution",
		logx.F("order_id", order.ID),
		logx.F("order_no", order.OrderNo),
		logx.F("execution_id", execution.ID),
		logx.F("pipeline_scope", execution.PipelineScope),
		logx.F("env_code", order.EnvCode),
	)
	if uc.argocdRepo == nil || uc.argocdFactory == nil {
		err := fmt.Errorf("%w: argocd executor is not configured", ErrInvalidInput)
		logx.Error("argocd_cd", "start_execution_failed", err,
			logx.F("order_id", order.ID),
			logx.F("order_no", order.OrderNo),
		)
		return err
	}

	template, _, _, templateGitOpsRules, _, err := uc.repo.GetTemplateByID(ctx, order.TemplateID)
	if err != nil {
		logx.Error("argocd_cd", "start_execution_failed", err,
			logx.F("order_id", order.ID),
			logx.F("template_id", order.TemplateID),
		)
		return err
	}
	gitopsType := normalizeTemplateGitOpsType(template.GitOpsType, true)
	if err := uc.ensureArgoCDExecutionSteps(ctx, order, executions, gitopsType); err != nil {
		logx.Error("argocd_cd", "ensure_steps_failed", err,
			logx.F("order_id", order.ID),
			logx.F("gitops_type", gitopsType),
		)
		return err
	}

	updateCode := scopeStepCode(execution.PipelineScope, "gitops_update")
	commitCode := scopeStepCode(execution.PipelineScope, "git_commit")
	pushCode := scopeStepCode(execution.PipelineScope, "git_push")
	syncCode := scopeStepCode(execution.PipelineScope, "argocd_sync")
	healthCode := scopeStepCode(execution.PipelineScope, "health_check")

	startedAt := execution.StartedAt
	if startedAt == nil {
		now := uc.now()
		startedAt = &now
	}
	if _, err := uc.repo.UpdateExecutionByScope(ctx, order.ID, execution.PipelineScope, domain.ExecutionUpdateInput{
		Status:    domain.ExecutionStatusRunning,
		StartedAt: startedAt,
		UpdatedAt: uc.now(),
	}); err != nil {
		logx.Error("argocd_cd", "start_execution_failed", err,
			logx.F("order_id", order.ID),
			logx.F("execution_id", execution.ID),
		)
		return err
	}

	_ = uc.markStepRunning(ctx, order.ID, updateCode, "开始更新 GitOps 配置")

	isRollback, rollbackErr := uc.shouldUseArgoCDRollback(ctx, order)
	if rollbackErr != nil {
		_ = uc.markStepFinished(ctx, order.ID, updateCode, domain.StepStatusFailed, "判定重放模式失败: "+rollbackErr.Error())
		return rollbackErr
	}
	logx.Info("argocd_cd", "execution_mode_resolved",
		logx.F("order_id", order.ID),
		logx.F("order_no", order.OrderNo),
		logx.F("operation_type", order.OperationType),
		logx.F("source_order_id", order.SourceOrderID),
		logx.F("is_rollback", isRollback),
	)
	if isRollback {
		return uc.startArgoCDRollbackExecution(
			ctx,
			order,
			execution,
			orderParams,
			updateCode,
			commitCode,
			pushCode,
			syncCode,
			healthCode,
			startedAt,
		)
	}

	binding, argocdInstance, client, err := uc.resolveArgoCDExecutionContext(ctx, order, execution, orderParams)
	if err != nil {
		_ = uc.markStepFinished(ctx, order.ID, updateCode, domain.StepStatusFailed, "解析 ArgoCD 绑定失败: "+err.Error())
		logx.Error("argocd_cd", "resolve_context_failed", err,
			logx.F("order_id", order.ID),
			logx.F("execution_id", execution.ID),
		)
		return err
	}
	gitopsService, err := uc.resolveGitOpsService(ctx, argocdInstance)
	if err != nil {
		_ = uc.markStepFinished(ctx, order.ID, updateCode, domain.StepStatusFailed, "解析 GitOps 实例失败: "+err.Error())
		logx.Error("argocd_cd", "resolve_gitops_failed", err,
			logx.F("order_id", order.ID),
			logx.F("argocd_instance_id", argocdInstance.ID),
			logx.F("argocd_instance_code", argocdInstance.InstanceCode),
		)
		return err
	}
	appKey := ""
	if uc.appRepo != nil {
		appRecord, appErr := uc.appRepo.GetByID(ctx, strings.TrimSpace(order.ApplicationID))
		if appErr == nil {
			appKey = strings.TrimSpace(appRecord.Key)
		}
	}
	environment := uc.resolveArgoCDEnvironment(order, orderParams)
	appName, app, err := resolveArgoCDApplicationByRef(ctx, client, binding.ExternalRef, environment, gitopsType)
	if err != nil {
		_ = uc.markStepFinished(ctx, order.ID, updateCode, domain.StepStatusFailed, "加载 ArgoCD Application 失败: "+err.Error())
		logx.Error("argocd_cd", "resolve_application_failed", err,
			logx.F("order_id", order.ID),
			logx.F("binding_id", binding.ID),
			logx.F("external_ref", binding.ExternalRef),
			logx.F("env_code", environment),
			logx.F("gitops_type", gitopsType),
		)
		return fmt.Errorf("%w: get argocd application failed: %v", ErrInvalidInput, err)
	}
	logx.Info("argocd_cd", "application_resolved",
		logx.F("order_id", order.ID),
		logx.F("app_name", appName),
		logx.F("repo_url", app.GetRepoURL()),
		logx.F("source_path", app.GetSourcePath()),
		logx.F("target_revision", app.GetTargetRevision()),
		logx.F("argocd_instance_code", argocdInstance.InstanceCode),
	)

	repoURL := strings.TrimSpace(app.GetRepoURL())
	sourcePath := strings.TrimSpace(app.GetSourcePath())
	gitopsBranch := uc.resolveGitOpsTargetBranch(ctx, order, orderParams, argocdInstance, app)
	if repoURL == "" || sourcePath == "" {
		_ = uc.markStepFinished(ctx, order.ID, updateCode, domain.StepStatusFailed, "ArgoCD Application 缺少仓库地址或源路径")
		err := fmt.Errorf("%w: argocd application source repo/path is incomplete", ErrInvalidInput)
		logx.Error("argocd_cd", "start_execution_failed", err,
			logx.F("order_id", order.ID),
			logx.F("app_name", appName),
			logx.F("repo_url", repoURL),
			logx.F("source_path", sourcePath),
		)
		return err
	}

	imageVersion := uc.resolveArgoCDImageVersion(order, orderParams, executions)
	if imageVersion == "" {
		_ = uc.markStepFinished(ctx, order.ID, updateCode, domain.StepStatusFailed, "未解析到 image_version，无法继续写入 GitOps 配置")
		err := fmt.Errorf("%w: image_version is required when cd executor is argocd", ErrInvalidInput)
		logx.Error("argocd_cd", "start_execution_failed", err,
			logx.F("order_id", order.ID),
			logx.F("order_no", order.OrderNo),
			logx.F("app_name", appName),
		)
		return err
	}
	commitFields := buildGitOpsCommitMessageFields(order, orderParams, appKey, environment, imageVersion, sourcePath)
	commitMessage := gitopsService.BuildCommitMessage(commitFields)
	logx.Info("argocd_cd", "resolved_runtime_context",
		logx.F("order_id", order.ID),
		logx.F("app_name", appName),
		logx.F("env_code", environment),
		logx.F("image_version", imageVersion),
		logx.F("gitops_type", gitopsType),
	)

	manifestChangedFiles := make([]string, 0)
	commitSHA := ""
	manifestChanged := false
	manifestPath := ""
	previousTag := ""
	changed := false
	switch gitopsType {
	case domain.GitOpsTypeHelm:
		valuesRules, buildErr := uc.buildArgoCDValuesRules(gitopsService, templateGitOpsRules, commitFields)
		if buildErr != nil {
			_ = uc.markStepFinished(ctx, order.ID, updateCode, domain.StepStatusFailed, "GitOps 规则渲染失败: "+buildErr.Error())
			return buildErr
		}
		_, valuesFiles, valuesCommitSHA, valuesChanged, applyErr := gitopsService.ApplyValuesRules(
			ctx,
			repoURL,
			gitopsBranch,
			valuesRules,
			commitMessage,
		)
		if applyErr != nil {
			_ = uc.markStepFinished(ctx, order.ID, updateCode, domain.StepStatusFailed, "Helm values 写回失败: "+applyErr.Error())
			logx.Error("argocd_cd", "apply_values_rules_failed", applyErr,
				logx.F("order_id", order.ID),
				logx.F("order_no", order.OrderNo),
				logx.F("repo_url", repoURL),
				logx.F("target_revision", app.GetTargetRevision()),
				logx.F("gitops_branch", gitopsBranch),
				logx.F("rules_count", len(valuesRules)),
			)
			return fmt.Errorf("%w: apply gitops values rules failed: %v", ErrInvalidInput, applyErr)
		}
		logx.Info("argocd_cd", "apply_values_rules_success",
			logx.F("order_id", order.ID),
			logx.F("changed_files_count", len(valuesFiles)),
			logx.F("commit_sha", valuesCommitSHA),
			logx.F("changed", valuesChanged),
		)
		manifestChangedFiles = append(manifestChangedFiles, valuesFiles...)
		if strings.TrimSpace(valuesCommitSHA) != "" {
			commitSHA = strings.TrimSpace(valuesCommitSHA)
		}
		manifestChanged = valuesChanged
		if snapshotErr := uc.saveHelmDeploySnapshot(ctx, order, argocdInstance, appName, repoURL, gitopsBranch, sourcePath, environment, imageVersion, valuesRules); snapshotErr != nil {
			_ = uc.markStepFinished(ctx, order.ID, updateCode, domain.StepStatusFailed, "保存部署快照失败: "+snapshotErr.Error())
			return fmt.Errorf("%w: save deploy snapshot failed: %v", ErrInvalidInput, snapshotErr)
		}
	default:
		if len(templateGitOpsRules) > 0 {
			manifestRules, buildErr := uc.buildArgoCDManifestRules(gitopsService, templateGitOpsRules, commitFields)
			if buildErr != nil {
				_ = uc.markStepFinished(ctx, order.ID, updateCode, domain.StepStatusFailed, "GitOps 规则渲染失败: "+buildErr.Error())
				return buildErr
			}
			if len(manifestRules) > 0 {
				_, manifestFiles, manifestCommitSHA, changed, applyErr := gitopsService.ApplyManifestRules(
					ctx,
					repoURL,
					gitopsBranch,
					manifestRules,
					commitMessage,
				)
				if applyErr != nil {
					_ = uc.markStepFinished(ctx, order.ID, updateCode, domain.StepStatusFailed, "GitOps YAML 替换失败: "+applyErr.Error())
					logx.Error("argocd_cd", "apply_manifest_rules_failed", applyErr,
						logx.F("order_id", order.ID),
						logx.F("repo_url", repoURL),
						logx.F("target_revision", app.GetTargetRevision()),
						logx.F("gitops_branch", gitopsBranch),
						logx.F("rules_count", len(manifestRules)),
					)
					return fmt.Errorf("%w: apply gitops manifest rules failed: %v", ErrInvalidInput, applyErr)
				}
				logx.Info("argocd_cd", "apply_manifest_rules_success",
					logx.F("order_id", order.ID),
					logx.F("changed_files_count", len(manifestFiles)),
					logx.F("commit_sha", manifestCommitSHA),
					logx.F("changed", changed),
				)
				manifestChangedFiles = append(manifestChangedFiles, manifestFiles...)
				if strings.TrimSpace(manifestCommitSHA) != "" {
					commitSHA = strings.TrimSpace(manifestCommitSHA)
				}
				manifestChanged = changed
			}
		}

		var imageCommitSHA string
		var imagePreviousTag string
		var imageChanged bool
		var updateErr error
		_, manifestPath, imageCommitSHA, imagePreviousTag, imageChanged, updateErr = gitopsService.UpdateKustomizationImage(
			ctx,
			repoURL,
			sourcePath,
			gitopsBranch,
			imageVersion,
			commitMessage,
		)
		if updateErr != nil {
			_ = uc.markStepFinished(ctx, order.ID, updateCode, domain.StepStatusFailed, "GitOps 仓库更新失败: "+updateErr.Error())
			logx.Error("argocd_cd", "update_kustomization_failed", updateErr,
				logx.F("order_id", order.ID),
				logx.F("repo_url", repoURL),
				logx.F("source_path", sourcePath),
				logx.F("target_revision", app.GetTargetRevision()),
				logx.F("gitops_branch", gitopsBranch),
				logx.F("image_version", imageVersion),
			)
			return fmt.Errorf("%w: update gitops repo failed: %v", ErrInvalidInput, updateErr)
		}
		logx.Info("argocd_cd", "update_kustomization_success",
			logx.F("order_id", order.ID),
			logx.F("manifest_path", manifestPath),
			logx.F("commit_sha", imageCommitSHA),
			logx.F("previous_tag", imagePreviousTag),
			logx.F("changed", imageChanged),
		)
		if strings.TrimSpace(imageCommitSHA) != "" {
			commitSHA = strings.TrimSpace(imageCommitSHA)
		}
		previousTag = imagePreviousTag
		changed = imageChanged
	}

	updateMessage := "GitOps 配置已更新"
	switch gitopsType {
	case domain.GitOpsTypeHelm:
		if manifestChanged {
			updateMessage = "Helm values 已更新"
		} else {
			updateMessage = "Helm values 无变化"
		}
	default:
		if changed {
			updateMessage = fmt.Sprintf("GitOps 配置已更新，image_version: %s -> %s", previousTag, imageVersion)
		} else if manifestChanged {
			updateMessage = "GitOps YAML 字段替换已应用"
		} else {
			updateMessage = fmt.Sprintf("GitOps 配置无变化，image_version 已是 %s", imageVersion)
		}
	}
	if len(manifestChangedFiles) > 0 {
		updateMessage += fmt.Sprintf("，变更文件 %d 个", len(manifestChangedFiles))
	}
	if manifestPath != "" {
		updateMessage += "，file: " + strings.TrimSpace(manifestPath)
	}
	_ = uc.markStepFinished(ctx, order.ID, updateCode, domain.StepStatusSuccess, updateMessage)

	_ = uc.markStepRunning(ctx, order.ID, commitCode, "开始提交 Git 变更")
	if strings.TrimSpace(commitSHA) != "" && (manifestChanged || changed) {
		_ = uc.markStepFinished(ctx, order.ID, commitCode, domain.StepStatusSuccess, "Git commit 成功，commit: "+strings.TrimSpace(commitSHA))
		_ = uc.markStepRunning(ctx, order.ID, pushCode, "开始推送 Git 变更")
		_ = uc.markStepFinished(ctx, order.ID, pushCode, domain.StepStatusSuccess, "Git push 成功，commit: "+strings.TrimSpace(commitSHA))
	} else {
		_ = uc.markStepFinished(ctx, order.ID, commitCode, domain.StepStatusSuccess, "GitOps 配置无变化，无需生成新的 commit")
		_ = uc.markStepFinished(ctx, order.ID, pushCode, domain.StepStatusSuccess, "GitOps 配置无变化，无需推送到远端仓库")
	}

	_ = uc.markStepRunning(ctx, order.ID, syncCode, "开始触发 ArgoCD Sync")
	if err := client.SyncApplicationWithRevision(ctx, appName, gitopsBranch); err != nil {
		_ = uc.markStepFinished(ctx, order.ID, syncCode, domain.StepStatusFailed, "触发 ArgoCD Sync 失败: "+err.Error())
		logx.Error("argocd_cd", "sync_application_failed", err,
			logx.F("order_id", order.ID),
			logx.F("app_name", appName),
			logx.F("argocd_instance_code", argocdInstance.InstanceCode),
			logx.F("gitops_branch", gitopsBranch),
		)
		return fmt.Errorf("%w: trigger argocd sync failed: %v", ErrInvalidInput, err)
	}
	logx.Info("argocd_cd", "sync_application_success",
		logx.F("order_id", order.ID),
		logx.F("app_name", appName),
		logx.F("argocd_instance_code", argocdInstance.InstanceCode),
	)

	now := uc.now()
	if _, err := uc.repo.UpdateExecutionByScope(ctx, order.ID, execution.PipelineScope, domain.ExecutionUpdateInput{
		Status:        domain.ExecutionStatusRunning,
		ExternalRunID: commitSHA,
		StartedAt:     startedAt,
		UpdatedAt:     now,
	}); err != nil {
		logx.Error("argocd_cd", "start_execution_failed", err,
			logx.F("order_id", order.ID),
			logx.F("execution_id", execution.ID),
		)
		return err
	}

	syncMessage := fmt.Sprintf("ArgoCD Sync 已触发，app: %s", appName)
	if strings.TrimSpace(commitSHA) != "" {
		syncMessage += "，commit: " + strings.TrimSpace(commitSHA)
	}
	_ = uc.markStepFinished(ctx, order.ID, syncCode, domain.StepStatusSuccess, syncMessage)
	_ = uc.markStepRunning(ctx, order.ID, healthCode, "ArgoCD Sync 已触发，等待健康检查回传")

	if _, refreshErr := uc.refreshPipelineStages(ctx, order, execution, binding); refreshErr != nil {
		// 阶段同步只是补充视图，失败不阻断实际发布链路。
	}
	logx.Info("argocd_cd", "start_execution_success",
		logx.F("order_id", order.ID),
		logx.F("order_no", order.OrderNo),
		logx.F("execution_id", execution.ID),
		logx.F("app_name", appName),
		logx.F("argocd_instance_code", argocdInstance.InstanceCode),
		logx.F("gitops_type", gitopsType),
		logx.F("commit_sha", commitSHA),
	)
	return nil
}

// shouldUseArgoCDRollback 封装当前模块的业务处理逻辑。
func (uc *ReleaseOrderManager) shouldUseArgoCDRollback(
	ctx context.Context,
	order domain.ReleaseOrder,
) (bool, error) {
	_ = ctx
	if !strings.EqualFold(strings.TrimSpace(string(order.OperationType)), string(domain.OperationTypeRollback)) {
		return false, nil
	}
	if strings.TrimSpace(order.SourceOrderID) == "" {
		return false, fmt.Errorf("%w: rollback source order is required", ErrInvalidInput)
	}
	return true, nil
}

// startArgoCDRollbackExecution 封装当前模块的业务处理逻辑。
func (uc *ReleaseOrderManager) startArgoCDRollbackExecution(
	ctx context.Context,
	order domain.ReleaseOrder,
	execution domain.ReleaseOrderExecution,
	orderParams []domain.ReleaseOrderParam,
	updateCode string,
	commitCode string,
	pushCode string,
	syncCode string,
	healthCode string,
	startedAt *time.Time,
) error {
	logx.Info("argocd_cd", "start_rollback_execution",
		logx.F("order_id", order.ID),
		logx.F("order_no", order.OrderNo),
		logx.F("source_order_id", order.SourceOrderID),
		logx.F("source_order_no", order.SourceOrderNo),
	)
	snapshot, err := uc.repo.GetDeploySnapshotByOrderID(ctx, strings.TrimSpace(order.SourceOrderID))
	if err != nil {
		_ = uc.markStepFinished(ctx, order.ID, updateCode, domain.StepStatusFailed, "加载历史部署快照失败: "+err.Error())
		return err
	}
	if normalizeTemplateGitOpsType(snapshot.GitOpsType, true) != domain.GitOpsTypeHelm {
		err = fmt.Errorf("%w: 当前来源单不支持标准回滚", ErrInvalidInput)
		_ = uc.markStepFinished(ctx, order.ID, updateCode, domain.StepStatusFailed, err.Error())
		return err
	}
	argocdInstance, err := uc.argocdRepo.GetInstanceByID(ctx, strings.TrimSpace(snapshot.ArgoCDInstanceID))
	if err != nil {
		_ = uc.markStepFinished(ctx, order.ID, updateCode, domain.StepStatusFailed, "加载 ArgoCD 实例失败: "+err.Error())
		return err
	}
	client := uc.argocdFactory.Build(argocdInstance)
	if client == nil {
		err = fmt.Errorf("%w: argocd client is not configured", ErrInvalidInput)
		_ = uc.markStepFinished(ctx, order.ID, updateCode, domain.StepStatusFailed, err.Error())
		return err
	}
	gitopsService, err := uc.resolveGitOpsService(ctx, argocdInstance)
	if err != nil {
		_ = uc.markStepFinished(ctx, order.ID, updateCode, domain.StepStatusFailed, "解析 GitOps 实例失败: "+err.Error())
		return err
	}
	valuesRules, imageVersion, err := decodeHelmDeploySnapshot(snapshot)
	if err != nil {
		_ = uc.markStepFinished(ctx, order.ID, updateCode, domain.StepStatusFailed, "解析历史部署快照失败: "+err.Error())
		return err
	}
	appKey := ""
	if uc.appRepo != nil {
		if appRecord, appErr := uc.appRepo.GetByID(ctx, strings.TrimSpace(order.ApplicationID)); appErr == nil {
			appKey = strings.TrimSpace(appRecord.Key)
		}
	}
	environment := firstNonEmpty(strings.TrimSpace(snapshot.EnvCode), uc.resolveArgoCDEnvironment(order, orderParams), strings.TrimSpace(order.EnvCode))
	appName := strings.TrimSpace(snapshot.ArgoCDAppName)
	if appName == "" {
		err = fmt.Errorf("%w: deploy snapshot argocd app name is empty", ErrInvalidInput)
		_ = uc.markStepFinished(ctx, order.ID, updateCode, domain.StepStatusFailed, err.Error())
		return err
	}
	commitFields := buildGitOpsCommitMessageFields(order, orderParams, appKey, environment, firstNonEmpty(imageVersion, strings.TrimSpace(order.ImageTag)), snapshot.SourcePath)
	commitMessage := gitopsService.BuildCommitMessage(commitFields)
	gitopsBranch := uc.resolveGitOpsBranchByApplication(ctx, order.ApplicationID, environment, argocdInstance, strings.TrimSpace(snapshot.Branch))

	_, changedFiles, commitSHA, changed, applyErr := gitopsService.ApplyValuesRules(
		ctx,
		strings.TrimSpace(snapshot.RepoURL),
		gitopsBranch,
		valuesRules,
		commitMessage,
	)
	if applyErr != nil {
		_ = uc.markStepFinished(ctx, order.ID, updateCode, domain.StepStatusFailed, "历史 Helm values 写回失败: "+applyErr.Error())
		return fmt.Errorf("%w: apply rollback values rules failed: %v", ErrInvalidInput, applyErr)
	}
	updateMessage := "历史 Helm values 已恢复"
	if !changed {
		updateMessage = "历史 Helm values 无变化"
	}
	if len(changedFiles) > 0 {
		updateMessage += fmt.Sprintf("，变更文件 %d 个", len(changedFiles))
	}
	_ = uc.markStepFinished(ctx, order.ID, updateCode, domain.StepStatusSuccess, updateMessage)
	if snapshotErr := uc.saveHelmDeploySnapshot(
		ctx,
		order,
		argocdInstance,
		appName,
		strings.TrimSpace(snapshot.RepoURL),
		gitopsBranch,
		strings.TrimSpace(snapshot.SourcePath),
		environment,
		firstNonEmpty(imageVersion, strings.TrimSpace(order.ImageTag)),
		valuesRules,
	); snapshotErr != nil {
		_ = uc.markStepFinished(ctx, order.ID, updateCode, domain.StepStatusFailed, "保存回滚部署快照失败: "+snapshotErr.Error())
		return fmt.Errorf("%w: save rollback deploy snapshot failed: %v", ErrInvalidInput, snapshotErr)
	}

	_ = uc.markStepRunning(ctx, order.ID, commitCode, "开始提交 Git 变更")
	if strings.TrimSpace(commitSHA) != "" && changed {
		_ = uc.markStepFinished(ctx, order.ID, commitCode, domain.StepStatusSuccess, "Git commit 成功，commit: "+strings.TrimSpace(commitSHA))
		_ = uc.markStepRunning(ctx, order.ID, pushCode, "开始推送 Git 变更")
		_ = uc.markStepFinished(ctx, order.ID, pushCode, domain.StepStatusSuccess, "Git push 成功，commit: "+strings.TrimSpace(commitSHA))
	} else {
		_ = uc.markStepFinished(ctx, order.ID, commitCode, domain.StepStatusSuccess, "历史 Helm values 无变化，无需生成新的 commit")
		_ = uc.markStepFinished(ctx, order.ID, pushCode, domain.StepStatusSuccess, "历史 Helm values 无变化，无需推送到远端仓库")
	}

	_ = uc.markStepRunning(ctx, order.ID, syncCode, "开始触发 ArgoCD Sync")
	if err := client.SyncApplicationWithRevision(ctx, appName, gitopsBranch); err != nil {
		_ = uc.markStepFinished(ctx, order.ID, syncCode, domain.StepStatusFailed, "触发 ArgoCD Sync 失败: "+err.Error())
		return fmt.Errorf("%w: trigger argocd sync failed: %v", ErrInvalidInput, err)
	}
	now := uc.now()
	if _, err := uc.repo.UpdateExecutionByScope(ctx, order.ID, execution.PipelineScope, domain.ExecutionUpdateInput{
		Status:        domain.ExecutionStatusRunning,
		ExternalRunID: commitSHA,
		StartedAt:     startedAt,
		UpdatedAt:     now,
	}); err != nil {
		return err
	}
	syncMessage := fmt.Sprintf("ArgoCD Sync 已触发，app: %s", appName)
	if strings.TrimSpace(commitSHA) != "" {
		syncMessage += "，commit: " + strings.TrimSpace(commitSHA)
	}
	_ = uc.markStepFinished(ctx, order.ID, syncCode, domain.StepStatusSuccess, syncMessage)
	_ = uc.markStepRunning(ctx, order.ID, healthCode, "ArgoCD Sync 已触发，等待健康检查回传")

	virtualBinding := pipelinedomain.PipelineBinding{
		ID:          execution.BindingID,
		Name:        firstNonEmpty(strings.TrimSpace(execution.BindingName), "ArgoCD"),
		Provider:    pipelinedomain.ProviderArgoCD,
		ExternalRef: appName,
	}
	if _, refreshErr := uc.refreshPipelineStages(ctx, order, execution, virtualBinding); refreshErr != nil {
	}
	return nil
}

// ensureArgoCDExecutionSteps 校验前置条件，不满足时写入对应错误响应。
func (uc *ReleaseOrderManager) ensureArgoCDExecutionSteps(
	ctx context.Context,
	order domain.ReleaseOrder,
	executions []domain.ReleaseOrderExecution,
	gitopsType domain.GitOpsType,
) error {
	steps, err := uc.repo.ListSteps(ctx, order.ID)
	if err != nil {
		return err
	}
	if findStepByCode(steps, scopeStepCode(domain.PipelineScopeCD, "gitops_update")) != nil {
		return nil
	}
	if findStepByCode(steps, scopeStepCode(domain.PipelineScopeCD, "trigger_pipeline")) == nil {
		return nil
	}

	rebuilt := defaultReleaseOrderSteps(order.ID, executions, uc.now(), gitopsType, nil, order.EnvCode)
	existingByCode := make(map[string]domain.ReleaseOrderStep, len(steps))
	for _, item := range steps {
		existingByCode[item.StepCode] = item
	}
	for idx := range rebuilt {
		if current, ok := existingByCode[rebuilt[idx].StepCode]; ok {
			rebuilt[idx].ID = current.ID
			rebuilt[idx].ExecutionID = firstNonEmpty(strings.TrimSpace(current.ExecutionID), strings.TrimSpace(rebuilt[idx].ExecutionID))
			rebuilt[idx].Status = current.Status
			rebuilt[idx].Message = current.Message
			rebuilt[idx].StartedAt = current.StartedAt
			rebuilt[idx].FinishedAt = current.FinishedAt
			rebuilt[idx].CreatedAt = current.CreatedAt
		}
	}
	return uc.repo.ReplaceSteps(ctx, order.ID, rebuilt)
}

// resolveExecutionBinding 解析上下文数据，得到后续流程需要的结果。
func (uc *ReleaseOrderManager) resolveExecutionBinding(
	ctx context.Context,
	order domain.ReleaseOrder,
	execution domain.ReleaseOrderExecution,
) (pipelinedomain.PipelineBinding, error) {
	bindingID := strings.TrimSpace(execution.BindingID)
	if bindingID != "" {
		binding, err := uc.pipelineRepo.GetBindingByID(ctx, bindingID)
		if err == nil {
			return binding, nil
		}
		if !strings.EqualFold(strings.TrimSpace(execution.Provider), string(pipelinedomain.ProviderArgoCD)) {
			return pipelinedomain.PipelineBinding{}, err
		}
	}
	if !strings.EqualFold(strings.TrimSpace(execution.Provider), string(pipelinedomain.ProviderArgoCD)) {
		return pipelinedomain.PipelineBinding{}, fmt.Errorf("%w: binding_id is required", ErrInvalidInput)
	}
	if uc.appRepo == nil {
		return pipelinedomain.PipelineBinding{}, fmt.Errorf("%w: application repository is not configured", ErrInvalidInput)
	}
	app, err := uc.appRepo.GetByID(ctx, strings.TrimSpace(order.ApplicationID))
	if err != nil {
		return pipelinedomain.PipelineBinding{}, err
	}
	externalRef, err := deriveArgoCDExternalRef(app)
	if err != nil {
		return pipelinedomain.PipelineBinding{}, err
	}
	// ArgoCD 已迁移到“模板直接启用”的模式后，CD 执行单元可能没有独立的绑定记录。
	// 这里兜底构造一个虚拟绑定，让执行、跟踪和进度模块仍然能复用统一逻辑。
	return pipelinedomain.PipelineBinding{
		ID:              bindingID,
		Name:            firstNonEmpty(strings.TrimSpace(execution.BindingName), "ArgoCD"),
		ApplicationID:   app.ID,
		ApplicationName: app.Name,
		BindingType:     pipelinedomain.BindingType(execution.PipelineScope),
		Provider:        pipelinedomain.ProviderArgoCD,
		ExternalRef:     externalRef,
		Status:          pipelinedomain.StatusActive,
	}, nil
}

// resolveArgoCDImageVersion 解析上下文数据，得到后续流程需要的结果。
func (uc *ReleaseOrderManager) resolveArgoCDImageVersion(
	order domain.ReleaseOrder,
	params []domain.ReleaseOrderParam,
	executions []domain.ReleaseOrderExecution,
) string {
	searchScopes := []domain.PipelineScope{domain.PipelineScopeCI, domain.PipelineScopeCD}
	for _, scope := range searchScopes {
		if value := findReleaseParamValue(params, scope, "image_version", "image_tag"); value != "" {
			return value
		}
	}
	if value := findReleaseParamValue(params, "", "image_version", "image_tag"); value != "" {
		return value
	}
	for _, item := range executions {
		if item.PipelineScope != domain.PipelineScopeCI {
			continue
		}
		if !strings.EqualFold(strings.TrimSpace(item.Provider), string(pipelinedomain.ProviderJenkins)) {
			continue
		}
		if buildNumber := parseJenkinsBuildNumber(item.BuildURL); buildNumber != "" {
			return buildNumber
		}
	}
	return strings.TrimSpace(order.ImageTag)
}

// resolveArgoCDEnvironment 解析上下文数据，得到后续流程需要的结果。
func (uc *ReleaseOrderManager) resolveArgoCDEnvironment(
	order domain.ReleaseOrder,
	params []domain.ReleaseOrderParam,
) string {
	searchScopes := []domain.PipelineScope{domain.PipelineScopeCD, domain.PipelineScopeCI}
	for _, scope := range searchScopes {
		if value := findReleaseParamValue(params, scope, "env", "env_code"); value != "" {
			return value
		}
	}
	if value := findReleaseParamValue(params, "", "env", "env_code"); value != "" {
		return value
	}
	return strings.TrimSpace(order.EnvCode)
}

// findReleaseParamValue 封装当前模块的业务处理逻辑。
func findReleaseParamValue(params []domain.ReleaseOrderParam, scope domain.PipelineScope, keys ...string) string {
	normalizedKeys := make([]string, 0, len(keys))
	for _, key := range keys {
		value := strings.ToLower(strings.TrimSpace(key))
		if value != "" {
			normalizedKeys = append(normalizedKeys, value)
		}
	}
	for _, item := range params {
		if scope != "" && item.PipelineScope != scope {
			continue
		}
		paramKey := strings.ToLower(strings.TrimSpace(item.ParamKey))
		for _, key := range normalizedKeys {
			if paramKey == key {
				value := strings.TrimSpace(item.ParamValue)
				if value != "" {
					return value
				}
			}
		}
	}
	return ""
}

// isArgoCDExecution 封装当前模块的业务处理逻辑。
func isArgoCDExecution(execution domain.ReleaseOrderExecution) bool {
	return strings.EqualFold(strings.TrimSpace(execution.Provider), string(pipelinedomain.ProviderArgoCD))
}

// buildGitOpsCommitMessageFields 组装业务执行所需的输入数据。
func buildGitOpsCommitMessageFields(
	order domain.ReleaseOrder,
	params []domain.ReleaseOrderParam,
	appKey string,
	environment string,
	imageVersion string,
	sourcePath string,
) map[string]string {
	branch := firstNonEmpty(
		findReleaseParamValue(params, domain.PipelineScopeCD, "branch", "git_ref"),
		findReleaseParamValue(params, domain.PipelineScopeCI, "branch", "git_ref"),
		findReleaseParamValue(params, "", "branch", "git_ref"),
		strings.TrimSpace(order.GitRef),
	)
	projectName := firstNonEmpty(
		findReleaseParamValue(params, domain.PipelineScopeCD, "project_name"),
		findReleaseParamValue(params, domain.PipelineScopeCI, "project_name"),
		findReleaseParamValue(params, "", "project_name"),
		strings.TrimSpace(order.SonService),
	)
	fields := map[string]string{
		"order_no":      strings.TrimSpace(order.OrderNo),
		"release_name":  strings.TrimSpace(order.ReleaseName),
		"app_name":      strings.TrimSpace(order.ApplicationName),
		"app_key":       strings.TrimSpace(appKey),
		"project_name":  projectName,
		"env":           strings.TrimSpace(environment),
		"branch":        branch,
		"git_ref":       branch,
		"image_version": strings.TrimSpace(imageVersion),
		"source_path":   strings.TrimSpace(sourcePath),
	}
	if value := findReleaseParamValue(params, domain.PipelineScopeCI, standardParamGOSArtifactURL); value != "" {
		fields[standardParamGOSArtifactURL] = value
	}
	for _, item := range params {
		paramKey := strings.ToLower(strings.TrimSpace(item.ParamKey))
		paramValue := strings.TrimSpace(item.ParamValue)
		if paramKey == "" || paramValue == "" {
			continue
		}
		if isCIOnlyStandardParamKey(paramKey) && item.PipelineScope != domain.PipelineScopeCI {
			continue
		}
		if _, exists := fields[paramKey]; exists && strings.TrimSpace(fields[paramKey]) != "" {
			continue
		}
		fields[paramKey] = paramValue
	}
	return fields
}

// buildArgoCDManifestRules 组装业务执行所需的输入数据。
func (uc *ReleaseOrderManager) buildArgoCDManifestRules(
	gitopsService GitOpsReleaseService,
	rules []domain.ReleaseTemplateGitOpsRule,
	fields map[string]string,
) ([]gitopsdomain.ManifestRule, error) {
	result := make([]gitopsdomain.ManifestRule, 0, len(rules))
	for _, item := range rules {
		sourceKey := strings.ToLower(strings.TrimSpace(item.SourceParamKey))
		if item.SourceFrom != domain.GitOpsRuleSourceCDInput && sourceKey != "" && strings.TrimSpace(fields[sourceKey]) == "" {
			return nil, fmt.Errorf("%w: gitops 替换规则缺少取值字段 %s", ErrInvalidInput, sourceKey)
		}
		locatorKey := strings.ToLower(strings.TrimSpace(item.LocatorParamKey))
		if locatorKey != "" && strings.TrimSpace(fields[locatorKey]) == "" {
			return nil, fmt.Errorf("%w: gitops 替换规则缺少定位字段 %s", ErrInvalidInput, locatorKey)
		}

		filePath := strings.TrimSpace(gitopsService.RenderTemplate(strings.TrimSpace(item.FilePathTemplate), fields))
		documentName := strings.TrimSpace(gitopsService.RenderTemplate(strings.TrimSpace(item.DocumentName), fields))
		value := strings.TrimSpace(gitopsService.RenderTemplate(strings.TrimSpace(item.ValueTemplate), fields))
		if filePath == "" {
			return nil, fmt.Errorf("%w: gitops 替换规则缺少目标文件路径", ErrInvalidInput)
		}
		if value == "" {
			return nil, fmt.Errorf("%w: gitops 替换规则字段 %s 未取到值", ErrInvalidInput, firstNonEmpty(item.SourceParamName, item.SourceParamKey))
		}
		if unresolvedGitOpsTemplate(filePath) || unresolvedGitOpsTemplate(documentName) || unresolvedGitOpsTemplate(value) {
			return nil, fmt.Errorf("%w: gitops 替换规则仍存在未解析占位符，请检查字段映射", ErrInvalidInput)
		}
		result = append(result, gitopsdomain.ManifestRule{
			FilePath:     normalizeArgoCDSourcePath(filePath),
			DocumentKind: strings.TrimSpace(item.DocumentKind),
			DocumentName: documentName,
			TargetPath:   strings.TrimSpace(item.TargetPath),
			Value:        value,
		})
	}
	return result, nil
}

// buildArgoCDValuesRules 组装业务执行所需的输入数据。
func (uc *ReleaseOrderManager) buildArgoCDValuesRules(
	gitopsService GitOpsReleaseService,
	rules []domain.ReleaseTemplateGitOpsRule,
	fields map[string]string,
) ([]gitopsdomain.ValuesRule, error) {
	result := make([]gitopsdomain.ValuesRule, 0, len(rules))
	for _, item := range rules {
		sourceKey := strings.ToLower(strings.TrimSpace(item.SourceParamKey))
		if item.SourceFrom != domain.GitOpsRuleSourceCDInput && sourceKey != "" && strings.TrimSpace(fields[sourceKey]) == "" {
			return nil, fmt.Errorf("%w: gitops 替换规则缺少取值字段 %s", ErrInvalidInput, sourceKey)
		}
		locatorKey := strings.ToLower(strings.TrimSpace(item.LocatorParamKey))
		if locatorKey != "" && strings.TrimSpace(fields[locatorKey]) == "" {
			return nil, fmt.Errorf("%w: gitops 替换规则缺少定位字段 %s", ErrInvalidInput, locatorKey)
		}

		filePath := strings.TrimSpace(gitopsService.RenderTemplate(strings.TrimSpace(item.FilePathTemplate), fields))
		value := strings.TrimSpace(gitopsService.RenderTemplate(strings.TrimSpace(item.ValueTemplate), fields))
		if filePath == "" {
			return nil, fmt.Errorf("%w: gitops 替换规则缺少目标 values 文件路径", ErrInvalidInput)
		}
		if value == "" {
			return nil, fmt.Errorf("%w: gitops 替换规则字段 %s 未取到值", ErrInvalidInput, firstNonEmpty(item.SourceParamName, item.SourceParamKey))
		}
		if unresolvedGitOpsTemplate(filePath) || unresolvedGitOpsTemplate(value) {
			return nil, fmt.Errorf("%w: gitops 替换规则仍存在未解析占位符，请检查字段映射", ErrInvalidInput)
		}
		targetPath := strings.TrimSpace(item.TargetPath)
		if targetPath == "" {
			return nil, fmt.Errorf("%w: gitops 替换规则缺少 values_path", ErrInvalidInput)
		}
		result = append(result, gitopsdomain.ValuesRule{
			FilePath:   normalizeArgoCDSourcePath(filePath),
			TargetPath: targetPath,
			Value:      value,
		})
	}
	return result, nil
}

// unresolvedGitOpsTemplate 解析上下文数据，得到后续流程需要的结果。
func unresolvedGitOpsTemplate(value string) bool {
	return gitopsTemplatePlaceholderPattern.MatchString(strings.TrimSpace(value))
}

// saveHelmDeploySnapshot 封装当前模块的业务处理逻辑。
func (uc *ReleaseOrderManager) saveHelmDeploySnapshot(
	ctx context.Context,
	order domain.ReleaseOrder,
	argocdInstance argocddomain.Instance,
	appName string,
	repoURL string,
	branch string,
	sourcePath string,
	envCode string,
	imageVersion string,
	rules []gitopsdomain.ValuesRule,
) error {
	payload, err := encodeHelmDeploySnapshot(imageVersion, rules)
	if err != nil {
		return err
	}
	return uc.repo.CreateDeploySnapshot(ctx, domain.DeploySnapshot{
		ID:               generateID("rds"),
		ReleaseOrderID:   order.ID,
		Provider:         string(pipelinedomain.ProviderArgoCD),
		GitOpsType:       domain.GitOpsTypeHelm,
		ArgoCDInstanceID: strings.TrimSpace(argocdInstance.ID),
		GitOpsInstanceID: strings.TrimSpace(argocdInstance.GitOpsInstanceID),
		ArgoCDAppName:    strings.TrimSpace(appName),
		RepoURL:          strings.TrimSpace(repoURL),
		Branch:           strings.TrimSpace(branch),
		SourcePath:       strings.TrimSpace(sourcePath),
		EnvCode:          strings.TrimSpace(envCode),
		SnapshotPayload:  payload,
		CreatedAt:        uc.now(),
	})
}

// encodeHelmDeploySnapshot 封装当前模块的业务处理逻辑。
func encodeHelmDeploySnapshot(imageVersion string, rules []gitopsdomain.ValuesRule) (string, error) {
	payload := helmDeploySnapshotPayload{
		ImageVersion: strings.TrimSpace(imageVersion),
		Rules:        make([]helmDeploySnapshotRule, 0, len(rules)),
	}
	for _, item := range rules {
		payload.Rules = append(payload.Rules, helmDeploySnapshotRule{
			FilePath:   strings.TrimSpace(item.FilePath),
			TargetPath: strings.TrimSpace(item.TargetPath),
			Value:      strings.TrimSpace(item.Value),
		})
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

// decodeHelmDeploySnapshot 封装当前模块的业务处理逻辑。
func decodeHelmDeploySnapshot(snapshot domain.DeploySnapshot) ([]gitopsdomain.ValuesRule, string, error) {
	var payload helmDeploySnapshotPayload
	if err := json.Unmarshal([]byte(strings.TrimSpace(snapshot.SnapshotPayload)), &payload); err != nil {
		return nil, "", err
	}
	rules := make([]gitopsdomain.ValuesRule, 0, len(payload.Rules))
	for _, item := range payload.Rules {
		filePath := strings.TrimSpace(item.FilePath)
		targetPath := strings.TrimSpace(item.TargetPath)
		if filePath == "" || targetPath == "" {
			continue
		}
		rules = append(rules, gitopsdomain.ValuesRule{
			FilePath:   filePath,
			TargetPath: targetPath,
			Value:      strings.TrimSpace(item.Value),
		})
	}
	if len(rules) == 0 {
		return nil, "", fmt.Errorf("%w: deploy snapshot payload is empty", ErrInvalidInput)
	}
	return rules, strings.TrimSpace(payload.ImageVersion), nil
}

// resolveArgoCDExecutionContext 解析上下文数据，得到后续流程需要的结果。
func (uc *ReleaseOrderManager) resolveArgoCDExecutionContext(
	ctx context.Context,
	order domain.ReleaseOrder,
	execution domain.ReleaseOrderExecution,
	orderParams []domain.ReleaseOrderParam,
) (pipelinedomain.PipelineBinding, argocddomain.Instance, ArgoCDApplicationClient, error) {
	binding, err := uc.resolveExecutionBinding(ctx, order, execution)
	if err != nil {
		return pipelinedomain.PipelineBinding{}, argocddomain.Instance{}, nil, err
	}
	instance, err := uc.resolveArgoCDInstance(ctx, order, orderParams)
	if err != nil {
		return pipelinedomain.PipelineBinding{}, argocddomain.Instance{}, nil, err
	}
	client := uc.argocdFactory.Build(instance)
	if client == nil {
		return pipelinedomain.PipelineBinding{}, argocddomain.Instance{}, nil, fmt.Errorf("%w: argocd client is not configured", ErrInvalidInput)
	}
	logx.Info("argocd_cd", "resolved_instance",
		logx.F("order_id", order.ID),
		logx.F("binding_id", binding.ID),
		logx.F("binding_name", binding.Name),
		logx.F("argocd_instance_id", instance.ID),
		logx.F("argocd_instance_code", instance.InstanceCode),
		logx.F("env_code", order.EnvCode),
	)
	return binding, instance, client, nil
}

// resolveArgoCDInstance 解析上下文数据，得到后续流程需要的结果。
func (uc *ReleaseOrderManager) resolveArgoCDInstance(
	ctx context.Context,
	order domain.ReleaseOrder,
	orderParams []domain.ReleaseOrderParam,
) (argocddomain.Instance, error) {
	if uc.argocdRepo == nil {
		return argocddomain.Instance{}, fmt.Errorf("%w: argocd repository is not configured", ErrInvalidInput)
	}
	envCode := uc.resolveArgoCDEnvironment(order, orderParams)
	if envCode == "" {
		envCode = strings.TrimSpace(order.EnvCode)
	}
	if envCode == "" {
		return argocddomain.Instance{}, fmt.Errorf("%w: env_code is required to resolve argocd instance", ErrInvalidInput)
	}
	instance, err := uc.argocdRepo.ResolveInstanceByEnv(ctx, envCode)
	if err != nil {
		return argocddomain.Instance{}, err
	}
	return instance, nil
}

// resolveGitOpsService 解析上下文数据，得到后续流程需要的结果。
func (uc *ReleaseOrderManager) resolveGitOpsService(
	ctx context.Context,
	argocdInstance argocddomain.Instance,
) (GitOpsReleaseService, error) {
	gitopsInstanceID := strings.TrimSpace(argocdInstance.GitOpsInstanceID)
	if gitopsInstanceID != "" && uc.gitopsRepo != nil && uc.gitopsFactory != nil {
		item, err := uc.gitopsRepo.GetInstanceByID(ctx, gitopsInstanceID)
		if err != nil {
			return nil, err
		}
		service := uc.gitopsFactory.Build(item)
		if service == nil {
			return nil, fmt.Errorf("%w: gitops service factory is not configured", ErrInvalidInput)
		}
		logx.Info("argocd_cd", "resolved_gitops_instance",
			logx.F("argocd_instance_id", argocdInstance.ID),
			logx.F("argocd_instance_code", argocdInstance.InstanceCode),
			logx.F("gitops_instance_id", item.ID),
			logx.F("gitops_instance_code", item.InstanceCode),
		)
		return service, nil
	}
	return nil, fmt.Errorf("%w: argocd instance %s is missing a bound gitops instance", ErrInvalidInput, strings.TrimSpace(argocdInstance.InstanceCode))
}

// resolveArgoCDApplicationByRef 兼容两种 external_ref：
// 1. 历史数据：直接填写 ArgoCD Application 名称；
// 2. 当前推荐：选择 GitOps 应用目录（apps/<应用目录>）。
//
// 当 external_ref 是 GitOps 应用目录时，平台会结合标准 Key `env`，
// 自动拼成 apps/<应用目录>/overlays/<env> 来定位真正的 ArgoCD Application。
func resolveArgoCDApplicationByRef(
	ctx context.Context,
	client ArgoCDApplicationClient,
	externalRef string,
	environment string,
	gitopsType domain.GitOpsType,
) (string, ArgoCDApplicationSnapshot, error) {
	ref := strings.TrimSpace(externalRef)
	if ref == "" {
		return "", nil, fmt.Errorf("%w: cd argocd binding requires external_ref", ErrInvalidInput)
	}
	app, err := client.GetApplication(ctx, ref)
	if err == nil {
		return ref, app, nil
	}
	if !isResourceNotFoundError(err) {
		return "", nil, err
	}
	normalizedRef := normalizeArgoCDSourcePath(ref)
	candidatePaths := buildArgoCDSourcePathCandidates(normalizedRef, environment, gitopsType)
	if len(candidatePaths) == 0 {
		return "", nil, fmt.Errorf("%w: env is required when cd argocd binding uses gitops application directory", ErrInvalidInput)
	}

	items, listErr := client.ListApplications(ctx)
	if listErr != nil {
		return "", nil, listErr
	}
	for _, targetPath := range candidatePaths {
		matched := make([]ArgoCDApplicationSnapshot, 0)
		for _, item := range items {
			if normalizeArgoCDSourcePath(item.GetSourcePath()) == targetPath {
				matched = append(matched, item)
			}
		}
		if len(matched) == 1 {
			return strings.TrimSpace(matched[0].GetName()), matched[0], nil
		}
		if len(matched) > 1 {
			return "", nil, fmt.Errorf("%w: multiple argocd applications match source path %s", ErrInvalidInput, targetPath)
		}
	}
	return "", nil, err
}

// buildArgoCDSourcePathCandidates 组装业务执行所需的输入数据。
func buildArgoCDSourcePathCandidates(ref string, environment string, gitopsType domain.GitOpsType) []string {
	ref = normalizeArgoCDSourcePath(ref)
	if ref == "" {
		return nil
	}
	candidates := make([]string, 0, 2)
	switch normalizeTemplateGitOpsType(gitopsType, true) {
	case domain.GitOpsTypeHelm:
		if strings.HasSuffix(ref, "/helm") {
			candidates = append(candidates, ref)
		} else {
			if parent := normalizeArgoCDSourcePath(path.Dir(ref)); parent != "." && parent != "" {
				candidates = append(candidates, normalizeArgoCDSourcePath(parent+"/helm"))
			}
			candidates = append(candidates, normalizeArgoCDSourcePath(ref+"/helm"))
		}
	default:
		if strings.Contains(ref, "/overlays/") {
			candidates = append(candidates, ref)
		} else if env := strings.Trim(strings.TrimSpace(environment), "/"); env != "" {
			candidates = append(candidates, normalizeArgoCDSourcePath(ref+"/overlays/"+env))
		}
	}
	candidates = append(candidates, ref)
	return uniqueStringSlice(candidates)
}

// uniqueStringSlice 封装当前模块的业务处理逻辑。
func uniqueStringSlice(values []string) []string {
	result := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, item := range values {
		value := strings.TrimSpace(item)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

// normalizeArgoCDSourcePath 标准化输入值，保证后续逻辑使用统一格式。
func normalizeArgoCDSourcePath(value string) string {
	normalized := strings.TrimSpace(value)
	normalized = strings.TrimPrefix(normalized, "./")
	normalized = strings.TrimPrefix(normalized, "/")
	return strings.TrimSuffix(normalized, "/")
}

// resolveGitOpsTargetBranch 解析上下文数据，得到后续流程需要的结果。
func (uc *ReleaseOrderManager) resolveGitOpsTargetBranch(
	ctx context.Context,
	order domain.ReleaseOrder,
	orderParams []domain.ReleaseOrderParam,
	argocdInstance argocddomain.Instance,
	app ArgoCDApplicationSnapshot,
) string {
	environment := uc.resolveArgoCDEnvironment(order, orderParams)
	return uc.resolveGitOpsBranchByApplication(ctx, order.ApplicationID, environment, argocdInstance, strings.TrimSpace(app.GetTargetRevision()))
}

// resolveGitOpsBranchByEnv 解析上下文数据，得到后续流程需要的结果。
func (uc *ReleaseOrderManager) resolveGitOpsBranchByEnv(
	environment string,
	_ argocddomain.Instance,
	fallback string,
) string {
	envBranch := strings.TrimSpace(environment)
	if envBranch != "" {
		return envBranch
	}
	revision := strings.TrimSpace(fallback)
	if revision != "" && !strings.EqualFold(revision, "HEAD") {
		return revision
	}
	return firstNonEmpty(revision, "master")
}

// resolveGitOpsBranchByApplication 解析上下文数据，得到后续流程需要的结果。
func (uc *ReleaseOrderManager) resolveGitOpsBranchByApplication(
	ctx context.Context,
	applicationID string,
	environment string,
	argocdInstance argocddomain.Instance,
	fallback string,
) string {
	environment = strings.TrimSpace(environment)
	if uc != nil && uc.appRepo != nil && strings.TrimSpace(applicationID) != "" {
		if app, err := uc.appRepo.GetByID(ctx, strings.TrimSpace(applicationID)); err == nil {
			if branch := resolveGitOpsBranchFromMappings(app.GitOpsBranchMappings, environment); branch != "" {
				return branch
			}
			if appKey := strings.TrimSpace(app.Key); appKey != "" && environment != "" {
				return fmt.Sprintf("%s-%s", appKey, environment)
			}
		}
	}
	return uc.resolveGitOpsBranchByEnv(environment, argocdInstance, fallback)
}

// resolveGitOpsBranchFromMappings 解析上下文数据，得到后续流程需要的结果。
func resolveGitOpsBranchFromMappings(items []appdomain.GitOpsBranchMapping, environment string) string {
	envCode := strings.ToLower(strings.TrimSpace(environment))
	if envCode == "" {
		return ""
	}
	for _, item := range items {
		if strings.ToLower(strings.TrimSpace(item.EnvCode)) != envCode {
			continue
		}
		if branch := strings.TrimSpace(item.Branch); branch != "" {
			return branch
		}
	}
	return ""
}
