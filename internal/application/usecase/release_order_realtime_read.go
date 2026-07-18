package usecase

import (
	"context"
	"errors"
	"fmt"
	"strings"

	domain "gos/internal/domain/release"
)

// ReleaseOrderStoredRealtimeAggregate is a strictly read-only view of persisted release data.
// It loads one coherent storage snapshot while holding the same order lock used by tracker/cancel/dispatch.
// Public detail methods are also read-only; external synchronization is performed separately by the tracker.
type ReleaseOrderStoredRealtimeAggregate struct {
	Order                   domain.ReleaseOrder
	Executions              []domain.ReleaseOrderExecution
	Steps                   []domain.ReleaseOrderStep
	ValueProgress           []ReleaseOrderValueProgressItem
	PipelineStageView       ReleaseOrderPipelineStageView
	ArtifactMetadata        []ReleaseOrderArtifactMetadataOutput
	ApprovalRecords         []domain.ReleaseOrderApprovalRecord
	ConcurrentBatchProgress *ReleaseOrderConcurrentBatchProgressOutput
	DeploySnapshots         []domain.DeploySnapshot
	AppReleaseState         *domain.AppReleaseState
	LiveStateCanConfirm     bool
}

// GetStoredReleaseOrderByID performs a raw repository read without reconciliation writes.
func (uc *ReleaseOrderManager) GetStoredReleaseOrderByID(
	ctx context.Context,
	orderID string,
) (domain.ReleaseOrder, error) {
	if uc == nil || uc.repo == nil {
		return domain.ReleaseOrder{}, fmt.Errorf("%w: release order manager is not configured", ErrInvalidInput)
	}
	orderID = strings.TrimSpace(orderID)
	if orderID == "" {
		return domain.ReleaseOrder{}, ErrInvalidID
	}
	return uc.repo.GetByID(ctx, orderID)
}

// LoadStoredReleaseOrderRealtimeAggregate loads one coherent API aggregate using storage reads only.
// No method in this path may call Update*/Replace*/Upsert* on the release repository.
func (uc *ReleaseOrderManager) LoadStoredReleaseOrderRealtimeAggregate(
	ctx context.Context,
	orderID string,
) (ReleaseOrderStoredRealtimeAggregate, error) {
	if uc == nil || uc.repo == nil {
		return ReleaseOrderStoredRealtimeAggregate{}, fmt.Errorf("%w: release order manager is not configured", ErrInvalidInput)
	}
	orderID = strings.TrimSpace(orderID)
	if orderID == "" {
		return ReleaseOrderStoredRealtimeAggregate{}, ErrInvalidID
	}
	// Keep the sequential raw reads consistent with tracker/cancel/dispatch within this process.
	// Unlike public detail methods, everything below is read-only, so taking the operation lock
	// here cannot recurse into another lock acquisition.
	unlock := uc.lockOrderOperation(orderID)
	defer unlock()

	order, err := uc.repo.GetByID(ctx, orderID)
	if err != nil {
		return ReleaseOrderStoredRealtimeAggregate{}, err
	}
	executions, err := uc.repo.ListExecutions(ctx, order.ID)
	if err != nil {
		return ReleaseOrderStoredRealtimeAggregate{}, err
	}
	steps, err := uc.repo.ListSteps(ctx, order.ID)
	if err != nil {
		return ReleaseOrderStoredRealtimeAggregate{}, err
	}
	steps = uc.enrichAgentTaskStepDetails(ctx, steps)
	valueProgress, err := uc.ListValueProgress(ctx, order.ID)
	if err != nil {
		return ReleaseOrderStoredRealtimeAggregate{}, err
	}
	stages, err := uc.repo.ListPipelineStages(ctx, order.ID)
	if err != nil {
		return ReleaseOrderStoredRealtimeAggregate{}, err
	}
	artifactItems, err := uc.repo.ListArtifactMetadata(ctx, order.ID)
	if err != nil {
		return ReleaseOrderStoredRealtimeAggregate{}, err
	}
	artifactMetadata := make([]ReleaseOrderArtifactMetadataOutput, 0, len(artifactItems))
	for _, item := range artifactItems {
		artifactMetadata = append(artifactMetadata, toReleaseOrderArtifactMetadataOutput(item))
	}
	approvalRecords, err := uc.repo.ListApprovalRecords(ctx, order.ID)
	if err != nil {
		return ReleaseOrderStoredRealtimeAggregate{}, err
	}
	deploySnapshots, err := uc.repo.ListDeploySnapshotsByOrderID(ctx, order.ID)
	if err != nil {
		return ReleaseOrderStoredRealtimeAggregate{}, err
	}

	var concurrentProgress *ReleaseOrderConcurrentBatchProgressOutput
	if order.IsConcurrent && strings.TrimSpace(order.ConcurrentBatchNo) != "" {
		resolved, progressErr := uc.getStoredConcurrentBatchProgress(ctx, order)
		if progressErr != nil {
			return ReleaseOrderStoredRealtimeAggregate{}, progressErr
		}
		concurrentProgress = &resolved
	}

	var (
		appReleaseState     *domain.AppReleaseState
		liveStateCanConfirm bool
	)
	state, stateErr := uc.repo.GetAppReleaseStateByOrderID(ctx, order.ID)
	if stateErr == nil {
		appReleaseState = &state
		if state.StateStatus == domain.AppReleaseStateStatusPendingConfirm {
			liveStateCanConfirm, err = uc.repo.IsLatestOrderByApplicationEnv(
				ctx,
				state.ApplicationID,
				state.EnvCode,
				state.ReleaseOrderID,
			)
			if err != nil {
				return ReleaseOrderStoredRealtimeAggregate{}, err
			}
		}
	} else if !errors.Is(stateErr, domain.ErrAppReleaseStateNotFound) {
		return ReleaseOrderStoredRealtimeAggregate{}, stateErr
	}

	return ReleaseOrderStoredRealtimeAggregate{
		Order:                   order,
		Executions:              executions,
		Steps:                   steps,
		ValueProgress:           valueProgress,
		PipelineStageView:       buildStoredPipelineStagesView(order, executions, stages),
		ArtifactMetadata:        artifactMetadata,
		ApprovalRecords:         approvalRecords,
		ConcurrentBatchProgress: concurrentProgress,
		DeploySnapshots:         deploySnapshots,
		AppReleaseState:         appReleaseState,
		LiveStateCanConfirm:     liveStateCanConfirm,
	}, nil
}
