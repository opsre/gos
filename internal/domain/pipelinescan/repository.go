package pipelinescan

import "context"

type Repository interface {
	InitSchema(ctx context.Context) error
	CreateRule(ctx context.Context, item Rule) error
	ListRules(ctx context.Context, filter RuleListFilter) ([]Rule, int64, error)
	ListEnabledRules(ctx context.Context) ([]Rule, error)
	GetRuleByID(ctx context.Context, id string) (Rule, error)
	UpdateRule(ctx context.Context, id string, input RuleUpdateInput) (Rule, error)
	DeleteRule(ctx context.Context, id string) error
	SaveScan(ctx context.Context, result Result, findings []Finding) error
	GetResultByPipelineID(ctx context.Context, pipelineID string) (Result, []Finding, error)
	ListResults(ctx context.Context, filter ResultListFilter) ([]Result, int64, error)
}
