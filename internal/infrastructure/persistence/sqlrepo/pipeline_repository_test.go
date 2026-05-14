package sqlrepo

import (
	"context"
	"database/sql"
	"os"
	"strings"
	"testing"
	"time"

	pipelinedomain "gos/internal/domain/pipeline"

	_ "modernc.org/sqlite"
)

func TestPipelineRepositoryListPipelinesQueryUsesStableTieBreaker(t *testing.T) {
	t.Parallel()

	source, err := os.ReadFile("pipeline_repository.go")
	if err != nil {
		t.Fatalf("read pipeline repository source failed: %v", err)
	}
	if !strings.Contains(string(source), "ORDER BY updated_at DESC, id DESC LIMIT ? OFFSET ?;") {
		t.Fatalf("ListPipelines query must order by updated_at and id to keep offset pagination stable")
	}
}

func TestPipelineRepositoryListPipelinesUsesStablePaginationOrder(t *testing.T) {
	t.Parallel()

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite failed: %v", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	t.Cleanup(func() {
		_ = db.Close()
	})

	ctx := context.Background()
	applications := NewApplicationRepository(db, "sqlite")
	if err := applications.InitSchema(ctx); err != nil {
		t.Fatalf("init application schema failed: %v", err)
	}
	repo := NewPipelineRepository(db, "sqlite")
	if err := repo.InitSchema(ctx); err != nil {
		t.Fatalf("init pipeline schema failed: %v", err)
	}

	now := time.Unix(1_710_000_000, 0).UTC()
	fixtures := []pipelinedomain.Pipeline{
		newPipelineRepositoryTestPipeline("pln-a", "job-a", now),
		newPipelineRepositoryTestPipeline("pln-b", "job-b", now),
		newPipelineRepositoryTestPipeline("pln-c", "job-c", now),
		newPipelineRepositoryTestPipeline("pln-d", "job-d", now),
	}
	if _, _, err := repo.UpsertPipelines(ctx, fixtures); err != nil {
		t.Fatalf("upsert pipelines failed: %v", err)
	}

	firstPage, _, err := repo.ListPipelines(ctx, pipelinedomain.PipelineListFilter{
		Provider: pipelinedomain.ProviderJenkins,
		Status:   pipelinedomain.StatusActive,
		Page:     1,
		PageSize: 2,
	})
	if err != nil {
		t.Fatalf("list first page failed: %v", err)
	}
	secondPage, _, err := repo.ListPipelines(ctx, pipelinedomain.PipelineListFilter{
		Provider: pipelinedomain.ProviderJenkins,
		Status:   pipelinedomain.StatusActive,
		Page:     2,
		PageSize: 2,
	})
	if err != nil {
		t.Fatalf("list second page failed: %v", err)
	}

	got := []string{
		firstPage[0].ID,
		firstPage[1].ID,
		secondPage[0].ID,
		secondPage[1].ID,
	}
	want := []string{"pln-d", "pln-c", "pln-b", "pln-a"}
	for idx := range want {
		if got[idx] != want[idx] {
			t.Fatalf("stable order = %#v, want %#v", got, want)
		}
	}
}

func newPipelineRepositoryTestPipeline(id string, jobName string, now time.Time) pipelinedomain.Pipeline {
	return pipelinedomain.Pipeline{
		ID:           id,
		Provider:     pipelinedomain.ProviderJenkins,
		JobFullName:  jobName,
		JobName:      jobName,
		JobURL:       "http://jenkins/job/" + jobName + "/",
		Status:       pipelinedomain.StatusActive,
		LastSyncedAt: now,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
}
