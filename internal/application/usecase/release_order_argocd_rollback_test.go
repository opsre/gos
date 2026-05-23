package usecase

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"testing"
	"time"

	argocddomain "gos/internal/domain/argocdapp"
	gitopsdomain "gos/internal/domain/gitops"
	pipelinedomain "gos/internal/domain/pipeline"
	domain "gos/internal/domain/release"
	"gos/internal/infrastructure/persistence/sqlrepo"

	_ "modernc.org/sqlite"
)

func TestStartArgoCDRollbackExecutionReplaysAllDeploySnapshots(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	ctx := context.Background()
	if _, err := db.Exec(`
CREATE TABLE IF NOT EXISTS sys_user (
	id TEXT PRIMARY KEY,
	username TEXT NOT NULL UNIQUE,
	display_name TEXT NOT NULL,
	email TEXT NOT NULL DEFAULT '',
	phone TEXT NOT NULL DEFAULT '',
	role TEXT NOT NULL,
	status TEXT NOT NULL DEFAULT 'active',
	password_hash TEXT NOT NULL,
	created_at INTEGER NOT NULL,
	updated_at INTEGER NOT NULL
);`); err != nil {
		t.Fatalf("create sys_user failed: %v", err)
	}
	releaseRepo := sqlrepo.NewReleaseRepository(db, "sqlite")
	if err := releaseRepo.InitSchema(ctx); err != nil {
		t.Fatalf("release InitSchema failed: %v", err)
	}
	if err := sqlrepo.NewGitOpsRepository(db, "sqlite").InitSchema(ctx); err != nil {
		t.Fatalf("gitops InitSchema failed: %v", err)
	}
	argocdRepo := sqlrepo.NewArgoCDApplicationRepository(db, "sqlite")
	if err := argocdRepo.InitSchema(ctx); err != nil {
		t.Fatalf("argocd InitSchema failed: %v", err)
	}

	now := time.Now().UTC()
	for _, item := range []argocddomain.Instance{
		{
			ID:           "argocd-shanghai",
			InstanceCode: "argocd-shanghai",
			Name:         "ArgoCD Shanghai",
			BaseURL:      "https://argocd-shanghai.example.com",
			AuthMode:     "token",
			Status:       argocddomain.StatusActive,
			CreatedAt:    now,
			UpdatedAt:    now,
		},
		{
			ID:           "argocd-east",
			InstanceCode: "argocd-east",
			Name:         "ArgoCD East",
			BaseURL:      "https://argocd-east.example.com",
			AuthMode:     "token",
			Status:       argocddomain.StatusActive,
			CreatedAt:    now,
			UpdatedAt:    now,
		},
	} {
		if _, err := argocdRepo.UpsertInstance(ctx, item); err != nil {
			t.Fatalf("UpsertInstance(%s) failed: %v", item.ID, err)
		}
	}

	sourceOrder := testReleaseOrder("ro-source-multi-rollback", "RO-SOURCE-MULTI-ROLLBACK", domain.OrderStatusSuccess, now)
	if err := releaseRepo.Create(ctx, sourceOrder, nil, nil, nil); err != nil {
		t.Fatalf("Create source order failed: %v", err)
	}
	for _, snapshot := range []domain.DeploySnapshot{
		{
			ID:               "snapshot-shanghai",
			ReleaseOrderID:   sourceOrder.ID,
			Provider:         "argocd",
			GitOpsType:       domain.GitOpsTypeHelm,
			ArgoCDInstanceID: "argocd-shanghai",
			ArgoCDAppName:    "demo-prod-shanghai",
			RepoURL:          "https://example.com/repo.git",
			Branch:           "demo-prod",
			SourcePath:       "apps/demo/helm",
			EnvCode:          "prod",
			SnapshotPayload:  `{"image_version":"175","rules":[{"file_path":"values.yaml","target_path":"image.tag","value":"175"}]}`,
			CreatedAt:        now,
		},
		{
			ID:               "snapshot-east",
			ReleaseOrderID:   sourceOrder.ID,
			Provider:         "argocd",
			GitOpsType:       domain.GitOpsTypeHelm,
			ArgoCDInstanceID: "argocd-east",
			ArgoCDAppName:    "demo-prod-east",
			RepoURL:          "https://example.com/repo.git",
			Branch:           "demo-prod",
			SourcePath:       "apps/demo/helm",
			EnvCode:          "prod",
			SnapshotPayload:  `{"image_version":"175","rules":[{"file_path":"values.yaml","target_path":"image.tag","value":"175"}]}`,
			CreatedAt:        now.Add(time.Second),
		},
	} {
		if err := releaseRepo.CreateDeploySnapshot(ctx, snapshot); err != nil {
			t.Fatalf("CreateDeploySnapshot(%s) failed: %v", snapshot.ArgoCDInstanceID, err)
		}
	}

	rollbackOrder := testReleaseOrder("ro-rollback-multi", "RO-ROLLBACK-MULTI", domain.OrderStatusDeploying, now)
	rollbackOrder.OperationType = domain.OperationTypeRollback
	rollbackOrder.SourceOrderID = sourceOrder.ID
	rollbackOrder.SourceOrderNo = sourceOrder.OrderNo
	execution := domain.ReleaseOrderExecution{
		ID:             "exec-rollback-cd",
		ReleaseOrderID: rollbackOrder.ID,
		PipelineScope:  domain.PipelineScopeCD,
		BindingID:      "binding-cd",
		BindingName:    "ArgoCD",
		Provider:       string(pipelinedomain.ProviderArgoCD),
		Status:         domain.ExecutionStatusPending,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	steps := []domain.ReleaseOrderStep{
		testReleaseStep(rollbackOrder.ID, "step-update", domain.StepScopeCD, scopeStepCode(domain.PipelineScopeCD, "gitops_update"), domain.StepStatusPending, 1, now),
		testReleaseStep(rollbackOrder.ID, "step-commit", domain.StepScopeCD, scopeStepCode(domain.PipelineScopeCD, "git_commit"), domain.StepStatusPending, 2, now),
		testReleaseStep(rollbackOrder.ID, "step-push", domain.StepScopeCD, scopeStepCode(domain.PipelineScopeCD, "git_push"), domain.StepStatusPending, 3, now),
		testReleaseStep(rollbackOrder.ID, "step-sync", domain.StepScopeCD, scopeStepCode(domain.PipelineScopeCD, "argocd_sync"), domain.StepStatusPending, 4, now),
		testReleaseStep(rollbackOrder.ID, "step-health", domain.StepScopeCD, scopeStepCode(domain.PipelineScopeCD, "health_check"), domain.StepStatusPending, 5, now),
	}
	if err := releaseRepo.Create(ctx, rollbackOrder, []domain.ReleaseOrderExecution{execution}, nil, steps); err != nil {
		t.Fatalf("Create rollback order failed: %v", err)
	}

	argocdFactory := &recordingArgoCDClientFactory{clients: map[string]*recordingArgoCDClient{}}
	gitopsService := &recordingGitOpsReleaseService{}
	manager := NewReleaseOrderManager(releaseRepo, nil, nil, nil, nil, nil, nil, nil, argocdRepo, nil, argocdFactory, nil, nil, gitopsService)
	manager.now = func() time.Time { return now }
	startedAt := now

	err = manager.startArgoCDRollbackExecution(
		ctx,
		rollbackOrder,
		execution,
		nil,
		scopeStepCode(domain.PipelineScopeCD, "gitops_update"),
		scopeStepCode(domain.PipelineScopeCD, "git_commit"),
		scopeStepCode(domain.PipelineScopeCD, "git_push"),
		scopeStepCode(domain.PipelineScopeCD, "argocd_sync"),
		scopeStepCode(domain.PipelineScopeCD, "health_check"),
		&startedAt,
	)
	if err != nil {
		t.Fatalf("startArgoCDRollbackExecution failed: %v", err)
	}

	if len(gitopsService.applyValuesCalls) != 2 {
		t.Fatalf("ApplyValuesRules called %d times, want 2", len(gitopsService.applyValuesCalls))
	}
	assertSyncedApplications(t, argocdFactory, "argocd-shanghai", []string{"demo-prod-shanghai@prod"})
	assertSyncedApplications(t, argocdFactory, "argocd-east", []string{"demo-prod-east@prod"})

	rollbackSnapshots, err := releaseRepo.ListDeploySnapshotsByOrderID(ctx, rollbackOrder.ID)
	if err != nil {
		t.Fatalf("ListDeploySnapshotsByOrderID rollback failed: %v", err)
	}
	if len(rollbackSnapshots) != 2 {
		t.Fatalf("rollback snapshot count = %d, want 2", len(rollbackSnapshots))
	}
}

type applyValuesCall struct {
	RepoURL string
	Branch  string
	Rules   []gitopsdomain.ValuesRule
}

type recordingGitOpsReleaseService struct {
	applyValuesCalls []applyValuesCall
}

func (s *recordingGitOpsReleaseService) UpdateKustomizationImage(
	context.Context,
	string,
	string,
	string,
	string,
	string,
) (string, string, string, string, bool, error) {
	return "", "", "", "", false, fmt.Errorf("UpdateKustomizationImage is not expected")
}

func (s *recordingGitOpsReleaseService) ApplyManifestRules(
	context.Context,
	string,
	string,
	[]gitopsdomain.ManifestRule,
	string,
) (string, []string, string, bool, error) {
	return "", nil, "", false, fmt.Errorf("ApplyManifestRules is not expected")
}

func (s *recordingGitOpsReleaseService) ApplyValuesRules(
	_ context.Context,
	repoURL string,
	branch string,
	rules []gitopsdomain.ValuesRule,
	_ string,
) (string, []string, string, bool, error) {
	s.applyValuesCalls = append(s.applyValuesCalls, applyValuesCall{RepoURL: repoURL, Branch: branch, Rules: append([]gitopsdomain.ValuesRule(nil), rules...)})
	return "", []string{"values.yaml"}, fmt.Sprintf("commit-%d", len(s.applyValuesCalls)), true, nil
}

func (s *recordingGitOpsReleaseService) BuildCommitMessage(map[string]string) string {
	return "rollback"
}

func (s *recordingGitOpsReleaseService) RenderTemplate(template string, fields map[string]string) string {
	result := template
	for key, value := range fields {
		result = strings.ReplaceAll(result, "{"+key+"}", value)
	}
	return result
}

type recordingArgoCDClientFactory struct {
	clients map[string]*recordingArgoCDClient
}

func (f *recordingArgoCDClientFactory) Build(instance argocddomain.Instance) ArgoCDApplicationClient {
	if f.clients == nil {
		f.clients = map[string]*recordingArgoCDClient{}
	}
	client := f.clients[instance.ID]
	if client == nil {
		client = &recordingArgoCDClient{}
		f.clients[instance.ID] = client
	}
	return client
}

type recordingArgoCDClient struct {
	syncs []string
}

func (c *recordingArgoCDClient) Ping(context.Context) error {
	return nil
}

func (c *recordingArgoCDClient) ListApplications(context.Context) ([]ArgoCDApplicationSnapshot, error) {
	return nil, nil
}

func (c *recordingArgoCDClient) GetApplication(context.Context, string) (ArgoCDApplicationSnapshot, error) {
	return nil, fmt.Errorf("GetApplication is not expected")
}

func (c *recordingArgoCDClient) SyncApplication(ctx context.Context, name string) error {
	return c.SyncApplicationWithRevision(ctx, name, "")
}

func (c *recordingArgoCDClient) SyncApplicationWithRevision(_ context.Context, name string, revision string) error {
	c.syncs = append(c.syncs, name+"@"+revision)
	return nil
}

func (c *recordingArgoCDClient) BuildApplicationURL(name string) string {
	return "https://argocd.example.com/applications/" + name
}

func assertSyncedApplications(t *testing.T, factory *recordingArgoCDClientFactory, instanceID string, want []string) {
	t.Helper()
	client := factory.clients[instanceID]
	if client == nil {
		t.Fatalf("argocd client for %s was not built", instanceID)
	}
	if len(client.syncs) != len(want) {
		t.Fatalf("sync count for %s = %d, want %d: %#v", instanceID, len(client.syncs), len(want), client.syncs)
	}
	for i := range want {
		if client.syncs[i] != want[i] {
			t.Fatalf("sync[%d] for %s = %q, want %q", i, instanceID, client.syncs[i], want[i])
		}
	}
}
