package usecase

import "errors"

var (
	ErrInvalidInput             = errors.New("invalid input")
	ErrInvalidID                = errors.New("invalid id")
	ErrInvalidStatus            = errors.New("invalid status")
	ErrInvalidProvider          = errors.New("invalid provider")
	ErrInvalidBindingType       = errors.New("invalid binding type")
	ErrInvalidTriggerMode       = errors.New("invalid trigger mode")
	ErrInvalidParamKey          = errors.New("invalid param key")
	ErrInvalidParamType         = errors.New("invalid param type")
	ErrInvalidExecutorType      = errors.New("invalid executor type")
	ErrInvalidSourceFrom        = errors.New("invalid source from")
	ErrBuiltinProtected         = errors.New("builtin record cannot be deleted")
	ErrReferencedConflict       = errors.New("record is referenced and cannot be changed")
	ErrConcurrentReleaseBlocked = errors.New("concurrent release blocked")
	ErrArtifactConnectionFailed = errors.New("artifact repository connection failed")
)
