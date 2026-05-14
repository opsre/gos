package release

import "errors"

var (
	ErrOrderNotFound                 = errors.New("release order not found")
	ErrOrderDuplicated               = errors.New("release order already exists")
	ErrExecutionNotFound             = errors.New("release order execution not found")
	ErrStepNotFound                  = errors.New("release order step not found")
	ErrPipelineStageNotFound         = errors.New("release order pipeline stage not found")
	ErrTemplateNotFound              = errors.New("release template not found")
	ErrTemplateDuplicated            = errors.New("release template already exists")
	ErrDeploySnapshotNotFound        = errors.New("release order deploy snapshot not found")
	ErrArtifactMetadataNotFound      = errors.New("release order artifact metadata not found")
	ErrExecutionLockNotFound         = errors.New("release execution lock not found")
	ErrAppReleaseStateNotFound       = errors.New("app release state not found")
	ErrAppReleaseStateNotConfirmable = errors.New("app release state is not confirmable")
	ErrScheduleNotFound              = errors.New("release order schedule not found")
	ErrScheduleDuplicated            = errors.New("release order schedule already exists")
)
