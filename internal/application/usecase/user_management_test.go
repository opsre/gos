package usecase

import (
	"testing"

	userdomain "gos/internal/domain/user"
)

// TestFilterUserPermissionsByReleaseEnvOptions 封装当前模块的业务处理逻辑。
func TestFilterUserPermissionsByReleaseEnvOptions(t *testing.T) {
	items := []userdomain.UserPermission{
		{PermissionCode: "release.create", ScopeType: "application_env", ScopeValue: "app-1::dev", Enabled: true},
		{PermissionCode: "release.create", ScopeType: "application_env", ScopeValue: "app-1::prod", Enabled: true},
		{PermissionCode: "release.create", ScopeType: "application", ScopeValue: "app-2", Enabled: true},
		{PermissionCode: "release.create", ScopeType: "application_env", ScopeValue: "broken", Enabled: true},
	}

	filtered := filterUserPermissionsByReleaseEnvOptions(items, map[string]struct{}{"prod": {}})
	if len(filtered) != 2 {
		t.Fatalf("expected 2 permissions after filtering, got %d", len(filtered))
	}
	if filtered[0].ScopeValue != "app-1::prod" && filtered[1].ScopeValue != "app-1::prod" {
		t.Fatalf("expected prod-scoped permission to be preserved")
	}
}

// TestMatchesReleaseScopedPermission 封装当前模块的业务处理逻辑。
func TestMatchesReleaseScopedPermission(t *testing.T) {
	if !matchesReleaseScopedPermission("application_env", "app-1::prod", "application", "app-1") {
		t.Fatalf("expected application_env permission to satisfy application-level lookup")
	}
	if !matchesReleaseScopedPermission("application", "app-1", "application_env", "app-1::prod") {
		t.Fatalf("expected legacy application permission to satisfy env-level lookup")
	}
	if matchesReleaseScopedPermission("application_env", "app-1::dev", "application_env", "app-1::prod") {
		t.Fatalf("expected different env permissions to not match")
	}
}
