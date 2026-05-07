package strategy

import "errors"

var (
	ErrTemplateNotFound          = errors.New("strategy template not found")
	ErrTemplateNameDuplicated    = errors.New("strategy template name already exists")
	ErrTemplateInUse             = errors.New("strategy template is referenced by bindings")
	ErrRuntimeBindingNotFound    = errors.New("application env runtime binding not found")
	ErrRuntimeBindingDuplicated  = errors.New("application env runtime binding already exists")
	ErrStrategyBindingNotFound   = errors.New("application env strategy binding not found")
	ErrStrategyBindingDuplicated = errors.New("application env strategy binding already exists")
	ErrNoStrategyBinding         = errors.New("no strategy binding configured for this application and environment")
	ErrPrecheckFailed            = errors.New("strategy precheck failed")
)
