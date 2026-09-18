package usecase

import (
	"context"
	"errors"
	"sort"
	"strings"
	"sync"
	"time"

	applicationdomain "gos/internal/domain/application"
	gitcredentialdomain "gos/internal/domain/gitcredential"
	releasedomain "gos/internal/domain/release"
	"gos/internal/infrastructure/gitcli"
	"gos/internal/infrastructure/gitlab"
)

const (
	gitCommitDefaultLimit = 5
	gitCommitMaxLimit     = 20
	gitCommitMaxOrders    = 50
	// gitCommitWindowSize is the history window one repository read pulls when a
	// lookup is anchored in time. Orders of the same repository/ref all filter
	// that one window at their own instant, so a page listing 50 orders of one
	// branch still costs a single remote read. The value matches the commit
	// ceiling of both channels (GitLab per_page, git CLI maxCommits).
	gitCommitWindowSize = 100
	// gitCommitCacheMaxEntries bounds the commit cache before expired entries are
	// swept. The key space is open ended because an anchored read that has to go
	// deeper than the window is cached per anchor instant.
	gitCommitCacheMaxEntries = 512
	// gitCommitCredentialCacheTTL hides the enabled-credential query from the 10s
	// release-order list poll.
	gitCommitCredentialCacheTTL = 30 * time.Second
	// gitCommitCacheTTL collapses the per-order lookups of one poll into a single
	// GitLab call per project/ref, which matters when a page lists 50 orders that
	// share a handful of repositories.
	gitCommitCacheTTL           = 60 * time.Second
	gitCommitFetchWorker        = 4
	gitCommitCredentialPage     = 100
	gitCommitCredentialMaxPages = 20
)

// GitCommitFetcher is the commit read the release order lookup needs. Declaring
// the interface here keeps the use case testable with a fake, and both channels
// implement it: the GitLab REST client (token credentials) and the git CLI
// client (password credentials, which the target GitLab rejects on API v4).
//
// asOf is the instant the read is anchored at: nil asks for the newest `limit`
// commits, a non-nil instant asks for the commits at or before it, newest first.
type GitCommitFetcher interface {
	ListCommits(ctx context.Context, projectPath string, ref string, limit int, asOf *time.Time) ([]gitlab.Commit, error)
}

// GitCommitFetcherFactory builds the fetcher of one credential. It is the seam a
// test replaces to assert which channel a credential type selects.
type GitCommitFetcherFactory func(credential gitcredentialdomain.Credential) GitCommitFetcher

// GitCommitInfo is one commit as the release order UI shows it.
type GitCommitInfo struct {
	SHA         string    `json:"sha"`
	ShortSHA    string    `json:"short_sha"`
	Title       string    `json:"title"`
	Message     string    `json:"message"`
	AuthorName  string    `json:"author_name"`
	AuthorEmail string    `json:"author_email"`
	CommittedAt time.Time `json:"committed_at"`
	WebURL      string    `json:"web_url"`
}

// ReleaseOrderRecentCommits is the per-order result of the recent commit
// lookup. Error is filled instead of Commits when the order cannot be resolved
// (no matching credential, unreachable GitLab, ...) so one broken repository
// never blanks out the whole list.
type ReleaseOrderRecentCommits struct {
	OrderID         string `json:"order_id"`
	ApplicationID   string `json:"application_id"`
	ApplicationName string `json:"application_name"`
	Repository      string `json:"repository"`
	Provider        string `json:"provider"`
	Ref             string `json:"ref"`
	WebURL          string `json:"web_url"`
	CredentialID    string `json:"credential_id"`
	CredentialName  string `json:"credential_name"`
	// AsOf is the instant the commits are read at: the moment the release started
	// executing, or the creation time of an order that never ran. Commits holds
	// the branch head at that instant and the commits behind it. An empty value
	// means the lookup was not anchored in time and Commits is simply the newest
	// history.
	AsOf    string          `json:"as_of"`
	Commits []GitCommitInfo `json:"commits"`
	Error   string          `json:"error"`
}

// GitCommitManager resolves the recent commits of a set of release orders by
// combining the application repository URL with the Git credential whose base
// URL is its longest prefix.
type GitCommitManager struct {
	orders       releasedomain.Repository
	applications applicationdomain.Repository
	credentials  gitcredentialdomain.Repository
	// newFetcher is the whole credential -> channel selection. It is nil unless a
	// test (or a caller that needs another provider) overrides it.
	newFetcher GitCommitFetcherFactory
	// newGitCLIClient and newGitLabClient are the two channels the default
	// selection picks from. They are fields so the server can hand in the git CLI
	// client it built and tests can watch or replace either channel.
	newGitCLIClient func(cfg gitcli.Config) GitCommitFetcher
	newGitLabClient func(cfg gitlab.Config) GitCommitFetcher
	now             func() time.Time

	credentialTTL time.Duration
	commitTTL     time.Duration

	// mu guards only the two caches below; the repository reads happen outside the lock.
	mu              sync.Mutex
	credentialCache cachedGitCredentials
	commitCache     map[string]cachedGitCommits
}

type cachedGitCredentials struct {
	valid     bool
	items     []gitcredentialdomain.Credential
	expiresAt time.Time
}

type cachedGitCommits struct {
	commits   []GitCommitInfo
	expiresAt time.Time
}

// gitCommitFetchPlan is one unique repository read: several orders sharing a
// repository and ref collapse into a single entry keyed by project/ref/credential.
type gitCommitFetchPlan struct {
	fetcher     GitCommitFetcher
	projectPath string
	ref         string
	// credentialID completes the cache identity of the read. The credential is
	// part of the key so switching a repository to another credential cannot serve
	// data fetched with the previous one.
	credentialID string
	// limit is what the channel is asked for: the whole history window for an
	// anchored read (the orders filter that window locally), the display limit
	// otherwise.
	limit int
	// asOf anchors the read when the shared window turned out to be too young for
	// the order; nil reads the newest history.
	asOf *time.Time
}

type gitCommitFetchOutcome struct {
	commits    []GitCommitInfo
	errMessage string
}

// NewGitCommitManager 创建并返回对应组件实例。
func NewGitCommitManager(
	orders releasedomain.Repository,
	applications applicationdomain.Repository,
	credentials gitcredentialdomain.Repository,
) *GitCommitManager {
	return &GitCommitManager{
		orders:       orders,
		applications: applications,
		credentials:  credentials,
		newGitLabClient: func(cfg gitlab.Config) GitCommitFetcher {
			return gitlab.NewClient(cfg)
		},
		newGitCLIClient: func(cfg gitcli.Config) GitCommitFetcher {
			return gitcli.NewClient(cfg)
		},
		now: func() time.Time {
			return time.Now().UTC()
		},
		credentialTTL: gitCommitCredentialCacheTTL,
		commitTTL:     gitCommitCacheTTL,
		commitCache:   make(map[string]cachedGitCommits),
	}
}

// SetGitCLIClientFactory rebinds the channel used for password credentials. The
// server calls it at wiring time so the git protocol dependency (and its cache
// directory) is visible in main.go; the default already builds the same client,
// which keeps the manager usable without any wiring.
func (uc *GitCommitManager) SetGitCLIClientFactory(factory func(cfg gitcli.Config) GitCommitFetcher) {
	if factory == nil {
		return
	}
	uc.newGitCLIClient = factory
}

// fetcherFor selects the read channel of one credential: an explicit override
// first, then the production selection by auth_type.
//
// A "token" credential keeps using GitLab API v4. A "password" credential has to
// go through the git protocol instead: the measured behaviour of the target
// GitLab is that API v4 answers 401 both for a basic account password and for a
// "PRIVATE-TOKEN: <password>" header, while the same account can clone and fetch
// the repository over git.
func (uc *GitCommitManager) fetcherFor(credential gitcredentialdomain.Credential) GitCommitFetcher {
	if uc.newFetcher != nil {
		return uc.newFetcher(credential)
	}
	if credential.AuthType == gitcredentialdomain.AuthTypePassword {
		return uc.newGitCLIClient(gitcli.Config{
			BaseURL:  credential.BaseURL,
			Username: credential.Username,
			Secret:   credential.Secret,
		})
	}
	return uc.newGitLabClient(gitlab.Config{
		BaseURL:  credential.BaseURL,
		Username: credential.Username,
		Secret:   credential.Secret,
		AuthType: string(credential.AuthType),
	})
}

// GitCommitQuery is the request shape of RecentCommits.
type GitCommitQuery struct {
	OrderIDs []string
	Limit    int
	// AsOf, when set, anchors every order at that instant instead of the order's
	// own execution/creation time. It is an escape hatch for a caller that wants
	// one history point (and for tests); the release order views leave it nil so
	// each order is read at the instant it actually ran.
	AsOf *time.Time
	// VisibleApplicationIDs, when non-nil, restricts the lookup to applications
	// the caller may see: orders outside the set are dropped before any GitLab
	// call. A nil slice means "no restriction", which is what an administrator
	// gets; an empty non-nil slice means "nothing is visible".
	VisibleApplicationIDs []string
}

// gitOrderPlan is the per-order intermediate result: the response entry that is
// always echoed back plus the unique repository read it needs, if any.
type gitOrderPlan struct {
	entry ReleaseOrderRecentCommits
	// fetchKey identifies the read that answers this order. It points at the
	// shared window first and is repointed at the order's own deep read when the
	// window does not reach its anchor. It is empty when the entry already carries
	// a terminal error, so no fetch is needed.
	fetchKey string
	fetch    gitCommitFetchPlan
	// anchor is the instant this order is read at. nil keeps the plain
	// newest-commits read; the entry is then never filtered locally.
	anchor *time.Time
	// skip drops the order from the response because the caller may not see it.
	skip bool
}

// RecentCommits returns the commits per order id. Every order is read at its own
// anchor instant: the moment its release started executing (the CI checked the
// branch out then), or its creation time for an order that never ran. Orders that
// do not exist and orders without a usable repository/credential keep their slot
// with a readable Error instead of dropping out, so the UI can explain the gap.
//
// The remote cost stays one read per repository/ref/credential: the first pass
// pulls a shared history window and every order filters it at its own anchor, and
// only an order whose anchor sits behind that window pays for a (cached) deeper
// read of its own.
func (uc *GitCommitManager) RecentCommits(
	ctx context.Context,
	query GitCommitQuery,
) (map[string]ReleaseOrderRecentCommits, error) {
	ids := normalizeGitCommitOrderIDs(query.OrderIDs)
	result := make(map[string]ReleaseOrderRecentCommits, len(ids))
	if len(ids) == 0 {
		return result, nil
	}
	limit := query.Limit
	if limit <= 0 {
		limit = gitCommitDefaultLimit
	}
	if limit > gitCommitMaxLimit {
		limit = gitCommitMaxLimit
	}

	credentials, err := uc.listActiveCredentials(ctx)
	if err != nil {
		return nil, err
	}
	visible := newVisibleApplicationFilter(query.VisibleApplicationIDs)

	plans := make([]gitOrderPlan, 0, len(ids))
	windows := make(map[string]gitCommitFetchPlan, len(ids))
	for _, orderID := range ids {
		plan, planErr := uc.buildOrderPlan(ctx, orderID, credentials, visible, limit, query.AsOf)
		if planErr != nil {
			return nil, planErr
		}
		if plan.skip {
			continue
		}
		plans = append(plans, plan)
		if plan.fetchKey == "" {
			continue
		}
		if _, exists := windows[plan.fetchKey]; !exists {
			windows[plan.fetchKey] = plan.fetch
		}
	}

	// First pass: the shared windows, one read per repository/ref/credential
	// whatever the anchors of its orders are.
	outcomes := uc.fetchPlans(ctx, windows)

	// Second pass: an anchor behind the shared window needs history the window does
	// not hold. Those orders get a read of their own, cached per anchor instant
	// because two orders rarely share one to the second.
	deepReads := make(map[string]gitCommitFetchPlan)
	for index := range plans {
		plan := &plans[index]
		if plan.fetchKey == "" || plan.anchor == nil {
			continue
		}
		outcome, ok := outcomes[plan.fetchKey]
		if !ok || outcome.errMessage != "" {
			// A failed window is reported as-is instead of stacking a second failure
			// on top of it.
			continue
		}
		if _, found := commitsAtOrBefore(outcome.commits, *plan.anchor); found {
			continue
		}
		plan.fetchKey = gitCommitAnchorCacheKey(plan.fetch.projectPath, plan.fetch.ref, plan.fetch.credentialID, *plan.anchor)
		plan.fetch.asOf = plan.anchor
		plan.fetch.limit = gitCommitWindowSize
		deepReads[plan.fetchKey] = plan.fetch
	}
	for key, outcome := range uc.fetchPlans(ctx, deepReads) {
		outcomes[key] = outcome
	}

	for _, plan := range plans {
		entry := plan.entry
		if plan.fetchKey != "" {
			outcome := outcomes[plan.fetchKey]
			entry.Commits = outcome.commits
			entry.Error = outcome.errMessage
			if plan.anchor != nil && entry.Error == "" {
				// The window is the newest history; the order only shows the part of
				// it the branch already had at its anchor.
				windowed, found := commitsAtOrBefore(outcome.commits, *plan.anchor)
				if !found {
					// The channel reported no error and still nothing at or before the
					// anchor: the history does not reach the release instant.
					entry.Error = gitlab.ErrHistoryTooShallow.Error()
					windowed = nil
				}
				entry.Commits = windowed
			}
			if len(entry.Commits) > limit {
				entry.Commits = entry.Commits[:limit]
			}
		}
		if entry.Commits == nil {
			entry.Commits = []GitCommitInfo{}
		}
		result[entry.OrderID] = entry
	}
	return result, nil
}

// buildOrderPlan resolves one order into a response entry plus, when the order
// can be read, the unique repository read it needs.
func (uc *GitCommitManager) buildOrderPlan(
	ctx context.Context,
	orderID string,
	credentials []gitcredentialdomain.Credential,
	visible *visibleApplicationFilter,
	limit int,
	forcedAsOf *time.Time,
) (gitOrderPlan, error) {
	entry := ReleaseOrderRecentCommits{OrderID: orderID, Commits: []GitCommitInfo{}}

	order, err := uc.orders.GetByID(ctx, orderID)
	if err != nil {
		if errors.Is(err, releasedomain.ErrOrderNotFound) {
			entry.Error = "发布单不存在或已删除"
			return gitOrderPlan{entry: entry}, nil
		}
		return gitOrderPlan{}, err
	}
	if visible != nil && !visible.allows(order.ApplicationID) {
		return gitOrderPlan{skip: true}, nil
	}
	entry.ApplicationID = order.ApplicationID
	entry.ApplicationName = order.ApplicationName
	entry.Ref = strings.TrimSpace(order.GitRef)
	anchor := gitCommitAnchor(order, forcedAsOf)
	if anchor != nil {
		entry.AsOf = anchor.UTC().Format(time.RFC3339)
	}

	application, err := uc.applications.GetByID(ctx, order.ApplicationID)
	if err != nil {
		if errors.Is(err, applicationdomain.ErrNotFound) {
			entry.Error = "应用不存在或已删除"
			return gitOrderPlan{entry: entry}, nil
		}
		return gitOrderPlan{}, err
	}
	if strings.TrimSpace(entry.ApplicationName) == "" {
		entry.ApplicationName = application.Name
	}
	repoURL := strings.TrimSpace(application.RepoURL)
	entry.Repository = repoURL
	if repoURL == "" {
		entry.Error = "应用未配置 Git 仓库地址"
		return gitOrderPlan{entry: entry}, nil
	}

	credential, ok := gitcredentialdomain.ResolveForRepoURL(credentials, repoURL)
	if !ok {
		entry.Error = "未找到匹配的 Git 凭证（按仓库地址前缀匹配）"
		return gitOrderPlan{entry: entry}, nil
	}
	projectPath := gitcredentialdomain.RepoPathFromURL(repoURL)
	if projectPath == "" {
		entry.Error = "无法从应用仓库地址解析 GitLab 项目路径"
		return gitOrderPlan{entry: entry}, nil
	}

	entry.Provider = string(credential.Provider)
	entry.CredentialID = credential.ID
	entry.CredentialName = credential.Name
	entry.WebURL = gitCommitRepoWebURL(repoURL, credential.BaseURL, projectPath)

	plan := gitCommitFetchPlan{
		fetcher:      uc.fetcherFor(credential),
		projectPath:  projectPath,
		ref:          entry.Ref,
		credentialID: credential.ID,
		limit:        limit,
	}
	key := gitCommitCacheKey(projectPath, entry.Ref, credential.ID, false)
	if anchor != nil {
		// Anchored orders share the window read: the channel is asked for the whole
		// gitCommitWindowSize window and each order filters it at its own instant,
		// so N orders of one branch cost one read.
		plan.limit = gitCommitWindowSize
		key = gitCommitCacheKey(projectPath, entry.Ref, credential.ID, true)
	}
	return gitOrderPlan{
		entry:    entry,
		fetchKey: key,
		fetch:    plan,
		anchor:   anchor,
	}, nil
}

// gitCommitAnchor is the instant one order is read at. A release order is read at
// the moment its execution started, because that is when the pipeline checked the
// branch out; an order that never started (queued, rejecting approval, failed
// before dispatch) falls back to its creation time, which is the head the release
// was prepared against. A record without any usable timestamp has no anchor and is
// read as the plain newest history.
func gitCommitAnchor(order releasedomain.ReleaseOrder, forcedAsOf *time.Time) *time.Time {
	if forcedAsOf != nil && !forcedAsOf.IsZero() {
		anchor := forcedAsOf.UTC()
		return &anchor
	}
	if order.StartedAt != nil && !order.StartedAt.IsZero() {
		anchor := order.StartedAt.UTC()
		return &anchor
	}
	if !order.CreatedAt.IsZero() {
		anchor := order.CreatedAt.UTC()
		return &anchor
	}
	return nil
}

// commitsAtOrBefore returns the newest-first tail of a commit window that is at or
// before the anchor: the head of that tail is the commit the branch pointed at
// when the release ran, and the tail continues into its history. The second result
// is false when the window holds no commit old enough, which tells the caller to
// read deeper history.
//
// A commit without a usable timestamp never satisfies the anchor: treating an
// unparsable date as "old enough" would report an arbitrary commit as the release
// head.
func commitsAtOrBefore(commits []GitCommitInfo, anchor time.Time) ([]GitCommitInfo, bool) {
	for index := range commits {
		committedAt := commits[index].CommittedAt
		if committedAt.IsZero() || committedAt.After(anchor) {
			continue
		}
		return commits[index:], true
	}
	return nil, false
}

// visibleApplicationFilter is the pre-fetch authorisation check. A nil filter
// allows everything; an empty one allows nothing.
type visibleApplicationFilter struct {
	byID map[string]struct{}
}

func newVisibleApplicationFilter(applicationIDs []string) *visibleApplicationFilter {
	if applicationIDs == nil {
		return nil
	}
	byID := make(map[string]struct{}, len(applicationIDs))
	for _, item := range applicationIDs {
		id := strings.TrimSpace(item)
		if id == "" {
			continue
		}
		byID[id] = struct{}{}
	}
	return &visibleApplicationFilter{byID: byID}
}

func (f *visibleApplicationFilter) allows(applicationID string) bool {
	if f == nil {
		return true
	}
	_, ok := f.byID[strings.TrimSpace(applicationID)]
	return ok
}

// fetchPlans serves every planned read from the commit cache when possible and
// fetches the rest with a bounded worker pool, then stores the successes.
func (uc *GitCommitManager) fetchPlans(
	ctx context.Context,
	plans map[string]gitCommitFetchPlan,
) map[string]gitCommitFetchOutcome {
	outcomes := make(map[string]gitCommitFetchOutcome, len(plans))
	pending := make([]string, 0, len(plans))
	now := uc.now()
	for key := range plans {
		if commits, ok := uc.cachedCommits(key, now); ok {
			outcomes[key] = gitCommitFetchOutcome{commits: commits}
			continue
		}
		pending = append(pending, key)
	}
	if len(pending) == 0 {
		return outcomes
	}
	// Deterministic order keeps worker assignment stable across polls.
	sort.Strings(pending)

	workerCount := gitCommitFetchWorker
	if len(pending) < workerCount {
		workerCount = len(pending)
	}
	type fetchResult struct {
		key        string
		commits    []GitCommitInfo
		errMessage string
	}
	jobs := make(chan string)
	results := make(chan fetchResult, len(pending))
	var wg sync.WaitGroup
	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for key := range jobs {
				commits, errMessage := uc.fetchCommits(ctx, plans[key])
				results <- fetchResult{key: key, commits: commits, errMessage: errMessage}
			}
		}()
	}
	go func() {
		for _, key := range pending {
			jobs <- key
		}
		close(jobs)
		wg.Wait()
		close(results)
	}()

	for item := range results {
		if item.errMessage == "" {
			uc.storeCommits(item.key, item.commits)
		}
		outcomes[item.key] = gitCommitFetchOutcome{commits: item.commits, errMessage: item.errMessage}
	}
	return outcomes
}

func (uc *GitCommitManager) fetchCommits(
	ctx context.Context,
	plan gitCommitFetchPlan,
) ([]GitCommitInfo, string) {
	if plan.fetcher == nil {
		return []GitCommitInfo{}, "Git 读取通道不可用"
	}
	commits, err := plan.fetcher.ListCommits(ctx, plan.projectPath, plan.ref, plan.limit, plan.asOf)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return []GitCommitInfo{}, "读取提交已取消或超时"
		}
		return []GitCommitInfo{}, describeGitLabFailure(err)
	}
	items := make([]GitCommitInfo, 0, len(commits))
	for _, item := range commits {
		items = append(items, GitCommitInfo{
			SHA:         item.ID,
			ShortSHA:    item.ShortID,
			Title:       item.Title,
			Message:     item.Message,
			AuthorName:  item.AuthorName,
			AuthorEmail: item.AuthorEmail,
			CommittedAt: item.CommittedAt,
			WebURL:      item.WebURL,
		})
	}
	// Both channels already return commits newest first (GitLab orders by date, git log
	// walks the history backwards); sorting makes the contract
	// explicit and covers a fixture or a proxy that reorders the payload.
	sort.SliceStable(items, func(i, j int) bool {
		return items[i].CommittedAt.After(items[j].CommittedAt)
	})
	// The window itself is not truncated: an anchored read is filtered per order
	// afterwards, and a window trimmed here would silently lose the anchor.
	if plan.limit > 0 && len(items) > plan.limit {
		items = items[:plan.limit]
	}
	return items, ""
}

// listActiveCredentials returns the enabled credentials, cached for 30s so the
// 10s poll does not hit the database on every refresh.
func (uc *GitCommitManager) listActiveCredentials(ctx context.Context) ([]gitcredentialdomain.Credential, error) {
	now := uc.now()
	uc.mu.Lock()
	if uc.credentialCache.valid && now.Before(uc.credentialCache.expiresAt) {
		items := uc.credentialCache.items
		uc.mu.Unlock()
		return items, nil
	}
	uc.mu.Unlock()

	items, err := uc.loadActiveCredentials(ctx)
	if err != nil {
		return nil, err
	}

	uc.mu.Lock()
	uc.credentialCache = cachedGitCredentials{
		valid:     true,
		items:     items,
		expiresAt: now.Add(uc.credentialTTL),
	}
	uc.mu.Unlock()
	return items, nil
}

// loadActiveCredentials pages through the filterable list instead of adding a
// second repository method, so the credential repository keeps one read API.
func (uc *GitCommitManager) loadActiveCredentials(ctx context.Context) ([]gitcredentialdomain.Credential, error) {
	items := make([]gitcredentialdomain.Credential, 0, gitCommitCredentialPage)
	for page := 1; page <= gitCommitCredentialMaxPages; page++ {
		pageItems, total, err := uc.credentials.List(ctx, gitcredentialdomain.ListFilter{
			Status:   gitcredentialdomain.StatusActive,
			Page:     page,
			PageSize: gitCommitCredentialPage,
		})
		if err != nil {
			return nil, err
		}
		items = append(items, pageItems...)
		if len(pageItems) < gitCommitCredentialPage || int64(len(items)) >= total {
			break
		}
	}
	return items, nil
}

func (uc *GitCommitManager) cachedCommits(key string, now time.Time) ([]GitCommitInfo, bool) {
	uc.mu.Lock()
	defer uc.mu.Unlock()
	item, ok := uc.commitCache[key]
	if !ok || !now.Before(item.expiresAt) {
		return nil, false
	}
	return item.commits, true
}

func (uc *GitCommitManager) storeCommits(key string, commits []GitCommitInfo) {
	uc.mu.Lock()
	defer uc.mu.Unlock()
	now := uc.now()
	if len(uc.commitCache) >= gitCommitCacheMaxEntries {
		uc.pruneCommitCacheLocked(now)
	}
	uc.commitCache[key] = cachedGitCommits{
		commits:   commits,
		expiresAt: now.Add(uc.commitTTL),
	}
}

// pruneCommitCacheLocked drops the entries whose TTL has passed. The cache key
// space is open ended since an anchored read that has to go deeper than the
// window is stored per anchor instant, so entries that nobody can hit again must
// not accumulate forever. Only expired entries are dropped: a live entry may
// still answer the poll in flight. The caller holds uc.mu.
func (uc *GitCommitManager) pruneCommitCacheLocked(now time.Time) {
	for key, item := range uc.commitCache {
		if !now.Before(item.expiresAt) {
			delete(uc.commitCache, key)
		}
	}
}

// gitCommitRepoWebURL builds the browser URL of the repository. The application
// URL is already the GitLab project URL with a ".git" suffix, so trimming it
// stays correct even when GitLab is served from a sub-path, which a
// base-url-plus-project-path join would break. The credential base is only the
// fallback for a repository URL that carries no scheme.
func gitCommitRepoWebURL(repoURL string, baseURL string, projectPath string) string {
	if strings.Contains(repoURL, "://") {
		return strings.TrimSuffix(repoURL, ".git")
	}
	return gitlab.ProjectWebURL(baseURL, projectPath)
}

// gitCommitCacheKey identifies one repository read. The credential is part of the
// key so switching a repository to another credential cannot serve stale data
// fetched with the previous one.
//
// The anchored flag separates the two shapes one repository can be read in: the
// shared history window (gitCommitWindowSize commits, filtered per order in this
// file) and the plain newest-limit read. The window content itself does not
// depend on the anchor instant, so the instant is not part of the key; letting a
// five commit read answer an anchored lookup would, however, silently drop the
// anchor, which is why the flag is there.
func gitCommitCacheKey(projectPath string, ref string, credentialID string, anchored bool) string {
	key := projectPath + "|" + ref + "|" + credentialID
	if anchored {
		return key + "|asof"
	}
	return key
}

// gitCommitAnchorCacheKey identifies the deep read of one anchor instant: the
// window the shared read could not reach. It is only used when the shared window
// is younger than the order's release instant.
func gitCommitAnchorCacheKey(projectPath string, ref string, credentialID string, anchor time.Time) string {
	return gitCommitCacheKey(projectPath, ref, credentialID, true) + "@" + anchor.UTC().Format(time.RFC3339Nano)
}

// normalizeGitCommitOrderIDs trims, de-duplicates and caps the requested ids.
func normalizeGitCommitOrderIDs(orderIDs []string) []string {
	result := make([]string, 0, len(orderIDs))
	seen := make(map[string]struct{}, len(orderIDs))
	for _, item := range orderIDs {
		id := strings.TrimSpace(item)
		if id == "" {
			continue
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		result = append(result, id)
		if len(result) >= gitCommitMaxOrders {
			break
		}
	}
	return result
}
