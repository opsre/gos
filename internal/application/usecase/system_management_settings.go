package usecase

import (
	"context"
	"fmt"
	"net/url"
	"strings"
)

type SystemManagementSettingsStore interface {
	LoadCurrentSiteURL(ctx context.Context) (string, error)
	SaveCurrentSiteURL(ctx context.Context, value string) error
}

type SystemManagementSettingsOutput struct {
	CurrentSiteURL string `json:"current_site_url"`
}

type QuerySystemManagementSettings struct {
	store SystemManagementSettingsStore
}

// NewQuerySystemManagementSettings 创建并返回系统管理设置查询组件。
func NewQuerySystemManagementSettings(store SystemManagementSettingsStore) *QuerySystemManagementSettings {
	return &QuerySystemManagementSettings{store: store}
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
	return SystemManagementSettingsOutput{
		CurrentSiteURL: strings.TrimSpace(currentSiteURL),
	}, nil
}

type UpdateSystemManagementSettingsInput struct {
	CurrentSiteURL string
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
	if err := uc.store.SaveCurrentSiteURL(ctx, currentSiteURL); err != nil {
		return SystemManagementSettingsOutput{}, err
	}
	return uc.reader.Execute(ctx)
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
