package usecase

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	domain "gos/internal/domain/gitcredential"
	"gos/internal/infrastructure/gitcli"
	"gos/internal/infrastructure/gitlab"
)

// GitLabUserClient is the slice of the GitLab client the connection test needs.
// It is declared here so the use case can be exercised without a live GitLab.
type GitLabUserClient interface {
	CurrentUser(ctx context.Context) (gitlab.User, error)
}

type GitCredentialManager struct {
	repo      domain.Repository
	now       func() time.Time
	newClient func(cfg gitlab.Config) GitLabUserClient
}

// GitCredentialInput is the single shape accepted by Create and Update. The
// handler fills the defaults, the use case normalises and validates them.
type GitCredentialInput struct {
	Name     string
	Provider string
	BaseURL  string
	Username string
	Secret   string
	AuthType string
	Status   string
	Remark   string
}

// GitCredentialConnectionTestResult is the outcome of the credential test. A
// GitLab failure is reported as OK=false with a readable message rather than as
// an error, so the UI can show it inline.
type GitCredentialConnectionTestResult struct {
	OK             bool
	Message        string
	GitLabUsername string
}

// NewGitCredentialManager 创建并返回对应组件实例。
func NewGitCredentialManager(repo domain.Repository) *GitCredentialManager {
	return &GitCredentialManager{
		repo: repo,
		now: func() time.Time {
			return time.Now().UTC()
		},
		newClient: func(cfg gitlab.Config) GitLabUserClient {
			return gitlab.NewClient(cfg)
		},
	}
}

// Create 创建业务资源并返回处理结果。
func (uc *GitCredentialManager) Create(ctx context.Context, input GitCredentialInput) (domain.Credential, error) {
	clean, err := normalizeGitCredentialInput(input, gitCredentialNormalizeOptions{
		requireName:   true,
		requireSecret: true,
	})
	if err != nil {
		return domain.Credential{}, err
	}

	now := uc.now()
	item := domain.Credential{
		ID:        generateID("gc"),
		Name:      clean.Name,
		Provider:  clean.Provider,
		BaseURL:   clean.BaseURL,
		Username:  clean.Username,
		Secret:    clean.Secret,
		AuthType:  clean.AuthType,
		Status:    clean.Status,
		Remark:    clean.Remark,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := uc.repo.Create(ctx, item); err != nil {
		return domain.Credential{}, err
	}
	return uc.repo.GetByID(ctx, item.ID)
}

// List 查询并返回列表数据。
func (uc *GitCredentialManager) List(ctx context.Context, filter domain.ListFilter) ([]domain.Credential, int64, error) {
	const (
		defaultPage     = 1
		defaultPageSize = 20
		maxPageSize     = 100
	)
	filter.Keyword = strings.TrimSpace(filter.Keyword)
	if filter.Status != "" && !filter.Status.Valid() {
		return nil, 0, ErrInvalidStatus
	}
	if filter.Page <= 0 {
		filter.Page = defaultPage
	}
	if filter.PageSize <= 0 {
		filter.PageSize = defaultPageSize
	}
	if filter.PageSize > maxPageSize {
		filter.PageSize = maxPageSize
	}
	return uc.repo.List(ctx, filter)
}

// GetByID 查询并返回指定资源数据。
func (uc *GitCredentialManager) GetByID(ctx context.Context, id string) (domain.Credential, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return domain.Credential{}, ErrInvalidID
	}
	return uc.repo.GetByID(ctx, id)
}

// Update 更新业务资源并返回处理结果。A blank secret keeps the stored one.
func (uc *GitCredentialManager) Update(ctx context.Context, id string, input GitCredentialInput) (domain.Credential, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return domain.Credential{}, ErrInvalidID
	}
	current, err := uc.repo.GetByID(ctx, id)
	if err != nil {
		return domain.Credential{}, err
	}
	clean, err := normalizeGitCredentialInput(input, gitCredentialNormalizeOptions{
		requireName: true,
		current:     current,
	})
	if err != nil {
		return domain.Credential{}, err
	}
	return uc.repo.Update(ctx, id, clean, uc.now())
}

// Delete 删除业务资源并返回处理结果。
func (uc *GitCredentialManager) Delete(ctx context.Context, id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return ErrInvalidID
	}
	return uc.repo.Delete(ctx, id)
}

// TestConnection calls GET /api/v4/user with the stored credential. Only a
// missing or unusable record fails the call itself; a rejected credential comes
// back as OK=false so the page does not turn an auth problem into a 500.
func (uc *GitCredentialManager) TestConnection(ctx context.Context, id string) (GitCredentialConnectionTestResult, error) {
	item, err := uc.GetByID(ctx, id)
	if err != nil {
		return GitCredentialConnectionTestResult{}, err
	}
	if strings.TrimSpace(item.BaseURL) == "" {
		return GitCredentialConnectionTestResult{OK: false, Message: "凭证未配置 Git 地址"}, nil
	}
	// A plain account password works for the git transport but is rejected by the
	// GitLab API (v4 requires a personal access token), so probing /api/v4/user
	// would report a usable credential as refused. For that mode, say how it is
	// actually verified instead of failing it.
	if item.AuthType == domain.AuthTypePassword {
		return GitCredentialConnectionTestResult{
			OK:      true,
			Message: "账号密码模式：提交信息通过 git 协议读取（GitLab API 校验需要访问令牌）；可在发布单列表/详情的「最近提交」确认是否生效",
		}, nil
	}
	client := uc.newClient(gitlab.Config{
		BaseURL:  item.BaseURL,
		Username: item.Username,
		Secret:   item.Secret,
		AuthType: string(item.AuthType),
	})
	if client == nil {
		return GitCredentialConnectionTestResult{OK: false, Message: "GitLab 客户端不可用"}, nil
	}

	user, err := client.CurrentUser(ctx)
	if err != nil {
		return GitCredentialConnectionTestResult{OK: false, Message: describeGitLabFailure(err)}, nil
	}
	username := strings.TrimSpace(user.Username)
	if username == "" {
		username = strings.TrimSpace(user.Name)
	}
	message := "连接成功"
	if username != "" {
		message = fmt.Sprintf("连接成功，当前 GitLab 账号：%s", username)
	}
	return GitCredentialConnectionTestResult{OK: true, Message: message, GitLabUsername: username}, nil
}

// gitCredentialCredential carries the stored secret so an update can keep it
// when the request leaves the field blank.
type gitCredentialNormalizeOptions struct {
	// requireName rejects a blank name. Both Create and Update require it.
	requireName bool
	// requireSecret rejects a blank secret instead of falling back to the stored
	// one. Creating requires it; updating does not.
	requireSecret bool
	// current supplies the stored record for the update path.
	current domain.Credential
}

// normalizeGitCredentialInput validates and defaults the request for every entry
// point, so Create and Update cannot drift apart.
func normalizeGitCredentialInput(input GitCredentialInput, options gitCredentialNormalizeOptions) (domain.UpdateInput, error) {
	name := strings.TrimSpace(input.Name)
	if options.requireName && name == "" {
		return domain.UpdateInput{}, fmt.Errorf("%w: name is required", ErrInvalidInput)
	}

	provider := domain.Provider(strings.ToLower(strings.TrimSpace(input.Provider)))
	if provider == "" {
		provider = domain.ProviderGitLab
	}
	if !provider.Valid() {
		return domain.UpdateInput{}, fmt.Errorf("%w: provider is invalid", ErrInvalidInput)
	}

	baseURL := domain.NormalizeBaseURL(input.BaseURL)
	if baseURL == "" {
		return domain.UpdateInput{}, fmt.Errorf("%w: base_url is required", ErrInvalidInput)
	}

	authType := domain.AuthType(strings.ToLower(strings.TrimSpace(input.AuthType)))
	if authType == "" {
		authType = domain.AuthTypeToken
	}
	if !authType.Valid() {
		return domain.UpdateInput{}, fmt.Errorf("%w: auth_type is invalid", ErrInvalidInput)
	}

	status := domain.Status(strings.ToLower(strings.TrimSpace(input.Status)))
	if status == "" {
		status = domain.StatusActive
	}
	if !status.Valid() {
		return domain.UpdateInput{}, ErrInvalidStatus
	}

	secret := resolveCredentialSecret(input.Secret, options.current.Secret)
	if options.requireSecret && secret == "" {
		return domain.UpdateInput{}, fmt.Errorf("%w: secret is required", ErrInvalidInput)
	}

	return domain.UpdateInput{
		Name:     name,
		Provider: provider,
		BaseURL:  baseURL,
		Username: strings.TrimSpace(input.Username),
		Secret:   secret,
		AuthType: authType,
		Status:   status,
		Remark:   strings.TrimSpace(input.Remark),
	}, nil
}

// resolveCredentialSecret keeps the stored secret when the request omits it,
// which is how the UI expresses "leave the secret unchanged".
func resolveCredentialSecret(value string, current string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return strings.TrimSpace(current)
	}
	return trimmed
}

// describeGitLabFailure turns a GitLab client error into one short, actionable
// line for the UI. The sentinels already carry the GitLab message; the remaining
// cases are classified by timeout shape so a network problem does not surface as
// a stack of wrapped errors.
func describeGitLabFailure(err error) string {
	switch {
	case err == nil:
		return ""
	case errors.Is(err, context.Canceled):
		return "请求已取消"
	case errors.Is(err, gitlab.ErrUnauthorized):
		return "认证被拒绝：GitLab API 需要访问令牌，或该账号缺少仓库读取权限"
	case errors.Is(err, gitlab.ErrProjectNotFound):
		return "仓库不存在，或当前凭证无权访问该仓库"
	// An anchored read (the release time of a publish order) can walk past the end
	// of the history git was asked to read; both channels report that with the
	// same sentinel and the message is meant for the release order list.
	case errors.Is(err, gitlab.ErrHistoryTooShallow):
		return gitlab.ErrHistoryTooShallow.Error()
	// The git protocol channel (password credentials) reports its own failures;
	// keeping the classification here means both channels reach the UI with the
	// same wording and neither one leaks the secret.
	case errors.Is(err, gitcli.ErrAuthenticationFailed):
		return gitcli.ErrAuthenticationFailed.Error()
	case errors.Is(err, gitcli.ErrBranchNotFound):
		return "分支不存在，请确认发布单配置的分支名"
	case errors.Is(err, gitcli.ErrTimeout):
		return "Git 拉取超时，请检查 GitLab 地址是否可达"
	case errors.Is(err, gitcli.ErrGitUnavailable):
		return "运行环境缺少 git 命令，无法通过 git 协议读取提交"
	case errors.Is(err, context.DeadlineExceeded):
		return "请求 GitLab 超时，请检查 GitLab 地址是否可达"
	}
	var timeout interface{ Timeout() bool }
	if errors.As(err, &timeout) && timeout.Timeout() {
		return "请求 GitLab 超时，请检查 GitLab 地址是否可达"
	}
	return "请求 GitLab 失败：" + err.Error()
}
