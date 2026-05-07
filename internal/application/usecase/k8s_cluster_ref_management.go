package usecase

import (
	"context"
	"fmt"
	"strings"
	"time"

	"gos/internal/domain/k8sinstance"
)

type K8sClusterRefManager struct {
	repo k8sinstance.Repository
	now  func() time.Time
}

func NewK8sClusterRefManager(repo k8sinstance.Repository) *K8sClusterRefManager {
	return &K8sClusterRefManager{repo: repo, now: time.Now}
}

type CreateK8sClusterRefInput struct {
	Code                   string                `json:"code"`
	ClusterName            string                `json:"cluster_name"`
	EnvironmentCode        string                `json:"environment_code"`
	APIServer              string                `json:"api_server"`
	DefaultNamespace       string                `json:"default_namespace"`
	ArgoCDInstanceID       string                `json:"argocd_instance_id"`
	SupportsNativeStrategy bool                  `json:"supports_native_strategy"`
	SupportsRollouts       bool                  `json:"supports_rollouts"`
	TrafficProvider        string                `json:"traffic_provider"`
}

func (m *K8sClusterRefManager) Create(ctx context.Context, input CreateK8sClusterRefInput) (k8sinstance.K8sClusterRef, error) {
	input.Code = strings.TrimSpace(input.Code)
	if input.Code == "" {
		return k8sinstance.K8sClusterRef{}, fmt.Errorf("%w: code is required", ErrInvalidInput)
	}
	input.APIServer = strings.TrimSpace(input.APIServer)
	if input.APIServer == "" {
		return k8sinstance.K8sClusterRef{}, fmt.Errorf("%w: api_server is required", ErrInvalidInput)
	}

	now := m.now().UTC()
	defaultNS := strings.TrimSpace(input.DefaultNamespace)
	if defaultNS == "" {
		defaultNS = "default"
	}

	item := k8sinstance.K8sClusterRef{
		ID:                     generateID("kcr"),
		Code:                   input.Code,
		ClusterName:            strings.TrimSpace(input.ClusterName),
		EnvironmentCode:        strings.TrimSpace(input.EnvironmentCode),
		APIServer:              input.APIServer,
		DefaultNamespace:       defaultNS,
		AccessMode:             k8sinstance.AccessModeArgoCD,
		ArgoCDInstanceID:       strings.TrimSpace(input.ArgoCDInstanceID),
		SupportsNativeStrategy: input.SupportsNativeStrategy,
		SupportsRollouts:       input.SupportsRollouts,
		TrafficProvider:        strings.TrimSpace(input.TrafficProvider),
		CreatedAt:              now,
		UpdatedAt:              now,
	}
	item = item.Clean()

	if err := m.repo.Create(ctx, item); err != nil {
		return k8sinstance.K8sClusterRef{}, err
	}
	return item, nil
}

func (m *K8sClusterRefManager) GetByID(ctx context.Context, id string) (k8sinstance.K8sClusterRef, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return k8sinstance.K8sClusterRef{}, fmt.Errorf("%w: id is required", ErrInvalidInput)
	}
	return m.repo.GetByID(ctx, id)
}

func (m *K8sClusterRefManager) List(ctx context.Context, filter k8sinstance.ListFilter) ([]k8sinstance.K8sClusterRef, int64, error) {
	if filter.Page <= 0 {
		filter.Page = 1
	}
	if filter.PageSize <= 0 || filter.PageSize > 100 {
		filter.PageSize = 20
	}
	return m.repo.List(ctx, filter)
}

type UpdateK8sClusterRefInput struct {
	Code                   *string `json:"code"`
	ClusterName            *string `json:"cluster_name"`
	EnvironmentCode        *string `json:"environment_code"`
	APIServer              *string `json:"api_server"`
	DefaultNamespace       *string `json:"default_namespace"`
	ArgoCDInstanceID       *string `json:"argocd_instance_id"`
	SupportsNativeStrategy *bool   `json:"supports_native_strategy"`
	SupportsRollouts       *bool   `json:"supports_rollouts"`
	TrafficProvider        *string `json:"traffic_provider"`
}

func (m *K8sClusterRefManager) Update(ctx context.Context, id string, input UpdateK8sClusterRefInput) (k8sinstance.K8sClusterRef, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return k8sinstance.K8sClusterRef{}, fmt.Errorf("%w: id is required", ErrInvalidInput)
	}

	existing, err := m.repo.GetByID(ctx, id)
	if err != nil {
		return k8sinstance.K8sClusterRef{}, err
	}

	updateInput := k8sinstance.UpdateInput{
		Code:                   existing.Code,
		ClusterName:            existing.ClusterName,
		EnvironmentCode:        existing.EnvironmentCode,
		APIServer:              existing.APIServer,
		DefaultNamespace:       existing.DefaultNamespace,
		ArgoCDInstanceID:       existing.ArgoCDInstanceID,
		TrafficProvider:        existing.TrafficProvider,
		SupportsNativeStrategy: &existing.SupportsNativeStrategy,
		SupportsRollouts:       &existing.SupportsRollouts,
	}

	if input.Code != nil {
		code := strings.TrimSpace(*input.Code)
		if code == "" {
			return k8sinstance.K8sClusterRef{}, fmt.Errorf("%w: code is required", ErrInvalidInput)
		}
		updateInput.Code = code
	}
	if input.ClusterName != nil {
		updateInput.ClusterName = strings.TrimSpace(*input.ClusterName)
	}
	if input.EnvironmentCode != nil {
		updateInput.EnvironmentCode = strings.TrimSpace(*input.EnvironmentCode)
	}
	if input.APIServer != nil {
		updateInput.APIServer = strings.TrimSpace(*input.APIServer)
	}
	if input.DefaultNamespace != nil {
		updateInput.DefaultNamespace = strings.TrimSpace(*input.DefaultNamespace)
	}
	if input.ArgoCDInstanceID != nil {
		updateInput.ArgoCDInstanceID = strings.TrimSpace(*input.ArgoCDInstanceID)
	}
	if input.SupportsNativeStrategy != nil {
		updateInput.SupportsNativeStrategy = input.SupportsNativeStrategy
	}
	if input.SupportsRollouts != nil {
		updateInput.SupportsRollouts = input.SupportsRollouts
	}
	if input.TrafficProvider != nil {
		updateInput.TrafficProvider = strings.TrimSpace(*input.TrafficProvider)
	}

	return m.repo.Update(ctx, id, updateInput)
}

func (m *K8sClusterRefManager) Delete(ctx context.Context, id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return fmt.Errorf("%w: id is required", ErrInvalidInput)
	}
	return m.repo.Delete(ctx, id)
}
