package usecase

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"

	domain "gos/internal/domain/argocdapp"
	gitopsdomain "gos/internal/domain/gitops"
	"gos/internal/support/logx"
)

var argocdInstanceCodePattern = regexp.MustCompile(`^[a-z][a-z0-9_-]*$`)

type ArgoCDInstanceManager struct {
	repo       domain.Repository
	gitopsRepo gitopsdomain.Repository
	factory    ArgoCDClientFactory
	now        func() time.Time
}

type CreateArgoCDInstanceInput struct {
	InstanceCode       string
	Name               string
	BaseURL            string
	InsecureSkipVerify bool
	AuthMode           string
	Token              string
	Username           string
	Password           string
	GitOpsInstanceID   string
	ClusterName        string
	DefaultNamespace   string
	Status             domain.Status
	Remark             string
}

type UpdateArgoCDInstanceInput struct {
	InstanceCode       string
	Name               string
	BaseURL            string
	InsecureSkipVerify bool
	AuthMode           string
	Token              string
	Username           string
	Password           string
	GitOpsInstanceID   string
	ClusterName        string
	DefaultNamespace   string
	Status             domain.Status
	Remark             string
}

type UpdateArgoCDEnvBindingItem struct {
	EnvCode          string
	ArgoCDInstanceID string
	Status           domain.Status
}

// NewArgoCDInstanceManager 创建并返回对应组件实例。
func NewArgoCDInstanceManager(
	repo domain.Repository,
	gitopsRepo gitopsdomain.Repository,
	factory ArgoCDClientFactory,
) *ArgoCDInstanceManager {
	return &ArgoCDInstanceManager{repo: repo, gitopsRepo: gitopsRepo, factory: factory, now: func() time.Time { return time.Now().UTC() }}
}

// List 查询并返回列表数据。
func (uc *ArgoCDInstanceManager) List(ctx context.Context, filter domain.InstanceListFilter) ([]domain.Instance, int64, error) {
	if uc == nil || uc.repo == nil {
		return nil, 0, fmt.Errorf("%w: argocd instance manager is not configured", ErrInvalidInput)
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

// Create 创建业务资源并返回处理结果。
func (uc *ArgoCDInstanceManager) Create(ctx context.Context, input CreateArgoCDInstanceInput) (domain.Instance, error) {
	if uc == nil || uc.repo == nil {
		return domain.Instance{}, fmt.Errorf("%w: argocd instance manager is not configured", ErrInvalidInput)
	}
	logx.Info("argocd_instance", "create_start",
		logx.F("instance_code", input.InstanceCode),
		logx.F("name", input.Name),
		logx.F("base_url", input.BaseURL),
		logx.F("gitops_instance_id", input.GitOpsInstanceID),
		logx.F("status", input.Status),
	)
	instance, err := uc.normalizeCreateInput(ctx, input)
	if err != nil {
		logx.Error("argocd_instance", "create_failed", err,
			logx.F("instance_code", input.InstanceCode),
			logx.F("name", input.Name),
			logx.F("base_url", input.BaseURL),
		)
		return domain.Instance{}, err
	}
	created, err := uc.repo.CreateInstance(ctx, instance)
	if err != nil {
		logx.Error("argocd_instance", "create_failed", err,
			logx.F("instance_id", instance.ID),
			logx.F("instance_code", instance.InstanceCode),
			logx.F("name", instance.Name),
		)
		return domain.Instance{}, err
	}
	logx.Info("argocd_instance", "create_success",
		logx.F("instance_id", created.ID),
		logx.F("instance_code", created.InstanceCode),
		logx.F("name", created.Name),
		logx.F("gitops_instance_id", created.GitOpsInstanceID),
	)
	return created, nil
}

// Update 更新业务资源并返回处理结果。
func (uc *ArgoCDInstanceManager) Update(ctx context.Context, id string, input UpdateArgoCDInstanceInput) (domain.Instance, error) {
	if uc == nil || uc.repo == nil {
		return domain.Instance{}, fmt.Errorf("%w: argocd instance manager is not configured", ErrInvalidInput)
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return domain.Instance{}, ErrInvalidID
	}
	logx.Info("argocd_instance", "update_start",
		logx.F("instance_id", id),
		logx.F("instance_code", input.InstanceCode),
		logx.F("name", input.Name),
		logx.F("base_url", input.BaseURL),
		logx.F("gitops_instance_id", input.GitOpsInstanceID),
		logx.F("status", input.Status),
	)
	current, err := uc.repo.GetInstanceByID(ctx, id)
	if err != nil {
		logx.Error("argocd_instance", "update_failed", err, logx.F("instance_id", id))
		return domain.Instance{}, err
	}
	instance, err := uc.normalizeUpdateInput(ctx, current, input)
	if err != nil {
		logx.Error("argocd_instance", "update_failed", err,
			logx.F("instance_id", id),
			logx.F("instance_code", current.InstanceCode),
			logx.F("name", current.Name),
		)
		return domain.Instance{}, err
	}
	updated, err := uc.repo.UpdateInstance(ctx, instance)
	if err != nil {
		logx.Error("argocd_instance", "update_failed", err,
			logx.F("instance_id", instance.ID),
			logx.F("instance_code", instance.InstanceCode),
			logx.F("name", instance.Name),
		)
		return domain.Instance{}, err
	}
	logx.Info("argocd_instance", "update_success",
		logx.F("instance_id", updated.ID),
		logx.F("instance_code", updated.InstanceCode),
		logx.F("name", updated.Name),
		logx.F("gitops_instance_id", updated.GitOpsInstanceID),
		logx.F("status", updated.Status),
	)
	return updated, nil
}

// Check 检查业务状态并返回校验结果。
func (uc *ArgoCDInstanceManager) Check(ctx context.Context, id string) (domain.Instance, error) {
	if uc == nil || uc.repo == nil || uc.factory == nil {
		return domain.Instance{}, fmt.Errorf("%w: argocd instance manager is not configured", ErrInvalidInput)
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return domain.Instance{}, ErrInvalidID
	}
	logx.Info("argocd_instance", "health_check_start", logx.F("instance_id", id))
	instance, err := uc.repo.GetInstanceByID(ctx, id)
	if err != nil {
		logx.Error("argocd_instance", "health_check_failed", err, logx.F("instance_id", id))
		return domain.Instance{}, err
	}
	client := uc.factory.Build(instance)
	if client == nil {
		err := fmt.Errorf("%w: argocd client factory is not configured", ErrInvalidInput)
		logx.Error("argocd_instance", "health_check_failed", err,
			logx.F("instance_id", instance.ID),
			logx.F("instance_code", instance.InstanceCode),
		)
		return domain.Instance{}, err
	}
	checkedAt := uc.now()
	healthStatus := "healthy"
	if err := client.Ping(ctx); err != nil {
		healthStatus = "unreachable"
		_ = uc.repo.UpdateInstanceHealth(ctx, instance.ID, healthStatus, checkedAt)
		logx.Error("argocd_instance", "health_check_failed", err,
			logx.F("instance_id", instance.ID),
			logx.F("instance_code", instance.InstanceCode),
			logx.F("base_url", instance.BaseURL),
			logx.F("health_status", healthStatus),
		)
		return domain.Instance{}, err
	}
	if err := uc.repo.UpdateInstanceHealth(ctx, instance.ID, healthStatus, checkedAt); err != nil {
		logx.Error("argocd_instance", "health_check_failed", err,
			logx.F("instance_id", instance.ID),
			logx.F("instance_code", instance.InstanceCode),
			logx.F("health_status", healthStatus),
		)
		return domain.Instance{}, err
	}
	checked, err := uc.repo.GetInstanceByID(ctx, instance.ID)
	if err != nil {
		logx.Error("argocd_instance", "health_check_failed", err,
			logx.F("instance_id", instance.ID),
			logx.F("instance_code", instance.InstanceCode),
		)
		return domain.Instance{}, err
	}
	logx.Info("argocd_instance", "health_check_success",
		logx.F("instance_id", checked.ID),
		logx.F("instance_code", checked.InstanceCode),
		logx.F("health_status", checked.HealthStatus),
		logx.F("checked_at", checked.LastCheckAt),
	)
	return checked, nil
}

// ListEnvBindings 查询并返回列表数据。
func (uc *ArgoCDInstanceManager) ListEnvBindings(ctx context.Context) ([]domain.EnvBinding, error) {
	if uc == nil || uc.repo == nil {
		return nil, fmt.Errorf("%w: argocd instance manager is not configured", ErrInvalidInput)
	}
	return uc.repo.ListEnvBindings(ctx)
}

// UpdateEnvBindings 更新业务资源并返回处理结果。
func (uc *ArgoCDInstanceManager) UpdateEnvBindings(ctx context.Context, items []UpdateArgoCDEnvBindingItem) ([]domain.EnvBinding, error) {
	if uc == nil || uc.repo == nil {
		return nil, fmt.Errorf("%w: argocd instance manager is not configured", ErrInvalidInput)
	}
	logx.Info("argocd_instance", "env_bindings_update_start", logx.F("items_count", len(items)))
	now := uc.now()
	payload := make([]domain.EnvBinding, 0, len(items))
	seenBinding := make(map[string]struct{}, len(items))
	priorityByEnv := make(map[string]int)
	for _, item := range items {
		envCode := strings.TrimSpace(item.EnvCode)
		instanceID := strings.TrimSpace(item.ArgoCDInstanceID)
		if envCode == "" || instanceID == "" {
			continue
		}
		bindingKey := envCode + "\x00" + instanceID
		if _, exists := seenBinding[bindingKey]; exists {
			err := fmt.Errorf("%w: 环境绑定存在重复实例: env_code=%s argocd_instance_id=%s", ErrInvalidInput, envCode, instanceID)
			logx.Error("argocd_instance", "env_bindings_update_failed", err,
				logx.F("env_code", envCode),
				logx.F("argocd_instance_id", instanceID),
			)
			return nil, err
		}
		seenBinding[bindingKey] = struct{}{}
		if _, err := uc.repo.GetInstanceByID(ctx, instanceID); err != nil {
			logx.Error("argocd_instance", "env_bindings_update_failed", err,
				logx.F("env_code", envCode),
				logx.F("argocd_instance_id", instanceID),
			)
			return nil, err
		}
		status := item.Status
		if status == "" {
			status = domain.StatusActive
		}
		if !status.Valid() {
			logx.Error("argocd_instance", "env_bindings_update_failed", ErrInvalidStatus,
				logx.F("env_code", envCode),
				logx.F("argocd_instance_id", instanceID),
				logx.F("status", status),
			)
			return nil, ErrInvalidStatus
		}
		priorityByEnv[envCode]++
		payload = append(payload, domain.EnvBinding{
			ID:               generateID("aeb"),
			EnvCode:          envCode,
			ArgoCDInstanceID: instanceID,
			Priority:         priorityByEnv[envCode],
			Status:           status,
			CreatedAt:        now,
			UpdatedAt:        now,
		})
	}
	if err := uc.repo.ReplaceEnvBindings(ctx, payload); err != nil {
		logx.Error("argocd_instance", "env_bindings_update_failed", err,
			logx.F("items_count", len(payload)),
		)
		return nil, err
	}
	bindings, err := uc.repo.ListEnvBindings(ctx)
	if err != nil {
		logx.Error("argocd_instance", "env_bindings_update_failed", err,
			logx.F("items_count", len(payload)),
		)
		return nil, err
	}
	logx.Info("argocd_instance", "env_bindings_update_success",
		logx.F("items_count", len(payload)),
		logx.F("bindings_count", len(bindings)),
	)
	return bindings, nil
}

// normalizeCreateInput 标准化输入值，保证后续逻辑使用统一格式。
func (uc *ArgoCDInstanceManager) normalizeCreateInput(ctx context.Context, input CreateArgoCDInstanceInput) (domain.Instance, error) {
	code, err := normalizeArgoCDInstanceCode(input.InstanceCode)
	if err != nil {
		return domain.Instance{}, err
	}
	baseURL, err := normalizeArgoCDBaseURL(input.BaseURL)
	if err != nil {
		return domain.Instance{}, err
	}
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return domain.Instance{}, fmt.Errorf("%w: name is required", ErrInvalidInput)
	}
	authMode := normalizeArgoCDAuthMode(input.AuthMode)
	if authMode == "" {
		return domain.Instance{}, fmt.Errorf("%w: auth_mode is required", ErrInvalidInput)
	}
	status := input.Status
	if status == "" {
		status = domain.StatusActive
	}
	if !status.Valid() {
		return domain.Instance{}, ErrInvalidStatus
	}
	if err := validateArgoCDCredentials(authMode, input.Token, input.Username, input.Password); err != nil {
		return domain.Instance{}, err
	}
	gitopsInstanceID := strings.TrimSpace(input.GitOpsInstanceID)
	if gitopsInstanceID != "" {
		if uc.gitopsRepo == nil {
			return domain.Instance{}, fmt.Errorf("%w: gitops repository is not configured", ErrInvalidInput)
		}
		if _, err := uc.gitopsRepo.GetInstanceByID(ctx, gitopsInstanceID); err != nil {
			return domain.Instance{}, err
		}
	}
	now := uc.now()
	return domain.Instance{
		ID:                 generateID("argocd"),
		InstanceCode:       code,
		Name:               name,
		BaseURL:            baseURL,
		InsecureSkipVerify: input.InsecureSkipVerify,
		AuthMode:           authMode,
		Token:              strings.TrimSpace(input.Token),
		Username:           strings.TrimSpace(input.Username),
		Password:           strings.TrimSpace(input.Password),
		GitOpsInstanceID:   gitopsInstanceID,
		ClusterName:        strings.TrimSpace(input.ClusterName),
		DefaultNamespace:   strings.TrimSpace(input.DefaultNamespace),
		Status:             status,
		HealthStatus:       "unknown",
		CreatedAt:          now,
		UpdatedAt:          now,
		Remark:             strings.TrimSpace(input.Remark),
	}, nil
}

// normalizeUpdateInput 标准化输入值，保证后续逻辑使用统一格式。
func (uc *ArgoCDInstanceManager) normalizeUpdateInput(ctx context.Context, current domain.Instance, input UpdateArgoCDInstanceInput) (domain.Instance, error) {
	code, err := normalizeArgoCDInstanceCode(input.InstanceCode)
	if err != nil {
		return domain.Instance{}, err
	}
	baseURL, err := normalizeArgoCDBaseURL(input.BaseURL)
	if err != nil {
		return domain.Instance{}, err
	}
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return domain.Instance{}, fmt.Errorf("%w: name is required", ErrInvalidInput)
	}
	authMode := normalizeArgoCDAuthMode(input.AuthMode)
	if authMode == "" {
		return domain.Instance{}, fmt.Errorf("%w: auth_mode is required", ErrInvalidInput)
	}
	status := input.Status
	if status == "" {
		status = current.Status
	}
	if !status.Valid() {
		return domain.Instance{}, ErrInvalidStatus
	}
	token := strings.TrimSpace(input.Token)
	if token == "" {
		token = current.Token
	}
	password := strings.TrimSpace(input.Password)
	if password == "" {
		password = current.Password
	}
	username := strings.TrimSpace(input.Username)
	if username == "" {
		username = current.Username
	}
	if err := validateArgoCDCredentials(authMode, token, username, password); err != nil {
		return domain.Instance{}, err
	}
	gitopsInstanceID := strings.TrimSpace(input.GitOpsInstanceID)
	if gitopsInstanceID == "" {
		gitopsInstanceID = current.GitOpsInstanceID
	}
	if gitopsInstanceID != "" {
		if uc.gitopsRepo == nil {
			return domain.Instance{}, fmt.Errorf("%w: gitops repository is not configured", ErrInvalidInput)
		}
		if _, err := uc.gitopsRepo.GetInstanceByID(ctx, gitopsInstanceID); err != nil {
			return domain.Instance{}, err
		}
	}
	return domain.Instance{
		ID:                 current.ID,
		InstanceCode:       code,
		Name:               name,
		BaseURL:            baseURL,
		InsecureSkipVerify: input.InsecureSkipVerify,
		AuthMode:           authMode,
		Token:              token,
		Username:           username,
		Password:           password,
		GitOpsInstanceID:   gitopsInstanceID,
		ClusterName:        strings.TrimSpace(input.ClusterName),
		DefaultNamespace:   strings.TrimSpace(input.DefaultNamespace),
		Status:             status,
		HealthStatus:       strings.TrimSpace(current.HealthStatus),
		LastCheckAt:        current.LastCheckAt,
		CreatedAt:          current.CreatedAt,
		UpdatedAt:          uc.now(),
		Remark:             strings.TrimSpace(input.Remark),
	}, nil
}

// normalizeArgoCDInstanceCode 标准化输入值，保证后续逻辑使用统一格式。
func normalizeArgoCDInstanceCode(value string) (string, error) {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" {
		return "", fmt.Errorf("%w: instance_code is required", ErrInvalidInput)
	}
	if !argocdInstanceCodePattern.MatchString(value) {
		return "", fmt.Errorf("%w: instance_code 格式无效", ErrInvalidInput)
	}
	return value, nil
}

// normalizeArgoCDBaseURL 标准化输入值，保证后续逻辑使用统一格式。
func normalizeArgoCDBaseURL(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", fmt.Errorf("%w: base_url is required", ErrInvalidInput)
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", fmt.Errorf("%w: base_url 格式无效", ErrInvalidInput)
	}
	return strings.TrimRight(parsed.String(), "/"), nil
}

// normalizeArgoCDAuthMode 标准化输入值，保证后续逻辑使用统一格式。
func normalizeArgoCDAuthMode(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	switch value {
	case "token", "password", "basic", "session":
		return value
	default:
		return ""
	}
}

// validateArgoCDCredentials 封装当前模块的业务处理逻辑。
func validateArgoCDCredentials(authMode, token, username, password string) error {
	switch normalizeArgoCDAuthMode(authMode) {
	case "token":
		if strings.TrimSpace(token) == "" {
			return fmt.Errorf("%w: token is required when auth_mode=token", ErrInvalidInput)
		}
	case "password", "basic", "session":
		if strings.TrimSpace(username) == "" || strings.TrimSpace(password) == "" {
			return fmt.Errorf("%w: username and password are required when auth_mode=%s", ErrInvalidInput, authMode)
		}
	default:
		return fmt.Errorf("%w: unsupported argocd auth_mode", ErrInvalidInput)
	}
	return nil
}
