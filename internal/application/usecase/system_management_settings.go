package usecase

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type SystemManagementSettingsStore interface {
	LoadCurrentSiteURL(ctx context.Context) (string, error)
	LoadMetricsAuthEnabled(ctx context.Context) (*bool, error)
	SaveSystemManagementSettings(ctx context.Context, currentSiteURL string, metricsAuthEnabled *bool) error
}

type SystemManagementSettingsOutput struct {
	CurrentSiteURL         string `json:"current_site_url"`
	MetricsAuthEnabled     bool   `json:"metrics_auth_enabled"`
	MetricsEnabled         bool   `json:"metrics_enabled"`
	MetricsTokenConfigured bool   `json:"metrics_token_configured"`
}

type SystemManagementMetricsOptions struct {
	Enabled               bool
	TokenConfigured       bool
	AuthRequiredByDefault bool
}

type QuerySystemManagementSettings struct {
	store              SystemManagementSettingsStore
	metrics            SystemManagementMetricsOptions
	metricsMu          sync.Mutex
	metricsAuthEnabled atomic.Bool
}

// NewQuerySystemManagementSettings 创建并返回系统管理设置查询组件。
func NewQuerySystemManagementSettings(store SystemManagementSettingsStore, options ...SystemManagementMetricsOptions) *QuerySystemManagementSettings {
	uc := &QuerySystemManagementSettings{store: store}
	if len(options) > 0 {
		uc.metrics = options[0]
	}
	uc.metricsAuthEnabled.Store(uc.metrics.AuthRequiredByDefault)
	return uc
}

// MetricsAuthRequired reads the active policy without database I/O on scrapes.
func (uc *QuerySystemManagementSettings) MetricsAuthRequired() bool {
	return uc.metricsAuthEnabled.Load()
}

func (uc *QuerySystemManagementSettings) loadMetricsAuthEnabled(ctx context.Context) (bool, error) {
	stored, err := uc.store.LoadMetricsAuthEnabled(ctx)
	if err != nil {
		return false, err
	}
	if stored == nil {
		return uc.metrics.AuthRequiredByDefault, nil
	}
	return *stored, nil
}

// RefreshMetricsAuthPolicy loads persisted changes, including those made by
// another server replica. Failed reads retain the last successfully loaded policy.
func (uc *QuerySystemManagementSettings) RefreshMetricsAuthPolicy(ctx context.Context) error {
	uc.metricsMu.Lock()
	defer uc.metricsMu.Unlock()
	enabled, err := uc.loadMetricsAuthEnabled(ctx)
	if err != nil {
		return err
	}
	uc.metricsAuthEnabled.Store(enabled)
	return nil
}

func (uc *QuerySystemManagementSettings) RunMetricsAuthPolicy(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			refreshCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
			_ = uc.RefreshMetricsAuthPolicy(refreshCtx)
			cancel()
		}
	}
}

func (uc *QuerySystemManagementSettings) output(currentSiteURL string) SystemManagementSettingsOutput {
	return SystemManagementSettingsOutput{
		CurrentSiteURL: strings.TrimSpace(currentSiteURL), MetricsAuthEnabled: uc.MetricsAuthRequired(),
		MetricsEnabled: uc.metrics.Enabled, MetricsTokenConfigured: uc.metrics.TokenConfigured,
	}
}

// Execute 查询系统管理设置。
func (uc *QuerySystemManagementSettings) Execute(ctx context.Context) (SystemManagementSettingsOutput, error) {
	if uc == nil || uc.store == nil {
		return SystemManagementSettingsOutput{}, fmt.Errorf("%w: system management settings are not configured", ErrInvalidInput)
	}
	currentSiteURL, err := uc.store.LoadCurrentSiteURL(ctx)
	if err != nil {
		return SystemManagementSettingsOutput{}, err
	}
	if err := uc.RefreshMetricsAuthPolicy(ctx); err != nil {
		return SystemManagementSettingsOutput{}, err
	}
	return uc.output(currentSiteURL), nil
}

type UpdateSystemManagementSettingsInput struct {
	CurrentSiteURL     string
	MetricsAuthEnabled *bool
}

type UpdateSystemManagementSettings struct {
	store  SystemManagementSettingsStore
	reader *QuerySystemManagementSettings
}

// NewUpdateSystemManagementSettings 创建并返回系统管理设置更新组件。
func NewUpdateSystemManagementSettings(
	store SystemManagementSettingsStore,
	reader *QuerySystemManagementSettings,
) *UpdateSystemManagementSettings {
	return &UpdateSystemManagementSettings{store: store, reader: reader}
}

// Execute 更新系统管理设置。
func (uc *UpdateSystemManagementSettings) Execute(
	ctx context.Context,
	input UpdateSystemManagementSettingsInput,
) (SystemManagementSettingsOutput, error) {
	if uc == nil || uc.store == nil || uc.reader == nil {
		return SystemManagementSettingsOutput{}, fmt.Errorf("%w: system management settings are not configured", ErrInvalidInput)
	}
	currentSiteURL, err := normalizeCurrentSiteURL(input.CurrentSiteURL)
	if err != nil {
		return SystemManagementSettingsOutput{}, err
	}
	uc.reader.metricsMu.Lock()
	defer uc.reader.metricsMu.Unlock()
	currentAuth, err := uc.reader.loadMetricsAuthEnabled(ctx)
	if err != nil {
		return SystemManagementSettingsOutput{}, err
	}
	if input.MetricsAuthEnabled != nil && *input.MetricsAuthEnabled && !currentAuth && !uc.reader.metrics.TokenConfigured {
		return SystemManagementSettingsOutput{}, fmt.Errorf("%w: 开启指标访问鉴权前，请先配置采集 Token", ErrInvalidInput)
	}
	if err := uc.store.SaveSystemManagementSettings(ctx, currentSiteURL, input.MetricsAuthEnabled); err != nil {
		return SystemManagementSettingsOutput{}, err
	}
	if input.MetricsAuthEnabled != nil {
		currentAuth = *input.MetricsAuthEnabled
	}
	uc.reader.metricsAuthEnabled.Store(currentAuth)
	return uc.reader.output(currentSiteURL), nil
}

func normalizeCurrentSiteURL(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", nil
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return "", fmt.Errorf("%w: 当前站点 URL 必须是有效的 HTTP 或 HTTPS 地址", ErrInvalidInput)
	}
	if parsed.User != nil {
		return "", fmt.Errorf("%w: 当前站点 URL 不能包含用户凭据", ErrInvalidInput)
	}
	return strings.TrimRight(value, "/"), nil
}
