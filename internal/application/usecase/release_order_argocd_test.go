package usecase

import (
	"context"
	"testing"
	"time"

	appdomain "gos/internal/domain/application"
	argocddomain "gos/internal/domain/argocdapp"
	releasedomain "gos/internal/domain/release"
)

type applicationRepositoryStub struct {
	app appdomain.Application
	err error
}

// Create 创建业务资源并返回处理结果。
func (s applicationRepositoryStub) Create(context.Context, appdomain.Application) error {
	return nil
}

// GetByID 查询并返回指定资源数据。
func (s applicationRepositoryStub) GetByID(context.Context, string) (appdomain.Application, error) {
	if s.err != nil {
		return appdomain.Application{}, s.err
	}
	return s.app, nil
}

// List 查询并返回列表数据。
func (s applicationRepositoryStub) List(context.Context, appdomain.ListFilter) ([]appdomain.Application, int64, error) {
	return nil, 0, nil
}

// Update 更新业务资源并返回处理结果。
func (s applicationRepositoryStub) Update(context.Context, string, appdomain.UpdateInput, time.Time) (appdomain.Application, error) {
	return appdomain.Application{}, nil
}

// Delete 删除业务资源并返回处理结果。
func (s applicationRepositoryStub) Delete(context.Context, string) error {
	return nil
}

// InitSchema 封装当前模块的业务处理逻辑。
func (s applicationRepositoryStub) InitSchema(context.Context) error {
	return nil
}

// TestBuildGitOpsCommitMessageFieldsUsesCIOnlyForGOSArtifactURL GitOps 变量中的 GOS 制品地址只能来自 CI 单元。
func TestBuildGitOpsCommitMessageFieldsUsesCIOnlyForGOSArtifactURL(t *testing.T) {
	t.Parallel()

	fields := buildGitOpsCommitMessageFields(
		releasedomain.ReleaseOrder{OrderNo: "RO-1"},
		[]releasedomain.ReleaseOrderParam{
			{
				PipelineScope: releasedomain.PipelineScopeCD,
				ParamKey:      "gos_artifact_url",
				ParamValue:    "https://cd.example.com/should-not-use.jar",
			},
			{
				PipelineScope: releasedomain.PipelineScopeCI,
				ParamKey:      "gos_artifact_url",
				ParamValue:    "https://ci.example.com/app.jar",
			},
		},
		"app-key",
		"dev",
		"1042",
		"apps/demo",
		"",
	)
	if got := fields["gos_artifact_url"]; got != "https://ci.example.com/app.jar" {
		t.Fatalf("gos_artifact_url = %q, want CI value", got)
	}
}

// TestBuildGitOpsCommitMessageFieldsUsesApplicationForGOSArtifactPath GitOps 变量中的制品路径来自 App 基础信息。
func TestBuildGitOpsCommitMessageFieldsUsesApplicationForGOSArtifactPath(t *testing.T) {
	t.Parallel()

	fields := buildGitOpsCommitMessageFields(
		releasedomain.ReleaseOrder{OrderNo: "RO-1"},
		[]releasedomain.ReleaseOrderParam{
			{
				PipelineScope: releasedomain.PipelineScopeCD,
				ParamKey:      "gos_artifact_path",
				ParamValue:    "release/from-cd-param",
			},
		},
		"app-key",
		"dev",
		"1042",
		"apps/demo",
		"release/pay-center",
	)
	if got := fields["gos_artifact_path"]; got != "release/pay-center" {
		t.Fatalf("gos_artifact_path = %q, want app artifact directory", got)
	}
}

type argoAppSnapshotStub struct {
	targetRevision string
}

// GetName 查询并返回指定资源数据。
func (s argoAppSnapshotStub) GetName() string { return "demo" }

// GetProject 查询并返回指定资源数据。
func (s argoAppSnapshotStub) GetProject() string { return "" }

// GetRepoURL 查询并返回指定资源数据。
func (s argoAppSnapshotStub) GetRepoURL() string { return "" }

// GetSourcePath 查询并返回指定资源数据。
func (s argoAppSnapshotStub) GetSourcePath() string { return "" }

// GetTargetRevision 查询并返回指定资源数据。
func (s argoAppSnapshotStub) GetTargetRevision() string { return s.targetRevision }

// GetDestServer 查询并返回指定资源数据。
func (s argoAppSnapshotStub) GetDestServer() string { return "" }

// GetDestNamespace 查询并返回指定资源数据。
func (s argoAppSnapshotStub) GetDestNamespace() string { return "" }

// GetSyncStatus 查询并返回指定资源数据。
func (s argoAppSnapshotStub) GetSyncStatus() string { return "" }

// GetHealthStatus 查询并返回指定资源数据。
func (s argoAppSnapshotStub) GetHealthStatus() string { return "" }

// GetOperationPhase 查询并返回指定资源数据。
func (s argoAppSnapshotStub) GetOperationPhase() string { return "" }

// GetRawMeta 查询并返回指定资源数据。
func (s argoAppSnapshotStub) GetRawMeta() string { return "" }

// TestResolveGitOpsTargetBranchUsesApplicationMapping 解析上下文数据，得到后续流程需要的结果。
func TestResolveGitOpsTargetBranchUsesApplicationMapping(t *testing.T) {
	manager := &ReleaseOrderManager{
		appRepo: applicationRepositoryStub{
			app: appdomain.Application{
				ID:  "app-1",
				Key: "java-nantong-test",
				GitOpsBranchMappings: []appdomain.GitOpsBranchMapping{
					{EnvCode: "prod", Branch: "java-nantong-test-prod"},
				},
			},
		},
	}

	branch := manager.resolveGitOpsTargetBranch(
		context.Background(),
		releasedomain.ReleaseOrder{ApplicationID: "app-1", EnvCode: "prod"},
		nil,
		argocddomain.Instance{},
		argoAppSnapshotStub{targetRevision: "master"},
	)

	if branch != "java-nantong-test-prod" {
		t.Fatalf("expected mapped branch, got %q", branch)
	}
}

// TestResolveGitOpsTargetBranchDefaultsToAppKeyEnv 解析上下文数据，得到后续流程需要的结果。
func TestResolveGitOpsTargetBranchDefaultsToAppKeyEnv(t *testing.T) {
	manager := &ReleaseOrderManager{
		appRepo: applicationRepositoryStub{
			app: appdomain.Application{
				ID:  "app-1",
				Key: "java-nantong-test",
			},
		},
	}

	branch := manager.resolveGitOpsTargetBranch(
		context.Background(),
		releasedomain.ReleaseOrder{ApplicationID: "app-1", EnvCode: "test"},
		nil,
		argocddomain.Instance{},
		argoAppSnapshotStub{targetRevision: "master"},
	)

	if branch != "java-nantong-test-test" {
		t.Fatalf("expected default app-env branch, got %q", branch)
	}
}

// TestBuildArgoCDSourcePathCandidatesPrefersHoistedHelmPath 组装业务执行所需的输入数据。
func TestBuildArgoCDSourcePathCandidatesPrefersHoistedHelmPath(t *testing.T) {
	items := buildArgoCDSourcePathCandidates("apps/java-nantong-test", "dev", releasedomain.GitOpsTypeHelm)
	if len(items) < 2 {
		t.Fatalf("expected multiple helm path candidates, got %v", items)
	}
	if items[0] != "apps/helm" {
		t.Fatalf("expected first candidate apps/helm, got %q", items[0])
	}
	if items[1] != "apps/java-nantong-test/helm" {
		t.Fatalf("expected second candidate old app helm path, got %q", items[1])
	}
}
