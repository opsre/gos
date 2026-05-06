package usecase

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"strings"
	"time"

	domain "gos/internal/domain/pipeline"
)

type JenkinsPipelineClient interface {
	ListJobs(ctx context.Context) ([]domain.JenkinsJob, error)
	GetJob(ctx context.Context, fullName string) (domain.JenkinsJob, error)
	GetPipelineScript(ctx context.Context, fullName string) (domain.JenkinsPipelineScript, error)
	GetPipelineConfigXML(ctx context.Context, fullName string) (string, error)
	BuildJobURL(fullName string) string
}

type SyncPipelines struct {
	repo    domain.Repository
	jenkins JenkinsPipelineClient
	now     func() time.Time
}

type SyncPipelinesOutput struct {
	Total       int `json:"total"`
	Created     int `json:"created"`
	Updated     int `json:"updated"`
	Inactivated int `json:"inactivated"`
	Skipped     int `json:"skipped"`
}

// NewSyncPipelines 创建并返回对应组件实例。
func NewSyncPipelines(repo domain.Repository, jenkins JenkinsPipelineClient) *SyncPipelines {
	return &SyncPipelines{
		repo:    repo,
		jenkins: jenkins,
		now: func() time.Time {
			return time.Now().UTC()
		},
	}
}

// Execute 封装当前模块的业务处理逻辑。
func (uc *SyncPipelines) Execute(ctx context.Context) (SyncPipelinesOutput, error) {
	jobs, err := uc.jenkins.ListJobs(ctx)
	if err != nil {
		return SyncPipelinesOutput{}, err
	}

	now := uc.now()
	items := make([]domain.Pipeline, 0, len(jobs))
	seen := make(map[string]struct{}, len(jobs))
	skipped := 0

	for _, job := range jobs {
		fullName := strings.Trim(strings.TrimSpace(job.FullName), "/")
		if fullName == "" {
			skipped++
			continue
		}
		if _, exists := seen[fullName]; exists {
			skipped++
			continue
		}
		seen[fullName] = struct{}{}

		items = append(items, domain.Pipeline{
			ID:            pipelineID(string(domain.ProviderJenkins), fullName),
			Provider:      domain.ProviderJenkins,
			JobFullName:   fullName,
			JobName:       strings.TrimSpace(job.Name),
			JobURL:        strings.TrimSpace(job.URL),
			Description:   "",
			CredentialRef: "",
			DefaultBranch: "",
			Status:        domain.StatusActive,
			LastSyncedAt:  now,
			CreatedAt:     now,
			UpdatedAt:     now,
		})
	}

	created, updated, err := uc.repo.UpsertPipelines(ctx, items)
	if err != nil {
		return SyncPipelinesOutput{}, err
	}
	keepIDs := make([]string, 0, len(items))
	for _, item := range items {
		keepIDs = append(keepIDs, item.ID)
	}
	inactivated, err := uc.repo.MarkMissingPipelinesInactive(ctx, domain.ProviderJenkins, keepIDs, now)
	if err != nil {
		return SyncPipelinesOutput{}, err
	}

	return SyncPipelinesOutput{
		Total:       len(items),
		Created:     created,
		Updated:     updated,
		Inactivated: inactivated,
		Skipped:     skipped,
	}, nil
}

// pipelineID 封装当前模块的业务处理逻辑。
func pipelineID(provider, fullName string) string {
	sum := sha1.Sum([]byte(provider + ":" + fullName))
	return "pln-" + hex.EncodeToString(sum[:12])
}
