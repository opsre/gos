package pipelinescan

import "time"

type Category string

const (
	CategoryArtifact   Category = "artifact"
	CategorySecurity   Category = "security"
	CategoryCredential Category = "credential"
	CategoryNaming     Category = "naming"
	CategoryCustom     Category = "custom"
)

// Valid 封装当前模块的业务处理逻辑。
func (c Category) Valid() bool {
	switch c {
	case CategoryArtifact, CategorySecurity, CategoryCredential, CategoryNaming, CategoryCustom:
		return true
	default:
		return false
	}
}

type Severity string

const (
	SeverityInfo    Severity = "info"
	SeverityWarning Severity = "warning"
	SeverityError   Severity = "error"
)

// Valid 封装当前模块的业务处理逻辑。
func (s Severity) Valid() bool {
	switch s {
	case SeverityInfo, SeverityWarning, SeverityError:
		return true
	default:
		return false
	}
}

type RuleStatus int

const (
	RuleStatusDisabled RuleStatus = 0
	RuleStatusEnabled  RuleStatus = 1
)

// Valid 封装当前模块的业务处理逻辑。
func (s RuleStatus) Valid() bool {
	switch s {
	case RuleStatusDisabled, RuleStatusEnabled:
		return true
	default:
		return false
	}
}

type ScanStatus string

const (
	ScanStatusCompliant ScanStatus = "compliant"
	ScanStatusWarning   ScanStatus = "warning"
	ScanStatusFailed    ScanStatus = "failed"
	ScanStatusUnknown   ScanStatus = "unknown"
)

type FindingStatus string

const (
	FindingStatusOpen    FindingStatus = "open"
	FindingStatusIgnored FindingStatus = "ignored"
	FindingStatusFixed   FindingStatus = "fixed"
)

type Rule struct {
	ID                       string
	RuleCode                 string
	RuleName                 string
	Category                 Category
	Severity                 Severity
	Enabled                  bool
	Builtin                  bool
	TemplateValidationScopes []string
	ScopeJSON                string
	RuleDSL                  string
	Message                  string
	Suggestion               string
	CreatedAt                time.Time
	UpdatedAt                time.Time
}

type RuleListFilter struct {
	Keyword  string
	Category Category
	Severity Severity
	Enabled  *bool
	Page     int
	PageSize int
}

type RuleUpdateInput struct {
	RuleCode                 string
	RuleName                 string
	Category                 Category
	Severity                 Severity
	Enabled                  bool
	TemplateValidationScopes []string
	ScopeJSON                string
	RuleDSL                  string
	Message                  string
	Suggestion               string
}

type Result struct {
	ID            string
	PipelineID    string
	PipelineName  string
	ScanStatus    ScanStatus
	TotalFindings int
	ErrorCount    int
	WarningCount  int
	InfoCount     int
	ScriptHash    string
	LastScannedAt time.Time
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

type ResultListFilter struct {
	PipelineName string
	ScanStatus   ScanStatus
	Page         int
	PageSize     int
}

type Finding struct {
	ID          string
	PipelineID  string
	RuleID      string
	RuleCode    string
	RuleName    string
	Severity    Severity
	LineNo      int
	MatchedText string
	Message     string
	Suggestion  string
	DetailsJSON string
	Status      FindingStatus
	CreatedAt   time.Time
	UpdatedAt   time.Time
}
