package bootstrap

import "testing"

func TestMetricsAccessPolicyIsManagedSeparatelyAndEndpointRemainsOptIn(t *testing.T) {
	cfg := defaultConfig()
	applyConfigDefaults(&cfg)
	cfg.Auth.AdminPassword = "test-admin-password"
	cfg.Security.EncryptionKey = "test-encryption-key"
	if cfg.Metrics.Enabled {
		t.Fatal("metrics must default to disabled")
	}
	if err := validateConfig(cfg); err != nil {
		t.Fatal(err)
	}
	cfg.Metrics.Enabled = true
	if err := validateConfig(cfg); err != nil {
		t.Fatalf("local metrics: %v", err)
	}
	cfg.Environment = "production"
	if err := validateConfig(cfg); err != nil {
		t.Fatalf("empty token must not prevent startup before a persisted anonymous-access policy can be loaded: %v", err)
	}
	cfg.Metrics.BearerToken = "test-metrics-token"
	if err := validateConfig(cfg); err != nil {
		t.Fatal(err)
	}
	cfg.Metrics.CollectionTimeoutSec = cfg.Metrics.RefreshIntervalSec + 1
	if err := validateConfig(cfg); err == nil {
		t.Fatal("expected bounded collection timeout")
	}
}
