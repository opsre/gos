package k8sinstance

import "errors"

var (
	ErrNotFound      = errors.New("k8s cluster ref not found")
	ErrCodeDuplicated = errors.New("k8s cluster ref code already exists")
	ErrInUse          = errors.New("k8s cluster ref is referenced by runtime bindings")
)
