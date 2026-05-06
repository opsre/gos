package usecase

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	domain "gos/internal/domain/argocdapp"
)

type ArgoCDApplicationClient interface {
	Ping(ctx context.Context) error
	ListApplications(ctx context.Context) ([]ArgoCDApplicationSnapshot, error)
	GetApplication(ctx context.Context, name string) (ArgoCDApplicationSnapshot, error)
	SyncApplication(ctx context.Context, name string) error
	SyncApplicationWithRevision(ctx context.Context, name string, revision string) error
	BuildApplicationURL(name string) string
}

type ArgoCDClientFactory interface {
	Build(instance domain.Instance) ArgoCDApplicationClient
}

type ArgoCDApplicationSnapshot interface {
	GetName() string
	GetProject() string
	GetRepoURL() string
	GetSourcePath() string
	GetTargetRevision() string
	GetDestServer() string
	GetDestNamespace() string
	GetSyncStatus() string
	GetHealthStatus() string
	GetOperationPhase() string
	GetRawMeta() string
}

type SyncArgoCDApplications struct {
	repo    domain.Repository
	factory ArgoCDClientFactory
	now     func() time.Time
}

type SyncArgoCDApplicationsOutput struct {
	Total       int `json:"total"`
	Created     int `json:"created"`
	Updated     int `json:"updated"`
	Inactivated int `json:"inactivated"`
}

type QueryArgoCDApplications struct {
	repo domain.Repository
}

type ArgoCDApplicationOriginalLinkOutput struct {
	Application  domain.Application `json:"application"`
	OriginalLink string             `json:"original_link"`
}

// NewSyncArgoCDApplications 创建并返回对应组件实例。
func NewSyncArgoCDApplications(repo domain.Repository, factory ArgoCDClientFactory) *SyncArgoCDApplications {
	return &SyncArgoCDApplications{
		repo:    repo,
		factory: factory,
		now: func() time.Time {
			return time.Now().UTC()
		},
	}
}

// NewQueryArgoCDApplications 创建并返回对应组件实例。
func NewQueryArgoCDApplications(repo domain.Repository) *QueryArgoCDApplications {
	return &QueryArgoCDApplications{repo: repo}
}

// Execute 封装当前模块的业务处理逻辑。
func (uc *SyncArgoCDApplications) Execute(ctx context.Context) (SyncArgoCDApplicationsOutput, error) {
	if uc == nil || uc.repo == nil || uc.factory == nil {
		return SyncArgoCDApplicationsOutput{}, fmt.Errorf("%w: argocd syncer is not configured", ErrInvalidInput)
	}
	instances, err := uc.repo.ListActiveInstances(ctx)
	if err != nil {
		return SyncArgoCDApplicationsOutput{}, err
	}
	if len(instances) == 0 {
		return SyncArgoCDApplicationsOutput{}, nil
	}

	result := SyncArgoCDApplicationsOutput{}
	now := uc.now()
	var firstErr error
	for _, instance := range instances {
		client := uc.factory.Build(instance)
		if client == nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("argocd client factory returned nil for instance %s", strings.TrimSpace(instance.InstanceCode))
			}
			_ = uc.repo.UpdateInstanceHealth(ctx, instance.ID, "unreachable", now)
			continue
		}
		items, listErr := client.ListApplications(ctx)
		if listErr != nil {
			if firstErr == nil {
				firstErr = listErr
			}
			_ = uc.repo.UpdateInstanceHealth(ctx, instance.ID, "unreachable", now)
			continue
		}
		_ = uc.repo.UpdateInstanceHealth(ctx, instance.ID, "healthy", now)

		models := make([]domain.Application, 0, len(items))
		keepNames := make([]string, 0, len(items))
		for _, item := range items {
			name := strings.TrimSpace(item.GetName())
			if name == "" {
				continue
			}
			keepNames = append(keepNames, name)
			models = append(models, domain.Application{
				ID:               argocdApplicationID(instance.ID, name),
				ArgoCDInstanceID: instance.ID,
				InstanceCode:     strings.TrimSpace(instance.InstanceCode),
				InstanceName:     strings.TrimSpace(instance.Name),
				ClusterName:      strings.TrimSpace(instance.ClusterName),
				InstanceBaseURL:  strings.TrimSpace(instance.BaseURL),
				AppName:          name,
				Project:          strings.TrimSpace(item.GetProject()),
				RepoURL:          strings.TrimSpace(item.GetRepoURL()),
				SourcePath:       strings.TrimSpace(item.GetSourcePath()),
				TargetRevision:   strings.TrimSpace(item.GetTargetRevision()),
				DestServer:       strings.TrimSpace(item.GetDestServer()),
				DestNamespace:    strings.TrimSpace(item.GetDestNamespace()),
				SyncStatus:       strings.TrimSpace(item.GetSyncStatus()),
				HealthStatus:     strings.TrimSpace(item.GetHealthStatus()),
				OperationPhase:   strings.TrimSpace(item.GetOperationPhase()),
				ArgoCDURL:        client.BuildApplicationURL(name),
				Status:           domain.StatusActive,
				RawMeta:          strings.TrimSpace(item.GetRawMeta()),
				LastSyncedAt:     now,
				CreatedAt:        now,
				UpdatedAt:        now,
			})
		}
		created, updated, upsertErr := uc.repo.UpsertApplications(ctx, models)
		if upsertErr != nil {
			if firstErr == nil {
				firstErr = upsertErr
			}
			continue
		}
		inactivated, markErr := uc.repo.MarkMissingApplicationsInactive(ctx, instance.ID, keepNames, now)
		if markErr != nil {
			if firstErr == nil {
				firstErr = markErr
			}
			continue
		}
		result.Total += len(models)
		result.Created += created
		result.Updated += updated
		result.Inactivated += inactivated
	}
	return result, firstErr
}

// List 查询并返回列表数据。
func (uc *QueryArgoCDApplications) List(ctx context.Context, filter domain.ListFilter) ([]domain.Application, int64, error) {
	if uc == nil || uc.repo == nil {
		return nil, 0, fmt.Errorf("%w: argocd query service is not configured", ErrInvalidInput)
	}
	filter.ArgoCDInstanceID = strings.TrimSpace(filter.ArgoCDInstanceID)
	filter.AppName = strings.TrimSpace(filter.AppName)
	filter.Project = strings.TrimSpace(filter.Project)
	filter.SyncStatus = strings.TrimSpace(filter.SyncStatus)
	filter.HealthStatus = strings.TrimSpace(filter.HealthStatus)
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
	return uc.repo.ListApplications(ctx, filter)
}

// GetByID 查询并返回指定资源数据。
func (uc *QueryArgoCDApplications) GetByID(ctx context.Context, id string) (domain.Application, error) {
	if uc == nil || uc.repo == nil {
		return domain.Application{}, fmt.Errorf("%w: argocd query service is not configured", ErrInvalidInput)
	}
	if strings.TrimSpace(id) == "" {
		return domain.Application{}, ErrInvalidID
	}
	return uc.repo.GetApplicationByID(ctx, id)
}

// GetOriginalLink 查询并返回指定资源数据。
func (uc *QueryArgoCDApplications) GetOriginalLink(ctx context.Context, id string) (ArgoCDApplicationOriginalLinkOutput, error) {
	item, err := uc.GetByID(ctx, id)
	if err != nil {
		return ArgoCDApplicationOriginalLinkOutput{}, err
	}
	link := strings.TrimSpace(item.ArgoCDURL)
	if link == "" && strings.TrimSpace(item.InstanceBaseURL) != "" {
		link = strings.TrimRight(strings.TrimSpace(item.InstanceBaseURL), "/") + "/applications/" + strings.TrimSpace(item.AppName)
	}
	if link == "" {
		return ArgoCDApplicationOriginalLinkOutput{}, fmt.Errorf("%w: argocd original link is unavailable", ErrInvalidInput)
	}
	return ArgoCDApplicationOriginalLinkOutput{Application: item, OriginalLink: link}, nil
}

type argoCDApplicationSnapshotAdapter struct {
	Name           string
	Project        string
	RepoURL        string
	SourcePath     string
	TargetRevision string
	DestServer     string
	DestNamespace  string
	SyncStatus     string
	HealthStatus   string
	OperationPhase string
	RawMeta        string
}

// GetName 查询并返回指定资源数据。
func (a argoCDApplicationSnapshotAdapter) GetName() string { return a.Name }

// GetProject 查询并返回指定资源数据。
func (a argoCDApplicationSnapshotAdapter) GetProject() string { return a.Project }

// GetRepoURL 查询并返回指定资源数据。
func (a argoCDApplicationSnapshotAdapter) GetRepoURL() string { return a.RepoURL }

// GetSourcePath 查询并返回指定资源数据。
func (a argoCDApplicationSnapshotAdapter) GetSourcePath() string { return a.SourcePath }

// GetTargetRevision 查询并返回指定资源数据。
func (a argoCDApplicationSnapshotAdapter) GetTargetRevision() string { return a.TargetRevision }

// GetDestServer 查询并返回指定资源数据。
func (a argoCDApplicationSnapshotAdapter) GetDestServer() string { return a.DestServer }

// GetDestNamespace 查询并返回指定资源数据。
func (a argoCDApplicationSnapshotAdapter) GetDestNamespace() string { return a.DestNamespace }

// GetSyncStatus 查询并返回指定资源数据。
func (a argoCDApplicationSnapshotAdapter) GetSyncStatus() string { return a.SyncStatus }

// GetHealthStatus 查询并返回指定资源数据。
func (a argoCDApplicationSnapshotAdapter) GetHealthStatus() string { return a.HealthStatus }

// GetOperationPhase 查询并返回指定资源数据。
func (a argoCDApplicationSnapshotAdapter) GetOperationPhase() string { return a.OperationPhase }

// GetRawMeta 查询并返回指定资源数据。
func (a argoCDApplicationSnapshotAdapter) GetRawMeta() string { return a.RawMeta }

// argocdApplicationID 封装当前模块的业务处理逻辑。
func argocdApplicationID(instanceID string, name string) string {
	hash := sha1.Sum([]byte(strings.TrimSpace(instanceID) + ":" + strings.TrimSpace(name)))
	return "argocd-app-" + hex.EncodeToString(hash[:])[:24]
}
