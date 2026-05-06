package usecase

import (
	"testing"

	agentdomain "gos/internal/domain/agent"
)

// TestIsReusableAgentTaskHookTarget 查询并返回指定资源数据。
func TestIsReusableAgentTaskHookTarget(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		task agentdomain.Task
		want bool
	}{
		{
			name: "manual temporary task",
			task: agentdomain.Task{TaskMode: agentdomain.TaskModeTemporary},
			want: true,
		},
		{
			name: "resident task",
			task: agentdomain.Task{TaskMode: agentdomain.TaskModeResident},
			want: false,
		},
		{
			name: "dispatched history task",
			task: agentdomain.Task{TaskMode: agentdomain.TaskModeTemporary, SourceTaskID: "agtask-source"},
			want: false,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := isReusableAgentTaskHookTarget(tc.task); got != tc.want {
				t.Fatalf("isReusableAgentTaskHookTarget() = %v, want %v", got, tc.want)
			}
		})
	}
}
