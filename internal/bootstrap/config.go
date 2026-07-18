package bootstrap

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
)

type Config struct {
	Environment string         `json:"environment"`
	Server      ServerConfig   `json:"server"`
	Database    DatabaseConfig `json:"database"`
	Jenkins     JenkinsConfig  `json:"jenkins"`
	ArgoCD      ArgoCDConfig   `json:"argocd"`
	GitOps      GitOpsConfig   `json:"gitops"`
	Release     ReleaseConfig  `json:"release"`
	Auth        AuthConfig     `json:"auth"`
	Security    SecurityConfig `json:"security"`
}

type ServerConfig struct {
	Addr                 string `json:"addr"`
	ReadHeaderTimeoutSec int    `json:"read_header_timeout_sec"`
	ReadTimeoutSec       int    `json:"read_timeout_sec"`
	WriteTimeoutSec      int    `json:"write_timeout_sec"`
	IdleTimeoutSec       int    `json:"idle_timeout_sec"`
	ShutdownTimeoutSec   int    `json:"shutdown_timeout_sec"`
}

type DatabaseConfig struct {
	Driver                  string `json:"driver"`
	MySQLDSN                string `json:"mysql_dsn"`
	SQLitePath              string `json:"sqlite_path"`
	MaxOpenConns            int    `json:"max_open_conns"`
	MaxIdleConns            int    `json:"max_idle_conns"`
	ConnMaxLifetimeSec      int    `json:"conn_max_lifetime_sec"`
	ConnMaxIdleTimeSec      int    `json:"conn_max_idle_time_sec"`
	PingTimeoutSec          int    `json:"ping_timeout_sec"`
	StartupMaxRetries       int    `json:"startup_max_retries"`
	StartupRetryIntervalSec int    `json:"startup_retry_interval_sec"`
}

type JenkinsConfig struct {
	Enabled                 bool   `json:"enabled"`
	BaseURL                 string `json:"base_url"`
	Username                string `json:"username"`
	APIToken                string `json:"api_token"`
	TimeoutSec              int    `json:"timeout_sec"`
	StartupCheckEnabled     bool   `json:"startup_check_enabled"`
	StartupMaxRetries       int    `json:"startup_max_retries"`
	StartupRetryIntervalSec int    `json:"startup_retry_interval_sec"`
	AutoSyncEnabled         bool   `json:"auto_sync_enabled"`
	AutoSyncIntervalSec     int    `json:"auto_sync_interval_sec"`
	ReleaseTrackEnabled     bool   `json:"release_track_enabled"`
	ReleaseTrackIntervalSec int    `json:"release_track_interval_sec"`
}

type ArgoCDConfig struct {
	Enabled             bool   `json:"enabled"`
	BaseURL             string `json:"base_url"`
	InsecureSkipVerify  bool   `json:"insecure_skip_verify"`
	AuthMode            string `json:"auth_mode"`
	Token               string `json:"token"`
	Username            string `json:"username"`
	Password            string `json:"password"`
	TimeoutSec          int    `json:"request_timeout_sec"`
	StartupCheckEnabled bool   `json:"startup_check_enabled"`
	SyncEnabled         bool   `json:"sync_enabled"`
	SyncIntervalSec     int    `json:"sync_interval_sec"`
}

// GitOpsConfig 描述平台在 ArgoCD CD 模式下操作声明式仓库所需的最小配置。
//
// 设计目标是让 gos 只承担“受控修改 + 提交推送”的职责：
// 1. local_root 是本地工作目录根路径，平台会在其中维护 Git 仓库副本；
// 2. default_branch 表示默认提交分支，若 ArgoCD Application 使用 HEAD，则回退到这里；
// 3. username/password 或 token 用于 clone / pull / push；
// 4. author_name / author_email 作为平台提交 Git 变更时的固定身份；
// 5. commit_message_template 用于统一 GitOps 提交信息模版格式，便于审计和排查。
type GitOpsConfig struct {
	Enabled               bool   `json:"enabled"`
	LocalRoot             string `json:"local_root"`
	DefaultBranch         string `json:"default_branch"`
	Username              string `json:"username"`
	Password              string `json:"password"`
	Token                 string `json:"token"`
	AuthorName            string `json:"author_name"`
	AuthorEmail           string `json:"author_email"`
	CommitMessageTemplate string `json:"commit_message_template"`
	CommandTimeoutSec     int    `json:"command_timeout_sec"`
}

type ReleaseConfig struct {
	EnvOptions     []string                   `json:"env_options"`
	EnvConfigs     []ReleaseEnvironmentConfig `json:"env_configs"`
	DefaultEnvCode string                     `json:"default_env_code"`
	Concurrency    ReleaseConcurrencyConfig   `json:"concurrency"`
	GitOpsConfig   ReleaseGitOpsConfig        `json:"gitops_config"`
}

type ReleaseEnvironmentConfig struct {
	Code        string `json:"code"`
	Description string `json:"description"`
}

type ReleaseGitOpsConfig struct {
	HelmScanPath      string `json:"helm_scan_path"`
	KustomizeScanPath string `json:"kustomize_scan_path"`
}

type ReleaseConcurrencyConfig struct {
	Enabled          bool   `json:"enabled"`
	LockScope        string `json:"lock_scope"`
	ConflictStrategy string `json:"conflict_strategy"`
	LockTimeoutSec   int    `json:"lock_timeout_sec"`
}

type AuthConfig struct {
	SessionTTLHours  int    `json:"session_ttl_hours"`
	AdminUsername    string `json:"admin_username"`
	AdminDisplayName string `json:"admin_display_name"`
	AdminPassword    string `json:"admin_password"`
}

type SecurityConfig struct {
	EncryptionKey string `json:"encryption_key"`
}

// LoadConfig 封装当前模块的业务处理逻辑。
func LoadConfig() (Config, error) {
	return LoadConfigFromPath("")
}

// LoadConfigFromPath 封装当前模块的业务处理逻辑。
func LoadConfigFromPath(path string) (Config, error) {
	cfg := defaultConfig()

	configPath := ResolveConfigPath(path)

	if err := loadFromFile(configPath, &cfg); err != nil {
		return Config{}, err
	}
	applyConfigDefaults(&cfg)

	if err := validateConfig(cfg); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// ResolveConfigPath 解析上下文数据，得到后续流程需要的结果。
func ResolveConfigPath(path string) string {
	configPath := strings.TrimSpace(path)
	if configPath == "" {
		configPath = "configs/config.local.json"
	}
	return configPath
}

// defaultConfig 封装当前模块的业务处理逻辑。
func defaultConfig() Config {
	return Config{
		Environment: "local",
		Server: ServerConfig{
			Addr:                 ":8080",
			ReadHeaderTimeoutSec: 5,
			ReadTimeoutSec:       120,
			WriteTimeoutSec:      120,
			IdleTimeoutSec:       60,
			ShutdownTimeoutSec:   10,
		},
		Database: DatabaseConfig{
			Driver:                  "sqlite",
			MySQLDSN:                "",
			SQLitePath:              "data/demo.db",
			MaxOpenConns:            10,
			MaxIdleConns:            5,
			ConnMaxLifetimeSec:      1800,
			ConnMaxIdleTimeSec:      600,
			PingTimeoutSec:          5,
			StartupMaxRetries:       10,
			StartupRetryIntervalSec: 2,
		},
		Jenkins: JenkinsConfig{
			Enabled:                 false,
			BaseURL:                 "http://127.0.0.1:8080",
			Username:                "admin",
			APIToken:                "",
			TimeoutSec:              60,
			StartupCheckEnabled:     false,
			StartupMaxRetries:       5,
			StartupRetryIntervalSec: 2,
			AutoSyncEnabled:         false,
			AutoSyncIntervalSec:     300,
			ReleaseTrackEnabled:     true,
			ReleaseTrackIntervalSec: 10,
		},
		ArgoCD: ArgoCDConfig{
			Enabled:             false,
			BaseURL:             "https://127.0.0.1:30443",
			InsecureSkipVerify:  true,
			AuthMode:            "token",
			Token:               "",
			Username:            "admin",
			Password:            "",
			TimeoutSec:          30,
			StartupCheckEnabled: false,
			SyncEnabled:         false,
			SyncIntervalSec:     300,
		},
		GitOps: GitOpsConfig{
			Enabled:               false,
			LocalRoot:             "data/gitops",
			DefaultBranch:         "master",
			AuthorName:            "gos-bot",
			AuthorEmail:           "gos@example.com",
			CommitMessageTemplate: "chore(release): {app_key}/{project_name}/{env} -> {image_version} ({branch})",
			CommandTimeoutSec:     30,
		},
		Release: ReleaseConfig{
			EnvOptions: []string{"dev", "test", "prod"},
			Concurrency: ReleaseConcurrencyConfig{
				Enabled:          false,
				LockScope:        "application_env",
				ConflictStrategy: "reject",
				LockTimeoutSec:   1800,
			},
		},
		Auth: AuthConfig{
			SessionTTLHours:  24,
			AdminUsername:    "admin",
			AdminDisplayName: "Administrator",
			AdminPassword:    "",
		},
		Security: SecurityConfig{},
	}
}

// loadFromFile 封装当前模块的业务处理逻辑。
func loadFromFile(path string, cfg *Config) error {
	file, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("open config file %q: %w", path, err)
	}
	defer func() { _ = file.Close() }()

	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(cfg); err != nil {
		return fmt.Errorf("decode config file %q: %w", path, err)
	}
	return nil
}

// applyConfigDefaults 封装当前模块的业务处理逻辑。
func applyConfigDefaults(cfg *Config) {
	cfg.Environment = strings.TrimSpace(cfg.Environment)
	if cfg.Environment == "" {
		cfg.Environment = "local"
	}

	cfg.Server.Addr = strings.TrimSpace(cfg.Server.Addr)
	if cfg.Server.Addr == "" {
		cfg.Server.Addr = ":8080"
	}
	if cfg.Server.ReadHeaderTimeoutSec <= 0 {
		cfg.Server.ReadHeaderTimeoutSec = 5
	}
	if cfg.Server.ReadTimeoutSec <= 0 {
		cfg.Server.ReadTimeoutSec = 15
	}
	if cfg.Server.WriteTimeoutSec <= 0 {
		cfg.Server.WriteTimeoutSec = 15
	}
	if cfg.Server.IdleTimeoutSec <= 0 {
		cfg.Server.IdleTimeoutSec = 60
	}
	if cfg.Server.ShutdownTimeoutSec <= 0 {
		cfg.Server.ShutdownTimeoutSec = 10
	}

	cfg.Release.Concurrency.LockScope = strings.TrimSpace(cfg.Release.Concurrency.LockScope)
	if cfg.Release.Concurrency.LockScope == "" {
		cfg.Release.Concurrency.LockScope = "application_env"
	}
	cfg.Release.Concurrency.ConflictStrategy = strings.TrimSpace(cfg.Release.Concurrency.ConflictStrategy)
	if cfg.Release.Concurrency.ConflictStrategy == "" {
		cfg.Release.Concurrency.ConflictStrategy = "reject"
	}
	if cfg.Release.Concurrency.LockTimeoutSec <= 0 {
		cfg.Release.Concurrency.LockTimeoutSec = 1800
	}
	if cfg.Release.Concurrency.LockTimeoutSec < 30 {
		cfg.Release.Concurrency.LockTimeoutSec = 30
	}
	if cfg.Release.Concurrency.LockTimeoutSec > 86400 {
		cfg.Release.Concurrency.LockTimeoutSec = 86400
	}

	cfg.Database.Driver = strings.ToLower(strings.TrimSpace(cfg.Database.Driver))
	if cfg.Database.Driver == "" {
		cfg.Database.Driver = "mysql"
	}
	cfg.Database.MySQLDSN = strings.TrimSpace(cfg.Database.MySQLDSN)
	cfg.Database.SQLitePath = strings.TrimSpace(cfg.Database.SQLitePath)
	if cfg.Database.MaxOpenConns <= 0 {
		cfg.Database.MaxOpenConns = 10
	}
	if cfg.Database.MaxIdleConns <= 0 {
		cfg.Database.MaxIdleConns = 5
	}
	if cfg.Database.ConnMaxLifetimeSec <= 0 {
		cfg.Database.ConnMaxLifetimeSec = 1800
	}
	if cfg.Database.ConnMaxIdleTimeSec <= 0 {
		cfg.Database.ConnMaxIdleTimeSec = 600
	}
	if cfg.Database.PingTimeoutSec <= 0 {
		cfg.Database.PingTimeoutSec = 5
	}
	if cfg.Database.StartupMaxRetries <= 0 {
		cfg.Database.StartupMaxRetries = 10
	}
	if cfg.Database.StartupRetryIntervalSec <= 0 {
		cfg.Database.StartupRetryIntervalSec = 2
	}

	cfg.Jenkins.BaseURL = strings.TrimSpace(cfg.Jenkins.BaseURL)
	cfg.Jenkins.Username = strings.TrimSpace(cfg.Jenkins.Username)
	cfg.Jenkins.APIToken = strings.TrimSpace(cfg.Jenkins.APIToken)
	if cfg.Jenkins.TimeoutSec <= 0 {
		cfg.Jenkins.TimeoutSec = 60
	}
	if cfg.Jenkins.StartupMaxRetries <= 0 {
		cfg.Jenkins.StartupMaxRetries = 5
	}
	if cfg.Jenkins.StartupRetryIntervalSec <= 0 {
		cfg.Jenkins.StartupRetryIntervalSec = 2
	}
	if cfg.Jenkins.AutoSyncIntervalSec <= 0 {
		cfg.Jenkins.AutoSyncIntervalSec = 300
	}
	if cfg.Jenkins.ReleaseTrackIntervalSec <= 0 {
		cfg.Jenkins.ReleaseTrackIntervalSec = 10
	}

	cfg.ArgoCD.BaseURL = strings.TrimSpace(cfg.ArgoCD.BaseURL)
	cfg.ArgoCD.AuthMode = strings.ToLower(strings.TrimSpace(cfg.ArgoCD.AuthMode))
	cfg.ArgoCD.Token = strings.TrimSpace(cfg.ArgoCD.Token)
	cfg.ArgoCD.Username = strings.TrimSpace(cfg.ArgoCD.Username)
	cfg.ArgoCD.Password = strings.TrimSpace(cfg.ArgoCD.Password)
	if cfg.ArgoCD.AuthMode == "" {
		cfg.ArgoCD.AuthMode = "token"
	}
	if cfg.ArgoCD.TimeoutSec <= 0 {
		cfg.ArgoCD.TimeoutSec = 30
	}
	if cfg.ArgoCD.SyncIntervalSec <= 0 {
		cfg.ArgoCD.SyncIntervalSec = 300
	}

	cfg.GitOps.LocalRoot = strings.TrimSpace(cfg.GitOps.LocalRoot)
	cfg.GitOps.DefaultBranch = strings.TrimSpace(cfg.GitOps.DefaultBranch)
	cfg.GitOps.Username = strings.TrimSpace(cfg.GitOps.Username)
	cfg.GitOps.Password = strings.TrimSpace(cfg.GitOps.Password)
	cfg.GitOps.Token = strings.TrimSpace(cfg.GitOps.Token)
	cfg.GitOps.AuthorName = strings.TrimSpace(cfg.GitOps.AuthorName)
	cfg.GitOps.AuthorEmail = strings.TrimSpace(cfg.GitOps.AuthorEmail)
	cfg.GitOps.CommitMessageTemplate = strings.TrimSpace(cfg.GitOps.CommitMessageTemplate)
	if cfg.GitOps.LocalRoot == "" {
		cfg.GitOps.LocalRoot = "data/gitops"
	}
	if cfg.GitOps.DefaultBranch == "" {
		cfg.GitOps.DefaultBranch = "master"
	}
	if cfg.GitOps.AuthorName == "" {
		cfg.GitOps.AuthorName = "gos-bot"
	}
	if cfg.GitOps.AuthorEmail == "" {
		cfg.GitOps.AuthorEmail = "gos@example.com"
	}
	if cfg.GitOps.CommitMessageTemplate == "" {
		cfg.GitOps.CommitMessageTemplate = "chore(release): {app_key}/{project_name}/{env} -> {image_version} ({branch})"
	}
	if cfg.GitOps.CommandTimeoutSec <= 0 {
		cfg.GitOps.CommandTimeoutSec = 30
	}

	cfg.Release.EnvOptions = normalizeStringList(cfg.Release.EnvOptions)
	cfg.Release.EnvConfigs = normalizeReleaseEnvironmentConfigs(cfg.Release.EnvConfigs)
	cfg.Release.DefaultEnvCode = strings.TrimSpace(cfg.Release.DefaultEnvCode)
	if len(cfg.Release.EnvOptions) == 0 {
		cfg.Release.EnvOptions = []string{"dev", "test", "prod"}
	}

	if cfg.Auth.SessionTTLHours <= 0 {
		cfg.Auth.SessionTTLHours = 24
	}
	cfg.Auth.AdminUsername = strings.TrimSpace(cfg.Auth.AdminUsername)
	cfg.Auth.AdminDisplayName = strings.TrimSpace(cfg.Auth.AdminDisplayName)
	cfg.Auth.AdminPassword = strings.TrimSpace(cfg.Auth.AdminPassword)
	if cfg.Auth.AdminUsername == "" {
		cfg.Auth.AdminUsername = "admin"
	}
	if cfg.Auth.AdminDisplayName == "" {
		cfg.Auth.AdminDisplayName = "Administrator"
	}
	cfg.Security.EncryptionKey = strings.TrimSpace(cfg.Security.EncryptionKey)
	if cfg.Security.EncryptionKey == "" && strings.EqualFold(cfg.Environment, "local") {
		cfg.Security.EncryptionKey = "gos-local-dev-encryption-key"
	}
}

// validateConfig 封装当前模块的业务处理逻辑。
func validateConfig(cfg Config) error {
	switch cfg.Database.Driver {
	case "sqlite":
		if cfg.Database.SQLitePath == "" {
			return errors.New("database.sqlite_path is required when database.driver=sqlite")
		}
	case "mysql":
		if cfg.Database.MySQLDSN == "" {
			return errors.New("database.mysql_dsn is required when database.driver=mysql")
		}
	default:
		return fmt.Errorf("unsupported database.driver %q", cfg.Database.Driver)
	}

	if cfg.Jenkins.Enabled {
		if cfg.Jenkins.BaseURL == "" {
			return errors.New("jenkins.base_url is required when jenkins.enabled=true")
		}
		if (cfg.Jenkins.Username == "") != (cfg.Jenkins.APIToken == "") {
			return errors.New("jenkins.username and jenkins.api_token must be set together")
		}
	}
	if cfg.ArgoCD.Enabled {
		if cfg.ArgoCD.BaseURL != "" {
			switch cfg.ArgoCD.AuthMode {
			case "token":
				if cfg.ArgoCD.Token == "" {
					return errors.New("argocd.token is required when argocd.auth_mode=token")
				}
			case "password", "basic", "session":
				if cfg.ArgoCD.Username == "" || cfg.ArgoCD.Password == "" {
					return fmt.Errorf("argocd.username and argocd.password are required when argocd.auth_mode=%s", cfg.ArgoCD.AuthMode)
				}
			default:
				return fmt.Errorf("unsupported argocd.auth_mode %q", cfg.ArgoCD.AuthMode)
			}
		}
	}
	if cfg.GitOps.Enabled {
		if cfg.GitOps.LocalRoot == "" {
			return errors.New("gitops.local_root is required when gitops.enabled=true")
		}
		if cfg.GitOps.DefaultBranch == "" {
			return errors.New("gitops.default_branch is required when gitops.enabled=true")
		}
		if cfg.GitOps.AuthorName == "" || cfg.GitOps.AuthorEmail == "" {
			return errors.New("gitops.author_name and gitops.author_email are required when gitops.enabled=true")
		}
	}

	switch cfg.Release.Concurrency.LockScope {
	case "application", "application_env", "gitops_repo_branch":
	default:
		return fmt.Errorf("unsupported release.concurrency.lock_scope %q", cfg.Release.Concurrency.LockScope)
	}
	switch cfg.Release.Concurrency.ConflictStrategy {
	case "reject", "queue":
	default:
		return fmt.Errorf("unsupported release.concurrency.conflict_strategy %q", cfg.Release.Concurrency.ConflictStrategy)
	}

	if cfg.Auth.AdminPassword == "" {
		return errors.New("auth.admin_password is required")
	}
	if cfg.Security.EncryptionKey == "" {
		return errors.New("security.encryption_key is required")
	}

	return nil
}

// normalizeStringList 标准化输入值，保证后续逻辑使用统一格式。
func normalizeStringList(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	result := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, item := range values {
		value := strings.TrimSpace(item)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

func normalizeReleaseEnvironmentConfigs(values []ReleaseEnvironmentConfig) []ReleaseEnvironmentConfig {
	if len(values) == 0 {
		return nil
	}
	result := make([]ReleaseEnvironmentConfig, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, item := range values {
		code := strings.TrimSpace(item.Code)
		if code == "" {
			continue
		}
		if _, exists := seen[code]; exists {
			continue
		}
		seen[code] = struct{}{}
		result = append(result, ReleaseEnvironmentConfig{
			Code:        code,
			Description: strings.TrimSpace(item.Description),
		})
	}
	if len(result) == 0 {
		return nil
	}
	return result
}
