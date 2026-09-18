package gitcredential

import (
	"net/url"
	"strings"
	"time"
)

// Provider is the Git server flavour a credential talks to. Only GitLab is
// supported today; the field exists so a second provider can be added without
// changing the stored rows of the first one.
type Provider string

const (
	ProviderGitLab Provider = "gitlab"
)

// Valid 封装当前模块的业务处理逻辑。
func (p Provider) Valid() bool {
	switch p {
	case ProviderGitLab:
		return true
	default:
		return false
	}
}

// AuthType selects how the secret is presented to the Git server: a personal
// access token in a PRIVATE-TOKEN header, or username/password basic auth.
type AuthType string

const (
	AuthTypeToken    AuthType = "token"
	AuthTypePassword AuthType = "password"
)

// Valid 封装当前模块的业务处理逻辑。
func (a AuthType) Valid() bool {
	switch a {
	case AuthTypeToken, AuthTypePassword:
		return true
	default:
		return false
	}
}

// Status controls whether a credential may be used to read commits. A disabled
// credential stays in the table (and keeps its ciphertext) but is ignored by the
// release order commit lookup.
type Status string

const (
	StatusActive   Status = "active"
	StatusDisabled Status = "disabled"
)

// Valid 封装当前模块的业务处理逻辑。
func (s Status) Valid() bool {
	switch s {
	case StatusActive, StatusDisabled:
		return true
	default:
		return false
	}
}

type Credential struct {
	ID       string
	Name     string
	Provider Provider
	// BaseURL is the address prefix the credential applies to, for example
	// http://git.cloud.local:9080. Applications are matched to a credential by
	// this prefix, which is why no application-side field was added.
	BaseURL  string
	Username string
	// Secret holds the access token or the password in clear text inside the
	// process; it is stored encrypted and never returned over HTTP.
	Secret    string
	AuthType  AuthType
	Status    Status
	Remark    string
	CreatedAt time.Time
	UpdatedAt time.Time
}

type ListFilter struct {
	Keyword  string
	Status   Status
	Page     int
	PageSize int
}

type UpdateInput struct {
	Name     string
	Provider Provider
	BaseURL  string
	Username string
	Secret   string
	AuthType AuthType
	Status   Status
	Remark   string
}

// NormalizeBaseURL canonicalises a Git base URL so both sides of the prefix
// match in ResolveForRepoURL use the same shape: a scheme is added when the
// caller typed a bare host, the scheme and host are lowercased (paths are not,
// GitLab project paths keep their case), and trailing slashes, query strings and
// fragments are dropped. An empty input maps to an empty result.
func NormalizeBaseURL(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}
	candidate := trimmed
	if !strings.Contains(candidate, "://") {
		candidate = "http://" + candidate
	}
	parsed, err := url.Parse(candidate)
	if err != nil || strings.TrimSpace(parsed.Host) == "" {
		return strings.TrimRight(trimmed, "/")
	}
	parsed.Scheme = strings.ToLower(parsed.Scheme)
	parsed.Host = strings.ToLower(parsed.Host)
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	parsed.RawPath = ""
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return strings.TrimRight(parsed.String(), "/")
}

// RepoPathFromURL extracts the GitLab project path from a repository URL, for
// example http://git.cloud.local:9080/code/bigData/fusion-source-web.git
// becomes code/bigData/fusion-source-web. The scheme, user info, host, port, a
// trailing ".git" and any query or fragment are dropped, which is exactly the
// "projects/:id" segment the GitLab API expects.
func RepoPathFromURL(repoURL string) string {
	raw := strings.TrimSpace(repoURL)
	if raw == "" {
		return ""
	}
	if idx := strings.IndexAny(raw, "?#"); idx >= 0 {
		raw = raw[:idx]
	}

	var path string
	switch {
	case strings.Contains(raw, "://"):
		raw = raw[strings.Index(raw, "://")+3:]
		path = repoPathAfterHost(raw)
	default:
		// scp-like syntax, for example git@gitlab.example.com:code/app.git. Only
		// treat the colon as a path separator when a user is present, otherwise
		// "host:port/path" would be split at the port.
		if colon := strings.Index(raw, ":"); colon > 0 && strings.Contains(raw[:colon], "@") {
			path = raw[colon+1:]
		} else {
			path = repoPathAfterHost(raw)
		}
	}

	path = strings.Trim(path, "/")
	path = strings.TrimSuffix(path, ".git")
	return strings.Trim(path, "/")
}

// repoPathAfterHost drops everything up to and including the first path
// separator, which removes the optional "user@" plus the host and port.
func repoPathAfterHost(raw string) string {
	slash := strings.Index(raw, "/")
	if slash < 0 {
		return ""
	}
	return raw[slash+1:]
}

// ResolveForRepoURL picks the credential whose normalized base URL is the
// longest prefix of repoURL. Longest-prefix wins so a credential scoped to one
// group (http://git.cloud.local:9080/code/bigData) beats the host-wide one
// (http://git.cloud.local:9080) for projects inside that group.
//
// Only the scheme and host are compared case-insensitively, following the
// normalisation above; the project path keeps its case. The caller is
// responsible for filtering out credentials that must not be used, such as
// disabled ones.
func ResolveForRepoURL(creds []Credential, repoURL string) (Credential, bool) {
	normalizedRepo := NormalizeBaseURL(repoURL)
	if normalizedRepo == "" {
		return Credential{}, false
	}
	best := Credential{}
	bestLength := 0
	for _, item := range creds {
		base := NormalizeBaseURL(item.BaseURL)
		if base == "" || len(base) <= bestLength {
			continue
		}
		if !hasRepoPrefix(normalizedRepo, base) {
			continue
		}
		best = item
		bestLength = len(base)
	}
	if bestLength == 0 {
		return Credential{}, false
	}
	return best, true
}

// hasRepoPrefix reports whether repoURL genuinely sits under base. A plain
// string prefix is not enough: "/code/app" must not swallow "/code/apple" or
// "/code/app-legacy". Only a path boundary or the ".git" suffix GitLab appends
// to clone URLs counts as a real match.
func hasRepoPrefix(repoURL string, base string) bool {
	if !strings.HasPrefix(repoURL, base) {
		return false
	}
	rest := repoURL[len(base):]
	if rest == "" {
		return true
	}
	switch rest[0] {
	case '/', '?', '#':
		return true
	}
	return strings.HasPrefix(rest, ".git")
}
