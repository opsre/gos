package platformparam

import "time"

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

type Status int

const (
	StatusDisabled Status = 0
	StatusEnabled  Status = 1
)

// Valid 封装当前模块的业务处理逻辑。
func (s Status) Valid() bool {
	return s == StatusDisabled || s == StatusEnabled
}

type PlatformParamDict struct {
	ID            string
	ParamKey      string
	Name          string
	Description   string
	ParamType     ParamType
	Required      bool
	GitOpsLocator bool
	CDSelfFill    bool
	Builtin       bool
	Status        Status
	CreatedAt     time.Time
	UpdatedAt     time.Time
}
