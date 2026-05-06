package usecase

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	domain "gos/internal/domain/release"
)

const (
	defaultScheduleTimezone         = "Asia/Shanghai"
	defaultScheduleMinLeadDuration  = 2 * time.Minute
	defaultScheduleStageMinInterval = 5 * time.Minute
)

type CreateReleaseOrderScheduleInput struct {
	ScheduleMode       domain.ScheduleMode
	BuildScheduledAt   *time.Time
	DeployScheduledAt  *time.Time
	ExecuteScheduledAt *time.Time
	Timezone           string
	Remark             string
	CreatorUserID      string
	CreatorName        string
}

type UpdateReleaseOrderScheduleInput = CreateReleaseOrderScheduleInput

type ListReleaseOrderScheduleInput struct {
	ApplicationID          string
	ApplicationIDs         []string
	VisibleToUserID        string
	ApprovalApproverUserID string
	CreatorUserID          string
	Keyword                string
	EnvCode                string
	ScheduleMode           domain.ScheduleMode
	Status                 domain.ScheduleStatus
	ScheduledAtFrom        *time.Time
	ScheduledAtTo          *time.Time
	Page                   int
	PageSize               int
}

type ListSchedulableReleaseOrderInput struct {
	ListReleaseOrderInput
	ScheduleMode domain.ScheduleMode
}

type RunDueReleaseOrderSchedulesOutput struct {
	Scanned    int
	Expired    int
	Dispatched int
	Blocked    int
	Failed     int
	Skipped    int
}

type dueScheduleStage string

const (
	dueScheduleStageBuild   dueScheduleStage = "build"
	dueScheduleStageDeploy  dueScheduleStage = "deploy"
	dueScheduleStageExecute dueScheduleStage = "execute"
)

func (uc *ReleaseOrderManager) ListSchedules(ctx context.Context, input ListReleaseOrderScheduleInput) ([]domain.ReleaseOrderSchedule, int64, error) {
	if input.Page <= 0 {
		input.Page = 1
	}
	if input.PageSize <= 0 {
		input.PageSize = 20
	}
	if input.PageSize > 100 {
		input.PageSize = 100
	}
	return uc.repo.ListSchedules(ctx, domain.ScheduleListFilter{
		ApplicationID:          strings.TrimSpace(input.ApplicationID),
		ApplicationIDs:         normalizeReleaseApplicationIDs(input.ApplicationIDs),
		VisibleToUserID:        strings.TrimSpace(input.VisibleToUserID),
		ApprovalApproverUserID: strings.TrimSpace(input.ApprovalApproverUserID),
		CreatorUserID:          strings.TrimSpace(input.CreatorUserID),
		Keyword:                strings.TrimSpace(input.Keyword),
		EnvCode:                strings.TrimSpace(input.EnvCode),
		ScheduleMode:           input.ScheduleMode,
		Status:                 input.Status,
		ScheduledAtFrom:        input.ScheduledAtFrom,
		ScheduledAtTo:          input.ScheduledAtTo,
		Page:                   input.Page,
		PageSize:               input.PageSize,
	})
}

func (uc *ReleaseOrderManager) ListSchedulableOrders(
	ctx context.Context,
	input ListSchedulableReleaseOrderInput,
) ([]domain.ReleaseOrder, int64, error) {
	if input.ScheduleMode != "" && !input.ScheduleMode.Valid() {
		return nil, 0, fmt.Errorf("%w: invalid schedule_mode", ErrInvalidInput)
	}
	if input.Page <= 0 {
		input.Page = 1
	}
	if input.PageSize <= 0 {
		input.PageSize = 20
	}
	if input.PageSize > 100 {
		input.PageSize = 100
	}
	baseInput := input.ListReleaseOrderInput
	baseInput.PageSize = 100

	eligible := make([]domain.ReleaseOrder, 0)
	for page := 1; ; page++ {
		baseInput.Page = page
		items, total, err := uc.List(ctx, baseInput)
		if err != nil {
			return nil, 0, err
		}
		if len(items) == 0 {
			break
		}
		for _, item := range items {
			var ok bool
			var err error
			if input.ScheduleMode == "" {
				ok, err = uc.isSchedulableOrderForAnyMode(ctx, item)
			} else {
				ok, err = uc.isSchedulableOrderForMode(ctx, item, input.ScheduleMode)
			}
			if err != nil {
				return nil, 0, err
			}
			if !ok {
				continue
			}
			eligible = append(eligible, item)
		}
		if int64(page*baseInput.PageSize) >= total {
			break
		}
	}
	sort.SliceStable(eligible, func(i, j int) bool {
		if !eligible[i].CreatedAt.Equal(eligible[j].CreatedAt) {
			return eligible[i].CreatedAt.Before(eligible[j].CreatedAt)
		}
		return eligible[i].ID < eligible[j].ID
	})
	offset := (input.Page - 1) * input.PageSize
	if offset >= len(eligible) {
		return nil, int64(len(eligible)), nil
	}
	end := offset + input.PageSize
	if end > len(eligible) {
		end = len(eligible)
	}
	return eligible[offset:end], int64(len(eligible)), nil
}

func (uc *ReleaseOrderManager) isSchedulableOrderForAnyMode(ctx context.Context, order domain.ReleaseOrder) (bool, error) {
	if order.Status.IsTerminal() {
		return false, nil
	}
	if _, err := uc.repo.GetActiveScheduleByOrderID(ctx, order.ID); err == nil {
		return false, nil
	} else if !errors.Is(err, domain.ErrScheduleNotFound) {
		return false, err
	}
	executions, err := uc.repo.ListExecutions(ctx, order.ID)
	if err != nil {
		return false, err
	}
	for _, mode := range []domain.ScheduleMode{
		domain.ScheduleModeBuild,
		domain.ScheduleModeDeploy,
		domain.ScheduleModeBuildDeploy,
		domain.ScheduleModeExecute,
	} {
		if err := validateScheduleExecutions(mode, order, executions); err == nil {
			return true, nil
		} else if !errors.Is(err, ErrInvalidInput) && !errors.Is(err, ErrInvalidStatus) {
			return false, err
		}
	}
	return false, nil
}

func (uc *ReleaseOrderManager) isSchedulableOrderForMode(ctx context.Context, order domain.ReleaseOrder, mode domain.ScheduleMode) (bool, error) {
	if order.Status.IsTerminal() {
		return false, nil
	}
	if _, err := uc.repo.GetActiveScheduleByOrderID(ctx, order.ID); err == nil {
		return false, nil
	} else if !errors.Is(err, domain.ErrScheduleNotFound) {
		return false, err
	}
	executions, err := uc.repo.ListExecutions(ctx, order.ID)
	if err != nil {
		return false, err
	}
	if err := validateScheduleExecutions(mode, order, executions); err != nil {
		if errors.Is(err, ErrInvalidInput) || errors.Is(err, ErrInvalidStatus) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (uc *ReleaseOrderManager) GetScheduleByID(ctx context.Context, id string) (domain.ReleaseOrderSchedule, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return domain.ReleaseOrderSchedule{}, ErrInvalidID
	}
	return uc.repo.GetScheduleByID(ctx, id)
}

func (uc *ReleaseOrderManager) GetActiveScheduleByOrderID(ctx context.Context, orderID string) (domain.ReleaseOrderSchedule, error) {
	orderID = strings.TrimSpace(orderID)
	if orderID == "" {
		return domain.ReleaseOrderSchedule{}, ErrInvalidID
	}
	return uc.repo.GetActiveScheduleByOrderID(ctx, orderID)
}

func (uc *ReleaseOrderManager) ListScheduleApprovalRecords(ctx context.Context, scheduleID string) ([]domain.ReleaseOrderScheduleApprovalRecord, error) {
	scheduleID = strings.TrimSpace(scheduleID)
	if scheduleID == "" {
		return nil, ErrInvalidID
	}
	if _, err := uc.repo.GetScheduleByID(ctx, scheduleID); err != nil {
		return nil, err
	}
	return uc.repo.ListScheduleApprovalRecords(ctx, scheduleID)
}

func (uc *ReleaseOrderManager) CreateSchedule(
	ctx context.Context,
	orderID string,
	input CreateReleaseOrderScheduleInput,
) (domain.ReleaseOrderSchedule, error) {
	orderID = strings.TrimSpace(orderID)
	if orderID == "" {
		return domain.ReleaseOrderSchedule{}, ErrInvalidID
	}
	order, err := uc.repo.GetByID(ctx, orderID)
	if err != nil {
		return domain.ReleaseOrderSchedule{}, err
	}
	executions, err := uc.repo.ListExecutions(ctx, order.ID)
	if err != nil {
		return domain.ReleaseOrderSchedule{}, err
	}
	template, _, _, _, _, err := uc.repo.GetTemplateByID(ctx, strings.TrimSpace(order.TemplateID))
	if err != nil {
		return domain.ReleaseOrderSchedule{}, err
	}
	if _, err := uc.repo.GetActiveScheduleByOrderID(ctx, order.ID); err == nil {
		return domain.ReleaseOrderSchedule{}, fmt.Errorf("%w: release order already has an active schedule", ErrReferencedConflict)
	} else if !errors.Is(err, domain.ErrScheduleNotFound) {
		return domain.ReleaseOrderSchedule{}, err
	}

	now := uc.now()
	mode := input.ScheduleMode
	if !mode.Valid() {
		return domain.ReleaseOrderSchedule{}, fmt.Errorf("%w: invalid schedule_mode", ErrInvalidInput)
	}
	if err := validateScheduleTimes(mode, input.BuildScheduledAt, input.DeployScheduledAt, input.ExecuteScheduledAt, now); err != nil {
		return domain.ReleaseOrderSchedule{}, err
	}
	if err := validateScheduleExecutions(mode, order, executions); err != nil {
		return domain.ReleaseOrderSchedule{}, err
	}
	cdConflictAt := resolveScheduleCDConflictAt(mode, input.DeployScheduledAt, input.ExecuteScheduledAt)
	if cdConflictAt != nil {
		if conflict, conflictErr := uc.repo.FindActiveScheduleCDConflict(ctx, order.ApplicationID, order.EnvCode, *cdConflictAt, ""); conflictErr == nil {
			return domain.ReleaseOrderSchedule{}, fmt.Errorf("%w: 同一应用同一环境在该 CD 时间已存在预约发布 %s", ErrConcurrentReleaseBlocked, conflict.ScheduleNo)
		} else if !errors.Is(conflictErr, domain.ErrScheduleNotFound) {
			return domain.ReleaseOrderSchedule{}, conflictErr
		}
	}

	status := domain.ScheduleStatusScheduled
	var approvedAt *time.Time
	approvedBy := ""
	if template.ApprovalEnabled {
		if shouldAutoApproveOnCreate(template.ApprovalEnabled, template.ApprovalApproverIDs, strings.TrimSpace(input.CreatorUserID)) {
			approvedAt = &now
			approvedBy = firstNonEmpty(strings.TrimSpace(input.CreatorName), strings.TrimSpace(input.CreatorUserID))
		} else {
			status = domain.ScheduleStatusApproving
		}
	}
	item := domain.ReleaseOrderSchedule{
		ID:                    generateID("rosch"),
		ScheduleNo:            generateScheduleNo(now),
		ReleaseOrderID:        order.ID,
		ReleaseOrderNo:        order.OrderNo,
		ApplicationID:         order.ApplicationID,
		ApplicationName:       order.ApplicationName,
		EnvCode:               order.EnvCode,
		TemplateID:            order.TemplateID,
		TemplateName:          order.TemplateName,
		ScheduleMode:          mode,
		BuildScheduledAt:      cloneTime(input.BuildScheduledAt),
		DeployScheduledAt:     cloneTime(input.DeployScheduledAt),
		ExecuteScheduledAt:    cloneTime(input.ExecuteScheduledAt),
		CDConflictAt:          cloneTime(cdConflictAt),
		Timezone:              firstNonEmpty(strings.TrimSpace(input.Timezone), defaultScheduleTimezone),
		Status:                status,
		ApprovalRequired:      template.ApprovalEnabled,
		ApprovalMode:          template.ApprovalMode,
		ApprovalApproverIDs:   append([]string(nil), template.ApprovalApproverIDs...),
		ApprovalApproverNames: append([]string(nil), template.ApprovalApproverNames...),
		ApprovedAt:            approvedAt,
		ApprovedBy:            approvedBy,
		Remark:                strings.TrimSpace(input.Remark),
		CreatorUserID:         strings.TrimSpace(input.CreatorUserID),
		CreatorName:           strings.TrimSpace(input.CreatorName),
		CreatedAt:             now,
		UpdatedAt:             now,
	}
	if err := uc.repo.CreateSchedule(ctx, item); err != nil {
		return domain.ReleaseOrderSchedule{}, err
	}
	if item.Status == domain.ScheduleStatusApproving {
		if err := uc.repo.CreateScheduleApprovalRecord(ctx, domain.ReleaseOrderScheduleApprovalRecord{
			ID:             generateID("rosar"),
			ScheduleID:     item.ID,
			Action:         domain.ReleaseOrderApprovalActionSubmit,
			OperatorUserID: strings.TrimSpace(input.CreatorUserID),
			OperatorName:   firstNonEmpty(strings.TrimSpace(input.CreatorName), strings.TrimSpace(input.CreatorUserID)),
			Comment:        strings.TrimSpace(input.Remark),
			CreatedAt:      now,
		}); err != nil {
			return domain.ReleaseOrderSchedule{}, err
		}
	}
	return uc.repo.GetScheduleByID(ctx, item.ID)
}

func (uc *ReleaseOrderManager) UpdateSchedule(
	ctx context.Context,
	scheduleID string,
	input UpdateReleaseOrderScheduleInput,
) (domain.ReleaseOrderSchedule, error) {
	scheduleID = strings.TrimSpace(scheduleID)
	if scheduleID == "" {
		return domain.ReleaseOrderSchedule{}, ErrInvalidID
	}
	existing, err := uc.repo.GetScheduleByID(ctx, scheduleID)
	if err != nil {
		return domain.ReleaseOrderSchedule{}, err
	}
	if existing.Status != domain.ScheduleStatusPendingApproval && existing.Status != domain.ScheduleStatusApproving {
		return domain.ReleaseOrderSchedule{}, fmt.Errorf("%w: approved or terminal schedule cannot be edited", ErrInvalidStatus)
	}
	order, err := uc.repo.GetByID(ctx, existing.ReleaseOrderID)
	if err != nil {
		return domain.ReleaseOrderSchedule{}, err
	}
	executions, err := uc.repo.ListExecutions(ctx, order.ID)
	if err != nil {
		return domain.ReleaseOrderSchedule{}, err
	}
	template, _, _, _, _, err := uc.repo.GetTemplateByID(ctx, strings.TrimSpace(order.TemplateID))
	if err != nil {
		return domain.ReleaseOrderSchedule{}, err
	}
	now := uc.now()
	if !input.ScheduleMode.Valid() {
		return domain.ReleaseOrderSchedule{}, fmt.Errorf("%w: invalid schedule_mode", ErrInvalidInput)
	}
	if err := validateScheduleTimes(input.ScheduleMode, input.BuildScheduledAt, input.DeployScheduledAt, input.ExecuteScheduledAt, now); err != nil {
		return domain.ReleaseOrderSchedule{}, err
	}
	if err := validateScheduleExecutions(input.ScheduleMode, order, executions); err != nil {
		return domain.ReleaseOrderSchedule{}, err
	}
	cdConflictAt := resolveScheduleCDConflictAt(input.ScheduleMode, input.DeployScheduledAt, input.ExecuteScheduledAt)
	if cdConflictAt != nil {
		if conflict, conflictErr := uc.repo.FindActiveScheduleCDConflict(ctx, order.ApplicationID, order.EnvCode, *cdConflictAt, existing.ID); conflictErr == nil {
			return domain.ReleaseOrderSchedule{}, fmt.Errorf("%w: 同一应用同一环境在该 CD 时间已存在预约发布 %s", ErrConcurrentReleaseBlocked, conflict.ScheduleNo)
		} else if !errors.Is(conflictErr, domain.ErrScheduleNotFound) {
			return domain.ReleaseOrderSchedule{}, conflictErr
		}
	}

	existing.ScheduleMode = input.ScheduleMode
	existing.BuildScheduledAt = cloneTime(input.BuildScheduledAt)
	existing.DeployScheduledAt = cloneTime(input.DeployScheduledAt)
	existing.ExecuteScheduledAt = cloneTime(input.ExecuteScheduledAt)
	existing.CDConflictAt = cloneTime(cdConflictAt)
	existing.Timezone = firstNonEmpty(strings.TrimSpace(input.Timezone), defaultScheduleTimezone)
	existing.Status = domain.ScheduleStatusScheduled
	var submitted bool
	if template.ApprovalEnabled {
		if shouldAutoApproveOnCreate(template.ApprovalEnabled, template.ApprovalApproverIDs, strings.TrimSpace(input.CreatorUserID)) {
			existing.ApprovedAt = &now
			existing.ApprovedBy = firstNonEmpty(strings.TrimSpace(input.CreatorName), strings.TrimSpace(input.CreatorUserID))
		} else {
			existing.Status = domain.ScheduleStatusApproving
			submitted = true
		}
	}
	existing.ApprovalRequired = template.ApprovalEnabled
	existing.ApprovalMode = template.ApprovalMode
	existing.ApprovalApproverIDs = append([]string(nil), template.ApprovalApproverIDs...)
	existing.ApprovalApproverNames = append([]string(nil), template.ApprovalApproverNames...)
	if !template.ApprovalEnabled || submitted {
		existing.ApprovedAt = nil
		existing.ApprovedBy = ""
	}
	existing.RejectedAt = nil
	existing.RejectedBy = ""
	existing.RejectedReason = ""
	existing.LastError = ""
	existing.Remark = strings.TrimSpace(input.Remark)
	existing.UpdatedAt = now
	if err := uc.repo.UpdateSchedule(ctx, existing); err != nil {
		return domain.ReleaseOrderSchedule{}, err
	}
	if submitted {
		if err := uc.repo.CreateScheduleApprovalRecord(ctx, domain.ReleaseOrderScheduleApprovalRecord{
			ID:             generateID("rosar"),
			ScheduleID:     existing.ID,
			Action:         domain.ReleaseOrderApprovalActionSubmit,
			OperatorUserID: strings.TrimSpace(input.CreatorUserID),
			OperatorName:   firstNonEmpty(strings.TrimSpace(input.CreatorName), strings.TrimSpace(input.CreatorUserID)),
			Comment:        strings.TrimSpace(input.Remark),
			CreatedAt:      now,
		}); err != nil {
			return domain.ReleaseOrderSchedule{}, err
		}
	}
	return uc.repo.GetScheduleByID(ctx, existing.ID)
}

func (uc *ReleaseOrderManager) SubmitScheduleApproval(
	ctx context.Context,
	scheduleID string,
	operatorUserID string,
	operatorName string,
	comment string,
) (domain.ReleaseOrderSchedule, error) {
	schedule, err := uc.getScheduleForApprovalAction(ctx, scheduleID, operatorUserID)
	if err != nil {
		return domain.ReleaseOrderSchedule{}, err
	}
	if !schedule.ApprovalRequired {
		return domain.ReleaseOrderSchedule{}, fmt.Errorf("%w: current schedule does not require approval", ErrInvalidInput)
	}
	if schedule.Status != domain.ScheduleStatusPendingApproval {
		return domain.ReleaseOrderSchedule{}, fmt.Errorf("%w: schedule cannot submit approval in current status", ErrInvalidStatus)
	}
	now := uc.now()
	if err := uc.repo.CreateScheduleApprovalRecord(ctx, domain.ReleaseOrderScheduleApprovalRecord{
		ID:             generateID("rosar"),
		ScheduleID:     schedule.ID,
		Action:         domain.ReleaseOrderApprovalActionSubmit,
		OperatorUserID: strings.TrimSpace(operatorUserID),
		OperatorName:   firstNonEmpty(strings.TrimSpace(operatorName), strings.TrimSpace(operatorUserID)),
		Comment:        strings.TrimSpace(comment),
		CreatedAt:      now,
	}); err != nil {
		return domain.ReleaseOrderSchedule{}, err
	}
	schedule.Status = domain.ScheduleStatusApproving
	schedule.UpdatedAt = now
	if err := uc.repo.UpdateSchedule(ctx, schedule); err != nil {
		return domain.ReleaseOrderSchedule{}, err
	}
	return uc.repo.GetScheduleByID(ctx, schedule.ID)
}

func (uc *ReleaseOrderManager) ApproveSchedule(
	ctx context.Context,
	scheduleID string,
	operatorUserID string,
	operatorName string,
	comment string,
) (domain.ReleaseOrderSchedule, error) {
	schedule, err := uc.getScheduleForApprovalAction(ctx, scheduleID, operatorUserID)
	if err != nil {
		return domain.ReleaseOrderSchedule{}, err
	}
	if !schedule.ApprovalRequired {
		return domain.ReleaseOrderSchedule{}, fmt.Errorf("%w: current schedule does not require approval", ErrInvalidInput)
	}
	if schedule.Status != domain.ScheduleStatusPendingApproval && schedule.Status != domain.ScheduleStatusApproving {
		return domain.ReleaseOrderSchedule{}, fmt.Errorf("%w: schedule cannot be approved in current status", ErrInvalidStatus)
	}
	if !approvalIncludesUser(schedule.ApprovalApproverIDs, operatorUserID) {
		return domain.ReleaseOrderSchedule{}, fmt.Errorf("%w: current user is not in approval approver list", ErrInvalidInput)
	}
	records, err := uc.repo.ListScheduleApprovalRecords(ctx, schedule.ID)
	if err != nil {
		return domain.ReleaseOrderSchedule{}, err
	}
	if scheduleApprovalAlreadyActed(records, operatorUserID, domain.ReleaseOrderApprovalActionApprove) {
		return domain.ReleaseOrderSchedule{}, fmt.Errorf("%w: current approver has already approved", ErrInvalidInput)
	}
	if scheduleApprovalAlreadyActed(records, operatorUserID, domain.ReleaseOrderApprovalActionReject) {
		return domain.ReleaseOrderSchedule{}, fmt.Errorf("%w: current approver has already rejected", ErrInvalidInput)
	}
	now := uc.now()
	if err := uc.repo.CreateScheduleApprovalRecord(ctx, domain.ReleaseOrderScheduleApprovalRecord{
		ID:             generateID("rosar"),
		ScheduleID:     schedule.ID,
		Action:         domain.ReleaseOrderApprovalActionApprove,
		OperatorUserID: strings.TrimSpace(operatorUserID),
		OperatorName:   firstNonEmpty(strings.TrimSpace(operatorName), strings.TrimSpace(operatorUserID)),
		Comment:        strings.TrimSpace(comment),
		CreatedAt:      now,
	}); err != nil {
		return domain.ReleaseOrderSchedule{}, err
	}
	if schedule.ApprovalMode == domain.TemplateApprovalModeAll && !scheduleApprovalAllApproversApproved(records, schedule.ApprovalApproverIDs, operatorUserID) {
		schedule.Status = domain.ScheduleStatusApproving
	} else {
		schedule.Status = domain.ScheduleStatusScheduled
		schedule.ApprovedAt = &now
		schedule.ApprovedBy = firstNonEmpty(strings.TrimSpace(operatorName), strings.TrimSpace(operatorUserID))
	}
	schedule.UpdatedAt = now
	if err := uc.repo.UpdateSchedule(ctx, schedule); err != nil {
		return domain.ReleaseOrderSchedule{}, err
	}
	return uc.repo.GetScheduleByID(ctx, schedule.ID)
}

func (uc *ReleaseOrderManager) RejectSchedule(
	ctx context.Context,
	scheduleID string,
	operatorUserID string,
	operatorName string,
	comment string,
) (domain.ReleaseOrderSchedule, error) {
	comment = strings.TrimSpace(comment)
	if comment == "" {
		return domain.ReleaseOrderSchedule{}, fmt.Errorf("%w: reject reason is required", ErrInvalidInput)
	}
	schedule, err := uc.getScheduleForApprovalAction(ctx, scheduleID, operatorUserID)
	if err != nil {
		return domain.ReleaseOrderSchedule{}, err
	}
	if schedule.Status != domain.ScheduleStatusPendingApproval && schedule.Status != domain.ScheduleStatusApproving {
		return domain.ReleaseOrderSchedule{}, fmt.Errorf("%w: schedule cannot be rejected in current status", ErrInvalidStatus)
	}
	if !approvalIncludesUser(schedule.ApprovalApproverIDs, operatorUserID) {
		return domain.ReleaseOrderSchedule{}, fmt.Errorf("%w: current user is not in approval approver list", ErrInvalidInput)
	}
	now := uc.now()
	if err := uc.repo.CreateScheduleApprovalRecord(ctx, domain.ReleaseOrderScheduleApprovalRecord{
		ID:             generateID("rosar"),
		ScheduleID:     schedule.ID,
		Action:         domain.ReleaseOrderApprovalActionReject,
		OperatorUserID: strings.TrimSpace(operatorUserID),
		OperatorName:   firstNonEmpty(strings.TrimSpace(operatorName), strings.TrimSpace(operatorUserID)),
		Comment:        comment,
		CreatedAt:      now,
	}); err != nil {
		return domain.ReleaseOrderSchedule{}, err
	}
	schedule.Status = domain.ScheduleStatusRejected
	schedule.RejectedAt = &now
	schedule.RejectedBy = firstNonEmpty(strings.TrimSpace(operatorName), strings.TrimSpace(operatorUserID))
	schedule.RejectedReason = comment
	schedule.UpdatedAt = now
	if err := uc.repo.UpdateSchedule(ctx, schedule); err != nil {
		return domain.ReleaseOrderSchedule{}, err
	}
	return uc.repo.GetScheduleByID(ctx, schedule.ID)
}

func (uc *ReleaseOrderManager) CancelSchedule(ctx context.Context, scheduleID string, operatorName string) (domain.ReleaseOrderSchedule, error) {
	scheduleID = strings.TrimSpace(scheduleID)
	if scheduleID == "" {
		return domain.ReleaseOrderSchedule{}, ErrInvalidID
	}
	schedule, err := uc.repo.GetScheduleByID(ctx, scheduleID)
	if err != nil {
		return domain.ReleaseOrderSchedule{}, err
	}
	switch schedule.Status {
	case domain.ScheduleStatusPendingApproval, domain.ScheduleStatusApproving, domain.ScheduleStatusScheduled:
	default:
		return domain.ReleaseOrderSchedule{}, fmt.Errorf("%w: schedule cannot be cancelled in current status", ErrInvalidStatus)
	}
	now := uc.now()
	schedule.Status = domain.ScheduleStatusCancelled
	schedule.CancelledAt = &now
	schedule.CancelledBy = strings.TrimSpace(operatorName)
	schedule.UpdatedAt = now
	if err := uc.repo.UpdateSchedule(ctx, schedule); err != nil {
		return domain.ReleaseOrderSchedule{}, err
	}
	return uc.repo.GetScheduleByID(ctx, schedule.ID)
}

func (uc *ReleaseOrderManager) RunDueSchedules(ctx context.Context, limit int) (RunDueReleaseOrderSchedulesOutput, error) {
	if limit <= 0 {
		limit = 50
	}
	now := uc.now()
	items, err := uc.repo.ListDueSchedules(ctx, now, limit)
	if err != nil {
		return RunDueReleaseOrderSchedulesOutput{}, err
	}
	output := RunDueReleaseOrderSchedulesOutput{Scanned: len(items)}
	for _, item := range items {
		switch item.Status {
		case domain.ScheduleStatusPendingApproval, domain.ScheduleStatusApproving:
			item.Status = domain.ScheduleStatusExpired
			item.ExpiredAt = &now
			item.LastError = "预约时间已到，但预约审批未通过"
			item.UpdatedAt = now
			if err := uc.repo.UpdateSchedule(ctx, item); err != nil {
				return output, err
			}
			output.Expired++
		case domain.ScheduleStatusScheduled:
			stage := resolveDueScheduleStage(item, now)
			if stage == "" {
				continue
			}
			result, dispatchErr := uc.dispatchDueScheduleStage(ctx, item, stage, now)
			if dispatchErr != nil {
				return output, dispatchErr
			}
			switch result {
			case domain.ScheduleStatusDispatched:
				output.Dispatched++
			case domain.ScheduleStatusBlocked:
				output.Blocked++
			case domain.ScheduleStatusFailed:
				output.Failed++
			case domain.ScheduleStatusSkipped:
				output.Skipped++
			case domain.ScheduleStatusScheduled:
				output.Dispatched++
			}
		}
	}
	return output, nil
}

func (uc *ReleaseOrderManager) dispatchDueScheduleStage(
	ctx context.Context,
	schedule domain.ReleaseOrderSchedule,
	stage dueScheduleStage,
	now time.Time,
) (domain.ScheduleStatus, error) {
	schedule.Status = domain.ScheduleStatusDispatching
	schedule.UpdatedAt = now
	if err := uc.repo.UpdateSchedule(ctx, schedule); err != nil {
		return "", err
	}

	var dispatchErr error
	switch stage {
	case dueScheduleStageBuild:
		_, dispatchErr = uc.Build(ctx, schedule.ReleaseOrderID, schedule.CreatorUserID, schedule.CreatorName)
	case dueScheduleStageDeploy:
		_, dispatchErr = uc.Deploy(ctx, schedule.ReleaseOrderID, schedule.CreatorUserID, schedule.CreatorName)
	case dueScheduleStageExecute:
		_, dispatchErr = uc.Execute(ctx, schedule.ReleaseOrderID, schedule.CreatorUserID, schedule.CreatorName)
	default:
		dispatchErr = fmt.Errorf("%w: invalid due schedule stage", ErrInvalidInput)
	}

	current, err := uc.repo.GetScheduleByID(ctx, schedule.ID)
	if err != nil {
		return "", err
	}
	current.UpdatedAt = uc.now()
	if dispatchErr != nil {
		if isScheduleBusinessBlocked(dispatchErr) {
			current.Status = domain.ScheduleStatusBlocked
			current.LastError = dispatchErr.Error()
			if err := uc.repo.UpdateSchedule(ctx, current); err != nil {
				return "", err
			}
			return domain.ScheduleStatusBlocked, nil
		}
		current.Status = domain.ScheduleStatusFailed
		current.LastError = dispatchErr.Error()
		if err := uc.repo.UpdateSchedule(ctx, current); err != nil {
			return "", err
		}
		return domain.ScheduleStatusFailed, nil
	}

	switch stage {
	case dueScheduleStageBuild:
		current.BuildDispatchedAt = &now
		if current.ScheduleMode == domain.ScheduleModeBuildDeploy {
			current.Status = domain.ScheduleStatusScheduled
		} else {
			current.Status = domain.ScheduleStatusDispatched
		}
	case dueScheduleStageDeploy:
		current.DeployDispatchedAt = &now
		current.Status = domain.ScheduleStatusDispatched
	case dueScheduleStageExecute:
		current.ExecuteDispatchedAt = &now
		current.Status = domain.ScheduleStatusDispatched
	}
	current.LastError = ""
	if err := uc.repo.UpdateSchedule(ctx, current); err != nil {
		return "", err
	}
	return current.Status, nil
}

func resolveDueScheduleStage(schedule domain.ReleaseOrderSchedule, now time.Time) dueScheduleStage {
	switch schedule.ScheduleMode {
	case domain.ScheduleModeBuild:
		if schedule.BuildDispatchedAt == nil && dueAt(schedule.BuildScheduledAt, now) {
			return dueScheduleStageBuild
		}
	case domain.ScheduleModeDeploy:
		if schedule.DeployDispatchedAt == nil && dueAt(schedule.DeployScheduledAt, now) {
			return dueScheduleStageDeploy
		}
	case domain.ScheduleModeBuildDeploy:
		if schedule.BuildDispatchedAt == nil && dueAt(schedule.BuildScheduledAt, now) {
			return dueScheduleStageBuild
		}
		if schedule.BuildDispatchedAt != nil && schedule.DeployDispatchedAt == nil && dueAt(schedule.DeployScheduledAt, now) {
			return dueScheduleStageDeploy
		}
	case domain.ScheduleModeExecute:
		if schedule.ExecuteDispatchedAt == nil && dueAt(schedule.ExecuteScheduledAt, now) {
			return dueScheduleStageExecute
		}
	}
	return ""
}

func dueAt(value *time.Time, now time.Time) bool {
	return value != nil && !value.After(now)
}

func isScheduleBusinessBlocked(err error) bool {
	return errors.Is(err, ErrInvalidInput) ||
		errors.Is(err, ErrInvalidStatus) ||
		errors.Is(err, ErrConcurrentReleaseBlocked)
}

func validateScheduleTimes(
	mode domain.ScheduleMode,
	buildAt *time.Time,
	deployAt *time.Time,
	executeAt *time.Time,
	now time.Time,
) error {
	minAt := now.Add(defaultScheduleMinLeadDuration)
	checkFuture := func(label string, value *time.Time) error {
		if value == nil {
			return fmt.Errorf("%w: %s is required", ErrInvalidInput, label)
		}
		if value.Before(minAt) {
			return fmt.Errorf("%w: %s must be at least %s in the future", ErrInvalidInput, label, defaultScheduleMinLeadDuration)
		}
		return nil
	}
	switch mode {
	case domain.ScheduleModeBuild:
		if deployAt != nil || executeAt != nil {
			return fmt.Errorf("%w: build schedule only accepts build_scheduled_at", ErrInvalidInput)
		}
		return checkFuture("build_scheduled_at", buildAt)
	case domain.ScheduleModeDeploy:
		if buildAt != nil || executeAt != nil {
			return fmt.Errorf("%w: deploy schedule only accepts deploy_scheduled_at", ErrInvalidInput)
		}
		return checkFuture("deploy_scheduled_at", deployAt)
	case domain.ScheduleModeBuildDeploy:
		if executeAt != nil {
			return fmt.Errorf("%w: build_deploy schedule does not accept execute_scheduled_at", ErrInvalidInput)
		}
		if err := checkFuture("build_scheduled_at", buildAt); err != nil {
			return err
		}
		if err := checkFuture("deploy_scheduled_at", deployAt); err != nil {
			return err
		}
		if !deployAt.After(*buildAt) || deployAt.Sub(*buildAt) < defaultScheduleStageMinInterval {
			return fmt.Errorf("%w: deploy_scheduled_at must be after build_scheduled_at by at least %s", ErrInvalidInput, defaultScheduleStageMinInterval)
		}
		return nil
	case domain.ScheduleModeExecute:
		if buildAt != nil || deployAt != nil {
			return fmt.Errorf("%w: execute schedule only accepts execute_scheduled_at", ErrInvalidInput)
		}
		return checkFuture("execute_scheduled_at", executeAt)
	default:
		return fmt.Errorf("%w: invalid schedule_mode", ErrInvalidInput)
	}
}

func validateScheduleExecutions(
	mode domain.ScheduleMode,
	order domain.ReleaseOrder,
	executions []domain.ReleaseOrderExecution,
) error {
	if order.Status.IsTerminal() {
		return fmt.Errorf("%w: terminal release order cannot be scheduled", ErrInvalidInput)
	}
	switch mode {
	case domain.ScheduleModeBuild:
		if findExecutionByScopeAndStatus(executions, domain.PipelineScopeCI, domain.ExecutionStatusPending) == nil {
			return fmt.Errorf("%w: release order has no pending CI execution", ErrInvalidInput)
		}
	case domain.ScheduleModeDeploy:
		if findExecutionByScopeAndStatus(executions, domain.PipelineScopeCD, domain.ExecutionStatusPending) == nil {
			return fmt.Errorf("%w: release order has no pending CD execution", ErrInvalidInput)
		}
	case domain.ScheduleModeBuildDeploy:
		if findExecutionByScopeAndStatus(executions, domain.PipelineScopeCI, domain.ExecutionStatusPending) == nil ||
			findExecutionByScopeAndStatus(executions, domain.PipelineScopeCD, domain.ExecutionStatusPending) == nil {
			return fmt.Errorf("%w: release order must have both pending CI and CD executions", ErrInvalidInput)
		}
	case domain.ScheduleModeExecute:
		hasPending := false
		for _, item := range executions {
			if item.Status == domain.ExecutionStatusPending {
				hasPending = true
				break
			}
		}
		if !hasPending {
			return fmt.Errorf("%w: release order has no pending execution", ErrInvalidInput)
		}
	}
	return nil
}

func resolveScheduleCDConflictAt(mode domain.ScheduleMode, deployAt *time.Time, executeAt *time.Time) *time.Time {
	switch mode {
	case domain.ScheduleModeDeploy, domain.ScheduleModeBuildDeploy:
		return deployAt
	case domain.ScheduleModeExecute:
		return executeAt
	default:
		return nil
	}
}

func (uc *ReleaseOrderManager) getScheduleForApprovalAction(ctx context.Context, scheduleID string, operatorUserID string) (domain.ReleaseOrderSchedule, error) {
	scheduleID = strings.TrimSpace(scheduleID)
	operatorUserID = strings.TrimSpace(operatorUserID)
	if scheduleID == "" {
		return domain.ReleaseOrderSchedule{}, ErrInvalidID
	}
	if operatorUserID == "" {
		return domain.ReleaseOrderSchedule{}, fmt.Errorf("%w: operator_user_id is required", ErrInvalidInput)
	}
	return uc.repo.GetScheduleByID(ctx, scheduleID)
}

func scheduleApprovalAlreadyActed(records []domain.ReleaseOrderScheduleApprovalRecord, operatorUserID string, action domain.ReleaseOrderApprovalAction) bool {
	operatorUserID = strings.TrimSpace(operatorUserID)
	for _, item := range records {
		if strings.TrimSpace(item.OperatorUserID) != operatorUserID {
			continue
		}
		if item.Action == action {
			return true
		}
	}
	return false
}

func scheduleApprovalAllApproversApproved(
	records []domain.ReleaseOrderScheduleApprovalRecord,
	approverIDs []string,
	currentOperatorUserID string,
) bool {
	approved := make(map[string]struct{}, len(approverIDs)+1)
	for _, item := range records {
		if item.Action != domain.ReleaseOrderApprovalActionApprove {
			continue
		}
		userID := strings.TrimSpace(item.OperatorUserID)
		if userID == "" {
			continue
		}
		approved[userID] = struct{}{}
	}
	if userID := strings.TrimSpace(currentOperatorUserID); userID != "" {
		approved[userID] = struct{}{}
	}
	for _, item := range approverIDs {
		userID := strings.TrimSpace(item)
		if userID == "" {
			continue
		}
		if _, ok := approved[userID]; !ok {
			return false
		}
	}
	return true
}

func cloneTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	cloned := value.UTC()
	return &cloned
}

func generateScheduleNo(now time.Time) string {
	return "RS-" + strings.TrimPrefix(generateOrderNo(now), "RO-")
}
