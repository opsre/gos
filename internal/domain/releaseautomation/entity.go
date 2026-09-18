package releaseautomation

import (
	"strings"
	"time"
)

// DispatchMode 是自动建单后要触发的派发动作。
//
// 取值与发布单链路上的 Build/Deploy/Execute 对齐：
//   - build：只触发构建（CI）；
//   - build_deploy：构建成功后接着部署（CI + CD）；
//   - execute：跳过构建，直接执行已就绪的执行（CD/回滚等）。
type DispatchMode string

const (
	DispatchModeBuild       DispatchMode = "build"
	DispatchModeBuildDeploy DispatchMode = "build_deploy"
	DispatchModeExecute     DispatchMode = "execute"
)

// Valid 判断派发方式是否合法。空值在归一化阶段会被补成 DispatchModeBuild。
func (m DispatchMode) Valid() bool {
	switch m {
	case DispatchModeBuild, DispatchModeBuildDeploy, DispatchModeExecute:
		return true
	default:
		return false
	}
}

// DefaultDispatchMode 是创建配置时 dispatch_mode 缺省值。
const DefaultDispatchMode = DispatchModeBuild

// Param 是一条「发布模板参数」配置，字段与建单接口的 params 元素一一对应。
//
// 表里整组参数以 params_json 存储：参数组合是配置的一部分，单独建表会引入
// 一张只有本模块使用的子表，收益不抵成本。
type Param struct {
	PipelineScope     string `json:"pipeline_scope"`
	ParamKey          string `json:"param_key"`
	ExecutorParamName string `json:"executor_param_name"`
	ParamValue        string `json:"param_value"`
	ValueSource       string `json:"value_source"`
}

// Automation 是一条「发布自动化」配置：给某应用某环境的某分支配好模板、参数和
// 派发方式，后台轮询分支 HEAD，一旦发现新提交就自动建发布单并派发。
type Automation struct {
	ID              string
	Name            string
	ApplicationID   string
	ApplicationName string
	TemplateID      string
	TemplateName    string
	EnvCode         string
	GitRef          string
	DispatchMode    DispatchMode
	Enabled         bool
	Params          []Param
	Remark          string
	// LastSeenSHA 是本模块的基线：轮询时与分支 HEAD 比较，只有 HEAD 变了才建单。
	// 首次保存配置时用当前 HEAD 做基线，因此「配置好就立刻发一版」不会发生。
	LastSeenSHA string
	// LastTriggeredSHA 是最近一次真正建单时的 HEAD，用于页面展示和排查。
	LastTriggeredSHA string
	// LastOrderID 是最近一次自动创建的发布单 ID，便于从配置跳到发布单。
	LastOrderID string
	// LastCheckedAt 只记录最近一次轮询检查时间，失败或无需建单时也会刷新。
	LastCheckedAt *time.Time
	// LastError 是最近一次轮询的可读原因（git 读失败、已有在途单、建单失败等）。
	// 轮询成功后清空；它同时是运维排查轮询不生效的第一现场。
	LastError string
	// CreatorUserID / CreatorName 记录配置创建者，自动建单时作为发布单发起人，
	// 保证「谁配的自动化」在发布单上可追溯。
	CreatorUserID string
	CreatorName   string
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// ListFilter 是配置列表的查询条件。
type ListFilter struct {
	Keyword       string
	ApplicationID string
	// Enabled 为 nil 表示不过滤启用状态（页面上的「全部」）。
	Enabled  *bool
	Page     int
	PageSize int
}

// IdentityChanged 判断应用/环境/分支是否发生变化。这三者共同决定「跟着哪个分支
// 发哪个环境」，任一项变化都必须重设基线，否则旧 sha 会被当成新提交。
func (a Automation) IdentityChanged(other Automation) bool {
	return strings.TrimSpace(a.ApplicationID) != strings.TrimSpace(other.ApplicationID) ||
		strings.TrimSpace(a.EnvCode) != strings.TrimSpace(other.EnvCode) ||
		strings.TrimSpace(a.GitRef) != strings.TrimSpace(other.GitRef)
}

// HeadUnchanged 判断分支 HEAD 是否与基线一致（即没有新提交）。
func HeadUnchanged(lastSeenSHA string, headSHA string) bool {
	seen := strings.TrimSpace(lastSeenSHA)
	head := strings.TrimSpace(headSHA)
	// 基线为空说明配置从未拿到过 HEAD：此时任何 HEAD 都视为「新提交」，
	// 让轮询去建第一单，而不是把空值当成「已处理」。
	return seen != "" && seen == head
}
