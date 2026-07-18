package sqlrepo

import (
	"context"
	"database/sql"
	"testing"
	"time"

	domain "gos/internal/domain/user"

	_ "modernc.org/sqlite"
)

func TestUserRepositoryResolvesManagerHierarchy(t *testing.T) {
	t.Parallel()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("sql.Open failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	repo := NewUserRepository(db, "sqlite")
	ctx := context.Background()
	if err := repo.InitSchema(ctx); err != nil {
		t.Fatalf("InitSchema failed: %v", err)
	}
	now := time.Now().UTC()
	for _, user := range []domain.User{
		{ID: "u-staff", Username: "staff", DisplayName: "员工", Role: domain.RoleNormal, Status: domain.StatusActive, PasswordHash: "x", CreatedAt: now, UpdatedAt: now},
		{ID: "u-manager", Username: "manager", DisplayName: "主管", Role: domain.RoleNormal, Status: domain.StatusActive, PasswordHash: "x", CreatedAt: now, UpdatedAt: now},
		{ID: "u-director", Username: "director", DisplayName: "总监", Role: domain.RoleNormal, Status: domain.StatusActive, PasswordHash: "x", CreatedAt: now, UpdatedAt: now},
		{ID: "u-admin", Username: "admin", DisplayName: "超级管理员", Role: domain.RoleAdmin, Status: domain.StatusActive, PasswordHash: "x", CreatedAt: now, UpdatedAt: now},
	} {
		if err := repo.CreateUser(ctx, user); err != nil {
			t.Fatalf("CreateUser(%s) failed: %v", user.ID, err)
		}
	}
	if err := repo.SetUserManagerID(ctx, "u-staff", "u-manager", now); err != nil {
		t.Fatalf("Set staff manager failed: %v", err)
	}
	if err := repo.SetUserManagerID(ctx, "u-manager", "u-director", now); err != nil {
		t.Fatalf("Set manager manager failed: %v", err)
	}
	first, err := repo.ResolveUserManager(ctx, "u-staff", 1)
	if err != nil || first.ID != "u-manager" {
		t.Fatalf("first manager = %#v err=%v", first, err)
	}
	second, err := repo.ResolveUserManager(ctx, "u-staff", 2)
	if err != nil || second.ID != "u-director" {
		t.Fatalf("second manager = %#v err=%v", second, err)
	}
	nodes, err := repo.ListUserOrganization(ctx)
	if err != nil || len(nodes) != 3 {
		t.Fatalf("ListUserOrganization nodes=%#v err=%v", nodes, err)
	}
	managerByUserID := map[string]string{}
	for _, node := range nodes {
		managerByUserID[node.User.ID] = node.ManagerUserID
	}
	if managerByUserID["u-staff"] != "u-manager" || managerByUserID["u-manager"] != "u-director" {
		t.Fatalf("organization manager map = %#v", managerByUserID)
	}
	if _, exists := managerByUserID["u-admin"]; exists {
		t.Fatalf("administrator must not be returned by organization query: %#v", managerByUserID)
	}
	if err := repo.DeleteUser(ctx, "u-manager"); err != nil {
		t.Fatalf("DeleteUser(manager) failed: %v", err)
	}
	if _, err := repo.GetUserManagerID(ctx, "u-staff"); err != domain.ErrUserManagerNotFound {
		t.Fatalf("staff manager after delete err=%v, want ErrUserManagerNotFound", err)
	}
}
