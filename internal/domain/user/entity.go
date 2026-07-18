package user

import "time"

type Role string

const (
	RoleAdmin  Role = "admin"
	RoleNormal Role = "normal"
)

// Valid 封装当前模块的业务处理逻辑。
func (r Role) Valid() bool {
	switch r {
	case RoleAdmin, RoleNormal:
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

type User struct {
	ID           string
	Username     string
	DisplayName  string
	Email        string
	Phone        string
	Role         Role
	Status       Status
	PasswordHash string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// OrganizationNode 是用户及其直属主管关系的聚合读取模型。
type OrganizationNode struct {
	User          User
	ManagerUserID string
}

type Permission struct {
	ID          string
	Code        string
	Name        string
	Module      string
	Action      string
	Description string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type UserPermission struct {
	ID             string
	UserID         string
	PermissionCode string
	ScopeType      string
	ScopeValue     string
	Enabled        bool
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type UserParamPermission struct {
	ID            string
	UserID        string
	ParamKey      string
	ApplicationID string
	CanView       bool
	CanEdit       bool
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

type UserSession struct {
	ID            string
	UserID        string
	AccessToken   string
	ExpiredAt     time.Time
	ClientIP      string
	UserAgent     string
	RevokedAt     *time.Time
	RevokedReason string
	CreatedAt     time.Time
}

type UserListFilter struct {
	Username string
	Name     string
	Role     Role
	Status   Status
	Page     int
	PageSize int
}

type UserUpdateInput struct {
	DisplayName  string
	Email        string
	Phone        string
	Role         Role
	Status       Status
	PasswordHash string
}

type UserPermissionGrant struct {
	PermissionCode string
	ScopeType      string
	ScopeValue     string
}

type PermissionFilter struct {
	Module string
	Action string
}
