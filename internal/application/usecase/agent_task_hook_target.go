package usecase

import (
	"strings"

	agentdomain "gos/internal/domain/agent"
)

// isReusableAgentTaskHookTarget 查询并返回指定资源数据。
func isReusableAgentTaskHookTarget(task agentdomain.Task) bool {
	return task.TaskMode == agentdomain.TaskModeTemporary && strings.TrimSpace(task.SourceTaskID) == ""
}
