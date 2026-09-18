package usecase

import (
	"context"
	"fmt"
	"strings"
	"time"

	domain "gos/internal/domain/release"
	"gos/internal/support/logx"
)

// headCommitResolveTimeout bounds one asynchronous HEAD resolution. The git
// protocol channel (password credentials) may have to clone or deepen a mirror,
// which is exactly the read that used to make a release order page slow, so the
// budget is generous; 120s matches the git CLI client's own command budget.
const headCommitResolveTimeout = 120 * time.Second

// ReleaseOrderHeadCommitResolver reads the current HEAD of one repository branch
// plus the newest non-merge commit at or before it. *GitCommitManager implements
// it, so the create path and the release order commit views share one credential
// selection and one channel selection.
type ReleaseOrderHeadCommitResolver interface {
	ResolveHeadCommit(ctx context.Context, repoURL string, ref string) (HeadCommit, error)
}

// SetHeadCommitResolver wires the create-time commit reader. The server calls it
// at wiring time; without it the create path simply leaves the columns empty.
func (uc *ReleaseOrderManager) SetHeadCommitResolver(resolver ReleaseOrderHeadCommitResolver) {
	if uc == nil {
		return
	}
	uc.headCommitResolver = resolver
}

// scheduleReleaseOrderHeadCommit resolves the branch HEAD of a just created
// release order in the background and stores it on the order, so every later
// list/detail read is a plain column read instead of a live git call.
//
// The resolution must never influence the create result: it runs after the order
// is committed, in its own context (the request context is already cancelled when
// the HTTP handler returns, and a git clone may legitimately outlive it), every
// failure is only logged, and the columns simply stay empty.
//
// repoURL may be empty when the caller does not have the application at hand; the
// background task then loads it itself, off the request path.
func (uc *ReleaseOrderManager) scheduleReleaseOrderHeadCommit(
	requestCtx context.Context,
	order domain.ReleaseOrder,
	repoURL string,
) {
	if uc == nil || uc.headCommitResolver == nil || uc.repo == nil {
		return
	}
	orderID := strings.TrimSpace(order.ID)
	if orderID == "" {
		return
	}
	applicationID := strings.TrimSpace(order.ApplicationID)
	orderNo := strings.TrimSpace(order.OrderNo)
	ref := strings.TrimSpace(order.GitRef)
	repoURL = strings.TrimSpace(repoURL)
	if requestCtx == nil {
		requestCtx = context.Background()
	}

	task := func() {
		defer func() {
			if recovered := recover(); recovered != nil {
				// A panic here must never take the process down: the create already
				// succeeded and the columns are simply not filled in.
				logx.Error("release_order", "head_commit_resolve_panicked", errorForPanic(recovered),
					logx.F("order_id", orderID),
					logx.F("order_no", orderNo),
				)
			}
		}()
		// context.WithoutCancel keeps the request values (trace/request ids) while
		// dropping its deadline, so the git read is not killed with the HTTP request.
		ctx, cancel := context.WithTimeout(context.WithoutCancel(requestCtx), headCommitResolveTimeout)
		defer cancel()

		targetRepoURL := repoURL
		if targetRepoURL == "" {
			loaded, err := uc.applicationRepoURL(ctx, applicationID)
			if err != nil {
				logx.Warn("release_order", "head_commit_application_load_failed",
					logx.F("order_id", orderID),
					logx.F("order_no", orderNo),
					logx.F("application_id", applicationID),
					logx.F("reason", err.Error()),
				)
				return
			}
			targetRepoURL = loaded
		}
		if targetRepoURL == "" {
			logx.Warn("release_order", "head_commit_repo_url_missing",
				logx.F("order_id", orderID),
				logx.F("order_no", orderNo),
				logx.F("application_id", applicationID),
			)
			return
		}

		head, err := uc.headCommitResolver.ResolveHeadCommit(ctx, targetRepoURL, ref)
		if err != nil {
			// 无凭证 / 网络失败 / 超时都只记日志：发布单已经创建成功，列保持空值。
			logx.Warn("release_order", "head_commit_resolve_failed",
				logx.F("order_id", orderID),
				logx.F("order_no", orderNo),
				logx.F("repository", targetRepoURL),
				logx.F("git_ref", ref),
				logx.F("reason", describeGitLabFailure(err)),
			)
			return
		}
		snapshot := domain.ReleaseOrderHeadCommit{
			CommitSHA: head.CommitSHA,
			// 解析器回填的分支名优先（它可能解析出默认分支），否则用建单时请求的分支。
			CommitRef:    firstNonEmpty(strings.TrimSpace(head.CommitRef), ref),
			ChangeSHA:    head.ChangeSHA,
			ChangeTitle:  head.ChangeTitle,
			ChangeAuthor: head.ChangeAuthor,
			ChangeAt:     head.ChangeAt,
			ChangeURL:    head.ChangeURL,
		}
		if err := uc.repo.UpdateHeadCommit(ctx, orderID, snapshot); err != nil {
			logx.Warn("release_order", "head_commit_store_failed",
				logx.F("order_id", orderID),
				logx.F("order_no", orderNo),
				logx.F("reason", err.Error()),
			)
			return
		}
		logx.Info("release_order", "head_commit_resolved",
			logx.F("order_id", orderID),
			logx.F("order_no", orderNo),
			logx.F("repository", targetRepoURL),
			logx.F("git_ref", ref),
			logx.F("head_commit_sha", snapshot.CommitSHA),
			logx.F("head_change_sha", snapshot.ChangeSHA),
		)
	}

	if uc.runAsync != nil {
		uc.runAsync(task)
		return
	}
	go task()
}

// applicationRepoURL loads the Git repository URL of one application for the
// background resolution. A missing repository/application is not an error the
// caller has to act on, so an empty URL is returned together with nil.
func (uc *ReleaseOrderManager) applicationRepoURL(ctx context.Context, applicationID string) (string, error) {
	if uc.appRepo == nil || applicationID == "" {
		return "", nil
	}
	application, err := uc.appRepo.GetByID(ctx, applicationID)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(application.RepoURL), nil
}

// errorForPanic turns a recovered panic value into an error the logger can print.
func errorForPanic(recovered any) error {
	if err, ok := recovered.(error); ok {
		return err
	}
	return fmt.Errorf("panic: %v", recovered)
}
