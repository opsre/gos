package sqlrepo

import (
	"context"
	"database/sql"
	domain "gos/internal/domain/platformparam"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

// TestPlatformParamRepositoryInitSchemaSyncsBuiltinAppKeyDescription 同步外部或内部状态数据。
func TestPlatformParamRepositoryInitSchemaSyncsBuiltinAppKeyDescription(t *testing.T) {
	t.Parallel()

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite failed: %v", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	t.Cleanup(func() {
		_ = db.Close()
	})

	repo := NewPlatformParamRepository(db, "sqlite")
	if err := repo.InitSchema(context.Background()); err != nil {
		t.Fatalf("InitSchema failed: %v", err)
	}

	item, err := repo.GetByParamKey(context.Background(), "app_key")
	if err != nil {
		t.Fatalf("GetByParamKey failed: %v", err)
	}

	const want = ""
	if item.Description != want {
		t.Fatalf("app_key description = %q, want %q", item.Description, want)
	}
}

// TestPlatformParamRepositoryInitSchemaSeedsBuiltinReleaseName 同步外部或内部状态数据。
func TestPlatformParamRepositoryInitSchemaSeedsBuiltinReleaseName(t *testing.T) {
	t.Parallel()

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite failed: %v", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	t.Cleanup(func() {
		_ = db.Close()
	})

	repo := NewPlatformParamRepository(db, "sqlite")
	if err := repo.InitSchema(context.Background()); err != nil {
		t.Fatalf("InitSchema failed: %v", err)
	}

	item, err := repo.GetByParamKey(context.Background(), "release_name")
	if err != nil {
		t.Fatalf("GetByParamKey release_name failed: %v", err)
	}
	if !item.Builtin {
		t.Fatal("release_name builtin = false, want true")
	}
	if item.Name != "发布名称" {
		t.Fatalf("release_name name = %q, want %q", item.Name, "发布名称")
	}
}

// TestPlatformParamRepositoryInitSchemaSeedsBuiltinOSSParams 同步制品库 OSS 内置字段。
func TestPlatformParamRepositoryInitSchemaSeedsBuiltinOSSParams(t *testing.T) {
	t.Parallel()

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite failed: %v", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	t.Cleanup(func() {
		_ = db.Close()
	})

	repo := NewPlatformParamRepository(db, "sqlite")
	if err := repo.InitSchema(context.Background()); err != nil {
		t.Fatalf("InitSchema failed: %v", err)
	}

	cases := []struct {
		key          string
		name         string
		jenkinsParam string
	}{
		{key: "oss_endpoint", name: "OSS Endpoint", jenkinsParam: "OSS_ENDPOINT"},
		{key: "oss_bucket", name: "OSS Bucket", jenkinsParam: "OSS_BUCKET"},
		{key: "oss_dir", name: "OSS 目录", jenkinsParam: "OSS_DIR"},
		{key: "oss_acl", name: "OSS ACL", jenkinsParam: "OSS_ACL"},
		{key: "oss_access_key_id", name: "OSS AccessKey ID", jenkinsParam: "OSS_ACCESS_KEY_ID"},
		{key: "oss_access_key_secret", name: "OSS AccessKey Secret", jenkinsParam: "OSS_ACCESS_KEY_SECRET"},
	}

	for _, tc := range cases {
		item, err := repo.GetByParamKey(context.Background(), tc.key)
		if err != nil {
			t.Fatalf("GetByParamKey %s failed: %v", tc.key, err)
		}
		if !item.Builtin {
			t.Fatalf("%s builtin = false, want true", tc.key)
		}
		if item.Status != domain.StatusEnabled {
			t.Fatalf("%s status = %d, want %d", tc.key, item.Status, domain.StatusEnabled)
		}
		if item.ParamType != domain.ParamTypeString {
			t.Fatalf("%s param_type = %q, want %q", tc.key, item.ParamType, domain.ParamTypeString)
		}
		if item.Required {
			t.Fatalf("%s required = true, want false", tc.key)
		}
		if item.Name != tc.name {
			t.Fatalf("%s name = %q, want %q", tc.key, item.Name, tc.name)
		}
		if !strings.Contains(item.Description, tc.jenkinsParam) {
			t.Fatalf("%s description = %q, want to contain %q", tc.key, item.Description, tc.jenkinsParam)
		}
	}
}

// TestPlatformParamRepositoryInitSchemaSeedsBuiltinGOSArtifactURL 同步 GOS 制品地址内置字段。
func TestPlatformParamRepositoryInitSchemaSeedsBuiltinGOSArtifactURL(t *testing.T) {
	t.Parallel()

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite failed: %v", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	t.Cleanup(func() {
		_ = db.Close()
	})

	repo := NewPlatformParamRepository(db, "sqlite")
	if err := repo.InitSchema(context.Background()); err != nil {
		t.Fatalf("InitSchema failed: %v", err)
	}

	item, err := repo.GetByParamKey(context.Background(), "gos_artifact_url")
	if err != nil {
		t.Fatalf("GetByParamKey gos_artifact_url failed: %v", err)
	}
	if !item.Builtin {
		t.Fatal("gos_artifact_url builtin = false, want true")
	}
	if item.Name != "GOS_ARTIFACT_URL" {
		t.Fatalf("gos_artifact_url name = %q, want GOS_ARTIFACT_URL", item.Name)
	}
	if !strings.Contains(item.Description, "GOS_ARTIFACT_URL=") {
		t.Fatalf("gos_artifact_url description = %q, want to contain GOS_ARTIFACT_URL=", item.Description)
	}
	for _, want := range []string{"管线扫描", "CI/CD", "只沿用 CI 单元", "不从 CD 单元取值"} {
		if !strings.Contains(item.Description, want) {
			t.Fatalf("gos_artifact_url description = %q, want to contain %q", item.Description, want)
		}
	}
}
