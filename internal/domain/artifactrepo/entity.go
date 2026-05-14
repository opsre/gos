package artifactrepo

import "time"

type RepositoryType string

const (
	RepositoryTypeOSS RepositoryType = "oss"
)

func (t RepositoryType) Valid() bool {
	switch t {
	case RepositoryTypeOSS:
		return true
	default:
		return false
	}
}

type ACL string

const (
	ACLPrivate    ACL = "private"
	ACLPublicRead ACL = "public-read"
)

func (a ACL) Valid() bool {
	switch a {
	case ACLPrivate, ACLPublicRead:
		return true
	default:
		return false
	}
}

type Status string

const (
	StatusEnabled  Status = "enabled"
	StatusDisabled Status = "disabled"
)

func (s Status) Valid() bool {
	switch s {
	case StatusEnabled, StatusDisabled:
		return true
	default:
		return false
	}
}

type ArtifactRepository struct {
	ID              string
	Name            string
	RepositoryType  RepositoryType
	Endpoint        string
	Bucket          string
	Directory       string
	AccessKeyID     string
	AccessKeySecret string
	ACL             ACL
	Status          Status
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

type ListFilter struct {
	Keyword        string
	RepositoryType RepositoryType
	Status         Status
	Page           int
	PageSize       int
}

type UpdateInput struct {
	Name            string
	RepositoryType  RepositoryType
	Endpoint        string
	Bucket          string
	Directory       string
	AccessKeyID     string
	AccessKeySecret string
	ACL             ACL
	Status          Status
}
