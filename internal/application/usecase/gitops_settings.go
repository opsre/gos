package usecase

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"

	platformparamdomain "gos/internal/domain/platformparam"
	gitopsinfra "gos/internal/infrastructure/gitops"
)

type GitOpsCommitTemplateUpdater interface {
	UpdateCommitMessageTemplate(template string) string
}

type GitOpsCommitTemplateStore interface {
	SaveCommitMessageTemplate(ctx context.Context, template string) error
}

type GitOpsCommitTemplateFieldReader interface {
	List(ctx context.Context, filter platformparamdomain.ListFilter) ([]platformparamdomain.PlatformParamDict, int64, error)
}

type UpdateGitOpsCommitTemplateInput struct {
	Template string
}

type UpdateGitOpsCommitTemplate struct {
	store        GitOpsCommitTemplateStore
	updater      GitOpsCommitTemplateUpdater
	reader       *QueryGitOpsStatus
	platformRepo GitOpsCommitTemplateFieldReader
}

// NewUpdateGitOpsCommitTemplate 创建并返回对应组件实例。
func NewUpdateGitOpsCommitTemplate(
	store GitOpsCommitTemplateStore,
	updater GitOpsCommitTemplateUpdater,
	reader *QueryGitOpsStatus,
	platformRepo GitOpsCommitTemplateFieldReader,
) *UpdateGitOpsCommitTemplate {
	return &UpdateGitOpsCommitTemplate{
		store:        store,
		updater:      updater,
		reader:       reader,
		platformRepo: platformRepo,
	}
}

// Execute 封装当前模块的业务处理逻辑。
func (uc *UpdateGitOpsCommitTemplate) Execute(
	ctx context.Context,
	input UpdateGitOpsCommitTemplateInput,
) (QueryGitOpsStatusOutput, error) {
	if uc == nil || uc.store == nil || uc.updater == nil || uc.reader == nil || uc.platformRepo == nil {
		return QueryGitOpsStatusOutput{}, fmt.Errorf("%w: gitops manager is not configured", ErrInvalidInput)
	}

	normalized, err := validateGitOpsCommitMessageTemplate(ctx, uc.platformRepo, strings.TrimSpace(input.Template))
	if err != nil {
		return QueryGitOpsStatusOutput{}, err
	}
	if err := uc.store.SaveCommitMessageTemplate(ctx, normalized); err != nil {
		return QueryGitOpsStatusOutput{}, err
	}
	uc.updater.UpdateCommitMessageTemplate(normalized)
	return uc.reader.Execute(ctx)
}

var gitOpsCommitTemplateTokenPattern = regexp.MustCompile(`\{([a-zA-Z0-9_]+)\}`)

// validateGitOpsCommitMessageTemplate 查询并返回指定资源数据。
func validateGitOpsCommitMessageTemplate(
	ctx context.Context,
	platformRepo GitOpsCommitTemplateFieldReader,
	template string,
) (string, error) {
	normalized := gitopsinfra.NormalizeCommitMessageTemplate(strings.TrimSpace(template))
	if platformRepo == nil {
		return "", fmt.Errorf("%w: platform param repository is not configured", ErrInvalidInput)
	}
	status := platformparamdomain.StatusEnabled
	items, _, err := platformRepo.List(ctx, platformparamdomain.ListFilter{
		Status:   &status,
		Page:     1,
		PageSize: 1000,
	})
	if err != nil {
		return "", err
	}
	allowed := make(map[string]struct{}, len(items))
	for _, item := range items {
		key := strings.ToLower(strings.TrimSpace(item.ParamKey))
		if key == "" {
			continue
		}
		allowed[key] = struct{}{}
	}
	matches := gitOpsCommitTemplateTokenPattern.FindAllStringSubmatch(normalized, -1)
	if len(matches) == 0 {
		return normalized, nil
	}
	unknown := make([]string, 0)
	seen := make(map[string]struct{})
	for _, match := range matches {
		if len(match) < 2 {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(match[1]))
		if key == "" {
			continue
		}
		if _, ok := allowed[key]; ok {
			continue
		}
		if _, duplicated := seen[key]; duplicated {
			continue
		}
		seen[key] = struct{}{}
		unknown = append(unknown, key)
	}
	if len(unknown) > 0 {
		sort.Strings(unknown)
		return "", fmt.Errorf("%w: commit message template placeholders must come from platform param dict, unknown keys: %s", ErrInvalidInput, strings.Join(unknown, ", "))
	}
	return normalized, nil
}
