package notification

import "time"

type SourceType string

const (
	SourceTypeDingTalk SourceType = "dingtalk"
	SourceTypeWeCom    SourceType = "wecom"
)

// Valid 封装当前模块的业务处理逻辑。
func (s SourceType) Valid() bool {
	switch s {
	case SourceTypeDingTalk, SourceTypeWeCom:
		return true
	default:
		return false
	}
}

type ConditionOperator string

const (
	ConditionOperatorEquals      ConditionOperator = "equals"
	ConditionOperatorNotEquals   ConditionOperator = "not_equals"
	ConditionOperatorContains    ConditionOperator = "contains"
	ConditionOperatorNotContains ConditionOperator = "not_contains"
	ConditionOperatorIsEmpty     ConditionOperator = "is_empty"
	ConditionOperatorNotEmpty    ConditionOperator = "not_empty"
)

// Valid 封装当前模块的业务处理逻辑。
func (s ConditionOperator) Valid() bool {
	switch s {
	case ConditionOperatorEquals,
		ConditionOperatorNotEquals,
		ConditionOperatorContains,
		ConditionOperatorNotContains,
		ConditionOperatorIsEmpty,
		ConditionOperatorNotEmpty:
		return true
	default:
		return false
	}
}

type Source struct {
	ID                string
	Name              string
	SourceType        SourceType
	WebhookURL        string
	VerificationParam string
	Enabled           bool
	Remark            string
	CreatedBy         string
	UpdatedBy         string
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

type MarkdownTemplateCondition struct {
	ParamKey      string
	Operator      ConditionOperator
	ExpectedValue string
	MarkdownText  string
	SortNo        int
}

type MarkdownTemplate struct {
	ID            string
	Name          string
	TitleTemplate string
	BodyTemplate  string
	Conditions    []MarkdownTemplateCondition
	Enabled       bool
	Remark        string
	CreatedBy     string
	UpdatedBy     string
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

type Hook struct {
	ID                   string
	Name                 string
	SourceID             string
	SourceName           string
	SourceType           SourceType
	MarkdownTemplateID   string
	MarkdownTemplateName string
	Enabled              bool
	Remark               string
	CreatedBy            string
	UpdatedBy            string
	CreatedAt            time.Time
	UpdatedAt            time.Time
}

type SourceListFilter struct {
	Keyword  string
	Type     SourceType
	Enabled  *bool
	Page     int
	PageSize int
}

type MarkdownTemplateListFilter struct {
	Keyword  string
	Enabled  *bool
	Page     int
	PageSize int
}

type HookListFilter struct {
	Keyword  string
	Enabled  *bool
	Page     int
	PageSize int
}
