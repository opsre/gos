package usecase

import (
	"context"
	"fmt"
	"strings"

	domain "gos/internal/domain/pipeline"
)

type JenkinsPipelineEditor interface {
	CreateRawPipeline(ctx context.Context, fullName string, cfg domain.JenkinsRawPipelineConfig) error
	UpdateRawPipeline(ctx context.Context, fullName string, cfg domain.JenkinsRawPipelineConfig) error
	DeletePipeline(ctx context.Context, fullName string) error
	GetPipelineScript(ctx context.Context, fullName string) (domain.JenkinsPipelineScript, error)
	RenderRawPipelineConfigXML(cfg domain.JenkinsRawPipelineConfig) (string, error)
}

type JenkinsPipelineManager struct {
	repo       domain.Repository
	editor     JenkinsPipelineEditor
	pipelineUC *SyncPipelines
	paramUC    *SyncExecutorParamDefs
}

type CreateJenkinsRawPipelineInput struct {
	FullName    string
	Description string
	Script      string
	Sandbox     bool
}

type UpdateJenkinsRawPipelineInput struct {
	Description string
	Script      string
	Sandbox     bool
}

type PreviewJenkinsRawPipelineConfigInput struct {
	FullName    string
	Description string
	Script      string
	Sandbox     bool
}

// NewJenkinsPipelineManager 创建并返回对应组件实例。
func NewJenkinsPipelineManager(
	repo domain.Repository,
	editor JenkinsPipelineEditor,
	pipelineUC *SyncPipelines,
	paramUC *SyncExecutorParamDefs,
) *JenkinsPipelineManager {
	return &JenkinsPipelineManager{
		repo:       repo,
		editor:     editor,
		pipelineUC: pipelineUC,
		paramUC:    paramUC,
	}
}

// CreateRaw 创建业务资源并返回处理结果。
func (uc *JenkinsPipelineManager) CreateRaw(
	ctx context.Context,
	input CreateJenkinsRawPipelineInput,
) (domain.Pipeline, error) {
	fullName, cfg, err := normalizeRawPipelineInput(input.FullName, input.Description, input.Script, input.Sandbox)
	if err != nil {
		return domain.Pipeline{}, err
	}

	if err := uc.editor.CreateRawPipeline(ctx, fullName, cfg); err != nil {
		return domain.Pipeline{}, fmt.Errorf("%w: create raw jenkins pipeline failed: %v", ErrInvalidInput, err)
	}
	if err := uc.syncAfterJenkinsPipelineMutation(ctx); err != nil {
		return domain.Pipeline{}, err
	}
	return uc.repo.GetPipelineByID(ctx, pipelineID(string(domain.ProviderJenkins), fullName))
}

// UpdateRaw 更新业务资源并返回处理结果。
func (uc *JenkinsPipelineManager) UpdateRaw(
	ctx context.Context,
	id string,
	input UpdateJenkinsRawPipelineInput,
) (domain.Pipeline, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return domain.Pipeline{}, ErrInvalidID
	}

	item, err := uc.repo.GetPipelineByID(ctx, id)
	if err != nil {
		return domain.Pipeline{}, err
	}
	if item.Provider != domain.ProviderJenkins {
		return domain.Pipeline{}, fmt.Errorf("%w: only jenkins pipeline is supported", ErrInvalidProvider)
	}
	if err := ensureActivePipelineRecord(item, "当前管线"); err != nil {
		return domain.Pipeline{}, err
	}
	if strings.TrimSpace(item.JobFullName) == "" {
		return domain.Pipeline{}, fmt.Errorf("%w: jenkins job full name is empty", ErrInvalidInput)
	}

	currentScript, err := uc.editor.GetPipelineScript(ctx, item.JobFullName)
	if err != nil {
		return domain.Pipeline{}, err
	}
	if currentScript.FromSCM {
		return domain.Pipeline{}, fmt.Errorf("%w: scm pipeline does not support inline raw editing", ErrInvalidInput)
	}

	_, cfg, err := normalizeRawPipelineInput(item.JobFullName, input.Description, input.Script, input.Sandbox)
	if err != nil {
		return domain.Pipeline{}, err
	}

	if err := uc.editor.UpdateRawPipeline(ctx, item.JobFullName, cfg); err != nil {
		return domain.Pipeline{}, fmt.Errorf("%w: update raw jenkins pipeline failed: %v", ErrInvalidInput, err)
	}
	if err := uc.syncAfterJenkinsPipelineMutation(ctx); err != nil {
		return domain.Pipeline{}, err
	}
	return uc.repo.GetPipelineByID(ctx, id)
}

// DeleteRaw 删除业务资源并返回处理结果。
func (uc *JenkinsPipelineManager) DeleteRaw(ctx context.Context, id string) (domain.Pipeline, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return domain.Pipeline{}, ErrInvalidID
	}

	item, err := uc.repo.GetPipelineByID(ctx, id)
	if err != nil {
		return domain.Pipeline{}, err
	}
	if item.Provider != domain.ProviderJenkins {
		return domain.Pipeline{}, fmt.Errorf("%w: only jenkins pipeline is supported", ErrInvalidProvider)
	}
	if err := ensureActivePipelineRecord(item, "当前管线"); err != nil {
		return domain.Pipeline{}, err
	}
	if strings.TrimSpace(item.JobFullName) == "" {
		return domain.Pipeline{}, fmt.Errorf("%w: jenkins job full name is empty", ErrInvalidInput)
	}

	currentScript, err := uc.editor.GetPipelineScript(ctx, item.JobFullName)
	if err != nil {
		return domain.Pipeline{}, err
	}
	if currentScript.FromSCM {
		return domain.Pipeline{}, fmt.Errorf("%w: scm pipeline does not support inline raw deletion", ErrInvalidInput)
	}

	if err := uc.editor.DeletePipeline(ctx, item.JobFullName); err != nil {
		return domain.Pipeline{}, fmt.Errorf("%w: delete raw jenkins pipeline failed: %v", ErrInvalidInput, err)
	}
	if err := uc.syncAfterJenkinsPipelineMutation(ctx); err != nil {
		return domain.Pipeline{}, err
	}
	return uc.repo.GetPipelineByID(ctx, id)
}

// PreviewRawConfigXML 封装当前模块的业务处理逻辑。
func (uc *JenkinsPipelineManager) PreviewRawConfigXML(
	_ context.Context,
	input PreviewJenkinsRawPipelineConfigInput,
) (string, error) {
	_, cfg, err := normalizeRawPipelineInput(input.FullName, input.Description, input.Script, input.Sandbox)
	if err != nil {
		return "", err
	}
	configXML, err := uc.editor.RenderRawPipelineConfigXML(cfg)
	if err != nil {
		return "", fmt.Errorf("%w: render raw pipeline config.xml failed: %v", ErrInvalidInput, err)
	}
	return configXML, nil
}

// syncAfterJenkinsPipelineMutation 同步外部或内部状态数据。
func (uc *JenkinsPipelineManager) syncAfterJenkinsPipelineMutation(ctx context.Context) error {
	if uc.pipelineUC != nil {
		if _, err := uc.pipelineUC.Execute(ctx); err != nil {
			return err
		}
	}
	if uc.paramUC != nil {
		if _, err := uc.paramUC.Execute(ctx); err != nil {
			return err
		}
	}
	return nil
}

// normalizeRawPipelineInput 标准化输入值，保证后续逻辑使用统一格式。
func normalizeRawPipelineInput(fullName string, description string, script string, sandbox bool) (string, domain.JenkinsRawPipelineConfig, error) {
	normalizedFullName := strings.Trim(strings.TrimSpace(fullName), "/")
	if normalizedFullName == "" {
		return "", domain.JenkinsRawPipelineConfig{}, fmt.Errorf("%w: jenkins path is required", ErrInvalidInput)
	}

	parts := strings.Split(normalizedFullName, "/")
	for _, part := range parts {
		if strings.TrimSpace(part) == "" {
			return "", domain.JenkinsRawPipelineConfig{}, fmt.Errorf("%w: jenkins path is invalid", ErrInvalidInput)
		}
	}

	normalizedScript := strings.ReplaceAll(script, "\r\n", "\n")
	normalizedScript = strings.ReplaceAll(normalizedScript, "\r", "\n")
	if strings.TrimSpace(normalizedScript) == "" {
		return "", domain.JenkinsRawPipelineConfig{}, fmt.Errorf("%w: raw pipeline script is required", ErrInvalidInput)
	}

	return normalizedFullName, domain.JenkinsRawPipelineConfig{
		Description: strings.TrimSpace(description),
		Script:      normalizedScript,
		Sandbox:     sandbox,
	}, nil
}
