package gitlab

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// defaultTimeoutSeconds bounds every GitLab call. GitLab itself is quick, but
// the platform also reaches it through cluster networking, so the budget is
// larger than the 5s used for Jenkins while still failing fast enough for a
// release-order list that polls every 10s.
const defaultTimeoutSeconds = 10

// maxResponseBytes caps how much of a GitLab response body is buffered.
const maxResponseBytes = 4 << 20

// maxCommitsPerPage mirrors the GitLab API ceiling for per_page.
const maxCommitsPerPage = 100

// maxCommitPages bounds the paging of one anchored read: a window read walks
// back towards the anchor, and two pages (200 commits) are the budget the
// release order lookup allows before reporting the history as too shallow.
const maxCommitPages = 2

var (
	// ErrUnauthorized covers 401 and 403. The most common cause in this platform
	// is a credential that only carries an account password while the GitLab
	// instance requires an access token.
	ErrUnauthorized = errors.New("gitlab authentication failed")
	// ErrProjectNotFound covers 404, which means either the project path is wrong
	// or the credential cannot see it.
	ErrProjectNotFound = errors.New("gitlab project not found")
	// ErrHistoryTooShallow means the repository history read did not reach the
	// requested as-of instant: the release ran so far back that the walk of the
	// commit history ended before it. The message is user facing (the release
	// order list shows it) and is shared with the git CLI channel.
	ErrHistoryTooShallow = errors.New("提交历史不足，无法定位发布时点的提交")
)

type Config struct {
	BaseURL  string
	Username string
	Secret   string
	// AuthType is "token" for a personal access token, anything else is treated
	// as username/password basic auth.
	AuthType   string
	TimeoutSec int
}

type Commit struct {
	ID          string
	ShortID     string
	Title       string
	Message     string
	AuthorName  string
	AuthorEmail string
	CommittedAt time.Time
	WebURL      string
}

type User struct {
	ID       int
	Username string
	Name     string
}

type Client struct {
	baseURL  string
	username string
	secret   string
	authType string
	client   *http.Client
}

// NewClient 创建并返回对应组件实例。
func NewClient(cfg Config) *Client {
	timeout := cfg.TimeoutSec
	if timeout <= 0 {
		timeout = defaultTimeoutSeconds
	}
	return &Client{
		baseURL:  strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/"),
		username: strings.TrimSpace(cfg.Username),
		secret:   strings.TrimSpace(cfg.Secret),
		authType: strings.ToLower(strings.TrimSpace(cfg.AuthType)),
		client: &http.Client{
			Timeout: time.Duration(timeout) * time.Second,
		},
	}
}

// ListCommits returns the commits of a project ref, newest first. limit is
// clamped to the GitLab per_page ceiling so a caller cannot request an
// unbounded page.
//
// asOf switches the read to a history window:
//
//   - nil: the newest `limit` commits, which is the plain list read.
//   - non-nil: the commits at or before that instant, newest first, up to
//     `limit` of them. The first entry is the commit the ref pointed at as of
//     that instant. The window starts on a full page (per_page=100) and walks
//     back one more page when the first one held nothing at or before asOf; when
//     even that does not reach the instant, ErrHistoryTooShallow is returned
//     instead of commits the caller would misread as the release head.
func (c *Client) ListCommits(ctx context.Context, projectPath string, ref string, limit int, asOf *time.Time) ([]Commit, error) {
	path := NormalizeProjectPath(projectPath)
	if path == "" {
		return nil, fmt.Errorf("gitlab project path is required")
	}
	if limit <= 0 {
		limit = 5
	}
	if limit > maxCommitsPerPage {
		limit = maxCommitsPerPage
	}
	ref = strings.TrimSpace(ref)

	// A plain read asks for exactly the limit; an anchored read asks for a full
	// page per step so the window is deep enough to hold the anchor commit.
	perPage := limit
	pages := 1
	if asOf != nil {
		perPage = maxCommitsPerPage
		pages = maxCommitPages
	}

	commits := make([]Commit, 0, perPage)
	requestedPages := 0
	for page := 1; page <= pages; page++ {
		requestedPages++
		pageCommits, err := c.listCommitPage(ctx, path, ref, perPage, page, asOf)
		if err != nil {
			return nil, err
		}
		pageSize := len(pageCommits)
		if asOf != nil {
			// The API filters the page by `until`, but the window is filtered here as
			// well: a server or a proxy that drops the parameter must never hand back
			// a commit newer than the anchor, because the caller treats the first
			// entry as the release head.
			pageCommits = CommitsAtOrBefore(pageCommits, *asOf)
		}
		commits = append(commits, pageCommits...)
		if len(commits) >= limit {
			break
		}
		if pageSize < perPage {
			// The project is exhausted: paging again would return an empty page.
			break
		}
	}
	if asOf != nil && len(commits) == 0 {
		return nil, fmt.Errorf("%w（已读取 %d 页，分支历史早于该时点）", ErrHistoryTooShallow, requestedPages)
	}
	if len(commits) > limit {
		commits = commits[:limit]
	}
	return commits, nil
}

// listCommitPage performs one /repository/commits request.
func (c *Client) listCommitPage(
	ctx context.Context,
	path string,
	ref string,
	perPage int,
	page int,
	asOf *time.Time,
) ([]Commit, error) {
	query := url.Values{}
	if ref != "" {
		query.Set("ref_name", ref)
	}
	query.Set("per_page", strconv.Itoa(perPage))
	query.Set("page", strconv.Itoa(page))
	if asOf != nil {
		// GitLab's `until` is inclusive ("before or on that date"), which is the
		// commit date comparison the window needs.
		query.Set("until", asOf.UTC().Format(time.RFC3339))
	}

	endpoint := c.baseURL + "/api/v4/projects/" + url.PathEscape(path) + "/repository/commits?" + query.Encode()
	body, err := c.get(ctx, endpoint)
	if err != nil {
		return nil, err
	}

	var payload []commitPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("decode gitlab commits: %w", err)
	}
	commits := make([]Commit, 0, len(payload))
	for _, item := range payload {
		commits = append(commits, Commit{
			ID:          strings.TrimSpace(item.ID),
			ShortID:     strings.TrimSpace(item.ShortID),
			Title:       strings.TrimSpace(item.Title),
			Message:     strings.TrimSpace(item.Message),
			AuthorName:  strings.TrimSpace(item.AuthorName),
			AuthorEmail: strings.TrimSpace(item.AuthorEmail),
			CommittedAt: parseGitLabTime(item.CommittedDate),
			WebURL:      strings.TrimSpace(item.WebURL),
		})
	}
	if len(commits) > perPage {
		commits = commits[:perPage]
	}
	return commits, nil
}

// CommitsAtOrBefore returns the newest-first tail of a commit list that is at or
// before the instant: the head of that tail is the commit the branch pointed at
// then, and the tail continues into its history. A nil result means the list
// held no commit old enough. A commit without a usable date never satisfies the
// instant, because treating an unparsable date as "old enough" would report an
// arbitrary commit as the release head.
//
// It is exported because the git CLI channel applies the same window rule to the
// commits it parses out of `git log`.
func CommitsAtOrBefore(commits []Commit, asOf time.Time) []Commit {
	for index := range commits {
		committedAt := commits[index].CommittedAt
		if committedAt.IsZero() || committedAt.After(asOf) {
			continue
		}
		return commits[index:]
	}
	return nil
}

// CurrentUser calls GET /api/v4/user. It is the credential connection test: a
// success proves both the address and the secret work.
func (c *Client) CurrentUser(ctx context.Context) (User, error) {
	body, err := c.get(ctx, c.baseURL+"/api/v4/user")
	if err != nil {
		return User{}, err
	}
	var payload struct {
		ID       int    `json:"id"`
		Username string `json:"username"`
		Name     string `json:"name"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return User{}, fmt.Errorf("decode gitlab user: %w", err)
	}
	return User{
		ID:       payload.ID,
		Username: strings.TrimSpace(payload.Username),
		Name:     strings.TrimSpace(payload.Name),
	}, nil
}

// ProjectWebURL builds the browser URL of a project, used as the fallback when a
// commit payload carries no web_url.
func ProjectWebURL(baseURL string, projectPath string) string {
	base := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	path := NormalizeProjectPath(projectPath)
	if base == "" || path == "" {
		return ""
	}
	return base + "/" + path
}

// NormalizeProjectPath trims the separators GitLab does not want in the
// projects/:id segment.
func NormalizeProjectPath(projectPath string) string {
	return strings.Trim(strings.TrimSpace(projectPath), "/")
}

type commitPayload struct {
	ID            string `json:"id"`
	ShortID       string `json:"short_id"`
	Title         string `json:"title"`
	Message       string `json:"message"`
	AuthorName    string `json:"author_name"`
	AuthorEmail   string `json:"author_email"`
	CommittedDate string `json:"committed_date"`
	WebURL        string `json:"web_url"`
}

func (c *Client) get(ctx context.Context, endpoint string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	c.authorize(req)

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, buildHTTPError(resp.StatusCode, body)
	}
	return body, nil
}

// authorize attaches the credential. A token goes in the PRIVATE-TOKEN header
// GitLab expects; everything else falls back to basic auth, which is only
// applied when both halves are present.
func (c *Client) authorize(req *http.Request) {
	if c.authType == "token" {
		if c.secret != "" {
			req.Header.Set("PRIVATE-TOKEN", c.secret)
		}
		return
	}
	if c.username != "" && c.secret != "" {
		req.SetBasicAuth(c.username, c.secret)
	}
}

// buildHTTPError normalises a GitLab failure into one readable line and wraps
// the two statuses the callers branch on.
func buildHTTPError(statusCode int, body []byte) error {
	message := extractMessage(body)
	if message == "" {
		message = http.StatusText(statusCode)
	}
	normalized := fmt.Errorf("gitlab request failed: status=%d message=%s", statusCode, message)
	switch statusCode {
	case http.StatusUnauthorized, http.StatusForbidden:
		return fmt.Errorf("%w: %v；GitLab API 需要访问令牌，请在凭证中使用访问令牌或确认该账号有仓库读取权限", ErrUnauthorized, normalized)
	case http.StatusNotFound:
		return fmt.Errorf("%w: %v", ErrProjectNotFound, normalized)
	default:
		return normalized
	}
}

// extractMessage pulls GitLab's {"message": ...} payload. The field is a string
// for most errors but GitLab returns an object for validation failures, so both
// shapes are folded into one line.
func extractMessage(body []byte) string {
	if len(body) == 0 {
		return ""
	}
	var payload struct {
		Message json.RawMessage `json:"message"`
		Error   string          `json:"error"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return ""
	}
	if len(payload.Message) > 0 {
		var text string
		if err := json.Unmarshal(payload.Message, &text); err == nil {
			if trimmed := strings.TrimSpace(text); trimmed != "" {
				return trimmed
			}
		}
		var decoded any
		if err := json.Unmarshal(payload.Message, &decoded); err == nil {
			if encoded, err := json.Marshal(decoded); err == nil {
				return strings.TrimSpace(string(encoded))
			}
		}
	}
	return strings.TrimSpace(payload.Error)
}

func parseGitLabTime(value string) time.Time {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return time.Time{}
	}
	if parsed, err := time.Parse(time.RFC3339, trimmed); err == nil {
		return parsed.UTC()
	}
	if parsed, err := time.Parse("2006-01-02T15:04:05.000Z0700", trimmed); err == nil {
		return parsed.UTC()
	}
	return time.Time{}
}
