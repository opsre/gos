package k8sinstance

import (
	"strings"
	"time"
)

type AccessMode string

const (
	AccessModeArgoCD AccessMode = "argocd"
)

func (a AccessMode) Valid() bool {
	switch a {
	case AccessModeArgoCD:
		return true
	default:
		return false
	}
}

type K8sClusterRef struct {
	ID                     string
	Code                   string
	ClusterName            string
	EnvironmentCode        string
	APIServer              string
	DefaultNamespace       string
	AccessMode             AccessMode
	ArgoCDInstanceID       string
	SupportsNativeStrategy bool
	SupportsRollouts       bool
	TrafficProvider        string
	CreatedAt              time.Time
	UpdatedAt              time.Time
}

func (r K8sClusterRef) Clean() K8sClusterRef {
	r.Code = strings.TrimSpace(r.Code)
	r.ClusterName = strings.TrimSpace(r.ClusterName)
	r.EnvironmentCode = strings.TrimSpace(r.EnvironmentCode)
	r.APIServer = strings.TrimSpace(r.APIServer)
	r.DefaultNamespace = strings.TrimSpace(r.DefaultNamespace)
	r.ArgoCDInstanceID = strings.TrimSpace(r.ArgoCDInstanceID)
	r.TrafficProvider = strings.TrimSpace(r.TrafficProvider)
	if r.AccessMode == "" {
		r.AccessMode = AccessModeArgoCD
	}
	return r
}
