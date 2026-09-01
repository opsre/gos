package usecase

import (
	"context"
	"strings"
	"testing"
	"time"

	appdomain "gos/internal/domain/application"
	domain "gos/internal/domain/release"
)

func TestBuildJenkinsExecutionParamsInjectsUpstreamCIJobAndBuild(t *testing.T) {
	t.Parallel()

	manager, repo := newReleaseOrderManagerForCancelTest(t)
	ctx := context.Background()
	now := time.Unix(1_780_000_000, 0).UTC()
	templateID := "rt-ci-copy-artifact"
	templateParams := []domain.ReleaseTemplateParam{
		{
			ID:                 "rtp-ci-job",
			TemplateID:         templateID,
			ExecutorParamDefID: "epd-ci-job",
			PipelineScope:      domain.PipelineScopeCD,
			ParamKey:           standardParamCIJob,
			ParamName:          "CI Job",
			ExecutorParamName:  "CI_JOB",
			ValueSource:        domain.TemplateParamValueSourceBuiltin,
			SourceParamKey:     standardParamCIJob,
			Required:           true,
			SortNo:             1,
			CreatedAt:          now,
			UpdatedAt:          now,
		},
		{
			ID:                 "rtp-ci-build",
			TemplateID:         templateID,
			ExecutorParamDefID: "epd-ci-build",
			PipelineScope:      domain.PipelineScopeCD,
			ParamKey:           standardParamCIBuild,
			ParamName:          "CI Build",
			ExecutorParamName:  "CI_BUILD",
			ValueSource:        domain.TemplateParamValueSourceBuiltin,
			SourceParamKey:     standardParamCIBuild,
			Required:           true,
			SortNo:             2,
			CreatedAt:          now,
			UpdatedAt:          now,
		},
	}
	if err := repo.CreateTemplate(ctx, domain.ReleaseTemplate{
		ID:          templateID,
		Name:        "CI Copy Artifact",
		Status:      domain.TemplateStatusActive,
		CreatedAt:   now,
		UpdatedAt:   now,
		BindingType: "ci_cd",
	}, nil, templateParams, nil, nil); err != nil {
		t.Fatalf("CreateTemplate failed: %v", err)
	}

	order := testReleaseOrder("ro-ci-copy-artifact", "RO-CI-COPY-ARTIFACT", domain.OrderStatusRunning, now)
	order.TemplateID = templateID
	ciExecution := testReleaseExecution(order.ID, "exec-ci", domain.PipelineScopeCI, domain.ExecutionStatusSuccess, now)
	ciExecution.BuildURL = "https://jenkins.example/jenkins/job/team/job/mcs-product-rest/job/main/42/"
	cdExecution := testReleaseExecution(order.ID, "exec-cd", domain.PipelineScopeCD, domain.ExecutionStatusPending, now)
	executions := []domain.ReleaseOrderExecution{ciExecution, cdExecution}
	staleParams := []domain.ReleaseOrderParam{
		{PipelineScope: domain.PipelineScopeCD, ParamKey: standardParamCIJob, ExecutorParamName: "CI_JOB", ParamValue: "stale-job"},
		{PipelineScope: domain.PipelineScopeCD, ParamKey: standardParamCIBuild, ExecutorParamName: "CI_BUILD", ParamValue: "1"},
	}

	got, err := manager.buildJenkinsExecutionParams(ctx, order, cdExecution, staleParams, executions)
	if err != nil {
		t.Fatalf("buildJenkinsExecutionParams failed: %v", err)
	}
	if got["CI_JOB"] != "team/mcs-product-rest/main" {
		t.Fatalf("CI_JOB = %q, want %q", got["CI_JOB"], "team/mcs-product-rest/main")
	}
	if got["CI_BUILD"] != "42" {
		t.Fatalf("CI_BUILD = %q, want %q", got["CI_BUILD"], "42")
	}
}

func TestBuildJenkinsExecutionParamsRejectsMissingUpstreamBuildURL(t *testing.T) {
	t.Parallel()

	manager, repo := newReleaseOrderManagerForCancelTest(t)
	ctx := context.Background()
	now := time.Unix(1_780_000_000, 0).UTC()
	templateID := "rt-ci-copy-artifact-missing-url"
	if err := repo.CreateTemplate(ctx, domain.ReleaseTemplate{
		ID: templateID, Name: "CI Copy Artifact", Status: domain.TemplateStatusActive, CreatedAt: now, UpdatedAt: now,
	}, nil, []domain.ReleaseTemplateParam{
		{
			ID: "rtp-ci-job-missing", TemplateID: templateID, PipelineScope: domain.PipelineScopeCD,
			ParamKey: standardParamCIJob, ExecutorParamName: "CI_JOB", ValueSource: domain.TemplateParamValueSourceBuiltin,
			SourceParamKey: standardParamCIJob, Required: true, SortNo: 1, CreatedAt: now, UpdatedAt: now,
		},
	}, nil, nil); err != nil {
		t.Fatalf("CreateTemplate failed: %v", err)
	}

	order := testReleaseOrder("ro-ci-copy-artifact-missing", "RO-CI-COPY-ARTIFACT-MISSING", domain.OrderStatusRunning, now)
	order.TemplateID = templateID
	ciExecution := testReleaseExecution(order.ID, "exec-ci", domain.PipelineScopeCI, domain.ExecutionStatusSuccess, now)
	cdExecution := testReleaseExecution(order.ID, "exec-cd", domain.PipelineScopeCD, domain.ExecutionStatusPending, now)

	_, err := manager.buildJenkinsExecutionParams(ctx, order, cdExecution, nil, []domain.ReleaseOrderExecution{ciExecution, cdExecution})
	if err == nil {
		t.Fatal("buildJenkinsExecutionParams error = nil, want missing CI_JOB error")
	}
	if !strings.Contains(err.Error(), "CI_JOB") {
		t.Fatalf("error = %q, want CI_JOB context", err.Error())
	}
}

func TestBuildJenkinsExecutionParamsUsesSuccessfulSourceCIForCDOnlyReplay(t *testing.T) {
	t.Parallel()

	manager, repo := newReleaseOrderManagerForCancelTest(t)
	ctx := context.Background()
	now := time.Unix(1_780_000_000, 0).UTC()
	templateID := "rt-ci-copy-artifact-cd-replay"
	if err := repo.CreateTemplate(ctx, domain.ReleaseTemplate{
		ID: templateID, Name: "CI Copy Artifact CD Replay", Status: domain.TemplateStatusActive, CreatedAt: now, UpdatedAt: now,
	}, nil, []domain.ReleaseTemplateParam{
		{
			ID: "rtp-ci-job-cd-replay", TemplateID: templateID, PipelineScope: domain.PipelineScopeCD,
			ExecutorParamDefID: "epd-ci-job-cd-replay",
			ParamKey:           standardParamCIJob, ExecutorParamName: "CI_JOB", ValueSource: domain.TemplateParamValueSourceBuiltin,
			SourceParamKey: standardParamCIJob, Required: true, SortNo: 1, CreatedAt: now, UpdatedAt: now,
		},
		{
			ID: "rtp-ci-build-cd-replay", TemplateID: templateID, PipelineScope: domain.PipelineScopeCD,
			ExecutorParamDefID: "epd-ci-build-cd-replay",
			ParamKey:           standardParamCIBuild, ExecutorParamName: "CI_BUILD", ValueSource: domain.TemplateParamValueSourceBuiltin,
			SourceParamKey: standardParamCIBuild, Required: true, SortNo: 2, CreatedAt: now, UpdatedAt: now,
		},
	}, nil, nil); err != nil {
		t.Fatalf("CreateTemplate failed: %v", err)
	}

	sourceOrder := testReleaseOrder("ro-source-ci-for-cd-replay", "RO-SOURCE-CI-FOR-CD-REPLAY", domain.OrderStatusDeployFailed, now)
	sourceCI := testReleaseExecution(sourceOrder.ID, "exec-source-ci-for-cd-replay", domain.PipelineScopeCI, domain.ExecutionStatusSuccess, now)
	sourceCI.BuildURL = "https://jenkins.example/job/team/job/CI_FRONT/8/"
	sourceCD := testReleaseExecution(sourceOrder.ID, "exec-source-cd-for-cd-replay", domain.PipelineScopeCD, domain.ExecutionStatusFailed, now)
	if err := repo.Create(ctx, sourceOrder, []domain.ReleaseOrderExecution{sourceCI, sourceCD}, nil, nil); err != nil {
		t.Fatalf("Create source order failed: %v", err)
	}

	replayOrder := testReleaseOrder("ro-cd-only-replay", "RO-CD-ONLY-REPLAY", domain.OrderStatusDeploying, now)
	replayOrder.TemplateID = templateID
	replayOrder.OperationType = domain.OperationTypeReplay
	replayOrder.SourceOrderID = sourceOrder.ID
	replayCD := testReleaseExecution(replayOrder.ID, "exec-cd-only-replay", domain.PipelineScopeCD, domain.ExecutionStatusPending, now)
	staleParams := []domain.ReleaseOrderParam{
		{PipelineScope: domain.PipelineScopeCD, ParamKey: standardParamCIJob, ExecutorParamName: "CI_JOB", ParamValue: "stale-job"},
		{PipelineScope: domain.PipelineScopeCD, ParamKey: standardParamCIBuild, ExecutorParamName: "CI_BUILD", ParamValue: "1"},
	}

	got, err := manager.buildJenkinsExecutionParams(ctx, replayOrder, replayCD, staleParams, []domain.ReleaseOrderExecution{replayCD})
	if err != nil {
		t.Fatalf("buildJenkinsExecutionParams failed: %v", err)
	}
	if got["CI_JOB"] != "team/CI_FRONT" || got["CI_BUILD"] != "8" {
		t.Fatalf("upstream params=%#v, want source CI_JOB=team/CI_FRONT CI_BUILD=8", got)
	}
}

func TestBuildJenkinsExecutionParamsInjectsApplicationRepoURL(t *testing.T) {
	t.Parallel()

	manager, repo := newReleaseOrderManagerForCancelTest(t)
	manager.appRepo = applicationRepositoryStub{app: appdomain.Application{
		ID:      "app-repo-url",
		Key:     "payment-service",
		RepoURL: "https://git.example.com/team/payment-service.git",
	}}
	ctx := context.Background()
	now := time.Unix(1_780_000_000, 0).UTC()
	templateID := "rt-cd-repo-url"
	if err := repo.CreateTemplate(ctx, domain.ReleaseTemplate{
		ID: templateID, Name: "CD Repo URL", Status: domain.TemplateStatusActive, CreatedAt: now, UpdatedAt: now,
	}, nil, []domain.ReleaseTemplateParam{
		{
			ID: "rtp-cd-repo-url", TemplateID: templateID, PipelineScope: domain.PipelineScopeCD,
			ParamKey: "repo_url", ParamName: "Git 仓库地址", ExecutorParamName: "REPO_URL",
			ValueSource: domain.TemplateParamValueSourceBuiltin, SourceParamKey: "repo_url", Required: true,
			SortNo: 1, CreatedAt: now, UpdatedAt: now,
		},
	}, nil, nil); err != nil {
		t.Fatalf("CreateTemplate failed: %v", err)
	}

	order := testReleaseOrder("ro-cd-repo-url", "RO-CD-REPO-URL", domain.OrderStatusRunning, now)
	order.TemplateID = templateID
	order.ApplicationID = "app-repo-url"
	cdExecution := testReleaseExecution(order.ID, "exec-cd-repo-url", domain.PipelineScopeCD, domain.ExecutionStatusPending, now)

	got, err := manager.buildJenkinsExecutionParams(ctx, order, cdExecution, nil, []domain.ReleaseOrderExecution{cdExecution})
	if err != nil {
		t.Fatalf("buildJenkinsExecutionParams failed: %v", err)
	}
	if got["REPO_URL"] != "https://git.example.com/team/payment-service.git" {
		t.Fatalf("REPO_URL = %q, want application repo url", got["REPO_URL"])
	}
}

func TestParseJenkinsJobFullName(t *testing.T) {
	t.Parallel()

	cases := map[string]string{
		"https://jenkins.example/job/demo/9/":                                       "demo",
		"https://jenkins.example/jenkins/job/team/job/service/42/":                  "team/service",
		"https://jenkins.example/job/team/job/mcs-product-rest/job/release%2Fv1/7/": "team/mcs-product-rest/release/v1",
		"": "",
	}
	for input, want := range cases {
		if got := parseJenkinsJobFullName(input); got != want {
			t.Fatalf("parseJenkinsJobFullName(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestDefaultJenkinsExecutorParamKey(t *testing.T) {
	t.Parallel()

	if got := defaultJenkinsExecutorParamKey("CI_JOB"); got != standardParamCIJob {
		t.Fatalf("CI_JOB key = %q, want %q", got, standardParamCIJob)
	}
	if got := defaultJenkinsExecutorParamKey("ci_build"); got != standardParamCIBuild {
		t.Fatalf("ci_build key = %q, want %q", got, standardParamCIBuild)
	}
	if got := defaultJenkinsExecutorParamKey("IMAGE_TAG"); got != "" {
		t.Fatalf("IMAGE_TAG key = %q, want empty", got)
	}
}

func TestResolveReleaseOrderValueProgressUsesUpstreamCIExecution(t *testing.T) {
	t.Parallel()

	now := time.Unix(1_780_000_000, 0).UTC()
	ciExecution := domain.ReleaseOrderExecution{
		PipelineScope: domain.PipelineScopeCI,
		Provider:      "jenkins",
		Status:        domain.ExecutionStatusSuccess,
		BuildURL:      "https://jenkins.example/job/team/job/mcs-product-rest/73/",
		UpdatedAt:     now,
	}
	progress := resolveReleaseOrderValueProgressItem(
		domain.ReleaseOrder{},
		domain.ReleaseTemplateParam{
			PipelineScope:     domain.PipelineScopeCD,
			ParamKey:          standardParamCIBuild,
			ExecutorParamName: "CI_BUILD",
			ValueSource:       domain.TemplateParamValueSourceBuiltin,
			Required:          true,
		},
		nil,
		map[domain.PipelineScope]domain.ReleaseOrderExecution{domain.PipelineScopeCI: ciExecution},
		"",
	)
	if progress.Status != ReleaseOrderValueProgressResolved || progress.Value != "73" {
		t.Fatalf("progress = %#v, want resolved CI_BUILD=73", progress)
	}
}
