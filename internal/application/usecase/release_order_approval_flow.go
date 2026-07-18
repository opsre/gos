package usecase

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"time"

	agentdomain "gos/internal/domain/agent"
	domain "gos/internal/domain/release"
	"gos/internal/support/logx"
)

type SaveApprovalFlowDefinitionInput struct {
	ID     string
	Name   string
	Status domain.ApprovalFlowStatus
	Nodes  []domain.ApprovalFlowNode
	Links  []domain.ApprovalFlowLink
}

type approvalFlowBindingLookupRepository interface {
	GetApplicationApprovalFlowBinding(ctx context.Context, applicationID string) (approvalFlowID string, exists bool, err error)
}

type approvalFlowInstanceDeleteRepository interface {
	DeleteApprovalFlowInstance(ctx context.Context, releaseOrderID string) error
}

// CreateApprovalFlowDefinition 创建线性发布单审批流定义。
func (uc *ReleaseOrderManager) CreateApprovalFlowDefinition(ctx context.Context, input SaveApprovalFlowDefinitionInput) (domain.ApprovalFlowDefinition, error) {
	nodes, err := normalizeApprovalFlowNodes(input.Nodes)
	if err != nil {
		return domain.ApprovalFlowDefinition{}, err
	}
	nodes, err = uc.validateApprovalFlowAgentTaskNodes(ctx, nodes)
	if err != nil {
		return domain.ApprovalFlowDefinition{}, err
	}
	name := strings.TrimSpace(input.Name)
	if name == "" || !input.Status.Valid() {
		return domain.ApprovalFlowDefinition{}, fmt.Errorf("%w: approval flow name, status and nodes are required", ErrInvalidInput)
	}
	now := uc.now()
	links, err := normalizeApprovalFlowLinks(nodes, input.Links)
	if err != nil {
		return domain.ApprovalFlowDefinition{}, err
	}
	item := domain.ApprovalFlowDefinition{ID: generateID("raf"), Name: name, Status: input.Status, Nodes: nodes, Links: links, CreatedAt: now, UpdatedAt: now}
	if err := uc.repo.CreateApprovalFlowDefinition(ctx, item); err != nil {
		return domain.ApprovalFlowDefinition{}, err
	}
	return item, nil
}

func (uc *ReleaseOrderManager) UpdateApprovalFlowDefinition(ctx context.Context, input SaveApprovalFlowDefinitionInput) (domain.ApprovalFlowDefinition, error) {
	if strings.TrimSpace(input.ID) == "" {
		return domain.ApprovalFlowDefinition{}, ErrInvalidID
	}
	nodes, err := normalizeApprovalFlowNodes(input.Nodes)
	if err != nil {
		return domain.ApprovalFlowDefinition{}, err
	}
	nodes, err = uc.validateApprovalFlowAgentTaskNodes(ctx, nodes)
	if err != nil {
		return domain.ApprovalFlowDefinition{}, err
	}
	name := strings.TrimSpace(input.Name)
	if name == "" || !input.Status.Valid() {
		return domain.ApprovalFlowDefinition{}, fmt.Errorf("%w: approval flow name, status and nodes are required", ErrInvalidInput)
	}
	existing, err := uc.repo.GetApprovalFlowDefinitionByID(ctx, input.ID)
	if err != nil {
		return domain.ApprovalFlowDefinition{}, err
	}
	links, err := normalizeApprovalFlowLinks(nodes, input.Links)
	if err != nil {
		return domain.ApprovalFlowDefinition{}, err
	}
	existing.Name, existing.Status, existing.Nodes, existing.Links, existing.UpdatedAt = name, input.Status, nodes, links, uc.now()
	if err := uc.repo.UpdateApprovalFlowDefinition(ctx, existing); err != nil {
		return domain.ApprovalFlowDefinition{}, err
	}
	return existing, nil
}

func normalizeApprovalFlowLinks(nodes []domain.ApprovalFlowNode, links []domain.ApprovalFlowLink) ([]domain.ApprovalFlowLink, error) {
	known := map[string]bool{"start": true, "waiting_deploy": true, "end": true}
	for _, node := range nodes {
		known[node.Code] = true
	}
	if len(links) == 0 {
		return nil, nil
	}
	result := make([]domain.ApprovalFlowLink, 0, len(links))
	seen := map[string]bool{}
	for index, link := range links {
		link.FromCode, link.ToCode = strings.TrimSpace(link.FromCode), strings.TrimSpace(link.ToCode)
		if link.FromCode == "" || link.ToCode == "" || link.FromCode == link.ToCode || !known[link.FromCode] || !known[link.ToCode] || link.ToCode == "start" || link.FromCode == "end" {
			return nil, fmt.Errorf("%w: invalid approval flow link", ErrInvalidInput)
		}
		key := link.FromCode + "->" + link.ToCode
		if seen[key] {
			return nil, fmt.Errorf("%w: duplicated approval flow link", ErrInvalidInput)
		}
		seen[key] = true
		scopes := make([]string, 0, len(link.ExecutionScopes))
		scopeSeen := map[string]struct{}{}
		for _, rawScope := range link.ExecutionScopes {
			scope := domain.ApprovalFlowExecutionScope(strings.TrimSpace(rawScope))
			if !scope.Valid() {
				return nil, fmt.Errorf("%w: invalid approval flow execution scope", ErrInvalidInput)
			}
			if _, exists := scopeSeen[string(scope)]; exists {
				continue
			}
			scopeSeen[string(scope)] = struct{}{}
			scopes = append(scopes, string(scope))
		}
		link.ExecutionScopes = scopes
		if link.Priority == 0 {
			link.Priority = index + 1
		}
		result = append(result, link)
	}
	return result, nil
}

func (uc *ReleaseOrderManager) ListApprovalFlowDefinitions(ctx context.Context, status domain.ApprovalFlowStatus) ([]domain.ApprovalFlowDefinition, error) {
	if status != "" && !status.Valid() {
		return nil, ErrInvalidStatus
	}
	return uc.repo.ListApprovalFlowDefinitions(ctx, status)
}

func (uc *ReleaseOrderManager) GetApplicationApprovalFlowID(ctx context.Context, applicationID string) (string, error) {
	if strings.TrimSpace(applicationID) == "" {
		return "", ErrInvalidID
	}
	return uc.repo.GetApplicationApprovalFlowID(ctx, applicationID)
}

func (uc *ReleaseOrderManager) SetApplicationApprovalFlowID(ctx context.Context, applicationID, approvalFlowID string) error {
	applicationID, approvalFlowID = strings.TrimSpace(applicationID), strings.TrimSpace(approvalFlowID)
	if applicationID == "" {
		return ErrInvalidID
	}
	if approvalFlowID != "" {
		flow, err := uc.repo.GetApprovalFlowDefinitionByID(ctx, approvalFlowID)
		if err != nil {
			return err
		}
		if flow.Status != domain.ApprovalFlowStatusActive {
			return fmt.Errorf("%w: approval flow is disabled", ErrInvalidInput)
		}
	}
	return uc.repo.UpsertApplicationApprovalFlowID(ctx, applicationID, approvalFlowID, uc.now())
}

func (uc *ReleaseOrderManager) GetApprovalFlowInstance(ctx context.Context, orderID string) (domain.ReleaseOrderApprovalFlowInstance, []domain.ReleaseOrderApprovalFlowTask, error) {
	orderID = strings.TrimSpace(orderID)
	if orderID == "" {
		return domain.ReleaseOrderApprovalFlowInstance{}, nil, ErrInvalidID
	}
	unlock := uc.lockOrderOperation(orderID)
	defer unlock()

	instance, err := uc.syncUnstartedApprovalFlowSnapshot(ctx, orderID)
	if err != nil {
		return domain.ReleaseOrderApprovalFlowInstance{}, nil, err
	}
	instance, err = uc.syncCurrentApprovalFlowAgentTask(ctx, instance)
	if err != nil {
		return domain.ReleaseOrderApprovalFlowInstance{}, nil, err
	}
	tasks, err := uc.repo.ListApprovalFlowTasks(ctx, orderID)
	if err != nil {
		return domain.ReleaseOrderApprovalFlowInstance{}, nil, err
	}
	for index := range tasks {
		tasks[index].Records, err = uc.repo.ListApprovalFlowTaskRecords(ctx, tasks[index].ID)
		if err != nil {
			return domain.ReleaseOrderApprovalFlowInstance{}, nil, err
		}
	}
	return instance, tasks, nil
}

// syncUnstartedApprovalFlowSnapshot keeps a pending release order aligned with
// the application's current approval-flow binding. Once an approval branch has
// started, the instance is intentionally frozen so an in-flight approval cannot
// be changed by later configuration edits.
func (uc *ReleaseOrderManager) syncUnstartedApprovalFlowSnapshot(ctx context.Context, orderID string) (domain.ReleaseOrderApprovalFlowInstance, error) {
	instance, instanceErr := uc.repo.GetApprovalFlowInstanceByOrderID(ctx, orderID)
	if instanceErr != nil && !errors.Is(instanceErr, domain.ErrApprovalFlowInstanceNotFound) {
		return domain.ReleaseOrderApprovalFlowInstance{}, instanceErr
	}

	order, err := uc.repo.GetByID(ctx, orderID)
	if errors.Is(err, domain.ErrOrderNotFound) {
		// A few internal callers build isolated flow instances in tests and tools.
		// Without an order/application there is no binding that can be refreshed.
		if instanceErr == nil {
			return instance, nil
		}
		return domain.ReleaseOrderApprovalFlowInstance{}, instanceErr
	}
	if err != nil {
		return domain.ReleaseOrderApprovalFlowInstance{}, err
	}
	if order.Status != domain.OrderStatusPending {
		if instanceErr != nil {
			return domain.ReleaseOrderApprovalFlowInstance{}, instanceErr
		}
		return instance, nil
	}

	if instanceErr == nil {
		tasks, listErr := uc.repo.ListApprovalFlowTasks(ctx, orderID)
		if listErr != nil {
			return domain.ReleaseOrderApprovalFlowInstance{}, listErr
		}
		if approvalFlowInstanceHasStarted(instance, tasks) {
			return instance, nil
		}
	}

	flowID, bindingExists, err := uc.getApplicationApprovalFlowBinding(ctx, order.ApplicationID)
	if err != nil {
		return domain.ReleaseOrderApprovalFlowInstance{}, err
	}
	// A missing binding row can belong to an old database or an isolated test.
	// Preserve an existing snapshot in that case. An explicit empty binding row,
	// on the other hand, means that the application was deliberately unbound.
	if !bindingExists {
		if instanceErr == nil {
			return instance, nil
		}
		return domain.ReleaseOrderApprovalFlowInstance{}, domain.ErrApprovalFlowInstanceNotFound
	}
	if flowID == "" {
		if instanceErr == nil {
			if deleteRepo, ok := uc.repo.(approvalFlowInstanceDeleteRepository); ok {
				if err := deleteRepo.DeleteApprovalFlowInstance(ctx, orderID); err != nil && !errors.Is(err, domain.ErrApprovalFlowInstanceNotFound) {
					return domain.ReleaseOrderApprovalFlowInstance{}, err
				}
			} else {
				instance.FlowDefinitionID, instance.FlowName = "", ""
				instance.Nodes, instance.Links = nil, nil
				instance.Status = domain.ApprovalFlowInstanceStatusRunning
				instance.CurrentGate, instance.CurrentScope = "", ""
				instance.CurrentNodeCode, instance.CurrentTaskID = "", ""
				instance.UpdatedAt = uc.now()
				if err := uc.repo.UpdateApprovalFlowInstance(ctx, instance); err != nil {
					return domain.ReleaseOrderApprovalFlowInstance{}, err
				}
			}
		}
		return domain.ReleaseOrderApprovalFlowInstance{}, domain.ErrApprovalFlowInstanceNotFound
	}

	definition, nodes, links, err := uc.loadApprovalFlowSnapshot(ctx, orderID, flowID)
	if err != nil {
		return domain.ReleaseOrderApprovalFlowInstance{}, err
	}
	now := uc.now()
	if instanceErr != nil {
		instance = domain.ReleaseOrderApprovalFlowInstance{
			ID:               generateID("rafi"),
			ReleaseOrderID:   orderID,
			FlowDefinitionID: definition.ID,
			FlowName:         definition.Name,
			Nodes:            nodes,
			Links:            links,
			Status:           domain.ApprovalFlowInstanceStatusRunning,
			CreatedAt:        now,
			UpdatedAt:        now,
		}
		if err := uc.repo.CreateApprovalFlowInstance(ctx, instance); err != nil {
			return domain.ReleaseOrderApprovalFlowInstance{}, err
		}
		return instance, nil
	}
	if instance.FlowDefinitionID == definition.ID &&
		instance.FlowName == definition.Name &&
		reflect.DeepEqual(instance.Nodes, nodes) &&
		reflect.DeepEqual(instance.Links, links) {
		return instance, nil
	}
	instance.FlowDefinitionID, instance.FlowName = definition.ID, definition.Name
	instance.Nodes, instance.Links = nodes, links
	instance.Status = domain.ApprovalFlowInstanceStatusRunning
	instance.CurrentGate, instance.CurrentScope = "", ""
	instance.CurrentNodeCode, instance.CurrentTaskID = "", ""
	instance.UpdatedAt = now
	if err := uc.repo.UpdateApprovalFlowInstance(ctx, instance); err != nil {
		return domain.ReleaseOrderApprovalFlowInstance{}, err
	}
	return instance, nil
}

func (uc *ReleaseOrderManager) getApplicationApprovalFlowBinding(ctx context.Context, applicationID string) (string, bool, error) {
	if repo, ok := uc.repo.(approvalFlowBindingLookupRepository); ok {
		return repo.GetApplicationApprovalFlowBinding(ctx, applicationID)
	}
	flowID, err := uc.repo.GetApplicationApprovalFlowID(ctx, applicationID)
	return flowID, strings.TrimSpace(flowID) != "", err
}

func approvalFlowInstanceHasStarted(instance domain.ReleaseOrderApprovalFlowInstance, tasks []domain.ReleaseOrderApprovalFlowTask) bool {
	return len(tasks) > 0 ||
		instance.Status != domain.ApprovalFlowInstanceStatusRunning ||
		instance.CurrentGate != "" ||
		instance.CurrentScope != "" ||
		strings.TrimSpace(instance.CurrentNodeCode) != "" ||
		strings.TrimSpace(instance.CurrentTaskID) != ""
}

type ListApprovalWorkbenchInput struct {
	UserID   string
	Page     int
	PageSize int
}

func (uc *ReleaseOrderManager) ListApprovalWorkbenchTasks(ctx context.Context, input ListApprovalWorkbenchInput) ([]domain.ReleaseApprovalWorkbenchTask, int64, error) {
	repo, ok := uc.repo.(domain.ApprovalWorkbenchRepository)
	if !ok {
		return nil, 0, fmt.Errorf("approval workbench repository is not configured")
	}
	filter, err := normalizeApprovalWorkbenchInput(input)
	if err != nil {
		return nil, 0, err
	}
	return repo.ListApprovalWorkbenchTasks(ctx, filter)
}

func (uc *ReleaseOrderManager) ListApprovalWorkbenchRecords(ctx context.Context, input ListApprovalWorkbenchInput) ([]domain.ReleaseApprovalWorkbenchRecord, int64, error) {
	repo, ok := uc.repo.(domain.ApprovalWorkbenchRepository)
	if !ok {
		return nil, 0, fmt.Errorf("approval workbench repository is not configured")
	}
	filter, err := normalizeApprovalWorkbenchInput(input)
	if err != nil {
		return nil, 0, err
	}
	return repo.ListApprovalWorkbenchRecords(ctx, filter)
}

func normalizeApprovalWorkbenchInput(input ListApprovalWorkbenchInput) (domain.ApprovalWorkbenchListFilter, error) {
	userID := strings.TrimSpace(input.UserID)
	if userID == "" {
		return domain.ApprovalWorkbenchListFilter{}, fmt.Errorf("%w: user_id is required", ErrInvalidInput)
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
	return domain.ApprovalWorkbenchListFilter{UserID: userID, Page: input.Page, PageSize: input.PageSize}, nil
}

func (uc *ReleaseOrderManager) initializeApprovalFlow(ctx context.Context, orderID string, flowID string) error {
	flowID = strings.TrimSpace(flowID)
	if flowID == "" {
		return nil
	}
	definition, nodes, links, err := uc.loadApprovalFlowSnapshot(ctx, orderID, flowID)
	if err != nil {
		return err
	}
	now := uc.now()
	instance := domain.ReleaseOrderApprovalFlowInstance{
		ID:               generateID("rafi"),
		ReleaseOrderID:   orderID,
		FlowDefinitionID: definition.ID,
		FlowName:         definition.Name,
		Nodes:            nodes,
		Links:            links,
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	// 创建发布单时只固化审批流快照。无论是历史线性流还是图形流，
	// 都必须等用户明确点击构建/发布后，才激活对应分支并创建审批任务。
	instance.Status = domain.ApprovalFlowInstanceStatusRunning
	return uc.repo.CreateApprovalFlowInstance(ctx, instance)
}

func (uc *ReleaseOrderManager) loadApprovalFlowSnapshot(ctx context.Context, orderID, flowID string) (domain.ApprovalFlowDefinition, []domain.ApprovalFlowNode, []domain.ApprovalFlowLink, error) {
	definition, err := uc.repo.GetApprovalFlowDefinitionByID(ctx, strings.TrimSpace(flowID))
	if err != nil {
		return domain.ApprovalFlowDefinition{}, nil, nil, err
	}
	if definition.Status != domain.ApprovalFlowStatusActive {
		return domain.ApprovalFlowDefinition{}, nil, nil, fmt.Errorf("%w: approval flow is disabled", ErrInvalidInput)
	}
	nodes, err := normalizeApprovalFlowNodes(definition.Nodes)
	if err != nil {
		return domain.ApprovalFlowDefinition{}, nil, nil, err
	}
	nodes, err = uc.resolveApprovalFlowNodeApprovers(ctx, orderID, nodes)
	if err != nil {
		return domain.ApprovalFlowDefinition{}, nil, nil, err
	}
	links, err := normalizeApprovalFlowLinks(nodes, definition.Links)
	if err != nil {
		return domain.ApprovalFlowDefinition{}, nil, nil, err
	}
	return definition, nodes, links, nil
}

func (uc *ReleaseOrderManager) ApproveApprovalFlowTask(ctx context.Context, orderID, taskID, operatorUserID, operatorName, comment string) (domain.ReleaseOrderApprovalFlowTask, error) {
	unlock := uc.lockOrderOperation(strings.TrimSpace(orderID))
	task, err := uc.actApprovalFlowTask(ctx, orderID, taskID, operatorUserID, operatorName, comment, domain.ReleaseOrderApprovalActionApprove)
	unlock()
	if err != nil || task.Status != domain.ApprovalFlowTaskStatusApproved {
		return task, err
	}
	uc.scheduleReleaseContinuationAfterApproval(orderID)
	return task, nil
}

func (uc *ReleaseOrderManager) RejectApprovalFlowTask(ctx context.Context, orderID, taskID, operatorUserID, operatorName, comment string) (domain.ReleaseOrderApprovalFlowTask, error) {
	if strings.TrimSpace(comment) == "" {
		return domain.ReleaseOrderApprovalFlowTask{}, fmt.Errorf("%w: reject comment is required", ErrInvalidInput)
	}
	unlock := uc.lockOrderOperation(strings.TrimSpace(orderID))
	defer unlock()
	return uc.actApprovalFlowTask(ctx, orderID, taskID, operatorUserID, operatorName, comment, domain.ReleaseOrderApprovalActionReject)
}

func (uc *ReleaseOrderManager) actApprovalFlowTask(ctx context.Context, orderID, taskID, operatorUserID, operatorName, comment string, action domain.ReleaseOrderApprovalAction) (domain.ReleaseOrderApprovalFlowTask, error) {
	orderID, taskID, operatorUserID = strings.TrimSpace(orderID), strings.TrimSpace(taskID), strings.TrimSpace(operatorUserID)
	if orderID == "" || taskID == "" || operatorUserID == "" {
		return domain.ReleaseOrderApprovalFlowTask{}, fmt.Errorf("%w: order_id, task_id and operator_user_id are required", ErrInvalidInput)
	}
	instance, err := uc.repo.GetApprovalFlowInstanceByOrderID(ctx, orderID)
	if err != nil {
		return domain.ReleaseOrderApprovalFlowTask{}, err
	}
	task, err := uc.repo.GetApprovalFlowTaskByID(ctx, taskID)
	if err != nil {
		return domain.ReleaseOrderApprovalFlowTask{}, err
	}
	if task.InstanceID != instance.ID || task.ReleaseOrderID != orderID || task.ID != instance.CurrentTaskID {
		return domain.ReleaseOrderApprovalFlowTask{}, fmt.Errorf("%w: approval task is not current", ErrInvalidInput)
	}
	if task.NodeType == domain.ApprovalFlowNodeTypeAgentTask {
		return domain.ReleaseOrderApprovalFlowTask{}, fmt.Errorf("%w: agent task nodes are completed automatically", ErrInvalidInput)
	}
	if task.Status != domain.ApprovalFlowTaskStatusPending {
		return domain.ReleaseOrderApprovalFlowTask{}, fmt.Errorf("%w: approval task has already finished", ErrInvalidInput)
	}
	if !approvalIncludesUser(task.ApproverIDs, operatorUserID) {
		return domain.ReleaseOrderApprovalFlowTask{}, fmt.Errorf("%w: current user is not in approval approver list", ErrInvalidInput)
	}
	records, err := uc.repo.ListApprovalFlowTaskRecords(ctx, task.ID)
	if err != nil {
		return domain.ReleaseOrderApprovalFlowTask{}, err
	}
	if approvalFlowTaskAlreadyActed(records, operatorUserID) {
		return domain.ReleaseOrderApprovalFlowTask{}, fmt.Errorf("%w: current approver has already acted", ErrInvalidInput)
	}
	now := uc.now()
	if err := uc.repo.CreateApprovalFlowTaskRecord(ctx, domain.ReleaseOrderApprovalFlowTaskRecord{
		ID: generateID("raftr"), TaskID: task.ID, Action: action, OperatorUserID: operatorUserID,
		OperatorName: firstNonEmpty(strings.TrimSpace(operatorName), operatorUserID), Comment: strings.TrimSpace(comment), CreatedAt: now,
	}); err != nil {
		return domain.ReleaseOrderApprovalFlowTask{}, err
	}
	if action == domain.ReleaseOrderApprovalActionReject {
		task.Status, task.UpdatedAt = domain.ApprovalFlowTaskStatusRejected, now
		instance.Status, instance.UpdatedAt = domain.ApprovalFlowInstanceStatusRejected, now
		if err := uc.repo.UpdateApprovalFlowTask(ctx, task); err != nil {
			return domain.ReleaseOrderApprovalFlowTask{}, err
		}
		if err := uc.repo.UpdateApprovalFlowInstance(ctx, instance); err != nil {
			return domain.ReleaseOrderApprovalFlowTask{}, err
		}
		if err := uc.syncReleaseOrderStatusAfterApprovalFlowAction(ctx, orderID, action, operatorUserID, operatorName, comment, now); err != nil {
			return domain.ReleaseOrderApprovalFlowTask{}, err
		}
		return task, nil
	}
	if task.ApprovalMode == domain.TemplateApprovalModeAll && !approvalFlowTaskAllApproved(records, task.ApproverIDs, operatorUserID) {
		if err := uc.syncReleaseOrderStatusAfterApprovalFlowAction(ctx, orderID, action, operatorUserID, operatorName, comment, now); err != nil {
			return domain.ReleaseOrderApprovalFlowTask{}, err
		}
		return task, nil
	}
	task.Status, task.UpdatedAt = domain.ApprovalFlowTaskStatusApproved, now
	if err := uc.repo.UpdateApprovalFlowTask(ctx, task); err != nil {
		return domain.ReleaseOrderApprovalFlowTask{}, err
	}
	if len(instance.Links) > 0 {
		advancedTask, advanceErr := uc.advanceGraphApprovalFlow(ctx, instance, task, now)
		if advanceErr != nil {
			return domain.ReleaseOrderApprovalFlowTask{}, advanceErr
		}
		if err := uc.syncReleaseOrderStatusAfterApprovalFlowAction(ctx, orderID, action, operatorUserID, operatorName, comment, now); err != nil {
			return domain.ReleaseOrderApprovalFlowTask{}, err
		}
		return advancedTask, nil
	}
	if err := uc.advanceLinearApprovalFlow(ctx, &instance, task, now); err != nil {
		return domain.ReleaseOrderApprovalFlowTask{}, err
	}
	if err := uc.syncReleaseOrderStatusAfterApprovalFlowAction(ctx, orderID, action, operatorUserID, operatorName, comment, now); err != nil {
		return domain.ReleaseOrderApprovalFlowTask{}, err
	}
	return task, nil
}

func (uc *ReleaseOrderManager) syncReleaseOrderStatusAfterApprovalFlowAction(
	ctx context.Context,
	orderID string,
	action domain.ReleaseOrderApprovalAction,
	operatorUserID string,
	operatorName string,
	comment string,
	now time.Time,
) error {
	order, err := uc.repo.GetByID(ctx, orderID)
	if errors.Is(err, domain.ErrOrderNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if order.Status != domain.OrderStatusPending && order.Status != domain.OrderStatusPendingApproval && order.Status != domain.OrderStatusApproving {
		return nil
	}
	operator := firstNonEmpty(strings.TrimSpace(operatorName), strings.TrimSpace(operatorUserID))
	if action == domain.ReleaseOrderApprovalActionReject {
		_, err = uc.repo.UpdateApprovalStatus(
			ctx,
			order.ID,
			domain.OrderStatusRejected,
			nil,
			"",
			&now,
			operator,
			strings.TrimSpace(comment),
			now,
		)
		return err
	}
	instance, err := uc.repo.GetApprovalFlowInstanceByOrderID(ctx, order.ID)
	if err != nil {
		return err
	}
	nextStatus := domain.OrderStatusApproving
	var approvedAt *time.Time
	approvedBy := ""
	if strings.TrimSpace(instance.CurrentTaskID) == "" &&
		instance.Status != domain.ApprovalFlowInstanceStatusPendingApproval &&
		instance.Status != domain.ApprovalFlowInstanceStatusRunningAgentTask {
		nextStatus = domain.OrderStatusApproved
		approvedAt = &now
		approvedBy = operator
	}
	_, err = uc.repo.UpdateApprovalStatus(
		ctx,
		order.ID,
		nextStatus,
		approvedAt,
		approvedBy,
		nil,
		"",
		"",
		now,
	)
	return err
}

// continueReleaseAfterApproval resumes the dispatch action that originally
// activated the approval branch. A newly-created next approval/Agent task keeps
// the order in approval; an execution boundary automatically starts CI/CD.
func (uc *ReleaseOrderManager) continueReleaseAfterApproval(ctx context.Context, orderID string) error {
	instance, err := uc.repo.GetApprovalFlowInstanceByOrderID(ctx, orderID)
	if errors.Is(err, domain.ErrApprovalFlowInstanceNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if strings.TrimSpace(instance.CurrentTaskID) != "" ||
		instance.Status == domain.ApprovalFlowInstanceStatusRejected ||
		instance.Status == domain.ApprovalFlowInstanceStatusRunningAgentTask ||
		instance.Status == domain.ApprovalFlowInstanceStatusAgentTaskFailed {
		return nil
	}
	order, err := uc.repo.GetByID(ctx, orderID)
	if errors.Is(err, domain.ErrOrderNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if order.Status != domain.OrderStatusApproved {
		return nil
	}
	// Isolated flow tests and maintenance tools may intentionally omit all
	// executors. Production dispatch always has at least one configured backend.
	if uc.jenkins == nil && uc.argocdFactory == nil {
		return nil
	}
	action, ok := approvalFlowContinuationAction(instance)
	if !ok {
		return nil
	}
	executorUserID := firstNonEmpty(strings.TrimSpace(order.ExecutorUserID), strings.TrimSpace(order.TriggeredBy), strings.TrimSpace(order.CreatorUserID))
	executorName := firstNonEmpty(strings.TrimSpace(order.ExecutorName), executorUserID)
	_, err = uc.dispatchOrder(ctx, orderID, action, executorUserID, executorName)
	return err
}

func (uc *ReleaseOrderManager) scheduleReleaseContinuationAfterApproval(orderID string) {
	task := func() {
		defer func() {
			if recovered := recover(); recovered != nil {
				logx.Error("release_order", "approval_auto_continue_panicked", fmt.Errorf("%v", recovered), logx.F("order_id", orderID))
			}
		}()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		if err := uc.continueReleaseAfterApproval(ctx, orderID); err != nil {
			logx.Error("release_order", "approval_auto_continue_failed", err, logx.F("order_id", orderID))
		}
	}
	if uc.runAsync != nil {
		uc.runAsync(task)
		return
	}
	go task()
}

func approvalFlowContinuationAction(instance domain.ReleaseOrderApprovalFlowInstance) (ReleaseOrderDispatchAction, bool) {
	switch instance.CurrentScope {
	case domain.ApprovalFlowExecutionScopeBuildOnly:
		return ReleaseOrderDispatchActionBuild, true
	case domain.ApprovalFlowExecutionScopeDeployOnly:
		return ReleaseOrderDispatchActionDeploy, true
	case domain.ApprovalFlowExecutionScopeFullRelease:
		if instance.Status == domain.ApprovalFlowInstanceStatusWaitingCD {
			return ReleaseOrderDispatchActionDeploy, true
		}
		return ReleaseOrderDispatchActionExecute, true
	}
	switch instance.CurrentGate {
	case domain.ApprovalFlowGateBeforeCI:
		return ReleaseOrderDispatchActionBuild, true
	case domain.ApprovalFlowGateBeforeCD:
		return ReleaseOrderDispatchActionDeploy, true
	case domain.ApprovalFlowGateBeforeExecute:
		return ReleaseOrderDispatchActionExecute, true
	default:
		return "", false
	}
}

func (uc *ReleaseOrderManager) ensureApprovalFlowDispatchAllowed(ctx context.Context, orderID string, action ReleaseOrderDispatchAction) error {
	return uc.ensureApprovalFlowDispatchAllowedForExecutions(ctx, orderID, action, nil)
}

func (uc *ReleaseOrderManager) ensureApprovalFlowDispatchAllowedForExecutions(ctx context.Context, orderID string, action ReleaseOrderDispatchAction, executions []domain.ReleaseOrderExecution) error {
	instance, err := uc.syncUnstartedApprovalFlowSnapshot(ctx, orderID)
	if errors.Is(err, domain.ErrApprovalFlowInstanceNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	instance, err = uc.syncCurrentApprovalFlowAgentTask(ctx, instance)
	if err != nil {
		return err
	}
	expectedGate := approvalFlowGateForDispatchAction(action)
	if instance.Status == domain.ApprovalFlowInstanceStatusRejected {
		return fmt.Errorf("%w: approval flow was rejected", ErrInvalidInput)
	}
	if instance.Status == domain.ApprovalFlowInstanceStatusAgentTaskFailed {
		return fmt.Errorf("%w: approval flow agent task failed", ErrInvalidInput)
	}
	if len(instance.Links) > 0 {
		return uc.ensureGraphApprovalFlowDispatchAllowed(ctx, instance, action, executions)
	}
	// 执行范围由用户在详情页选择的动作决定：Build/Deploy/Execute 分别
	// 激活 CI/CD/整单分支。一个已完成的分支不阻塞后续选择另一分支。
	if instance.CurrentTaskID == "" && instance.CurrentGate != expectedGate {
		envCode, envErr := uc.approvalFlowEnvironmentCode(ctx, orderID)
		if envErr != nil {
			return envErr
		}
		if node := firstApprovalFlowNodeForGate(instance.Nodes, expectedGate, envCode); node != nil {
			task, startErr := uc.startApprovalFlowNode(ctx, &instance, *node, uc.now())
			if startErr != nil {
				return startErr
			}
			return fmt.Errorf("%w: %s", ErrApprovalFlowPending, task.NodeName)
		}
		// 当前模式没有审批节点，允许直接进入对应执行分支。
		instance.CurrentGate, instance.UpdatedAt = expectedGate, uc.now()
		return uc.repo.UpdateApprovalFlowInstance(ctx, instance)
	}
	if instance.CurrentGate != expectedGate {
		return fmt.Errorf("%w: approval flow is waiting for %s", ErrInvalidInput, approvalFlowGateLabel(instance.CurrentGate))
	}
	if instance.CurrentTaskID == "" {
		return nil
	}
	task, err := uc.repo.GetApprovalFlowTaskByID(ctx, instance.CurrentTaskID)
	if err != nil {
		return err
	}
	if task.Status != domain.ApprovalFlowTaskStatusApproved {
		return fmt.Errorf("%w: %s", ErrApprovalFlowPending, task.NodeName)
	}
	return nil
}

func (uc *ReleaseOrderManager) markApprovalFlowDispatched(ctx context.Context, orderID string, action ReleaseOrderDispatchAction) error {
	instance, err := uc.repo.GetApprovalFlowInstanceByOrderID(ctx, orderID)
	if errors.Is(err, domain.ErrApprovalFlowInstanceNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if len(instance.Links) == 0 && instance.CurrentGate != approvalFlowGateForDispatchAction(action) {
		return nil
	}
	instance.CurrentTaskID, instance.UpdatedAt = "", uc.now()
	if len(instance.Links) > 0 {
		switch action {
		case ReleaseOrderDispatchActionBuild:
			instance.Status = domain.ApprovalFlowInstanceStatusWaitingCI
		case ReleaseOrderDispatchActionExecute:
			envCode, envErr := uc.approvalFlowEnvironmentCode(ctx, orderID)
			if envErr != nil {
				return envErr
			}
			if approvalFlowGraphNeedsCDGate(instance, envCode) {
				instance.Status = domain.ApprovalFlowInstanceStatusWaitingCI
			} else {
				instance.Status = domain.ApprovalFlowInstanceStatusRunning
			}
		default:
			instance.Status = domain.ApprovalFlowInstanceStatusRunning
		}
		return uc.repo.UpdateApprovalFlowInstance(ctx, instance)
	}
	switch action {
	case ReleaseOrderDispatchActionBuild:
		instance.Status = domain.ApprovalFlowInstanceStatusWaitingCI
	case ReleaseOrderDispatchActionDeploy:
		instance.Status = domain.ApprovalFlowInstanceStatusRunning
	default:
		instance.Status = domain.ApprovalFlowInstanceStatusRunning
	}
	return uc.repo.UpdateApprovalFlowInstance(ctx, instance)
}

func (uc *ReleaseOrderManager) activateApprovalFlowGate(ctx context.Context, orderID string, gate domain.ApprovalFlowGate) error {
	instance, err := uc.repo.GetApprovalFlowInstanceByOrderID(ctx, orderID)
	if errors.Is(err, domain.ErrApprovalFlowInstanceNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if len(instance.Links) > 0 {
		return uc.activateGraphApprovalFlowGate(ctx, instance, gate)
	}
	if instance.CurrentGate == gate {
		return nil
	}
	envCode, err := uc.approvalFlowEnvironmentCode(ctx, orderID)
	if err != nil {
		return err
	}
	instance.CurrentGate, instance.CurrentTaskID, instance.UpdatedAt = gate, "", uc.now()
	if node := firstApprovalFlowNodeForGate(instance.Nodes, gate, envCode); node != nil {
		_, err = uc.startApprovalFlowNode(ctx, &instance, *node, instance.UpdatedAt)
		return err
	}
	instance.Status = domain.ApprovalFlowInstanceStatusWaitingCD
	return uc.repo.UpdateApprovalFlowInstance(ctx, instance)
}

// completeApprovalFlowWithoutDeployment 将没有 CD 执行单元的发布单直接推进到流程结束。
// 调用方必须先确保构建完成 Hook（包括 Agent 任务）已经结束，避免绕过自动检查。
func (uc *ReleaseOrderManager) completeApprovalFlowWithoutDeployment(ctx context.Context, orderID string) error {
	instance, err := uc.repo.GetApprovalFlowInstanceByOrderID(ctx, orderID)
	if errors.Is(err, domain.ErrApprovalFlowInstanceNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if instance.Status == domain.ApprovalFlowInstanceStatusRejected || instance.CurrentTaskID != "" {
		return nil
	}
	instance.CurrentNodeCode = "end"
	instance.CurrentTaskID = ""
	instance.CurrentGate = ""
	instance.Status = domain.ApprovalFlowInstanceStatusCompleted
	instance.UpdatedAt = uc.now()
	return uc.repo.UpdateApprovalFlowInstance(ctx, instance)
}

func (uc *ReleaseOrderManager) ensureGraphApprovalFlowDispatchAllowed(ctx context.Context, instance domain.ReleaseOrderApprovalFlowInstance, action ReleaseOrderDispatchAction, executions []domain.ReleaseOrderExecution) error {
	envCode, err := uc.approvalFlowEnvironmentCode(ctx, instance.ReleaseOrderID)
	if err != nil {
		return err
	}
	scope := approvalFlowScopeForDispatch(instance, action)
	preserveWaitingDeploy := action == ReleaseOrderDispatchActionDeploy && instance.CurrentNodeCode == "waiting_deploy"
	if preserveWaitingDeploy {
		// 兼容旧图：旧定义只有 start -> CD，没有显式的“待部署 -> CD”连线。
		// 实例仍会进入待部署状态，但发起部署时从旧的部署入口继续。
		if _, found := nextApprovalFlowLink(instance.Links, "waiting_deploy", scope); !found {
			preserveWaitingDeploy = false
		}
	}
	if (instance.CurrentScope != scope && !preserveWaitingDeploy) || strings.TrimSpace(instance.CurrentNodeCode) == "" {
		instance.CurrentScope, instance.CurrentNodeCode, instance.CurrentTaskID, instance.CurrentGate = scope, "start", "", ""
	} else if preserveWaitingDeploy {
		instance.CurrentScope, instance.CurrentTaskID, instance.CurrentGate = scope, "", domain.ApprovalFlowGateBeforeCD
	}
	if instance.CurrentTaskID != "" {
		task, err := uc.repo.GetApprovalFlowTaskByID(ctx, instance.CurrentTaskID)
		if err != nil {
			return err
		}
		if task.Status != domain.ApprovalFlowTaskStatusApproved {
			return fmt.Errorf("%w: %s", ErrApprovalFlowPending, task.NodeName)
		}
	}
	target, found := nextGraphApprovalTarget(instance, instance.CurrentNodeCode, envCode)
	if !found || target.Code == "end" {
		instance.CurrentNodeCode, instance.CurrentTaskID, instance.CurrentGate, instance.Status, instance.UpdatedAt = "end", "", approvalFlowGateForDispatchAction(action), domain.ApprovalFlowInstanceStatusRunning, uc.now()
		return uc.repo.UpdateApprovalFlowInstance(ctx, instance)
	}
	if target.Code == "waiting_deploy" {
		// 待部署是“仅构建执行完成”后的发布单状态，不在构建前提前推进游标。
		instance.CurrentGate, instance.CurrentTaskID, instance.Status, instance.UpdatedAt = domain.ApprovalFlowGateBeforeCI, "", domain.ApprovalFlowInstanceStatusWaitingCI, uc.now()
		return uc.repo.UpdateApprovalFlowInstance(ctx, instance)
	}
	next := target.Node
	if next == nil {
		return fmt.Errorf("%w: approval flow target %s is invalid", ErrInvalidInput, target.Code)
	}
	if approvalFlowGateReadyForDispatch(action, next.Gate, executions) {
		task, startErr := uc.startApprovalFlowNode(ctx, &instance, *next, uc.now())
		if startErr != nil {
			return startErr
		}
		return fmt.Errorf("%w: %s", ErrApprovalFlowPending, task.NodeName)
	}
	if !approvalFlowGateCanWaitForDispatch(scope, action, next.Gate, executions) {
		return fmt.Errorf("%w: %s 分支不能在当前执行动作等待 %s", ErrInvalidInput, approvalFlowScopeLabel(scope), approvalFlowGateLabel(next.Gate))
	}
	instance.CurrentGate, instance.CurrentTaskID, instance.Status, instance.UpdatedAt = next.Gate, "", approvalFlowInstanceWaitingStatus(next.Gate), uc.now()
	return uc.repo.UpdateApprovalFlowInstance(ctx, instance)
}

func (uc *ReleaseOrderManager) advanceGraphApprovalFlow(ctx context.Context, instance domain.ReleaseOrderApprovalFlowInstance, task domain.ReleaseOrderApprovalFlowTask, now time.Time) (domain.ReleaseOrderApprovalFlowTask, error) {
	envCode, err := uc.approvalFlowEnvironmentCode(ctx, instance.ReleaseOrderID)
	if err != nil {
		return domain.ReleaseOrderApprovalFlowTask{}, err
	}
	instance.CurrentNodeCode, instance.CurrentTaskID, instance.UpdatedAt = task.NodeCode, "", now
	target, found := nextGraphApprovalTarget(instance, task.NodeCode, envCode)
	if !found || target.Code == "end" {
		instance.CurrentNodeCode, instance.Status = "end", approvalFlowInstanceStatusAfterApproval(task.Gate)
		if err := uc.repo.UpdateApprovalFlowInstance(ctx, instance); err != nil {
			return domain.ReleaseOrderApprovalFlowTask{}, err
		}
		return task, nil
	}
	if target.Code == "waiting_deploy" {
		instance.CurrentGate, instance.Status = domain.ApprovalFlowGateBeforeCI, domain.ApprovalFlowInstanceStatusWaitingCI
		if err := uc.repo.UpdateApprovalFlowInstance(ctx, instance); err != nil {
			return domain.ReleaseOrderApprovalFlowTask{}, err
		}
		return task, nil
	}
	next := target.Node
	if next == nil {
		return domain.ReleaseOrderApprovalFlowTask{}, fmt.Errorf("%w: approval flow target %s is invalid", ErrInvalidInput, target.Code)
	}
	if graphApprovalNodeStartsImmediately(instance.CurrentScope, task.Gate, next.Gate) {
		if _, err := uc.startApprovalFlowNode(ctx, &instance, *next, now); err != nil {
			return domain.ReleaseOrderApprovalFlowTask{}, err
		}
		return task, nil
	}
	instance.CurrentGate, instance.Status = next.Gate, approvalFlowInstanceWaitingStatus(next.Gate)
	if err := uc.repo.UpdateApprovalFlowInstance(ctx, instance); err != nil {
		return domain.ReleaseOrderApprovalFlowTask{}, err
	}
	return task, nil
}

func (uc *ReleaseOrderManager) activateGraphApprovalFlowGate(ctx context.Context, instance domain.ReleaseOrderApprovalFlowInstance, gate domain.ApprovalFlowGate) error {
	// 仅构建不是流程结束：CI 完成后，审批实例与发布单一起进入“待部署”。
	if instance.CurrentScope == domain.ApprovalFlowExecutionScopeBuildOnly && gate == domain.ApprovalFlowGateBeforeCD {
		instance.CurrentNodeCode, instance.CurrentTaskID, instance.CurrentGate, instance.Status, instance.UpdatedAt = "waiting_deploy", "", domain.ApprovalFlowGateBeforeCD, domain.ApprovalFlowInstanceStatusWaitingCD, uc.now()
		return uc.repo.UpdateApprovalFlowInstance(ctx, instance)
	}
	if instance.CurrentScope == "" || instance.CurrentNodeCode == "end" || instance.CurrentTaskID != "" {
		return nil
	}
	envCode, err := uc.approvalFlowEnvironmentCode(ctx, instance.ReleaseOrderID)
	if err != nil {
		return err
	}
	target, found := nextGraphApprovalTarget(instance, instance.CurrentNodeCode, envCode)
	if !found || target.Code == "end" {
		instance.CurrentNodeCode, instance.CurrentTaskID, instance.CurrentGate, instance.Status, instance.UpdatedAt = "end", "", gate, approvalFlowInstanceWaitingStatus(gate), uc.now()
		return uc.repo.UpdateApprovalFlowInstance(ctx, instance)
	}
	if target.Code == "waiting_deploy" {
		instance.CurrentNodeCode, instance.CurrentTaskID, instance.CurrentGate, instance.Status, instance.UpdatedAt = "waiting_deploy", "", domain.ApprovalFlowGateBeforeCD, domain.ApprovalFlowInstanceStatusWaitingCD, uc.now()
		return uc.repo.UpdateApprovalFlowInstance(ctx, instance)
	}
	next := target.Node
	if next == nil {
		return fmt.Errorf("%w: approval flow target %s is invalid", ErrInvalidInput, target.Code)
	}
	if next.Gate != gate {
		return fmt.Errorf("%w: approval flow is waiting for %s", ErrInvalidInput, approvalFlowGateLabel(next.Gate))
	}
	_, err = uc.startApprovalFlowNode(ctx, &instance, *next, uc.now())
	return err
}

type approvalFlowGraphTarget struct {
	Code string
	Node *domain.ApprovalFlowNode
}

func nextGraphApprovalTarget(instance domain.ReleaseOrderApprovalFlowInstance, fromCode string, envCode string) (approvalFlowGraphTarget, bool) {
	currentCode := fromCode
	visited := make(map[string]struct{}, len(instance.Nodes)+2)
	for steps := 0; steps <= len(instance.Nodes)+1; steps++ {
		if _, exists := visited[currentCode]; exists {
			return approvalFlowGraphTarget{}, false
		}
		visited[currentCode] = struct{}{}
		link, found := nextApprovalFlowLink(instance.Links, currentCode, instance.CurrentScope)
		if !found {
			return approvalFlowGraphTarget{}, false
		}
		target := approvalFlowGraphTarget{Code: link.ToCode}
		if link.ToCode == "end" || link.ToCode == "waiting_deploy" {
			return target, true
		}
		for index := range instance.Nodes {
			if instance.Nodes[index].Code != link.ToCode {
				continue
			}
			if approvalFlowNodeMatchesEnvironment(instance.Nodes[index], envCode) {
				target.Node = &instance.Nodes[index]
				return target, true
			}
			currentCode = instance.Nodes[index].Code
			target.Node = nil
			break
		}
		if currentCode != link.ToCode {
			return approvalFlowGraphTarget{}, false
		}
	}
	return approvalFlowGraphTarget{}, false
}

func nextApprovalFlowLink(links []domain.ApprovalFlowLink, fromCode string, scope domain.ApprovalFlowExecutionScope) (domain.ApprovalFlowLink, bool) {
	candidates := make([]domain.ApprovalFlowLink, 0)
	for _, link := range links {
		if link.FromCode != fromCode || !approvalFlowLinkMatchesScope(link, scope) {
			continue
		}
		candidates = append(candidates, link)
	}
	if len(candidates) == 0 {
		return domain.ApprovalFlowLink{}, false
	}
	sort.SliceStable(candidates, func(i, j int) bool { return candidates[i].Priority < candidates[j].Priority })
	return candidates[0], true
}

func approvalFlowLinkMatchesScope(link domain.ApprovalFlowLink, scope domain.ApprovalFlowExecutionScope) bool {
	if len(link.ExecutionScopes) == 0 {
		return true
	}
	for _, item := range link.ExecutionScopes {
		if domain.ApprovalFlowExecutionScope(item) == scope {
			return true
		}
	}
	return false
}

func approvalFlowScopeForDispatch(instance domain.ReleaseOrderApprovalFlowInstance, action ReleaseOrderDispatchAction) domain.ApprovalFlowExecutionScope {
	if action == ReleaseOrderDispatchActionDeploy && instance.CurrentScope == domain.ApprovalFlowExecutionScopeFullRelease {
		return domain.ApprovalFlowExecutionScopeFullRelease
	}
	switch action {
	case ReleaseOrderDispatchActionBuild:
		return domain.ApprovalFlowExecutionScopeBuildOnly
	case ReleaseOrderDispatchActionDeploy:
		return domain.ApprovalFlowExecutionScopeDeployOnly
	default:
		return domain.ApprovalFlowExecutionScopeFullRelease
	}
}

func approvalFlowGateReadyForDispatch(action ReleaseOrderDispatchAction, gate domain.ApprovalFlowGate, executions []domain.ReleaseOrderExecution) bool {
	switch action {
	case ReleaseOrderDispatchActionBuild:
		return gate == domain.ApprovalFlowGateBeforeCI
	case ReleaseOrderDispatchActionDeploy:
		return gate == domain.ApprovalFlowGateBeforeCD
	default:
		if gate == domain.ApprovalFlowGateBeforeExecute {
			return true
		}
		if len(executions) == 0 {
			return gate == domain.ApprovalFlowGateBeforeCI
		}
		if findExecutionByScopeAndStatus(executions, domain.PipelineScopeCI, domain.ExecutionStatusPending) != nil {
			return gate == domain.ApprovalFlowGateBeforeCI
		}
		return gate == domain.ApprovalFlowGateBeforeCD
	}
}

func approvalFlowGateCanWaitForDispatch(scope domain.ApprovalFlowExecutionScope, action ReleaseOrderDispatchAction, gate domain.ApprovalFlowGate, executions []domain.ReleaseOrderExecution) bool {
	if scope != domain.ApprovalFlowExecutionScopeFullRelease || action != ReleaseOrderDispatchActionExecute || gate != domain.ApprovalFlowGateBeforeCD {
		return false
	}
	return len(executions) == 0 || findExecutionByScopeAndStatus(executions, domain.PipelineScopeCI, domain.ExecutionStatusPending) != nil
}

func graphApprovalNodeStartsImmediately(scope domain.ApprovalFlowExecutionScope, current, next domain.ApprovalFlowGate) bool {
	if current == next {
		return true
	}
	switch scope {
	case domain.ApprovalFlowExecutionScopeBuildOnly:
		return next == domain.ApprovalFlowGateBeforeCI
	case domain.ApprovalFlowExecutionScopeDeployOnly:
		return next == domain.ApprovalFlowGateBeforeCD
	case domain.ApprovalFlowExecutionScopeFullRelease:
		return current == domain.ApprovalFlowGateBeforeExecute && (next == domain.ApprovalFlowGateBeforeExecute || next == domain.ApprovalFlowGateBeforeCI)
	default:
		return false
	}
}

func approvalFlowInstanceStatusAfterApproval(gate domain.ApprovalFlowGate) domain.ApprovalFlowInstanceStatus {
	switch gate {
	case domain.ApprovalFlowGateBeforeCI:
		return domain.ApprovalFlowInstanceStatusWaitingCI
	case domain.ApprovalFlowGateBeforeCD:
		return domain.ApprovalFlowInstanceStatusWaitingCD
	default:
		return domain.ApprovalFlowInstanceStatusRunning
	}
}

func approvalFlowInstanceWaitingStatus(gate domain.ApprovalFlowGate) domain.ApprovalFlowInstanceStatus {
	if gate == domain.ApprovalFlowGateBeforeCD {
		return domain.ApprovalFlowInstanceStatusWaitingCI
	}
	return domain.ApprovalFlowInstanceStatusRunning
}

func approvalFlowGraphNeedsCDGate(instance domain.ReleaseOrderApprovalFlowInstance, envCode string) bool {
	if instance.CurrentScope != domain.ApprovalFlowExecutionScopeFullRelease {
		return false
	}
	current := instance.CurrentNodeCode
	if current == "" {
		current = "start"
	}
	for steps := 0; steps <= len(instance.Nodes)+1; steps++ {
		target, found := nextGraphApprovalTarget(instance, current, envCode)
		if !found || target.Code == "end" {
			return false
		}
		if target.Code == "waiting_deploy" {
			current = target.Code
			continue
		}
		next := target.Node
		if next == nil {
			return false
		}
		if next.Gate == domain.ApprovalFlowGateBeforeCD {
			return true
		}
		current = next.Code
	}
	return false
}

func (uc *ReleaseOrderManager) shouldStageGraphFullRelease(ctx context.Context, orderID string, action ReleaseOrderDispatchAction, executions []domain.ReleaseOrderExecution) (bool, error) {
	if action != ReleaseOrderDispatchActionExecute || findExecutionByScopeAndStatus(executions, domain.PipelineScopeCI, domain.ExecutionStatusPending) == nil || !hasExecutionForScope(executions, domain.PipelineScopeCD) {
		return false, nil
	}
	instance, err := uc.repo.GetApprovalFlowInstanceByOrderID(ctx, orderID)
	if errors.Is(err, domain.ErrApprovalFlowInstanceNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	envCode, err := uc.approvalFlowEnvironmentCode(ctx, orderID)
	if err != nil {
		return false, err
	}
	return len(instance.Links) > 0 && approvalFlowGraphNeedsCDGate(instance, envCode), nil
}

func approvalFlowScopeLabel(scope domain.ApprovalFlowExecutionScope) string {
	switch scope {
	case domain.ApprovalFlowExecutionScopeBuildOnly:
		return "仅构建"
	case domain.ApprovalFlowExecutionScopeDeployOnly:
		return "仅部署"
	default:
		return "完整发布"
	}
}

func normalizeApprovalFlowNodes(items []domain.ApprovalFlowNode) ([]domain.ApprovalFlowNode, error) {
	if len(items) == 0 {
		return nil, fmt.Errorf("%w: approval flow needs at least one node", ErrInvalidInput)
	}
	result := make([]domain.ApprovalFlowNode, 0, len(items))
	seen := map[string]struct{}{}
	for index, item := range items {
		item.Code, item.Name = strings.TrimSpace(item.Code), strings.TrimSpace(item.Name)
		if item.NodeType == "" {
			item.NodeType = domain.ApprovalFlowNodeTypeApproval
		}
		if item.NodeType == domain.ApprovalFlowNodeTypeAgentTask {
			return nil, fmt.Errorf("%w: independent agent task approval nodes are no longer supported; use a build-complete agent hook before CD approval", ErrInvalidInput)
		}
		if item.ApproverSource == "" {
			item.ApproverSource = domain.ApprovalFlowApproverSourceUsers
		}
		if item.ApprovalMode == "" {
			item.ApprovalMode = domain.TemplateApprovalModeAny
		}
		if item.Code == "" || item.Name == "" || !item.Gate.Valid() || !item.NodeType.Valid() {
			return nil, fmt.Errorf("%w: approval flow node is incomplete", ErrInvalidInput)
		}
		if item.NodeType == domain.ApprovalFlowNodeTypeAgentTask {
			item.AgentTaskID = strings.TrimSpace(item.AgentTaskID)
			item.AgentTaskName = strings.TrimSpace(item.AgentTaskName)
			if item.AgentTaskID == "" {
				return nil, fmt.Errorf("%w: agent task node needs a task template", ErrInvalidInput)
			}
			item.ApprovalMode = domain.TemplateApprovalModeAny
			item.ApproverSource = domain.ApprovalFlowApproverSourceUsers
			item.ManagerLevel = 0
			item.ApproverIDs, item.ApproverNames = nil, nil
		} else {
			item.AgentTaskID, item.AgentTaskName = "", ""
			if !item.ApprovalMode.Valid() || !item.ApproverSource.Valid() {
				return nil, fmt.Errorf("%w: approval flow node is incomplete", ErrInvalidInput)
			}
			if item.ApproverSource == domain.ApprovalFlowApproverSourceManager {
				if item.ManagerLevel <= 0 || item.ManagerLevel > 20 {
					return nil, fmt.Errorf("%w: manager approval level must be between 1 and 20", ErrInvalidInput)
				}
				item.ApproverIDs, item.ApproverNames = nil, nil
			} else {
				item.ManagerLevel = 0
			}
		}
		if _, ok := seen[item.Code]; ok {
			return nil, fmt.Errorf("%w: duplicated approval flow node code", ErrInvalidInput)
		}
		seen[item.Code] = struct{}{}
		item.SortNo = index + 1
		item.ApplicableEnvCodes = normalizeApprovalFlowEnvironmentCodes(item.ApplicableEnvCodes)
		item.ApproverIDs = normalizeApprovalFlowApproverIDs(item.ApproverIDs)
		item.ApproverNames = append([]string(nil), item.ApproverNames...)
		if item.NodeType == domain.ApprovalFlowNodeTypeApproval && item.ApproverSource == domain.ApprovalFlowApproverSourceUsers && len(item.ApproverIDs) == 0 {
			return nil, fmt.Errorf("%w: approval flow node needs approvers", ErrInvalidInput)
		}
		result = append(result, item)
	}
	sort.SliceStable(result, func(i, j int) bool { return result[i].SortNo < result[j].SortNo })
	return result, nil
}

func (uc *ReleaseOrderManager) validateApprovalFlowAgentTaskNodes(ctx context.Context, nodes []domain.ApprovalFlowNode) ([]domain.ApprovalFlowNode, error) {
	validated := append([]domain.ApprovalFlowNode(nil), nodes...)
	for index := range validated {
		if validated[index].NodeType != domain.ApprovalFlowNodeTypeAgentTask {
			continue
		}
		if uc.agentRepo == nil {
			return nil, fmt.Errorf("%w: agent repository is not configured", ErrInvalidInput)
		}
		task, err := uc.agentRepo.GetTaskByID(ctx, validated[index].AgentTaskID)
		if err != nil {
			return nil, err
		}
		if !isReusableAgentTaskHookTarget(task) {
			return nil, fmt.Errorf("%w: agent task node target must be a manual temporary task", ErrInvalidInput)
		}
		validated[index].AgentTaskName = firstNonEmpty(strings.TrimSpace(task.Name), strings.TrimSpace(task.ScriptName), task.ID)
	}
	return validated, nil
}

func (uc *ReleaseOrderManager) resolveApprovalFlowNodeApprovers(ctx context.Context, orderID string, nodes []domain.ApprovalFlowNode) ([]domain.ApprovalFlowNode, error) {
	needsManager := false
	for _, node := range nodes {
		if node.NodeType == domain.ApprovalFlowNodeTypeApproval && node.ApproverSource == domain.ApprovalFlowApproverSourceManager {
			needsManager = true
			break
		}
	}
	if !needsManager {
		return nodes, nil
	}
	if uc.approvalManagerResolver == nil {
		return nil, fmt.Errorf("%w: organization manager resolver is not configured", ErrInvalidInput)
	}
	order, err := uc.repo.GetByID(ctx, orderID)
	if err != nil {
		return nil, err
	}
	creatorUserID := strings.TrimSpace(order.CreatorUserID)
	if creatorUserID == "" {
		return nil, fmt.Errorf("%w: release creator is required for manager approval", ErrInvalidInput)
	}
	resolved := append([]domain.ApprovalFlowNode(nil), nodes...)
	for index := range resolved {
		if resolved[index].NodeType != domain.ApprovalFlowNodeTypeApproval || resolved[index].ApproverSource != domain.ApprovalFlowApproverSourceManager {
			continue
		}
		manager, resolveErr := uc.approvalManagerResolver.ResolveUserManager(ctx, creatorUserID, resolved[index].ManagerLevel)
		if resolveErr != nil {
			return nil, fmt.Errorf("%w: cannot resolve level %d manager for release creator: %v", ErrInvalidInput, resolved[index].ManagerLevel, resolveErr)
		}
		resolved[index].ApproverIDs = []string{manager.ID}
		resolved[index].ApproverNames = []string{firstNonEmpty(strings.TrimSpace(manager.DisplayName), strings.TrimSpace(manager.Username), manager.ID)}
	}
	return resolved, nil
}

func approvalFlowTaskFromNode(instance domain.ReleaseOrderApprovalFlowInstance, node domain.ApprovalFlowNode, now time.Time) domain.ReleaseOrderApprovalFlowTask {
	nodeType := node.NodeType
	if nodeType == "" {
		nodeType = domain.ApprovalFlowNodeTypeApproval
	}
	return domain.ReleaseOrderApprovalFlowTask{
		ID: generateID("raft"), InstanceID: instance.ID, ReleaseOrderID: instance.ReleaseOrderID,
		NodeCode: node.Code, NodeName: node.Name, Gate: node.Gate, NodeType: nodeType,
		ApprovalMode: node.ApprovalMode, ApproverIDs: append([]string(nil), node.ApproverIDs...), ApproverNames: append([]string(nil), node.ApproverNames...),
		AgentTaskID: node.AgentTaskID, AgentTaskName: node.AgentTaskName,
		Status: domain.ApprovalFlowTaskStatusPending, CreatedAt: now, UpdatedAt: now,
	}
}

func (uc *ReleaseOrderManager) prepareApprovalFlowTaskForNode(ctx context.Context, instance domain.ReleaseOrderApprovalFlowInstance, node domain.ApprovalFlowNode, now time.Time) (domain.ReleaseOrderApprovalFlowTask, error) {
	task := approvalFlowTaskFromNode(instance, node, now)
	if task.NodeType != domain.ApprovalFlowNodeTypeAgentTask {
		return task, nil
	}
	batchID, message, err := uc.dispatchApprovalFlowAgentTask(ctx, instance, node)
	if err != nil {
		return domain.ReleaseOrderApprovalFlowTask{}, err
	}
	task.AgentBatchID, task.Message, task.Status = batchID, message, domain.ApprovalFlowTaskStatusRunning
	return task, nil
}

func (uc *ReleaseOrderManager) dispatchApprovalFlowAgentTask(ctx context.Context, instance domain.ReleaseOrderApprovalFlowInstance, node domain.ApprovalFlowNode) (string, string, error) {
	if uc.agentRepo == nil {
		return "", "", fmt.Errorf("%w: agent repository is not configured", ErrInvalidInput)
	}
	sourceTask, err := uc.agentRepo.GetTaskByID(ctx, strings.TrimSpace(node.AgentTaskID))
	if err != nil {
		return "", "", err
	}
	sourceTask, err = syncManagedScriptSnapshotForTask(ctx, uc.agentRepo, sourceTask, nil)
	if err != nil {
		return "", "", err
	}
	if !isReusableAgentTaskHookTarget(sourceTask) {
		return "", "", fmt.Errorf("%w: agent task node target must be a manual temporary task", ErrInvalidInput)
	}
	order, err := uc.repo.GetByID(ctx, instance.ReleaseOrderID)
	if err != nil {
		return "", "", err
	}
	executions, err := uc.repo.ListExecutions(ctx, order.ID)
	if err != nil {
		return "", "", err
	}
	variables, err := uc.buildHookTaskVariables(ctx, order, executions, domain.ReleaseTemplateHook{}, domain.TemplateHookExecuteStagePostRelease)
	if err != nil {
		return "", "", err
	}
	mergeAgentTaskVariables(variables, sourceTask.Variables)
	variables["approval_flow_name"] = strings.TrimSpace(instance.FlowName)
	variables["approval_node_code"] = strings.TrimSpace(node.Code)
	variables["approval_node_name"] = strings.TrimSpace(node.Name)
	variables["approval_gate"] = strings.TrimSpace(string(node.Gate))
	variables["release_stage"] = "approval_" + strings.TrimPrefix(strings.TrimSpace(string(node.Gate)), "before_")
	targets, err := resolveTaskDispatchTargets(ctx, uc.agentRepo, sourceTask)
	if err != nil {
		return "", "", err
	}
	batchID := generateID("agbatch")
	dispatched, err := dispatchTemporaryTaskBatch(ctx, uc.agentRepo, sourceTask, targets,
		fmt.Sprintf("%s · %s", strings.TrimSpace(node.Name), strings.TrimSpace(order.OrderNo)), variables, "approval_flow", batchID, uc.now)
	if err != nil {
		return "", "", err
	}
	message := buildTaskBatchSummary(fmt.Sprintf("Agent 任务：%s", firstNonEmpty(strings.TrimSpace(node.AgentTaskName), strings.TrimSpace(sourceTask.Name), sourceTask.ID)), dispatched)
	return batchID, message, nil
}

func (uc *ReleaseOrderManager) startApprovalFlowNode(ctx context.Context, instance *domain.ReleaseOrderApprovalFlowInstance, node domain.ApprovalFlowNode, now time.Time) (domain.ReleaseOrderApprovalFlowTask, error) {
	task, err := uc.prepareApprovalFlowTaskForNode(ctx, *instance, node, now)
	if err != nil {
		return domain.ReleaseOrderApprovalFlowTask{}, err
	}
	instance.CurrentTaskID, instance.CurrentGate, instance.UpdatedAt = task.ID, task.Gate, now
	if task.NodeType == domain.ApprovalFlowNodeTypeAgentTask {
		instance.Status = domain.ApprovalFlowInstanceStatusRunningAgentTask
	} else {
		instance.Status = domain.ApprovalFlowInstanceStatusPendingApproval
	}
	if err := uc.repo.UpdateApprovalFlowInstance(ctx, *instance); err != nil {
		return domain.ReleaseOrderApprovalFlowTask{}, err
	}
	if err := uc.repo.CreateApprovalFlowTask(ctx, task); err != nil {
		return domain.ReleaseOrderApprovalFlowTask{}, err
	}
	return task, nil
}

func (uc *ReleaseOrderManager) advanceLinearApprovalFlow(ctx context.Context, instance *domain.ReleaseOrderApprovalFlowInstance, task domain.ReleaseOrderApprovalFlowTask, now time.Time) error {
	envCode, err := uc.approvalFlowEnvironmentCode(ctx, instance.ReleaseOrderID)
	if err != nil {
		return err
	}
	if next := nextApprovalFlowNode(instance.Nodes, task.NodeCode, task.Gate, envCode); next != nil {
		_, err = uc.startApprovalFlowNode(ctx, instance, *next, now)
		return err
	}
	instance.CurrentTaskID, instance.UpdatedAt = "", now
	switch task.Gate {
	case domain.ApprovalFlowGateBeforeCI:
		instance.Status = domain.ApprovalFlowInstanceStatusWaitingCI
	case domain.ApprovalFlowGateBeforeCD:
		instance.Status = domain.ApprovalFlowInstanceStatusWaitingCD
	default:
		instance.Status = domain.ApprovalFlowInstanceStatusRunning
	}
	return uc.repo.UpdateApprovalFlowInstance(ctx, *instance)
}

func (uc *ReleaseOrderManager) syncCurrentApprovalFlowAgentTask(ctx context.Context, instance domain.ReleaseOrderApprovalFlowInstance) (domain.ReleaseOrderApprovalFlowInstance, error) {
	if strings.TrimSpace(instance.CurrentTaskID) == "" {
		return instance, nil
	}
	task, err := uc.repo.GetApprovalFlowTaskByID(ctx, instance.CurrentTaskID)
	if err != nil {
		return instance, err
	}
	if task.NodeType != domain.ApprovalFlowNodeTypeAgentTask || task.Status != domain.ApprovalFlowTaskStatusRunning {
		return instance, nil
	}
	if uc.agentRepo == nil {
		return instance, fmt.Errorf("%w: agent repository is not configured", ErrInvalidInput)
	}
	items, _, err := uc.agentRepo.ListTasks(ctx, agentdomain.TaskListFilter{SourceTaskID: task.AgentTaskID, DispatchBatchID: task.AgentBatchID, Page: 1, PageSize: 500})
	if err != nil {
		return instance, err
	}
	now := uc.now()
	if len(items) == 0 {
		task.Status, task.Message, task.UpdatedAt = domain.ApprovalFlowTaskStatusFailed, "Agent 任务批次不存在，无法继续执行", now
		instance.Status, instance.UpdatedAt = domain.ApprovalFlowInstanceStatusAgentTaskFailed, now
		if err := uc.repo.UpdateApprovalFlowTask(ctx, task); err != nil {
			return instance, err
		}
		if err := uc.repo.UpdateApprovalFlowInstance(ctx, instance); err != nil {
			return instance, err
		}
		return instance, nil
	}
	message := buildTaskBatchSummary(fmt.Sprintf("Agent 任务：%s", firstNonEmpty(task.AgentTaskName, task.AgentTaskID)), items)
	switch aggregateTaskBatchStatus(items) {
	case agentdomain.TaskStatusSuccess:
		task.Status, task.Message, task.UpdatedAt = domain.ApprovalFlowTaskStatusApproved, message, now
		if err := uc.repo.UpdateApprovalFlowTask(ctx, task); err != nil {
			return instance, err
		}
		if len(instance.Links) > 0 {
			if _, err := uc.advanceGraphApprovalFlow(ctx, instance, task, now); err != nil {
				return instance, err
			}
		} else if err := uc.advanceLinearApprovalFlow(ctx, &instance, task, now); err != nil {
			return instance, err
		}
		return uc.repo.GetApprovalFlowInstanceByOrderID(ctx, instance.ReleaseOrderID)
	case agentdomain.TaskStatusFailed, agentdomain.TaskStatusCancelled:
		task.Status, task.Message, task.UpdatedAt = domain.ApprovalFlowTaskStatusFailed, message, now
		instance.Status, instance.UpdatedAt = domain.ApprovalFlowInstanceStatusAgentTaskFailed, now
		if err := uc.repo.UpdateApprovalFlowTask(ctx, task); err != nil {
			return instance, err
		}
		if err := uc.repo.UpdateApprovalFlowInstance(ctx, instance); err != nil {
			return instance, err
		}
		return instance, nil
	default:
		if strings.TrimSpace(task.Message) != strings.TrimSpace(message) {
			task.Message, task.UpdatedAt = message, now
			if err := uc.repo.UpdateApprovalFlowTask(ctx, task); err != nil {
				return instance, err
			}
		}
		return instance, nil
	}
}

func nextApprovalFlowNode(nodes []domain.ApprovalFlowNode, currentCode string, gate domain.ApprovalFlowGate, envCode string) *domain.ApprovalFlowNode {
	for index, item := range nodes {
		if item.Code != currentCode {
			continue
		}
		for nextIndex := index + 1; nextIndex < len(nodes) && nodes[nextIndex].Gate == gate; nextIndex++ {
			if approvalFlowNodeMatchesEnvironment(nodes[nextIndex], envCode) {
				return &nodes[nextIndex]
			}
		}
		return nil
	}
	return nil
}

func firstApprovalFlowNodeForEnvironment(nodes []domain.ApprovalFlowNode, envCode string) *domain.ApprovalFlowNode {
	for index := range nodes {
		if approvalFlowNodeMatchesEnvironment(nodes[index], envCode) {
			return &nodes[index]
		}
	}
	return nil
}

func firstApprovalFlowNodeForGate(nodes []domain.ApprovalFlowNode, gate domain.ApprovalFlowGate, envCode string) *domain.ApprovalFlowNode {
	for index := range nodes {
		if nodes[index].Gate == gate && approvalFlowNodeMatchesEnvironment(nodes[index], envCode) {
			return &nodes[index]
		}
	}
	return nil
}

func approvalFlowNodeMatchesEnvironment(node domain.ApprovalFlowNode, envCode string) bool {
	if len(node.ApplicableEnvCodes) == 0 || strings.TrimSpace(envCode) == "" {
		return true
	}
	normalizedEnvCode := strings.ToLower(strings.TrimSpace(envCode))
	for _, item := range node.ApplicableEnvCodes {
		if strings.ToLower(strings.TrimSpace(item)) == normalizedEnvCode {
			return true
		}
	}
	return false
}

func (uc *ReleaseOrderManager) approvalFlowEnvironmentCode(ctx context.Context, orderID string) (string, error) {
	order, err := uc.repo.GetByID(ctx, strings.TrimSpace(orderID))
	if errors.Is(err, domain.ErrOrderNotFound) {
		// 兼容只构造审批流实例的历史调用与测试；真实发布单始终会提供环境。
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return strings.ToLower(strings.TrimSpace(order.EnvCode)), nil
}

func approvalFlowTaskAlreadyActed(records []domain.ReleaseOrderApprovalFlowTaskRecord, userID string) bool {
	for _, item := range records {
		if strings.TrimSpace(item.OperatorUserID) == strings.TrimSpace(userID) {
			return true
		}
	}
	return false
}

func approvalFlowTaskAllApproved(records []domain.ReleaseOrderApprovalFlowTaskRecord, approvers []string, currentUserID string) bool {
	approved := map[string]struct{}{strings.TrimSpace(currentUserID): {}}
	for _, item := range records {
		if item.Action == domain.ReleaseOrderApprovalActionApprove {
			approved[strings.TrimSpace(item.OperatorUserID)] = struct{}{}
		}
	}
	for _, item := range approvers {
		if _, ok := approved[strings.TrimSpace(item)]; !ok {
			return false
		}
	}
	return true
}

func approvalFlowGateForDispatchAction(action ReleaseOrderDispatchAction) domain.ApprovalFlowGate {
	switch action {
	case ReleaseOrderDispatchActionBuild:
		return domain.ApprovalFlowGateBeforeCI
	case ReleaseOrderDispatchActionDeploy:
		return domain.ApprovalFlowGateBeforeCD
	default:
		return domain.ApprovalFlowGateBeforeExecute
	}
}

func approvalFlowGateLabel(gate domain.ApprovalFlowGate) string {
	switch gate {
	case domain.ApprovalFlowGateBeforeCI:
		return "CI 审批"
	case domain.ApprovalFlowGateBeforeCD:
		return "CD 审批"
	default:
		return "整单审批"
	}
}

func approvalFlowTaskBlockingState(task domain.ReleaseOrderApprovalFlowTask) string {
	if task.NodeType == domain.ApprovalFlowNodeTypeAgentTask {
		if task.Status == domain.ApprovalFlowTaskStatusFailed {
			return "failed"
		}
		return "running"
	}
	return "pending"
}

func normalizeApprovalFlowApproverIDs(items []string) []string {
	result := make([]string, 0, len(items))
	seen := make(map[string]struct{}, len(items))
	for _, item := range items {
		value := strings.TrimSpace(item)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func normalizeApprovalFlowEnvironmentCodes(items []string) []string {
	result := make([]string, 0, len(items))
	seen := make(map[string]struct{}, len(items))
	for _, item := range items {
		value := strings.ToLower(strings.TrimSpace(item))
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}
