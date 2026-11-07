package observability

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	metrics "gos/internal/domain/observability"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var orderStates = []string{"draft", "pending_execution", "pending_approval", "approving", "approved", "queued", "building", "built_waiting_deploy", "deploying", "deploy_success", "deploy_failed", "rejected", "cancelled"}
var agentStates = []string{"online", "offline", "busy", "disabled", "maintenance"}
var executionResults = []string{"success", "failed", "cancelled", "skipped"}

type metricSet struct{ metrics []prometheus.Metric }
type businessState struct {
	values      []float64
	up          float64
	lastSuccess float64
}

// Exporter publishes immutable snapshots. Scrapes never perform I/O to the
// database or business endpoints, and only the refresh loop mutates caches.
type Exporter struct {
	repo         metrics.Repository
	cfg          metrics.Config
	registry     *prometheus.Registry
	desc         map[string]*prometheus.Desc
	businessKeys map[string][]string
	current      atomic.Pointer[metricSet]
	refreshMu    sync.Mutex
	core         metrics.Snapshot
	sources      map[string]businessState
	lastSuccess  float64
	now          func() time.Time
	client       *http.Client
}

func NewExporter(repo metrics.Repository, cfg metrics.Config) (*Exporter, error) {
	cfg.Defaults()
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	if repo == nil {
		return nil, errors.New("metrics repository is required")
	}
	e := &Exporter{
		repo: repo, cfg: cfg, registry: prometheus.NewRegistry(), desc: map[string]*prometheus.Desc{},
		businessKeys: map[string][]string{}, sources: map[string]businessState{}, now: time.Now,
		client: &http.Client{
			Timeout:       time.Duration(cfg.CollectionTimeoutSec) * time.Second,
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
	}
	e.initDescriptors()
	e.current.Store(&metricSet{metrics: e.buildMetrics(0, 0)})
	if err := e.registry.Register(e); err != nil {
		return nil, err
	}
	e.registry.MustRegister(collectors.NewGoCollector(), collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))
	return e, nil
}

func (e *Exporter) Handler() http.Handler {
	return promhttp.HandlerFor(e.registry, promhttp.HandlerOpts{MaxRequestsInFlight: 4, EnableOpenMetrics: true})
}

func (e *Exporter) Describe(ch chan<- *prometheus.Desc) {
	for _, d := range e.desc {
		ch <- d
	}
}

func (e *Exporter) Collect(ch chan<- prometheus.Metric) {
	if snapshot := e.current.Load(); snapshot != nil {
		for _, sample := range snapshot.metrics {
			ch <- sample
		}
	}
}

// Run refreshes after each interval; call Refresh once before serving to prime
// the initial baseline. Cancellation stops both SQL and HTTP collection.
func (e *Exporter) Run(ctx context.Context) {
	ticker := time.NewTicker(time.Duration(e.cfg.RefreshIntervalSec) * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			_ = e.Refresh(ctx)
		}
	}
}

func (e *Exporter) Refresh(parent context.Context) error {
	e.refreshMu.Lock()
	defer e.refreshMu.Unlock()
	ctx, cancel := context.WithTimeout(parent, time.Duration(e.cfg.CollectionTimeoutSec)*time.Second)
	defer cancel()
	started := e.now()
	core, dbErr := e.repo.LoadSnapshot(ctx)
	success := 0.0
	if dbErr == nil {
		e.core, e.lastSuccess, success = core, float64(e.now().UnixNano())/1e9, 1
	}
	apps := map[string]metrics.Application{}
	for _, a := range e.core.Applications {
		apps[a.ID] = a
	}
	var errs []error
	if dbErr != nil {
		errs = append(errs, fmt.Errorf("collect GOS metrics: %w", dbErr))
	}
	type result struct {
		id     string
		values []float64
		err    error
	}
	results := make(chan result, len(e.cfg.BusinessSources))
	sem := make(chan struct{}, 4)
	var wg sync.WaitGroup
	for _, source := range e.cfg.BusinessSources {
		wg.Add(1)
		go func(source metrics.BusinessSource) {
			defer wg.Done()
			if _, exists := apps[source.ApplicationID]; !exists {
				results <- result{id: source.ID, err: errors.New("application not found")}
				return
			}
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				results <- result{id: source.ID, err: ctx.Err()}
				return
			}
			values, err := e.collectBusinessSource(ctx, source)
			results <- result{id: source.ID, values: values, err: err}
		}(source)
	}
	wg.Wait()
	close(results)
	for result := range results {
		state := e.sources[result.id]
		state.up = 0
		if result.err == nil {
			state.values, state.up, state.lastSuccess = result.values, 1, float64(e.now().UnixNano())/1e9
		} else {
			errs = append(errs, fmt.Errorf("business source %q: %w", result.id, result.err))
		}
		e.sources[result.id] = state
	}
	e.current.Store(&metricSet{metrics: e.buildMetrics(success, e.now().Sub(started).Seconds())})
	return errors.Join(errs...)
}

func (e *Exporter) initDescriptors() {
	base := []string{"project_id", "application_id", "env"}
	add := func(name, help string, labels ...string) { e.desc[name] = prometheus.NewDesc(name, help, labels, nil) }
	app := func(name, help string, labels ...string) {
		add(name, help, append(append([]string{}, base...), labels...)...)
	}
	add("gos_project_info", "Project identity and display metadata.", "project_id", "project_key", "project_name")
	add("gos_application_info", "Application identity and current project metadata.", "project_id", "application_id", "application_key", "application_name")
	app("gos_application_env_info", "Known application and environment bindings.")
	app("gos_release_orders", "Current retained release orders by projected business status.", "status")
	app("gos_release_latest_status", "One-hot projected status of the latest created release order.", "status")
	app("gos_release_orders_created_total", "Persisted release order creation events, including drafts.", "trigger_type", "operation")
	app("gos_release_executions_total", "Persisted terminal CI/CD execution events, counted once per execution.", "scope", "provider", "result")
	app("gos_release_execution_duration_seconds", "Execution wall time including external queue and tracking delay, for completed started success/failed executions.", "scope", "provider")
	app("gos_release_last_success_timestamp_seconds", "Unix timestamp of the last successful CD execution; zero if none.")
	app("gos_release_approval_pending", "Pending human approval tasks, with legacy order approvals counted once.", "gate")
	add("gos_agents", "Current Agent count by environment and runtime state.", "env", "status")
	add("gos_agent_info", "Agent identity and display metadata.", "agent_id", "agent_code", "agent_name", "env")
	add("gos_agent_state", "One-hot current Agent runtime state.", "agent_id", "env", "status")
	add("gos_agent_last_heartbeat_timestamp_seconds", "Agent heartbeat Unix timestamp; zero if never received.", "agent_id", "env")
	add("gos_metrics_collection_success", "Whether the last GOS database collection succeeded.")
	add("gos_metrics_last_success_timestamp_seconds", "Unix timestamp of the last successful GOS database collection.")
	add("gos_metrics_collection_duration_seconds", "Wall time of the last complete metrics refresh.")
	app("gos_business_source_up", "Whether the last business source collection succeeded.", "source_id")
	app("gos_business_source_last_success_timestamp_seconds", "Unix timestamp of the last successful business source collection.", "source_id")
	for _, source := range e.cfg.BusinessSources {
		for _, m := range source.Metrics {
			if _, exists := e.desc[m.Name]; exists {
				continue
			}
			var keys []string
			for key := range m.Labels {
				keys = append(keys, key)
			}
			sort.Strings(keys)
			e.businessKeys[m.Name] = keys
			app(m.Name, m.Help, append([]string{"source_id"}, keys...)...)
		}
	}
}

func (e *Exporter) buildMetrics(success, duration float64) []prometheus.Metric {
	var samples []prometheus.Metric
	emit := func(name string, typ prometheus.ValueType, value float64, labels ...string) {
		samples = append(samples, prometheus.MustNewConstMetric(e.desc[name], typ, value, labels...))
	}
	gauge := func(name string, value float64, labels ...string) {
		emit(name, prometheus.GaugeValue, value, labels...)
	}
	apps := map[string]metrics.Application{}
	for _, a := range e.core.Applications {
		apps[a.ID] = a
		gauge("gos_application_info", 1, a.ProjectID, a.ID, a.Key, a.Name)
	}
	for _, p := range e.core.Projects {
		gauge("gos_project_info", 1, p.ID, p.Key, p.Name)
	}
	base := func(s metrics.Scope) []string {
		return []string{apps[s.ApplicationID].ProjectID, s.ApplicationID, s.Env}
	}
	with := func(s metrics.Scope, more ...string) []string { return append(base(s), more...) }
	scopes := map[metrics.Scope]bool{}
	for _, s := range e.core.Scopes {
		scopes[s] = true
	}
	for _, s := range e.cfg.BusinessSources {
		if _, exists := apps[s.ApplicationID]; exists {
			scopes[metrics.Scope{ApplicationID: s.ApplicationID, Env: s.Env}] = true
		}
	}
	type stateKey struct {
		scope metrics.Scope
		state string
	}
	orders, latest, approvals := map[stateKey]float64{}, map[stateKey]float64{}, map[stateKey]float64{}
	lastSuccess := map[metrics.Scope]float64{}
	for _, v := range e.core.Orders {
		orders[stateKey{v.Scope, v.Status}] += v.Count
	}
	for _, v := range e.core.LatestOrders {
		latest[stateKey{v.Scope, v.Status}] = 1
	}
	for _, v := range e.core.Approvals {
		approvals[stateKey{v.Scope, v.Gate}] += v.Count
	}
	for _, v := range e.core.LastSuccess {
		lastSuccess[v.Scope] = v.Seconds
	}
	for scope := range scopes {
		gauge("gos_application_env_info", 1, base(scope)...)
		for _, status := range orderStates {
			gauge("gos_release_orders", orders[stateKey{scope, status}], with(scope, status)...)
			gauge("gos_release_latest_status", latest[stateKey{scope, status}], with(scope, status)...)
		}
		for _, gate := range []string{"before_execute", "before_ci", "before_cd"} {
			gauge("gos_release_approval_pending", approvals[stateKey{scope, gate}], with(scope, gate)...)
		}
		gauge("gos_release_last_success_timestamp_seconds", lastSuccess[scope], base(scope)...)
	}
	// Zero known combinations before overlaying observed values. Counters include
	// historical events on first scrape, forming a baseline rather than a burst.
	type createdKey struct {
		scope              metrics.Scope
		trigger, operation string
	}
	created := map[createdKey]float64{}
	for scope := range scopes {
		for _, trigger := range []string{"manual", "webhook", "schedule", "automation"} {
			for _, operation := range []string{"deploy", "rollback", "replay"} {
				created[createdKey{scope, trigger, operation}] = 0
			}
		}
	}
	for _, v := range e.core.Created {
		trigger, operation := v.Trigger, v.Operation
		if trigger == "scheduled" {
			trigger = "schedule"
		}
		if operation == "" {
			operation = "deploy"
		}
		created[createdKey{v.Scope, trigger, operation}] += v.Count
	}
	for key, count := range created {
		emit("gos_release_orders_created_total", prometheus.CounterValue, count, with(key.scope, key.trigger, key.operation)...)
	}
	type executionKey struct {
		scope  metrics.ExecutionScope
		result string
	}
	executions := map[executionKey]float64{}
	durations := map[metrics.ExecutionScope]metrics.Duration{}
	for _, scope := range e.core.ExecutionScopes {
		for _, result := range executionResults {
			executions[executionKey{scope, result}] = 0
		}
		durations[scope] = metrics.Duration{ExecutionScope: scope, Buckets: map[float64]uint64{}}
	}
	for _, v := range e.core.Executions {
		executions[executionKey{v.ExecutionScope, v.Result}] += v.Count
	}
	for key, count := range executions {
		emit("gos_release_executions_total", prometheus.CounterValue, count, with(key.scope.Scope, key.scope.PipelineScope, key.scope.Provider, key.result)...)
	}
	for _, v := range e.core.Durations {
		durations[v.ExecutionScope] = v
	}
	for scope, v := range durations {
		buckets := map[float64]uint64{}
		for _, upper := range metrics.DurationBuckets {
			buckets[upper] = v.Buckets[upper]
		}
		samples = append(samples, prometheus.MustNewConstHistogram(e.desc["gos_release_execution_duration_seconds"], v.Count, v.Sum, buckets, with(scope.Scope, scope.PipelineScope, scope.Provider)...))
	}
	type agentKey struct{ env, state string }
	agentCounts, agentEnvs := map[agentKey]float64{}, map[string]bool{}
	for _, a := range e.core.Agents {
		agentEnvs[a.Env] = true
		agentCounts[agentKey{a.Env, a.Status}]++
		gauge("gos_agent_info", 1, a.ID, a.Code, a.Name, a.Env)
		gauge("gos_agent_last_heartbeat_timestamp_seconds", a.LastHeartbeat, a.ID, a.Env)
		for _, status := range agentStates {
			v := 0.0
			if a.Status == status {
				v = 1
			}
			gauge("gos_agent_state", v, a.ID, a.Env, status)
		}
	}
	for scope := range scopes {
		agentEnvs[scope.Env] = true
	}
	for env := range agentEnvs {
		for _, status := range agentStates {
			gauge("gos_agents", agentCounts[agentKey{env, status}], env, status)
		}
	}
	for _, source := range e.cfg.BusinessSources {
		state := e.sources[source.ID]
		scope := metrics.Scope{ApplicationID: source.ApplicationID, Env: source.Env}
		gauge("gos_business_source_up", state.up, with(scope, source.ID)...)
		gauge("gos_business_source_last_success_timestamp_seconds", state.lastSuccess, with(scope, source.ID)...)
		if _, exists := apps[source.ApplicationID]; !exists {
			continue
		}
		for i, m := range source.Metrics {
			if i >= len(state.values) {
				continue
			}
			labels := with(scope, source.ID)
			for _, key := range e.businessKeys[m.Name] {
				labels = append(labels, m.Labels[key])
			}
			typ := prometheus.GaugeValue
			if m.Type == "counter" {
				typ = prometheus.CounterValue
			}
			emit(m.Name, typ, state.values[i], labels...)
		}
	}
	gauge("gos_metrics_collection_success", success)
	gauge("gos_metrics_last_success_timestamp_seconds", e.lastSuccess)
	gauge("gos_metrics_collection_duration_seconds", duration)
	return samples
}
