package usecase

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"strings"
	"time"

	domain "gos/internal/domain/executorparam"
)

type JenkinsExecutorParamClient interface {
	ListJobParamSets(ctx context.Context) ([]domain.JenkinsJobParamSet, error)
}

type SyncExecutorParamDefs struct {
	repo    domain.Repository
	jenkins JenkinsExecutorParamClient
	now     func() time.Time
}

type SyncExecutorParamDefsOutput struct {
	Total       int `json:"total"`
	Created     int `json:"created"`
	Updated     int `json:"updated"`
	Inactivated int `json:"inactivated"`
	Skipped     int `json:"skipped"`
}

// NewSyncExecutorParamDefs 创建并返回对应组件实例。
func NewSyncExecutorParamDefs(repo domain.Repository, jenkins JenkinsExecutorParamClient) *SyncExecutorParamDefs {
	return &SyncExecutorParamDefs{
		repo:    repo,
		jenkins: jenkins,
		now: func() time.Time {
			return time.Now().UTC()
		},
	}
}

// Execute 封装当前模块的业务处理逻辑。
func (uc *SyncExecutorParamDefs) Execute(ctx context.Context) (SyncExecutorParamDefsOutput, error) {
	jobSets, err := uc.jenkins.ListJobParamSets(ctx)
	if err != nil {
		return SyncExecutorParamDefsOutput{}, err
	}

	now := uc.now()
	items := make([]domain.ExecutorParamDef, 0)
	seen := make(map[string]struct{})
	skipped := 0

	for _, jobSet := range jobSets {
		fullName := strings.Trim(strings.TrimSpace(jobSet.JobFullName), "/")
		if fullName == "" {
			skipped++
			continue
		}

		pipelineID := pipelineID("jenkins", fullName)
		for index, snapshot := range jobSet.Params {
			paramName := strings.TrimSpace(snapshot.Name)
			if paramName == "" {
				skipped++
				continue
			}

			uniqueKey := pipelineID + ":" + string(domain.ExecutorTypeJenkins) + ":" + paramName
			if _, exists := seen[uniqueKey]; exists {
				skipped++
				continue
			}
			seen[uniqueKey] = struct{}{}

			paramType := snapshot.ParamType
			if !paramType.Valid() {
				paramType = domain.ParamTypeString
			}

			sortNo := snapshot.SortNo
			if sortNo <= 0 {
				sortNo = index + 1
			}

			items = append(items, domain.ExecutorParamDef{
				ID:                executorParamDefID(pipelineID, string(domain.ExecutorTypeJenkins), paramName),
				PipelineID:        pipelineID,
				ExecutorType:      domain.ExecutorTypeJenkins,
				ExecutorParamName: paramName,
				ParamKey:          "",
				ParamType:         paramType,
				SingleSelect:      snapshot.SingleSelect,
				Required:          snapshot.Required,
				DefaultValue:      strings.TrimSpace(snapshot.DefaultValue),
				Description:       strings.TrimSpace(snapshot.Description),
				Visible:           true,
				Editable:          true,
				SourceFrom:        domain.SourceFromSyncJenkins,
				Status:            domain.StatusActive,
				RawMeta:           strings.TrimSpace(snapshot.RawMeta),
				SortNo:            sortNo,
				CreatedAt:         now,
				UpdatedAt:         now,
			})
		}
	}

	created, updated, err := uc.repo.Upsert(ctx, items)
	if err != nil {
		return SyncExecutorParamDefsOutput{}, err
	}
	keepIDs := make([]string, 0, len(items))
	for _, item := range items {
		keepIDs = append(keepIDs, item.ID)
	}
	inactivated, err := uc.repo.MarkMissingInactive(ctx, domain.ExecutorTypeJenkins, keepIDs, now)
	if err != nil {
		return SyncExecutorParamDefsOutput{}, err
	}

	return SyncExecutorParamDefsOutput{
		Total:       len(items),
		Created:     created,
		Updated:     updated,
		Inactivated: inactivated,
		Skipped:     skipped,
	}, nil
}

// executorParamDefID 封装当前模块的业务处理逻辑。
func executorParamDefID(pipelineID, executorType, executorParamName string) string {
	sum := sha1.Sum([]byte(pipelineID + ":" + executorType + ":" + executorParamName))
	return "ppf-" + hex.EncodeToString(sum[:12])
}
