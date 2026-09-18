package usecase

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	appdomain "gos/internal/domain/application"
	gitcredentialdomain "gos/internal/domain/gitcredential"
	domain "gos/internal/domain/release"
	"gos/internal/infrastructure/gitlab"
)

const (
	headCommitTestRepoURL = "http://git.cloud.local:9080/code/bigData/fusion-source-web.git"
	headCommitTestRef     = "release/2026-09-18"
	headCommitTestAppID   = "app-head-commit"
)

// headCommitResolverFake 记录创建流程调用的仓库与分支，并按测试需要返回快照或错误。
type headCommitResolverFake struct {
	head     HeadCommit
	err      error
	calls    int
	repoURLs []string
	refs     []string
}

func (f *headCommitResolverFake) ResolveHeadCommit(_ context.Context, repoURL string, ref string) (HeadCommit, error) {
	f.calls++
	f.repoURLs = append(f.repoURLs, repoURL)
	f.refs = append(f.refs, ref)
	if f.err != nil {
		return HeadCommit{}, f.err
	}
	return f.head, nil
}

func headCommitTestApp() appdomain.Application {
	return appdomain.Application{
		ID:      headCommitTestAppID,
		Name:    "fusion-source-web",
		Key:     "fusion-source-web",
		Status:  appdomain.StatusActive,
		RepoURL: headCommitTestRepoURL,
	}
}

// newHeadCommitCreateManager 准备一个创建流程可用的 manager：真实 sqlite 发布单仓储 +
// 同步执行的 runAsync，因此「异步落库」在测试里是确定性的。
func newHeadCommitCreateManager(t *testing.T, resolver ReleaseOrderHeadCommitResolver) (*ReleaseOrderManager, string) {
	t.Helper()

	manager, repo := newReleaseOrderManagerForCancelTest(t)
	ctx := context.Background()
	now := time.Now().UTC()
	manager.now = func() time.Time { return now }
	manager.appRepo = releaseOrderUpdateApplicationRepoStub{app: headCommitTestApp()}
	manager.SetHeadCommitResolver(resolver)

	template := domain.ReleaseTemplate{
		ID:              "rt-head-commit",
		Name:            "template-head-commit",
		ApplicationID:   headCommitTestAppID,
		ApplicationName: "fusion-source-web",
		BindingID:       headCommitTestAppID,
		BindingName:     "fusion-source-web",
		BindingType:     "application",
		Status:          domain.TemplateStatusActive,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	bindings := []domain.ReleaseTemplateBinding{
		{
			ID:            "rtb-head-commit-ci",
			TemplateID:    template.ID,
			PipelineScope: domain.PipelineScopeCI,
			BindingID:     "binding-head-commit-ci",
			BindingName:   "CI",
			Provider:      "jenkins",
			PipelineID:    "pipeline-head-commit-ci",
			Enabled:       true,
			SortNo:        1,
			CreatedAt:     now,
			UpdatedAt:     now,
		},
	}
	params := []domain.ReleaseTemplateParam{
		{
			ID:                 "rtp-head-commit-branch",
			TemplateID:         template.ID,
			TemplateBindingID:  bindings[0].ID,
			PipelineScope:      domain.PipelineScopeCI,
			BindingID:          bindings[0].BindingID,
			ExecutorParamDefID: "ep-head-commit-branch",
			ParamKey:           "branch",
			ParamName:          "分支",
			ExecutorParamName:  "BRANCH",
			ValueSource:        domain.TemplateParamValueSourceReleaseInput,
			Required:           true,
			SortNo:             1,
			CreatedAt:          now,
			UpdatedAt:          now,
		},
	}
	if err := repo.CreateTemplate(ctx, template, bindings, params, nil, nil); err != nil {
		t.Fatalf("CreateTemplate failed: %v", err)
	}
	return manager, template.ID
}

// headCommitCreateInput 组装一次建单输入：分支来自模板参数，因此发布单会带上 git_ref。
func headCommitCreateInput(templateID string, ref string) CreateReleaseOrderInput {
	return CreateReleaseOrderInput{
		ApplicationID: headCommitTestAppID,
		TemplateID:    templateID,
		ReleaseName:   "head commit release",
		EnvCode:       "prod",
		GitRef:        ref,
		CreatorUserID: "user-1",
		TriggeredBy:   "user-1",
		Params: []CreateReleaseOrderParamInput{
			{
				PipelineScope:     domain.PipelineScopeCI,
				ParamKey:          "branch",
				ExecutorParamName: "BRANCH",
				ParamValue:        ref,
				ValueSource:       domain.ValueSourceReleaseInput,
			},
		},
	}
}

// TestCreateReleaseOrderStoresResolvedHeadCommit 覆盖「创建成功后异步解析并落库」：
// HEAD 与 change 必须分别是两条提交，仓库地址取应用 repo_url，分支取发布单 git_ref。
func TestCreateReleaseOrderStoresResolvedHeadCommit(t *testing.T) {
	t.Parallel()

	changeAt := time.Date(2026, 9, 18, 9, 30, 0, 0, time.UTC)
	resolver := &headCommitResolverFake{head: HeadCommit{
		CommitSHA:    "sha-head-0001",
		ChangeSHA:    "sha-change-0002",
		ChangeTitle:  "feat: 支持发布单 HEAD 落库",
		ChangeAuthor: "Alice",
		ChangeAt:     &changeAt,
		ChangeURL:    "http://git.cloud.local:9080/code/bigData/fusion-source-web/-/commit/sha-change-0002",
	}}
	manager, templateID := newHeadCommitCreateManager(t, resolver)

	order, err := manager.Create(context.Background(), headCommitCreateInput(templateID, headCommitTestRef))
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	if order.HeadCommitSHA != "sha-head-0001" {
		t.Fatalf("head_commit_sha = %q, want sha-head-0001", order.HeadCommitSHA)
	}
	if order.HeadCommitRef != headCommitTestRef {
		t.Fatalf("head_commit_ref = %q, want %q", order.HeadCommitRef, headCommitTestRef)
	}
	if order.HeadChangeSHA != "sha-change-0002" || order.HeadChangeTitle != "feat: 支持发布单 HEAD 落库" || order.HeadChangeAuthor != "Alice" {
		t.Fatalf("head_change_* = %q / %q / %q", order.HeadChangeSHA, order.HeadChangeTitle, order.HeadChangeAuthor)
	}
	if order.HeadChangeAt == nil || !order.HeadChangeAt.Equal(changeAt) {
		t.Fatalf("head_change_at = %v, want %v", order.HeadChangeAt, changeAt)
	}
	if order.HeadChangeURL != "http://git.cloud.local:9080/code/bigData/fusion-source-web/-/commit/sha-change-0002" {
		t.Fatalf("head_change_url = %q", order.HeadChangeURL)
	}
	if len(resolver.repoURLs) != 1 || resolver.repoURLs[0] != headCommitTestRepoURL {
		t.Fatalf("resolved repo urls = %#v, want the application repo_url", resolver.repoURLs)
	}
	if len(resolver.refs) != 1 || resolver.refs[0] != headCommitTestRef {
		t.Fatalf("resolved refs = %#v, want the release order git_ref", resolver.refs)
	}
}

// TestCreateReleaseOrderKeepsOrderWhenHeadCommitResolutionFails 覆盖「解析失败不能影响建单」：
// 单子照常返回，HEAD 列保持为空。
func TestCreateReleaseOrderKeepsOrderWhenHeadCommitResolutionFails(t *testing.T) {
	t.Parallel()

	resolver := &headCommitResolverFake{err: errors.New("未找到匹配的 Git 凭证（按仓库地址前缀匹配）")}
	manager, templateID := newHeadCommitCreateManager(t, resolver)

	order, err := manager.Create(context.Background(), headCommitCreateInput(templateID, headCommitTestRef))
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	if order.ID == "" {
		t.Fatal("Create returned an empty order id")
	}
	if resolver.calls != 1 {
		t.Fatalf("resolver calls = %d, want 1", resolver.calls)
	}
	if order.HeadCommitSHA != "" || order.HeadCommitRef != "" || order.HeadChangeSHA != "" ||
		order.HeadChangeTitle != "" || order.HeadChangeAuthor != "" || order.HeadChangeURL != "" ||
		order.HeadChangeAt != nil {
		t.Fatalf("head commit columns must stay empty after a failed resolution: %+v", order)
	}
}

// TestCreateReleaseOrderSkipsHeadCommitResolutionWithoutRepoURL 覆盖应用未配置仓库地址：
// 不发起解析，建单仍然成功。
func TestCreateReleaseOrderSkipsHeadCommitResolutionWithoutRepoURL(t *testing.T) {
	t.Parallel()

	resolver := &headCommitResolverFake{head: HeadCommit{CommitSHA: "sha-head"}}
	manager, templateID := newHeadCommitCreateManager(t, resolver)
	manager.appRepo = releaseOrderUpdateApplicationRepoStub{app: appdomain.Application{
		ID:     headCommitTestAppID,
		Name:   "fusion-source-web",
		Key:    "fusion-source-web",
		Status: appdomain.StatusActive,
	}}

	order, err := manager.Create(context.Background(), headCommitCreateInput(templateID, headCommitTestRef))
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	if resolver.calls != 0 {
		t.Fatalf("resolver calls = %d, want 0 when the application has no repository url", resolver.calls)
	}
	if order.HeadCommitSHA != "" {
		t.Fatalf("head_commit_sha = %q, want empty", order.HeadCommitSHA)
	}
}

// TestCreateReleaseOrderWithoutHeadCommitResolverStillCreates 覆盖未注入解析器的情况
// （server 未接线 / 单测里的裸 manager）：建单链路必须照常工作。
func TestCreateReleaseOrderWithoutHeadCommitResolverStillCreates(t *testing.T) {
	t.Parallel()

	manager, templateID := newHeadCommitCreateManager(t, nil)

	order, err := manager.Create(context.Background(), headCommitCreateInput(templateID, headCommitTestRef))
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	if order.HeadCommitSHA != "" {
		t.Fatalf("head_commit_sha = %q, want empty", order.HeadCommitSHA)
	}
}

func headCommitClient(commits []gitlab.Commit, err error) *gitCommitClientFake {
	return &gitCommitClientFake{commits: commits, err: err}
}

func newHeadCommitGitCommitManager(credentials []gitcredentialdomain.Credential, client *gitCommitClientFake) *GitCommitManager {
	manager := NewGitCommitManager(nil, nil, &gitCommitCredentialRepoFake{items: credentials})
	manager.newFetcher = func(gitcredentialdomain.Credential) GitCommitFetcher { return client }
	return manager
}

func headCommitTestCredential() gitcredentialdomain.Credential {
	return gitcredentialdomain.Credential{
		ID: "gc-cloud", Name: "cloud", Provider: gitcredentialdomain.ProviderGitLab,
		BaseURL: "http://git.cloud.local:9080", Secret: "s",
		AuthType: gitcredentialdomain.AuthTypeToken, Status: gitcredentialdomain.StatusActive,
	}
}

// TestGitCommitManagerResolveHeadCommitSeparatesHeadFromChange 覆盖「HEAD 自身」与
// 「HEAD 之前（含 HEAD）最新一条非 merge 提交」的区分，以及提交页面地址的拼装规则。
func TestGitCommitManagerResolveHeadCommitSeparatesHeadFromChange(t *testing.T) {
	t.Parallel()

	now := time.Unix(9000, 0).UTC()
	client := headCommitClient([]gitlab.Commit{
		{ID: "sha-head", ShortID: "sha-head", Title: "Merge branch 'release/x' into 'main'", AuthorName: "Merger", CommittedAt: now},
		{ID: "sha-change", ShortID: "sha-change", Title: "feat: 支持批量发布", AuthorName: "Alice", CommittedAt: now.Add(-time.Hour)},
		{ID: "sha-old", ShortID: "sha-old", Title: "chore: bump", AuthorName: "Bob", CommittedAt: now.Add(-2 * time.Hour)},
	}, nil)
	manager := newHeadCommitGitCommitManager([]gitcredentialdomain.Credential{headCommitTestCredential()}, client)

	head, err := manager.ResolveHeadCommit(context.Background(), headCommitTestRepoURL, "main")
	if err != nil {
		t.Fatalf("ResolveHeadCommit failed: %v", err)
	}
	if head.CommitSHA != "sha-head" {
		t.Fatalf("commit sha = %q, want sha-head", head.CommitSHA)
	}
	if head.CommitRef != "main" {
		t.Fatalf("commit ref = %q, want main", head.CommitRef)
	}
	if head.ChangeSHA != "sha-change" || head.ChangeTitle != "feat: 支持批量发布" || head.ChangeAuthor != "Alice" {
		t.Fatalf("change = %q / %q / %q", head.ChangeSHA, head.ChangeTitle, head.ChangeAuthor)
	}
	if head.ChangeAt == nil || !head.ChangeAt.Equal(now.Add(-time.Hour)) {
		t.Fatalf("change at = %v, want %v", head.ChangeAt, now.Add(-time.Hour))
	}
	if head.ChangeURL != "http://git.cloud.local:9080/code/bigData/fusion-source-web/-/commit/sha-change" {
		t.Fatalf("change url = %q", head.ChangeURL)
	}
	project, ref, limit, asOf := client.lastRequest()
	if project != "code/bigData/fusion-source-web" || ref != "main" {
		t.Fatalf("read %q/%q, want the application project path and the requested ref", project, ref)
	}
	if limit != gitCommitHeadWindow {
		t.Fatalf("read limit = %d, want %d", limit, gitCommitHeadWindow)
	}
	if asOf != nil {
		t.Fatalf("create-time read must not be anchored in time, got asOf=%v", asOf)
	}
}

// TestGitCommitManagerResolveHeadCommitFallsBackToHead 覆盖窗口里只有 merge 提交的情况：
// change 退化成 HEAD 自身，而不是留空。
func TestGitCommitManagerResolveHeadCommitFallsBackToHead(t *testing.T) {
	t.Parallel()

	now := time.Unix(9100, 0).UTC()
	client := headCommitClient([]gitlab.Commit{
		{ID: "sha-head", ShortID: "sha-head", Title: "Merge branch 'a' into 'main'", CommittedAt: now},
		{ID: "sha-merge-2", ShortID: "sha-merge-2", Title: "Merge branch 'b' into 'main'", CommittedAt: now.Add(-time.Hour)},
	}, nil)
	manager := newHeadCommitGitCommitManager([]gitcredentialdomain.Credential{headCommitTestCredential()}, client)

	head, err := manager.ResolveHeadCommit(context.Background(), headCommitTestRepoURL, "main")
	if err != nil {
		t.Fatalf("ResolveHeadCommit failed: %v", err)
	}
	if head.ChangeSHA != "sha-head" || head.ChangeTitle != "Merge branch 'a' into 'main'" {
		t.Fatalf("change = %q / %q, want the head itself", head.ChangeSHA, head.ChangeTitle)
	}
}

// TestGitCommitManagerResolveHeadCommitPrefersChannelWebURL 覆盖通道自带 web_url 的情况：
// GitLab 自己给出的提交地址优先于本地拼装。
func TestGitCommitManagerResolveHeadCommitPrefersChannelWebURL(t *testing.T) {
	t.Parallel()

	now := time.Unix(9200, 0).UTC()
	client := headCommitClient([]gitlab.Commit{
		{ID: "sha-head", ShortID: "sha-head", Title: "feat: x", CommittedAt: now, WebURL: "http://git.example.com/group/app/-/commit/sha-head"},
	}, nil)
	manager := newHeadCommitGitCommitManager([]gitcredentialdomain.Credential{headCommitTestCredential()}, client)

	head, err := manager.ResolveHeadCommit(context.Background(), headCommitTestRepoURL, "main")
	if err != nil {
		t.Fatalf("ResolveHeadCommit failed: %v", err)
	}
	if head.ChangeURL != "http://git.example.com/group/app/-/commit/sha-head" {
		t.Fatalf("change url = %q, want the channel web url", head.ChangeURL)
	}
}

// TestGitCommitManagerResolveHeadCommitRequiresCredential 覆盖无匹配凭证：
// 直接报错，且不发起任何远端读取。
func TestGitCommitManagerResolveHeadCommitRequiresCredential(t *testing.T) {
	t.Parallel()

	client := headCommitClient(nil, nil)
	manager := newHeadCommitGitCommitManager(nil, client)

	_, err := manager.ResolveHeadCommit(context.Background(), headCommitTestRepoURL, "main")
	if err == nil {
		t.Fatal("ResolveHeadCommit err = nil, want a missing-credential error")
	}
	if !strings.Contains(err.Error(), "Git 凭证") {
		t.Fatalf("err = %v, want a credential error", err)
	}
	if client.callCount() != 0 {
		t.Fatalf("channel calls = %d, want 0", client.callCount())
	}
}

// TestGitCommitManagerResolveHeadCommitReportsChannelFailure 覆盖网络/鉴权失败：
// 错误向上返回并带上读取上下文，由创建流程只记日志。
func TestGitCommitManagerResolveHeadCommitReportsChannelFailure(t *testing.T) {
	t.Parallel()

	client := headCommitClient(nil, errors.New("gitlab request failed: status=401"))
	manager := newHeadCommitGitCommitManager([]gitcredentialdomain.Credential{headCommitTestCredential()}, client)

	_, err := manager.ResolveHeadCommit(context.Background(), headCommitTestRepoURL, "main")
	if err == nil {
		t.Fatal("ResolveHeadCommit err = nil, want the channel failure")
	}
	if !strings.Contains(err.Error(), "读取分支 HEAD 失败") {
		t.Fatalf("err = %v, want the wrapped channel failure", err)
	}
}

// TestGitCommitManagerResolveHeadCommitRejectsEmptyRepoURL 覆盖应用未配置仓库地址。
func TestGitCommitManagerResolveHeadCommitRejectsEmptyRepoURL(t *testing.T) {
	t.Parallel()

	client := headCommitClient(nil, nil)
	manager := newHeadCommitGitCommitManager([]gitcredentialdomain.Credential{headCommitTestCredential()}, client)

	if _, err := manager.ResolveHeadCommit(context.Background(), "  ", "main"); err == nil {
		t.Fatal("ResolveHeadCommit err = nil, want a repository url error")
	}
	if client.callCount() != 0 {
		t.Fatalf("channel calls = %d, want 0", client.callCount())
	}
}
