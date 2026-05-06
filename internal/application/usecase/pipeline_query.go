package usecase

import (
	"context"
	"strings"
	"time"

	domain "gos/internal/domain/pipeline"
)

type QueryPipeline struct {
	repo    domain.Repository
	jenkins JenkinsPipelineClient
	now     func() time.Time
}

type VerifyPipelineOutput struct {
	Verified bool            `json:"verified"`
	JobName  string          `json:"job_name"`
	JobURL   string          `json:"job_url"`
	Pipeline domain.Pipeline `json:"pipeline"`
}

type PipelineRawScriptOutput struct {
	Pipeline        domain.Pipeline `json:"pipeline"`
	DefinitionClass string          `json:"definition_class"`
	Description     string          `json:"description"`
	Script          string          `json:"script"`
	ScriptPath      string          `json:"script_path"`
	Sandbox         bool            `json:"sandbox"`
	FromSCM         bool            `json:"from_scm"`
}

type PipelineConfigXMLOutput struct {
	Pipeline  domain.Pipeline `json:"pipeline"`
	ConfigXML string          `json:"config_xml"`
}

type PipelineOriginalLinkOutput struct {
	Pipeline     domain.Pipeline `json:"pipeline"`
	OriginalLink string          `json:"original_link"`
}

// NewQueryPipeline 创建并返回对应组件实例。
func NewQueryPipeline(repo domain.Repository, jenkins JenkinsPipelineClient) *QueryPipeline {
	return &QueryPipeline{
		repo:    repo,
		jenkins: jenkins,
		now: func() time.Time {
			return time.Now().UTC()
		},
	}
}

// List 查询并返回列表数据。
func (uc *QueryPipeline) List(ctx context.Context, filter domain.PipelineListFilter) ([]domain.Pipeline, int64, error) {
	const (
		defaultPage     = 1
		defaultPageSize = 20
		maxPageSize     = 100
	)

	filter.Name = strings.TrimSpace(filter.Name)
	if filter.Provider == "" {
		filter.Provider = domain.ProviderJenkins
	}
	if !filter.Provider.Valid() {
		return nil, 0, ErrInvalidProvider
	}
	if filter.Status != "" && !filter.Status.Valid() {
		return nil, 0, ErrInvalidStatus
	}
	if filter.Page <= 0 {
		filter.Page = defaultPage
	}
	if filter.PageSize <= 0 {
		filter.PageSize = defaultPageSize
	}
	if filter.PageSize > maxPageSize {
		filter.PageSize = maxPageSize
	}
	return uc.repo.ListPipelines(ctx, filter)
}

// GetByID 查询并返回指定资源数据。
func (uc *QueryPipeline) GetByID(ctx context.Context, id string) (domain.Pipeline, error) {
	if strings.TrimSpace(id) == "" {
		return domain.Pipeline{}, ErrInvalidID
	}
	return uc.repo.GetPipelineByID(ctx, id)
}

// Verify 封装当前模块的业务处理逻辑。
func (uc *QueryPipeline) Verify(ctx context.Context, id string) (VerifyPipelineOutput, error) {
	if strings.TrimSpace(id) == "" {
		return VerifyPipelineOutput{}, ErrInvalidID
	}

	p, err := uc.repo.GetPipelineByID(ctx, id)
	if err != nil {
		return VerifyPipelineOutput{}, err
	}

	job, err := uc.jenkins.GetJob(ctx, p.JobFullName)
	if err != nil {
		return VerifyPipelineOutput{}, err
	}

	updated, err := uc.repo.MarkPipelineVerified(ctx, id, uc.now(), uc.now())
	if err != nil {
		return VerifyPipelineOutput{}, err
	}

	return VerifyPipelineOutput{
		Verified: true,
		JobName:  job.Name,
		JobURL:   job.URL,
		Pipeline: updated,
	}, nil
}

// GetRawScript 查询并返回指定资源数据。
func (uc *QueryPipeline) GetRawScript(ctx context.Context, id string) (PipelineRawScriptOutput, error) {
	if strings.TrimSpace(id) == "" {
		return PipelineRawScriptOutput{}, ErrInvalidID
	}

	p, err := uc.repo.GetPipelineByID(ctx, id)
	if err != nil {
		return PipelineRawScriptOutput{}, err
	}
	if p.Provider != domain.ProviderJenkins {
		return PipelineRawScriptOutput{}, ErrInvalidProvider
	}
	if strings.TrimSpace(p.JobFullName) == "" {
		return PipelineRawScriptOutput{}, ErrInvalidInput
	}

	script, err := uc.jenkins.GetPipelineScript(ctx, p.JobFullName)
	if err != nil {
		return PipelineRawScriptOutput{}, err
	}

	return PipelineRawScriptOutput{
		Pipeline:        p,
		DefinitionClass: script.DefinitionClass,
		Description:     script.Description,
		Script:          script.Script,
		ScriptPath:      script.ScriptPath,
		Sandbox:         script.Sandbox,
		FromSCM:         script.FromSCM,
	}, nil
}

// GetConfigXML 查询并返回指定资源数据。
func (uc *QueryPipeline) GetConfigXML(ctx context.Context, id string) (PipelineConfigXMLOutput, error) {
	if strings.TrimSpace(id) == "" {
		return PipelineConfigXMLOutput{}, ErrInvalidID
	}

	p, err := uc.repo.GetPipelineByID(ctx, id)
	if err != nil {
		return PipelineConfigXMLOutput{}, err
	}
	if p.Provider != domain.ProviderJenkins {
		return PipelineConfigXMLOutput{}, ErrInvalidProvider
	}
	if strings.TrimSpace(p.JobFullName) == "" {
		return PipelineConfigXMLOutput{}, ErrInvalidInput
	}

	configXML, err := uc.jenkins.GetPipelineConfigXML(ctx, p.JobFullName)
	if err != nil {
		return PipelineConfigXMLOutput{}, err
	}

	return PipelineConfigXMLOutput{
		Pipeline:  p,
		ConfigXML: configXML,
	}, nil
}

// GetOriginalLink 查询并返回指定资源数据。
func (uc *QueryPipeline) GetOriginalLink(ctx context.Context, id string) (PipelineOriginalLinkOutput, error) {
	if strings.TrimSpace(id) == "" {
		return PipelineOriginalLinkOutput{}, ErrInvalidID
	}

	p, err := uc.repo.GetPipelineByID(ctx, id)
	if err != nil {
		return PipelineOriginalLinkOutput{}, err
	}
	if p.Provider != domain.ProviderJenkins {
		return PipelineOriginalLinkOutput{}, ErrInvalidProvider
	}
	if strings.TrimSpace(p.JobFullName) == "" {
		return PipelineOriginalLinkOutput{}, ErrInvalidInput
	}

	return PipelineOriginalLinkOutput{
		Pipeline:     p,
		OriginalLink: uc.jenkins.BuildJobURL(p.JobFullName),
	}, nil
}
