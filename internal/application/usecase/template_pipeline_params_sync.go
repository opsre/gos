package usecase

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"strings"
	"time"

	domain "gos/internal/domain/executorparam"
	pipelinedomain "gos/internal/domain/pipeline"
	releasedomain "gos/internal/domain/release"
)

type JenkinsTemplateParamClient interface {
	GetJobParamSet(ctx context.Context, fullName string) (domain.JenkinsJobParamSet, error)
}

type SyncTemplatePipelineParams struct {
	releaseRepo  releasedomain.Repository
	pipelineRepo pipelinedomain.Repository
	paramRepo    domain.Repository
	jenkins      JenkinsTemplateParamClient
	now          func() time.Time
}

func NewSyncTemplatePipelineParams(
	releaseRepo releasedomain.Repository,
	pipelineRepo pipelinedomain.Repository,
	paramRepo domain.Repository,
	jenkins JenkinsTemplateParamClient,
) *SyncTemplatePipelineParams {
	return &SyncTemplatePipelineParams{
		releaseRepo:  releaseRepo,
		pipelineRepo: pipelineRepo,
		paramRepo:    paramRepo,
		jenkins:      jenkins,
		now: func() time.Time {
			return time.Now().UTC()
		},
	}
}

func (uc *SyncTemplatePipelineParams) Execute(ctx context.Context, templateID string) (SyncExecutorParamDefsOutput, error) {
	_, bindings, _, _, _, err := uc.releaseRepo.GetTemplateByID(ctx, templateID)
	if err != nil {
		return SyncExecutorParamDefsOutput{}, err
	}

	type pipelineRef struct {
		pipelineID string
		fullName   string
	}
	pipelineSet := make(map[string]pipelineRef)
	for _, b := range bindings {
		if b.Provider != "jenkins" || b.PipelineID == "" {
			continue
		}
		if _, exists := pipelineSet[b.PipelineID]; exists {
			continue
		}
		p, err := uc.pipelineRepo.GetPipelineByID(ctx, b.PipelineID)
		if err != nil {
			return SyncExecutorParamDefsOutput{}, err
		}
		if p.JobFullName == "" {
			continue
		}
		pipelineSet[b.PipelineID] = pipelineRef{
			pipelineID: b.PipelineID,
			fullName:   p.JobFullName,
		}
	}

	now := uc.now()
	items := make([]domain.ExecutorParamDef, 0)
	seen := make(map[string]struct{})
	skipped := 0

	for _, ref := range pipelineSet {
		jobSet, err := uc.jenkins.GetJobParamSet(ctx, ref.fullName)
		if err != nil {
			return SyncExecutorParamDefsOutput{}, err
		}

		for index, snapshot := range jobSet.Params {
			paramName := strings.TrimSpace(snapshot.Name)
			if paramName == "" {
				skipped++
				continue
			}

			uniqueKey := ref.pipelineID + ":" + string(domain.ExecutorTypeJenkins) + ":" + paramName
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
				ID:                templatePipelineParamDefID(ref.pipelineID, string(domain.ExecutorTypeJenkins), paramName),
				PipelineID:        ref.pipelineID,
				ExecutorType:      domain.ExecutorTypeJenkins,
				ExecutorParamName: paramName,
				ParamKey:          defaultJenkinsExecutorParamKey(paramName),
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

	created, updated, err := uc.paramRepo.Upsert(ctx, items)
	if err != nil {
		return SyncExecutorParamDefsOutput{}, err
	}

	return SyncExecutorParamDefsOutput{
		Total:       len(items),
		Created:     created,
		Updated:     updated,
		Inactivated: 0,
		Skipped:     skipped,
	}, nil
}

func templatePipelineParamDefID(pipelineID, executorType, executorParamName string) string {
	sum := sha1.Sum([]byte(pipelineID + ":" + executorType + ":" + executorParamName))
	return "ppf-" + hex.EncodeToString(sum[:12])
}
