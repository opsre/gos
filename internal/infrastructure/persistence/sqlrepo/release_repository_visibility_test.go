package sqlrepo

import (
	"strings"
	"testing"

	releasedomain "gos/internal/domain/release"
)

// TestBuildReleaseOrderVisibilityClauseWithEnvScopes 组装业务执行所需的输入数据。
func TestBuildReleaseOrderVisibilityClauseWithEnvScopes(t *testing.T) {
	clause, args := buildReleaseOrderVisibilityClauseWithAlias(
		"ro",
		[]string{"app-global"},
		[]releasedomain.ApplicationEnvScope{{ApplicationID: "app-env", EnvCode: "prod"}},
		"user-1",
	)

	if !strings.Contains(clause, "ro.application_id IN") {
		t.Fatalf("expected application visibility clause, got %s", clause)
	}
	if !strings.Contains(clause, "ro.application_id = ? AND ro.env_code = ?") {
		t.Fatalf("expected env visibility clause, got %s", clause)
	}
	if !strings.Contains(clause, "ro.creator_user_id = ?") {
		t.Fatalf("expected creator visibility clause, got %s", clause)
	}
	if len(args) != 5 {
		t.Fatalf("expected 5 query args, got %d", len(args))
	}
}
