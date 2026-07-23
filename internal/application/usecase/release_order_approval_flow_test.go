package usecase

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	domain "gos/internal/domain/release"
	userdomain "gos/internal/domain/user"
)

func TestReleaseOrderApprovalFlowGatesBuildThenDeploy(t *testing.T) {
	t.Parallel()

	manager, _ := newReleaseOrderManagerForCancelTest(t)
	ctx := context.Background()
	flow, err := manager.CreateApprovalFlowDefinition(ctx, SaveApprovalFlowDefinitionInput{
		Name:   "生产 CI/CD 分段审批",
		Status: domain.ApprovalFlowStatusActive,
		Nodes: []domain.ApprovalFlowNode{
			{Code: "ci-owner", Name: "CI 审批", Gate: domain.ApprovalFlowGateBeforeCI, ApprovalMode: domain.TemplateApprovalModeAny, ApproverIDs: []string{"u-ci"}},
			{Code: "cd-sre", Name: "CD 审批", Gate: domain.ApprovalFlowGateBeforeCD, ApprovalMode: domain.TemplateApprovalModeAll, ApproverIDs: []string{"u-sre", "u-owner"}},
		},
	})
	if err != nil {
		t.Fatalf("CreateApprovalFlowDefinition failed: %v", err)
	}
	if err := manager.initializeApprovalFlow(ctx, "order-flow", flow.ID); err != nil {
		t.Fatalf("initializeApprovalFlow failed: %v", err)
	}
	if err := manager.ensureApprovalFlowDispatchAllowed(ctx, "order-flow", ReleaseOrderDispatchActionBuild); err == nil {
		t.Fatal("Build should be blocked before CI approval")
	}
	instance, tasks, err := manager.GetApprovalFlowInstance(ctx, "order-flow")
	if err != nil || instance.CurrentGate != domain.ApprovalFlowGateBeforeCI || len(tasks) != 1 {
		t.Fatalf("initial flow state = %#v tasks=%#v err=%v", instance, tasks, err)
	}
	if _, err := manager.ApproveApprovalFlowTask(ctx, "order-flow", tasks[0].ID, "u-ci", "CI Owner", "ok"); err != nil {
		t.Fatalf("ApproveApprovalFlowTask(ci) failed: %v", err)
	}
	if err := manager.ensureApprovalFlowDispatchAllowed(ctx, "order-flow", ReleaseOrderDispatchActionBuild); err != nil {
		t.Fatalf("Build should be allowed after CI approval: %v", err)
	}
	if err := manager.markApprovalFlowDispatched(ctx, "order-flow", ReleaseOrderDispatchActionBuild); err != nil {
		t.Fatalf("markApprovalFlowDispatched(build) failed: %v", err)
	}
	if err := manager.activateApprovalFlowGate(ctx, "order-flow", domain.ApprovalFlowGateBeforeCD); err != nil {
		t.Fatalf("activateApprovalFlowGate(cd) failed: %v", err)
	}
	instance, tasks, err = manager.GetApprovalFlowInstance(ctx, "order-flow")
	if err != nil || instance.CurrentGate != domain.ApprovalFlowGateBeforeCD || len(tasks) != 2 {
		t.Fatalf("CD flow state = %#v tasks=%#v err=%v", instance, tasks, err)
	}
	cdTask := tasks[1]
	if _, err := manager.ApproveApprovalFlowTask(ctx, "order-flow", cdTask.ID, "u-sre", "SRE", "ok"); err != nil {
		t.Fatalf("ApproveApprovalFlowTask(first CD approver) failed: %v", err)
	}
	if err := manager.ensureApprovalFlowDispatchAllowed(ctx, "order-flow", ReleaseOrderDispatchActionDeploy); err == nil {
		t.Fatal("Deploy should wait for all CD approvers")
	}
	if _, err := manager.ApproveApprovalFlowTask(ctx, "order-flow", cdTask.ID, "u-owner", "Owner", "ok"); err != nil {
		t.Fatalf("ApproveApprovalFlowTask(second CD approver) failed: %v", err)
	}
	if err := manager.ensureApprovalFlowDispatchAllowed(ctx, "order-flow", ReleaseOrderDispatchActionDeploy); err != nil {
		t.Fatalf("Deploy should be allowed after CD approval: %v", err)
	}
}

func TestApprovalFlowSkipsNodesOutsideReleaseEnvironment(t *testing.T) {
	t.Parallel()

	manager, repo := newReleaseOrderManagerForCancelTest(t)
	ctx := context.Background()
	now := time.Now().UTC()
	order := testReleaseOrder("order-env-flow", "RO-ENV-FLOW", domain.OrderStatusPending, now)
	order.EnvCode = "prod"
	if err := repo.Create(ctx, order, nil, nil, nil); err != nil {
		t.Fatalf("Create order failed: %v", err)
	}
	flow, err := manager.CreateApprovalFlowDefinition(ctx, SaveApprovalFlowDefinitionInput{
		Name:   "按环境审批",
		Status: domain.ApprovalFlowStatusActive,
		Nodes: []domain.ApprovalFlowNode{
			{Code: "test-approval", Name: "测试环境审批", Gate: domain.ApprovalFlowGateBeforeExecute, ApplicableEnvCodes: []string{"test"}, ApprovalMode: domain.TemplateApprovalModeAny, ApproverIDs: []string{"u-test"}},
			{Code: "prod-approval", Name: "生产环境审批", Gate: domain.ApprovalFlowGateBeforeExecute, ApplicableEnvCodes: []string{" PROD ", "prod"}, ApprovalMode: domain.TemplateApprovalModeAny, ApproverIDs: []string{"u-prod"}},
		},
		Links: []domain.ApprovalFlowLink{
			{FromCode: "start", ToCode: "test-approval", ExecutionScopes: []string{"full_release"}},
			{FromCode: "test-approval", ToCode: "prod-approval", ExecutionScopes: []string{"full_release"}},
			{FromCode: "prod-approval", ToCode: "end", ExecutionScopes: []string{"full_release"}},
		},
	})
	if err != nil {
		t.Fatalf("CreateApprovalFlowDefinition failed: %v", err)
	}
	if len(flow.Nodes[1].ApplicableEnvCodes) != 1 || flow.Nodes[1].ApplicableEnvCodes[0] != "prod" {
		t.Fatalf("normalized applicable environments = %#v, want [prod]", flow.Nodes[1].ApplicableEnvCodes)
	}
	if err := manager.initializeApprovalFlow(ctx, order.ID, flow.ID); err != nil {
		t.Fatalf("initializeApprovalFlow failed: %v", err)
	}
	if err := manager.ensureApprovalFlowDispatchAllowed(ctx, order.ID, ReleaseOrderDispatchActionExecute); err == nil {
		t.Fatal("full release should wait for the matching production approval node")
	}
	instance, tasks, err := manager.GetApprovalFlowInstance(ctx, order.ID)
	if err != nil {
		t.Fatalf("GetApprovalFlowInstance failed: %v", err)
	}
	if instance.CurrentTaskID == "" || len(tasks) != 1 || tasks[0].NodeCode != "prod-approval" {
		t.Fatalf("environment-filtered tasks = %#v instance=%#v, want only prod-approval", tasks, instance)
	}
}

func TestLinearApprovalFlowStartsAtFirstMatchingEnvironmentNode(t *testing.T) {
	t.Parallel()

	manager, repo := newReleaseOrderManagerForCancelTest(t)
	ctx := context.Background()
	now := time.Now().UTC()
	order := testReleaseOrder("order-linear-env", "RO-LINEAR-ENV", domain.OrderStatusPending, now)
	order.EnvCode = "prod"
	if err := repo.Create(ctx, order, nil, nil, nil); err != nil {
		t.Fatalf("Create order failed: %v", err)
	}
	flow, err := manager.CreateApprovalFlowDefinition(ctx, SaveApprovalFlowDefinitionInput{
		Name:   "历史线性环境审批",
		Status: domain.ApprovalFlowStatusActive,
		Nodes: []domain.ApprovalFlowNode{
			{Code: "test-linear", Name: "测试审批", Gate: domain.ApprovalFlowGateBeforeExecute, ApplicableEnvCodes: []string{"test"}, ApprovalMode: domain.TemplateApprovalModeAny, ApproverIDs: []string{"u-test"}},
			{Code: "prod-linear", Name: "生产审批", Gate: domain.ApprovalFlowGateBeforeExecute, ApplicableEnvCodes: []string{"prod"}, ApprovalMode: domain.TemplateApprovalModeAny, ApproverIDs: []string{"u-prod"}},
		},
	})
	if err != nil {
		t.Fatalf("CreateApprovalFlowDefinition failed: %v", err)
	}
	if err := manager.initializeApprovalFlow(ctx, order.ID, flow.ID); err != nil {
		t.Fatalf("initializeApprovalFlow failed: %v", err)
	}
	_, tasks, err := manager.GetApprovalFlowInstance(ctx, order.ID)
	if err != nil {
		t.Fatalf("GetApprovalFlowInstance failed: %v", err)
	}
	if len(tasks) != 0 {
		t.Fatalf("linear flow should not create tasks with the release order: %#v", tasks)
	}
	if dispatchErr := manager.ensureApprovalFlowDispatchAllowed(ctx, order.ID, ReleaseOrderDispatchActionExecute); !errors.Is(dispatchErr, ErrApprovalFlowPending) {
		t.Fatalf("first dispatch should start approval, got %v", dispatchErr)
	}
	_, tasks, err = manager.GetApprovalFlowInstance(ctx, order.ID)
	if err != nil {
		t.Fatalf("GetApprovalFlowInstance after dispatch failed: %v", err)
	}
	if len(tasks) != 1 || tasks[0].NodeCode != "prod-linear" {
		t.Fatalf("linear environment-filtered tasks after dispatch = %#v, want prod-linear", tasks)
	}
}

func TestDispatchStartsCustomApprovalWithoutReturningAnExecutionError(t *testing.T) {
	t.Parallel()

	manager, repo := newReleaseOrderManagerForCancelTest(t)
	manager.jenkins = &releasePrecheckJenkinsFake{}
	ctx := context.Background()
	now := time.Now().UTC()
	order := testReleaseOrder("order-start-approval", "RO-START-APPROVAL", domain.OrderStatusPending, now)
	execution := testReleaseExecution(order.ID, "exec-start-approval", domain.PipelineScopeCI, domain.ExecutionStatusPending, now)
	if err := repo.Create(ctx, order, []domain.ReleaseOrderExecution{execution}, nil, nil); err != nil {
		t.Fatalf("Create order failed: %v", err)
	}
	flow, err := manager.CreateApprovalFlowDefinition(ctx, SaveApprovalFlowDefinitionInput{
		Name:   "点击发布后审批",
		Status: domain.ApprovalFlowStatusActive,
		Nodes: []domain.ApprovalFlowNode{{
			Code: "whole-order", Name: "整单审批", Gate: domain.ApprovalFlowGateBeforeExecute,
			ApprovalMode: domain.TemplateApprovalModeAny, ApproverIDs: []string{"u-approver"},
		}},
		Links: []domain.ApprovalFlowLink{
			{FromCode: "start", ToCode: "whole-order", ExecutionScopes: []string{"full_release"}},
			{FromCode: "whole-order", ToCode: "end", ExecutionScopes: []string{"full_release"}},
		},
	})
	if err != nil {
		t.Fatalf("CreateApprovalFlowDefinition failed: %v", err)
	}
	if err := manager.initializeApprovalFlow(ctx, order.ID, flow.ID); err != nil {
		t.Fatalf("initializeApprovalFlow failed: %v", err)
	}
	_, tasks, err := manager.GetApprovalFlowInstance(ctx, order.ID)
	if err != nil || len(tasks) != 0 {
		t.Fatalf("new release should only contain a frozen flow snapshot, tasks=%#v err=%v", tasks, err)
	}

	started, err := manager.Execute(ctx, order.ID, "u-release", "Release User")
	if err != nil {
		t.Fatalf("Execute should start approval without returning an error: %v", err)
	}
	if started.Status != domain.OrderStatusPendingApproval || started.StartedAt != nil {
		t.Fatalf("approval-started order = %#v, want pending approval without execution start", started)
	}
	_, tasks, err = manager.GetApprovalFlowInstance(ctx, order.ID)
	if err != nil || len(tasks) != 1 || tasks[0].NodeCode != "whole-order" || tasks[0].Status != domain.ApprovalFlowTaskStatusPending {
		t.Fatalf("started approval tasks=%#v err=%v", tasks, err)
	}
	storedExecutions, err := repo.ListExecutions(ctx, order.ID)
	if err != nil || len(storedExecutions) != 1 || storedExecutions[0].Status != domain.ExecutionStatusPending {
		t.Fatalf("approval start must not dispatch executors: %#v err=%v", storedExecutions, err)
	}

	var scheduledContinuation func()
	manager.runAsync = func(task func()) { scheduledContinuation = task }
	if _, err := manager.ApproveApprovalFlowTask(ctx, order.ID, tasks[0].ID, "u-approver", "Approver", "approval note"); err != nil {
		t.Fatalf("ApproveApprovalFlowTask failed: %v", err)
	}
	approved, err := repo.GetByID(ctx, order.ID)
	if err != nil || approved.Status != domain.OrderStatusApproved || approved.StartedAt != nil || scheduledContinuation == nil {
		t.Fatalf("approval response should finish before automatic continuation: order=%#v scheduled=%t err=%v", approved, scheduledContinuation != nil, err)
	}
	records, err := repo.ListApprovalFlowTaskRecords(ctx, tasks[0].ID)
	if err != nil || len(records) != 1 || records[0].Comment != "approval note" {
		t.Fatalf("approval note records=%#v err=%v", records, err)
	}
	_, hydratedTasks, err := manager.GetApprovalFlowInstance(ctx, order.ID)
	if err != nil || len(hydratedTasks) != 1 || len(hydratedTasks[0].Records) != 1 || hydratedTasks[0].Records[0].Comment != "approval note" {
		t.Fatalf("approval flow response should hydrate task records: tasks=%#v err=%v", hydratedTasks, err)
	}
	scheduledContinuation()
	continued, err := repo.GetByID(ctx, order.ID)
	if err != nil || continued.Status == domain.OrderStatusApproved || continued.StartedAt == nil {
		t.Fatalf("approval should automatically continue release execution: order=%#v err=%v", continued, err)
	}
	if continued.ExecutorUserID != "u-release" || continued.ExecutorName != "Release User" {
		t.Fatalf("automatic continuation executor = %q/%q, want original release operator", continued.ExecutorUserID, continued.ExecutorName)
	}
}

func TestApprovalAutomaticallyAdvancesNextNodeBeforeDispatch(t *testing.T) {
	t.Parallel()

	manager, repo := newReleaseOrderManagerForCancelTest(t)
	manager.jenkins = &releasePrecheckJenkinsFake{}
	ctx := context.Background()
	now := time.Now().UTC()
	order := testReleaseOrder("order-auto-next-node", "RO-AUTO-NEXT-NODE", domain.OrderStatusPending, now)
	execution := testReleaseExecution(order.ID, "exec-auto-next-node", domain.PipelineScopeCI, domain.ExecutionStatusPending, now)
	if err := repo.Create(ctx, order, []domain.ReleaseOrderExecution{execution}, nil, nil); err != nil {
		t.Fatalf("Create order failed: %v", err)
	}
	flow, err := manager.CreateApprovalFlowDefinition(ctx, SaveApprovalFlowDefinitionInput{
		Name:   "连续审批后自动执行",
		Status: domain.ApprovalFlowStatusActive,
		Nodes: []domain.ApprovalFlowNode{
			{Code: "first-approval", Name: "第一审批", Gate: domain.ApprovalFlowGateBeforeExecute, ApprovalMode: domain.TemplateApprovalModeAny, ApproverIDs: []string{"u-first"}},
			{Code: "second-approval", Name: "第二审批", Gate: domain.ApprovalFlowGateBeforeExecute, ApprovalMode: domain.TemplateApprovalModeAny, ApproverIDs: []string{"u-second"}},
		},
		Links: []domain.ApprovalFlowLink{
			{FromCode: "start", ToCode: "first-approval", ExecutionScopes: []string{"full_release"}},
			{FromCode: "first-approval", ToCode: "second-approval", ExecutionScopes: []string{"full_release"}},
			{FromCode: "second-approval", ToCode: "end", ExecutionScopes: []string{"full_release"}},
		},
	})
	if err != nil {
		t.Fatalf("CreateApprovalFlowDefinition failed: %v", err)
	}
	if err := manager.initializeApprovalFlow(ctx, order.ID, flow.ID); err != nil {
		t.Fatalf("initializeApprovalFlow failed: %v", err)
	}
	started, err := manager.Execute(ctx, order.ID, "u-release", "Release User")
	if err != nil || started.Status != domain.OrderStatusPendingApproval {
		t.Fatalf("Execute should start first approval: order=%#v err=%v", started, err)
	}
	_, tasks, err := manager.GetApprovalFlowInstance(ctx, order.ID)
	if err != nil || len(tasks) != 1 || tasks[0].NodeCode != "first-approval" {
		t.Fatalf("first approval tasks=%#v err=%v", tasks, err)
	}
	if _, err := manager.ApproveApprovalFlowTask(ctx, order.ID, tasks[0].ID, "u-first", "First Approver", "ok"); err != nil {
		t.Fatalf("approve first node failed: %v", err)
	}
	_, tasks, err = manager.GetApprovalFlowInstance(ctx, order.ID)
	if err != nil || len(tasks) != 2 || tasks[1].NodeCode != "second-approval" || tasks[1].Status != domain.ApprovalFlowTaskStatusPending {
		t.Fatalf("flow should automatically advance to second approval: tasks=%#v err=%v", tasks, err)
	}
	beforeFinalApproval, err := repo.GetByID(ctx, order.ID)
	if err != nil || beforeFinalApproval.StartedAt != nil || beforeFinalApproval.Status != domain.OrderStatusApproving {
		t.Fatalf("execution must wait for second approval: order=%#v err=%v", beforeFinalApproval, err)
	}
	if _, err := manager.ApproveApprovalFlowTask(ctx, order.ID, tasks[1].ID, "u-second", "Second Approver", "ok"); err != nil {
		t.Fatalf("approve second node failed: %v", err)
	}
	continued, err := repo.GetByID(ctx, order.ID)
	if err != nil || continued.Status == domain.OrderStatusApproved || continued.StartedAt == nil {
		t.Fatalf("final approval should automatically dispatch release: order=%#v err=%v", continued, err)
	}
}

func TestApprovalFlowContinuationActionUsesCurrentExecutionBoundary(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		instance domain.ReleaseOrderApprovalFlowInstance
		want     ReleaseOrderDispatchAction
	}{
		{name: "linear ci", instance: domain.ReleaseOrderApprovalFlowInstance{CurrentGate: domain.ApprovalFlowGateBeforeCI}, want: ReleaseOrderDispatchActionBuild},
		{name: "linear cd", instance: domain.ReleaseOrderApprovalFlowInstance{CurrentGate: domain.ApprovalFlowGateBeforeCD}, want: ReleaseOrderDispatchActionDeploy},
		{name: "whole release", instance: domain.ReleaseOrderApprovalFlowInstance{CurrentGate: domain.ApprovalFlowGateBeforeExecute}, want: ReleaseOrderDispatchActionExecute},
		{name: "full release before ci", instance: domain.ReleaseOrderApprovalFlowInstance{CurrentScope: domain.ApprovalFlowExecutionScopeFullRelease, Status: domain.ApprovalFlowInstanceStatusWaitingCI}, want: ReleaseOrderDispatchActionExecute},
		{name: "full release before cd", instance: domain.ReleaseOrderApprovalFlowInstance{CurrentScope: domain.ApprovalFlowExecutionScopeFullRelease, Status: domain.ApprovalFlowInstanceStatusWaitingCD}, want: ReleaseOrderDispatchActionDeploy},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := approvalFlowContinuationAction(tt.instance)
			if !ok || got != tt.want {
				t.Fatalf("continuation action = %q ok=%t, want %q", got, ok, tt.want)
			}
		})
	}
}

func TestUnstartedApprovalFlowFollowsLatestApplicationBinding(t *testing.T) {
	t.Parallel()

	manager, repo := newReleaseOrderManagerForCancelTest(t)
	ctx := context.Background()
	now := time.Now().UTC()
	order := testReleaseOrder("order-follow-binding", "RO-FOLLOW-BINDING", domain.OrderStatusPending, now)
	if err := repo.Create(ctx, order, nil, nil, nil); err != nil {
		t.Fatalf("Create order failed: %v", err)
	}
	flowA, err := manager.CreateApprovalFlowDefinition(ctx, SaveApprovalFlowDefinitionInput{
		Name:   "原审批流",
		Status: domain.ApprovalFlowStatusActive,
		Nodes: []domain.ApprovalFlowNode{{
			Code: "approval-a", Name: "原审批节点", Gate: domain.ApprovalFlowGateBeforeExecute,
			ApprovalMode: domain.TemplateApprovalModeAny, ApproverIDs: []string{"u-a"},
		}},
	})
	if err != nil {
		t.Fatalf("Create flow A failed: %v", err)
	}
	if err := manager.SetApplicationApprovalFlowID(ctx, order.ApplicationID, flowA.ID); err != nil {
		t.Fatalf("bind flow A failed: %v", err)
	}
	if err := manager.initializeApprovalFlow(ctx, order.ID, flowA.ID); err != nil {
		t.Fatalf("initialize flow A failed: %v", err)
	}

	flowB, err := manager.CreateApprovalFlowDefinition(ctx, SaveApprovalFlowDefinitionInput{
		Name:   "最新审批流",
		Status: domain.ApprovalFlowStatusActive,
		Nodes: []domain.ApprovalFlowNode{{
			Code: "approval-b", Name: "最新审批节点", Gate: domain.ApprovalFlowGateBeforeExecute,
			ApprovalMode: domain.TemplateApprovalModeAny, ApproverIDs: []string{"u-b"},
		}},
	})
	if err != nil {
		t.Fatalf("Create flow B failed: %v", err)
	}
	if err := manager.SetApplicationApprovalFlowID(ctx, order.ApplicationID, flowB.ID); err != nil {
		t.Fatalf("rebind flow B failed: %v", err)
	}

	if dispatchErr := manager.ensureApprovalFlowDispatchAllowed(ctx, order.ID, ReleaseOrderDispatchActionExecute); !errors.Is(dispatchErr, ErrApprovalFlowPending) {
		t.Fatalf("dispatch should start latest flow B, got %v", dispatchErr)
	}
	instance, tasks, err := manager.GetApprovalFlowInstance(ctx, order.ID)
	if err != nil || instance.FlowDefinitionID != flowB.ID || instance.FlowName != flowB.Name || len(instance.Nodes) != 1 || instance.Nodes[0].Code != "approval-b" || len(tasks) != 1 || tasks[0].NodeCode != "approval-b" {
		t.Fatalf("dispatch used stale flow: instance=%#v tasks=%#v err=%v", instance, tasks, err)
	}
}

func TestUnstartedApprovalFlowRefreshesEditedDefinition(t *testing.T) {
	t.Parallel()

	manager, repo := newReleaseOrderManagerForCancelTest(t)
	ctx := context.Background()
	now := time.Now().UTC()
	order := testReleaseOrder("order-follow-definition", "RO-FOLLOW-DEFINITION", domain.OrderStatusPending, now)
	if err := repo.Create(ctx, order, nil, nil, nil); err != nil {
		t.Fatalf("Create order failed: %v", err)
	}
	flow, err := manager.CreateApprovalFlowDefinition(ctx, SaveApprovalFlowDefinitionInput{
		Name:   "变更前流程",
		Status: domain.ApprovalFlowStatusActive,
		Nodes: []domain.ApprovalFlowNode{{
			Code: "approval-old", Name: "变更前节点", Gate: domain.ApprovalFlowGateBeforeExecute,
			ApprovalMode: domain.TemplateApprovalModeAny, ApproverIDs: []string{"u-old"},
		}},
	})
	if err != nil {
		t.Fatalf("Create flow failed: %v", err)
	}
	if err := manager.SetApplicationApprovalFlowID(ctx, order.ApplicationID, flow.ID); err != nil {
		t.Fatalf("bind flow failed: %v", err)
	}
	if err := manager.initializeApprovalFlow(ctx, order.ID, flow.ID); err != nil {
		t.Fatalf("initialize flow failed: %v", err)
	}
	updated, err := manager.UpdateApprovalFlowDefinition(ctx, SaveApprovalFlowDefinitionInput{
		ID:     flow.ID,
		Name:   "变更后流程",
		Status: domain.ApprovalFlowStatusActive,
		Nodes: []domain.ApprovalFlowNode{{
			Code: "approval-new", Name: "变更后节点", Gate: domain.ApprovalFlowGateBeforeExecute,
			ApprovalMode: domain.TemplateApprovalModeAll, ApproverIDs: []string{"u-new-1", "u-new-2"},
		}},
	})
	if err != nil {
		t.Fatalf("UpdateApprovalFlowDefinition failed: %v", err)
	}

	instance, tasks, err := manager.GetApprovalFlowInstance(ctx, order.ID)
	if err != nil {
		t.Fatalf("GetApprovalFlowInstance failed: %v", err)
	}
	if instance.FlowDefinitionID != flow.ID || instance.FlowName != updated.Name || len(instance.Nodes) != 1 || instance.Nodes[0].Code != "approval-new" || instance.Nodes[0].ApprovalMode != domain.TemplateApprovalModeAll {
		t.Fatalf("refreshed instance = %#v, want edited definition", instance)
	}
	if len(tasks) != 0 {
		t.Fatalf("unstarted edited flow tasks = %#v, want none", tasks)
	}
}

func TestStartedApprovalFlowKeepsSnapshotAfterApplicationRebind(t *testing.T) {
	t.Parallel()

	manager, repo := newReleaseOrderManagerForCancelTest(t)
	ctx := context.Background()
	now := time.Now().UTC()
	order := testReleaseOrder("order-freeze-started", "RO-FREEZE-STARTED", domain.OrderStatusPending, now)
	if err := repo.Create(ctx, order, nil, nil, nil); err != nil {
		t.Fatalf("Create order failed: %v", err)
	}
	flowA, err := manager.CreateApprovalFlowDefinition(ctx, SaveApprovalFlowDefinitionInput{
		Name:   "已启动流程",
		Status: domain.ApprovalFlowStatusActive,
		Nodes: []domain.ApprovalFlowNode{{
			Code: "started-node", Name: "已启动节点", Gate: domain.ApprovalFlowGateBeforeExecute,
			ApprovalMode: domain.TemplateApprovalModeAny, ApproverIDs: []string{"u-started"},
		}},
	})
	if err != nil {
		t.Fatalf("Create flow A failed: %v", err)
	}
	if err := manager.SetApplicationApprovalFlowID(ctx, order.ApplicationID, flowA.ID); err != nil {
		t.Fatalf("bind flow A failed: %v", err)
	}
	if err := manager.initializeApprovalFlow(ctx, order.ID, flowA.ID); err != nil {
		t.Fatalf("initialize flow A failed: %v", err)
	}
	if dispatchErr := manager.ensureApprovalFlowDispatchAllowed(ctx, order.ID, ReleaseOrderDispatchActionExecute); !errors.Is(dispatchErr, ErrApprovalFlowPending) {
		t.Fatalf("dispatch should start flow A, got %v", dispatchErr)
	}

	flowB, err := manager.CreateApprovalFlowDefinition(ctx, SaveApprovalFlowDefinitionInput{
		Name:   "换绑后的流程",
		Status: domain.ApprovalFlowStatusActive,
		Nodes: []domain.ApprovalFlowNode{{
			Code: "replacement-node", Name: "换绑节点", Gate: domain.ApprovalFlowGateBeforeExecute,
			ApprovalMode: domain.TemplateApprovalModeAny, ApproverIDs: []string{"u-replacement"},
		}},
	})
	if err != nil {
		t.Fatalf("Create flow B failed: %v", err)
	}
	if err := manager.SetApplicationApprovalFlowID(ctx, order.ApplicationID, flowB.ID); err != nil {
		t.Fatalf("rebind flow B failed: %v", err)
	}

	instance, tasks, err := manager.GetApprovalFlowInstance(ctx, order.ID)
	if err != nil {
		t.Fatalf("GetApprovalFlowInstance failed: %v", err)
	}
	if instance.FlowDefinitionID != flowA.ID || len(instance.Nodes) != 1 || instance.Nodes[0].Code != "started-node" {
		t.Fatalf("started instance changed after rebind: %#v", instance)
	}
	if len(tasks) != 1 || tasks[0].NodeCode != "started-node" || tasks[0].Status != domain.ApprovalFlowTaskStatusPending {
		t.Fatalf("started tasks changed after rebind: %#v", tasks)
	}
}

func TestUnstartedApprovalFlowIsRemovedAfterApplicationUnbind(t *testing.T) {
	t.Parallel()

	manager, repo := newReleaseOrderManagerForCancelTest(t)
	ctx := context.Background()
	now := time.Now().UTC()
	order := testReleaseOrder("order-unbind-flow", "RO-UNBIND-FLOW", domain.OrderStatusPending, now)
	if err := repo.Create(ctx, order, nil, nil, nil); err != nil {
		t.Fatalf("Create order failed: %v", err)
	}
	flow, err := manager.CreateApprovalFlowDefinition(ctx, SaveApprovalFlowDefinitionInput{
		Name:   "待解绑流程",
		Status: domain.ApprovalFlowStatusActive,
		Nodes: []domain.ApprovalFlowNode{{
			Code: "unbind-node", Name: "待解绑节点", Gate: domain.ApprovalFlowGateBeforeExecute,
			ApprovalMode: domain.TemplateApprovalModeAny, ApproverIDs: []string{"u-unbind"},
		}},
	})
	if err != nil {
		t.Fatalf("Create flow failed: %v", err)
	}
	if err := manager.SetApplicationApprovalFlowID(ctx, order.ApplicationID, flow.ID); err != nil {
		t.Fatalf("bind flow failed: %v", err)
	}
	if err := manager.initializeApprovalFlow(ctx, order.ID, flow.ID); err != nil {
		t.Fatalf("initialize flow failed: %v", err)
	}
	if err := manager.SetApplicationApprovalFlowID(ctx, order.ApplicationID, ""); err != nil {
		t.Fatalf("unbind flow failed: %v", err)
	}

	if _, _, err := manager.GetApprovalFlowInstance(ctx, order.ID); !errors.Is(err, domain.ErrApprovalFlowInstanceNotFound) {
		t.Fatalf("GetApprovalFlowInstance after unbind error = %v, want not found", err)
	}
	if err := manager.ensureApprovalFlowDispatchAllowed(ctx, order.ID, ReleaseOrderDispatchActionExecute); err != nil {
		t.Fatalf("unbound release should not be blocked by old flow: %v", err)
	}
	if _, err := repo.GetApprovalFlowInstanceByOrderID(ctx, order.ID); !errors.Is(err, domain.ErrApprovalFlowInstanceNotFound) {
		t.Fatalf("stored flow after unbind error = %v, want not found", err)
	}
}

func TestApprovalFlowRejectsIndependentAgentTaskNodes(t *testing.T) {
	t.Parallel()
	manager, _ := newReleaseOrderManagerForCancelTest(t)
	ctx := context.Background()
	_, err := manager.CreateApprovalFlowDefinition(ctx, SaveApprovalFlowDefinitionInput{
		Name:   "Agent 自动检查",
		Status: domain.ApprovalFlowStatusActive,
		Nodes: []domain.ApprovalFlowNode{{
			Code: "agent-check", Name: "Agent 安全检查", Gate: domain.ApprovalFlowGateBeforeCD,
			NodeType: domain.ApprovalFlowNodeTypeAgentTask, AgentTaskID: "agtask-flow-source",
		}},
	})
	if err == nil {
		t.Fatal("independent Agent approval nodes should be rejected")
	}
	if !strings.Contains(err.Error(), "build-complete agent hook before CD approval") {
		t.Fatalf("unexpected validation error: %v", err)
	}
}

func TestApprovalFlowWithoutCDCompletesAfterBuildChecks(t *testing.T) {
	t.Parallel()
	manager, repo := newReleaseOrderManagerForCancelTest(t)
	ctx := context.Background()
	now := time.Now().UTC()
	order := testReleaseOrder("order-no-cd", "RO-NO-CD", domain.OrderStatusBuilding, now)
	if err := repo.Create(ctx, order, nil, nil, nil); err != nil {
		t.Fatalf("Create order failed: %v", err)
	}
	flow, err := manager.CreateApprovalFlowDefinition(ctx, SaveApprovalFlowDefinitionInput{
		Name:   "无 CD 发布流",
		Status: domain.ApprovalFlowStatusActive,
		Nodes: []domain.ApprovalFlowNode{{
			Code: "ci", Name: "CI 审批", Gate: domain.ApprovalFlowGateBeforeCI,
			ApprovalMode: domain.TemplateApprovalModeAny, ApproverIDs: []string{"u-ci"},
		}},
		Links: []domain.ApprovalFlowLink{
			{FromCode: "start", ToCode: "ci", ExecutionScopes: []string{"build_only"}},
			{FromCode: "ci", ToCode: "waiting_deploy", ExecutionScopes: []string{"build_only"}},
		},
	})
	if err != nil {
		t.Fatalf("CreateApprovalFlowDefinition failed: %v", err)
	}
	if err := manager.initializeApprovalFlow(ctx, order.ID, flow.ID); err != nil {
		t.Fatalf("initializeApprovalFlow failed: %v", err)
	}
	if err := manager.ensureApprovalFlowDispatchAllowed(ctx, order.ID, ReleaseOrderDispatchActionBuild); err == nil {
		t.Fatal("build should first activate CI approval")
	}
	instance, tasks, err := manager.GetApprovalFlowInstance(ctx, order.ID)
	if err != nil || len(tasks) != 1 {
		t.Fatalf("active CI approval instance=%#v tasks=%#v err=%v", instance, tasks, err)
	}
	if _, err := manager.ApproveApprovalFlowTask(ctx, order.ID, tasks[0].ID, "u-ci", "CI approver", "ok"); err != nil {
		t.Fatalf("ApproveApprovalFlowTask failed: %v", err)
	}
	if err := manager.markApprovalFlowDispatched(ctx, order.ID, ReleaseOrderDispatchActionBuild); err != nil {
		t.Fatalf("markApprovalFlowDispatched failed: %v", err)
	}
	if err := manager.completeApprovalFlowWithoutDeployment(ctx, order.ID); err != nil {
		t.Fatalf("completeApprovalFlowWithoutDeployment failed: %v", err)
	}
	instance, _, err = manager.GetApprovalFlowInstance(ctx, order.ID)
	if err != nil {
		t.Fatalf("GetApprovalFlowInstance failed: %v", err)
	}
	if instance.CurrentNodeCode != "end" || instance.CurrentTaskID != "" || instance.Status != domain.ApprovalFlowInstanceStatusCompleted {
		t.Fatalf("completed no-CD flow = %#v", instance)
	}
}

func TestReleaseOrderApprovalFlowGraphSelectsExecutionScopeAndWaitsForCD(t *testing.T) {
	t.Parallel()

	manager, _ := newReleaseOrderManagerForCancelTest(t)
	ctx := context.Background()
	flow, err := manager.CreateApprovalFlowDefinition(ctx, SaveApprovalFlowDefinitionInput{
		Name:   "按执行范围分支",
		Status: domain.ApprovalFlowStatusActive,
		Nodes: []domain.ApprovalFlowNode{
			{Code: "ci", Name: "CI 审批", Gate: domain.ApprovalFlowGateBeforeCI, ApprovalMode: domain.TemplateApprovalModeAny, ApproverIDs: []string{"u-ci"}},
			{Code: "cd", Name: "CD 审批", Gate: domain.ApprovalFlowGateBeforeCD, ApprovalMode: domain.TemplateApprovalModeAny, ApproverIDs: []string{"u-cd"}},
		},
		Links: []domain.ApprovalFlowLink{
			{FromCode: "start", ToCode: "ci", ExecutionScopes: []string{"build_only", "full_release"}},
			{FromCode: "ci", ToCode: "end", ExecutionScopes: []string{"build_only"}},
			{FromCode: "ci", ToCode: "cd", ExecutionScopes: []string{"full_release"}},
			{FromCode: "start", ToCode: "cd", ExecutionScopes: []string{"deploy_only"}},
			{FromCode: "cd", ToCode: "end", ExecutionScopes: []string{"deploy_only", "full_release"}},
		},
	})
	if err != nil {
		t.Fatalf("CreateApprovalFlowDefinition failed: %v", err)
	}
	if err := manager.initializeApprovalFlow(ctx, "order-graph", flow.ID); err != nil {
		t.Fatalf("initializeApprovalFlow failed: %v", err)
	}
	instance, tasks, err := manager.GetApprovalFlowInstance(ctx, "order-graph")
	if err != nil || len(tasks) != 0 || len(instance.Links) != 5 || instance.CurrentScope != "" {
		t.Fatalf("new graph instance = %#v tasks=%#v err=%v", instance, tasks, err)
	}

	executions := []domain.ReleaseOrderExecution{
		{ID: "exec-ci", PipelineScope: domain.PipelineScopeCI, Status: domain.ExecutionStatusPending},
		{ID: "exec-cd", PipelineScope: domain.PipelineScopeCD, Status: domain.ExecutionStatusPending},
	}
	if err := manager.ensureApprovalFlowDispatchAllowedForExecutions(ctx, "order-graph", ReleaseOrderDispatchActionExecute, executions); err == nil {
		t.Fatal("full release should be blocked by CI approval")
	}
	instance, tasks, err = manager.GetApprovalFlowInstance(ctx, "order-graph")
	if err != nil || instance.CurrentScope != domain.ApprovalFlowExecutionScopeFullRelease || instance.CurrentNodeCode != "ci" || len(tasks) != 1 || tasks[0].NodeCode != "ci" {
		t.Fatalf("CI graph state = %#v tasks=%#v err=%v", instance, tasks, err)
	}
	if _, err := manager.ApproveApprovalFlowTask(ctx, "order-graph", tasks[0].ID, "u-ci", "CI", "ok"); err != nil {
		t.Fatalf("ApproveApprovalFlowTask(ci) failed: %v", err)
	}
	instance, tasks, err = manager.GetApprovalFlowInstance(ctx, "order-graph")
	if err != nil || instance.CurrentNodeCode != "ci" || instance.CurrentGate != domain.ApprovalFlowGateBeforeCD || instance.CurrentTaskID != "" || len(tasks) != 1 {
		t.Fatalf("after CI approval = %#v tasks=%#v err=%v", instance, tasks, err)
	}
	if err := manager.ensureApprovalFlowDispatchAllowedForExecutions(ctx, "order-graph", ReleaseOrderDispatchActionExecute, executions); err != nil {
		t.Fatalf("full release should pass CI and wait for CD stage: %v", err)
	}
	if staged, err := manager.shouldStageGraphFullRelease(ctx, "order-graph", ReleaseOrderDispatchActionExecute, executions); err != nil || !staged {
		t.Fatalf("staged full release = %t err=%v, want true", staged, err)
	}
	if err := manager.activateApprovalFlowGate(ctx, "order-graph", domain.ApprovalFlowGateBeforeCD); err != nil {
		t.Fatalf("activateApprovalFlowGate(cd) failed: %v", err)
	}
	instance, tasks, err = manager.GetApprovalFlowInstance(ctx, "order-graph")
	if err != nil || instance.CurrentScope != domain.ApprovalFlowExecutionScopeFullRelease || len(tasks) != 2 || tasks[1].NodeCode != "cd" {
		t.Fatalf("CD graph state = %#v tasks=%#v err=%v", instance, tasks, err)
	}
	if _, err := manager.ApproveApprovalFlowTask(ctx, "order-graph", tasks[1].ID, "u-cd", "CD", "ok"); err != nil {
		t.Fatalf("ApproveApprovalFlowTask(cd) failed: %v", err)
	}
	if err := manager.ensureApprovalFlowDispatchAllowedForExecutions(ctx, "order-graph", ReleaseOrderDispatchActionDeploy, executions); err != nil {
		t.Fatalf("deploy should continue the full-release branch after CD approval: %v", err)
	}
}

func TestReleaseOrderApprovalFlowBuildOnlyPausesAtWaitingDeployBeforeCD(t *testing.T) {
	t.Parallel()

	manager, _ := newReleaseOrderManagerForCancelTest(t)
	ctx := context.Background()
	flow, err := manager.CreateApprovalFlowDefinition(ctx, SaveApprovalFlowDefinitionInput{
		Name:   "仅构建后继续部署审批",
		Status: domain.ApprovalFlowStatusActive,
		Nodes: []domain.ApprovalFlowNode{
			{Code: "ci", Name: "CI 审批", Gate: domain.ApprovalFlowGateBeforeCI, ApprovalMode: domain.TemplateApprovalModeAny, ApproverIDs: []string{"u-ci"}},
			{Code: "cd", Name: "CD 审批", Gate: domain.ApprovalFlowGateBeforeCD, ApprovalMode: domain.TemplateApprovalModeAny, ApproverIDs: []string{"u-cd"}},
		},
		Links: []domain.ApprovalFlowLink{
			{FromCode: "start", ToCode: "ci", ExecutionScopes: []string{"build_only"}},
			{FromCode: "ci", ToCode: "waiting_deploy", ExecutionScopes: []string{"build_only"}},
			{FromCode: "waiting_deploy", ToCode: "cd", ExecutionScopes: []string{"deploy_only"}},
			{FromCode: "cd", ToCode: "end", ExecutionScopes: []string{"deploy_only"}},
		},
	})
	if err != nil {
		t.Fatalf("CreateApprovalFlowDefinition failed: %v", err)
	}
	if err := manager.initializeApprovalFlow(ctx, "order-build-then-deploy", flow.ID); err != nil {
		t.Fatalf("initializeApprovalFlow failed: %v", err)
	}

	if err := manager.ensureApprovalFlowDispatchAllowed(ctx, "order-build-then-deploy", ReleaseOrderDispatchActionBuild); err == nil {
		t.Fatal("build should be blocked by CI approval")
	}
	instance, tasks, err := manager.GetApprovalFlowInstance(ctx, "order-build-then-deploy")
	if err != nil || len(tasks) != 1 || tasks[0].NodeCode != "ci" {
		t.Fatalf("CI graph state = %#v tasks=%#v err=%v", instance, tasks, err)
	}
	if _, err := manager.ApproveApprovalFlowTask(ctx, "order-build-then-deploy", tasks[0].ID, "u-ci", "CI", "ok"); err != nil {
		t.Fatalf("ApproveApprovalFlowTask(ci) failed: %v", err)
	}
	if err := manager.ensureApprovalFlowDispatchAllowed(ctx, "order-build-then-deploy", ReleaseOrderDispatchActionBuild); err != nil {
		t.Fatalf("build should be allowed after CI approval: %v", err)
	}
	if err := manager.markApprovalFlowDispatched(ctx, "order-build-then-deploy", ReleaseOrderDispatchActionBuild); err != nil {
		t.Fatalf("markApprovalFlowDispatched(build) failed: %v", err)
	}
	if err := manager.activateApprovalFlowGate(ctx, "order-build-then-deploy", domain.ApprovalFlowGateBeforeCD); err != nil {
		t.Fatalf("activateApprovalFlowGate(cd) failed: %v", err)
	}
	instance, tasks, err = manager.GetApprovalFlowInstance(ctx, "order-build-then-deploy")
	if err != nil || instance.CurrentNodeCode != "waiting_deploy" || instance.CurrentScope != domain.ApprovalFlowExecutionScopeBuildOnly || instance.CurrentGate != domain.ApprovalFlowGateBeforeCD || instance.Status != domain.ApprovalFlowInstanceStatusWaitingCD || len(tasks) != 1 {
		t.Fatalf("waiting deploy state = %#v tasks=%#v err=%v", instance, tasks, err)
	}

	if err := manager.ensureApprovalFlowDispatchAllowed(ctx, "order-build-then-deploy", ReleaseOrderDispatchActionDeploy); err == nil {
		t.Fatal("deploy should be blocked by CD approval")
	}
	instance, tasks, err = manager.GetApprovalFlowInstance(ctx, "order-build-then-deploy")
	if err != nil || instance.CurrentScope != domain.ApprovalFlowExecutionScopeDeployOnly || instance.CurrentNodeCode != "cd" || len(tasks) != 2 || tasks[1].NodeCode != "cd" {
		t.Fatalf("CD graph state = %#v tasks=%#v err=%v", instance, tasks, err)
	}
	if _, err := manager.ApproveApprovalFlowTask(ctx, "order-build-then-deploy", tasks[1].ID, "u-cd", "CD", "ok"); err != nil {
		t.Fatalf("ApproveApprovalFlowTask(cd) failed: %v", err)
	}
	if err := manager.ensureApprovalFlowDispatchAllowed(ctx, "order-build-then-deploy", ReleaseOrderDispatchActionDeploy); err != nil {
		t.Fatalf("deploy should be allowed after CD approval: %v", err)
	}
}

func TestApprovalFlowDefinitionPersistsCanvasNodePosition(t *testing.T) {
	t.Parallel()

	manager, _ := newReleaseOrderManagerForCancelTest(t)
	ctx := context.Background()
	flow, err := manager.CreateApprovalFlowDefinition(ctx, SaveApprovalFlowDefinitionInput{
		Name:   "画布坐标持久化",
		Status: domain.ApprovalFlowStatusActive,
		Nodes: []domain.ApprovalFlowNode{{
			Code: "approval-canvas", Name: "画布审批", Gate: domain.ApprovalFlowGateBeforeExecute,
			ApprovalMode: domain.TemplateApprovalModeAny, ApproverIDs: []string{"u-canvas"}, PositionX: 318, PositionY: 426,
		}},
	})
	if err != nil {
		t.Fatalf("CreateApprovalFlowDefinition failed: %v", err)
	}
	loaded, err := manager.ListApprovalFlowDefinitions(ctx, domain.ApprovalFlowStatusActive)
	if err != nil || len(loaded) != 1 {
		t.Fatalf("ListApprovalFlowDefinitions = %#v err=%v", loaded, err)
	}
	node := loaded[0].Nodes[0]
	if flow.ID != loaded[0].ID || node.PositionX != 318 || node.PositionY != 426 {
		t.Fatalf("persisted canvas position = %#v, want x=318 y=426", node)
	}
}

type approvalManagerResolverFake struct {
	manager userdomain.User
	userID  string
	level   int
}

func (f *approvalManagerResolverFake) ResolveUserManager(_ context.Context, userID string, level int) (userdomain.User, error) {
	f.userID, f.level = userID, level
	return f.manager, nil
}

func TestApprovalFlowResolvesCreatorOrganizationManagerIntoTaskSnapshot(t *testing.T) {
	t.Parallel()

	manager, repo := newReleaseOrderManagerForCancelTest(t)
	resolver := &approvalManagerResolverFake{manager: userdomain.User{ID: "u-manager", Username: "manager", DisplayName: "直属主管", Status: userdomain.StatusActive}}
	manager.SetApprovalManagerResolver(resolver)
	ctx := context.Background()
	now := time.Now().UTC()
	order := testReleaseOrder("order-manager-approval", "RO-MANAGER", domain.OrderStatusPending, now)
	order.CreatorUserID = "u-creator"
	if err := repo.Create(ctx, order, nil, nil, nil); err != nil {
		t.Fatalf("Create order failed: %v", err)
	}
	flow, err := manager.CreateApprovalFlowDefinition(ctx, SaveApprovalFlowDefinitionInput{
		Name:   "直属主管审批",
		Status: domain.ApprovalFlowStatusActive,
		Nodes: []domain.ApprovalFlowNode{{
			Code: "manager-approval", Name: "二级主管审批", Gate: domain.ApprovalFlowGateBeforeExecute,
			ApprovalMode: domain.TemplateApprovalModeAny, ApproverSource: domain.ApprovalFlowApproverSourceManager, ManagerLevel: 2,
		}},
	})
	if err != nil {
		t.Fatalf("CreateApprovalFlowDefinition failed: %v", err)
	}
	if err := manager.initializeApprovalFlow(ctx, order.ID, flow.ID); err != nil {
		t.Fatalf("initializeApprovalFlow failed: %v", err)
	}
	instance, tasks, err := manager.GetApprovalFlowInstance(ctx, order.ID)
	if err != nil || len(tasks) != 0 || len(instance.Nodes) != 1 {
		t.Fatalf("new approval snapshot instance=%#v tasks=%#v err=%v", instance, tasks, err)
	}
	if resolver.userID != "u-creator" || resolver.level != 2 {
		t.Fatalf("resolver input user=%q level=%d", resolver.userID, resolver.level)
	}
	if len(instance.Nodes[0].ApproverIDs) != 1 || instance.Nodes[0].ApproverIDs[0] != "u-manager" || len(instance.Nodes[0].ApproverNames) != 1 || instance.Nodes[0].ApproverNames[0] != "直属主管" {
		t.Fatalf("resolved node snapshot = %#v", instance.Nodes[0])
	}
	if dispatchErr := manager.ensureApprovalFlowDispatchAllowed(ctx, order.ID, ReleaseOrderDispatchActionExecute); !errors.Is(dispatchErr, ErrApprovalFlowPending) {
		t.Fatalf("first dispatch should start manager approval, got %v", dispatchErr)
	}
	_, tasks, err = manager.GetApprovalFlowInstance(ctx, order.ID)
	if err != nil || len(tasks) != 1 {
		t.Fatalf("started manager approval tasks=%#v err=%v", tasks, err)
	}
	if len(tasks[0].ApproverIDs) != 1 || tasks[0].ApproverIDs[0] != "u-manager" || len(tasks[0].ApproverNames) != 1 || tasks[0].ApproverNames[0] != "直属主管" {
		t.Fatalf("resolved task snapshot = %#v", tasks[0])
	}
}
