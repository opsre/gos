package usecase

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	applicationdomain "gos/internal/domain/application"
	gitcredentialdomain "gos/internal/domain/gitcredential"
	releasedomain "gos/internal/domain/release"
	"gos/internal/infrastructure/gitcli"
	"gos/internal/infrastructure/gitlab"
)

type gitCommitReleaseRepoFake struct {
	releasedomain.Repository
	orders map[string]releasedomain.ReleaseOrder
	// err simulates an infrastructure failure instead of a missing order.
	err error
}

func (r *gitCommitReleaseRepoFake) GetByID(_ context.Context, id string) (releasedomain.ReleaseOrder, error) {
	if r.err != nil {
		return releasedomain.ReleaseOrder{}, r.err
	}
	order, ok := r.orders[id]
	if !ok {
		return releasedomain.ReleaseOrder{}, releasedomain.ErrOrderNotFound
	}
	return order, nil
}

type gitCommitApplicationRepoFake struct {
	applicationdomain.Repository
	applications map[string]applicationdomain.Application
}

func (r *gitCommitApplicationRepoFake) GetByID(_ context.Context, id string) (applicationdomain.Application, error) {
	application, ok := r.applications[id]
	if !ok {
		return applicationdomain.Application{}, applicationdomain.ErrNotFound
	}
	return application, nil
}

// gitCommitCredentialRepoFake reuses the credential fake from the credential
// tests so both suites exercise the same filter semantics.
type gitCommitCredentialRepoFake struct {
	gitcredentialdomain.Repository
	items []gitcredentialdomain.Credential
	// err simulates an infrastructure failure of the credential query.
	err   error
	calls int
}

func (r *gitCommitCredentialRepoFake) List(_ context.Context, filter gitcredentialdomain.ListFilter) ([]gitcredentialdomain.Credential, int64, error) {
	r.calls++
	if r.err != nil {
		return nil, 0, r.err
	}
	items := make([]gitcredentialdomain.Credential, 0, len(r.items))
	for _, item := range r.items {
		if filter.Status != "" && item.Status != filter.Status {
			continue
		}
		items = append(items, item)
	}
	return items, int64(len(items)), nil
}

type gitCommitClientFake struct {
	// mu guards the recorded request state: the manager fetches distinct
	// project/ref combinations from a worker pool, so this fake is called
	// concurrently.
	mu          sync.Mutex
	commits     []gitlab.Commit
	err         error
	calls       int
	lastProject string
	lastRef     string
	lastLimit   int
	lastAsOf    time.Time
	lastAsOfSet bool
	// anchors records the anchor of every call in call order, "" for a plain read,
	// so a test can assert which instant a read was asked for.
	anchors []string
	// serve, when set, answers a call with a window of its own. A test uses it to
	// give an anchored read (the deep read of an old order) a different window than
	// the plain one.
	serve func(limit int, asOf *time.Time) []gitlab.Commit
}

func (f *gitCommitClientFake) ListCommits(_ context.Context, projectPath string, ref string, limit int, asOf *time.Time) ([]gitlab.Commit, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	f.lastProject = projectPath
	f.lastRef = ref
	f.lastLimit = limit
	f.lastAsOf = time.Time{}
	f.lastAsOfSet = asOf != nil
	anchor := ""
	if asOf != nil {
		f.lastAsOf = asOf.UTC()
		anchor = asOf.UTC().Format(time.RFC3339)
	}
	f.anchors = append(f.anchors, anchor)
	if f.err != nil {
		return nil, f.err
	}
	if f.serve != nil {
		return f.serve(limit, asOf), nil
	}
	return f.commits, nil
}

func (f *gitCommitClientFake) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

func (f *gitCommitClientFake) anchorList() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.anchors...)
}

func (f *gitCommitClientFake) setError(err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.err = err
}

func (f *gitCommitClientFake) setServe(serve func(limit int, asOf *time.Time) []gitlab.Commit) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.serve = serve
}

func (f *gitCommitClientFake) setCommits(commits []gitlab.Commit) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.commits = commits
}

// lastRequest snapshots the request state of the most recent call.
func (f *gitCommitClientFake) lastRequest() (string, string, int, *time.Time) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var asOf *time.Time
	if f.lastAsOfSet {
		value := f.lastAsOf
		asOf = &value
	}
	return f.lastProject, f.lastRef, f.lastLimit, asOf
}

type gitCommitFixture struct {
	manager     *GitCommitManager
	orders      *gitCommitReleaseRepoFake
	credentials *gitCommitCredentialRepoFake
	client      *gitCommitClientFake
	// channelCredentials records the credentials the factory was asked to build a
	// fetcher for, in order.
	channelCredentials []gitcredentialdomain.Credential
}

func newGitCommitFixture(t *testing.T, orders map[string]releasedomain.ReleaseOrder, credentials []gitcredentialdomain.Credential, now time.Time) *gitCommitFixture {
	t.Helper()
	orderRepo := &gitCommitReleaseRepoFake{orders: orders}
	applicationRepo := &gitCommitApplicationRepoFake{applications: map[string]applicationdomain.Application{
		"app-1": {ID: "app-1", Name: "fusion-source-web", RepoURL: "http://git.cloud.local:9080/code/bigData/fusion-source-web.git"},
		"app-2": {ID: "app-2", Name: "fusion-source-api", RepoURL: "http://192.168.2.34:9080/code/bigData/fusion-source-api.git"},
	}}
	credentialRepo := &gitCommitCredentialRepoFake{items: credentials}
	client := &gitCommitClientFake{commits: []gitlab.Commit{
		{ID: "sha-old", ShortID: "sha-old", Title: "old", CommittedAt: now.Add(-48 * time.Hour)},
		{ID: "sha-new", ShortID: "sha-new", Title: "new", CommittedAt: now.Add(-2 * time.Hour)},
	}}
	fixture := &gitCommitFixture{
		orders:      orderRepo,
		credentials: credentialRepo,
		client:      client,
	}
	fixture.manager = NewGitCommitManager(orderRepo, applicationRepo, credentialRepo)
	fixture.manager.now = func() time.Time { return now }
	// The fixture replaces the whole credential -> channel selection so the fake
	// serves both auth types.
	fixture.manager.newFetcher = func(credential gitcredentialdomain.Credential) GitCommitFetcher {
		fixture.channelCredentials = append(fixture.channelCredentials, credential)
		return client
	}
	return fixture
}

func gitCommitTestOrder(id string, applicationID string, ref string) releasedomain.ReleaseOrder {
	return releasedomain.ReleaseOrder{
		ID:            id,
		ApplicationID: applicationID,
		GitRef:        ref,
	}
}

// gitCommitTimedOrder is an order with an explicit history anchor: startedAt is
// the execution start (nil for an order that never ran) and createdAt the
// fallback the manager uses then.
func gitCommitTimedOrder(
	id string,
	applicationID string,
	ref string,
	startedAt *time.Time,
	createdAt time.Time,
) releasedomain.ReleaseOrder {
	order := gitCommitTestOrder(id, applicationID, ref)
	order.StartedAt = startedAt
	order.CreatedAt = createdAt
	return order
}

// gitCommitWindow builds a newest-first commit window with fixed descending
// timestamps: window[0] sits at base, window[1] an hour earlier, and so on.
func gitCommitWindow(base time.Time, count int) []gitlab.Commit {
	commits := make([]gitlab.Commit, 0, count)
	for index := 0; index < count; index++ {
		sha := "sha-" + strconv.Itoa(index)
		commits = append(commits, gitlab.Commit{
			ID:          sha,
			ShortID:     sha,
			Title:       sha,
			CommittedAt: base.Add(-time.Duration(index) * time.Hour),
		})
	}
	return commits
}

func gitCommitWindowSHAs(commits []GitCommitInfo) []string {
	shas := make([]string, 0, len(commits))
	for _, commit := range commits {
		shas = append(shas, commit.SHA)
	}
	return shas
}

func gitCommitHostCredential() gitcredentialdomain.Credential {
	return gitcredentialdomain.Credential{
		ID: "gc-cloud", Name: "cloud", Provider: gitcredentialdomain.ProviderGitLab,
		BaseURL: "http://git.cloud.local:9080", Secret: "s",
		AuthType: gitcredentialdomain.AuthTypeToken, Status: gitcredentialdomain.StatusActive,
	}
}

func TestGitCommitManagerResolvesLongestBaseURLPrefix(t *testing.T) {
	now := time.Unix(5000, 0).UTC()
	fixture := newGitCommitFixture(t,
		map[string]releasedomain.ReleaseOrder{
			"ro-1": gitCommitTestOrder("ro-1", "app-1", "main"),
		},
		[]gitcredentialdomain.Credential{
			{ID: "gc-host", Name: "host", Provider: gitcredentialdomain.ProviderGitLab, BaseURL: "http://git.cloud.local:9080", Secret: "host-secret", AuthType: gitcredentialdomain.AuthTypeToken, Status: gitcredentialdomain.StatusActive},
			{ID: "gc-group", Name: "group", Provider: gitcredentialdomain.ProviderGitLab, BaseURL: "http://git.cloud.local:9080/code/bigData", Username: "group-user", Secret: "group-secret", AuthType: gitcredentialdomain.AuthTypePassword, Status: gitcredentialdomain.StatusActive},
			{ID: "gc-disabled", Name: "disabled", Provider: gitcredentialdomain.ProviderGitLab, BaseURL: "http://git.cloud.local:9080/code/bigData/fusion-source-web", Secret: "disabled-secret", AuthType: gitcredentialdomain.AuthTypeToken, Status: gitcredentialdomain.StatusDisabled},
		},
		now,
	)

	result, err := fixture.manager.RecentCommits(context.Background(), GitCommitQuery{OrderIDs: []string{"ro-1"}, Limit: 5})
	if err != nil {
		t.Fatalf("RecentCommits err = %v", err)
	}
	entry, ok := result["ro-1"]
	if !ok {
		t.Fatalf("result = %+v, want an entry for ro-1", result)
	}
	if entry.Error != "" {
		t.Fatalf("entry error = %q", entry.Error)
	}
	// The group credential is longer than the host credential, and the disabled
	// project credential must not be considered at all.
	if entry.CredentialID != "gc-group" || entry.CredentialName != "group" {
		t.Fatalf("credential = %q / %q, want gc-group", entry.CredentialID, entry.CredentialName)
	}
	if entry.Repository != "http://git.cloud.local:9080/code/bigData/fusion-source-web.git" {
		t.Fatalf("Repository = %q", entry.Repository)
	}
	if entry.Provider != string(gitcredentialdomain.ProviderGitLab) {
		t.Fatalf("Provider = %q", entry.Provider)
	}
	if entry.WebURL != "http://git.cloud.local:9080/code/bigData/fusion-source-web" {
		t.Fatalf("WebURL = %q", entry.WebURL)
	}
	if entry.Ref != "main" || entry.ApplicationName != "fusion-source-web" {
		t.Fatalf("entry = %+v", entry)
	}
	if len(fixture.channelCredentials) != 1 {
		t.Fatalf("fetcher factory calls = %d, want 1", len(fixture.channelCredentials))
	}
	channel := fixture.channelCredentials[0]
	if channel.BaseURL != "http://git.cloud.local:9080/code/bigData" || channel.Secret != "group-secret" || channel.AuthType != gitcredentialdomain.AuthTypePassword || channel.Username != "group-user" {
		t.Fatalf("channel credential = %+v", channel)
	}
	if fixture.client.lastProject != "code/bigData/fusion-source-web" {
		t.Fatalf("project path = %q", fixture.client.lastProject)
	}
	if fixture.client.lastRef != "main" {
		t.Fatalf("ref = %q", fixture.client.lastRef)
	}
}

func TestGitCommitManagerSortsCommitsNewestFirstAndHonoursLimit(t *testing.T) {
	now := time.Unix(6000, 0).UTC()
	fixture := newGitCommitFixture(t,
		map[string]releasedomain.ReleaseOrder{"ro-1": gitCommitTestOrder("ro-1", "app-1", "main")},
		[]gitcredentialdomain.Credential{
			{ID: "gc-1", Name: "host", Provider: gitcredentialdomain.ProviderGitLab, BaseURL: "http://git.cloud.local:9080", Secret: "s", AuthType: gitcredentialdomain.AuthTypeToken, Status: gitcredentialdomain.StatusActive},
		},
		now,
	)

	result, err := fixture.manager.RecentCommits(context.Background(), GitCommitQuery{OrderIDs: []string{"ro-1"}, Limit: 1})
	if err != nil {
		t.Fatalf("RecentCommits err = %v", err)
	}
	entry := result["ro-1"]
	if len(entry.Commits) != 1 {
		t.Fatalf("commits = %+v, want the limit to be honoured", entry.Commits)
	}
	if entry.Commits[0].SHA != "sha-new" {
		t.Fatalf("first commit = %q, want the newest one", entry.Commits[0].SHA)
	}
	if fixture.client.lastLimit != 1 {
		t.Fatalf("limit passed to gitlab = %d, want 1", fixture.client.lastLimit)
	}
}

func TestGitCommitManagerAggregatesOrdersAndCachesFetches(t *testing.T) {
	now := time.Unix(7000, 0).UTC()
	fixture := newGitCommitFixture(t,
		map[string]releasedomain.ReleaseOrder{
			"ro-1": gitCommitTestOrder("ro-1", "app-1", "main"),
			"ro-2": gitCommitTestOrder("ro-2", "app-1", "main"),
			"ro-3": gitCommitTestOrder("ro-3", "app-1", "release"),
			"ro-4": gitCommitTestOrder("ro-4", "app-2", "main"),
		},
		[]gitcredentialdomain.Credential{
			{ID: "gc-cloud", Name: "cloud", Provider: gitcredentialdomain.ProviderGitLab, BaseURL: "http://git.cloud.local:9080", Secret: "s", AuthType: gitcredentialdomain.AuthTypeToken, Status: gitcredentialdomain.StatusActive},
			{ID: "gc-ip", Name: "ip", Provider: gitcredentialdomain.ProviderGitLab, BaseURL: "http://192.168.2.34:9080", Secret: "s", AuthType: gitcredentialdomain.AuthTypeToken, Status: gitcredentialdomain.StatusActive},
		},
		now,
	)
	orderIDs := []string{"ro-1", "ro-2", "ro-3", "ro-4"}

	first, err := fixture.manager.RecentCommits(context.Background(), GitCommitQuery{OrderIDs: orderIDs, Limit: 5})
	if err != nil {
		t.Fatalf("RecentCommits err = %v", err)
	}
	if len(first) != 4 {
		t.Fatalf("result len = %d, want 4", len(first))
	}
	// Three distinct project/ref/credential combinations: ro-1 and ro-2 share the
	// main branch of one project.
	if fixture.client.callCount() != 3 {
		t.Fatalf("gitlab calls = %d, want 3", fixture.client.callCount())
	}
	if len(first["ro-1"].Commits) != 2 || first["ro-1"].Error != "" {
		t.Fatalf("ro-1 entry = %+v", first["ro-1"])
	}
	if first["ro-4"].CredentialID != "gc-ip" {
		t.Fatalf("ro-4 credential = %q, want gc-ip", first["ro-4"].CredentialID)
	}

	// A second poll within the commit TTL is served from the cache.
	second, err := fixture.manager.RecentCommits(context.Background(), GitCommitQuery{OrderIDs: orderIDs, Limit: 5})
	if err != nil {
		t.Fatalf("second RecentCommits err = %v", err)
	}
	if fixture.client.callCount() != 3 {
		t.Fatalf("gitlab calls after the cached poll = %d, want 3", fixture.client.callCount())
	}
	if len(second["ro-2"].Commits) != 2 {
		t.Fatalf("cached ro-2 entry = %+v", second["ro-2"])
	}
	// The enabled credential query is cached as well.
	if fixture.credentials.calls != 1 {
		t.Fatalf("credential list calls = %d, want 1", fixture.credentials.calls)
	}

	// Past the commit TTL the next poll refreshes from GitLab.
	fixture.manager.now = func() time.Time { return now.Add(2 * time.Minute) }
	if _, err := fixture.manager.RecentCommits(context.Background(), GitCommitQuery{OrderIDs: orderIDs, Limit: 5}); err != nil {
		t.Fatalf("third RecentCommits err = %v", err)
	}
	if fixture.client.callCount() != 6 {
		t.Fatalf("gitlab calls after the TTL expired = %d, want 6", fixture.client.callCount())
	}
	if fixture.credentials.calls != 2 {
		t.Fatalf("credential list calls after the TTL expired = %d, want 2", fixture.credentials.calls)
	}
}

func TestGitCommitManagerReportsUnusableOrders(t *testing.T) {
	now := time.Unix(8000, 0).UTC()
	orderRepo := &gitCommitReleaseRepoFake{orders: map[string]releasedomain.ReleaseOrder{
		"ro-1": gitCommitTestOrder("ro-1", "app-missing", "main"),
		"ro-2": gitCommitTestOrder("ro-2", "app-no-repo", "main"),
		"ro-3": gitCommitTestOrder("ro-3", "app-1", "main"),
	}}
	applicationRepo := &gitCommitApplicationRepoFake{applications: map[string]applicationdomain.Application{
		"app-no-repo": {ID: "app-no-repo", Name: "no-repo"},
		"app-1":       {ID: "app-1", Name: "fusion-source-web", RepoURL: "http://git.other.local:9080/code/app.git"},
	}}
	credentialRepo := &gitCommitCredentialRepoFake{items: []gitcredentialdomain.Credential{
		{ID: "gc-cloud", Name: "cloud", Provider: gitcredentialdomain.ProviderGitLab, BaseURL: "http://git.cloud.local:9080", Secret: "s", AuthType: gitcredentialdomain.AuthTypeToken, Status: gitcredentialdomain.StatusActive},
	}}
	manager := NewGitCommitManager(orderRepo, applicationRepo, credentialRepo)
	manager.now = func() time.Time { return now }
	client := &gitCommitClientFake{}
	manager.newFetcher = func(gitcredentialdomain.Credential) GitCommitFetcher { return client }

	result, err := manager.RecentCommits(context.Background(), GitCommitQuery{
		OrderIDs: []string{"ro-1", "ro-2", "ro-3", "ro-missing"},
		Limit:    5,
	})
	if err != nil {
		t.Fatalf("RecentCommits err = %v", err)
	}
	if got := result["ro-1"].Error; !strings.Contains(got, "应用不存在") {
		t.Fatalf("ro-1 error = %q", got)
	}
	if got := result["ro-2"].Error; !strings.Contains(got, "未配置 Git 仓库地址") {
		t.Fatalf("ro-2 error = %q", got)
	}
	if got := result["ro-3"].Error; !strings.Contains(got, "未找到匹配的 Git 凭证") {
		t.Fatalf("ro-3 error = %q", got)
	}
	if got := result["ro-missing"].Error; !strings.Contains(got, "发布单不存在") {
		t.Fatalf("ro-missing error = %q", got)
	}
	if client.callCount() != 0 {
		t.Fatalf("gitlab calls = %d, want 0", client.calls)
	}
	for _, entry := range result {
		if entry.Commits == nil {
			t.Fatalf("entry %q has a nil commit list, want an empty one", entry.OrderID)
		}
	}
}

func TestGitCommitManagerSurfacesGitLabFailures(t *testing.T) {
	now := time.Unix(9000, 0).UTC()
	fixture := newGitCommitFixture(t,
		map[string]releasedomain.ReleaseOrder{"ro-1": gitCommitTestOrder("ro-1", "app-1", "main")},
		[]gitcredentialdomain.Credential{
			{ID: "gc-1", Name: "host", Provider: gitcredentialdomain.ProviderGitLab, BaseURL: "http://git.cloud.local:9080", Secret: "s", AuthType: gitcredentialdomain.AuthTypeToken, Status: gitcredentialdomain.StatusActive},
		},
		now,
	)
	fixture.client.setError(gitlab.ErrUnauthorized)

	result, err := fixture.manager.RecentCommits(context.Background(), GitCommitQuery{OrderIDs: []string{"ro-1"}, Limit: 5})
	if err != nil {
		t.Fatalf("RecentCommits err = %v", err)
	}
	entry := result["ro-1"]
	if entry.Error == "" || !strings.Contains(entry.Error, "访问令牌") {
		t.Fatalf("entry error = %q, want the access token hint", entry.Error)
	}
	if len(entry.Commits) != 0 {
		t.Fatalf("commits = %+v, want none", entry.Commits)
	}
	// Failures are not cached: a fixed credential is picked up on the next poll.
	if _, err := fixture.manager.RecentCommits(context.Background(), GitCommitQuery{OrderIDs: []string{"ro-1"}, Limit: 5}); err != nil {
		t.Fatalf("second RecentCommits err = %v", err)
	}
	if fixture.client.callCount() != 2 {
		t.Fatalf("gitlab calls = %d, want 2", fixture.client.callCount())
	}
}

// TestGitCommitManagerSelectsChannelByAuthType covers the production factory:
// a token credential keeps using the GitLab REST API, a password credential is
// read over the git protocol instead (GitLab API v4 rejects an account password
// with 401, while the same account can clone the repository).
func TestGitCommitManagerSelectsChannelByAuthType(t *testing.T) {
	now := time.Unix(12000, 0).UTC()
	orderRepo := &gitCommitReleaseRepoFake{orders: map[string]releasedomain.ReleaseOrder{
		"ro-password": gitCommitTestOrder("ro-password", "app-1", "main"),
		"ro-token":    gitCommitTestOrder("ro-token", "app-2", "main"),
	}}
	applicationRepo := &gitCommitApplicationRepoFake{applications: map[string]applicationdomain.Application{
		"app-1": {ID: "app-1", Name: "fusion-source-web", RepoURL: "http://git.cloud.local:9080/code/bigData/fusion-source-web.git"},
		"app-2": {ID: "app-2", Name: "fusion-source-api", RepoURL: "http://192.168.2.34:9080/code/bigData/fusion-source-api.git"},
	}}
	credentialRepo := &gitCommitCredentialRepoFake{items: []gitcredentialdomain.Credential{
		{ID: "gc-password", Name: "账号密码", Provider: gitcredentialdomain.ProviderGitLab, BaseURL: "http://git.cloud.local:9080", Username: "client011", Secret: "p@ss w0rd", AuthType: gitcredentialdomain.AuthTypePassword, Status: gitcredentialdomain.StatusActive},
		{ID: "gc-token", Name: "访问令牌", Provider: gitcredentialdomain.ProviderGitLab, BaseURL: "http://192.168.2.34:9080", Username: "client011", Secret: "glpat-1", AuthType: gitcredentialdomain.AuthTypeToken, Status: gitcredentialdomain.StatusActive},
	}}

	manager := NewGitCommitManager(orderRepo, applicationRepo, credentialRepo)
	manager.now = func() time.Time { return now }
	gitLabFetcher := &gitCommitClientFake{commits: []gitlab.Commit{{ID: "sha-api", ShortID: "sha-api", Title: "api", CommittedAt: now}}}
	gitCLIFetcher := &gitCommitClientFake{commits: []gitlab.Commit{{ID: "sha-git", ShortID: "sha-git", Title: "git", CommittedAt: now}}}
	var gitLabConfigs []gitlab.Config
	var gitCLIConfigs []gitcli.Config
	manager.newGitLabClient = func(cfg gitlab.Config) GitCommitFetcher {
		gitLabConfigs = append(gitLabConfigs, cfg)
		return gitLabFetcher
	}
	manager.newGitCLIClient = func(cfg gitcli.Config) GitCommitFetcher {
		gitCLIConfigs = append(gitCLIConfigs, cfg)
		return gitCLIFetcher
	}

	result, err := manager.RecentCommits(context.Background(), GitCommitQuery{
		OrderIDs: []string{"ro-password", "ro-token"},
		Limit:    5,
	})
	if err != nil {
		t.Fatalf("RecentCommits err = %v", err)
	}

	if len(gitCLIConfigs) != 1 {
		t.Fatalf("git cli clients = %d, want the password credential to use the git protocol", len(gitCLIConfigs))
	}
	if config := gitCLIConfigs[0]; config.BaseURL != "http://git.cloud.local:9080" || config.Username != "client011" || config.Secret != "p@ss w0rd" {
		t.Fatalf("git cli config = %+v", config)
	}
	if len(gitLabConfigs) != 1 {
		t.Fatalf("gitlab clients = %d, want the token credential to use the API", len(gitLabConfigs))
	}
	if config := gitLabConfigs[0]; config.BaseURL != "http://192.168.2.34:9080" || config.Secret != "glpat-1" || config.AuthType != "token" {
		t.Fatalf("gitlab config = %+v", config)
	}

	passwordEntry := result["ro-password"]
	if passwordEntry.Error != "" || len(passwordEntry.Commits) != 1 || passwordEntry.Commits[0].SHA != "sha-git" {
		t.Fatalf("ro-password entry = %+v", passwordEntry)
	}
	if passwordEntry.CredentialID != "gc-password" {
		t.Fatalf("ro-password credential = %q", passwordEntry.CredentialID)
	}
	tokenEntry := result["ro-token"]
	if tokenEntry.Error != "" || len(tokenEntry.Commits) != 1 || tokenEntry.Commits[0].SHA != "sha-api" {
		t.Fatalf("ro-token entry = %+v", tokenEntry)
	}
	if gitCLIFetcher.callCount() != 1 || gitLabFetcher.callCount() != 1 {
		t.Fatalf("channel calls = git %d / gitlab %d, want 1 each", gitCLIFetcher.callCount(), gitLabFetcher.callCount())
	}
	if gitCLIFetcher.lastProject != "code/bigData/fusion-source-web" {
		t.Fatalf("git cli project = %q", gitCLIFetcher.lastProject)
	}
}

func TestGitCommitManagerDropsInvisibleApplicationsBeforeFetching(t *testing.T) {
	now := time.Unix(10000, 0).UTC()
	fixture := newGitCommitFixture(t,
		map[string]releasedomain.ReleaseOrder{
			"ro-1": gitCommitTestOrder("ro-1", "app-1", "main"),
			"ro-2": gitCommitTestOrder("ro-2", "app-2", "main"),
		},
		[]gitcredentialdomain.Credential{
			{ID: "gc-cloud", Name: "cloud", Provider: gitcredentialdomain.ProviderGitLab, BaseURL: "http://git.cloud.local:9080", Secret: "s", AuthType: gitcredentialdomain.AuthTypeToken, Status: gitcredentialdomain.StatusActive},
			{ID: "gc-ip", Name: "ip", Provider: gitcredentialdomain.ProviderGitLab, BaseURL: "http://192.168.2.34:9080", Secret: "s", AuthType: gitcredentialdomain.AuthTypeToken, Status: gitcredentialdomain.StatusActive},
		},
		now,
	)

	result, err := fixture.manager.RecentCommits(context.Background(), GitCommitQuery{
		OrderIDs:              []string{"ro-1", "ro-2"},
		Limit:                 5,
		VisibleApplicationIDs: []string{"app-1"},
	})
	if err != nil {
		t.Fatalf("RecentCommits err = %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("result = %+v, want only the visible order", result)
	}
	if _, ok := result["ro-1"]; !ok {
		t.Fatalf("result = %+v, want ro-1", result)
	}
	if fixture.client.callCount() != 1 {
		t.Fatalf("gitlab calls = %d, want only the visible order to be fetched", fixture.client.callCount())
	}

	empty, err := fixture.manager.RecentCommits(context.Background(), GitCommitQuery{
		OrderIDs:              []string{"ro-1", "ro-2"},
		Limit:                 5,
		VisibleApplicationIDs: []string{},
	})
	if err != nil {
		t.Fatalf("RecentCommits err = %v", err)
	}
	if len(empty) != 0 {
		t.Fatalf("result = %+v, want nothing for an empty visibility set", empty)
	}

	unrestricted, err := fixture.manager.RecentCommits(context.Background(), GitCommitQuery{OrderIDs: []string{"ro-1", "ro-2"}, Limit: 5})
	if err != nil {
		t.Fatalf("RecentCommits err = %v", err)
	}
	if len(unrestricted) != 2 {
		t.Fatalf("result len = %d, want both orders for a nil visibility set", len(unrestricted))
	}
}

func TestNormalizeGitCommitOrderIDsDeduplicatesAndCaps(t *testing.T) {
	got := normalizeGitCommitOrderIDs([]string{" ro-1 ", "ro-1", "", "ro-2"})
	if len(got) != 2 || got[0] != "ro-1" || got[1] != "ro-2" {
		t.Fatalf("normalizeGitCommitOrderIDs = %+v", got)
	}

	many := make([]string, 0, gitCommitMaxOrders+10)
	for index := 0; index < gitCommitMaxOrders+10; index++ {
		many = append(many, "ro-"+strings.Repeat("x", index%5)+string(rune('a'+index%26))+string(rune('0'+index%10))+string(rune('a'+(index/10)%26)))
	}
	if len(normalizeGitCommitOrderIDs(many)) > gitCommitMaxOrders {
		t.Fatalf("normalizeGitCommitOrderIDs did not cap the request")
	}
}

func TestGitCommitManagerPropagatesInfrastructureErrors(t *testing.T) {
	now := time.Unix(11000, 0).UTC()
	databaseDown := errors.New("database is down")
	orderRepo := &gitCommitReleaseRepoFake{orders: map[string]releasedomain.ReleaseOrder{}, err: databaseDown}
	manager := NewGitCommitManager(orderRepo, &gitCommitApplicationRepoFake{}, &gitCommitCredentialRepoFake{})
	manager.now = func() time.Time { return now }

	// A missing order is a readable per-order error, but a broken dependency must
	// not be reported as "no commits": it has to surface as a failed request.
	_, err := manager.RecentCommits(context.Background(), GitCommitQuery{OrderIDs: []string{"ro-1"}, Limit: 5})
	if !errors.Is(err, databaseDown) {
		t.Fatalf("RecentCommits err = %v, want the repository error", err)
	}

	credentialRepo := &gitCommitCredentialRepoFake{err: databaseDown}
	manager = NewGitCommitManager(
		&gitCommitReleaseRepoFake{orders: map[string]releasedomain.ReleaseOrder{}},
		&gitCommitApplicationRepoFake{},
		credentialRepo,
	)
	manager.now = func() time.Time { return now }
	if _, err := manager.RecentCommits(context.Background(), GitCommitQuery{OrderIDs: []string{"ro-1"}, Limit: 5}); !errors.Is(err, databaseDown) {
		t.Fatalf("RecentCommits err = %v, want the credential repository error", err)
	}
}

// TestGitCommitManagerReadsEveryOrderAtItsOwnAnchor covers the core of the
// as-of lookup: each order shows the branch head of its own execution start, all
// three anchors are served from the single window read of the repository, and a
// poll within the TTL reuses that window.
func TestGitCommitManagerReadsEveryOrderAtItsOwnAnchor(t *testing.T) {
	now := time.Unix(20000, 0).UTC()
	startedRecent := now.Add(-2*time.Hour - 30*time.Minute)
	startedExact := now.Add(-4 * time.Hour)
	createdOnly := now.Add(-4*time.Hour - 30*time.Minute)
	fixture := newGitCommitFixture(t,
		map[string]releasedomain.ReleaseOrder{
			// ro-recent ran half an hour after the third commit: sha-3 is its head.
			"ro-recent": gitCommitTimedOrder("ro-recent", "app-1", "main", &startedRecent, now.Add(-6*time.Hour)),
			// ro-exact ran exactly at the timestamp of sha-4, which has to be included.
			"ro-exact": gitCommitTimedOrder("ro-exact", "app-1", "main", &startedExact, now.Add(-6*time.Hour)),
			// ro-created never ran, so its anchor is the creation time.
			"ro-created": gitCommitTimedOrder("ro-created", "app-1", "main", nil, createdOnly),
		},
		[]gitcredentialdomain.Credential{gitCommitHostCredential()},
		now,
	)
	fixture.client.setCommits(gitCommitWindow(now, 6))

	result, err := fixture.manager.RecentCommits(context.Background(), GitCommitQuery{
		OrderIDs: []string{"ro-recent", "ro-exact", "ro-created"},
		Limit:    3,
	})
	if err != nil {
		t.Fatalf("RecentCommits err = %v", err)
	}

	recent := result["ro-recent"]
	if got := gitCommitWindowSHAs(recent.Commits); len(got) != 3 || got[0] != "sha-3" || got[2] != "sha-5" {
		t.Fatalf("ro-recent commits = %+v, want the history of sha-3 at the execution time", recent.Commits)
	}
	if recent.AsOf != startedRecent.UTC().Format(time.RFC3339) {
		t.Fatalf("ro-recent as_of = %q, want %q", recent.AsOf, startedRecent.UTC().Format(time.RFC3339))
	}
	exact := result["ro-exact"]
	if got := gitCommitWindowSHAs(exact.Commits); len(got) != 2 || got[0] != "sha-4" || got[1] != "sha-5" {
		t.Fatalf("ro-exact commits = %+v, want the anchor commit included", exact.Commits)
	}
	created := result["ro-created"]
	if got := gitCommitWindowSHAs(created.Commits); len(got) != 1 || got[0] != "sha-5" {
		t.Fatalf("ro-created commits = %+v, want the creation time to be the anchor", created.Commits)
	}
	if created.AsOf != createdOnly.UTC().Format(time.RFC3339) {
		t.Fatalf("ro-created as_of = %q, want the creation time", created.AsOf)
	}

	// Three anchors, one repository read: the shared window is the plain newest
	// history and every order filters it locally, so the read does not depend on
	// any single anchor.
	if fixture.client.callCount() != 1 {
		t.Fatalf("fetcher calls = %d, want one shared window read", fixture.client.callCount())
	}
	project, ref, limit, asOf := fixture.client.lastRequest()
	if project != "code/bigData/fusion-source-web" || ref != "main" {
		t.Fatalf("window read = %q / %q", project, ref)
	}
	if limit != gitCommitWindowSize {
		t.Fatalf("window limit = %d, want %d", limit, gitCommitWindowSize)
	}
	if asOf != nil {
		t.Fatalf("window read asOf = %v, want the shared window to be read without an anchor", asOf)
	}

	// A second poll inside the commit TTL is served from the cache.
	if _, err := fixture.manager.RecentCommits(context.Background(), GitCommitQuery{
		OrderIDs: []string{"ro-recent", "ro-exact", "ro-created"},
		Limit:    3,
	}); err != nil {
		t.Fatalf("second RecentCommits err = %v", err)
	}
	if fixture.client.callCount() != 1 {
		t.Fatalf("fetcher calls after a cached poll = %d, want 1", fixture.client.callCount())
	}
}

// TestGitCommitManagerKeepsTheNewestReadWithoutAnAnchor guards the behaviour the
// lookup had before the as-of filtering: an order without any timestamp, and a
// query that does not force one either, keeps reading the newest commits.
func TestGitCommitManagerKeepsTheNewestReadWithoutAnAnchor(t *testing.T) {
	now := time.Unix(22000, 0).UTC()
	fixture := newGitCommitFixture(t,
		map[string]releasedomain.ReleaseOrder{
			"ro-plain": gitCommitTestOrder("ro-plain", "app-1", "main"),
		},
		[]gitcredentialdomain.Credential{gitCommitHostCredential()},
		now,
	)
	fixture.client.setCommits(gitCommitWindow(now, 6))

	result, err := fixture.manager.RecentCommits(context.Background(), GitCommitQuery{OrderIDs: []string{"ro-plain"}, Limit: 2})
	if err != nil {
		t.Fatalf("RecentCommits err = %v", err)
	}
	entry := result["ro-plain"]
	if entry.AsOf != "" {
		t.Fatalf("as_of = %q, want empty for a lookup that is not anchored in time", entry.AsOf)
	}
	if got := gitCommitWindowSHAs(entry.Commits); len(got) != 2 || got[0] != "sha-0" || got[1] != "sha-1" {
		t.Fatalf("commits = %+v, want the newest history", entry.Commits)
	}
	_, _, limit, asOf := fixture.client.lastRequest()
	if asOf != nil || limit != 2 {
		t.Fatalf("read = limit %d asOf %v, want the plain newest read", limit, asOf)
	}

	// A query level anchor overrides the per-order timestamps: every order of the
	// request is then read at that one instant, out of the shared window.
	forced := now.Add(-3*time.Hour - 30*time.Minute)
	forcedResult, err := fixture.manager.RecentCommits(context.Background(), GitCommitQuery{
		OrderIDs: []string{"ro-plain"},
		Limit:    2,
		AsOf:     &forced,
	})
	if err != nil {
		t.Fatalf("forced RecentCommits err = %v", err)
	}
	forcedEntry := forcedResult["ro-plain"]
	if forcedEntry.AsOf != forced.UTC().Format(time.RFC3339) {
		t.Fatalf("forced as_of = %q, want %q", forcedEntry.AsOf, forced.UTC().Format(time.RFC3339))
	}
	if got := gitCommitWindowSHAs(forcedEntry.Commits); len(got) != 2 || got[0] != "sha-4" || got[1] != "sha-5" {
		t.Fatalf("forced commits = %+v, want the window filtered at the forced instant", forcedEntry.Commits)
	}
}

// TestGitCommitManagerDeepensHistoryBehindTheWindow covers an order whose release
// instant sits behind the shared window: that order gets its own read anchored at
// its instant, and the read is cached for the next poll.
func TestGitCommitManagerDeepensHistoryBehindTheWindow(t *testing.T) {
	now := time.Unix(23000, 0).UTC()
	recentAnchor := now.Add(-1 * time.Hour)
	deepAnchor := now.Add(-400 * time.Hour)
	fixture := newGitCommitFixture(t,
		map[string]releasedomain.ReleaseOrder{
			"ro-old":    gitCommitTimedOrder("ro-old", "app-1", "main", &deepAnchor, now),
			"ro-recent": gitCommitTimedOrder("ro-recent", "app-1", "main", &recentAnchor, now),
		},
		[]gitcredentialdomain.Credential{gitCommitHostCredential()},
		now,
	)
	window := gitCommitWindow(now, 3)
	deepWindow := gitCommitWindow(deepAnchor, 3)
	fixture.client.setServe(func(_ int, asOf *time.Time) []gitlab.Commit {
		if asOf == nil {
			return window
		}
		return deepWindow
	})

	result, err := fixture.manager.RecentCommits(context.Background(), GitCommitQuery{
		OrderIDs: []string{"ro-old", "ro-recent"},
		Limit:    2,
	})
	if err != nil {
		t.Fatalf("RecentCommits err = %v", err)
	}
	old := result["ro-old"]
	if old.Error != "" {
		t.Fatalf("ro-old error = %q", old.Error)
	}
	if got := gitCommitWindowSHAs(old.Commits); len(got) != 2 || got[0] != "sha-0" || got[1] != "sha-1" {
		t.Fatalf("ro-old commits = %+v, want the window of its own anchor", old.Commits)
	}
	if old.AsOf != deepAnchor.UTC().Format(time.RFC3339) {
		t.Fatalf("ro-old as_of = %q", old.AsOf)
	}
	recent := result["ro-recent"]
	if got := gitCommitWindowSHAs(recent.Commits); len(got) != 2 || got[0] != "sha-1" || got[1] != "sha-2" {
		t.Fatalf("ro-recent commits = %+v, want the shared window filtered at its own anchor", recent.Commits)
	}
	// The order behind the window asked for its own anchor, the shared window read
	// did not.
	anchors := fixture.client.anchorList()
	if len(anchors) != 2 {
		t.Fatalf("fetcher calls = %d, want the shared window plus one deep read", len(anchors))
	}
	found := false
	for _, anchor := range anchors {
		if anchor == deepAnchor.UTC().Format(time.RFC3339) {
			found = true
		}
	}
	if !found {
		t.Fatalf("anchors = %+v, want the deep read of ro-old anchored at %v", anchors, deepAnchor)
	}

	// Both reads are cached: the next poll of the TTL window does not touch the
	// remote again.
	if _, err := fixture.manager.RecentCommits(context.Background(), GitCommitQuery{
		OrderIDs: []string{"ro-old", "ro-recent"},
		Limit:    2,
	}); err != nil {
		t.Fatalf("second RecentCommits err = %v", err)
	}
	if got := fixture.client.callCount(); got != 2 {
		t.Fatalf("fetcher calls after a cached poll = %d, want 2", got)
	}
}

// TestGitCommitManagerReportsHistoryThatDoesNotReachTheAnchor covers the failure
// the deeper read can run into: the repository history ends after the release
// instant, so no commit can be reported as its head.
func TestGitCommitManagerReportsHistoryThatDoesNotReachTheAnchor(t *testing.T) {
	now := time.Unix(24000, 0).UTC()
	deepAnchor := now.Add(-4000 * time.Hour)
	fixture := newGitCommitFixture(t,
		map[string]releasedomain.ReleaseOrder{
			"ro-ancient": gitCommitTimedOrder("ro-ancient", "app-1", "main", &deepAnchor, now),
		},
		[]gitcredentialdomain.Credential{gitCommitHostCredential()},
		now,
	)
	window := gitCommitWindow(now, 3)
	// The channel cannot reach the anchor either and says so by returning no commit
	// at or before it.
	fixture.client.setServe(func(_ int, asOf *time.Time) []gitlab.Commit {
		if asOf == nil {
			return window
		}
		return nil
	})

	result, err := fixture.manager.RecentCommits(context.Background(), GitCommitQuery{
		OrderIDs: []string{"ro-ancient"},
		Limit:    5,
	})
	if err != nil {
		t.Fatalf("RecentCommits err = %v", err)
	}
	entry := result["ro-ancient"]
	if !strings.Contains(entry.Error, "提交历史不足") {
		t.Fatalf("error = %q, want the shallow history hint", entry.Error)
	}
	if len(entry.Commits) != 0 {
		t.Fatalf("commits = %+v, want none instead of commits newer than the anchor", entry.Commits)
	}
	if entry.AsOf != deepAnchor.UTC().Format(time.RFC3339) {
		t.Fatalf("as_of = %q, want the anchor even when the history is too shallow", entry.AsOf)
	}
}

// TestDescribeGitLabFailureCoversShallowHistory keeps the two channels' shallow
// history sentinel readable for the release order UI.
func TestDescribeGitLabFailureCoversShallowHistory(t *testing.T) {
	wrapped := fmt.Errorf("读取提交失败：%w", gitlab.ErrHistoryTooShallow)
	if got := describeGitLabFailure(gitlab.ErrHistoryTooShallow); got != gitlab.ErrHistoryTooShallow.Error() {
		t.Fatalf("describeGitLabFailure = %q", got)
	}
	if got := describeGitLabFailure(wrapped); got != gitlab.ErrHistoryTooShallow.Error() {
		t.Fatalf("describeGitLabFailure of a wrapped error = %q", got)
	}
}
