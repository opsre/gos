package observability

import "context"

// DurationBuckets are seconds, shared by SQL aggregation and Prometheus exposition.
var DurationBuckets = [...]float64{1, 5, 10, 15, 30, 45, 60, 90, 120, 180, 240, 300, 450, 600, 900, 1800, 3600, 7200}

type Scope struct {
	ApplicationID string
	Env           string
}

type Application struct {
	ID, Key, Name, ProjectID string
}

type Project struct {
	ID, Key, Name string
}

type OrderCount struct {
	Scope
	Status string
	Count  float64
}

type CreatedCount struct {
	Scope
	Trigger, Operation string
	Count              float64
}

type ExecutionScope struct {
	Scope
	PipelineScope, Provider string
}

type ExecutionCount struct {
	ExecutionScope
	Result string
	Count  float64
}

type Duration struct {
	ExecutionScope
	Count   uint64
	Sum     float64
	Buckets map[float64]uint64
}

type Timestamp struct {
	Scope
	Seconds float64
}

type ApprovalCount struct {
	Scope
	Gate  string
	Count float64
}

type Agent struct {
	ID, Code, Name, Env, Status string
	LastHeartbeat               float64
}

type Snapshot struct {
	Applications    []Application
	Projects        []Project
	Scopes          []Scope
	Orders          []OrderCount
	LatestOrders    []OrderCount
	Created         []CreatedCount
	Executions      []ExecutionCount
	ExecutionScopes []ExecutionScope
	Durations       []Duration
	LastSuccess     []Timestamp
	Approvals       []ApprovalCount
	Agents          []Agent
}

type Repository interface {
	LoadSnapshot(context.Context) (Snapshot, error)
}
