package sqlrepo

import (
	"context"
	"database/sql"
	"testing"
	"time"

	appdomain "gos/internal/domain/application"
	executordomain "gos/internal/domain/executorparam"
	pipelinedomain "gos/internal/domain/pipeline"

	_ "modernc.org/sqlite"
)

// TestExecutorParamRepositoryListByApplicationsMatchesKeywordAcrossApplicationAndParamKey 查询并返回列表数据。
func TestExecutorParamRepositoryListByApplicationsMatchesKeywordAcrossApplicationAndParamKey(t *testing.T) {
	t.Parallel()

	bundle := newTestExecutorParamRepositoryBundle(t)
	seedExecutorParamSearchFixture(t, bundle)

	cases := []struct {
		name               string
		keyword            string
		wantApplicationIDs []string
	}{
		{
			name:               "matches application name",
			keyword:            "支付",
			wantApplicationIDs: []string{"app-pay"},
		},
		{
			name:               "matches application key",
			keyword:            "order-hub",
			wantApplicationIDs: []string{"app-order"},
		},
		{
			name:               "matches mapped platform key",
			keyword:            "branch",
			wantApplicationIDs: []string{"app-pay"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			items, total, err := bundle.executorParams.ListByApplications(context.Background(), executordomain.ApplicationListFilter{
				Keyword:     tc.keyword,
				BindingType: pipelinedomain.BindingTypeCI,
				Page:        1,
				PageSize:    20,
			})
			if err != nil {
				t.Fatalf("ListByApplications failed: %v", err)
			}
			if int(total) != len(tc.wantApplicationIDs) {
				t.Fatalf("total = %d, want %d", total, len(tc.wantApplicationIDs))
			}
			if len(items) != len(tc.wantApplicationIDs) {
				t.Fatalf("len(items) = %d, want %d", len(items), len(tc.wantApplicationIDs))
			}
			for index, wantApplicationID := range tc.wantApplicationIDs {
				if items[index].ApplicationID != wantApplicationID {
					t.Fatalf("items[%d].ApplicationID = %q, want %q", index, items[index].ApplicationID, wantApplicationID)
				}
			}
		})
	}
}

// TestExecutorParamRepositoryListByApplicationsHonorsApplicationScope 查询并返回列表数据。
func TestExecutorParamRepositoryListByApplicationsHonorsApplicationScope(t *testing.T) {
	t.Parallel()

	bundle := newTestExecutorParamRepositoryBundle(t)
	seedExecutorParamSearchFixture(t, bundle)

	items, total, err := bundle.executorParams.ListByApplications(context.Background(), executordomain.ApplicationListFilter{
		ApplicationIDs: []string{"app-order"},
		BindingType:    pipelinedomain.BindingTypeCI,
		Page:           1,
		PageSize:       20,
	})
	if err != nil {
		t.Fatalf("ListByApplications failed: %v", err)
	}
	if total != 1 {
		t.Fatalf("total = %d, want 1", total)
	}
	if len(items) != 1 {
		t.Fatalf("len(items) = %d, want 1", len(items))
	}
	if items[0].ApplicationID != "app-order" {
		t.Fatalf("items[0].ApplicationID = %q, want %q", items[0].ApplicationID, "app-order")
	}
}

type executorParamRepositoryTestBundle struct {
	applications   *ApplicationRepository
	pipelines      *PipelineRepository
	executorParams *ExecutorParamRepository
}

// newTestExecutorParamRepositoryBundle 封装当前模块的业务处理逻辑。
func newTestExecutorParamRepositoryBundle(t *testing.T) executorParamRepositoryTestBundle {
	t.Helper()

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite failed: %v", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	t.Cleanup(func() {
		_ = db.Close()
	})

	applications := NewApplicationRepository(db, "sqlite")
	pipelines := NewPipelineRepository(db, "sqlite")
	executorParams := NewExecutorParamRepository(db, "sqlite")

	ctx := context.Background()
	if err := applications.InitSchema(ctx); err != nil {
		t.Fatalf("init application schema failed: %v", err)
	}
	if err := pipelines.InitSchema(ctx); err != nil {
		t.Fatalf("init pipeline schema failed: %v", err)
	}
	if err := executorParams.InitSchema(ctx); err != nil {
		t.Fatalf("init executor param schema failed: %v", err)
	}

	return executorParamRepositoryTestBundle{
		applications:   applications,
		pipelines:      pipelines,
		executorParams: executorParams,
	}
}

// seedExecutorParamSearchFixture 封装当前模块的业务处理逻辑。
func seedExecutorParamSearchFixture(t *testing.T, bundle executorParamRepositoryTestBundle) {
	t.Helper()

	ctx := context.Background()
	now := time.Unix(1_710_000_000, 0).UTC()

	createApplication := func(id, name, key string) {
		app := appdomain.Application{
			ID:           id,
			Name:         name,
			Key:          key,
			Status:       appdomain.StatusActive,
			ArtifactType: "docker-image",
			CreatedAt:    now,
			UpdatedAt:    now,
		}
		app.SetLanguage("golang")
		if err := bundle.applications.Create(ctx, app); err != nil {
			t.Fatalf("Create application %s failed: %v", id, err)
		}
	}

	createApplication("app-pay", "支付中心", "pay-center")
	createApplication("app-order", "订单枢纽", "order-hub")

	createBinding := func(id, name, applicationID, applicationName, pipelineID string) {
		err := bundle.pipelines.CreateBinding(ctx, pipelinedomain.PipelineBinding{
			ID:              id,
			Name:            name,
			ApplicationID:   applicationID,
			ApplicationName: applicationName,
			BindingType:     pipelinedomain.BindingTypeCI,
			Provider:        pipelinedomain.ProviderJenkins,
			PipelineID:      pipelineID,
			TriggerMode:     pipelinedomain.TriggerManual,
			Status:          pipelinedomain.StatusActive,
			CreatedAt:       now,
			UpdatedAt:       now,
		})
		if err != nil {
			t.Fatalf("CreateBinding %s failed: %v", id, err)
		}
	}

	createBinding("binding-pay", "支付 CI", "app-pay", "支付中心", "pipeline-pay")
	createBinding("binding-order", "订单 CI", "app-order", "订单枢纽", "pipeline-order")

	items := []executordomain.ExecutorParamDef{
		{
			ID:                "param-pay-branch",
			PipelineID:        "pipeline-pay",
			ExecutorType:      executordomain.ExecutorTypeJenkins,
			ExecutorParamName: "GIT_BRANCH",
			ParamKey:          "branch",
			ParamType:         executordomain.ParamTypeString,
			Required:          true,
			Visible:           true,
			Editable:          true,
			SourceFrom:        executordomain.SourceFromSyncJenkins,
			Status:            executordomain.StatusActive,
			RawMeta:           "{}",
			SortNo:            10,
			CreatedAt:         now,
			UpdatedAt:         now,
		},
		{
			ID:                "param-order-version",
			PipelineID:        "pipeline-order",
			ExecutorType:      executordomain.ExecutorTypeJenkins,
			ExecutorParamName: "IMAGE_TAG",
			ParamKey:          "image_tag",
			ParamType:         executordomain.ParamTypeString,
			Required:          true,
			Visible:           true,
			Editable:          true,
			SourceFrom:        executordomain.SourceFromSyncJenkins,
			Status:            executordomain.StatusActive,
			RawMeta:           "{}",
			SortNo:            20,
			CreatedAt:         now,
			UpdatedAt:         now,
		},
	}
	if _, _, err := bundle.executorParams.Upsert(ctx, items); err != nil {
		t.Fatalf("Upsert executor params failed: %v", err)
	}
}
