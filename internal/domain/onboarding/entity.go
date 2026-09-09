package onboarding

import (
	"context"
	"errors"
	"time"
)

var (
	ErrNotFound  = errors.New("onboarding session not found")
	ErrConflict  = errors.New("配置已变化或正在保存，请刷新后重试")
	ErrForbidden = errors.New("无权访问此接入任务")
)

type Identity struct {
	ProjectID   string `json:"project_id"`
	ProjectName string `json:"project_name"`
	ProjectKey  string `json:"project_key"`
	Name        string `json:"name"`
	Key         string `json:"key"`
	OwnerUserID string `json:"owner_user_id"`
	Description string `json:"description"`
}
type Parameter struct {
	Scope          string `json:"scope"`
	Name           string `json:"name"`
	ParamKey       string `json:"param_key"`
	NewFieldName   string `json:"new_field_name"`
	ValueSource    string `json:"value_source"`
	SourceParamKey string `json:"source_param_key"`
	FixedValue     string `json:"fixed_value"`
	Omit           bool   `json:"omit"`
}
type Draft struct {
	Identity     Identity    `json:"identity"`
	CIPipelineID string      `json:"ci_pipeline_id"`
	CDPipelineID string      `json:"cd_pipeline_id"`
	Params       []Parameter `json:"params"`
	TemplateName string      `json:"template_name"`
	// Existing application policies are preserved unless this explicit switch is set.
	ChangeApprovalFlow bool   `json:"change_approval_flow"`
	ApprovalFlowID     string `json:"approval_flow_id"`
	SaveAppSources     bool   `json:"save_app_sources"`
	RepoURL            string `json:"repo_url"`
	Branch             string `json:"branch"`
}
type References struct {
	ProjectID     string `json:"project_id"`
	ApplicationID string `json:"application_id"`
	CIBindingID   string `json:"ci_binding_id"`
	CDBindingID   string `json:"cd_binding_id"`
	TemplateID    string `json:"template_id"`
}
type Issue struct {
	Code      string `json:"code"`
	Severity  string `json:"severity"`
	Step      string `json:"step"`
	FieldPath string `json:"field_path"`
	Message   string `json:"message"`
	Remedy    string `json:"remedy"`
}
type Check struct {
	Status string  `json:"status"`
	Issues []Issue `json:"issues"`
}
type Session struct {
	ID                  string     `json:"id"`
	OwnerUserID         string     `json:"owner_user_id"`
	Mode                string     `json:"mode"`
	Status              string     `json:"status"`
	CurrentStep         string     `json:"current_step"`
	Version             int64      `json:"version"`
	Draft               Draft      `json:"draft"`
	Refs                References `json:"refs"`
	Check               Check      `json:"check"`
	FirstReleaseOrderID string     `json:"first_release_order_id"`
	CreatedAt           time.Time  `json:"created_at"`
	UpdatedAt           time.Time  `json:"updated_at"`
}
type Repository interface {
	Create(context.Context, Session) error
	Get(context.Context, string) (Session, error)
	List(context.Context, string) ([]Session, error)
	Save(context.Context, Session, int64) (Session, error)
	BeginOperation(ctx context.Context, id, key, hash, step string, version int64) (Session, bool, error)
	FinishOperation(ctx context.Context, session Session, key string, success bool) (Session, error)
}
