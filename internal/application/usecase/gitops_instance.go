package usecase

import (
	"context"
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	domain "gos/internal/domain/gitops"
	gitopsinfra "gos/internal/infrastructure/gitops"
	"gos/internal/support/logx"
)

var gitopsInstanceCodePattern = regexp.MustCompile(`^[a-z][a-z0-9_-]*$`)

type GitOpsServiceFactory interface {
	Build(instance domain.Instance) *gitopsinfra.Service
}

type GitOpsInstanceManager struct {
	repo         domain.Repository
	factory      GitOpsServiceFactory
	platformRepo GitOpsCommitTemplateFieldReader
	now          func() time.Time
}

type CreateGitOpsInstanceInput struct {
	InstanceCode          string
	Name                  string
	LocalRoot             string
	DefaultBranch         string
	Username              string
	Password              string
	Token                 string
	AuthorName            string
	AuthorEmail           string
	CommitMessageTemplate string
	CommandTimeoutSec     int
	Status                domain.Status
	Remark                string
}

type UpdateGitOpsInstanceInput = CreateGitOpsInstanceInput

// NewGitOpsInstanceManager 创建并返回对应组件实例。
func NewGitOpsInstanceManager(repo domain.Repository, factory GitOpsServiceFactory, platformRepo GitOpsCommitTemplateFieldReader) *GitOpsInstanceManager {
	return &GitOpsInstanceManager{repo: repo, factory: factory, platformRepo: platformRepo, now: func() time.Time { return time.Now().UTC() }}
}

// List 查询并返回列表数据。
func (uc *GitOpsInstanceManager) List(ctx context.Context, filter domain.InstanceListFilter) ([]domain.Instance, int64, error) {
	if uc == nil || uc.repo == nil {
		return nil, 0, fmt.Errorf("%w: gitops instance manager is not configured", ErrInvalidInput)
	}
	if filter.Status != "" && !filter.Status.Valid() {
		return nil, 0, ErrInvalidStatus
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
	return uc.repo.ListInstances(ctx, filter)
}

// ListActive 查询并返回列表数据。
func (uc *GitOpsInstanceManager) ListActive(ctx context.Context) ([]domain.Instance, error) {
	if uc == nil || uc.repo == nil {
		return nil, fmt.Errorf("%w: gitops instance manager is not configured", ErrInvalidInput)
	}
	return uc.repo.ListActiveInstances(ctx)
}

// Create 创建业务资源并返回处理结果。
func (uc *GitOpsInstanceManager) Create(ctx context.Context, input CreateGitOpsInstanceInput) (domain.Instance, error) {
	if uc == nil || uc.repo == nil {
		return domain.Instance{}, fmt.Errorf("%w: gitops instance manager is not configured", ErrInvalidInput)
	}
	logx.Info("gitops_instance", "create_start",
		logx.F("instance_code", input.InstanceCode),
		logx.F("name", input.Name),
		logx.F("local_root", input.LocalRoot),
		logx.F("default_branch", input.DefaultBranch),
		logx.F("status", input.Status),
	)
	item, err := uc.normalizeCreateInput(ctx, input)
	if err != nil {
		logx.Error("gitops_instance", "create_failed", err,
			logx.F("instance_code", input.InstanceCode),
			logx.F("name", input.Name),
		)
		return domain.Instance{}, err
	}
	created, err := uc.repo.CreateInstance(ctx, item)
	if err != nil {
		logx.Error("gitops_instance", "create_failed", err,
			logx.F("instance_id", item.ID),
			logx.F("instance_code", item.InstanceCode),
			logx.F("name", item.Name),
		)
		return domain.Instance{}, err
	}
	logx.Info("gitops_instance", "create_success",
		logx.F("instance_id", created.ID),
		logx.F("instance_code", created.InstanceCode),
		logx.F("name", created.Name),
		logx.F("local_root", created.LocalRoot),
	)
	return created, nil
}

// Update 更新业务资源并返回处理结果。
func (uc *GitOpsInstanceManager) Update(ctx context.Context, id string, input UpdateGitOpsInstanceInput) (domain.Instance, error) {
	if uc == nil || uc.repo == nil {
		return domain.Instance{}, fmt.Errorf("%w: gitops instance manager is not configured", ErrInvalidInput)
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return domain.Instance{}, ErrInvalidID
	}
	logx.Info("gitops_instance", "update_start",
		logx.F("instance_id", id),
		logx.F("instance_code", input.InstanceCode),
		logx.F("name", input.Name),
		logx.F("local_root", input.LocalRoot),
		logx.F("default_branch", input.DefaultBranch),
		logx.F("status", input.Status),
	)
	current, err := uc.repo.GetInstanceByID(ctx, id)
	if err != nil {
		logx.Error("gitops_instance", "update_failed", err, logx.F("instance_id", id))
		return domain.Instance{}, err
	}
	item, err := uc.normalizeUpdateInput(ctx, current, input)
	if err != nil {
		logx.Error("gitops_instance", "update_failed", err,
			logx.F("instance_id", id),
			logx.F("instance_code", current.InstanceCode),
			logx.F("name", current.Name),
		)
		return domain.Instance{}, err
	}
	updated, err := uc.repo.UpdateInstance(ctx, item)
	if err != nil {
		logx.Error("gitops_instance", "update_failed", err,
			logx.F("instance_id", item.ID),
			logx.F("instance_code", item.InstanceCode),
			logx.F("name", item.Name),
		)
		return domain.Instance{}, err
	}
	logx.Info("gitops_instance", "update_success",
		logx.F("instance_id", updated.ID),
		logx.F("instance_code", updated.InstanceCode),
		logx.F("name", updated.Name),
		logx.F("local_root", updated.LocalRoot),
		logx.F("status", updated.Status),
	)
	return updated, nil
}

// GetStatus 查询并返回指定资源数据。
func (uc *GitOpsInstanceManager) GetStatus(ctx context.Context, id string) (gitopsinfra.Status, domain.Instance, error) {
	if uc == nil || uc.repo == nil || uc.factory == nil {
		return gitopsinfra.Status{}, domain.Instance{}, fmt.Errorf("%w: gitops instance manager is not configured", ErrInvalidInput)
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return gitopsinfra.Status{}, domain.Instance{}, ErrInvalidID
	}
	logx.Info("gitops_instance", "status_check_start", logx.F("instance_id", id))
	item, err := uc.repo.GetInstanceByID(ctx, id)
	if err != nil {
		logx.Error("gitops_instance", "status_check_failed", err, logx.F("instance_id", id))
		return gitopsinfra.Status{}, domain.Instance{}, err
	}
	service := uc.factory.Build(item)
	if service == nil {
		err := fmt.Errorf("%w: gitops service factory is not configured", ErrInvalidInput)
		logx.Error("gitops_instance", "status_check_failed", err,
			logx.F("instance_id", item.ID),
			logx.F("instance_code", item.InstanceCode),
		)
		return gitopsinfra.Status{}, domain.Instance{}, err
	}
	status, err := service.GetStatus(ctx)
	if err != nil {
		logx.Error("gitops_instance", "status_check_failed", err,
			logx.F("instance_id", item.ID),
			logx.F("instance_code", item.InstanceCode),
			logx.F("local_root", item.LocalRoot),
		)
		return gitopsinfra.Status{}, domain.Instance{}, err
	}
	logx.Info("gitops_instance", "status_check_success",
		logx.F("instance_id", item.ID),
		logx.F("instance_code", item.InstanceCode),
		logx.F("local_root", item.LocalRoot),
		logx.F("path_exists", status.PathExists),
		logx.F("is_git_repo", status.IsGitRepo),
		logx.F("remote_reachable", status.RemoteReachable),
	)
	return status, item, err
}

// BuildServiceByID 组装业务执行所需的输入数据。
func (uc *GitOpsInstanceManager) BuildServiceByID(ctx context.Context, id string) (domain.Instance, *gitopsinfra.Service, error) {
	if uc == nil || uc.repo == nil || uc.factory == nil {
		return domain.Instance{}, nil, fmt.Errorf("%w: gitops instance manager is not configured", ErrInvalidInput)
	}
	item, err := uc.repo.GetInstanceByID(ctx, strings.TrimSpace(id))
	if err != nil {
		return domain.Instance{}, nil, err
	}
	service := uc.factory.Build(item)
	if service == nil {
		return domain.Instance{}, nil, fmt.Errorf("%w: gitops service factory is not configured", ErrInvalidInput)
	}
	return item, service, nil
}

// ListFieldCandidates 查询并返回列表数据。
func (uc *GitOpsInstanceManager) ListFieldCandidates(ctx context.Context, appKey string) ([]domain.FieldCandidate, error) {
	if uc == nil || uc.repo == nil || uc.factory == nil {
		return nil, fmt.Errorf("%w: gitops instance manager is not configured", ErrInvalidInput)
	}
	appKey = strings.TrimSpace(appKey)
	if appKey == "" {
		return []domain.FieldCandidate{}, nil
	}
	instances, err := uc.repo.ListActiveInstances(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]domain.FieldCandidate, 0)
	seen := make(map[string]struct{})
	var firstErr error
	for _, item := range instances {
		service := uc.factory.Build(item)
		if service == nil {
			continue
		}
		candidates, scanErr := service.ListFieldCandidates(ctx, appKey)
		if scanErr != nil {
			logx.Warn("gitops_instance", "list_field_candidates_scan_failed",
				logx.F("instance_id", item.ID),
				logx.F("instance_code", item.InstanceCode),
				logx.F("app_key", appKey),
				logx.F("reason", scanErr.Error()),
			)
			if firstErr == nil {
				firstErr = scanErr
			}
			continue
		}
		for _, candidate := range candidates {
			key := strings.Join([]string{
				strings.TrimSpace(candidate.FilePathTemplate),
				strings.TrimSpace(candidate.DocumentKind),
				strings.TrimSpace(candidate.DocumentName),
				strings.TrimSpace(candidate.TargetPath),
			}, "::")
			if _, exists := seen[key]; exists {
				continue
			}
			seen[key] = struct{}{}
			result = append(result, candidate)
		}
	}
	if len(result) == 0 && firstErr != nil {
		return nil, firstErr
	}
	sort.Slice(result, func(i, j int) bool {
		left := strings.Join([]string{result[i].FilePathTemplate, result[i].DocumentKind, result[i].DocumentName, result[i].TargetPath}, "::")
		right := strings.Join([]string{result[j].FilePathTemplate, result[j].DocumentKind, result[j].DocumentName, result[j].TargetPath}, "::")
		return left < right
	})
	logx.Info("gitops_instance", "list_field_candidates_success",
		logx.F("app_key", appKey),
		logx.F("instances_count", len(instances)),
		logx.F("candidates_count", len(result)),
	)
	return result, nil
}

// ListValuesCandidates 查询并返回列表数据。
func (uc *GitOpsInstanceManager) ListValuesCandidates(ctx context.Context, appKey string) ([]domain.ValuesCandidate, error) {
	if uc == nil || uc.repo == nil || uc.factory == nil {
		return nil, fmt.Errorf("%w: gitops instance manager is not configured", ErrInvalidInput)
	}
	appKey = strings.TrimSpace(appKey)
	if appKey == "" {
		return []domain.ValuesCandidate{}, nil
	}
	instances, err := uc.repo.ListActiveInstances(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]domain.ValuesCandidate, 0)
	seen := make(map[string]struct{})
	var firstErr error
	for _, item := range instances {
		service := uc.factory.Build(item)
		if service == nil {
			continue
		}
		candidates, scanErr := service.ListValuesCandidates(ctx, appKey)
		if scanErr != nil {
			logx.Warn("gitops_instance", "list_values_candidates_scan_failed",
				logx.F("instance_id", item.ID),
				logx.F("instance_code", item.InstanceCode),
				logx.F("app_key", appKey),
				logx.F("reason", scanErr.Error()),
			)
			if firstErr == nil {
				firstErr = scanErr
			}
			continue
		}
		for _, candidate := range candidates {
			key := strings.Join([]string{
				strings.TrimSpace(candidate.FilePathTemplate),
				strings.TrimSpace(candidate.TargetPath),
			}, "::")
			if _, exists := seen[key]; exists {
				continue
			}
			seen[key] = struct{}{}
			result = append(result, candidate)
		}
	}
	if len(result) == 0 && firstErr != nil {
		return nil, firstErr
	}
	sort.Slice(result, func(i, j int) bool {
		left := strings.Join([]string{result[i].FilePathTemplate, result[i].TargetPath}, "::")
		right := strings.Join([]string{result[j].FilePathTemplate, result[j].TargetPath}, "::")
		return left < right
	})
	logx.Info("gitops_instance", "list_values_candidates_success",
		logx.F("app_key", appKey),
		logx.F("instances_count", len(instances)),
		logx.F("candidates_count", len(result)),
	)
	return result, nil
}

// CheckScanPath 检查业务状态并返回校验结果。
func (uc *GitOpsInstanceManager) CheckScanPath(ctx context.Context, appKey string, gitopsType string) (string, bool, error) {
	if uc == nil || uc.repo == nil || uc.factory == nil {
		return "", false, fmt.Errorf("%w: gitops instance manager is not configured", ErrInvalidInput)
	}
	instances, err := uc.repo.ListActiveInstances(ctx)
	if err != nil {
		return "", false, err
	}
	for _, item := range instances {
		service := uc.factory.Build(item)
		if service == nil {
			continue
		}
		pathTemplate, exists, scanErr := service.CheckScanPath(ctx, appKey, gitopsType)
		if scanErr != nil {
			logx.Warn("gitops_instance", "check_scan_path_failed",
				logx.F("instance_id", item.ID),
				logx.F("instance_code", item.InstanceCode),
				logx.F("app_key", appKey),
				logx.F("gitops_type", gitopsType),
				logx.F("reason", scanErr.Error()),
			)
			continue
		}
		return pathTemplate, exists, nil
	}
	return "", false, nil
}

// normalizeCreateInput 标准化输入值，保证后续逻辑使用统一格式。
func (uc *GitOpsInstanceManager) normalizeCreateInput(ctx context.Context, input CreateGitOpsInstanceInput) (domain.Instance, error) {
	code, err := normalizeGitOpsInstanceCode(input.InstanceCode)
	if err != nil {
		return domain.Instance{}, err
	}
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return domain.Instance{}, fmt.Errorf("%w: name is required", ErrInvalidInput)
	}
	localRoot, err := normalizeGitOpsLocalRoot(input.LocalRoot)
	if err != nil {
		return domain.Instance{}, err
	}
	defaultBranch := strings.TrimSpace(input.DefaultBranch)
	if defaultBranch == "" {
		defaultBranch = "master"
	}
	authorName := strings.TrimSpace(input.AuthorName)
	if authorName == "" {
		authorName = "gos-bot"
	}
	authorEmail := strings.TrimSpace(input.AuthorEmail)
	if authorEmail == "" {
		authorEmail = "gos@example.com"
	}
	commandTimeoutSec := input.CommandTimeoutSec
	if commandTimeoutSec <= 0 {
		commandTimeoutSec = 30
	}
	status := input.Status
	if status == "" {
		status = domain.StatusActive
	}
	if !status.Valid() {
		return domain.Instance{}, ErrInvalidStatus
	}
	now := uc.now()
	commitMessageTemplate, err := validateGitOpsCommitMessageTemplate(ctx, uc.platformRepo, input.CommitMessageTemplate)
	if err != nil {
		return domain.Instance{}, err
	}
	return domain.Instance{
		ID:                    generateID("gitops"),
		InstanceCode:          code,
		Name:                  name,
		LocalRoot:             localRoot,
		DefaultBranch:         defaultBranch,
		Username:              strings.TrimSpace(input.Username),
		Password:              strings.TrimSpace(input.Password),
		Token:                 strings.TrimSpace(input.Token),
		AuthorName:            authorName,
		AuthorEmail:           authorEmail,
		CommitMessageTemplate: commitMessageTemplate,
		CommandTimeoutSec:     commandTimeoutSec,
		Status:                status,
		Remark:                strings.TrimSpace(input.Remark),
		CreatedAt:             now,
		UpdatedAt:             now,
	}, nil
}

// normalizeUpdateInput 标准化输入值，保证后续逻辑使用统一格式。
func (uc *GitOpsInstanceManager) normalizeUpdateInput(ctx context.Context, current domain.Instance, input UpdateGitOpsInstanceInput) (domain.Instance, error) {
	code, err := normalizeGitOpsInstanceCode(input.InstanceCode)
	if err != nil {
		return domain.Instance{}, err
	}
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return domain.Instance{}, fmt.Errorf("%w: name is required", ErrInvalidInput)
	}
	localRoot, err := normalizeGitOpsLocalRoot(input.LocalRoot)
	if err != nil {
		return domain.Instance{}, err
	}
	defaultBranch := strings.TrimSpace(input.DefaultBranch)
	if defaultBranch == "" {
		defaultBranch = current.DefaultBranch
	}
	if defaultBranch == "" {
		defaultBranch = "master"
	}
	authorName := strings.TrimSpace(input.AuthorName)
	if authorName == "" {
		authorName = current.AuthorName
	}
	if authorName == "" {
		authorName = "gos-bot"
	}
	authorEmail := strings.TrimSpace(input.AuthorEmail)
	if authorEmail == "" {
		authorEmail = current.AuthorEmail
	}
	if authorEmail == "" {
		authorEmail = "gos@example.com"
	}
	commandTimeoutSec := input.CommandTimeoutSec
	if commandTimeoutSec <= 0 {
		commandTimeoutSec = current.CommandTimeoutSec
	}
	if commandTimeoutSec <= 0 {
		commandTimeoutSec = 30
	}
	status := input.Status
	if status == "" {
		status = current.Status
	}
	if !status.Valid() {
		return domain.Instance{}, ErrInvalidStatus
	}
	password := strings.TrimSpace(input.Password)
	if password == "" {
		password = current.Password
	}
	token := strings.TrimSpace(input.Token)
	if token == "" {
		token = current.Token
	}
	username := strings.TrimSpace(input.Username)
	if username == "" {
		username = current.Username
	}
	commitMessageTemplate, err := validateGitOpsCommitMessageTemplate(ctx, uc.platformRepo, firstNonEmpty(strings.TrimSpace(input.CommitMessageTemplate), current.CommitMessageTemplate))
	if err != nil {
		return domain.Instance{}, err
	}
	return domain.Instance{
		ID:                    current.ID,
		InstanceCode:          code,
		Name:                  name,
		LocalRoot:             localRoot,
		DefaultBranch:         defaultBranch,
		Username:              username,
		Password:              password,
		Token:                 token,
		AuthorName:            authorName,
		AuthorEmail:           authorEmail,
		CommitMessageTemplate: commitMessageTemplate,
		CommandTimeoutSec:     commandTimeoutSec,
		Status:                status,
		Remark:                strings.TrimSpace(input.Remark),
		CreatedAt:             current.CreatedAt,
		UpdatedAt:             uc.now(),
	}, nil
}

// normalizeGitOpsInstanceCode 标准化输入值，保证后续逻辑使用统一格式。
func normalizeGitOpsInstanceCode(value string) (string, error) {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" {
		return "", fmt.Errorf("%w: instance_code is required", ErrInvalidInput)
	}
	if !gitopsInstanceCodePattern.MatchString(value) {
		return "", fmt.Errorf("%w: instance_code 格式无效", ErrInvalidInput)
	}
	return value, nil
}

// normalizeGitOpsLocalRoot 标准化输入值，保证后续逻辑使用统一格式。
func normalizeGitOpsLocalRoot(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", fmt.Errorf("%w: local_root is required", ErrInvalidInput)
	}
	cleaned := filepath.Clean(value)
	if strings.TrimSpace(cleaned) == "" || cleaned == "." {
		return "", fmt.Errorf("%w: local_root 格式无效", ErrInvalidInput)
	}
	return cleaned, nil
}
