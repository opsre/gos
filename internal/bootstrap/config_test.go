package bootstrap

import (
	"os"
	"path/filepath"
	"testing"
)

// TestLoadConfigFromPathDoesNotUseEnvOverrides 封装当前模块的业务处理逻辑。
func TestLoadConfigFromPathDoesNotUseEnvOverrides(t *testing.T) {
	t.Setenv("MYSQL_DSN", "env-dsn-should-not-be-used")
	t.Setenv("AUTH_ADMIN_PASSWORD", "env-admin-password")
	t.Setenv("APP_ENCRYPTION_KEY", "env-encryption-key")

	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	content := `{
  "database": {
    "driver": "mysql",
    "mysql_dsn": "file-dsn"
  },
  "auth": {
    "admin_password": "file-admin-password"
  },
  "security": {
    "encryption_key": "file-encryption-key"
  }
}`
	if err := os.WriteFile(configPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := LoadConfigFromPath(configPath)
	if err != nil {
		t.Fatalf("LoadConfigFromPath() error = %v", err)
	}

	if cfg.Database.MySQLDSN != "file-dsn" {
		t.Fatalf("expected mysql_dsn from file, got %q", cfg.Database.MySQLDSN)
	}
	if cfg.Auth.AdminPassword != "file-admin-password" {
		t.Fatalf("expected admin_password from file, got %q", cfg.Auth.AdminPassword)
	}
	if cfg.Security.EncryptionKey != "file-encryption-key" {
		t.Fatalf("expected encryption_key from file, got %q", cfg.Security.EncryptionKey)
	}
}

// TestResolveConfigPathUsesDefaultWhenEmpty 解析上下文数据，得到后续流程需要的结果。
func TestResolveConfigPathUsesDefaultWhenEmpty(t *testing.T) {
	if got := ResolveConfigPath(""); got != "configs/config.local.json" {
		t.Fatalf("ResolveConfigPath(\"\") = %q, want %q", got, "configs/config.local.json")
	}
	if got := ResolveConfigPath("  configs/config.production.json  "); got != "configs/config.production.json" {
		t.Fatalf("ResolveConfigPath(custom) = %q", got)
	}
}

func TestLoadConfigFromPathAcceptsReleaseEnvironmentSettings(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	content := `{
  "auth": {
    "admin_password": "test-password"
  },
  "release": {
    "default_env_code": " prod ",
    "env_configs": [
      {"code": "dev", "description": "日常联调"},
      {"code": "prod", "description": "生产发布"}
    ]
  }
}`
	if err := os.WriteFile(configPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := LoadConfigFromPath(configPath)
	if err != nil {
		t.Fatalf("LoadConfigFromPath() error = %v", err)
	}
	if cfg.Release.DefaultEnvCode != "prod" {
		t.Fatalf("DefaultEnvCode = %q, want prod", cfg.Release.DefaultEnvCode)
	}
	if len(cfg.Release.EnvConfigs) != 2 || cfg.Release.EnvConfigs[0].Code != "dev" || cfg.Release.EnvConfigs[1].Description != "生产发布" {
		t.Fatalf("EnvConfigs = %#v, want configured env metadata", cfg.Release.EnvConfigs)
	}
}
