package sqlrepo

import (
	"context"
	"testing"
	"time"

	app "gos/internal/domain/application"
	metrics "gos/internal/domain/observability"
	project "gos/internal/domain/project"
	release "gos/internal/domain/release"
)

func newMetricsTestRepository(t *testing.T) (*ReleaseRepository, *MetricsRepository) {
	t.Helper()
	r := newTestReleaseRepository(t)
	if err := NewAgentRepository(r.db, "sqlite").InitSchema(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := NewProjectRepository(r.db, "sqlite").Create(context.Background(), project.Project{ID: "project-trade", Name: "交易平台", Key: "trade"}); err != nil {
		t.Fatal(err)
	}
	if err := NewApplicationRepository(r.db, "sqlite").Create(context.Background(), app.Application{ID: "app-order", Name: "订单服务", Key: "order-service", ProjectID: "project-trade"}); err != nil {
		t.Fatal(err)
	}
	return r, NewMetricsRepository(r.db)
}

func TestMetricsEventsPersistAcrossRetriesDeletionAndRestart(t *testing.T) {
	t.Parallel()
	r, m := newMetricsTestRepository(t)
	ctx := context.Background()
	now := time.Now().UTC()
	started := now.Add(-78 * time.Second)
	order := newTestReleaseOrder("metrics-order", "METRICS-ORDER", "app-order", "prod", release.OrderStatusDeploying, started)
	execution := release.ReleaseOrderExecution{ID: "metrics-cd", ReleaseOrderID: order.ID, PipelineScope: release.PipelineScopeCD, Provider: "argocd", Status: release.ExecutionStatusRunning, StartedAt: &started, CreatedAt: started, UpdatedAt: started}
	if err := r.Create(ctx, order, []release.ReleaseOrderExecution{execution}, nil, nil); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		finished := now.Add(time.Duration(i) * time.Second)
		if _, err := r.UpdateExecutionByScope(ctx, order.ID, release.PipelineScopeCD, release.ExecutionUpdateInput{Status: release.ExecutionStatusSuccess, StartedAt: &started, FinishedAt: &finished, UpdatedAt: finished}); err != nil {
			t.Fatal(err)
		}
	}
	check := func() metrics.Snapshot {
		s, err := m.LoadSnapshot(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if len(s.Created) != 1 || s.Created[0].Count != 1 || len(s.Executions) != 1 || s.Executions[0].Count != 1 {
			t.Fatalf("events duplicated or lost: created=%+v executions=%+v", s.Created, s.Executions)
		}
		if len(s.Durations) != 1 || s.Durations[0].Count != 1 || s.Durations[0].Sum != 78 || s.Durations[0].Buckets[60] != 0 || s.Durations[0].Buckets[90] != 1 {
			t.Fatalf("duration observation changed: %+v", s.Durations)
		}
		if len(s.LastSuccess) != 1 || s.LastSuccess[0].Seconds != float64(now.UnixNano())/1e9 {
			t.Fatalf("last success changed: %+v", s.LastSuccess)
		}
		return s
	}
	check()
	if err := r.DeleteOrders(ctx, []string{order.ID}); err != nil {
		t.Fatal(err)
	}
	if err := NewReleaseRepository(r.db, "sqlite").InitSchema(ctx); err != nil {
		t.Fatal(err)
	}
	s := check()
	if len(s.Orders) != 0 || len(s.Scopes) != 1 {
		t.Fatalf("snapshot/retained event mismatch: %+v", s)
	}
}

func TestMetricsSnapshotProjectsActualCIAndKeepsEnvironmentsSeparate(t *testing.T) {
	t.Parallel()
	r, m := newMetricsTestRepository(t)
	ctx := context.Background()
	now := time.Now().UTC()
	for _, env := range []string{"prod", "test"} {
		order := newTestReleaseOrder("phase-"+env, "PHASE-"+env, "app-order", env, release.OrderStatusDeploying, now)
		status := release.ExecutionStatusRunning
		if env == "test" {
			status = release.ExecutionStatusFailed
		}
		execution := release.ReleaseOrderExecution{ID: "ci-" + env, ReleaseOrderID: order.ID, PipelineScope: release.PipelineScopeCI, Provider: "jenkins", Status: status, CreatedAt: now, UpdatedAt: now}
		if err := r.Create(ctx, order, []release.ReleaseOrderExecution{execution}, nil, nil); err != nil {
			t.Fatal(err)
		}
	}
	s, err := m.LoadSnapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	states := map[string]string{}
	for _, v := range s.Orders {
		states[v.Env] = v.Status
	}
	if states["prod"] != "building" || states["test"] != "deploy_failed" {
		t.Fatalf("incorrect phase projection: %+v", states)
	}
	if len(s.Applications) != 1 || s.Applications[0].ProjectID != "project-trade" || len(s.Projects) != 1 {
		t.Fatalf("missing metadata: %+v", s)
	}
}

func TestMetricsBackfillIsIdempotentAndIgnoresUnfinishedExecutions(t *testing.T) {
	t.Parallel()
	r, _ := newMetricsTestRepository(t)
	ctx := context.Background()
	now := time.Now().UTC()
	started := now.Add(-30 * time.Second)
	order := newTestReleaseOrder("backfill", "BACKFILL", "app-order", "prod", release.OrderStatusSuccess, started)
	executions := []release.ReleaseOrderExecution{
		{ID: "backfill-ci", ReleaseOrderID: order.ID, PipelineScope: release.PipelineScopeCI, Provider: "jenkins", Status: release.ExecutionStatusSuccess, StartedAt: &started, FinishedAt: &now, CreatedAt: started, UpdatedAt: now},
		{ID: "backfill-cd", ReleaseOrderID: order.ID, PipelineScope: release.PipelineScopeCD, Provider: "argocd", Status: release.ExecutionStatusPending, CreatedAt: started, UpdatedAt: now},
	}
	if err := r.Create(ctx, order, executions, nil, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := r.db.Exec(`DELETE FROM gos_metric_release_event`); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err := r.initMetricsSchema(ctx); err != nil {
			t.Fatal(err)
		}
	}
	var count int
	if err := r.db.QueryRow(`SELECT COUNT(*) FROM gos_metric_release_event`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("backfill count=%d, want 2", count)
	}
}

func TestMetricEventsRollBackWithFailedOrderCreation(t *testing.T) {
	t.Parallel()
	r, _ := newMetricsTestRepository(t)
	now := time.Now().UTC()
	order := newTestReleaseOrder("rollback", "ROLLBACK", "app-order", "prod", release.OrderStatusPending, now)
	params := []release.ReleaseOrderParam{{ID: "duplicate", ReleaseOrderID: order.ID}, {ID: "duplicate", ReleaseOrderID: order.ID}}
	if err := r.Create(context.Background(), order, nil, params, nil); err == nil {
		t.Fatal("expected duplicate parameter failure")
	}
	for _, table := range []string{"release_order", "gos_metric_release_event"} {
		var count int
		if err := r.db.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Fatalf("%s retained %d rows after rollback", table, count)
		}
	}
}

func TestMetricsDurationsExcludeUnstartedInvalidAndCancelledExecutions(t *testing.T) {
	t.Parallel()
	r, m := newMetricsTestRepository(t)
	now := time.Now().UTC()
	later := now.Add(time.Minute)
	for _, tc := range []struct {
		id      string
		status  release.ExecutionStatus
		started *time.Time
	}{
		{"unstarted", release.ExecutionStatusFailed, nil},
		{"invalid-time", release.ExecutionStatusSuccess, &later},
		{"cancelled", release.ExecutionStatusCancelled, &now},
		{"skipped", release.ExecutionStatusSkipped, &now},
	} {
		order := newTestReleaseOrder(tc.id, tc.id, "app-order", "prod", release.OrderStatusRunning, now)
		execution := release.ReleaseOrderExecution{ID: tc.id, ReleaseOrderID: tc.id, PipelineScope: release.PipelineScopeCI,
			Provider: "jenkins", Status: tc.status, StartedAt: tc.started, FinishedAt: &now, CreatedAt: now, UpdatedAt: now}
		if err := r.Create(context.Background(), order, []release.ReleaseOrderExecution{execution}, nil, nil); err != nil {
			t.Fatal(err)
		}
	}
	s, err := m.LoadSnapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Executions) != 4 || len(s.Durations) != 0 {
		t.Fatalf("terminal counts or duration exclusions incorrect: executions=%+v durations=%+v", s.Executions, s.Durations)
	}
}

func TestMetricsAgentStatesAndApprovalBacklog(t *testing.T) {
	t.Parallel()
	r, m := newMetricsTestRepository(t)
	ctx := context.Background()
	now := time.Now().UTC()
	m.now = func() time.Time { return now }
	for _, v := range []struct {
		id, status, task string
		age              time.Duration
	}{{"online", "active", "", 0}, {"busy", "active", "task", 0}, {"offline", "active", "task", time.Minute}, {"maintenance", "maintenance", "", time.Hour}} {
		if _, err := r.db.Exec(`INSERT INTO agent_instance (id,agent_code,name,environment_code,work_dir,status,last_heartbeat_at,current_task_id,created_at,updated_at) VALUES (?,?,?,'prod','/work',?,?,?,?,?)`, v.id, v.id, v.id, v.status, now.Add(-v.age).UnixNano(), v.task, now.UnixNano(), now.UnixNano()); err != nil {
			t.Fatal(err)
		}
	}
	order := newTestReleaseOrder("approval", "APPROVAL", "app-order", "prod", release.OrderStatusPendingApproval, now)
	order.ApprovalRequired = true
	if err := r.Create(ctx, order, nil, nil, nil); err != nil {
		t.Fatal(err)
	}
	s, err := m.LoadSnapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range s.Agents {
		if a.ID != a.Status {
			t.Fatalf("agent %s state=%s", a.ID, a.Status)
		}
	}
	if len(s.Approvals) != 1 || s.Approvals[0].Count != 1 || s.Approvals[0].Gate != "before_execute" {
		t.Fatalf("legacy approval missing: %+v", s.Approvals)
	}
	if _, err := r.db.Exec(`INSERT INTO release_order_approval_flow_instance (id,release_order_id,flow_definition_id,flow_name,flow_snapshot_json,status,created_at,updated_at) VALUES ('flow','approval','','','{}','pending_approval',0,0)`); err != nil {
		t.Fatal(err)
	}
	for _, v := range []struct{ id, nodeType, status string }{{"approval-task", "approval", "pending"}, {"agent-task", "agent_task", "running"}, {"finished-task", "approval", "approved"}} {
		if _, err := r.db.Exec(`INSERT INTO release_order_approval_flow_task (id,instance_id,release_order_id,node_code,node_name,gate,node_type,approval_mode,approver_ids_json,approver_names_json,status,created_at,updated_at) VALUES (?,'flow','approval','','','before_cd',?,'any','[]','[]',?,0,0)`, v.id, v.nodeType, v.status); err != nil {
			t.Fatal(err)
		}
	}
	s, err = m.LoadSnapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Approvals) != 1 || s.Approvals[0].Count != 1 || s.Approvals[0].Gate != "before_cd" {
		t.Fatalf("approval double counted or included Agent tasks: %+v", s.Approvals)
	}
}
