package executorparam

import "time"

// ExecutorType 表示参数定义所属的执行器类型。
// 这里故意使用“执行器”而不是“管线”，是为了兼容 Jenkins、ArgoCD
// 以及后续可能接入的其他执行端，避免模型被单一 CI/CD 工具绑死。
type ExecutorType string

const (
	ExecutorTypeJenkins ExecutorType = "jenkins"
	ExecutorTypeArgoCD  ExecutorType = "argocd"
	ExecutorTypeCustom  ExecutorType = "custom"
)

// Valid 封装当前模块的业务处理逻辑。
func (t ExecutorType) Valid() bool {
	switch t {
	case ExecutorTypeJenkins, ExecutorTypeArgoCD, ExecutorTypeCustom:
		return true
	default:
		return false
	}
}

type ParamType string

const (
	ParamTypeString ParamType = "string"
	ParamTypeChoice ParamType = "choice"
	ParamTypeBool   ParamType = "bool"
	ParamTypeNumber ParamType = "number"
)

// Valid 封装当前模块的业务处理逻辑。
func (t ParamType) Valid() bool {
	switch t {
	case ParamTypeString, ParamTypeChoice, ParamTypeBool, ParamTypeNumber:
		return true
	default:
		return false
	}
}

type SourceFrom string

const (
	SourceFromSyncJenkins SourceFrom = "sync_jenkins"
	SourceFromManual      SourceFrom = "manual"
)

// Valid 封装当前模块的业务处理逻辑。
func (s SourceFrom) Valid() bool {
	switch s {
	case SourceFromSyncJenkins, SourceFromManual:
		return true
	default:
		return false
	}
}

type Status string

const (
	StatusActive   Status = "active"
	StatusInactive Status = "inactive"
)

// Valid 封装当前模块的业务处理逻辑。
func (s Status) Valid() bool {
	switch s {
	case StatusActive, StatusInactive:
		return true
	default:
		return false
	}
}

// ExecutorParamDef 是平台统一维护的“执行器参数定义”模型。
//
// 设计上它不再只绑定 Jenkins pipeline：
// 1. pipeline_id 仍然保留，用于关联当前执行记录来源；
// 2. executor_type 决定这条参数定义属于 Jenkins / ArgoCD / custom；
// 3. executor_param_name 保存执行器侧的真实参数名；
// 4. param_key 保存映射后的平台标准字段 key。
//
// 这样后续扩展 CD=ArgoCD 时，平台仍然可以复用同一套参数映射、模板与发布单逻辑。
type ExecutorParamDef struct {
	ID                string
	ApplicationID     string
	ApplicationName   string
	ApplicationKey    string
	BindingType       string
	PipelineName      string
	PipelineID        string
	ExecutorType      ExecutorType
	ExecutorParamName string
	ParamKey          string
	ParamType         ParamType
	SingleSelect      bool
	Required          bool
	DefaultValue      string
	Description       string
	Visible           bool
	Editable          bool
	SourceFrom        SourceFrom
	Status            Status
	RawMeta           string
	SortNo            int
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

type JenkinsParamSnapshot struct {
	Name         string
	ParamType    ParamType
	SingleSelect bool
	Required     bool
	DefaultValue string
	Description  string
	RawMeta      string
	SortNo       int
}

type JenkinsJobParamSet struct {
	JobName     string
	JobFullName string
	Params      []JenkinsParamSnapshot
}
