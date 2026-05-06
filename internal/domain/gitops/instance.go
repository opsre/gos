package gitops

import "time"

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

type Instance struct {
	ID                    string
	InstanceCode          string
	Name                  string
	LocalRoot             string
	DefaultBranch         string
	Username              string
	Password              string
	Token                 string
	AuthorName            string
	AuthorEmail           string
	CommitMessageTemplate string
	CommandTimeoutSec     int
	Status                Status
	Remark                string
	CreatedAt             time.Time
	UpdatedAt             time.Time
}

type InstanceListFilter struct {
	Keyword  string
	Status   Status
	Page     int
	PageSize int
}
