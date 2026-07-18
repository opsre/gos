package configstore

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestReleaseStoreLoadEnvConfigsFromFileAndRespectExplicitEmpty(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte(`{
  "release": {
    "env_configs": [
      {"code": "dev", "description": "日常联调环境"},
      {"code": "prod", "description": "生产环境"}
    ]
  }
}`), 0o644); err != nil {
		t.Fatalf("write config failed: %v", err)
	}

	store := NewReleaseStore(path)
	configs, err := store.LoadEnvConfigs(context.Background())
	if err != nil {
		t.Fatalf("LoadEnvConfigs failed: %v", err)
	}
	if len(configs) != 2 || configs[0].Description != "日常联调环境" || configs[1].Code != "prod" {
		t.Fatalf("env configs = %#v, want configured descriptions", configs)
	}

	if err := store.SaveEnvOptions(context.Background(), nil); err != nil {
		t.Fatalf("SaveEnvOptions empty failed: %v", err)
	}
	options, err := store.LoadEnvOptions(context.Background())
	if err != nil {
		t.Fatalf("LoadEnvOptions empty failed: %v", err)
	}
	if len(options) != 0 {
		t.Fatalf("env options after explicit empty = %#v, want empty", options)
	}
}
