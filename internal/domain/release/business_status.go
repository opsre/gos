package release

// DeriveBusinessStatus 封装当前模块的业务处理逻辑。
func DeriveBusinessStatus(
	status OrderStatus,
	executions []ReleaseOrderExecution,
) ReleaseBusinessStatus {
	switch status {
	case OrderStatusDraft:
		return ReleaseBusinessStatusDraft
	case OrderStatusPendingApproval:
		return ReleaseBusinessStatusPendingApproval
	case OrderStatusApproving:
		return ReleaseBusinessStatusApproving
	case OrderStatusApproved:
		return ReleaseBusinessStatusApproved
	case OrderStatusBuiltWaitingDeploy:
		return ReleaseBusinessStatusBuiltWaitingDeploy
	case OrderStatusRejected:
		return ReleaseBusinessStatusRejected
	case OrderStatusDeploySuccess:
		return ReleaseBusinessStatusDeploySuccess
	case OrderStatusDeployFailed:
		return ReleaseBusinessStatusDeployFailed
	case OrderStatusCancelled:
		return ReleaseBusinessStatusCancelled
	case OrderStatusSuccess:
		return ReleaseBusinessStatusDeploySuccess
	case OrderStatusFailed:
		return ReleaseBusinessStatusDeployFailed
	case OrderStatusPending:
		return ReleaseBusinessStatusPendingExecution
	}

	// The persisted order status represents the overall release intent. A full
	// CI -> CD release therefore remains "deploying" while its CI execution is
	// actually building. Project the user-facing phase from the active execution
	// so the list can distinguish Jenkins queueing, CI building and CD deploying.
	if runningStatus, ok := deriveRunningExecutionBusinessStatus(executions); ok {
		return runningStatus
	}
	// The tracker parks a settling order on OrderStatusRunning so the next sync
	// still picks it up before the order reaches its terminal status. Once every
	// execution has ended, project that outcome: a release whose CI failed must
	// not keep showing the queue/phase status it was parked on.
	if terminalStatus, ok := deriveEndedExecutionBusinessStatus(executions); ok {
		return terminalStatus
	}

	switch status {
	case OrderStatusBuilding:
		return ReleaseBusinessStatusBuilding
	case OrderStatusQueued:
		return ReleaseBusinessStatusQueued
	case OrderStatusDeploying:
		return ReleaseBusinessStatusDeploying
	case OrderStatusRunning:
		return ReleaseBusinessStatusQueued
	default:
		return ReleaseBusinessStatusPendingExecution
	}
}

func deriveEndedExecutionBusinessStatus(
	executions []ReleaseOrderExecution,
) (ReleaseBusinessStatus, bool) {
	if len(executions) == 0 {
		return "", false
	}
	hasFailed := false
	hasCancelled := false
	for _, execution := range executions {
		if !execution.Status.IsTerminal() {
			return "", false
		}
		switch execution.Status {
		case ExecutionStatusFailed:
			hasFailed = true
		case ExecutionStatusCancelled:
			hasCancelled = true
		}
	}
	switch {
	case hasFailed:
		return ReleaseBusinessStatusDeployFailed, true
	case hasCancelled:
		return ReleaseBusinessStatusCancelled, true
	default:
		return "", false
	}
}

func deriveRunningExecutionBusinessStatus(
	executions []ReleaseOrderExecution,
) (ReleaseBusinessStatus, bool) {
	hasRunningCI := false
	hasRunningCD := false
	for _, execution := range executions {
		if execution.Status != ExecutionStatusRunning {
			continue
		}
		switch execution.PipelineScope {
		case PipelineScopeCI:
			hasRunningCI = true
		case PipelineScopeCD:
			hasRunningCD = true
		}
	}
	// The execution status is the authoritative phase signal. Jenkins can already
	// be building while build_url is still waiting for the next tracker refresh;
	// treating that synchronization window as the release/concurrency queue makes
	// the whole-order banner contradict the running execution card.
	if hasRunningCD {
		return ReleaseBusinessStatusDeploying, true
	}
	if hasRunningCI {
		return ReleaseBusinessStatusBuilding, true
	}
	return "", false
}
