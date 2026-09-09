package usecase

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	app "gos/internal/domain/application"
	ep "gos/internal/domain/executorparam"
	ob "gos/internal/domain/onboarding"
	pipe "gos/internal/domain/pipeline"
	pp "gos/internal/domain/platformparam"
	project "gos/internal/domain/project"
	rel "gos/internal/domain/release"
	usr "gos/internal/domain/user"
)

type OnboardingUsers interface {
	GetUserByID(context.Context, string) (usr.User, error)
}
type OnboardingDependencies struct {
	Apps           app.Repository
	Projects       project.Repository
	Pipelines      pipe.Repository
	Params         ep.Repository
	Fields         pp.Repository
	Orders         *ReleaseOrderManager
	Templates      *ReleaseTemplateManager
	Jenkins        JenkinsReleaseParamReader
	Users          OnboardingUsers
	Settings       *QueryReleaseSettings
	JenkinsEnabled bool
}
type OnboardingManager struct {
	repo ob.Repository
	d    OnboardingDependencies
}

func NewOnboardingManager(repo ob.Repository, d OnboardingDependencies) *OnboardingManager {
	return &OnboardingManager{repo, d}
}
func onboardingID(session, kind string) string {
	sum := sha256.Sum256([]byte(session + ":" + kind))
	return fmt.Sprintf("%s-%x", strings.Split(kind, ":")[0], sum[:12])
}
func onboardingIssue(step, field, code, message string) ob.Issue {
	return ob.Issue{Code: code, Severity: "blocking", Step: step, FieldPath: field, Message: message, Remedy: "返回对应步骤修复后重新检查"}
}
func (m *OnboardingManager) Get(ctx context.Context, id string) (ob.Session, error) {
	return m.repo.Get(ctx, id)
}
func (m *OnboardingManager) List(ctx context.Context, owner string) ([]ob.Session, error) {
	return m.repo.List(ctx, owner)
}
func (m *OnboardingManager) Status(ctx context.Context, owner string) (map[string]any, error) {
	sessions, err := m.repo.List(ctx, owner)
	if err != nil {
		return nil, err
	}
	_, count, err := m.d.Pipelines.ListPipelines(ctx, pipe.PipelineListFilter{Provider: pipe.ProviderJenkins, Status: pipe.StatusActive, Page: 1, PageSize: 1})
	if err != nil {
		return nil, err
	}
	envs := []string{}
	if m.d.Settings != nil {
		settings, e := m.d.Settings.Execute(ctx)
		if e != nil {
			return nil, e
		}
		envs = settings.EnvOptions
	}
	return map[string]any{"sessions": sessions, "jenkins_enabled": m.d.JenkinsEnabled, "pipeline_count": count, "env_options": envs}, nil
}
func (m *OnboardingManager) CreateSession(ctx context.Context, owner, mode, applicationID, projectID string) (ob.Session, error) {
	if mode != "create_application" && mode != "complete_application" {
		return ob.Session{}, fmt.Errorf("%w: 接入模式无效", ErrInvalidInput)
	}
	if (mode == "create_application" && applicationID != "") || (mode == "complete_application" && applicationID == "") {
		return ob.Session{}, fmt.Errorf("%w: 接入模式与应用不匹配", ErrInvalidInput)
	}
	now := time.Now().UTC()
	s := ob.Session{ID: generateID("onb"), OwnerUserID: owner, Mode: mode, Status: "draft", CurrentStep: "preflight", Version: 1, CreatedAt: now, UpdatedAt: now, Check: ob.Check{Status: "unknown", Issues: []ob.Issue{}}}
	s.Draft.Identity.OwnerUserID = owner
	s.Draft.Params = []ob.Parameter{}
	if projectID != "" {
		if _, err := m.d.Projects.GetByID(ctx, projectID); err != nil {
			return s, err
		}
		s.Draft.Identity.ProjectID = projectID
	}
	if mode == "complete_application" {
		a, err := m.d.Apps.GetByID(ctx, applicationID)
		if err != nil {
			return s, err
		}
		sessions, err := m.repo.List(ctx, owner)
		if err != nil {
			return s, err
		}
		for _, old := range sessions {
			if old.Refs.ApplicationID == applicationID && old.Mode == mode && old.Status != "abandoned" && old.Status != "verified" && old.Status != "configured" {
				return old, nil
			}
		}
		s.Refs.ApplicationID = a.ID
		s.Refs.ProjectID = a.ProjectID
		s.Draft.Identity = ob.Identity{ProjectID: a.ProjectID, Name: a.Name, Key: a.Key, OwnerUserID: a.OwnerUserID, Description: a.Description}
		s.Draft.RepoURL = a.RepoURL
		if len(a.ReleaseBranches) > 0 {
			s.Draft.Branch = a.ReleaseBranches[0].Branch
		}
		bindings, _, err := m.d.Pipelines.ListBindingsByApplication(ctx, pipe.BindingListFilter{ApplicationID: a.ID, Page: 1, PageSize: 100})
		if err != nil {
			return s, err
		}
		for _, b := range bindings {
			if b.Provider != pipe.ProviderJenkins {
				continue
			}
			switch b.BindingType {
			case pipe.BindingTypeCI:
				s.Refs.CIBindingID = b.ID
				s.Draft.CIPipelineID = b.PipelineID
			case pipe.BindingTypeCD:
				s.Refs.CDBindingID = b.ID
				s.Draft.CDPipelineID = b.PipelineID
			}
		}
		s.Draft.ApprovalFlowID, err = m.d.Orders.GetApplicationApprovalFlowID(ctx, a.ID)
		if err != nil {
			return s, err
		}
		templates, _, err := m.d.Orders.repo.ListTemplates(ctx, rel.TemplateListFilter{ApplicationID: a.ID, Page: 1, PageSize: 100})
		if err != nil {
			return s, err
		}
		if len(templates) == 1 {
			t, _, params, _, _, err := m.d.Templates.GetByID(ctx, templates[0].ID)
			if err != nil {
				return s, err
			}
			s.Refs.TemplateID = t.ID
			s.Draft.TemplateName = t.Name
			for _, p := range params {
				s.Draft.Params = append(s.Draft.Params, ob.Parameter{Scope: string(p.PipelineScope), Name: p.ExecutorParamName, ParamKey: p.ParamKey, ValueSource: string(p.ValueSource), SourceParamKey: p.SourceParamKey})
			}
			// Imported templates remain authoritative and read-only. Do not copy
			// their values into another store; metadata may hide a secret type.
		}
		s.CurrentStep = "pipelines"
	}
	if err := m.repo.Create(ctx, s); err != nil {
		return s, err
	}
	return s, nil
}
func validateOnboardingDraft(d ob.Draft) error {
	if len(d.Params) > 300 {
		return fmt.Errorf("%w: 参数数量超出接入限制", ErrInvalidInput)
	}
	for _, p := range d.Params {
		if p.FixedValue != "" && (onboardingSensitive.MatchString(p.Name) || onboardingSensitive.MatchString(p.ParamKey)) {
			return fmt.Errorf("%w: 敏感参数不能保存到接入草稿", ErrInvalidInput)
		}
	}
	return nil
}
func (m *OnboardingManager) SaveDraft(ctx context.Context, id string, version int64, draft ob.Draft) (ob.Session, error) {
	s, err := m.repo.Get(ctx, id)
	if err != nil {
		return s, err
	}
	if s.Status == "abandoned" {
		return s, ob.ErrConflict
	}
	if s.Refs.ApplicationID != "" && draft.Identity != s.Draft.Identity {
		return s, fmt.Errorf("%w: 应用基本信息已保存，请通过应用编辑修改", ob.ErrConflict)
	}
	if (s.Refs.CIBindingID != "" && draft.CIPipelineID != s.Draft.CIPipelineID) || (s.Refs.CDBindingID != "" && draft.CDPipelineID != s.Draft.CDPipelineID) {
		return s, fmt.Errorf("%w: 已有绑定不能通过草稿替换，请使用管线绑定配置", ob.ErrConflict)
	}
	if err = validateOnboardingDraft(draft); err != nil {
		return s, err
	}
	// Verify type metadata before persisting any fixed value. A harmless-looking
	// parameter name can still be a password/credentials parameter in Jenkins.
	for _, p := range draft.Params {
		if p.FixedValue == "" {
			continue
		}
		inspection, e := m.Inspect(ctx, draft)
		if e != nil {
			return s, e
		}
		if len(inspection.Issues) != 0 {
			return s, fmt.Errorf("无法确认参数类型，暂不能保存固定值；请先恢复管线连接")
		}
		for _, candidate := range draft.Params {
			if candidate.FixedValue == "" {
				continue
			}
			safe := false
			for _, row := range inspection.Rows {
				if row.Scope == candidate.Scope && row.Name == candidate.Name && !row.Sensitive && row.Problem == "" {
					safe = true
				}
			}
			if !safe {
				return s, fmt.Errorf("%w: 未确认类型或敏感参数不能保存固定值", ErrInvalidInput)
			}
		}
		break
	}
	// Existing template settings are authoritative, not editable snapshots. Use
	// the established template editor to change them, then re-open the guide.
	if s.Refs.TemplateID != "" {
		old, _ := json.Marshal(s.Draft)
		next, _ := json.Marshal(draft)
		if string(old) != string(next) {
			return s, fmt.Errorf("%w: 已有模板请通过高级配置修改，避免覆盖原有设置", ob.ErrConflict)
		}
	}
	s.Draft = draft
	s.Check = ob.Check{Status: "unknown", Issues: []ob.Issue{}}
	if s.Status == "configured" || s.Status == "verified" {
		s.Status = "in_progress"
	}
	return m.repo.Save(ctx, s, version)
}
func (m *OnboardingManager) Inspect(ctx context.Context, draft ob.Draft) (OnboardingInspection, error) {
	out := OnboardingInspection{Rows: []OnboardingParamRow{}, Fields: []OnboardingField{}, Issues: []ob.Issue{}}
	if !m.d.JenkinsEnabled {
		out.Issues = append(out.Issues, onboardingIssue("preflight", "", "connection_unavailable", "Jenkins 执行端未启用，请先完成安装配置"))
		return out, nil
	}
	fields := []pp.PlatformParamDict{}
	for page := 1; ; page++ {
		items, total, err := m.d.Fields.List(ctx, pp.ListFilter{Page: page, PageSize: 100})
		if err != nil {
			return out, err
		}
		fields = append(fields, items...)
		if len(items) == 0 || int64(len(fields)) >= total {
			break
		}
	}
	builtin, err := m.d.Templates.listBuiltinPlatformParamsForTemplate(ctx)
	if err != nil {
		return out, err
	}
	for _, field := range fields {
		if field.Status != pp.StatusEnabled || field.CDSelfFill {
			continue
		}
		_, isBuiltin := builtin[field.ParamKey]
		out.Fields = append(out.Fields, OnboardingField{field.ParamKey, field.Name, string(field.ParamType), isBuiltin})
	}
	for _, selection := range []struct{ scope, id string }{{"ci", draft.CIPipelineID}, {"cd", draft.CDPipelineID}} {
		if selection.id == "" {
			continue
		}
		p, err := m.d.Pipelines.GetPipelineByID(ctx, selection.id)
		if err != nil {
			return out, err
		}
		if p.Status != pipe.StatusActive || p.Provider != pipe.ProviderJenkins {
			out.Issues = append(out.Issues, onboardingIssue("pipelines", selection.scope, "pipeline_unavailable", "所选管线已停用或不是 Jenkins 管线"))
			continue
		}
		if m.d.Jenkins == nil {
			out.Issues = append(out.Issues, onboardingIssue("preflight", "", "connection_unavailable", "执行端尚未配置"))
			continue
		}
		snapshot, err := m.d.Jenkins.GetJobParamSet(ctx, p.JobFullName)
		if err != nil {
			out.Issues = append(out.Issues, onboardingIssue("pipelines", selection.scope, "live_read_failed", strings.ToUpper(selection.scope)+" 管线参数读取失败，请检查连接、读取权限后重试"))
			continue
		}
		stored, _, err := m.d.Params.ListByPipeline(ctx, ep.ListFilter{PipelineID: p.ID, Page: 1, PageSize: 10000})
		if err != nil {
			return out, err
		}
		byName := map[string]ep.ExecutorParamDef{}
		for _, v := range stored {
			byName[v.ExecutorParamName] = v
		}
		for _, param := range snapshot.Params {
			out.Rows = append(out.Rows, ResolveOnboardingParamSuggestions(selection.scope, p.ID, param, byName[param.Name], fields))
		}
	}
	if draft.CIPipelineID == "" && draft.CDPipelineID == "" {
		out.Issues = append(out.Issues, onboardingIssue("pipelines", "", "pipeline_required", "请至少选择一条构建或部署管线"))
	}
	return out, nil
}
func (m *OnboardingManager) ApplyStep(ctx context.Context, id, key, step string, version int64) (ob.Session, error) {
	if key == "" || len(key) > 100 {
		return ob.Session{}, fmt.Errorf("%w: request_key 无效", ErrInvalidInput)
	}
	s, err := m.repo.Get(ctx, id)
	if err != nil {
		return s, err
	}
	if s.Status == "abandoned" {
		return s, ob.ErrConflict
	}
	switch step {
	case "preflight", "identity", "pipelines", "parameters", "template_flow", "review":
	default:
		return s, fmt.Errorf("%w: 无效步骤", ErrInvalidInput)
	}
	payload, _ := json.Marshal(s.Draft)
	hash := fmt.Sprintf("%x", sha256.Sum256(append([]byte(step+":"), payload...)))
	s, replayed, err := m.repo.BeginOperation(ctx, id, key, hash, step, version)
	if err != nil || replayed {
		return s, err
	}
	work, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	s.Check = ob.Check{Status: "unknown", Issues: []ob.Issue{}}
	switch step {
	case "preflight":
		if !m.d.JenkinsEnabled {
			err = fmt.Errorf("请先在安装配置中启用 Jenkins 并同步已有管线")
		} else {
			s.CurrentStep = "identity"
		}
	case "identity":
		err = m.applyIdentity(work, &s)
		if err == nil {
			s.CurrentStep = "pipelines"
		}
	case "pipelines":
		err = m.applyPipelines(work, &s)
		if err == nil {
			s.CurrentStep = "parameters"
		}
	case "parameters":
		err = m.applyParameters(work, &s)
		if err == nil {
			s.CurrentStep = "template_flow"
		}
	case "template_flow":
		err = m.applyTemplate(work, &s)
		if err == nil {
			s.CurrentStep = "review"
		}
	case "review":
		s.Check, err = m.CheckSession(work, s)
		if err == nil && s.Check.Status != "ready" {
			err = fmt.Errorf("配置检查尚未通过，请修复标记项")
		}
		if err == nil {
			s.Status = "configured"
			s.CurrentStep = "first_release"
		}
	}
	if err != nil {
		s.Status = "blocked"
		if len(s.Check.Issues) == 0 {
			s.Check = ob.Check{Status: "blocked", Issues: []ob.Issue{onboardingIssue(step, "", "step_failed", err.Error())}}
		}
	} else if s.Status != "configured" {
		s.Status = "in_progress"
		s.Check = ob.Check{Status: "unknown", Issues: []ob.Issue{}}
	}
	persist, cancelPersist := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancelPersist()
	saved, saveErr := m.repo.FinishOperation(persist, s, key, err == nil)
	if saveErr != nil {
		return saved, saveErr
	}
	return saved, err
}
func (m *OnboardingManager) applyIdentity(ctx context.Context, s *ob.Session) error {
	d := s.Draft.Identity
	if s.Refs.ApplicationID != "" {
		a, err := m.d.Apps.GetByID(ctx, s.Refs.ApplicationID)
		if err != nil {
			return err
		}
		repoURL := strings.TrimSpace(s.Draft.RepoURL)
		if repoURL == "" || repoURL == a.RepoURL {
			return nil
		}
		_, err = NewUpdateApplication(m.d.Apps, m.d.Projects).Execute(ctx, a.ID, app.UpdateInput{Name: a.Name, Key: a.Key, ProjectID: a.ProjectID, RepoURL: repoURL, Description: a.Description, OwnerUserID: a.OwnerUserID, Owner: a.Owner, Status: a.Status, ArtifactType: a.ArtifactType, Language: a.Language(), ArtifactRepositoryID: a.ArtifactRepositoryID, ArtifactDirectory: a.ArtifactDirectory, GitOpsBranchMappings: a.GitOpsBranchMappings, ReleaseBranches: a.ReleaseBranches})
		return err
	}
	owner, err := m.d.Users.GetUserByID(ctx, d.OwnerUserID)
	if err != nil {
		return err
	}
	if owner.Status != usr.StatusActive {
		return fmt.Errorf("应用负责人已停用")
	}
	projectID := d.ProjectID
	if projectID == "" {
		projectID = onboardingID(s.ID, "prj")
		existing, e := m.d.Projects.GetByID(ctx, projectID)
		if errors.Is(e, project.ErrNotFound) {
			_, e = NewProjectManager(m.d.Projects).Create(withRecoveryID(ctx, "prj", projectID), CreateProjectInput{Name: d.ProjectName, Key: d.ProjectKey, Status: project.StatusActive})
		} else if e == nil && (existing.Name != strings.TrimSpace(d.ProjectName) || existing.Key != strings.TrimSpace(d.ProjectKey)) {
			return ob.ErrConflict
		}
		if e != nil {
			return e
		}
	}
	prj, err := m.d.Projects.GetByID(ctx, projectID)
	if err != nil {
		return err
	}
	if prj.Status != project.StatusActive {
		return fmt.Errorf("项目已停用")
	}
	s.Refs.ProjectID = projectID
	appID := onboardingID(s.ID, "app")
	repoURL := strings.TrimSpace(s.Draft.RepoURL)
	existing, err := m.d.Apps.GetByID(ctx, appID)
	if errors.Is(err, app.ErrNotFound) {
		_, err = NewCreateApplication(m.d.Apps, m.d.Projects).Execute(withRecoveryID(ctx, "app", appID), CreateInput{Name: d.Name, Key: d.Key, ProjectID: projectID, RepoURL: repoURL, OwnerUserID: owner.ID, Owner: firstNonEmpty(owner.DisplayName, owner.Username), Description: d.Description})
	} else if err == nil && (existing.Key != strings.TrimSpace(d.Key) || existing.Name != strings.TrimSpace(d.Name) || existing.ProjectID != projectID || existing.RepoURL != repoURL || existing.OwnerUserID != owner.ID || existing.Description != strings.TrimSpace(d.Description)) {
		return ob.ErrConflict
	}
	if err == nil {
		s.Refs.ApplicationID = appID
	}
	return err
}
func (m *OnboardingManager) applyPipelines(ctx context.Context, s *ob.Session) error {
	if s.Refs.ApplicationID == "" {
		return fmt.Errorf("请先创建或选择应用")
	}
	inspection, err := m.Inspect(ctx, s.Draft)
	if err != nil {
		return err
	}
	if len(inspection.Issues) > 0 {
		s.Check = ob.Check{Status: "blocked", Issues: inspection.Issues}
		return fmt.Errorf("管线检查未通过")
	}
	bindings, _, err := m.d.Pipelines.ListBindingsByApplication(ctx, pipe.BindingListFilter{ApplicationID: s.Refs.ApplicationID, Page: 1, PageSize: 100})
	if err != nil {
		return err
	}
	manager := NewPipelineBindingManager(m.d.Pipelines, m.d.Apps)
	for _, selection := range []struct {
		scope, id string
		ref       *string
	}{{"ci", s.Draft.CIPipelineID, &s.Refs.CIBindingID}, {"cd", s.Draft.CDPipelineID, &s.Refs.CDBindingID}} {
		if selection.id == "" {
			*selection.ref = ""
			continue
		}
		var current *pipe.PipelineBinding
		for i := range bindings {
			if string(bindings[i].BindingType) == selection.scope {
				current = &bindings[i]
				break
			}
		}
		if current != nil {
			if current.Provider != pipe.ProviderJenkins || current.PipelineID != selection.id || current.Status != pipe.StatusActive {
				return fmt.Errorf("应用已存在不同或停用的 %s 绑定，请通过高级配置确认变更影响", selection.scope)
			}
			*selection.ref = current.ID
			continue
		}
		id := onboardingID(s.ID, "pb:"+selection.scope)
		b, err := manager.Create(withRecoveryID(ctx, "pb", id), s.Refs.ApplicationID, CreatePipelineBindingInput{BindingType: pipe.BindingType(selection.scope), Provider: pipe.ProviderJenkins, PipelineID: selection.id, TriggerMode: pipe.TriggerManual, Status: pipe.StatusActive})
		if err != nil {
			return err
		}
		*selection.ref = b.ID
	}
	// Sync only selected snapshots. Never mark definitions of unrelated jobs inactive.
	defs := []ep.ExecutorParamDef{}
	seen := map[string]bool{}
	now := time.Now().UTC()
	for _, row := range inspection.Rows {
		if seen[row.ID] {
			continue
		}
		seen[row.ID] = true
		if row.Sensitive {
			continue
		}
		defs = append(defs, ep.ExecutorParamDef{ID: row.ID, PipelineID: row.PipelineID, ExecutorType: ep.ExecutorTypeJenkins, ExecutorParamName: row.Name, ParamKey: defaultJenkinsExecutorParamKey(row.Name), ParamType: row.Live.ParamType, SingleSelect: row.Live.SingleSelect, Required: row.Required, DefaultValue: row.DefaultValue, Description: row.Description, Visible: true, Editable: true, SourceFrom: ep.SourceFromSyncJenkins, Status: ep.StatusActive, RawMeta: row.Live.RawMeta, SortNo: len(defs) + 1, CreatedAt: now, UpdatedAt: now})
	}
	_, _, err = m.d.Params.Upsert(ctx, defs)
	return err
}
func (m *OnboardingManager) validateParameters(inspection OnboardingInspection, s ob.Session) ([]ob.Issue, map[string]ob.Parameter) {
	issues := append([]ob.Issue{}, inspection.Issues...)
	chosen := map[string]ob.Parameter{}
	fields := map[string]OnboardingField{}
	for _, f := range inspection.Fields {
		fields[f.Key] = f
	}
	for _, p := range s.Draft.Params {
		k := p.Scope + ":" + p.Name
		if _, ok := chosen[k]; ok {
			issues = append(issues, onboardingIssue("parameters", k, "duplicate_parameter", "参数配置重复"))
		}
		chosen[k] = p
	}
	ciKeys := map[string]bool{}
	ciTypes := map[string]string{}
	for _, p := range s.Draft.Params {
		if p.Scope == "ci" && !p.Omit {
			ciKeys[p.ParamKey] = true
			for _, row := range inspection.Rows {
				if row.Scope == "ci" && row.Name == p.Name {
					ciTypes[p.ParamKey] = row.Type
				}
			}
		}
	}
	liveKeys := map[string]bool{}
	used := map[string]bool{}
	for _, row := range inspection.Rows {
		k := row.Scope + ":" + row.Name
		liveKeys[k] = true
		p, ok := chosen[k]
		message := ""
		switch {
		case row.Problem != "":
			message = row.Problem
		case !ok:
			message = "请选择参数来源或明确使用管线默认值"
		case p.Omit && (row.Required || row.Runtime):
			message = "必填参数不能省略"
		case p.Omit:
			continue
		case p.ParamKey == "":
			message = "请选择或新建标准字段"
		case row.MappedKey != "" && row.MappedKey != p.ParamKey:
			message = "已有共享映射不能通过向导覆盖"
		default:
			f, exists := fields[p.ParamKey]
			if !exists {
				if p.NewFieldName == "" {
					message = "标准字段不存在，请就地创建"
				} else if _, e := normalizePlatformParamKey(p.ParamKey); e != nil {
					message = "标准字段 Key 格式无效"
				}
			} else if !onboardingCompatible(row.Live.ParamType, pp.ParamType(f.Type)) {
				message = "标准字段类型与真实参数不兼容"
			}
			if message == "" {
				switch p.ValueSource {
				case "release_input":
					if row.Runtime {
						message = "上游构建信息必须使用运行时来源"
					}
				case "fixed":
					if row.Runtime {
						message = "上游构建信息不能使用固定值"
					} else if e := onboardingValidateValue(row, p.FixedValue); e != nil {
						message = e.Error()
					}
				case "builtin":
					source, exists := fields[p.SourceParamKey]
					if !exists || !source.Builtin {
						message = "请选择有效的发布基础字段"
					} else if !onboardingCompatible(row.Live.ParamType, pp.ParamType(source.Type)) {
						message = "来源字段类型不兼容"
					}
					if row.Runtime && (p.SourceParamKey != defaultJenkinsExecutorParamKey(row.Name) || s.Draft.CIPipelineID == "") {
						message = "此部署参数需要本次 CI 构建结果，请绑定 CI 管线"
					}
				case "ci_param":
					if row.Runtime {
						message = "上游构建信息必须使用运行时来源"
					} else if row.Scope != "cd" || !ciKeys[p.SourceParamKey] {
						message = "只能引用当前已选择的 CI 标准字段"
					} else if !onboardingCompatible(row.Live.ParamType, pp.ParamType(ciTypes[p.SourceParamKey])) && !(row.Type == "string" && ciTypes[p.SourceParamKey] == "choice") {
						message = "CI 来源字段类型不兼容"
					}
				default:
					message = "请选择参数值来源"
				}
			}
		}
		if !p.Omit && p.ParamKey != "" {
			fieldKey := row.Scope + ":" + p.ParamKey
			if used[fieldKey] {
				message = "同一阶段不能重复使用同一标准字段"
			}
			used[fieldKey] = true
		}
		if message != "" {
			issues = append(issues, onboardingIssue("parameters", k, "parameter_invalid", row.Name+"："+message))
		}
	}
	for k := range chosen {
		if !liveKeys[k] {
			issues = append(issues, onboardingIssue("parameters", k, "parameter_removed", "管线参数已变化，请重新读取并确认"))
		}
	}
	return issues, chosen
}
func (m *OnboardingManager) applyParameters(ctx context.Context, s *ob.Session) error {
	if s.Refs.ApplicationID == "" {
		return fmt.Errorf("请先创建应用")
	}
	inspection, err := m.Inspect(ctx, s.Draft)
	if err != nil {
		return err
	}
	issues, chosen := m.validateParameters(inspection, *s)
	if len(issues) > 0 {
		s.Check = ob.Check{Status: "blocked", Issues: issues}
		return fmt.Errorf("参数配置尚未完成")
	}
	writer, ok := m.d.Params.(interface {
		MapUnmappedParameter(context.Context, string, string) (ep.ExecutorParamDef, error)
	})
	if !ok {
		return fmt.Errorf("参数仓储不支持安全映射")
	}
	for _, row := range inspection.Rows {
		p := chosen[row.Scope+":"+row.Name]
		if p.Omit {
			continue
		}
		field, err := m.d.Fields.GetByParamKey(ctx, p.ParamKey)
		if err == nil && p.NewFieldName != "" && field.ID != onboardingID(s.ID, "ppd:"+p.ParamKey) {
			return fmt.Errorf("标准字段 %s 已存在，请重新读取并明确选择复用", p.ParamKey)
		}
		if errors.Is(err, pp.ErrNotFound) && p.NewFieldName != "" {
			field, err = NewPlatformParamDictManager(m.d.Fields, m.d.Params).Create(withRecoveryID(ctx, "ppd", onboardingID(s.ID, "ppd:"+p.ParamKey)), CreatePlatformParamDictInput{ParamKey: p.ParamKey, Name: p.NewFieldName, ParamType: pp.ParamType(row.Type), Status: pp.StatusEnabled})
		}
		if err != nil {
			return err
		}
		if field.Status != pp.StatusEnabled || field.CDSelfFill || !onboardingCompatible(row.Live.ParamType, field.ParamType) {
			return fmt.Errorf("标准字段 %s 已变化，请重新确认", p.ParamKey)
		}
		if _, err = writer.MapUnmappedParameter(ctx, row.ID, p.ParamKey); err != nil {
			return err
		}
	}
	if s.Draft.SaveAppSources {
		a, err := m.d.Apps.GetByID(ctx, s.Refs.ApplicationID)
		if err != nil {
			return err
		}
		branches := a.ReleaseBranches
		if strings.TrimSpace(s.Draft.Branch) != "" {
			exists := false
			for _, b := range branches {
				if b.Branch == s.Draft.Branch {
					exists = true
				}
			}
			if !exists {
				branches = append(branches, app.ReleaseBranchOption{Name: s.Draft.Branch, Branch: s.Draft.Branch})
			}
		}
		_, err = NewUpdateApplication(m.d.Apps, m.d.Projects).Execute(ctx, a.ID, app.UpdateInput{Name: a.Name, Key: a.Key, ProjectID: a.ProjectID, RepoURL: s.Draft.RepoURL, Description: a.Description, OwnerUserID: a.OwnerUserID, Owner: a.Owner, Status: a.Status, ArtifactType: a.ArtifactType, Language: a.Language(), ArtifactRepositoryID: a.ArtifactRepositoryID, ArtifactDirectory: a.ArtifactDirectory, GitOpsBranchMappings: a.GitOpsBranchMappings, ReleaseBranches: branches})
		if err != nil {
			return err
		}
	}
	return nil
}
func onboardingTemplateInput(s ob.Session) CreateReleaseTemplateInput {
	input := CreateReleaseTemplateInput{Name: s.Draft.TemplateName, ApplicationID: s.Refs.ApplicationID, CIBindingID: s.Refs.CIBindingID, CDBindingID: s.Refs.CDBindingID, Status: rel.TemplateStatusActive}
	if input.CDBindingID != "" {
		input.CDProvider = pipe.ProviderJenkins
	}
	for _, p := range s.Draft.Params {
		if p.Omit {
			continue
		}
		pipelineID := s.Draft.CIPipelineID
		if p.Scope == "cd" {
			pipelineID = s.Draft.CDPipelineID
		}
		config := ReleaseTemplateParamConfigInput{ExecutorParamDefID: executorParamDefID(pipelineID, "jenkins", p.Name), ValueSource: rel.TemplateParamValueSource(p.ValueSource), SourceParamKey: p.SourceParamKey, FixedValue: p.FixedValue}
		if p.Scope == "ci" {
			input.CIParamConfigs = append(input.CIParamConfigs, config)
		} else {
			input.CDParamConfigs = append(input.CDParamConfigs, config)
		}
	}
	return input
}
func (m *OnboardingManager) applyTemplate(ctx context.Context, s *ob.Session) error {
	if s.Refs.CIBindingID == "" && s.Refs.CDBindingID == "" {
		return fmt.Errorf("请先完成管线绑定")
	}
	if s.Refs.TemplateID != "" {
		_, _, _, _, _, err := m.d.Templates.GetByID(ctx, s.Refs.TemplateID)
		// Only finish a flow change for a template created by this session. Never
		// mutate an existing application's flow implicitly while importing it.
		if err == nil && s.Refs.TemplateID == onboardingID(s.ID, "rt") && s.Draft.ChangeApprovalFlow {
			err = m.d.Orders.SetApplicationApprovalFlowID(ctx, s.Refs.ApplicationID, s.Draft.ApprovalFlowID)
		}
		return err
	}
	inspection, err := m.Inspect(ctx, s.Draft)
	if err != nil {
		return err
	}
	issues, _ := m.validateParameters(inspection, *s)
	if len(issues) > 0 {
		s.Check = ob.Check{Status: "blocked", Issues: issues}
		return fmt.Errorf("请先完成参数配置")
	}
	if s.Draft.ChangeApprovalFlow && s.Draft.ApprovalFlowID != "" {
		if _, err := m.d.Orders.repo.GetApprovalFlowDefinitionByID(ctx, s.Draft.ApprovalFlowID); err != nil {
			return err
		}
	}
	input := onboardingTemplateInput(*s)
	if strings.TrimSpace(input.Name) == "" {
		input.Name = s.Draft.Identity.Name + "-默认发布"
	}
	id := onboardingID(s.ID, "rt")
	existing, _, _, _, _, err := m.d.Templates.GetByID(ctx, id)
	if errors.Is(err, rel.ErrTemplateNotFound) {
		templates, _, e := m.d.Orders.repo.ListTemplates(ctx, rel.TemplateListFilter{ApplicationID: s.Refs.ApplicationID, Page: 1, PageSize: 1000})
		if e != nil {
			return e
		}
		for _, t := range templates {
			if t.Name == input.Name {
				return fmt.Errorf("已存在同名模板，请更改名称或使用高级配置")
			}
		}
		existing, _, _, _, _, err = m.d.Templates.Create(withRecoveryID(ctx, "rt", id), input)
	}
	if err != nil {
		return err
	}
	if existing.ApplicationID != s.Refs.ApplicationID {
		return ob.ErrConflict
	}
	s.Refs.TemplateID = existing.ID
	if s.Draft.ChangeApprovalFlow {
		if err = m.d.Orders.SetApplicationApprovalFlowID(ctx, s.Refs.ApplicationID, s.Draft.ApprovalFlowID); err != nil {
			return err
		}
	}
	return nil
}
func (m *OnboardingManager) CheckSession(ctx context.Context, s ob.Session) (ob.Check, error) {
	result := ob.Check{Status: "ready", Issues: []ob.Issue{}}
	add := func(step, code, message string) {
		result.Issues = append(result.Issues, onboardingIssue(step, "", code, message))
		result.Status = "blocked"
	}
	if s.Refs.ApplicationID == "" {
		add("identity", "application_missing", "请先创建应用")
		return result, nil
	}
	a, err := m.d.Apps.GetByID(ctx, s.Refs.ApplicationID)
	if err != nil {
		return result, err
	}
	if a.Status != app.StatusActive {
		add("identity", "application_inactive", "应用已停用")
	}
	owner, err := m.d.Users.GetUserByID(ctx, a.OwnerUserID)
	if err != nil || owner.Status != usr.StatusActive {
		add("identity", "owner_invalid", "应用负责人不存在或已停用")
	}
	if s.Refs.TemplateID == "" {
		add("template_flow", "template_missing", "尚未创建发布模板")
		return result, nil
	}
	t, bindings, params, _, _, err := m.d.Templates.GetByID(ctx, s.Refs.TemplateID)
	if err != nil {
		return result, err
	}
	if t.ApplicationID != a.ID || t.Status != rel.TemplateStatusActive {
		add("template_flow", "template_inactive", "模板停用或不属于当前应用")
	}
	if string(t.ComplianceStatus) == "violated" {
		add("template_flow", "pipeline_compliance", t.ComplianceSummary)
	}
	selected := map[string]rel.ReleaseTemplateParam{}
	ciKeys := map[string]bool{}
	stageKeys := map[string]bool{}
	for _, p := range params {
		stageKey := string(p.PipelineScope) + ":" + p.ParamKey
		if stageKeys[stageKey] {
			add("parameters", "duplicate_field", p.ParamKey+" 在同一阶段重复使用，请在高级配置中修复")
		}
		stageKeys[stageKey] = true
		selected[string(p.PipelineScope)+":"+p.ExecutorParamName] = p
		if p.PipelineScope == rel.PipelineScopeCI {
			ciKeys[p.ParamKey] = true
		}
	}
	draft := ob.Draft{}
	for _, b := range bindings {
		if !b.Enabled {
			continue
		}
		if b.Provider != "jenkins" {
			add("template_flow", "advanced_executor", "此应用使用高级执行器，请进入原模板编辑器检查")
			continue
		}
		binding, e := m.d.Pipelines.GetBindingByID(ctx, b.BindingID)
		if e != nil || binding.Status != pipe.StatusActive || binding.ApplicationID != a.ID || string(binding.BindingType) != string(b.PipelineScope) || binding.PipelineID != b.PipelineID {
			add("pipelines", "binding_invalid", "模板引用的管线绑定已变化或停用")
			continue
		}
		if b.PipelineScope == rel.PipelineScopeCI {
			draft.CIPipelineID = b.PipelineID
		} else {
			draft.CDPipelineID = b.PipelineID
		}
	}
	inspection, err := m.Inspect(ctx, draft)
	if err != nil {
		return result, err
	}
	for _, issue := range inspection.Issues {
		result.Issues = append(result.Issues, issue)
		result.Status = "blocked"
		if issue.Code == "live_read_failed" {
			result.Status = "unknown"
		}
	}
	live := map[string]bool{}
	for _, row := range inspection.Rows {
		k := row.Scope + ":" + row.Name
		live[k] = true
		p, exists := selected[k]
		if !exists {
			if row.Required || row.Runtime {
				add("parameters", "required_parameter_missing", row.Name+" 必填参数尚未配置")
			}
			continue
		}
		if row.Problem != "" {
			add("parameters", "unsupported_parameter", row.Name+"："+row.Problem)
		}
		if row.Runtime && (p.ValueSource != rel.TemplateParamValueSourceBuiltin || p.SourceParamKey != defaultJenkinsExecutorParamKey(row.Name) || draft.CIPipelineID == "") {
			add("parameters", "runtime_source_invalid", row.Name+" 必须使用本次 CI 的运行时结果，且需要绑定 CI")
		}
		messages := m.d.Orders.validateSingleTemplateParamMapping(ctx, p, map[string]ep.JenkinsParamSnapshot{strings.ToLower(row.Name): row.Live}, p.FixedValue)
		for _, message := range messages {
			add("parameters", "parameter_invalid", message)
		}
		field, e := m.d.Fields.GetByParamKey(ctx, p.ParamKey)
		if e != nil || field.Status != pp.StatusEnabled || field.CDSelfFill || !onboardingCompatible(row.Live.ParamType, field.ParamType) {
			add("parameters", "field_invalid", row.Name+" 的标准字段无效")
		}
		if row.MappedKey != p.ParamKey {
			add("parameters", "mapping_changed", row.Name+" 的共享映射已变化")
		}
		if p.ValueSource == rel.TemplateParamValueSourceFixed {
			if e := onboardingValidateValue(row, p.FixedValue); e != nil {
				add("parameters", "fixed_invalid", e.Error())
			}
		}
		if p.ValueSource == rel.TemplateParamValueSourceCIParam && !ciKeys[p.SourceParamKey] {
			add("parameters", "ci_source_missing", row.Name+" 引用的 CI 来源不存在")
		}
		if p.ValueSource == rel.TemplateParamValueSourceBuiltin {
			validSource := false
			for _, f := range inspection.Fields {
				if f.Key == p.SourceParamKey && f.Builtin && onboardingCompatible(row.Live.ParamType, pp.ParamType(f.Type)) {
					validSource = true
				}
			}
			if !validSource {
				add("parameters", "builtin_source_invalid", row.Name+" 的基础字段来源无效")
			}
			if isCIJenkinsRuntimeParamKey(p.SourceParamKey) && draft.CIPipelineID == "" {
				add("pipelines", "ci_source_missing", "CD 需要上游 CI 构建结果，请绑定 CI")
			}
			if p.SourceParamKey == "repo_url" && a.RepoURL == "" {
				add("parameters", "repo_source_missing", "选择了应用仓库来源，但应用仓库尚未填写")
			}
		}
	}
	for k, p := range selected {
		if !live[k] {
			add("parameters", "parameter_removed", p.ExecutorParamName+" 无法在真实管线中确认，请重新检查")
		}
	}
	flowID, err := m.d.Orders.GetApplicationApprovalFlowID(ctx, a.ID)
	if err != nil {
		return result, err
	}
	if s.Refs.TemplateID == onboardingID(s.ID, "rt") && s.Draft.ChangeApprovalFlow && flowID != s.Draft.ApprovalFlowID {
		add("template_flow", "approval_not_applied", "本次选择的应用审批流程尚未保存，请返回重试流程设置")
	}
	if flowID != "" {
		flow, e := m.d.Orders.repo.GetApprovalFlowDefinitionByID(ctx, flowID)
		if e != nil || string(flow.Status) != "active" {
			add("template_flow", "approval_invalid", "应用审批流程不存在或已停用")
		}
	}
	return result, nil
}
func (m *OnboardingManager) Check(ctx context.Context, id string) (ob.Session, error) {
	s, err := m.repo.Get(ctx, id)
	if err != nil {
		return s, err
	}
	if s.Status == "abandoned" {
		return s, ob.ErrConflict
	}
	s.Check, err = m.CheckSession(ctx, s)
	if err != nil {
		return s, err
	}
	if s.Check.Status == "ready" {
		s.Status = "configured"
		s.CurrentStep = "first_release"
		if s.FirstReleaseOrderID != "" {
			order, e := m.d.Orders.repo.GetByID(ctx, s.FirstReleaseOrderID)
			if e == nil && (order.Status == rel.OrderStatusSuccess || order.Status == rel.OrderStatusDeploySuccess) {
				s.Status = "verified"
			}
		}
	} else {
		s.Status = "blocked"
	}
	return m.repo.Save(ctx, s, s.Version)
}
func (m *OnboardingManager) SetupStatus(ctx context.Context, applicationID string) (map[string]any, error) {
	a, err := m.d.Apps.GetByID(ctx, applicationID)
	if err != nil {
		return nil, err
	}
	templates := []rel.ReleaseTemplate{}
	for page := 1; ; page++ {
		items, total, err := m.d.Orders.repo.ListTemplates(ctx, rel.TemplateListFilter{ApplicationID: a.ID, Page: page, PageSize: 100})
		if err != nil {
			return nil, err
		}
		templates = append(templates, items...)
		if len(items) == 0 || int64(len(templates)) >= total {
			break
		}
	}
	results := []map[string]any{}
	status := "blocked"
	for _, t := range templates {
		check, e := m.CheckSession(ctx, ob.Session{Refs: ob.References{ApplicationID: a.ID, TemplateID: t.ID}})
		if e != nil {
			// A broken template must not hide a different, usable template. Never
			// expose a remote response or credential-bearing error in the summary.
			check = ob.Check{Status: "unknown", Issues: []ob.Issue{onboardingIssue("review", "", "check_unavailable", "暂时无法检查此模板，请重试或进入高级配置")}}
		}
		results = append(results, map[string]any{"template_id": t.ID, "template_name": t.Name, "check": check})
		if check.Status == "ready" {
			status = "ready"
		} else if status != "ready" && check.Status == "unknown" {
			status = "unknown"
		}
	}
	return map[string]any{"status": status, "templates": results, "application_id": a.ID}, nil
}
func (m *OnboardingManager) Abandon(ctx context.Context, id string, version int64) (ob.Session, error) {
	s, err := m.repo.Get(ctx, id)
	if err != nil {
		return s, err
	}
	s.Status = "abandoned"
	return m.repo.Save(ctx, s, version)
}
func (m *OnboardingManager) CreateFirstRelease(ctx context.Context, id, key string, version int64, input CreateReleaseOrderInput) (ob.Session, error) {
	ctx, cancelWork := context.WithTimeout(ctx, 90*time.Second)
	defer cancelWork()
	if key == "" || len(key) > 100 {
		return ob.Session{}, fmt.Errorf("%w: request_key 无效", ErrInvalidInput)
	}
	s, err := m.repo.Get(ctx, id)
	if err != nil {
		return s, err
	}
	if s.Status == "abandoned" || s.Refs.TemplateID == "" {
		return s, ob.ErrConflict
	}
	if input.ApplicationID != s.Refs.ApplicationID || input.TemplateID != s.Refs.TemplateID {
		return s, fmt.Errorf("%w: 发布单必须属于当前接入应用和模板", ErrInvalidInput)
	}
	// The first order is pinned to this session; later releases use the regular
	// creation page. Recover its reference even after a lost HTTP response.
	if s.FirstReleaseOrderID == "" {
		check, e := m.CheckSession(ctx, s)
		if e != nil {
			return s, e
		}
		if check.Status != "ready" {
			s.Check = check
			return s, fmt.Errorf("配置已变化，请返回接入向导重新检查")
		}
	}
	payload, _ := json.Marshal(input)
	hash := fmt.Sprintf("%x", sha256.Sum256(payload))
	s, replayed, err := m.repo.BeginOperation(ctx, id, key, hash, "first_release", version)
	if err != nil || replayed {
		return s, err
	}
	orderID := onboardingID(s.ID, "ro")
	order, e := m.d.Orders.repo.GetByID(ctx, orderID)
	if errors.Is(e, rel.ErrOrderNotFound) {
		order, e = m.d.Orders.Create(withRecoveryID(ctx, "ro", orderID), input)
	}
	if e == nil {
		if order.ApplicationID != s.Refs.ApplicationID || order.TemplateID != s.Refs.TemplateID {
			e = ob.ErrConflict
		} else {
			s.FirstReleaseOrderID = order.ID
		}
	}
	persist, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	saved, saveErr := m.repo.FinishOperation(persist, s, key, e == nil)
	if saveErr != nil {
		return saved, saveErr
	}
	return saved, e
}
