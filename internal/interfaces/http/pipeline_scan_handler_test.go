package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"gos/internal/application/usecase"
	pipelinedomain "gos/internal/domain/pipeline"
	scandomain "gos/internal/domain/pipelinescan"
	userdomain "gos/internal/domain/user"
)

func TestPipelineScanHandlerScanAllPipelines(t *testing.T) {
	gin.SetMode(gin.TestMode)

	pipeline := pipelinedomain.Pipeline{
		ID:          "pln-1",
		Provider:    pipelinedomain.ProviderJenkins,
		JobFullName: "folder/demo",
		JobName:     "demo",
		Status:      pipelinedomain.StatusActive,
	}
	pipelineRepo := &pipelineScanHandlerPipelineRepoFake{
		pipelines: map[string]pipelinedomain.Pipeline{
			pipeline.ID: pipeline,
		},
	}
	scanRepo := &pipelineScanHandlerScanRepoFake{}
	jenkins := &pipelineScanHandlerJenkinsFake{
		scripts: map[string]pipelinedomain.JenkinsPipelineScript{
			pipeline.JobFullName: {Script: "pipeline { stages {} }"},
		},
	}
	manager := usecase.NewPipelineScanManager(scanRepo, pipelineRepo, jenkins)

	router := gin.New()
	router.Use(func(c *gin.Context) {
		setCurrentUser(c, userdomain.User{ID: "usr-1"})
		c.Next()
	})
	NewPipelineScanHandler(manager, pipelineScanHandlerAllowAllAuthorizer{}).RegisterRoutes(router)

	req := httptest.NewRequest(http.MethodPost, "/pipeline-scan/scan", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var resp PipelineScanBatchResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Data.Total != 1 || resp.Data.Scanned != 1 || resp.Data.Failed != 0 {
		t.Fatalf("scan output = %#v, want total=1 scanned=1 failed=0", resp.Data)
	}
	if scanRepo.result.PipelineID != pipeline.ID {
		t.Fatalf("saved scan pipeline ID = %q, want %q", scanRepo.result.PipelineID, pipeline.ID)
	}
}

type pipelineScanHandlerAllowAllAuthorizer struct{}

func (pipelineScanHandlerAllowAllAuthorizer) HasPermission(context.Context, userdomain.User, string, string, string) (bool, error) {
	return true, nil
}

func (pipelineScanHandlerAllowAllAuthorizer) ListEffectivePermissions(context.Context, userdomain.User) ([]userdomain.UserPermission, error) {
	return nil, nil
}

type pipelineScanHandlerScanRepoFake struct {
	rules    []scandomain.Rule
	result   scandomain.Result
	findings []scandomain.Finding
}

func (r *pipelineScanHandlerScanRepoFake) InitSchema(context.Context) error { return nil }

func (r *pipelineScanHandlerScanRepoFake) CreateRule(_ context.Context, item scandomain.Rule) error {
	r.rules = append(r.rules, item)
	return nil
}

func (r *pipelineScanHandlerScanRepoFake) ListRules(context.Context, scandomain.RuleListFilter) ([]scandomain.Rule, int64, error) {
	return append([]scandomain.Rule(nil), r.rules...), int64(len(r.rules)), nil
}

func (r *pipelineScanHandlerScanRepoFake) ListEnabledRules(context.Context) ([]scandomain.Rule, error) {
	items := make([]scandomain.Rule, 0, len(r.rules))
	for _, rule := range r.rules {
		if rule.Enabled {
			items = append(items, rule)
		}
	}
	return items, nil
}

func (r *pipelineScanHandlerScanRepoFake) GetRuleByID(_ context.Context, id string) (scandomain.Rule, error) {
	for _, rule := range r.rules {
		if rule.ID == id {
			return rule, nil
		}
	}
	return scandomain.Rule{}, scandomain.ErrRuleNotFound
}

func (r *pipelineScanHandlerScanRepoFake) UpdateRule(_ context.Context, id string, input scandomain.RuleUpdateInput) (scandomain.Rule, error) {
	for i, rule := range r.rules {
		if rule.ID == id {
			r.rules[i].RuleCode = input.RuleCode
			r.rules[i].RuleName = input.RuleName
			r.rules[i].Category = input.Category
			r.rules[i].Severity = input.Severity
			r.rules[i].Enabled = input.Enabled
			r.rules[i].ScopeJSON = input.ScopeJSON
			r.rules[i].RuleDSL = input.RuleDSL
			r.rules[i].Message = input.Message
			r.rules[i].Suggestion = input.Suggestion
			return r.rules[i], nil
		}
	}
	return scandomain.Rule{}, scandomain.ErrRuleNotFound
}

func (r *pipelineScanHandlerScanRepoFake) DeleteRule(_ context.Context, id string) error {
	for i, rule := range r.rules {
		if rule.ID == id {
			r.rules = append(r.rules[:i], r.rules[i+1:]...)
			return nil
		}
	}
	return scandomain.ErrRuleNotFound
}

func (r *pipelineScanHandlerScanRepoFake) SaveScan(_ context.Context, result scandomain.Result, findings []scandomain.Finding) error {
	r.result = result
	r.findings = append([]scandomain.Finding(nil), findings...)
	return nil
}

func (r *pipelineScanHandlerScanRepoFake) GetResultByPipelineID(_ context.Context, pipelineID string) (scandomain.Result, []scandomain.Finding, error) {
	if r.result.PipelineID != pipelineID {
		return scandomain.Result{}, nil, scandomain.ErrResultNotFound
	}
	return r.result, append([]scandomain.Finding(nil), r.findings...), nil
}

func (r *pipelineScanHandlerScanRepoFake) ListResults(context.Context, scandomain.ResultListFilter) ([]scandomain.Result, int64, error) {
	if r.result.ID == "" {
		return nil, 0, nil
	}
	return []scandomain.Result{r.result}, 1, nil
}

type pipelineScanHandlerPipelineRepoFake struct {
	pipelines map[string]pipelinedomain.Pipeline
}

func (r *pipelineScanHandlerPipelineRepoFake) InitSchema(context.Context) error { return nil }

func (r *pipelineScanHandlerPipelineRepoFake) UpsertPipelines(context.Context, []pipelinedomain.Pipeline) (int, int, error) {
	return 0, 0, nil
}

func (r *pipelineScanHandlerPipelineRepoFake) MarkMissingPipelinesInactive(context.Context, pipelinedomain.Provider, []string, time.Time) (int, error) {
	return 0, nil
}

func (r *pipelineScanHandlerPipelineRepoFake) ListPipelines(_ context.Context, filter pipelinedomain.PipelineListFilter) ([]pipelinedomain.Pipeline, int64, error) {
	items := make([]pipelinedomain.Pipeline, 0, len(r.pipelines))
	for _, item := range r.pipelines {
		if filter.Provider != "" && item.Provider != filter.Provider {
			continue
		}
		if filter.Status != "" && item.Status != filter.Status {
			continue
		}
		items = append(items, item)
	}
	return items, int64(len(items)), nil
}

func (r *pipelineScanHandlerPipelineRepoFake) GetPipelineByID(_ context.Context, id string) (pipelinedomain.Pipeline, error) {
	item, ok := r.pipelines[id]
	if !ok {
		return pipelinedomain.Pipeline{}, pipelinedomain.ErrPipelineNotFound
	}
	return item, nil
}

func (r *pipelineScanHandlerPipelineRepoFake) MarkPipelineVerified(context.Context, string, time.Time, time.Time) (pipelinedomain.Pipeline, error) {
	return pipelinedomain.Pipeline{}, nil
}

func (r *pipelineScanHandlerPipelineRepoFake) CreateBinding(context.Context, pipelinedomain.PipelineBinding) error {
	return nil
}

func (r *pipelineScanHandlerPipelineRepoFake) ListBindingsByApplication(context.Context, pipelinedomain.BindingListFilter) ([]pipelinedomain.PipelineBinding, int64, error) {
	return nil, 0, nil
}

func (r *pipelineScanHandlerPipelineRepoFake) GetBindingByID(context.Context, string) (pipelinedomain.PipelineBinding, error) {
	return pipelinedomain.PipelineBinding{}, pipelinedomain.ErrBindingNotFound
}

func (r *pipelineScanHandlerPipelineRepoFake) UpdateBinding(context.Context, string, pipelinedomain.BindingUpdateInput, time.Time) (pipelinedomain.PipelineBinding, error) {
	return pipelinedomain.PipelineBinding{}, nil
}

func (r *pipelineScanHandlerPipelineRepoFake) DeleteBinding(context.Context, string) error {
	return nil
}

type pipelineScanHandlerJenkinsFake struct {
	scripts map[string]pipelinedomain.JenkinsPipelineScript
}

func (j *pipelineScanHandlerJenkinsFake) ListJobs(context.Context) ([]pipelinedomain.JenkinsJob, error) {
	return nil, nil
}

func (j *pipelineScanHandlerJenkinsFake) GetJob(context.Context, string) (pipelinedomain.JenkinsJob, error) {
	return pipelinedomain.JenkinsJob{}, nil
}

func (j *pipelineScanHandlerJenkinsFake) GetPipelineScript(_ context.Context, fullName string) (pipelinedomain.JenkinsPipelineScript, error) {
	return j.scripts[fullName], nil
}

func (j *pipelineScanHandlerJenkinsFake) GetPipelineConfigXML(context.Context, string) (string, error) {
	return "", nil
}

func (j *pipelineScanHandlerJenkinsFake) BuildJobURL(fullName string) string {
	return fullName
}
