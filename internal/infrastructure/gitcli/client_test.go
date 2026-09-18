package gitcli

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"gos/internal/infrastructure/gitlab"
)

// requireGit skips the suite where the git binary is missing instead of failing:
// the runtime image installs it, a bare developer machine may not.
func requireGit(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath(gitBinary); err != nil {
		t.Skip("git 不可用，跳过 gitcli 测试")
	}
}

// runGit runs one git command and fails the test on error. The environment is
// made hermetic so a developer's ~/.gitconfig cannot change the fixture.
func runGit(t *testing.T, dir string, env []string, args ...string) string {
	t.Helper()
	cmd := exec.Command(gitBinary, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_CONFIG_GLOBAL=/dev/null",
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_TERMINAL_PROMPT=0",
	)
	cmd.Env = append(cmd.Env, env...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
}

type commitSpec struct {
	title string
	body  string
	// when is the commit timestamp. The fixture writes fixed, ascending timestamps
	// so the newest-first assertion cannot depend on the test's wall clock.
	when time.Time
}

// remoteFixture is a local bare repository used as the "remote" of the client,
// plus the work tree that pushes into it.
type remoteFixture struct {
	root   string
	remote string
	work   string
	// shas holds the commit ids of the main branch, oldest first.
	shas []string
}

func newRemoteFixture(t *testing.T, specs []commitSpec) *remoteFixture {
	t.Helper()
	requireGit(t)
	root := t.TempDir()
	fixture := &remoteFixture{
		root:   root,
		remote: filepath.Join(root, "app.git"),
		work:   filepath.Join(root, "work"),
	}
	runGit(t, root, nil, "init", "--bare", "--quiet", "--initial-branch=main", fixture.remote)
	runGit(t, root, nil, "init", "--quiet", "--initial-branch=main", fixture.work)
	runGit(t, fixture.work, nil, "config", "user.name", "发布机器人")
	runGit(t, fixture.work, nil, "config", "user.email", "release@example.com")
	runGit(t, fixture.work, nil, "remote", "add", "origin", fixture.remote)
	for _, spec := range specs {
		fixture.shas = append(fixture.shas, fixture.commit(t, spec))
	}
	runGit(t, fixture.work, nil, "push", "--quiet", "origin", "main")
	return fixture
}

// baseURL is the credential base URL. It is the parent directory of the bare
// remote, so the client's "<baseURL>/<projectPath>.git" rule still applies.
func (f *remoteFixture) baseURL() string {
	return "file://" + f.root
}

func (f *remoteFixture) commit(t *testing.T, spec commitSpec) string {
	t.Helper()
	path := filepath.Join(f.work, "app.txt")
	if err := os.WriteFile(path, []byte(spec.title+"\n"), 0o644); err != nil {
		t.Fatalf("write work file: %v", err)
	}
	runGit(t, f.work, nil, "add", ".")
	message := spec.title
	if spec.body != "" {
		message += "\n\n" + spec.body
	}
	stamp := spec.when.Format(time.RFC3339)
	env := []string{"GIT_AUTHOR_DATE=" + stamp, "GIT_COMMITTER_DATE=" + stamp}
	runGit(t, f.work, env, "commit", "--quiet", "-m", message)
	return runGit(t, f.work, nil, "rev-parse", "HEAD")
}

// pushMain adds commits to the checked out main branch and pushes them.
func (f *remoteFixture) pushMain(t *testing.T, specs []commitSpec) []string {
	t.Helper()
	shas := f.commitAll(t, specs)
	runGit(t, f.work, nil, "push", "--quiet", "origin", "main")
	return shas
}

// pushBranch branches off main, commits and pushes, returning the new commit ids.
func (f *remoteFixture) pushBranch(t *testing.T, branch string, specs []commitSpec) []string {
	t.Helper()
	runGit(t, f.work, nil, "checkout", "--quiet", "-b", branch, "main")
	shas := f.commitAll(t, specs)
	runGit(t, f.work, nil, "push", "--quiet", "origin", branch)
	runGit(t, f.work, nil, "checkout", "--quiet", "main")
	return shas
}

func (f *remoteFixture) commitAll(t *testing.T, specs []commitSpec) []string {
	t.Helper()
	shas := make([]string, 0, len(specs))
	for _, spec := range specs {
		shas = append(shas, f.commit(t, spec))
	}
	return shas
}

func testCommitSpecs(base time.Time) []commitSpec {
	return []commitSpec{
		{title: "feat: 初始化仓库", when: base},
		{title: "fix: 修复发布单列表", body: "附带说明：同步了参数定义", when: base.Add(2 * time.Hour)},
		{title: "chore: 升级依赖", when: base.Add(4 * time.Hour)},
	}
}

// pushEmptyCommits appends count commits with step between them, without touching
// the work tree: the deepening test needs a history deeper than the window the
// client fetches on its first try, which would be slow to build file by file.
func (f *remoteFixture) pushEmptyCommits(t *testing.T, count int, base time.Time, step time.Duration) []string {
	t.Helper()
	shas := make([]string, 0, count)
	for index := 0; index < count; index++ {
		stamp := base.Add(time.Duration(index) * step).Format(time.RFC3339)
		env := []string{"GIT_AUTHOR_DATE=" + stamp, "GIT_COMMITTER_DATE=" + stamp}
		runGit(t, f.work, env, "commit", "--quiet", "--allow-empty", "-m", "chore: 第 "+strconv.Itoa(index+1)+" 次提交")
		shas = append(shas, runGit(t, f.work, nil, "rev-parse", "HEAD"))
	}
	runGit(t, f.work, nil, "push", "--quiet", "origin", "main")
	return shas
}

func TestListCommitsReadsLocalRepositoryNewestFirst(t *testing.T) {
	base := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	fixture := newRemoteFixture(t, testCommitSpecs(base))
	client := NewClient(Config{
		BaseURL:  fixture.baseURL(),
		Username: "client011",
		Secret:   "p@ss w0rd",
		CacheDir: t.TempDir(),
	})

	commits, err := client.ListCommits(context.Background(), "app", "main", 5, nil)
	if err != nil {
		t.Fatalf("ListCommits err = %v", err)
	}
	if len(commits) != 3 {
		t.Fatalf("commits = %+v, want 3", commits)
	}
	for index, commit := range commits {
		want := fixture.shas[len(fixture.shas)-1-index]
		if commit.ID != want {
			t.Fatalf("commit[%d].ID = %q, want %q (newest first)", index, commit.ID, want)
		}
		if commit.ShortID == "" || !strings.HasPrefix(commit.ID, commit.ShortID) {
			t.Fatalf("commit[%d].ShortID = %q, want a prefix of %q", index, commit.ShortID, commit.ID)
		}
		if commit.WebURL != fixture.baseURL()+"/app/-/commit/"+commit.ID {
			t.Fatalf("commit[%d].WebURL = %q", index, commit.WebURL)
		}
	}
	head := commits[0]
	if head.Title != "chore: 升级依赖" || head.Message != "chore: 升级依赖" {
		t.Fatalf("head = %+v", head)
	}
	if head.AuthorName != "发布机器人" || head.AuthorEmail != "release@example.com" {
		t.Fatalf("head author = %q <%q>", head.AuthorName, head.AuthorEmail)
	}
	if !head.CommittedAt.Equal(base.Add(4 * time.Hour)) {
		t.Fatalf("head.CommittedAt = %v", head.CommittedAt)
	}
	// The multi line message of the second commit has to survive the -z parsing.
	if got := commits[1].Message; !strings.Contains(got, "附带说明") || !strings.Contains(got, "\n") {
		t.Fatalf("commits[1].Message = %q, want the body kept", got)
	}
	if commits[1].Title != "fix: 修复发布单列表" {
		t.Fatalf("commits[1].Title = %q", commits[1].Title)
	}
}

func TestListCommitsHonoursLimit(t *testing.T) {
	base := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	fixture := newRemoteFixture(t, testCommitSpecs(base))
	client := NewClient(Config{BaseURL: fixture.baseURL(), CacheDir: t.TempDir()})

	commits, err := client.ListCommits(context.Background(), "app", "main", 2, nil)
	if err != nil {
		t.Fatalf("ListCommits err = %v", err)
	}
	if len(commits) != 2 {
		t.Fatalf("commits = %+v, want the limit honoured", commits)
	}
	if commits[0].ID != fixture.shas[2] || commits[1].ID != fixture.shas[1] {
		t.Fatalf("commits = %+v, want the two newest", commits)
	}
}

// TestListCommitsReadsAtTheAnchoredInstant covers the as-of read: the caller gets
// the commit the branch pointed at then, not the current head.
func TestListCommitsReadsAtTheAnchoredInstant(t *testing.T) {
	base := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	specs := make([]commitSpec, 0, 6)
	for index := 0; index < 6; index++ {
		specs = append(specs, commitSpec{
			title: "chore: 第 " + strconv.Itoa(index+1) + " 次提交",
			when:  base.Add(time.Duration(index) * 2 * time.Hour),
		})
	}
	fixture := newRemoteFixture(t, specs)
	client := NewClient(Config{BaseURL: fixture.baseURL(), CacheDir: t.TempDir()})

	// The third newest commit (shas[3]) sits at base+6h: reading at that instant
	// has to report it as the head instead of the newest commit shas[5].
	anchor := base.Add(6 * time.Hour)
	commits, err := client.ListCommits(context.Background(), "app", "main", 5, &anchor)
	if err != nil {
		t.Fatalf("ListCommits err = %v", err)
	}
	if len(commits) != 4 {
		t.Fatalf("commits = %+v, want the anchor commit and the three before it", commits)
	}
	if commits[0].ID != fixture.shas[3] {
		t.Fatalf("head = %q, want %q (the head at the anchor, not the current tip)", commits[0].ID, fixture.shas[3])
	}
	for index, commit := range commits {
		if want := fixture.shas[3-index]; commit.ID != want {
			t.Fatalf("commit[%d].ID = %q, want %q", index, commit.ID, want)
		}
		if !commit.CommittedAt.Equal(base.Add(time.Duration(3-index) * 2 * time.Hour)) {
			t.Fatalf("commit[%d].CommittedAt = %v", index, commit.CommittedAt)
		}
	}

	limited, err := client.ListCommits(context.Background(), "app", "main", 2, &anchor)
	if err != nil {
		t.Fatalf("limited ListCommits err = %v", err)
	}
	if len(limited) != 2 || limited[0].ID != fixture.shas[3] || limited[1].ID != fixture.shas[2] {
		t.Fatalf("limited = %+v, want the two newest commits at the anchor", limited)
	}

	// The plain read is untouched: it still starts at the tip.
	plain, err := client.ListCommits(context.Background(), "app", "main", 2, nil)
	if err != nil {
		t.Fatalf("plain ListCommits err = %v", err)
	}
	if len(plain) != 2 || plain[0].ID != fixture.shas[5] {
		t.Fatalf("plain = %+v, want the newest commits", plain)
	}

	// An instant between two commits still resolves to the commit the branch
	// pointed at then (shas[2] at base+4h).
	between := base.Add(5 * time.Hour)
	betweenCommits, err := client.ListCommits(context.Background(), "app", "main", 1, &between)
	if err != nil {
		t.Fatalf("between ListCommits err = %v", err)
	}
	if len(betweenCommits) != 1 || betweenCommits[0].ID != fixture.shas[2] {
		t.Fatalf("between = %+v, want the commit before the instant", betweenCommits)
	}
}

// TestListCommitsDeepensHistoryForAnOldAnchor covers an anchor behind the fetched
// window: the mirror grows with git fetch --deepen until the window reaches the
// release instant, and an instant older than the repository history is reported
// instead of being answered with newer commits.
func TestListCommitsDeepensHistoryForAnOldAnchor(t *testing.T) {
	base := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	// The fixture needs one commit for its own push; the 150 that follow are the
	// history the anchored read has to walk.
	fixture := newRemoteFixture(t, []commitSpec{{title: "feat: 初始化仓库", when: base.Add(-time.Hour)}})
	// One hundred and fifty commits, one hour apart: the first fetch only holds the
	// newest hundred, so the anchor below sits in the history behind it.
	shas := fixture.pushEmptyCommits(t, 150, base, time.Hour)
	client := NewClient(Config{BaseURL: fixture.baseURL(), CacheDir: t.TempDir()})

	// shas[30] is the 120th newest commit, at base+30h.
	anchor := base.Add(30 * time.Hour)
	commits, err := client.ListCommits(context.Background(), "app", "main", 3, &anchor)
	if err != nil {
		t.Fatalf("ListCommits err = %v", err)
	}
	if len(commits) != 3 {
		t.Fatalf("commits = %+v, want the limit honoured", commits)
	}
	for index, commit := range commits {
		if want := shas[30-index]; commit.ID != want {
			t.Fatalf("commit[%d].ID = %q, want %q (the deepened history)", index, commit.ID, want)
		}
	}

	// An instant older than the root commit cannot be answered: reporting the
	// newest commits would show a head that did not exist when the release ran.
	tooOld := base.Add(-24 * time.Hour)
	_, err = client.ListCommits(context.Background(), "app", "main", 3, &tooOld)
	if err == nil {
		t.Fatalf("ListCommits err = nil, want a shallow history failure")
	}
	if !errors.Is(err, gitlab.ErrHistoryTooShallow) {
		t.Fatalf("err = %v, want ErrHistoryTooShallow", err)
	}
	if !strings.Contains(err.Error(), "提交历史不足") {
		t.Fatalf("err = %q, want a readable message", err.Error())
	}
}

func TestListCommitsRefreshesCachedMirror(t *testing.T) {
	base := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	fixture := newRemoteFixture(t, testCommitSpecs(base))
	cacheDir := t.TempDir()
	client := NewClient(Config{BaseURL: fixture.baseURL(), CacheDir: cacheDir})

	first, err := client.ListCommits(context.Background(), "app", "main", 5, nil)
	if err != nil {
		t.Fatalf("first ListCommits err = %v", err)
	}
	if len(first) != 3 {
		t.Fatalf("first commits = %+v, want 3", first)
	}
	// The mirror was created under the cache directory.
	entries, err := os.ReadDir(cacheDir)
	if err != nil || len(entries) != 1 {
		t.Fatalf("cache dir = %v / %v, want one mirror", entries, err)
	}

	newSHA := fixture.pushMain(t, []commitSpec{
		{title: "feat: 新增提交", when: base.Add(6 * time.Hour)},
	})[0]

	second, err := client.ListCommits(context.Background(), "app", "main", 5, nil)
	if err != nil {
		t.Fatalf("second ListCommits err = %v", err)
	}
	if len(second) != 4 {
		t.Fatalf("second commits = %+v, want the fetched commit", second)
	}
	if second[0].ID != newSHA {
		t.Fatalf("second[0].ID = %q, want %q (the mirror was not refreshed)", second[0].ID, newSHA)
	}
	if entries, _ = os.ReadDir(cacheDir); len(entries) != 1 {
		t.Fatalf("cache dir = %v, want the mirror reused", entries)
	}
}

func TestListCommitsServesAnotherBranchFromTheSameMirror(t *testing.T) {
	base := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	fixture := newRemoteFixture(t, testCommitSpecs(base))
	devSHAs := fixture.pushBranch(t, "dev", []commitSpec{
		{title: "feat: dev 分支提交", when: base.Add(8 * time.Hour)},
	})
	client := NewClient(Config{BaseURL: fixture.baseURL(), CacheDir: t.TempDir()})

	if _, err := client.ListCommits(context.Background(), "app", "main", 5, nil); err != nil {
		t.Fatalf("main ListCommits err = %v", err)
	}
	// The mirror was cloned with --single-branch for main; a second ref still has
	// to be fetchable into it, because a release repository is read for several
	// branches in the same poll.
	commits, err := client.ListCommits(context.Background(), "app", "dev", 5, nil)
	if err != nil {
		t.Fatalf("dev ListCommits err = %v", err)
	}
	if len(commits) != 4 || commits[0].ID != devSHAs[0] {
		t.Fatalf("dev commits = %+v, want the dev tip first", commits)
	}
}

func TestListCommitsReportsMissingBranch(t *testing.T) {
	fixture := newRemoteFixture(t, testCommitSpecs(time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)))
	client := NewClient(Config{BaseURL: fixture.baseURL(), CacheDir: t.TempDir()})

	commits, err := client.ListCommits(context.Background(), "app", "release/does-not-exist", 5, nil)
	if err == nil {
		t.Fatalf("ListCommits = %+v, want a missing branch error", commits)
	}
	if !errors.Is(err, ErrBranchNotFound) {
		t.Fatalf("err = %v, want ErrBranchNotFound", err)
	}
	if !strings.Contains(err.Error(), "分支不存在") {
		t.Fatalf("err = %v, want a readable message", err)
	}
}

func TestListCommitsRejectsUnknownProject(t *testing.T) {
	fixture := newRemoteFixture(t, testCommitSpecs(time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)))
	client := NewClient(Config{BaseURL: fixture.baseURL(), CacheDir: t.TempDir()})

	if _, err := client.ListCommits(context.Background(), "missing-project", "main", 5, nil); err == nil {
		t.Fatalf("ListCommits err = nil, want a failure for a repository that does not exist")
	}
}

// TestListCommitsSendsBasicHeaderAndReportsRejectedCredential covers the measured
// production behaviour: the git transport carries the account password in an
// Authorization header, and a rejected credential surfaces as a readable error
// that never contains the password.
func TestListCommitsSendsBasicHeaderAndReportsRejectedCredential(t *testing.T) {
	requireGit(t)
	const (
		username = "client011"
		secret   = "p@ss w0rd#!X"
	)
	var (
		mu       sync.Mutex
		seenAuth []string
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		seenAuth = append(seenAuth, r.Header.Get("Authorization"))
		mu.Unlock()
		w.Header().Set("WWW-Authenticate", `Basic realm="GitLab"`)
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	client := NewClient(Config{BaseURL: server.URL, Username: username, Secret: secret, CacheDir: t.TempDir()})
	_, err := client.ListCommits(context.Background(), "code/MapMineSystem/mcs-product-web", "main", 5, nil)
	if err == nil {
		t.Fatalf("ListCommits err = nil, want the credential to be rejected")
	}
	if !errors.Is(err, ErrAuthenticationFailed) {
		t.Fatalf("err = %v, want ErrAuthenticationFailed", err)
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatalf("err = %v, the secret leaked into the error message", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(seenAuth) == 0 {
		t.Fatalf("no request reached the server")
	}
	want := "Basic " + base64.StdEncoding.EncodeToString([]byte(username+":"+secret))
	if seenAuth[0] != want {
		t.Fatalf("Authorization = %q, want %q", seenAuth[0], want)
	}
}

func TestListCommitsTimesOut(t *testing.T) {
	requireGit(t)
	// The handler never answers, so the client's own budget has to end the clone.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(5 * time.Second)
	}))
	defer server.Close()

	client := NewClient(Config{BaseURL: server.URL, Username: "client011", Secret: "s", CacheDir: t.TempDir(), TimeoutSec: 1})
	_, err := client.ListCommits(context.Background(), "app", "main", 5, nil)
	if err == nil {
		t.Fatalf("ListCommits err = nil, want a timeout")
	}
	if !errors.Is(err, ErrTimeout) && !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want ErrTimeout", err)
	}
}

func TestListCommitsRunsConcurrentlyOnOneMirror(t *testing.T) {
	base := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	fixture := newRemoteFixture(t, testCommitSpecs(base))
	cacheDir := t.TempDir()
	// Two clients with different credentials resolve to the same mirror directory,
	// which is the case the shared lock has to protect.
	clients := []*Client{
		NewClient(Config{BaseURL: fixture.baseURL(), Username: "a", Secret: "1", CacheDir: cacheDir}),
		NewClient(Config{BaseURL: fixture.baseURL(), Username: "b", Secret: "2", CacheDir: cacheDir}),
	}

	var wg sync.WaitGroup
	errs := make([]error, 8)
	for index := range errs {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			client := clients[index%len(clients)]
			commits, err := client.ListCommits(context.Background(), "app", "main", 5, nil)
			if err == nil && len(commits) != 3 {
				err = errors.New("unexpected commit count")
			}
			errs[index] = err
		}(index)
	}
	wg.Wait()
	for index, err := range errs {
		if err != nil {
			t.Fatalf("worker %d err = %v", index, err)
		}
	}
}

func TestMirrorDirIsKeyedByRepositoryNotCredential(t *testing.T) {
	cacheDir := t.TempDir()
	first := NewClient(Config{BaseURL: "http://git.cloud.local:9080", Username: "a", Secret: "1", CacheDir: cacheDir})
	// A trailing slash and another credential still describe the same mirror.
	second := NewClient(Config{BaseURL: "http://git.cloud.local:9080/", Username: "b", Secret: "2", CacheDir: cacheDir})

	if first.mirrorDir("code/app") != second.mirrorDir("code/app") {
		t.Fatalf("mirror dir differs per credential: %q / %q", first.mirrorDir("code/app"), second.mirrorDir("code/app"))
	}
	if first.mirrorDir("code/app") == first.mirrorDir("code/other") {
		t.Fatalf("mirror dir is not per repository")
	}
	if !strings.HasPrefix(first.mirrorDir("code/app"), cacheDir) {
		t.Fatalf("mirror dir = %q, want it under the cache dir", first.mirrorDir("code/app"))
	}
}

func TestFailClassifiesAndSanitisesGitOutput(t *testing.T) {
	const secret = "p@ss w0rd"
	client := NewClient(Config{BaseURL: "http://git.cloud.local:9080", Username: "client011", Secret: secret})
	header := "Authorization: Basic " + base64.StdEncoding.EncodeToString([]byte("client011:"+secret))
	ctx := context.Background()

	auth := client.fail(ctx, "fetch", gitOutput{
		stderr: "fatal: Authentication failed for 'http://git.cloud.local:9080/code/app.git/'\n" + header + "\n" + secret,
		err:    errors.New("exit status 128"),
	})
	if !errors.Is(auth, ErrAuthenticationFailed) {
		t.Fatalf("err = %v, want ErrAuthenticationFailed", auth)
	}
	if strings.Contains(auth.Error(), secret) || strings.Contains(auth.Error(), header) {
		t.Fatalf("err = %v, the credential leaked", auth)
	}

	missing := client.fail(ctx, "fetch", gitOutput{
		stderr: "fatal: couldn't find remote ref refs/heads/release/1.0",
		err:    errors.New("exit status 128"),
	})
	if !errors.Is(missing, ErrBranchNotFound) {
		t.Fatalf("err = %v, want ErrBranchNotFound", missing)
	}

	remoteBranch := client.fail(ctx, "clone", gitOutput{
		stderr: "Cloning into bare repository...\nfatal: Remote branch nope not found in upstream origin",
		err:    errors.New("exit status 128"),
	})
	if !errors.Is(remoteBranch, ErrBranchNotFound) {
		t.Fatalf("err = %v, want the clone branch error to be classified", remoteBranch)
	}

	expired, cancel := context.WithDeadline(ctx, time.Now().Add(-time.Second))
	defer cancel()
	if err := client.fail(expired, "clone", gitOutput{err: errors.New("signal: killed")}); !errors.Is(err, ErrTimeout) {
		t.Fatalf("err = %v, want ErrTimeout", err)
	}

	cancelled, cancelCall := context.WithCancel(ctx)
	cancelCall()
	if err := client.fail(cancelled, "clone", gitOutput{err: errors.New("signal: killed")}); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want the caller's cancellation", err)
	}
}

func TestEnvKeepsCredentialOutOfTheCommandLine(t *testing.T) {
	const secret = "p@ss w0rd"
	// An inherited variable must not shadow the value the client sets.
	t.Setenv("GIT_TERMINAL_PROMPT", "1")
	t.Setenv("GIT_CONFIG_COUNT", "9")
	client := NewClient(Config{BaseURL: "http://git.cloud.local:9080", Username: "client011", Secret: secret})
	env := strings.Join(client.env(), "\n")
	want := "GIT_CONFIG_VALUE_0=Authorization: Basic " + base64.StdEncoding.EncodeToString([]byte("client011:"+secret))
	if !strings.Contains(env, want) {
		t.Fatalf("env = %q, want the Authorization header through GIT_CONFIG_*", env)
	}
	for _, required := range []string{"GIT_CONFIG_COUNT=1", "GIT_CONFIG_KEY_0=http.extraHeader", "GIT_TERMINAL_PROMPT=0", "GIT_CONFIG_NOSYSTEM=1"} {
		if !strings.Contains(env, required) {
			t.Fatalf("env = %q, want %s", env, required)
		}
	}
	for _, args := range [][]string{
		cloneArgs("main", client.repoURL("code/app"), "/tmp/mirror", 20),
		fetchArgs("/tmp/mirror", "main", 20),
		logArgs("/tmp/mirror", "refs/heads/main", 5),
	} {
		if joined := strings.Join(args, " "); strings.Contains(joined, secret) {
			t.Fatalf("argv = %q, the secret must not reach the command line", joined)
		}
	}
}

func TestParseLogSkipsBlankRecordsAndFallsBackToTitle(t *testing.T) {
	payload := strings.Join([]string{
		"sha1\x1fsha1\x1ffix: 标题\x1f张三\x1fz@example.com\x1f2026-09-18T10:00:00+08:00\x1f修复说明\n",
		"",
		"sha2\x1fsha2\x1fchore: 无正文\x1f李四\x1fl@example.com\x1f2026-09-17T10:00:00Z",
	}, "\x00")
	commits := parseLog(payload, "http://git.cloud.local:9080/code/app/-/commit")
	if len(commits) != 2 {
		t.Fatalf("commits = %+v, want the blank record skipped", commits)
	}
	if commits[0].Message != "修复说明" || commits[0].WebURL != "http://git.cloud.local:9080/code/app/-/commit/sha1" {
		t.Fatalf("commits[0] = %+v", commits[0])
	}
	if commits[1].Message != "chore: 无正文" {
		t.Fatalf("commits[1].Message = %q, want the title fallback", commits[1].Message)
	}
	if !commits[0].CommittedAt.Equal(time.Date(2026, 9, 18, 2, 0, 0, 0, time.UTC)) {
		t.Fatalf("commits[0].CommittedAt = %v", commits[0].CommittedAt)
	}
}

func TestNormalizeProjectPath(t *testing.T) {
	cases := map[string]string{
		"/code/bigData/app/":    "code/bigData/app",
		"code/bigData/app.git":  "code/bigData/app",
		" code/bigData/app ":    "code/bigData/app",
		"code/bigData/app.git/": "code/bigData/app",
		"":                      "",
	}
	for input, want := range cases {
		if got := normalizeProjectPath(input); got != want {
			t.Fatalf("normalizeProjectPath(%q) = %q, want %q", input, got, want)
		}
	}
}
