package usecase

import (
	"context"
	"errors"
	"testing"
	"time"

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

func TestAuthSessionManagerLoginInvalidatesPreviousSession(t *testing.T) {
	t.Parallel()

	passwordHash, err := HashPassword("secret")
	if err != nil {
		t.Fatalf("HashPassword failed: %v", err)
	}
	now := time.Unix(1_710_000_000, 0).UTC()
	repo := &authSessionUserRepoFake{
		usersByUsername: map[string]userdomain.User{
			"admin": {
				ID:           "usr-1",
				Username:     "admin",
				DisplayName:  "管理员",
				Role:         userdomain.RoleAdmin,
				Status:       userdomain.StatusActive,
				PasswordHash: passwordHash,
				CreatedAt:    now,
				UpdatedAt:    now,
			},
		},
		usersByID: map[string]userdomain.User{},
		sessions:  map[string]userdomain.UserSession{},
	}
	repo.usersByID["usr-1"] = repo.usersByUsername["admin"]

	manager := NewAuthSessionManager(repo, nil, time.Hour)
	manager.now = func() time.Time { return now }

	first, err := manager.Login(context.Background(), LoginInput{Username: "admin", Password: "secret"})
	if err != nil {
		t.Fatalf("first Login failed: %v", err)
	}
	second, err := manager.Login(context.Background(), LoginInput{Username: "admin", Password: "secret"})
	if err != nil {
		t.Fatalf("second Login failed: %v", err)
	}
	if first.AccessToken == second.AccessToken {
		t.Fatalf("expected different tokens")
	}
	if active := repo.activeSessionCount(); active != 1 {
		t.Fatalf("active sessions = %d, want 1", active)
	}
	if _, _, err := manager.ResolveUserByToken(context.Background(), first.AccessToken); !errors.Is(err, userdomain.ErrSessionRevoked) {
		t.Fatalf("old token err = %v, want ErrSessionRevoked", err)
	}
	if _, _, err := manager.ResolveUserByToken(context.Background(), second.AccessToken); err != nil {
		t.Fatalf("new token should remain valid: %v", err)
	}
}

type authSessionUserRepoFake struct {
	usersByUsername map[string]userdomain.User
	usersByID       map[string]userdomain.User
	sessions        map[string]userdomain.UserSession
}

func (r *authSessionUserRepoFake) InitSchema(context.Context) error { return nil }

func (r *authSessionUserRepoFake) EnsureSeedData(context.Context, string, string, string, time.Time) error {
	return nil
}

func (r *authSessionUserRepoFake) CreateUser(_ context.Context, item userdomain.User) error {
	r.usersByUsername[item.Username] = item
	r.usersByID[item.ID] = item
	return nil
}

func (r *authSessionUserRepoFake) GetUserByID(_ context.Context, id string) (userdomain.User, error) {
	item, ok := r.usersByID[id]
	if !ok {
		return userdomain.User{}, userdomain.ErrUserNotFound
	}
	return item, nil
}

func (r *authSessionUserRepoFake) GetUserByUsername(_ context.Context, username string) (userdomain.User, error) {
	item, ok := r.usersByUsername[username]
	if !ok {
		return userdomain.User{}, userdomain.ErrUserNotFound
	}
	return item, nil
}

func (r *authSessionUserRepoFake) ListUsers(context.Context, userdomain.UserListFilter) ([]userdomain.User, int64, error) {
	return nil, 0, nil
}

func (r *authSessionUserRepoFake) UpdateUser(context.Context, string, userdomain.UserUpdateInput, time.Time) (userdomain.User, error) {
	return userdomain.User{}, nil
}

func (r *authSessionUserRepoFake) DeleteUser(context.Context, string) error { return nil }

func (r *authSessionUserRepoFake) ListUserOptions(context.Context) ([]userdomain.User, error) {
	return nil, nil
}

func (r *authSessionUserRepoFake) ListPermissions(context.Context, userdomain.PermissionFilter) ([]userdomain.Permission, error) {
	return nil, nil
}

func (r *authSessionUserRepoFake) ListUserPermissions(context.Context, string) ([]userdomain.UserPermission, error) {
	return nil, nil
}

func (r *authSessionUserRepoFake) GrantUserPermissions(context.Context, string, []userdomain.UserPermissionGrant, time.Time) error {
	return nil
}

func (r *authSessionUserRepoFake) RevokeUserPermissions(context.Context, string, []userdomain.UserPermissionGrant) error {
	return nil
}

func (r *authSessionUserRepoFake) ListUserParamPermissions(context.Context, string, string) ([]userdomain.UserParamPermission, error) {
	return nil, nil
}

func (r *authSessionUserRepoFake) UpsertUserParamPermission(context.Context, userdomain.UserParamPermission) (userdomain.UserParamPermission, error) {
	return userdomain.UserParamPermission{}, nil
}

func (r *authSessionUserRepoFake) DeleteUserParamPermission(context.Context, string) error {
	return nil
}

func (r *authSessionUserRepoFake) CreateSession(_ context.Context, item userdomain.UserSession) error {
	r.sessions[item.AccessToken] = item
	return nil
}

func (r *authSessionUserRepoFake) GetSessionByAccessToken(_ context.Context, token string) (userdomain.UserSession, error) {
	item, ok := r.sessions[token]
	if !ok {
		return userdomain.UserSession{}, userdomain.ErrSessionNotFound
	}
	return item, nil
}

func (r *authSessionUserRepoFake) DeleteSessionByAccessToken(_ context.Context, token string) error {
	delete(r.sessions, token)
	return nil
}

func (r *authSessionUserRepoFake) RevokeSessionsByUserID(_ context.Context, userID string, reason string, revokedAt time.Time) (int64, error) {
	var revoked int64
	for token, session := range r.sessions {
		if session.UserID == userID {
			session.RevokedAt = &revokedAt
			session.RevokedReason = reason
			r.sessions[token] = session
			revoked++
		}
	}
	return revoked, nil
}

func (r *authSessionUserRepoFake) DeleteExpiredSessions(_ context.Context, now time.Time) (int64, error) {
	var deleted int64
	for token, session := range r.sessions {
		if !session.ExpiredAt.After(now) {
			delete(r.sessions, token)
			deleted++
		}
	}
	return deleted, nil
}

func (r *authSessionUserRepoFake) activeSessionCount() int {
	count := 0
	for _, session := range r.sessions {
		if session.RevokedAt == nil {
			count++
		}
	}
	return count
}
