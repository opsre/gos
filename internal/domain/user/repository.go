package user

import (
	"context"
	"time"
)

type Repository interface {
	InitSchema(ctx context.Context) error
	EnsureSeedData(ctx context.Context, adminUsername string, adminDisplayName string, adminPasswordHash string, now time.Time) error

	CreateUser(ctx context.Context, item User) error
	GetUserByID(ctx context.Context, id string) (User, error)
	GetUserByUsername(ctx context.Context, username string) (User, error)
	ListUsers(ctx context.Context, filter UserListFilter) ([]User, int64, error)
	UpdateUser(ctx context.Context, id string, input UserUpdateInput, updatedAt time.Time) (User, error)
	DeleteUser(ctx context.Context, id string) error
	ListUserOptions(ctx context.Context) ([]User, error)

	ListPermissions(ctx context.Context, filter PermissionFilter) ([]Permission, error)
	ListUserPermissions(ctx context.Context, userID string) ([]UserPermission, error)
	GrantUserPermissions(ctx context.Context, userID string, items []UserPermissionGrant, now time.Time) error
	RevokeUserPermissions(ctx context.Context, userID string, items []UserPermissionGrant) error

	ListUserParamPermissions(ctx context.Context, userID string, applicationID string) ([]UserParamPermission, error)
	UpsertUserParamPermission(ctx context.Context, item UserParamPermission) (UserParamPermission, error)
	DeleteUserParamPermission(ctx context.Context, id string) error

	CreateSession(ctx context.Context, item UserSession) error
	GetSessionByAccessToken(ctx context.Context, token string) (UserSession, error)
	DeleteSessionByAccessToken(ctx context.Context, token string) error
	RevokeSessionsByUserID(ctx context.Context, userID string, reason string, revokedAt time.Time) (int64, error)
	DeleteExpiredSessions(ctx context.Context, now time.Time) (int64, error)
}
