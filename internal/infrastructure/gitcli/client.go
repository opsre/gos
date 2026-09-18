// Package gitcli reads repository commits through the git binary instead of the
// GitLab REST API.
//
// The platform normally reads commits from GitLab API v4 (see
// internal/infrastructure/gitlab), but an instance can be configured so that the
// API refuses the credentials a user has: the target GitLab answers 401 both for
// "PRIVATE-TOKEN: <password>" and for HTTP basic auth with an account password,
// while the very same account can clone the same repository over the git
// protocol. This package is that second channel, and the release order commit
// lookup selects it for password credentials.
package gitcli

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"gos/internal/infrastructure/gitlab"
)

const (
	// defaultTimeoutSeconds bounds one clone/fetch/log sequence. A cold mirror of a
	// release repository can be tens of megabytes over a slow internal link, and an
	// older anchor may need an extra --deepen round, so the budget is generous: the
	// mirror it warms is reused by every later request, which is what keeps the
	// common case in the low milliseconds.
	defaultTimeoutSeconds = 120
	// defaultCacheDirName is the directory under the system temp dir that holds
	// the bare mirror of every repository the process has read.
	defaultCacheDirName = "gos-git-cache"
	// minFetchDepth keeps a few more commits in the mirror than the current
	// request needs, so a later poll that asks for a bigger limit is served
	// without another fetch.
	minFetchDepth = 20
	// defaultCommitLimit mirrors the GitLab client's default.
	defaultCommitLimit = 5
	// maxCommits caps limit, mirroring the GitLab client's per-page ceiling.
	maxCommits = 100
	// historyWindow is the commit window an anchored read pulls first. It is kept
	// small deliberately: the cold clone pays for it, and a repository with heavy
	// assets can move tens of megabytes for a few dozen commits, so a deeper window
	// is only fetched (through --deepen) by the orders that actually need it.
	historyWindow = 30
	// deepenStep is how much history one deepening round adds to the mirror.
	deepenStep = 60
	// maxDeepenRounds bounds the deepening of one anchored read, mirroring the
	// two page budget of the GitLab channel.
	maxDeepenRounds = 4
	// gitBinary is resolved from PATH; the runtime image installs git.
	gitBinary = "git"
	// maxOutputBytes caps how much git output is buffered. The git log payload is
	// the largest read: an anchored window walks the history back in rounds, so the
	// buffer has to hold a few hundred commits with their full messages.
	maxOutputBytes = 4 << 20
	// maxDetailBytes caps the git output embedded into an error message.
	maxDetailBytes = 300
	// logFieldSeparator is the ASCII unit separator: it cannot appear in a commit
	// message, author name or date, so it is safe as the field separator.
	logFieldSeparator = "\x1f"
	// logRecordSeparator is NUL, which git forbids inside commit content and
	// git log -z emits between two commits.
	logRecordSeparator = "\x00"
	// logFormat keeps one commit per record. %B (the full message) is placed last
	// because it is the only field that may contain newlines.
	logFormat = "%H%x1f%h%x1f%s%x1f%an%x1f%ae%x1f%cI%x1f%B"
)

var (
	// ErrAuthenticationFailed means the account/password pair was rejected by the
	// git transport. The message is user facing (the release order list shows it).
	ErrAuthenticationFailed = errors.New("Git 凭证被拒绝（请确认账号密码可访问该仓库）")
	// ErrBranchNotFound covers both a clone of a missing branch and a fetch of a
	// missing remote ref.
	ErrBranchNotFound = errors.New("分支不存在")
	// ErrTimeout means the git command did not finish inside the configured
	// budget (or was killed with it).
	ErrTimeout = errors.New("Git 拉取超时")
	// ErrGitUnavailable means the runtime image has no usable git binary.
	ErrGitUnavailable = errors.New("运行环境缺少 git 命令")
)

// authFailurePattern classifies the git stderr of a rejected credential. Git
// reports a rejected Authorization header either as a 401/403 from the HTTP
// remote or as "could not read Username ... terminal prompts disabled", because
// terminal prompting is switched off.
var authFailurePattern = regexp.MustCompile(`(?i)(authentication failed|could not read (username|password)|invalid credentials|terminal prompts disabled|\b40[13]\b)`)

// branchMissingPattern matches the clone error of a branch that does not exist
// on the remote ("Remote branch <ref> not found in upstream origin").
var branchMissingPattern = regexp.MustCompile(`(?i)remote branch .* not found`)

// Config describes one credential's git access.
type Config struct {
	// BaseURL is the GitLab base address the credential applies to, for example
	// http://git.cloud.local:9080. Only the HTTP(S) form is supported here.
	BaseURL  string
	Username string
	// Secret is the account password. It is never placed on a command line.
	Secret string
	// TimeoutSec bounds one clone/fetch/log sequence; <= 0 means 20 seconds.
	TimeoutSec int
	// CacheDir is the parent directory of the per-repository mirrors; empty means
	// <os.TempDir()>/gos-git-cache.
	CacheDir string
}

// Client reads commits of one credential through the git binary. It is safe for
// concurrent use: clone and fetch of one repository are serialised, different
// repositories run in parallel.
type Client struct {
	baseURL  string
	username string
	secret   string
	// authHeader is the precomputed "Authorization: Basic ..." value handed to git
	// through the GIT_CONFIG_* environment variables.
	authHeader string
	timeout    time.Duration
	cacheDir   string
}

// NewClient 创建并返回对应组件实例。
func NewClient(cfg Config) *Client {
	timeout := cfg.TimeoutSec
	if timeout <= 0 {
		timeout = defaultTimeoutSeconds
	}
	cacheDir := strings.TrimSpace(cfg.CacheDir)
	if cacheDir == "" {
		cacheDir = filepath.Join(os.TempDir(), defaultCacheDirName)
	}
	client := &Client{
		baseURL:  strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/"),
		username: strings.TrimSpace(cfg.Username),
		// The password is kept verbatim: trimming it would break an account whose
		// password starts or ends with a space.
		secret:   cfg.Secret,
		timeout:  time.Duration(timeout) * time.Second,
		cacheDir: cacheDir,
	}
	if client.username != "" && client.secret != "" {
		encoded := base64.StdEncoding.EncodeToString([]byte(client.username + ":" + client.secret))
		client.authHeader = "Authorization: Basic " + encoded
	}
	return client
}

// ListCommits returns the commits of a repository ref, newest first, mirroring
// the GitLab REST client's contract closely enough for the release order lookup
// to treat both channels alike.
//
// asOf switches the read to a history window:
//
//   - nil: the newest `limit` commits, which is the plain list read.
//   - non-nil: the commits at or before that instant, newest first, up to `limit`
//     of them, the first being the commit the ref pointed at as of that instant.
//     The mirror is always read as a full historyWindow; while the oldest commit
//     of the window is still newer than asOf, the mirror is deepened by
//     `git fetch --deepen=<deepenStep>` (at most maxDeepenRounds rounds) and read
//     again. A history that still does not reach the instant yields
//     gitlab.ErrHistoryTooShallow instead of commits the caller would misread as
//     the release head.
func (c *Client) ListCommits(ctx context.Context, projectPath string, ref string, limit int, asOf *time.Time) ([]gitlab.Commit, error) {
	path := normalizeProjectPath(projectPath)
	if path == "" {
		return nil, errors.New("git project path is required")
	}
	if c.baseURL == "" {
		return nil, errors.New("git base url is required")
	}
	if limit <= 0 {
		limit = defaultCommitLimit
	}
	if limit > maxCommits {
		limit = maxCommits
	}
	ref = strings.TrimSpace(ref)
	// readDepth is both the depth of the mirror and the size of the log read. An
	// anchored read starts at the full window, so the caller finds the anchor
	// commit in the window, and grows by deepenStep per deepening round.
	readDepth := limit
	if asOf != nil && readDepth < historyWindow {
		readDepth = historyWindow
	}
	if readDepth < minFetchDepth {
		readDepth = minFetchDepth
	}

	if err := os.MkdirAll(c.cacheDir, 0o755); err != nil {
		return nil, fmt.Errorf("创建 Git 缓存目录失败：%v", err)
	}

	// The mirror sync is cache warm-up work that may outlive one HTTP request: the
	// release list gives up at 60s, and killing the clone at that point would throw
	// the transfer away so the next poll starts from zero. Detach from the request
	// context and bound the sequence with the client's own budget instead; the
	// mirror it leaves behind serves every later read in milliseconds.
	callCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), c.timeout)
	defer cancel()

	dir := c.mirrorDir(path)
	// The mirror is a shared resource: every caller of this repository has to wait
	// for the clone or fetch in flight, because two concurrent git processes in
	// one object store corrupt it.
	unlock := lockMirror(dir)
	defer unlock()

	repoURL := c.repoURL(path)
	webURLBase := c.baseURL + "/" + path + "/-/commit"
	for round := 0; ; round++ {
		if round == 0 {
			if err := c.syncMirror(callCtx, repoURL, dir, ref, readDepth); err != nil {
				return nil, err
			}
		} else {
			if err := c.deepenMirror(callCtx, dir, ref, deepenStep); err != nil {
				return nil, err
			}
			readDepth += deepenStep
		}

		out := c.runGit(callCtx, logArgs(dir, revFor(ref), readDepth)...)
		if out.err != nil {
			return nil, c.fail(callCtx, "log", out)
		}
		commits := parseLog(out.stdout, webURLBase)
		if asOf == nil {
			return truncateCommits(commits, limit), nil
		}
		if windowed := gitlab.CommitsAtOrBefore(commits, *asOf); len(windowed) > 0 {
			return truncateCommits(windowed, limit), nil
		}
		// The whole window is newer than asOf. Deepening only helps while the
		// mirror is still shallow: a log that returned less than the requested
		// depth already read the root commit, so the instant predates the history.
		if len(commits) < readDepth || round >= maxDeepenRounds {
			return nil, fmt.Errorf("%w（已加深 %d 轮，分支历史早于该时点）", gitlab.ErrHistoryTooShallow, round)
		}
	}
}

// deepenMirror adds deepenStep commits behind the current shallow boundary of the
// mirror. It is the anchored read's second gear: the first sync only has to serve
// the recent history, and only an order whose release instant sits further back
// pays for the deeper history.
func (c *Client) deepenMirror(ctx context.Context, dir string, ref string, step int) error {
	out := c.runGit(ctx, deepenArgs(dir, ref, step)...)
	if out.err != nil {
		return c.fail(ctx, "fetch", out)
	}
	return nil
}

// truncateCommits caps a read at the caller's limit. The commits are newest first,
// so the cap drops the oldest ones.
func truncateCommits(commits []gitlab.Commit, limit int) []gitlab.Commit {
	if limit > 0 && len(commits) > limit {
		return commits[:limit]
	}
	return commits
}

// syncMirror makes dir hold the current tip of ref: a mirror that is missing or
// damaged is cloned, an existing one is fetched. Without the fetch a cached
// mirror would keep serving the commits of the first call forever.
func (c *Client) syncMirror(ctx context.Context, repoURL string, dir string, ref string, depth int) error {
	if usableMirror(dir) {
		out := c.runGit(ctx, fetchArgs(dir, ref, depth)...)
		if out.err == nil {
			return nil
		}
		// A healthy mirror is never rebuilt: a rejected credential or a missing ref
		// fails the same way after a second clone. A mirror git cannot even open is
		// damaged instead, and the repair below is cheaper than reporting a
		// confusing "not a git repository".
		if usableMirror(dir) && !strings.Contains(strings.ToLower(out.stderr), "not a git repository") {
			return c.fail(ctx, "fetch", out)
		}
	}
	if err := c.rebuildMirror(ctx, repoURL, dir, ref, depth); err != nil {
		return err
	}
	// The clone only populates the object store; this fetch writes the local ref
	// the log reads. A plain source name is resolved by git against refs/heads and
	// refs/tags, so a release that references a tag works as well.
	out := c.runGit(ctx, fetchArgs(dir, ref, depth)...)
	if out.err != nil {
		return c.fail(ctx, "fetch", out)
	}
	return nil
}

// rebuildMirror drops dir and clones it again. The removal is not optional: git
// clone refuses to write into a directory that is not empty.
func (c *Client) rebuildMirror(ctx context.Context, repoURL string, dir string, ref string, depth int) error {
	if err := os.RemoveAll(dir); err != nil {
		return fmt.Errorf("清理 Git 缓存目录失败：%v", err)
	}
	out := c.runGit(ctx, cloneArgs(ref, repoURL, dir, depth)...)
	if out.err != nil {
		// Keep a half written mirror out of the cache so the next call clones again
		// instead of fetching into the leftovers.
		_ = os.RemoveAll(dir)
		return c.fail(ctx, "clone", out)
	}
	return nil
}

// runGit executes one git command with the credential injected through the
// environment. cmd args, env and working directory are fixed here so no caller
// can leak the secret into a command line (visible in /proc/<pid>/cmdline) or
// into a git credential helper's store.
func (c *Client) runGit(ctx context.Context, args ...string) gitOutput {
	// An empty helper resets any helper configured by a user or system gitconfig.
	argv := append([]string{"-c", "credential.helper="}, args...)
	cmd := exec.CommandContext(ctx, gitBinary, argv...)
	cmd.Dir = c.cacheDir
	cmd.Env = c.env()

	var stdout, stderr limitedBuffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return gitOutput{stdout: stdout.String(), stderr: stderr.String(), err: err}
}

// ownedEnvPrefixes are the variables env sets itself. An inherited duplicate
// would win over the value set here, because a process sees the first match of a
// key, so they are dropped before the credential is added.
var ownedEnvPrefixes = []string{
	"GIT_ASKPASS=",
	"GIT_TERMINAL_PROMPT=",
	"GIT_CONFIG_NOSYSTEM=",
	"GIT_CONFIG_COUNT=",
	"GIT_CONFIG_KEY_",
	"GIT_CONFIG_VALUE_",
}

// env builds the child environment. The Authorization header travels through
// GIT_CONFIG_COUNT/GIT_CONFIG_KEY_0/GIT_CONFIG_VALUE_0 (git >= 2.31), which keeps
// it out of argv, and the remaining variables remove every interactive prompt so
// a rejected credential fails fast instead of hanging on a password question.
func (c *Client) env() []string {
	inherited := os.Environ()
	environ := make([]string, 0, len(inherited)+6)
	for _, item := range inherited {
		if isOwnedEnv(item) {
			continue
		}
		environ = append(environ, item)
	}
	environ = append(environ,
		"GIT_TERMINAL_PROMPT=0",
		"GIT_CONFIG_NOSYSTEM=1",
	)
	// GIT_ASKPASS is only set when the helper exists: git fails the whole command
	// with "cannot exec" when the configured program is missing, which would turn
	// an authentication failure into a confusing error.
	if stat, err := os.Stat(askPassHelper); err == nil && !stat.IsDir() {
		environ = append(environ, "GIT_ASKPASS="+askPassHelper)
	}
	if c.authHeader != "" {
		environ = append(environ,
			"GIT_CONFIG_COUNT=1",
			"GIT_CONFIG_KEY_0=http.extraHeader",
			"GIT_CONFIG_VALUE_0="+c.authHeader,
		)
	} else {
		environ = append(environ, "GIT_CONFIG_COUNT=0")
	}
	return environ
}

// askPassHelper is the non-interactive askpass program. It is part of the
// runtime image (rockylinux coreutils); a machine without it simply runs git
// with terminal prompting disabled.
const askPassHelper = "/bin/true"

func isOwnedEnv(item string) bool {
	for _, prefix := range ownedEnvPrefixes {
		if strings.HasPrefix(item, prefix) {
			return true
		}
	}
	return false
}

// mirrorDir maps one repository to its mirror directory. The key is the base URL
// plus the project path and deliberately not the secret, so the same repository
// read through two credentials reuses one mirror; the SHA-1 is a cache key, not
// a security boundary.
func (c *Client) mirrorDir(projectPath string) string {
	sum := sha1.Sum([]byte(c.baseURL + "|" + projectPath))
	return filepath.Join(c.cacheDir, hex.EncodeToString(sum[:]))
}

// repoURL is the clone URL of the project. Credentials are never embedded in it:
// a password with special characters breaks git's URL parsing ("Could not
// resolve host: @git.cloud.local"), which is why the Authorization header is
// passed separately.
func (c *Client) repoURL(projectPath string) string {
	return c.baseURL + "/" + projectPath + ".git"
}

// fail turns a failed git invocation into a readable error. The output is
// sanitised first, so neither the error message nor a log line can carry the
// account password.
func (c *Client) fail(ctx context.Context, action string, out gitOutput) error {
	// The caller's context wins: a cancelled request is reported as such by the use
	// case, not as a git failure.
	if ctx.Err() != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return fmt.Errorf("%w（git %s）", ErrTimeout, action)
		}
		return ctx.Err()
	}
	if out.err != nil && (errors.Is(out.err, exec.ErrNotFound) || errors.Is(out.err, os.ErrNotExist)) && strings.TrimSpace(out.stderr) == "" {
		return fmt.Errorf("%w：%v", ErrGitUnavailable, out.err)
	}
	// The classification looks at the whole output: git prints progress first and
	// the fatal line last, so a first-line match would miss every failure.
	output := c.sanitize(out.stderr)
	if strings.TrimSpace(output) == "" {
		output = c.sanitize(out.stdout)
	}
	detail := gitDetail(output)
	switch {
	case authFailurePattern.MatchString(output):
		return fmt.Errorf("%w（git %s：%s）", ErrAuthenticationFailed, action, detail)
	case isMissingRef(output):
		return fmt.Errorf("%w（git %s：%s）", ErrBranchNotFound, action, detail)
	}
	if detail == "" {
		return fmt.Errorf("git %s 失败：%v", action, out.err)
	}
	return fmt.Errorf("git %s 失败：%s", action, detail)
}

// sanitize removes the credential from git output. git echoes the remote URL and
// some failures print the rejected Authorization header, so both the password
// and the full header value are masked before the text is used anywhere.
func (c *Client) sanitize(text string) string {
	if text == "" {
		return ""
	}
	if c.secret != "" {
		text = strings.ReplaceAll(text, c.secret, "***")
	}
	if c.authHeader != "" {
		text = strings.ReplaceAll(text, c.authHeader, "***")
	}
	return text
}

// gitOutput is the result of one git invocation.
type gitOutput struct {
	stdout string
	stderr string
	err    error
}

// cloneArgs clones the mirror. --single-branch with a branch name keeps a cold
// cache from downloading every branch of a release repository.
func cloneArgs(ref string, repoURL string, dir string, depth int) []string {
	args := []string{"clone", "--bare", "--depth=" + strconv.Itoa(depth), "--single-branch"}
	if ref != "" {
		args = append(args, "--branch", ref)
	}
	return append(args, repoURL, dir)
}

// fetchArgs fetches the ref into the local ref revFor returns, so the revision
// the log runs against is always the freshly fetched tip. A plain source name is
// used on purpose: git resolves it against refs/heads and refs/tags, so a release
// that references a tag works as well. An empty ref keeps the remote's own
// refspec, which updates the default branch.
func fetchArgs(dir string, ref string, depth int) []string {
	args := []string{"--git-dir=" + dir, "fetch", "--depth=" + strconv.Itoa(depth)}
	if ref == "" {
		return append(args, "origin")
	}
	return append(args, "origin", "+"+ref+":"+revFor(ref))
}

// deepenArgs extends the shallow boundary of an existing mirror by step commits.
// It mirrors fetchArgs' refspec so the local ref the log reads is updated as
// well: a plain source name is resolved against refs/heads and refs/tags, and an
// empty ref keeps the remote's own refspec.
func deepenArgs(dir string, ref string, step int) []string {
	args := []string{"--git-dir=" + dir, "fetch", "--deepen=" + strconv.Itoa(step)}
	if ref == "" {
		return append(args, "origin")
	}
	return append(args, "origin", "+"+ref+":"+revFor(ref))
}

// logArgs reads the newest commits of the mirror's local ref.
func logArgs(dir string, rev string, limit int) []string {
	return []string{
		"--git-dir=" + dir,
		"log",
		"-n", strconv.Itoa(limit),
		"--date=iso-strict",
		"-z",
		"--format=" + logFormat,
		rev,
	}
}

// revFor is the local ref the fetch writes and the log reads. It is always fully
// qualified so a branch and a tag with the same name cannot be confused by the
// revision lookup rules; an empty ref logs HEAD, which is the default branch of
// the mirror.
func revFor(ref string) string {
	if ref == "" {
		return "HEAD"
	}
	return "refs/heads/" + ref
}

// parseLog turns the -z separated git log payload into commits. Records with a
// missing field are skipped rather than failing the whole read.
func parseLog(output string, webURLBase string) []gitlab.Commit {
	records := strings.Split(output, logRecordSeparator)
	commits := make([]gitlab.Commit, 0, len(records))
	for _, record := range records {
		// %B ends with a newline and git log -z adds one more as the record
		// terminator; both are noise for the caller.
		record = strings.TrimRight(record, "\n")
		if strings.TrimSpace(record) == "" {
			continue
		}
		fields := strings.SplitN(record, logFieldSeparator, 7)
		if len(fields) < 6 {
			continue
		}
		commit := gitlab.Commit{
			ID:          strings.TrimSpace(fields[0]),
			ShortID:     strings.TrimSpace(fields[1]),
			Title:       strings.TrimSpace(fields[2]),
			AuthorName:  strings.TrimSpace(fields[3]),
			AuthorEmail: strings.TrimSpace(fields[4]),
			CommittedAt: parseCommitTime(fields[5]),
		}
		if len(fields) == 7 {
			// SplitN keeps any separator inside the message intact.
			commit.Message = strings.TrimSpace(fields[6])
		}
		if commit.Message == "" {
			commit.Message = commit.Title
		}
		if commit.ID != "" {
			commit.WebURL = webURLBase + "/" + commit.ID
		}
		commits = append(commits, commit)
	}
	return commits
}

func parseCommitTime(value string) time.Time {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return time.Time{}
	}
	// %cI is strict ISO 8601, which is RFC 3339.
	if parsed, err := time.Parse(time.RFC3339, trimmed); err == nil {
		return parsed.UTC()
	}
	return time.Time{}
}

// normalizeProjectPath trims the separators and the ".git" suffix a project path
// may carry, so "/code/app/" and "code/app.git" describe the same mirror.
func normalizeProjectPath(projectPath string) string {
	return strings.TrimSuffix(strings.Trim(strings.TrimSpace(projectPath), "/"), ".git")
}

// isMissingRef matches the two ways git reports a ref the remote does not have.
func isMissingRef(detail string) bool {
	if branchMissingPattern.MatchString(detail) {
		return true
	}
	lower := strings.ToLower(detail)
	return strings.Contains(lower, "couldn't find remote ref") ||
		strings.Contains(lower, "could not find remote ref") ||
		strings.Contains(lower, "not found in upstream")
}

// usableMirror reports whether dir looks like a complete bare repository: an
// interrupted clone leaves a directory behind that git refuses to fetch into.
func usableMirror(dir string) bool {
	for _, entry := range []string{"HEAD", "config", "objects"} {
		if _, err := os.Stat(filepath.Join(dir, entry)); err != nil {
			return false
		}
	}
	return true
}

// gitDetail picks the line of git's output that explains the failure: the fatal
// (or error) line when there is one, otherwise the first line that carries text.
// Progress lines like "Cloning into ..." are therefore never reported as the
// reason of a failure.
func gitDetail(text string) string {
	fallback := ""
	for _, line := range strings.Split(text, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		lower := strings.ToLower(trimmed)
		if strings.HasPrefix(lower, "fatal:") || strings.HasPrefix(lower, "error:") || strings.Contains(lower, "not found") {
			return truncateDetail(trimmed)
		}
		if fallback == "" {
			fallback = truncateDetail(trimmed)
		}
	}
	return fallback
}

func truncateDetail(line string) string {
	if len(line) > maxDetailBytes {
		return line[:maxDetailBytes] + "..."
	}
	return line
}

// limitedBuffer buffers at most maxOutputBytes and silently drops the rest, so a
// chatty git process cannot grow the heap. It always reports every byte as
// written, which is what a child process expects from a pipe writer.
type limitedBuffer struct {
	buf bytes.Buffer
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	if remaining := maxOutputBytes - b.buf.Len(); remaining > 0 {
		if len(p) > remaining {
			p = p[:remaining]
		}
		b.buf.Write(p)
	}
	return len(p), nil
}

func (b *limitedBuffer) String() string {
	return b.buf.String()
}

// mirrorLock serialises the git processes of one mirror directory.
type mirrorLock struct {
	mu sync.Mutex
	// waiters counts the callers that hold or want the lock, so the registry can
	// drop a directory once nobody uses it.
	waiters int
}

var (
	mirrorLocksMu sync.Mutex
	mirrorLocks   = map[string]*mirrorLock{}
)

// lockMirror locks the mirror directory and returns its release function. The
// registry is package level because two clients built from different credentials
// resolve to the same directory (the key is the base URL plus the project path),
// so a per-client lock would let two processes clone into one object store.
func lockMirror(dir string) func() {
	mirrorLocksMu.Lock()
	lock, ok := mirrorLocks[dir]
	if !ok {
		lock = &mirrorLock{}
		mirrorLocks[dir] = lock
	}
	lock.waiters++
	mirrorLocksMu.Unlock()

	lock.mu.Lock()
	return func() {
		lock.mu.Unlock()
		mirrorLocksMu.Lock()
		lock.waiters--
		if lock.waiters == 0 {
			delete(mirrorLocks, dir)
		}
		mirrorLocksMu.Unlock()
	}
}
