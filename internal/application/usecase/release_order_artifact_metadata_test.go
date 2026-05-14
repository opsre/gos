package usecase

import (
	"context"
	"errors"
	"testing"
	"time"

	domain "gos/internal/domain/release"
)

// TestRecordArtifactMetadataPersistsReleaseOrderArtifact 记录发布单制品元信息。
func TestRecordArtifactMetadataPersistsReleaseOrderArtifact(t *testing.T) {
	t.Parallel()

	manager, repo := newReleaseOrderManagerForCancelTest(t)
	ctx := context.Background()
	now := time.Date(2026, 5, 12, 10, 30, 0, 0, time.UTC)
	manager.now = func() time.Time { return now }

	order := testReleaseOrder("ro-artifact-meta", "RO-ARTIFACT-META", domain.OrderStatusBuilding, now)
	execution := testReleaseExecution(order.ID, "exec-ci-artifact", domain.PipelineScopeCI, domain.ExecutionStatusSuccess, now)
	if err := repo.Create(ctx, order, []domain.ReleaseOrderExecution{execution}, nil, nil); err != nil {
		t.Fatalf("Create release order failed: %v", err)
	}

	output, err := manager.RecordArtifactMetadata(ctx, order.ID, RecordReleaseOrderArtifactMetadataInput{
		ExecutionID:      execution.ID,
		PipelineScope:    string(domain.PipelineScopeCI),
		ArtifactName:     "gc-certificate.jar",
		ArtifactType:     "jar",
		ArtifactVersion:  "1042",
		ArtifactURL:      "https://gc-oa.oss-cn-shanghai.aliyuncs.com/tempUpdate/gc-certificate-1042.jar",
		RepositoryID:     "repo-oss",
		RepositoryName:   "生产 OSS",
		Bucket:           "gc-oa",
		ObjectKey:        "tempUpdate/gc-certificate-1042.jar",
		Checksum:         "abc123",
		ChecksumType:     "sha256",
		SizeBytes:        171609374,
		BuildNumber:      "1042",
		AdditionalFields: map[string]any{"commit": "abcdef1"},
	})
	if err != nil {
		t.Fatalf("RecordArtifactMetadata failed: %v", err)
	}
	if output.ReleaseOrderID != order.ID {
		t.Fatalf("ReleaseOrderID = %q, want %q", output.ReleaseOrderID, order.ID)
	}
	if output.ArtifactName != "gc-certificate.jar" {
		t.Fatalf("ArtifactName = %q", output.ArtifactName)
	}
	if output.AdditionalFields["commit"] != "abcdef1" {
		t.Fatalf("AdditionalFields[commit] = %v", output.AdditionalFields["commit"])
	}

	items, err := manager.ListArtifactMetadata(ctx, order.ID)
	if err != nil {
		t.Fatalf("ListArtifactMetadata failed: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("artifact metadata count = %d, want 1", len(items))
	}
	if items[0].ArtifactURL != output.ArtifactURL {
		t.Fatalf("ArtifactURL = %q, want %q", items[0].ArtifactURL, output.ArtifactURL)
	}
}

func TestDeleteArtifactMetadataAllowsManualArtifactOnly(t *testing.T) {
	t.Parallel()

	manager, repo := newReleaseOrderManagerForCancelTest(t)
	ctx := context.Background()
	now := time.Date(2026, 5, 13, 9, 0, 0, 0, time.UTC)
	manager.now = func() time.Time { return now }

	order := testReleaseOrder("ro-artifact-delete", "RO-ARTIFACT-DELETE", domain.OrderStatusSuccess, now)
	execution := testReleaseExecution(order.ID, "exec-ci-artifact-delete", domain.PipelineScopeCI, domain.ExecutionStatusSuccess, now)
	if err := repo.Create(ctx, order, []domain.ReleaseOrderExecution{execution}, nil, nil); err != nil {
		t.Fatalf("Create release order failed: %v", err)
	}

	manualArtifact, err := manager.RecordArtifactMetadata(ctx, order.ID, RecordReleaseOrderArtifactMetadataInput{
		PipelineScope:    string(domain.PipelineScopeCI),
		ArtifactName:     "manual-package.zip",
		ArtifactURL:      "https://oss.example.com/manual-package.zip",
		AdditionalFields: map[string]any{"source": "manual"},
	})
	if err != nil {
		t.Fatalf("Record manual artifact metadata failed: %v", err)
	}
	processArtifact, err := manager.RecordArtifactMetadata(ctx, order.ID, RecordReleaseOrderArtifactMetadataInput{
		ExecutionID:   execution.ID,
		PipelineScope: string(domain.PipelineScopeCI),
		ArtifactName:  "pipeline-package.zip",
		ArtifactURL:   "https://oss.example.com/pipeline-package.zip",
	})
	if err != nil {
		t.Fatalf("Record process artifact metadata failed: %v", err)
	}

	if err := manager.DeleteArtifactMetadata(ctx, order.ID, manualArtifact.ID); err != nil {
		t.Fatalf("DeleteArtifactMetadata manual artifact failed: %v", err)
	}

	items, err := manager.ListArtifactMetadata(ctx, order.ID)
	if err != nil {
		t.Fatalf("ListArtifactMetadata failed: %v", err)
	}
	if len(items) != 1 || items[0].ID != processArtifact.ID {
		t.Fatalf("remaining artifacts = %#v, want only process artifact %q", items, processArtifact.ID)
	}

	err = manager.DeleteArtifactMetadata(ctx, order.ID, processArtifact.ID)
	if !errors.Is(err, ErrInvalidStatus) {
		t.Fatalf("DeleteArtifactMetadata process artifact err = %v, want ErrInvalidStatus", err)
	}
}

func TestListArtifactMetadataSummariesFiltersAndBuildsReleaseDisplayName(t *testing.T) {
	t.Parallel()

	manager, repo := newReleaseOrderManagerForCancelTest(t)
	ctx := context.Background()
	now := time.Date(2026, 5, 12, 15, 11, 21, 0, time.UTC)
	manager.now = func() time.Time { return now }

	order := testReleaseOrder("ro-artifact-center-summary", "RO-20260512070847-8C67ECB7", domain.OrderStatusSuccess, now)
	order.ReleaseName = "尚信前端测试发布"
	order.ApplicationID = "app-notary"
	order.ApplicationName = "尚信前端-测试"
	order.EnvCode = "dev"
	if err := repo.Create(ctx, order, nil, nil, nil); err != nil {
		t.Fatalf("Create release order failed: %v", err)
	}
	if _, err := manager.RecordArtifactMetadata(ctx, order.ID, RecordReleaseOrderArtifactMetadataInput{
		PipelineScope:    string(domain.PipelineScopeCI),
		ArtifactName:     "notarybusiness-9.zip",
		ArtifactType:     "zip",
		ArtifactVersion:  "9",
		ArtifactURL:      "https://oss.example.com/notarybusiness-9.zip",
		RepositoryID:     "repo-oss",
		RepositoryName:   "生产 OSS",
		Bucket:           "gc-oa",
		ObjectKey:        "release/notarybusiness-9.zip",
		BuildNumber:      "9",
		AdditionalFields: map[string]any{"commit": "abcdef1"},
	}); err != nil {
		t.Fatalf("RecordArtifactMetadata failed: %v", err)
	}

	items, total, err := manager.ListArtifactMetadataSummaries(ctx, ListReleaseOrderArtifactMetadataInput{
		ApplicationID: "app-notary",
		Keyword:       "前端测试",
		PipelineScope: string(domain.PipelineScopeCI),
		Page:          1,
		PageSize:      20,
	})
	if err != nil {
		t.Fatalf("ListArtifactMetadataSummaries failed: %v", err)
	}
	if total != 1 || len(items) != 1 {
		t.Fatalf("total=%d len=%d, want 1", total, len(items))
	}
	got := items[0]
	if got.ReleaseDisplayName != "尚信前端测试发布 - RO-20260512070847-8C67ECB7" {
		t.Fatalf("ReleaseDisplayName = %q", got.ReleaseDisplayName)
	}
	if got.ApplicationName != "尚信前端-测试" || got.EnvCode != "dev" {
		t.Fatalf("context = %q/%q", got.ApplicationName, got.EnvCode)
	}
	if got.AdditionalFields["commit"] != "abcdef1" {
		t.Fatalf("metadata commit = %v", got.AdditionalFields["commit"])
	}
}

// TestTrackReleaseExecutionRecordsArtifactURLFromJenkinsLog 从 Jenkins 日志提取 GOS_ARTIFACT_URL。
func TestTrackReleaseExecutionRecordsArtifactURLFromJenkinsLog(t *testing.T) {
	t.Parallel()

	manager, repo := newReleaseOrderManagerForCancelTest(t)
	ctx := context.Background()
	now := time.Date(2026, 5, 12, 14, 0, 0, 0, time.UTC)
	manager.now = func() time.Time { return now }

	order := testReleaseOrder("ro-log-artifact", "RO-LOG-ARTIFACT", domain.OrderStatusRunning, now)
	execution := testReleaseExecution(order.ID, "exec-log-ci", domain.PipelineScopeCI, domain.ExecutionStatusRunning, now)
	execution.BuildURL = "http://jenkins.example/job/demo/1042/"
	steps := []domain.ReleaseOrderStep{
		testReleaseStep(order.ID, "step-running", domain.StepScopeCI, "ci:pipeline_running", domain.StepStatusRunning, 10, now),
		testReleaseStep(order.ID, "step-success", domain.StepScopeCI, "ci:pipeline_success", domain.StepStatusPending, 20, now),
		testReleaseStep(order.ID, "step-finish", domain.StepScopeGlobal, "global:release_finish", domain.StepStatusPending, 99, now),
	}
	if err := repo.Create(ctx, order, []domain.ReleaseOrderExecution{execution}, nil, steps); err != nil {
		t.Fatalf("Create release order failed: %v", err)
	}

	tracker := NewTrackReleaseExecution(manager, &releaseArtifactJenkinsLogFake{
		log: `
Upload OSS done
GOS_ARTIFACT_URL=https://gc-oa.oss-cn-shanghai.aliyuncs.com/tempUpdate/gc-certificate-1042.jar
GOS_ARTIFACT_URL=https://gc-oa.oss-cn-shanghai.aliyuncs.com/tempUpdate/gc-admin-1042.jar
`,
	})
	tracker.now = func() time.Time { return now.Add(time.Minute) }

	if _, _, err := tracker.syncOrder(ctx, order); err != nil {
		t.Fatalf("syncOrder failed: %v", err)
	}

	items, err := manager.ListArtifactMetadata(ctx, order.ID)
	if err != nil {
		t.Fatalf("ListArtifactMetadata failed: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("artifact metadata count = %d, want 2", len(items))
	}
	certificate := findArtifactMetadataByName(items, "gc-certificate-1042.jar")
	if certificate == nil {
		t.Fatalf("gc-certificate-1042.jar artifact metadata not found: %#v", items)
	}
	if certificate.ArtifactURL != "https://gc-oa.oss-cn-shanghai.aliyuncs.com/tempUpdate/gc-certificate-1042.jar" {
		t.Fatalf("artifact_url = %q", certificate.ArtifactURL)
	}
	if certificate.ArtifactType != "jar" {
		t.Fatalf("artifact_type = %q, want jar", certificate.ArtifactType)
	}
	if certificate.BuildNumber != "1042" {
		t.Fatalf("build_number = %q, want 1042", certificate.BuildNumber)
	}
}

func findArtifactMetadataByName(items []ReleaseOrderArtifactMetadataOutput, name string) *ReleaseOrderArtifactMetadataOutput {
	for i := range items {
		if items[i].ArtifactName == name {
			return &items[i]
		}
	}
	return nil
}

type releaseArtifactJenkinsLogFake struct {
	log string
}

func (f *releaseArtifactJenkinsLogFake) GetQueueItem(context.Context, string) (string, bool, string, error) {
	return "", false, "", nil
}

func (f *releaseArtifactJenkinsLogFake) GetBuildStatus(context.Context, string) (bool, string, error) {
	return false, "SUCCESS", nil
}

func (f *releaseArtifactJenkinsLogFake) GetBuildConsoleText(_ context.Context, _ string, start int64) (string, int64, bool, error) {
	if start > 0 {
		return "", start, false, nil
	}
	return f.log, int64(len(f.log)), false, nil
}
