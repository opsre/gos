package usecase

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	appdomain "gos/internal/domain/application"
	releasedomain "gos/internal/domain/release"
	automationdomain "gos/internal/domain/releaseautomation"
)

const (
	releaseAutomationTestAppID      = "app-automation"
	releaseAutomationTestRepoURL    = "http://git.cloud.local:9080/code/bigData/fusion-source-web.git"
	releaseAutomationTestTemplateID = "rt-automation"
	releaseAutomationTestEnvCode    = "prod"
	releaseAutomationTestGitRef     = "release/2026-09-18"
)

// releaseAutomationRepoFake 是内存版仓储，唯一键、检查结果刷新与 CAS 写回都按真实语义
// 实现：CAS 只有 expectedSeenSHA 与当前基线一致时才推进，测试用它区分「重复建单」和
// 「丢弃本次结果」。
type releaseAutomationRepoFake struct {
	mu      sync.Mutex
	items   map[string]automationdomain.Automation
	order   []string
	created int

	createErr error
	commitErr error
	// commitResults 可选：按顺序覆写 CAS 结果（false 表示抢占失败）。
	commitResults []bool
	commitCalls   int
	checkStates   int
}

func newReleaseAutomationRepoFake() *releaseAutomationRepoFake {
	return &releaseAutomationRepoFake{items: make(map[string]automationdomain.Automation)}
}

func (r *releaseAutomationRepoFake) InitSchema(context.Context) error { return nil }

func (r *releaseAutomationRepoFake) Create(_ context.Context, item automationdomain.Automation) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.createErr != nil {
		return r.createErr
	}
	for _, existing := range r.items {
		if existing.ApplicationID == item.ApplicationID &&
			existing.EnvCode == item.EnvCode &&
			existing.GitRef == item.GitRef {
			return automationdomain.ErrDuplicated
		}
	}
	r.created++
	r.items[item.ID] = item
	r.order = append(r.order, item.ID)
	return nil
}

func (r *releaseAutomationRepoFake) GetByID(_ context.Context, id string) (automationdomain.Automation, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	item, ok := r.items[id]
	if !ok {
		return automationdomain.Automation{}, automationdomain.ErrNotFound
	}
	return item, nil
}

func (r *releaseAutomationRepoFake) List(_ context.Context, _ automationdomain.ListFilter) ([]automationdomain.Automation, int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	items := make([]automationdomain.Automation, 0, len(r.order))
	for _, id := range r.order {
		items = append(items, r.items[id])
	}
	return items, int64(len(items)), nil
}

func (r *releaseAutomationRepoFake) Update(_ context.Context, item automationdomain.Automation) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.items[item.ID]; !ok {
		return automationdomain.ErrNotFound
	}
	for id, existing := range r.items {
		if id == item.ID {
			continue
		}
		if existing.ApplicationID == item.ApplicationID &&
			existing.EnvCode == item.EnvCode &&
			existing.GitRef == item.GitRef {
			return automationdomain.ErrDuplicated
		}
	}
	r.items[item.ID] = item
	return nil
}

func (r *releaseAutomationRepoFake) Delete(_ context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.items[id]; !ok {
		return automationdomain.ErrNotFound
	}
	delete(r.items, id)
	return nil
}

func (r *releaseAutomationRepoFake) ListEnabled(_ context.Context, limit int) ([]automationdomain.Automation, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	items := make([]automationdomain.Automation, 0, len(r.order))
	for _, id := range r.order {
		item, ok := r.items[id]
		if !ok || !item.Enabled {
			continue
		}
		items = append(items, item)
		if len(items) >= limit {
			break
		}
	}
	return items, nil
}

func (r *releaseAutomationRepoFake) ResetBaseline(
	_ context.Context,
	id string,
	applicationID string,
	envCode string,
	gitRef string,
	expectedSeenSHA string,
	seenSHA string,
	updatedAt time.Time,
) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	item, ok := r.items[id]
	if !ok {
		return false, nil
	}
	if item.ApplicationID != applicationID || item.EnvCode != envCode || item.GitRef != gitRef {
		return false, nil
	}
	if item.LastSeenSHA != expectedSeenSHA {
		return false, nil
	}
	item.LastSeenSHA = seenSHA
	item.LastTriggeredSHA = ""
	item.LastOrderID = ""
	item.UpdatedAt = updatedAt
	r.items[id] = item
	return true, nil
}

func (r *releaseAutomationRepoFake) UpdateCheckState(
	_ context.Context,
	id string,
	checkedAt time.Time,
	lastError string,
) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	item, ok := r.items[id]
	if !ok {
		return automationdomain.ErrNotFound
	}
	r.checkStates++
	checked := checkedAt
	item.LastCheckedAt = &checked
	item.LastError = lastError
	r.items[id] = item
	return nil
}

func (r *releaseAutomationRepoFake) CommitTrigger(
	_ context.Context,
	id string,
	expectedSeenSHA string,
	seenSHA string,
	triggeredSHA string,
	orderID string,
	checkedAt time.Time,
	lastError string,
) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.commitCalls++
	if r.commitErr != nil {
		return false, r.commitErr
	}
	if len(r.commitResults) > 0 {
		scripted := r.commitResults[0]
		r.commitResults = r.commitResults[1:]
		if !scripted {
			return false, nil
		}
	}
	item, ok := r.items[id]
	if !ok {
		return false, nil
	}
	if item.LastSeenSHA != expectedSeenSHA {
		return false, nil
	}
	checked := checkedAt
	item.LastSeenSHA = seenSHA
	item.LastTriggeredSHA = triggeredSHA
	item.LastOrderID = orderID
	item.LastCheckedAt = &checked
	item.LastError = lastError
	r.items[id] = item
	return true, nil
}

type releaseAutomationAppStub struct {
	app appdomain.Application
	err error
}

func (s releaseAutomationAppStub) GetByID(context.Context, string) (appdomain.Application, error) {
	if s.err != nil {
		return appdomain.Application{}, s.err
	}
	return s.app, nil
}

type releaseAutomationTemplateStub struct {
	template releasedomain.ReleaseTemplate
	err      error
}

func (s releaseAutomationTemplateStub) GetTemplateByID(
	context.Context,
	string,
) (releasedomain.ReleaseTemplate, []releasedomain.ReleaseTemplateBinding, []releasedomain.ReleaseTemplateParam, []releasedomain.ReleaseTemplateGitOpsRule, []releasedomain.ReleaseTemplateHook, error) {
	if s.err != nil {
		return releasedomain.ReleaseTemplate{}, nil, nil, nil, nil, s.err
	}
	return s.template, nil, nil, nil, nil, nil
}

// releaseAutomationOrderStub 记录自动化调用的建单与派发动作，并可以模拟在途单、
// 并发冲突和 panic。
type releaseAutomationOrderStub struct {
	created    []CreateReleaseOrderInput
	actions    []string
	activeErr  error
	activeNo   string
	createErr  error
	buildErr   error
	deployErr  error
	executeErr error
	buildState releasedomain.OrderStatus
	// autoDeployStatus 模拟上一轮建的单现在停在什么状态：只有停在待部署才会被自动续跑。
	autoDeployStatus releasedomain.OrderStatus
	autoDeployErr    error
	panicOnRun       bool
	panicOnce        bool
	panicked         bool
}

func (s *releaseAutomationOrderStub) Create(_ context.Context, input CreateReleaseOrderInput) (releasedomain.ReleaseOrder, error) {
	if s.panicOnRun && !s.panicked {
		s.panicked = true
		panic("release order create exploded")
	}
	if s.createErr != nil {
		return releasedomain.ReleaseOrder{}, s.createErr
	}
	s.created = append(s.created, input)
	return releasedomain.ReleaseOrder{
		ID:          "ro-auto-1",
		OrderNo:     "RO-AUTO-1",
		Status:      releasedomain.OrderStatusPending,
		GitRef:      input.GitRef,
		EnvCode:     input.EnvCode,
		TriggerType: input.TriggerType,
	}, nil
}

func (s *releaseAutomationOrderStub) Build(context.Context, string, string, string) (releasedomain.ReleaseOrder, error) {
	if s.buildErr != nil {
		return releasedomain.ReleaseOrder{}, s.buildErr
	}
	s.actions = append(s.actions, "build")
	status := s.buildState
	if status == "" {
		status = releasedomain.OrderStatusBuilding
	}
	return releasedomain.ReleaseOrder{ID: "ro-auto-1", OrderNo: "RO-AUTO-1", Status: status}, nil
}

func (s *releaseAutomationOrderStub) Deploy(context.Context, string, string, string) (releasedomain.ReleaseOrder, error) {
	if s.deployErr != nil {
		return releasedomain.ReleaseOrder{}, s.deployErr
	}
	s.actions = append(s.actions, "deploy")
	return releasedomain.ReleaseOrder{ID: "ro-auto-1", OrderNo: "RO-AUTO-1", Status: releasedomain.OrderStatusDeploying}, nil
}

func (s *releaseAutomationOrderStub) AutoDeployBuiltOrder(_ context.Context, _ string, _ string, _ string) (bool, error) {
	if s.autoDeployErr != nil {
		return false, s.autoDeployErr
	}
	if s.autoDeployStatus != releasedomain.OrderStatusBuiltWaitingDeploy {
		return false, nil
	}
	s.actions = append(s.actions, "deploy")
	return true, nil
}

func (s *releaseAutomationOrderStub) Execute(context.Context, string, string, string) (releasedomain.ReleaseOrder, error) {
	if s.executeErr != nil {
		return releasedomain.ReleaseOrder{}, s.executeErr
	}
	s.actions = append(s.actions, "execute")
	return releasedomain.ReleaseOrder{ID: "ro-auto-1", OrderNo: "RO-AUTO-1", Status: releasedomain.OrderStatusRunning}, nil
}

// FindActiveOrderByApplicationEnv 默认表示「没有在途单」，只有测试显式设了
// activeNo 才返回一条阻塞单。
func (s *releaseAutomationOrderStub) FindActiveOrderByApplicationEnv(context.Context, string, string) (releasedomain.ReleaseOrder, error) {
	if s.activeErr != nil {
		return releasedomain.ReleaseOrder{}, s.activeErr
	}
	if strings.TrimSpace(s.activeNo) == "" {
		return releasedomain.ReleaseOrder{}, releasedomain.ErrOrderNotFound
	}
	return releasedomain.ReleaseOrder{ID: "ro-active", OrderNo: s.activeNo}, nil
}

func (s *releaseAutomationOrderStub) FindOpenOrderByApplicationEnv(context.Context, string, string, string) (releasedomain.ReleaseOrder, error) {
	if s.activeErr != nil {
		return releasedomain.ReleaseOrder{}, s.activeErr
	}
	if strings.TrimSpace(s.activeNo) == "" {
		return releasedomain.ReleaseOrder{}, releasedomain.ErrOrderNotFound
	}
	return releasedomain.ReleaseOrder{ID: "ro-active", OrderNo: s.activeNo}, nil
}

type releaseAutomationGitStub struct {
	head      HeadCommit
	err       error
	headByRef map[string]HeadCommit
	calls     int
}

func (s *releaseAutomationGitStub) ResolveHeadCommit(_ context.Context, _ string, ref string) (HeadCommit, error) {
	s.calls++
	if s.err != nil {
		return HeadCommit{}, s.err
	}
	if s.headByRef != nil {
		if head, ok := s.headByRef[ref]; ok {
			return head, nil
		}
	}
	return s.head, nil
}

func newReleaseAutomationTestManager(
	t *testing.T,
	repo *releaseAutomationRepoFake,
	orders *releaseAutomationOrderStub,
	git *releaseAutomationGitStub,
) *ReleaseAutomationManager {
	t.Helper()
	manager := NewReleaseAutomationManager(
		repo,
		releaseAutomationAppStub{app: appdomain.Application{
			ID:      releaseAutomationTestAppID,
			Name:    "fusion-source-web",
			Status:  appdomain.StatusActive,
			RepoURL: releaseAutomationTestRepoURL,
		}},
		releaseAutomationTemplateStub{template: releasedomain.ReleaseTemplate{
			ID:            releaseAutomationTestTemplateID,
			Name:          "生产发布模板",
			ApplicationID: releaseAutomationTestAppID,
			Status:        releasedomain.TemplateStatusActive,
		}},
		orders,
		git,
	)
	manager.now = func() time.Time { return time.Unix(1700000000, 0).UTC() }
	return manager
}

func releaseAutomationTestInput() ReleaseAutomationInput {
	return ReleaseAutomationInput{
		Name:           "前端生产自动发布",
		ApplicationID:  releaseAutomationTestAppID,
		TemplateID:     releaseAutomationTestTemplateID,
		EnvCode:        releaseAutomationTestEnvCode,
		GitRef:         releaseAutomationTestGitRef,
		DispatchMode:   automationdomain.DispatchModeBuild,
		OperatorUserID: "usr-1",
		OperatorName:   "张三",
	}
}

// 首次保存只把当前 HEAD 落成基线，不能顺手发一单。
func TestReleaseAutomationCreateRecordsBaselineWithoutTriggering(t *testing.T) {
	repo := newReleaseAutomationRepoFake()
	orders := &releaseAutomationOrderStub{}
	git := &releaseAutomationGitStub{head: HeadCommit{CommitSHA: "sha-1"}}
	manager := newReleaseAutomationTestManager(t, repo, orders, git)

	item, err := manager.Create(context.Background(), releaseAutomationTestInput())
	if err != nil {
		t.Fatalf("Create err = %v", err)
	}
	if item.LastSeenSHA != "sha-1" {
		t.Fatalf("LastSeenSHA = %q, want sha-1", item.LastSeenSHA)
	}
	if len(orders.created) != 0 {
		t.Fatalf("Create triggered %d release orders, want 0", len(orders.created))
	}
	if item.ApplicationName != "fusion-source-web" || item.TemplateName != "生产发布模板" {
		t.Fatalf("snapshot names = %q / %q", item.ApplicationName, item.TemplateName)
	}
	if item.CreatorUserID != "usr-1" {
		t.Fatalf("CreatorUserID = %q, want usr-1", item.CreatorUserID)
	}
}

func TestReleaseAutomationCreateRejectsWhenGitCheckFails(t *testing.T) {
	repo := newReleaseAutomationRepoFake()
	orders := &releaseAutomationOrderStub{}
	git := &releaseAutomationGitStub{err: errors.New("Git 凭证被拒绝或已过期")}
	manager := newReleaseAutomationTestManager(t, repo, orders, git)

	_, err := manager.Create(context.Background(), releaseAutomationTestInput())
	if !errors.Is(err, ErrAutomationGitUnreachable) {
		t.Fatalf("Create err = %v, want ErrAutomationGitUnreachable", err)
	}
	if !strings.Contains(err.Error(), "Git 凭证被拒绝或已过期") {
		t.Fatalf("Create err = %v, want the readable git reason", err)
	}
	if repo.created != 0 {
		t.Fatalf("created = %d records, want 0 (git check failed)", repo.created)
	}
}

func TestReleaseAutomationCreateRequiresTemplateOfTheSameApplication(t *testing.T) {
	repo := newReleaseAutomationRepoFake()
	manager := NewReleaseAutomationManager(
		repo,
		releaseAutomationAppStub{app: appdomain.Application{ID: "app-1", Name: "app"}},
		releaseAutomationTemplateStub{template: releasedomain.ReleaseTemplate{ID: "rt-1", Name: "tpl", ApplicationID: "app-2"}},
		&releaseAutomationOrderStub{},
		&releaseAutomationGitStub{head: HeadCommit{CommitSHA: "sha-1"}},
	)
	_, err := manager.Create(context.Background(), ReleaseAutomationInput{
		Name:          "x",
		ApplicationID: "app-1",
		TemplateID:    "rt-1",
		EnvCode:       "prod",
		GitRef:        "main",
	})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("Create err = %v, want ErrInvalidInput", err)
	}
	if repo.created != 0 {
		t.Fatalf("created = %d records, want 0", repo.created)
	}
}

// 分支 HEAD 没有变化时只刷新检查时间，不建单、不推进基线。
func TestReleaseAutomationRunKeepsBaselineWhenHeadUnchanged(t *testing.T) {
	repo := newReleaseAutomationRepoFake()
	orders := &releaseAutomationOrderStub{}
	git := &releaseAutomationGitStub{head: HeadCommit{CommitSHA: "sha-1"}}
	manager := newReleaseAutomationTestManager(t, repo, orders, git)

	created, err := manager.Create(context.Background(), releaseAutomationTestInput())
	if err != nil {
		t.Fatalf("Create err = %v", err)
	}

	output, err := manager.RunDueAutomations(context.Background(), 20)
	if err != nil {
		t.Fatalf("RunDueAutomations err = %v", err)
	}
	if output.Scanned != 1 || output.Unchanged != 1 || output.Triggered != 0 {
		t.Fatalf("output = %+v, want scanned=1 unchanged=1 triggered=0", output)
	}
	if len(orders.created) != 0 {
		t.Fatalf("created %d orders, want 0", len(orders.created))
	}
	current, err := repo.GetByID(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("GetByID err = %v", err)
	}
	if current.LastSeenSHA != "sha-1" {
		t.Fatalf("LastSeenSHA = %q, want sha-1", current.LastSeenSHA)
	}
	if current.LastCheckedAt == nil {
		t.Fatalf("LastCheckedAt was not refreshed")
	}
	if current.LastError != "" {
		t.Fatalf("LastError = %q, want empty", current.LastError)
	}
}

func TestReleaseAutomationRunTriggersWhenHeadMoves(t *testing.T) {
	repo := newReleaseAutomationRepoFake()
	orders := &releaseAutomationOrderStub{}
	git := &releaseAutomationGitStub{head: HeadCommit{CommitSHA: "sha-1"}}
	manager := newReleaseAutomationTestManager(t, repo, orders, git)

	created, err := manager.Create(context.Background(), releaseAutomationTestInput())
	if err != nil {
		t.Fatalf("Create err = %v", err)
	}
	// 分支前进了。
	git.head = HeadCommit{CommitSHA: "sha-2"}

	output, err := manager.RunDueAutomations(context.Background(), 20)
	if err != nil {
		t.Fatalf("RunDueAutomations err = %v", err)
	}
	if output.Triggered != 1 {
		t.Fatalf("output = %+v, want triggered=1", output)
	}
	if len(orders.created) != 1 {
		t.Fatalf("created %d orders, want 1", len(orders.created))
	}
	input := orders.created[0]
	if input.TriggerType != releasedomain.TriggerTypeAutomation {
		t.Fatalf("TriggerType = %q, want automation", input.TriggerType)
	}
	if input.CreatorUserID != "usr-1" || input.TriggeredBy != "张三" {
		t.Fatalf("creator = %q / %q, want usr-1 / 张三", input.CreatorUserID, input.TriggeredBy)
	}
	if input.EnvCode != releaseAutomationTestEnvCode || input.GitRef != releaseAutomationTestGitRef {
		t.Fatalf("target = %q / %q", input.EnvCode, input.GitRef)
	}

	current, err := repo.GetByID(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("GetByID err = %v", err)
	}
	if current.LastSeenSHA != "sha-2" || current.LastTriggeredSHA != "sha-2" {
		t.Fatalf("baseline = %q / triggered = %q, want sha-2", current.LastSeenSHA, current.LastTriggeredSHA)
	}
	if current.LastOrderID != "ro-auto-1" {
		t.Fatalf("LastOrderID = %q, want ro-auto-1", current.LastOrderID)
	}

	// 再轮询一次：HEAD 未变，不允许重复建单。
	second, err := manager.RunDueAutomations(context.Background(), 20)
	if err != nil {
		t.Fatalf("second RunDueAutomations err = %v", err)
	}
	if second.Triggered != 0 || len(orders.created) != 1 {
		t.Fatalf("second run = %+v, created=%d; want no duplicate order", second, len(orders.created))
	}
}

func TestReleaseAutomationDispatchModes(t *testing.T) {
	cases := []struct {
		mode   automationdomain.DispatchMode
		expect []string
	}{
		{automationdomain.DispatchModeBuild, []string{"build"}},
		// build_deploy 先只把构建发出去，构建完成后的部署由续跑补上（见下面的续跑用例）
		{automationdomain.DispatchModeBuildDeploy, []string{"build"}},
		{automationdomain.DispatchModeExecute, []string{"execute"}},
	}
	for _, tc := range cases {
		t.Run(string(tc.mode), func(t *testing.T) {
			repo := newReleaseAutomationRepoFake()
			orders := &releaseAutomationOrderStub{}
			git := &releaseAutomationGitStub{head: HeadCommit{CommitSHA: "sha-1"}}
			manager := newReleaseAutomationTestManager(t, repo, orders, git)

			input := releaseAutomationTestInput()
			input.DispatchMode = tc.mode
			if _, err := manager.Create(context.Background(), input); err != nil {
				t.Fatalf("Create err = %v", err)
			}
			git.head = HeadCommit{CommitSHA: "sha-2"}

			if _, err := manager.RunDueAutomations(context.Background(), 20); err != nil {
				t.Fatalf("RunDueAutomations err = %v", err)
			}
			if strings.Join(orders.actions, ",") != strings.Join(tc.expect, ",") {
				t.Fatalf("dispatch actions = %v, want %v", orders.actions, tc.expect)
			}
		})
	}
}

// 构建并发布的单子停在「已构建待部署」时，轮询要把它推去部署。
func TestReleaseAutomationContinuesBuiltOrderToDeploy(t *testing.T) {
	repo := newReleaseAutomationRepoFake()
	orders := &releaseAutomationOrderStub{autoDeployStatus: releasedomain.OrderStatusBuiltWaitingDeploy}
	git := &releaseAutomationGitStub{head: HeadCommit{CommitSHA: "sha-1"}}
	manager := newReleaseAutomationTestManager(t, repo, orders, git)

	input := releaseAutomationTestInput()
	input.DispatchMode = automationdomain.DispatchModeBuildDeploy
	created, err := manager.Create(context.Background(), input)
	if err != nil {
		t.Fatalf("Create err = %v", err)
	}
	// 模拟上一轮已经建过单：基线已推进，LastOrderID 指向停在待部署的单。
	git.head = HeadCommit{CommitSHA: "sha-2"}
	if _, err := manager.RunDueAutomations(context.Background(), 20); err != nil {
		t.Fatalf("first RunDueAutomations err = %v", err)
	}
	orders.actions = nil

	second, err := manager.RunDueAutomations(context.Background(), 20)
	if err != nil {
		t.Fatalf("second RunDueAutomations err = %v", err)
	}
	if strings.Join(orders.actions, ",") != "deploy" {
		t.Fatalf("dispatch actions = %v, want [deploy]", orders.actions)
	}
	if second.Deployed != 1 {
		t.Fatalf("Deployed = %d, want 1", second.Deployed)
	}

	current, err := repo.GetByID(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("GetByID err = %v", err)
	}
	if strings.TrimSpace(current.LastError) != "" {
		t.Fatalf("LastError = %q, want cleared after the deploy is dispatched", current.LastError)
	}
}

// 派不动（并发锁被占等）时把原因记下来，下一轮继续试，不吞掉问题。
func TestReleaseAutomationRecordsReasonWhenDeployIsBlocked(t *testing.T) {
	repo := newReleaseAutomationRepoFake()
	orders := &releaseAutomationOrderStub{
		autoDeployStatus: releasedomain.OrderStatusBuiltWaitingDeploy,
		autoDeployErr:    errors.New("concurrent release blocked: 已有在途发布单"),
	}
	git := &releaseAutomationGitStub{head: HeadCommit{CommitSHA: "sha-1"}}
	manager := newReleaseAutomationTestManager(t, repo, orders, git)

	input := releaseAutomationTestInput()
	input.DispatchMode = automationdomain.DispatchModeBuildDeploy
	created, err := manager.Create(context.Background(), input)
	if err != nil {
		t.Fatalf("Create err = %v", err)
	}
	git.head = HeadCommit{CommitSHA: "sha-2"}
	if _, err := manager.RunDueAutomations(context.Background(), 20); err != nil {
		t.Fatalf("first RunDueAutomations err = %v", err)
	}

	second, err := manager.RunDueAutomations(context.Background(), 20)
	if err != nil {
		t.Fatalf("second RunDueAutomations err = %v", err)
	}
	if second.DeployWaiting != 1 || second.Deployed != 0 {
		t.Fatalf("second run = %+v, want deploy_waiting=1", second)
	}
	current, err := repo.GetByID(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("GetByID err = %v", err)
	}
	if !strings.Contains(current.LastError, "concurrent release blocked") {
		t.Fatalf("LastError = %q, want the blocking reason", current.LastError)
	}
}

// 模板要求审批时 Build 只会把发布单推进审批流，build_deploy 不能再接着 Deploy。
func TestReleaseAutomationBuildDeployStopsWhenApprovalIsRequired(t *testing.T) {
	repo := newReleaseAutomationRepoFake()
	orders := &releaseAutomationOrderStub{buildState: releasedomain.OrderStatusApproving}
	git := &releaseAutomationGitStub{head: HeadCommit{CommitSHA: "sha-1"}}
	manager := newReleaseAutomationTestManager(t, repo, orders, git)

	input := releaseAutomationTestInput()
	input.DispatchMode = automationdomain.DispatchModeBuildDeploy
	if _, err := manager.Create(context.Background(), input); err != nil {
		t.Fatalf("Create err = %v", err)
	}
	git.head = HeadCommit{CommitSHA: "sha-2"}

	if _, err := manager.RunDueAutomations(context.Background(), 20); err != nil {
		t.Fatalf("RunDueAutomations err = %v", err)
	}
	if strings.Join(orders.actions, ",") != "build" {
		t.Fatalf("dispatch actions = %v, want [build]", orders.actions)
	}
}

// 同应用同环境有在途单时不建单、不推进基线，等在途单结束后补建。
func TestReleaseAutomationRunBlocksAndKeepsBaselineWhileOrderIsActive(t *testing.T) {
	repo := newReleaseAutomationRepoFake()
	orders := &releaseAutomationOrderStub{activeErr: nil, activeNo: "RO-20260918-001"}
	git := &releaseAutomationGitStub{head: HeadCommit{CommitSHA: "sha-1"}}
	manager := newReleaseAutomationTestManager(t, repo, orders, git)

	created, err := manager.Create(context.Background(), releaseAutomationTestInput())
	if err != nil {
		t.Fatalf("Create err = %v", err)
	}
	git.head = HeadCommit{CommitSHA: "sha-2"}

	output, err := manager.RunDueAutomations(context.Background(), 20)
	if err != nil {
		t.Fatalf("RunDueAutomations err = %v", err)
	}
	if output.Blocked != 1 || output.Triggered != 0 {
		t.Fatalf("output = %+v, want blocked=1 triggered=0", output)
	}
	if len(orders.created) != 0 {
		t.Fatalf("created %d orders, want 0 while an order is active", len(orders.created))
	}
	current, err := repo.GetByID(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("GetByID err = %v", err)
	}
	if current.LastSeenSHA != "sha-1" {
		t.Fatalf("LastSeenSHA = %q, want the baseline kept at sha-1", current.LastSeenSHA)
	}
	if !strings.Contains(current.LastError, "RO-20260918-001") {
		t.Fatalf("LastError = %q, want the blocking order number", current.LastError)
	}

	// 在途单结束：下一轮必须补建，sha-2 不会被漏掉。
	orders.activeErr = releasedomain.ErrOrderNotFound
	output, err = manager.RunDueAutomations(context.Background(), 20)
	if err != nil {
		t.Fatalf("second RunDueAutomations err = %v", err)
	}
	if output.Triggered != 1 || len(orders.created) != 1 {
		t.Fatalf("second run = %+v, created=%d; want the pending commit released", output, len(orders.created))
	}
}

// 建单时才发现并发冲突（预检查之后插进来的在途单）：按 blocked 处理，同样不动基线。
func TestReleaseAutomationRunTreatsConcurrentCreateAsBlocked(t *testing.T) {
	repo := newReleaseAutomationRepoFake()
	cause := errors.Join(ErrConcurrentReleaseBlocked, errors.New("同一应用同一环境已有在途发布单"))
	orders := &releaseAutomationOrderStub{activeErr: releasedomain.ErrOrderNotFound, createErr: cause}
	git := &releaseAutomationGitStub{head: HeadCommit{CommitSHA: "sha-1"}}
	manager := newReleaseAutomationTestManager(t, repo, orders, git)

	created, err := manager.Create(context.Background(), releaseAutomationTestInput())
	if err != nil {
		t.Fatalf("Create err = %v", err)
	}
	git.head = HeadCommit{CommitSHA: "sha-2"}

	output, err := manager.RunDueAutomations(context.Background(), 20)
	if err != nil {
		t.Fatalf("RunDueAutomations err = %v", err)
	}
	if output.Blocked != 1 {
		t.Fatalf("output = %+v, want blocked=1", output)
	}
	current, err := repo.GetByID(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("GetByID err = %v", err)
	}
	if current.LastSeenSHA != "sha-1" {
		t.Fatalf("LastSeenSHA = %q, want sha-1", current.LastSeenSHA)
	}
}

// CAS 抢占失败：说明别的副本已经处理过这一轮，本次结果必须丢弃，不覆盖基线。
func TestReleaseAutomationRunDiscardsResultWhenCasLoses(t *testing.T) {
	repo := newReleaseAutomationRepoFake()
	orders := &releaseAutomationOrderStub{activeErr: releasedomain.ErrOrderNotFound}
	git := &releaseAutomationGitStub{head: HeadCommit{CommitSHA: "sha-1"}}
	manager := newReleaseAutomationTestManager(t, repo, orders, git)

	created, err := manager.Create(context.Background(), releaseAutomationTestInput())
	if err != nil {
		t.Fatalf("Create err = %v", err)
	}
	git.head = HeadCommit{CommitSHA: "sha-2"}
	repo.commitResults = []bool{false}

	output, err := manager.RunDueAutomations(context.Background(), 20)
	if err != nil {
		t.Fatalf("RunDueAutomations err = %v", err)
	}
	if output.Failed != 1 || output.Triggered != 0 {
		t.Fatalf("output = %+v, want failed=1 triggered=0", output)
	}
	if repo.commitCalls != 1 {
		t.Fatalf("CommitTrigger calls = %d, want 1", repo.commitCalls)
	}
	// 别的副本写入的基线（仍是 sha-1）不能被本次覆盖。
	current, err := repo.GetByID(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("GetByID err = %v", err)
	}
	if current.LastSeenSHA != "sha-1" {
		t.Fatalf("LastSeenSHA = %q, want the other replica's value sha-1", current.LastSeenSHA)
	}
}

// git 读取失败：刷新 last_error，不动基线，等待下一轮重试。
func TestReleaseAutomationRunRecordsGitFailure(t *testing.T) {
	repo := newReleaseAutomationRepoFake()
	orders := &releaseAutomationOrderStub{}
	git := &releaseAutomationGitStub{head: HeadCommit{CommitSHA: "sha-1"}}
	manager := newReleaseAutomationTestManager(t, repo, orders, git)

	created, err := manager.Create(context.Background(), releaseAutomationTestInput())
	if err != nil {
		t.Fatalf("Create err = %v", err)
	}
	git.err = errors.New("读取分支 HEAD 失败：分支不存在")

	output, err := manager.RunDueAutomations(context.Background(), 20)
	if err != nil {
		t.Fatalf("RunDueAutomations err = %v", err)
	}
	if output.Failed != 1 {
		t.Fatalf("output = %+v, want failed=1", output)
	}
	current, err := repo.GetByID(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("GetByID err = %v", err)
	}
	if !strings.Contains(current.LastError, "分支不存在") {
		t.Fatalf("LastError = %q, want the git reason", current.LastError)
	}
	if current.LastSeenSHA != "sha-1" {
		t.Fatalf("LastSeenSHA = %q, want sha-1", current.LastSeenSHA)
	}
}

// 单条配置 panic 不能拖垮整轮：其余配置照常处理。
func TestReleaseAutomationRunRecoversPerConfigPanic(t *testing.T) {
	repo := newReleaseAutomationRepoFake()
	orders := &releaseAutomationOrderStub{panicOnRun: true, activeErr: releasedomain.ErrOrderNotFound}
	git := &releaseAutomationGitStub{head: HeadCommit{CommitSHA: "sha-1"}}
	manager := newReleaseAutomationTestManager(t, repo, orders, git)

	first := releaseAutomationTestInput()
	if _, err := manager.Create(context.Background(), first); err != nil {
		t.Fatalf("Create first err = %v", err)
	}
	second := releaseAutomationTestInput()
	second.Name = "后端生产自动发布"
	second.GitRef = "release/2026-09-19"
	if _, err := manager.Create(context.Background(), second); err != nil {
		t.Fatalf("Create second err = %v", err)
	}
	git.head = HeadCommit{CommitSHA: "sha-2"}

	output, err := manager.RunDueAutomations(context.Background(), 20)
	if err != nil {
		t.Fatalf("RunDueAutomations err = %v", err)
	}
	if output.Scanned != 2 {
		t.Fatalf("Scanned = %d, want 2", output.Scanned)
	}
	if output.Triggered != 1 {
		t.Fatalf("output = %+v, want the second config still processed (triggered=1)", output)
	}
}

func TestReleaseAutomationUpdateResetsBaselineWhenBranchChanges(t *testing.T) {
	repo := newReleaseAutomationRepoFake()
	orders := &releaseAutomationOrderStub{}
	git := &releaseAutomationGitStub{
		headByRef: map[string]HeadCommit{
			releaseAutomationTestGitRef: {CommitSHA: "sha-old"},
			"release/2026-09-19":        {CommitSHA: "sha-new"},
		},
	}
	manager := newReleaseAutomationTestManager(t, repo, orders, git)

	created, err := manager.Create(context.Background(), releaseAutomationTestInput())
	if err != nil {
		t.Fatalf("Create err = %v", err)
	}
	if created.LastSeenSHA != "sha-old" {
		t.Fatalf("LastSeenSHA = %q, want sha-old", created.LastSeenSHA)
	}

	updateInput := releaseAutomationTestInput()
	updateInput.GitRef = "release/2026-09-19"
	updated, err := manager.Update(context.Background(), created.ID, updateInput)
	if err != nil {
		t.Fatalf("Update err = %v", err)
	}
	if updated.LastSeenSHA != "sha-new" {
		t.Fatalf("LastSeenSHA = %q, want the new branch baseline sha-new", updated.LastSeenSHA)
	}
	if updated.LastTriggeredSHA != "" || updated.LastOrderID != "" {
		t.Fatalf("stale trigger trace = %q / %q, want empty", updated.LastTriggeredSHA, updated.LastOrderID)
	}
}

func TestReleaseAutomationUpdateKeepsBaselineWhenOnlyMetadataChanges(t *testing.T) {
	repo := newReleaseAutomationRepoFake()
	orders := &releaseAutomationOrderStub{}
	git := &releaseAutomationGitStub{head: HeadCommit{CommitSHA: "sha-1"}}
	manager := newReleaseAutomationTestManager(t, repo, orders, git)

	created, err := manager.Create(context.Background(), releaseAutomationTestInput())
	if err != nil {
		t.Fatalf("Create err = %v", err)
	}
	// 分支前进但还没轮询到，此时改个名字不能把待发的提交抹掉。
	git.head = HeadCommit{CommitSHA: "sha-2"}
	checkGit := false
	updateInput := releaseAutomationTestInput()
	updateInput.Name = "前端生产自动发布（改名）"
	updateInput.CheckGit = &checkGit

	updated, err := manager.Update(context.Background(), created.ID, updateInput)
	if err != nil {
		t.Fatalf("Update err = %v", err)
	}
	if updated.LastSeenSHA != "sha-1" {
		t.Fatalf("LastSeenSHA = %q, want the untouched baseline sha-1", updated.LastSeenSHA)
	}
}

func TestReleaseAutomationUpdateRejectsWhenGitCheckFails(t *testing.T) {
	repo := newReleaseAutomationRepoFake()
	orders := &releaseAutomationOrderStub{}
	git := &releaseAutomationGitStub{head: HeadCommit{CommitSHA: "sha-1"}}
	manager := newReleaseAutomationTestManager(t, repo, orders, git)

	created, err := manager.Create(context.Background(), releaseAutomationTestInput())
	if err != nil {
		t.Fatalf("Create err = %v", err)
	}
	git.err = errors.New("Git 凭证被拒绝或已过期")

	updateInput := releaseAutomationTestInput()
	updateInput.Name = "改个名字"
	_, err = manager.Update(context.Background(), created.ID, updateInput)
	if !errors.Is(err, ErrAutomationGitUnreachable) {
		t.Fatalf("Update err = %v, want ErrAutomationGitUnreachable", err)
	}
	current, err := repo.GetByID(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("GetByID err = %v", err)
	}
	if current.Name != created.Name {
		t.Fatalf("Name = %q, want the untouched %q", current.Name, created.Name)
	}
}

func TestReleaseAutomationCheckGitReportsUnreachableWithoutError(t *testing.T) {
	repo := newReleaseAutomationRepoFake()
	orders := &releaseAutomationOrderStub{}
	git := &releaseAutomationGitStub{head: HeadCommit{CommitSHA: "sha-1"}}
	manager := newReleaseAutomationTestManager(t, repo, orders, git)

	created, err := manager.Create(context.Background(), releaseAutomationTestInput())
	if err != nil {
		t.Fatalf("Create err = %v", err)
	}

	result, err := manager.CheckNow(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("CheckNow err = %v", err)
	}
	if !result.Reachable || result.HeadSHA != "sha-1" {
		t.Fatalf("result = %+v, want reachable with sha-1", result)
	}

	git.err = errors.New("未找到匹配的 Git 凭证（按仓库地址前缀匹配）")
	result, err = manager.CheckNow(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("CheckNow err = %v, want a result instead of an error", err)
	}
	if result.Reachable {
		t.Fatalf("Reachable = true, want false")
	}
	if !strings.Contains(result.Message, "未找到匹配的 Git 凭证") {
		t.Fatalf("Message = %q, want the readable reason", result.Message)
	}
}

func TestReleaseAutomationCreateRejectsDuplicateTarget(t *testing.T) {
	repo := newReleaseAutomationRepoFake()
	orders := &releaseAutomationOrderStub{}
	git := &releaseAutomationGitStub{head: HeadCommit{CommitSHA: "sha-1"}}
	manager := newReleaseAutomationTestManager(t, repo, orders, git)

	if _, err := manager.Create(context.Background(), releaseAutomationTestInput()); err != nil {
		t.Fatalf("Create err = %v", err)
	}
	_, err := manager.Create(context.Background(), releaseAutomationTestInput())
	if !errors.Is(err, automationdomain.ErrDuplicated) {
		t.Fatalf("second Create err = %v, want ErrDuplicated", err)
	}
}
