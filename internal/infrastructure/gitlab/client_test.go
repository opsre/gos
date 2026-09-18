package gitlab

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestListCommitsEscapesProjectPathAndSendsToken(t *testing.T) {
	var (
		gotPath    string
		gotToken   string
		gotRef     string
		gotPerPage string
		gotBasic   string
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.EscapedPath()
		gotToken = r.Header.Get("PRIVATE-TOKEN")
		_, gotBasic, _ = r.BasicAuth()
		gotRef = r.URL.Query().Get("ref_name")
		gotPerPage = r.URL.Query().Get("per_page")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[
			{
				"id": "abcdef1234567890",
				"short_id": "abcdef1",
				"title": "fix: 修复发布单列表",
				"message": "fix: 修复发布单列表\n\n附带说明",
				"author_name": "张三",
				"author_email": "zhangsan@example.com",
				"committed_date": "2026-09-18T10:00:00.000+08:00",
				"web_url": "http://git.cloud.local:9080/code/bigData/app/-/commit/abcdef1234567890"
			},
			{
				"id": "oldsha",
				"short_id": "oldsha",
				"title": "old",
				"committed_date": "2026-09-17T10:00:00Z"
			}
		]`))
	}))
	defer server.Close()

	client := NewClient(Config{BaseURL: server.URL + "/", Secret: "glpat-1", AuthType: "token", TimeoutSec: 3})
	commits, err := client.ListCommits(context.Background(), "/code/bigData/app/", "main", 5, nil)
	if err != nil {
		t.Fatalf("ListCommits err = %v", err)
	}
	if len(commits) != 2 {
		t.Fatalf("commits = %+v, want 2", commits)
	}
	if gotPath != "/api/v4/projects/code%2FbigData%2Fapp/repository/commits" {
		t.Fatalf("path = %q, want the escaped project path", gotPath)
	}
	if gotToken != "glpat-1" {
		t.Fatalf("PRIVATE-TOKEN = %q", gotToken)
	}
	if gotBasic != "" {
		t.Fatalf("token auth must not send basic auth")
	}
	if gotRef != "main" || gotPerPage != "5" {
		t.Fatalf("query ref=%q per_page=%q", gotRef, gotPerPage)
	}
	first := commits[0]
	if first.ShortID != "abcdef1" || first.AuthorName != "张三" || first.AuthorEmail != "zhangsan@example.com" {
		t.Fatalf("commit = %+v", first)
	}
	if !strings.Contains(first.Message, "附带说明") {
		t.Fatalf("message = %q", first.Message)
	}
	if first.CommittedAt.IsZero() {
		t.Fatalf("committed_at was not parsed")
	}
	if !first.CommittedAt.Equal(time.Date(2026, 9, 18, 2, 0, 0, 0, time.UTC)) {
		t.Fatalf("committed_at = %v, want the UTC instant", first.CommittedAt)
	}
}

func TestListCommitsUsesBasicAuthForPasswordCredentials(t *testing.T) {
	var (
		user     string
		password string
		token    string
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, password, _ = r.BasicAuth()
		token = r.Header.Get("PRIVATE-TOKEN")
		_, _ = w.Write([]byte(`[]`))
	}))
	defer server.Close()

	client := NewClient(Config{BaseURL: server.URL, Username: "release-bot", Secret: "p@ss", AuthType: "password"})
	if _, err := client.ListCommits(context.Background(), "code/app", "", 0, nil); err != nil {
		t.Fatalf("ListCommits err = %v", err)
	}
	if user != "release-bot" || password != "p@ss" {
		t.Fatalf("basic auth = %q / %q", user, password)
	}
	if token != "" {
		t.Fatalf("password auth must not send PRIVATE-TOKEN")
	}
}

func TestCurrentUser(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/user" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		if r.Header.Get("PRIVATE-TOKEN") != "glpat-1" {
			t.Fatalf("missing token header")
		}
		_, _ = w.Write([]byte(`{"id":7,"username":"release-bot","name":"发布机器人"}`))
	}))
	defer server.Close()

	client := NewClient(Config{BaseURL: server.URL + "/", Secret: "glpat-1", AuthType: "token"})
	user, err := client.CurrentUser(context.Background())
	if err != nil {
		t.Fatalf("CurrentUser err = %v", err)
	}
	if user.ID != 7 || user.Username != "release-bot" || user.Name != "发布机器人" {
		t.Fatalf("user = %+v", user)
	}
}

func TestClientNormalizesErrors(t *testing.T) {
	tests := []struct {
		name       string
		status     int
		body       string
		wantErr    error
		wantInText string
	}{
		{
			name:       "unauthorized hints at the access token",
			status:     http.StatusUnauthorized,
			body:       `{"message":"401 Unauthorized"}`,
			wantErr:    ErrUnauthorized,
			wantInText: "访问令牌",
		},
		{
			name:       "forbidden is also an auth problem",
			status:     http.StatusForbidden,
			body:       `{"message":"403 Forbidden"}`,
			wantErr:    ErrUnauthorized,
			wantInText: "status=403 message=403 Forbidden",
		},
		{
			name:       "not found is a project problem",
			status:     http.StatusNotFound,
			body:       `{"message":"404 Project Not Found"}`,
			wantErr:    ErrProjectNotFound,
			wantInText: "status=404 message=404 Project Not Found",
		},
		{
			name:       "object shaped message",
			status:     http.StatusBadRequest,
			body:       `{"message":{"ref_name":["is invalid"]}}`,
			wantInText: `status=400 message={"ref_name":["is invalid"]}`,
		},
		{
			name:       "plain text body",
			status:     http.StatusBadGateway,
			body:       `<html>bad gateway</html>`,
			wantInText: "status=502",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(test.status)
				_, _ = w.Write([]byte(test.body))
			}))
			defer server.Close()

			client := NewClient(Config{BaseURL: server.URL, Secret: "glpat-1", AuthType: "token"})
			_, err := client.ListCommits(context.Background(), "code/app", "main", 5, nil)
			if err == nil {
				t.Fatalf("ListCommits err = nil, want a failure")
			}
			if test.wantErr != nil && !errors.Is(err, test.wantErr) {
				t.Fatalf("err = %v, want %v", err, test.wantErr)
			}
			if !strings.Contains(err.Error(), test.wantInText) {
				t.Fatalf("err = %q, want it to contain %q", err.Error(), test.wantInText)
			}
		})
	}
}

func TestListCommitsRequiresProjectPath(t *testing.T) {
	client := NewClient(Config{BaseURL: "http://git.cloud.local:9080", Secret: "s", AuthType: "token"})
	if _, err := client.ListCommits(context.Background(), "  ", "main", 5, nil); err == nil {
		t.Fatalf("ListCommits err = nil, want a project path error")
	}
}

func TestProjectWebURL(t *testing.T) {
	tests := []struct {
		base string
		path string
		want string
	}{
		{base: "http://git.cloud.local:9080/", path: "code/bigData/app", want: "http://git.cloud.local:9080/code/bigData/app"},
		{base: "http://git.cloud.local:9080", path: "/code/bigData/app/", want: "http://git.cloud.local:9080/code/bigData/app"},
		{base: "", path: "code/app", want: ""},
		{base: "http://git.cloud.local:9080", path: "", want: ""},
	}
	for _, test := range tests {
		if got := ProjectWebURL(test.base, test.path); got != test.want {
			t.Fatalf("ProjectWebURL(%q, %q) = %q, want %q", test.base, test.path, got, test.want)
		}
	}
}

func TestDecodeRejectsInvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"message":"not an array"}`))
	}))
	defer server.Close()

	client := NewClient(Config{BaseURL: server.URL, Secret: "s", AuthType: "token"})
	_, err := client.ListCommits(context.Background(), "code/app", "main", 5, nil)
	if err == nil || !strings.Contains(err.Error(), "decode gitlab commits") {
		t.Fatalf("err = %v, want a decode error", err)
	}
}

// commitPageBody renders one commits payload. index is the position of the
// commit in the newest-first history, so the date of a commit is derived from its
// distance to the base instant.
func commitPageBody(base time.Time, count int, firstIndex int) string {
	items := make([]string, 0, count)
	for index := firstIndex; index < firstIndex+count; index++ {
		items = append(items, `{"id":"sha-`+strconv.Itoa(index)+`","short_id":"sha-`+strconv.Itoa(index)+
			`","title":"sha-`+strconv.Itoa(index)+`","committed_date":"`+
			base.Add(-time.Duration(index)*time.Hour).UTC().Format(time.RFC3339)+`"}`)
	}
	return "[" + strings.Join(items, ",") + "]"
}

// TestListCommitsReadsWindowAtAnchoredInstant covers the anchored read: the
// window asks for a full page ending at the instant, and a commit newer than the
// anchor never reaches the caller, which reads the first entry as the head the
// release ran at.
func TestListCommitsReadsWindowAtAnchoredInstant(t *testing.T) {
	base := time.Date(2026, 9, 18, 10, 0, 0, 0, time.UTC)
	anchor := base.Add(-3 * time.Hour)
	var (
		requests int
		gotUntil []string
		gotPages []string
		gotSizes []string
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		gotUntil = append(gotUntil, r.URL.Query().Get("until"))
		gotPages = append(gotPages, r.URL.Query().Get("page"))
		gotSizes = append(gotSizes, r.URL.Query().Get("per_page"))
		w.Header().Set("Content-Type", "application/json")
		// The API answers honestly: everything is at or before the anchor.
		_, _ = w.Write([]byte(commitPageBody(anchor, 6, 0)))
	}))
	defer server.Close()

	client := NewClient(Config{BaseURL: server.URL, Secret: "s", AuthType: "token"})
	commits, err := client.ListCommits(context.Background(), "code/app", "main", 3, &anchor)
	if err != nil {
		t.Fatalf("ListCommits err = %v", err)
	}
	if len(commits) != 3 || commits[0].ID != "sha-0" || commits[2].ID != "sha-2" {
		t.Fatalf("commits = %+v, want the first three of the anchored window", commits)
	}
	if requests != 1 {
		t.Fatalf("requests = %d, want one page for a window that already reaches the anchor", requests)
	}
	if gotUntil[0] != anchor.UTC().Format(time.RFC3339) {
		t.Fatalf("until = %q, want the anchor instant", gotUntil[0])
	}
	if gotSizes[0] != strconv.Itoa(maxCommitsPerPage) {
		t.Fatalf("per_page = %q, want a full window page", gotSizes[0])
	}
	if gotPages[0] != "1" {
		t.Fatalf("page = %q, want the walk to start at the newest page", gotPages[0])
	}
}

// TestListCommitsPagesWhenTheFirstWindowIsTooYoung covers the GitLab side of the
// deepening: a first page that holds nothing at or before the anchor makes the
// client walk one page further back.
func TestListCommitsPagesWhenTheFirstWindowIsTooYoung(t *testing.T) {
	base := time.Date(2026, 9, 18, 10, 0, 0, 0, time.UTC)
	anchor := base.Add(-250 * time.Hour)
	var pages []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		page := r.URL.Query().Get("page")
		pages = append(pages, page)
		w.Header().Set("Content-Type", "application/json")
		switch page {
		case "2":
			// The second page reaches the anchor: an older commit and its parent.
			_, _ = w.Write([]byte(commitPageBody(base, 2, 300)))
		default:
			// A full page entirely newer than the anchor.
			_, _ = w.Write([]byte(commitPageBody(base, maxCommitsPerPage, 0)))
		}
	}))
	defer server.Close()

	client := NewClient(Config{BaseURL: server.URL, Secret: "s", AuthType: "token"})
	commits, err := client.ListCommits(context.Background(), "code/app", "main", 2, &anchor)
	if err != nil {
		t.Fatalf("ListCommits err = %v", err)
	}
	if len(pages) != 2 || pages[0] != "1" || pages[1] != "2" {
		t.Fatalf("pages = %+v, want a walk of two pages", pages)
	}
	if len(commits) != 2 || commits[0].ID != "sha-300" || commits[1].ID != "sha-301" {
		t.Fatalf("commits = %+v, want the page that reaches the anchor", commits)
	}
}

// TestListCommitsReportsShallowHistory covers a release instant that predates the
// repository history: reporting the newest commits instead would show a head that
// did not exist when the release ran.
func TestListCommitsReportsShallowHistory(t *testing.T) {
	anchor := time.Date(2026, 9, 18, 10, 0, 0, 0, time.UTC)
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[]`))
	}))
	defer server.Close()

	client := NewClient(Config{BaseURL: server.URL, Secret: "s", AuthType: "token"})
	commits, err := client.ListCommits(context.Background(), "code/app", "main", 5, &anchor)
	if err == nil {
		t.Fatalf("ListCommits = %+v, want a shallow history error", commits)
	}
	if !errors.Is(err, ErrHistoryTooShallow) {
		t.Fatalf("err = %v, want ErrHistoryTooShallow", err)
	}
	if !strings.Contains(err.Error(), "提交历史不足") {
		t.Fatalf("err = %q, want a readable message", err.Error())
	}
	if requests != 1 {
		t.Fatalf("requests = %d, want the empty first page to end the walk", requests)
	}
}

func TestCommitsAtOrBeforeKeepsTheHistoryTail(t *testing.T) {
	base := time.Date(2026, 9, 18, 10, 0, 0, 0, time.UTC)
	commits := []Commit{
		{ID: "new", CommittedAt: base},
		{ID: "unknown"},
		{ID: "anchor", CommittedAt: base.Add(-2 * time.Hour)},
		{ID: "older", CommittedAt: base.Add(-3 * time.Hour)},
	}
	windowed := CommitsAtOrBefore(commits, base.Add(-2*time.Hour))
	if len(windowed) != 2 || windowed[0].ID != "anchor" || windowed[1].ID != "older" {
		t.Fatalf("windowed = %+v, want the anchor and the history behind it", windowed)
	}
	// A commit without a usable date never becomes the head, and a window that
	// does not reach the instant reports nothing instead of guessing.
	if got := CommitsAtOrBefore(commits, base.Add(-5*time.Hour)); got != nil {
		t.Fatalf("windowed = %+v, want no window", got)
	}
	if got := CommitsAtOrBefore(nil, base); got != nil {
		t.Fatalf("windowed = %+v, want no window for an empty list", got)
	}
}
