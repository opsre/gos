package usecase

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	ep "gos/internal/domain/executorparam"
	ob "gos/internal/domain/onboarding"
	pipe "gos/internal/domain/pipeline"
	pp "gos/internal/domain/platformparam"
	rel "gos/internal/domain/release"
	usr "gos/internal/domain/user"
	"gos/internal/infrastructure/persistence/sqlrepo"
	_ "modernc.org/sqlite"
)

type onboardingReaderFake struct {
	jobs  map[string][]ep.JenkinsParamSnapshot
	err   error
	reads int
}

func (r *onboardingReaderFake) GetJobParamSet(_ context.Context, name string) (ep.JenkinsJobParamSet, error) {
	r.reads++
	return ep.JenkinsJobParamSet{JobFullName: name, Params: r.jobs[name]}, r.err
}

type onboardingUsersFake struct{}

func (onboardingUsersFake) GetUserByID(_ context.Context, id string) (usr.User, error) {
	return usr.User{ID: id, Username: "owner", Status: usr.StatusActive}, nil
}

func newOnboardingTestManager(t *testing.T) (*OnboardingManager, *onboardingReaderFake, *sql.DB) {
	t.Helper()
	ctx := context.Background()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })
	apps := sqlrepo.NewApplicationRepository(db, "sqlite")
	projects := sqlrepo.NewProjectRepository(db, "sqlite")
	pipelines := sqlrepo.NewPipelineRepository(db, "sqlite")
	params := sqlrepo.NewExecutorParamRepository(db, "sqlite")
	fields := sqlrepo.NewPlatformParamRepository(db, "sqlite")
	releases := sqlrepo.NewReleaseRepository(db, "sqlite")
	sessions := sqlrepo.NewOnboardingRepository(db, "sqlite")
	users := sqlrepo.NewUserRepository(db, "sqlite")
	for _, init := range []func(context.Context) error{projects.InitSchema, apps.InitSchema, pipelines.InitSchema, params.InitSchema, fields.InitSchema, users.InitSchema, releases.InitSchema, sessions.InitSchema} {
		if err := init(ctx); err != nil {
			t.Fatal(err)
		}
	}
	now := time.Now().UTC()
	if _, _, err := pipelines.UpsertPipelines(ctx, []pipe.Pipeline{
		{ID: "ci", Provider: pipe.ProviderJenkins, JobName: "ci", JobFullName: "ci", Status: pipe.StatusActive, CreatedAt: now, UpdatedAt: now, LastSyncedAt: now},
		{ID: "cd", Provider: pipe.ProviderJenkins, JobName: "cd", JobFullName: "cd", Status: pipe.StatusActive, CreatedAt: now, UpdatedAt: now, LastSyncedAt: now},
	}); err != nil {
		t.Fatal(err)
	}
	reader := &onboardingReaderFake{jobs: map[string][]ep.JenkinsParamSnapshot{
		"ci": {{Name: "DEPLOY_ENV", ParamType: ep.ParamTypeChoice, SingleSelect: true, Required: true, RawMeta: `{"choices":["dev","prod"]}`}},
		"cd": {{Name: "CI_JOB", ParamType: ep.ParamTypeString, Required: true}, {Name: "CI_BUILD", ParamType: ep.ParamTypeString, Required: true}},
	}}
	templates := NewReleaseTemplateManager(releases, apps, pipelines, params, fields, nil, nil, nil, nil)
	orders := NewReleaseOrderManager(releases, apps, pipelines, params, fields, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	return NewOnboardingManager(sessions, OnboardingDependencies{Apps: apps, Projects: projects, Pipelines: pipelines, Params: params, Fields: fields, Orders: orders, Templates: templates, Jenkins: reader, Users: onboardingUsersFake{}, JenkinsEnabled: true}), reader, db
}
func onboardingStep(t *testing.T, m *OnboardingManager, s ob.Session, step string) ob.Session {
	t.Helper()
	next, err := m.ApplyStep(context.Background(), s.ID, fmt.Sprintf("%s-%d", step, s.Version), step, s.Version)
	if err != nil {
		t.Fatalf("step %s: %v / %+v", step, err, next.Check)
	}
	return next
}
func onboardingIdentity(t *testing.T, m *OnboardingManager, key, projectID string) ob.Session {
	t.Helper()
	ctx := context.Background()
	s, err := m.CreateSession(ctx, "owner", "create_application", "", projectID)
	if err != nil {
		t.Fatal(err)
	}
	d := s.Draft
	d.Identity.Name = key
	d.Identity.Key = key
	d.Identity.ProjectName = "Project"
	d.Identity.ProjectKey = "project"
	s, err = m.SaveDraft(ctx, s.ID, s.Version, d)
	if err != nil {
		t.Fatal(err)
	}
	return onboardingStep(t, m, s, "identity")
}
func onboardingConfigure(t *testing.T, m *OnboardingManager, s ob.Session, ci, cd string) ob.Session {
	t.Helper()
	ctx := context.Background()
	d := s.Draft
	d.CIPipelineID = ci
	d.CDPipelineID = cd
	var err error
	s, err = m.SaveDraft(ctx, s.ID, s.Version, d)
	if err != nil {
		t.Fatal(err)
	}
	s = onboardingStep(t, m, s, "pipelines")
	inspection, err := m.Inspect(ctx, s.Draft)
	if err != nil {
		t.Fatal(err)
	}
	d = s.Draft
	d.Params = nil
	for _, row := range inspection.Rows {
		p := ob.Parameter{Scope: row.Scope, Name: row.Name, ParamKey: firstNonEmpty(row.MappedKey, row.SuggestedKey, row.NewKey), ValueSource: "release_input"}
		if row.MappedKey == "" && row.SuggestedKey == "" {
			p.NewFieldName = row.Name
		}
		if row.Runtime {
			p.ValueSource = "builtin"
			p.SourceParamKey = defaultJenkinsExecutorParamKey(row.Name)
		}
		d.Params = append(d.Params, p)
	}
	s, err = m.SaveDraft(ctx, s.ID, s.Version, d)
	if err != nil {
		t.Fatal(err)
	}
	s = onboardingStep(t, m, s, "parameters")
	s = onboardingStep(t, m, s, "template_flow")
	return onboardingStep(t, m, s, "review")
}
func TestOnboardingIdentityPersistsOptionalRepositoryURL(t *testing.T) {
	m, _, _ := newOnboardingTestManager(t)
	ctx := context.Background()
	s, err := m.CreateSession(ctx, "owner", "create_application", "", "")
	if err != nil {
		t.Fatal(err)
	}
	d := s.Draft
	d.Identity.Name = "repo-app"
	d.Identity.Key = "repo-app"
	d.Identity.ProjectName = "Project"
	d.Identity.ProjectKey = "project"
	d.RepoURL = " https://git.example.com/team/repository.git "
	s, err = m.SaveDraft(ctx, s.ID, s.Version, d)
	if err != nil {
		t.Fatal(err)
	}
	s = onboardingStep(t, m, s, "identity")
	a, err := m.d.Apps.GetByID(ctx, s.Refs.ApplicationID)
	if err != nil {
		t.Fatal(err)
	}
	if a.RepoURL != "https://git.example.com/team/repository.git" {
		t.Fatalf("application repo URL = %q", a.RepoURL)
	}
	if len(a.ReleaseBranches) != 0 {
		t.Fatalf("repository URL unexpectedly created release branches: %+v", a.ReleaseBranches)
	}
	d = s.Draft
	d.RepoURL = "https://git.example.com/team/repository-v2.git"
	s, err = m.SaveDraft(ctx, s.ID, s.Version, d)
	if err != nil {
		t.Fatal(err)
	}
	s = onboardingStep(t, m, s, "identity")
	a, err = m.d.Apps.GetByID(ctx, s.Refs.ApplicationID)
	if err != nil {
		t.Fatal(err)
	}
	if a.RepoURL != "https://git.example.com/team/repository-v2.git" {
		t.Fatalf("existing application repo URL = %q", a.RepoURL)
	}
}

func TestOnboardingBuiltinBranchDoesNotRequireSavedReleaseBranch(t *testing.T) {
	m, reader, _ := newOnboardingTestManager(t)
	ctx := context.Background()
	now := time.Now().UTC()
	if err := m.d.Fields.Create(ctx, pp.PlatformParamDict{
		ID: "ppd-branch", ParamKey: "branch", Name: "发布分支", ParamType: pp.ParamTypeString,
		Builtin: true, Status: pp.StatusEnabled, CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	reader.jobs["ci"] = []ep.JenkinsParamSnapshot{{Name: "BRANCH", ParamType: ep.ParamTypeString, Required: true}}
	s := onboardingIdentity(t, m, "branch-app", "")
	d := s.Draft
	d.CIPipelineID = "ci"
	var err error
	s, err = m.SaveDraft(ctx, s.ID, s.Version, d)
	if err != nil {
		t.Fatal(err)
	}
	s = onboardingStep(t, m, s, "pipelines")
	d = s.Draft
	d.Params = []ob.Parameter{{Scope: "ci", Name: "BRANCH", ParamKey: "branch", ValueSource: "builtin", SourceParamKey: "branch"}}
	s, err = m.SaveDraft(ctx, s.ID, s.Version, d)
	if err != nil {
		t.Fatal(err)
	}
	s = onboardingStep(t, m, s, "parameters")
	s = onboardingStep(t, m, s, "template_flow")
	s = onboardingStep(t, m, s, "review")
	if s.Check.Status != "ready" {
		t.Fatalf("optional release branch blocked onboarding: %+v", s.Check)
	}
	a, err := m.d.Apps.GetByID(ctx, s.Refs.ApplicationID)
	if err != nil {
		t.Fatal(err)
	}
	if len(a.ReleaseBranches) != 0 {
		t.Fatalf("onboarding invented release branches: %+v", a.ReleaseBranches)
	}
}

func TestOnboardingFirstAndSubsequentApplicationReuse(t *testing.T) {
	m, reader, db := newOnboardingTestManager(t)
	ctx := context.Background()
	first := onboardingConfigure(t, m, onboardingIdentity(t, m, "first-app", ""), "ci", "cd")
	if first.Status != "configured" {
		t.Fatalf("status: %s", first.Status)
	}
	second := onboardingConfigure(t, m, onboardingIdentity(t, m, "second-app", first.Refs.ProjectID), "ci", "cd")
	if first.Refs.ProjectID != second.Refs.ProjectID || first.Refs.ApplicationID == second.Refs.ApplicationID || first.Refs.TemplateID == second.Refs.TemplateID || first.Refs.CIBindingID == second.Refs.CIBindingID || first.Refs.CDBindingID == second.Refs.CDBindingID {
		t.Fatalf("references are not isolated: %+v %+v", first.Refs, second.Refs)
	}
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM platform_param_dict WHERE param_key='deploy_env'`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("field reuse: %d %v", count, err)
	}
	a, err := m.d.Apps.GetByID(ctx, first.Refs.ApplicationID)
	if err != nil {
		t.Fatal(err)
	}
	if a.ArtifactType != "" || a.Language() != "" || a.RepoURL != "" {
		t.Fatal("invented application metadata")
	}
	input := CreateReleaseOrderInput{ApplicationID: first.Refs.ApplicationID, TemplateID: first.Refs.TemplateID, EnvCode: "dev", CreatorUserID: "owner", Params: []CreateReleaseOrderParamInput{{PipelineScope: rel.PipelineScopeCI, ParamKey: "deploy_env", ExecutorParamName: "DEPLOY_ENV", ParamValue: "dev", ValueSource: "release_input"}}}
	first, err = m.CreateFirstRelease(ctx, first.ID, "first-order", first.Version, input)
	if err != nil {
		t.Fatal(err)
	}
	again, err := m.CreateFirstRelease(ctx, first.ID, "first-order", 1, input)
	if err != nil || again.FirstReleaseOrderID != first.FirstReleaseOrderID {
		t.Fatalf("first order replay: %v", err)
	}
	order, err := m.d.Orders.repo.GetByID(ctx, first.FirstReleaseOrderID)
	if err != nil {
		t.Fatal(err)
	}
	if order.Status != rel.OrderStatusPending {
		t.Fatalf("unexpected execution: %s", order.Status)
	}
	if _, err := db.Exec(`UPDATE release_order SET status='deploy_success' WHERE id=?`, order.ID); err != nil {
		t.Fatal(err)
	}
	verified, err := m.Check(ctx, first.ID)
	if err != nil || verified.Status != "verified" {
		t.Fatalf("actual terminal status not recognized: %s %v", verified.Status, err)
	}
	if reader.reads == 0 {
		t.Fatal("live metadata not read")
	}
	reader.jobs["ci"] = append(reader.jobs["ci"], ep.JenkinsParamSnapshot{Name: "NEW_REQUIRED", ParamType: ep.ParamTypeString, Required: true})
	checked, err := m.Check(ctx, second.ID)
	if err != nil || checked.Check.Status == "ready" {
		t.Fatalf("drift accepted: %+v %v", checked.Check, err)
	}
	reader.err = errors.New("connection token=must-not-leak")
	inspection, err := m.Inspect(ctx, second.Draft)
	if err != nil {
		t.Fatal(err)
	}
	if len(inspection.Issues) == 0 || strings.Contains(fmt.Sprint(inspection.Issues), "must-not-leak") {
		t.Fatal("read failure not safely reported")
	}
}
func TestOnboardingIndependentCDAndMissingCIRuntime(t *testing.T) {
	m, reader, _ := newOnboardingTestManager(t)
	ctx := context.Background()
	s := onboardingIdentity(t, m, "cd-app", "")
	d := s.Draft
	d.CDPipelineID = "cd"
	inspection, err := m.Inspect(ctx, d)
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range inspection.Rows {
		d.Params = append(d.Params, ob.Parameter{Scope: "cd", Name: row.Name, ParamKey: row.SuggestedKey, ValueSource: "builtin", SourceParamKey: row.SuggestedKey})
	}
	s.Draft = d
	issues, _ := m.validateParameters(inspection, s)
	if len(issues) == 0 {
		t.Fatal("accepted CD runtime without CI")
	}
	reader.jobs["cd"] = nil
	s.Draft.Params = nil
	result := onboardingConfigure(t, m, s, "", "cd")
	if result.Check.Status != "ready" {
		t.Fatal(result.Check)
	}
}
func TestOnboardingRecoveryDoesNotDuplicateCreatedApplication(t *testing.T) {
	m, _, db := newOnboardingTestManager(t)
	ctx := context.Background()
	s, err := m.CreateSession(ctx, "owner", "create_application", "", "")
	if err != nil {
		t.Fatal(err)
	}
	d := s.Draft
	d.Identity = ob.Identity{Name: "app", Key: "app", ProjectName: "project", ProjectKey: "project", OwnerUserID: "owner"}
	s, err = m.SaveDraft(ctx, s.ID, s.Version, d)
	if err != nil {
		t.Fatal(err)
	}
	unrecorded := s
	if err = m.applyIdentity(ctx, &unrecorded); err != nil {
		t.Fatal(err)
	}
	resumed := onboardingStep(t, m, s, "identity")
	if resumed.Refs.ApplicationID != unrecorded.Refs.ApplicationID {
		t.Fatal("lost reference")
	}
	var count int
	if err = db.QueryRow(`SELECT COUNT(*) FROM applications`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("duplicate app: %d %v", count, err)
	}
}
func TestOnboardingSensitiveTypeCannotEnterDraft(t *testing.T) {
	m, reader, _ := newOnboardingTestManager(t)
	ctx := context.Background()
	reader.jobs["ci"] = []ep.JenkinsParamSnapshot{{Name: "VALUE", ParamType: ep.ParamTypeString, DefaultValue: "never-store-this", RawMeta: `{"_class":"hudson.model.PasswordParameterDefinition","defaultValue":"never-store-this"}`}}
	s := onboardingIdentity(t, m, "safe-app", "")
	d := s.Draft
	d.CIPipelineID = "ci"
	d.Params = []ob.Parameter{{Scope: "ci", Name: "VALUE", FixedValue: "never-store-this"}}
	if _, err := m.SaveDraft(ctx, s.ID, s.Version, d); err == nil {
		t.Fatal("password type persisted")
	}
	row := ResolveOnboardingParamSuggestions("ci", "ci", reader.jobs["ci"][0], ep.ExecutorParamDef{}, nil)
	if !row.Sensitive || row.DefaultValue != "" || row.Live.RawMeta != "" {
		t.Fatal("sensitive metadata not redacted")
	}
}
func TestOnboardingSuggestionsAndFixedValueValidation(t *testing.T) {
	fields := []pp.PlatformParamDict{{ParamKey: "deploy_env", Name: "DEPLOY_ENV", ParamType: pp.ParamTypeString, Status: pp.StatusEnabled}, {ParamKey: "ambiguous", Name: "DEPLOY_ENV", ParamType: pp.ParamTypeString, Status: pp.StatusEnabled}}
	live := ep.JenkinsParamSnapshot{Name: "DEPLOY_ENV", ParamType: ep.ParamTypeChoice, SingleSelect: true, RawMeta: `{"choices":["dev","prod"]}`}
	row := ResolveOnboardingParamSuggestions("ci", "ci", live, ep.ExecutorParamDef{}, fields)
	if row.SuggestedKey != "" || len(row.Candidates) != 2 {
		t.Fatal("ambiguous suggestion applied")
	}
	row = ResolveOnboardingParamSuggestions("ci", "ci", live, ep.ExecutorParamDef{ParamKey: "deploy_env"}, fields)
	if row.SuggestedKey != "deploy_env" || row.Problem != "" {
		t.Fatal("existing mapping not preserved")
	}
	if onboardingValidateValue(row, "other") == nil {
		t.Fatal("invalid choice accepted")
	}
	for _, value := range []string{"NaN", "Inf", "not-a-number"} {
		if onboardingValidateValue(OnboardingParamRow{Name: "COUNT", Type: "number"}, value) == nil {
			t.Fatal("invalid numeric value accepted")
		}
	}
	if onboardingSuggestedKey("123--APP") != "p_123_app" {
		t.Fatal("invalid normalized key")
	}
}

type onboardingFailFlowRepository struct {
	rel.Repository
	calls int
}

func (r *onboardingFailFlowRepository) UpsertApplicationApprovalFlowID(ctx context.Context, appID, flowID string, now time.Time) error {
	r.calls++
	if r.calls == 1 {
		return errors.New("injected flow write failure")
	}
	return r.Repository.UpsertApplicationApprovalFlowID(ctx, appID, flowID, now)
}
func TestOnboardingTemplateRetryFinishesPendingApplicationFlow(t *testing.T) {
	m, reader, _ := newOnboardingTestManager(t)
	ctx := context.Background()
	reader.jobs["ci"] = nil
	s := onboardingIdentity(t, m, "flow-retry", "")
	d := s.Draft
	d.CIPipelineID = "ci"
	d.ChangeApprovalFlow = true
	var err error
	s, err = m.SaveDraft(ctx, s.ID, s.Version, d)
	if err != nil {
		t.Fatal(err)
	}
	s = onboardingStep(t, m, s, "pipelines")
	s = onboardingStep(t, m, s, "parameters")
	repo := &onboardingFailFlowRepository{Repository: m.d.Orders.repo}
	m.d.Orders.repo = repo
	failed, err := m.ApplyStep(ctx, s.ID, "template-flow", "template_flow", s.Version)
	if err == nil || failed.Refs.TemplateID == "" {
		t.Fatalf("expected recoverable partial template: %+v %v", failed, err)
	}
	recovered, err := m.ApplyStep(ctx, s.ID, "template-flow", "template_flow", failed.Version)
	if err != nil || repo.calls != 2 || recovered.Refs.TemplateID != failed.Refs.TemplateID {
		t.Fatalf("flow retry: %d %+v %v", repo.calls, recovered, err)
	}
}

func TestOnboardingImportedTemplateAndAbandonPreserveResources(t *testing.T) {
	m, _, db := newOnboardingTestManager(t)
	ctx := context.Background()
	s := onboardingConfigure(t, m, onboardingIdentity(t, m, "existing", ""), "ci", "cd")
	if _, err := db.Exec(`UPDATE applications SET artifact_type='image', language='typescript', repo_url='https://example.invalid/code' WHERE id=?`, s.Refs.ApplicationID); err != nil {
		t.Fatal(err)
	}
	imported, err := m.CreateSession(ctx, "owner", "complete_application", s.Refs.ApplicationID, "")
	if err != nil {
		t.Fatal(err)
	}
	if imported.Refs.TemplateID != s.Refs.TemplateID || imported.Refs.CIBindingID != s.Refs.CIBindingID {
		t.Fatal("did not reuse existing resources")
	}
	modified := imported.Draft
	modified.TemplateName = "do-not-overwrite"
	if _, err := m.SaveDraft(ctx, imported.ID, imported.Version, modified); err == nil {
		t.Fatal("overwrote existing template draft")
	}
	if _, err := m.Abandon(ctx, imported.ID, imported.Version); err != nil {
		t.Fatal(err)
	}
	a, err := m.d.Apps.GetByID(ctx, s.Refs.ApplicationID)
	if err != nil || a.ArtifactType != "image" || a.Language() != "typescript" || a.RepoURL != "https://example.invalid/code" {
		t.Fatal("import/abandon altered application")
	}
	if _, _, _, _, _, err := m.d.Templates.GetByID(ctx, s.Refs.TemplateID); err != nil {
		t.Fatal("abandon deleted template")
	}
}

type onboardingBrokenTemplateRepository struct {
	rel.Repository
	broken rel.ReleaseTemplate
}

func (r onboardingBrokenTemplateRepository) ListTemplates(ctx context.Context, filter rel.TemplateListFilter) ([]rel.ReleaseTemplate, int64, error) {
	items, total, err := r.Repository.ListTemplates(ctx, filter)
	return append(items, r.broken), total + 1, err
}

func TestOnboardingSetupStatusDoesNotHideUsableTemplate(t *testing.T) {
	m, _, _ := newOnboardingTestManager(t)
	ctx := context.Background()
	s := onboardingConfigure(t, m, onboardingIdentity(t, m, "setup-status", ""), "ci", "cd")
	m.d.Orders.repo = onboardingBrokenTemplateRepository{Repository: m.d.Orders.repo, broken: rel.ReleaseTemplate{ID: "missing", ApplicationID: s.Refs.ApplicationID, Name: "broken"}}
	result, err := m.SetupStatus(ctx, s.Refs.ApplicationID)
	if err != nil || result["status"] != "ready" {
		t.Fatalf("usable template hidden: %+v %v", result, err)
	}
	if len(result["templates"].([]map[string]any)) != 2 {
		t.Fatal("template-specific results missing")
	}
}

func TestOnboardingRuntimeSharedMappingCannotBeRepurposed(t *testing.T) {
	row := ResolveOnboardingParamSuggestions("cd", "cd", ep.JenkinsParamSnapshot{Name: "CI_BUILD", ParamType: ep.ParamTypeString}, ep.ExecutorParamDef{ParamKey: "ordinary_field"}, []pp.PlatformParamDict{{ParamKey: "ordinary_field", ParamType: pp.ParamTypeString, Status: pp.StatusEnabled}})
	if !row.Runtime || row.Problem == "" || row.MappedKey != "ordinary_field" {
		t.Fatalf("runtime shared mapping must be preserved but blocked: %+v", row)
	}
}
