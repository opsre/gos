package k8sinstance

import "context"

type ListFilter struct {
	EnvironmentCode string
	Code            string
	Page            int
	PageSize        int
}

type UpdateInput struct {
	Code                   string
	ClusterName            string
	EnvironmentCode        string
	APIServer              string
	DefaultNamespace       string
	ArgoCDInstanceID       string
	SupportsNativeStrategy *bool
	SupportsRollouts       *bool
	TrafficProvider        string
}

type Repository interface {
	InitSchema(ctx context.Context) error
	Create(ctx context.Context, item K8sClusterRef) error
	GetByID(ctx context.Context, id string) (K8sClusterRef, error)
	GetByCode(ctx context.Context, code string) (K8sClusterRef, error)
	List(ctx context.Context, filter ListFilter) ([]K8sClusterRef, int64, error)
	Update(ctx context.Context, id string, input UpdateInput) (K8sClusterRef, error)
	Delete(ctx context.Context, id string) error
}
