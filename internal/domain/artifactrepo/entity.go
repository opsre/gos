package artifactrepo

import "time"

type RepositoryType string

const (
	RepositoryTypeOSS  RepositoryType = "oss"
	RepositoryTypeFTP  RepositoryType = "ftp"
	RepositoryTypeSFTP RepositoryType = "sftp"
)

func (t RepositoryType) Valid() bool {
	switch t {
	case RepositoryTypeOSS, RepositoryTypeFTP, RepositoryTypeSFTP:
		return true
	default:
		return false
	}
}

// UsesObjectStorage reports whether the type is backed by object storage.
// Only object storage types project into the oss_* pipeline params.
func (t RepositoryType) UsesObjectStorage() bool {
	return t == RepositoryTypeOSS
}

// DefaultPort returns the well-known port for network file transfer types.
// Object storage returns 0 because its port is carried by the endpoint URL.
func (t RepositoryType) DefaultPort() int {
	switch t {
	case RepositoryTypeFTP:
		return 21
	case RepositoryTypeSFTP:
		return 22
	default:
		return 0
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
	ID             string
	Name           string
	RepositoryType RepositoryType
	Endpoint       string
	// Port is 0 when the caller did not pin one; the effective port then comes
	// from the endpoint or from RepositoryType.DefaultPort().
	Port            int
	Bucket          string
	Directory       string
	AccessKeyID     string
	AccessKeySecret string
	Username        string
	Password        string
	PrivateKey      string
	// DisableEPSV applies to FTP only. Transfers are always passive because the
	// client library has no active mode; this switch only forces the older PASV
	// command for servers that lack the EPSV extension.
	DisableEPSV bool
	// HostKeyFingerprint applies to SFTP only. It is optional: an empty value
	// means the SSH host key is not verified at all, which the UI states plainly.
	HostKeyFingerprint string
	ACL                ACL
	Status             Status
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

type ListFilter struct {
	Keyword        string
	RepositoryType RepositoryType
	Status         Status
	Page           int
	PageSize       int
}

type UpdateInput struct {
	Name               string
	RepositoryType     RepositoryType
	Endpoint           string
	Port               int
	Bucket             string
	Directory          string
	AccessKeyID        string
	AccessKeySecret    string
	Username           string
	Password           string
	PrivateKey         string
	DisableEPSV        bool
	HostKeyFingerprint string
	ACL                ACL
	Status             Status
}
