package observability

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/expfmt"
	metrics "gos/internal/domain/observability"
)

type snapshotStub struct {
	snapshot metrics.Snapshot
	err      error
	calls    atomic.Int32
}

func (r *snapshotStub) LoadSnapshot(context.Context) (metrics.Snapshot, error) {
	r.calls.Add(1)
	return r.snapshot, r.err
}

func metricsFixture() metrics.Snapshot {
	scope := metrics.Scope{ApplicationID: "app-order", Env: "prod"}
	execution := metrics.ExecutionScope{Scope: scope, PipelineScope: "cd", Provider: "argocd"}
	return metrics.Snapshot{
		Applications:    []metrics.Application{{ID: "app-order", ProjectID: "project-trade", Key: "order-service", Name: "订单服务"}},
		Projects:        []metrics.Project{{ID: "project-trade", Key: "trade", Name: "交易平台"}},
		Scopes:          []metrics.Scope{scope, {ApplicationID: "app-order", Env: "test"}},
		Orders:          []metrics.OrderCount{{Scope: scope, Status: "building", Count: 2}, {Scope: scope, Status: "building", Count: 1}},
		LatestOrders:    []metrics.OrderCount{{Scope: scope, Status: "building", Count: 1}},
		Created:         []metrics.CreatedCount{{Scope: scope, Trigger: "manual", Operation: "deploy", Count: 128}},
		ExecutionScopes: []metrics.ExecutionScope{execution},
		Executions:      []metrics.ExecutionCount{{ExecutionScope: execution, Result: "success", Count: 123}, {ExecutionScope: execution, Result: "failed", Count: 5}},
		Durations:       []metrics.Duration{{ExecutionScope: execution, Count: 1, Sum: 78, Buckets: map[float64]uint64{90: 1, 120: 1, 180: 1, 240: 1, 300: 1, 450: 1, 600: 1, 900: 1, 1800: 1, 3600: 1, 7200: 1}}},
		Approvals:       []metrics.ApprovalCount{{Scope: scope, Gate: "before_cd", Count: 3}},
		Agents:          []metrics.Agent{{ID: "agent-1", Code: "prod-01", Name: "生产节点", Env: "prod", Status: "offline", LastHeartbeat: 1700000000}},
	}
}

func findSample(t *testing.T, e *Exporter, name string, labels map[string]string) *dto.Metric {
	t.Helper()
	families, err := e.registry.Gather()
	if err != nil {
		t.Fatal(err)
	}
	for _, family := range families {
		if family.GetName() != name {
			continue
		}
		for _, sample := range family.Metric {
			actual := map[string]string{}
			for _, label := range sample.Label {
				actual[label.GetName()] = label.GetValue()
			}
			matches := true
			for key, value := range labels {
				if actual[key] != value {
					matches = false
				}
			}
			if matches {
				return sample
			}
		}
	}
	t.Fatalf("missing sample %s %v", name, labels)
	return nil
}

func TestExporterContractDimensionsZerosAndCachedScrapes(t *testing.T) {
	t.Parallel()
	r := &snapshotStub{snapshot: metricsFixture()}
	e, err := NewExporter(r, metrics.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := e.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	labels := map[string]string{"project_id": "project-trade", "application_id": "app-order", "env": "prod", "status": "building"}
	if got := findSample(t, e, "gos_release_orders", labels).GetGauge().GetValue(); got != 3 {
		t.Fatalf("building count=%v", got)
	}
	labels["status"] = "deploy_failed"
	if got := findSample(t, e, "gos_release_orders", labels).GetGauge().GetValue(); got != 0 {
		t.Fatalf("missing zero failed state: %v", got)
	}
	labels["env"] = "test"
	if got := findSample(t, e, "gos_release_orders", labels).GetGauge().GetValue(); got != 0 {
		t.Fatalf("environment leaked: %v", got)
	}
	if got := findSample(t, e, "gos_release_executions_total", map[string]string{"result": "cancelled"}).GetCounter().GetValue(); got != 0 {
		t.Fatalf("missing zero result: %v", got)
	}
	h := findSample(t, e, "gos_release_execution_duration_seconds", map[string]string{"scope": "cd"}).GetHistogram()
	if h.GetSampleCount() != 1 || h.GetSampleSum() != 78 {
		t.Fatalf("bad histogram: %v", h)
	}
	if got := findSample(t, e, "gos_agent_state", map[string]string{"agent_id": "agent-1", "status": "offline"}).GetGauge().GetValue(); got != 1 {
		t.Fatalf("offline Agent=%v", got)
	}
	server := httptest.NewServer(e.Handler())
	defer server.Close()
	response, err := http.Get(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	decoder := expfmt.NewDecoder(response.Body, expfmt.ResponseFormat(response.Header))
	count := 0
	for {
		var family dto.MetricFamily
		err := decoder.Decode(&family)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		count++
	}
	if count < 10 || response.StatusCode != 200 {
		t.Fatalf("invalid exposition: families=%d status=%d", count, response.StatusCode)
	}
	if r.calls.Load() != 1 {
		t.Fatal("scrape performed database I/O")
	}
}

func TestExporterRetainsLastGoodSnapshotAndRemovesOldMetadata(t *testing.T) {
	t.Parallel()
	r := &snapshotStub{snapshot: metricsFixture()}
	e, err := NewExporter(r, metrics.Config{})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(1700000000, 0)
	e.now = func() time.Time { return now }
	if err := e.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	r.err = errors.New("database unavailable")
	now = now.Add(time.Minute)
	if err := e.Refresh(context.Background()); err == nil {
		t.Fatal("expected refresh failure")
	}
	if got := findSample(t, e, "gos_metrics_collection_success", nil).GetGauge().GetValue(); got != 0 {
		t.Fatal("failed collection reported success")
	}
	if got := findSample(t, e, "gos_metrics_last_success_timestamp_seconds", nil).GetGauge().GetValue(); got != 1700000000 {
		t.Fatalf("freshness advanced on failure: %v", got)
	}
	if got := findSample(t, e, "gos_release_orders", map[string]string{"status": "building", "env": "prod"}).GetGauge().GetValue(); got != 3 {
		t.Fatal("last good data lost")
	}
	r.err = nil
	r.snapshot.Applications[0].Name = "订单中心"
	if err := e.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	families, err := e.registry.Gather()
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range families {
		if f.GetName() == "gos_application_info" && len(f.Metric) != 1 {
			t.Fatal("renamed application left a stale metadata series")
		}
	}
	findSample(t, e, "gos_application_info", map[string]string{"application_name": "订单中心"})
}

func TestBusinessSourcesCollectValuesAndIsolateFailures(t *testing.T) {
	t.Parallel()
	var failed atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer source-token" {
			w.WriteHeader(401)
			return
		}
		if failed.Load() {
			_, _ = w.Write([]byte(`{"orders":{"created_total":2000}}`))
			return
		}
		_, _ = w.Write([]byte(`{"orders":{"created_total":1000,"pending":28},"payments":{"success":9850,"failed":150}}`))
	}))
	defer server.Close()
	source := metrics.BusinessSource{ID: "order-prod", ApplicationID: "app-order", Env: "prod", URL: server.URL, BearerToken: "source-token", Metrics: []metrics.BusinessMetric{
		{Name: "biz_orders_created_total", Help: "Created orders.", Type: "counter", Path: "/orders/created_total"},
		{Name: "biz_pending_orders", Help: "Pending orders.", Type: "gauge", Path: "/orders/pending"},
		{Name: "biz_payment_attempts_total", Help: "Payment attempts.", Type: "counter", Path: "/payments/success", Labels: map[string]string{"result": "success"}},
		{Name: "biz_payment_attempts_total", Help: "Payment attempts.", Type: "counter", Path: "/payments/failed", Labels: map[string]string{"result": "failed"}},
	}}
	e, err := NewExporter(&snapshotStub{snapshot: metricsFixture()}, metrics.Config{BusinessSources: []metrics.BusinessSource{source}})
	if err != nil {
		t.Fatal(err)
	}
	if err := e.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := findSample(t, e, "biz_pending_orders", map[string]string{"source_id": "order-prod", "project_id": "project-trade", "application_id": "app-order", "env": "prod"}).GetGauge().GetValue(); got != 28 {
		t.Fatalf("pending=%v", got)
	}
	if got := findSample(t, e, "biz_payment_attempts_total", map[string]string{"result": "success"}).GetCounter().GetValue(); got != 9850 {
		t.Fatalf("payment count=%v", got)
	}
	failed.Store(true)
	if err := e.Refresh(context.Background()); err == nil {
		t.Fatal("missing JSON field must fail the whole source")
	}
	if got := findSample(t, e, "gos_business_source_up", nil).GetGauge().GetValue(); got != 0 {
		t.Fatal("failed source reported up")
	}
	if got := findSample(t, e, "gos_metrics_collection_success", nil).GetGauge().GetValue(); got != 1 {
		t.Fatal("business failure hid healthy database collection")
	}
	if got := findSample(t, e, "biz_orders_created_total", nil).GetCounter().GetValue(); got != 1000 {
		t.Fatal("source published partial update")
	}
}

func TestBusinessSourceRejectsInvalidResponses(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, body string
		status     int
	}{
		{"missing", `{}`, 200}, {"string", `{"value":"28"}`, 200}, {"negative counter", `{"value":-1}`, 200},
		{"nonfinite", `{"value":1e999}`, 200}, {"trailing JSON", `{"value":1} {"value":2}`, 200},
		{"oversize", strings.Repeat("x", maxBusinessResponseBytes+1), 200}, {"redirect", ``, 302}, {"unavailable", ``, 503},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer server.Close()
			source := metrics.BusinessSource{ID: "source", ApplicationID: "app-order", Env: "prod", URL: server.URL, Metrics: []metrics.BusinessMetric{{Name: "biz_value_total", Help: "Value.", Type: "counter", Path: "/value"}}}
			e, err := NewExporter(&snapshotStub{snapshot: metricsFixture()}, metrics.Config{BusinessSources: []metrics.BusinessSource{source}})
			if err != nil {
				t.Fatal(err)
			}
			if err := e.Refresh(context.Background()); err == nil {
				t.Fatal("expected invalid-source failure")
			}
			if got := findSample(t, e, "gos_business_source_up", nil).GetGauge().GetValue(); got != 0 {
				t.Fatal("invalid source reported up")
			}
		})
	}
}

func TestBusinessSourceTimeoutDoesNotBlockScrapes(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { <-r.Context().Done() }))
	defer server.Close()
	source := metrics.BusinessSource{ID: "slow", ApplicationID: "app-order", Env: "prod", URL: server.URL, Metrics: []metrics.BusinessMetric{{Name: "biz_value", Help: "Value.", Type: "gauge", Path: "/value"}}}
	e, err := NewExporter(&snapshotStub{snapshot: metricsFixture()}, metrics.Config{BusinessSources: []metrics.BusinessSource{source}})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if err := e.Refresh(ctx); err == nil {
		t.Fatal("expected source timeout")
	}
	rec := httptest.NewRecorder()
	e.Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/metrics", nil))
	if rec.Code != 200 {
		t.Fatalf("scrape blocked on failed dependency: %d", rec.Code)
	}
}

func TestExporterConcurrentRefreshAndScrapes(t *testing.T) {
	t.Parallel()
	e, err := NewExporter(&snapshotStub{snapshot: metricsFixture()}, metrics.Config{})
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 5; j++ {
				rec := httptest.NewRecorder()
				e.Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/metrics", nil))
				if rec.Code != 200 {
					t.Errorf("concurrent scrape status=%d", rec.Code)
				}
			}
		}()
	}
	for i := 0; i < 5; i++ {
		if err := e.Refresh(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	wg.Wait()
}
