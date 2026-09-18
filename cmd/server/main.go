package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"gos/internal/application/usecase"
	"gos/internal/bootstrap"
	argocddomain "gos/internal/domain/argocdapp"
	gitopsdomain "gos/internal/domain/gitops"
	releasedomain "gos/internal/domain/release"
	aiinfra "gos/internal/infrastructure/ai"
	argocdinfra "gos/internal/infrastructure/argocd"
	configstore "gos/internal/infrastructure/configstore"
	"gos/internal/infrastructure/gitcli"
	gitopsinfra "gos/internal/infrastructure/gitops"
	"gos/internal/infrastructure/jenkins"
	"gos/internal/infrastructure/persistence/sqlrepo"
	httpapi "gos/internal/interfaces/http"
	"gos/internal/support/secure"
)

//go:generate swag init -g cmd/server/main.go -o docs --parseInternal

// @title           GOS Release API
// @version         1.0
// @description     Internal deployment platform API.
// @BasePath        /
// @schemes         http https
func main() {
	configPath := flag.String("config", "configs/config.local.json", "server config file path")
	flag.Parse()

	resolvedConfigPath := bootstrap.ResolveConfigPath(*configPath)

	cfg, err := bootstrap.LoadConfigFromPath(resolvedConfigPath)
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	listener, err := net.Listen("tcp", cfg.Server.Addr)
	if err != nil {
		log.Fatalf("listen on %s: %v", cfg.Server.Addr, err)
	}

	secure.SetSecretKey(cfg.Security.EncryptionKey)
	if err := bootstrap.CheckJenkinsConnection(cfg); err != nil {
		log.Fatalf("check jenkins: %v", err)
	}
	if err := bootstrap.CheckArgoCDConnection(cfg); err != nil {
		log.Fatalf("check argocd: %v", err)
	}

	db, err := bootstrap.OpenDatabase(cfg)
	if err != nil {
		log.Fatalf("open database: %v", err)
	}
	defer func() { _ = db.Close() }()

	projectRepo := sqlrepo.NewProjectRepository(db, cfg.Database.Driver)
	if err := bootstrap.InitSchema(projectRepo); err != nil {
		log.Fatalf("init project schema: %v", err)
	}
	repo := sqlrepo.NewApplicationRepository(db, cfg.Database.Driver)
	if err := bootstrap.InitSchema(repo); err != nil {
		log.Fatalf("init schema: %v", err)
	}

	pipelineRepo := sqlrepo.NewPipelineRepository(db, cfg.Database.Driver)
	if err := bootstrap.InitSchema(pipelineRepo); err != nil {
		log.Fatalf("init pipeline schema: %v", err)
	}
	pipelineScanRepo := sqlrepo.NewPipelineScanRepository(db, cfg.Database.Driver)
	if err := bootstrap.InitSchema(pipelineScanRepo); err != nil {
		log.Fatalf("init pipeline scan schema: %v", err)
	}

	platformParamRepo := sqlrepo.NewPlatformParamRepository(db, cfg.Database.Driver)
	if err := bootstrap.InitSchema(platformParamRepo); err != nil {
		log.Fatalf("init platform param schema: %v", err)
	}
	artifactRepositoryRepo := sqlrepo.NewArtifactRepositoryConfigRepository(db, cfg.Database.Driver)
	if err := bootstrap.InitSchema(artifactRepositoryRepo); err != nil {
		log.Fatalf("init artifact repository schema: %v", err)
	}
	gitCredentialRepo := sqlrepo.NewGitCredentialRepository(db, cfg.Database.Driver)
	if err := bootstrap.InitSchema(gitCredentialRepo); err != nil {
		log.Fatalf("init git credential schema: %v", err)
	}

	executorParamRepo := sqlrepo.NewExecutorParamRepository(db, cfg.Database.Driver)
	if err := bootstrap.InitSchema(executorParamRepo); err != nil {
		log.Fatalf("init executor param schema: %v", err)
	}
	agentRepo := sqlrepo.NewAgentRepository(db, cfg.Database.Driver)
	if err := bootstrap.InitSchema(agentRepo); err != nil {
		log.Fatalf("init agent schema: %v", err)
	}
	userRepo := sqlrepo.NewUserRepository(db, cfg.Database.Driver)
	if err := bootstrap.InitSchema(userRepo); err != nil {
		log.Fatalf("init user schema: %v", err)
	}
	releaseRepo := sqlrepo.NewReleaseRepository(db, cfg.Database.Driver)
	if err := bootstrap.InitSchema(releaseRepo); err != nil {
		log.Fatalf("init release schema: %v", err)
	}
	argocdAppRepo := sqlrepo.NewArgoCDApplicationRepository(db, cfg.Database.Driver)
	if err := bootstrap.InitSchema(argocdAppRepo); err != nil {
		log.Fatalf("init argocd schema: %v", err)
	}
	gitopsRepo := sqlrepo.NewGitOpsRepository(db, cfg.Database.Driver)
	if err := bootstrap.InitSchema(gitopsRepo); err != nil {
		log.Fatalf("init gitops schema: %v", err)
	}
	notificationRepo := sqlrepo.NewNotificationRepository(db, cfg.Database.Driver)
	if err := bootstrap.InitSchema(notificationRepo); err != nil {
		log.Fatalf("init notification schema: %v", err)
	}
	announcementRepo := sqlrepo.NewAnnouncementRepository(db, cfg.Database.Driver)
	if err := bootstrap.InitSchema(announcementRepo); err != nil {
		log.Fatalf("init announcement schema: %v", err)
	}
	releaseStoreFallback := configstore.NewReleaseStore(resolvedConfigPath)
	releaseStore := configstore.NewDatabaseReleaseStore(db, cfg.Database.Driver, releaseStoreFallback)
	if err := bootstrap.InitSchema(releaseStore); err != nil {
		log.Fatalf("init release settings schema: %v", err)
	}
	aiModelConfigRepo := sqlrepo.NewAIModelConfigRepository(db, cfg.Database.Driver)
	if err := bootstrap.InitSchema(aiModelConfigRepo); err != nil {
		log.Fatalf("init ai model config schema: %v", err)
	}
	stageDiagnosisRepo := sqlrepo.NewStageDiagnosisRepository(db, cfg.Database.Driver)
	if err := bootstrap.InitSchema(stageDiagnosisRepo); err != nil {
		log.Fatalf("init stage diagnosis schema: %v", err)
	}
	if err := argocdAppRepo.CleanupLegacyApplications(context.Background()); err != nil {
		log.Fatalf("cleanup legacy argocd applications: %v", err)
	}

	jenkinsClient := jenkins.NewClient(jenkins.Config{
		BaseURL:    cfg.Jenkins.BaseURL,
		Username:   cfg.Jenkins.Username,
		APIToken:   cfg.Jenkins.APIToken,
		TimeoutSec: cfg.Jenkins.TimeoutSec,
	})
	gitopsServiceFactory := gitOpsServiceFactory{}
	argocdClientFactory := argoCDClientFactory{}
	syncPipelines := usecase.NewSyncPipelines(pipelineRepo, jenkinsClient)
	pipelineScanManager := usecase.NewPipelineScanManager(pipelineScanRepo, pipelineRepo, jenkinsClient)
	syncPipelines.SetScanHook(pipelineScanManager)
	syncExecutorParamDefs := usecase.NewSyncExecutorParamDefs(executorParamRepo, jenkinsClient)
	syncArgoCDApplications := usecase.NewSyncArgoCDApplications(argocdAppRepo, argocdClientFactory)
	gitopsInstanceManager := usecase.NewGitOpsInstanceManager(gitopsRepo, gitopsServiceFactory, platformParamRepo)
	argocdInstanceManager := usecase.NewArgoCDInstanceManager(argocdAppRepo, gitopsRepo, argocdClientFactory)
	userManagement := usecase.NewUserManagement(userRepo)
	authSessionManager := usecase.NewAuthSessionManager(
		userRepo,
		releaseStore,
		time.Duration(cfg.Auth.SessionTTLHours)*time.Hour,
	)
	if err := userManagement.EnsureSeedData(
		context.Background(),
		cfg.Auth.AdminUsername,
		cfg.Auth.AdminDisplayName,
		cfg.Auth.AdminPassword,
	); err != nil {
		log.Fatalf("ensure auth seed data: %v", err)
	}

	authHandler := httpapi.NewAuthHandler(authSessionManager, userManagement)
	agentHandler := httpapi.NewAgentHandler(
		usecase.NewAgentManager(agentRepo),
		usecase.NewAgentTaskManager(agentRepo),
		usecase.NewAgentScriptManager(agentRepo),
		authSessionManager,
	)
	userHandler := httpapi.NewUserHandler(userManagement, authSessionManager)
	handler := httpapi.NewApplicationHandler(
		usecase.NewCreateApplication(repo, projectRepo),
		usecase.NewQueryApplication(repo),
		usecase.NewUpdateApplication(repo, projectRepo),
		usecase.NewDeleteApplication(repo),
		userManagement,
		authSessionManager,
	)
	projectHandler := httpapi.NewProjectHandler(
		usecase.NewProjectManager(projectRepo),
		authSessionManager,
	)
	releaseSettingsQuery := usecase.NewQueryReleaseSettings(releaseStore)
	systemManagementSettingsQuery := usecase.NewQuerySystemManagementSettings(releaseStore)
	aiClientFactory := aiinfra.NewOpenAICompatibleClientFactory()
	aiModelConfigManager := usecase.NewAIModelConfigManager(aiModelConfigRepo)
	systemSettingsHandler := httpapi.NewSystemSettingsHandler(
		releaseSettingsQuery,
		usecase.NewUpdateReleaseSettings(
			releaseStore,
			releaseSettingsQuery,
		),
		systemManagementSettingsQuery,
		usecase.NewUpdateSystemManagementSettings(
			releaseStore,
			systemManagementSettingsQuery,
		),
		authSessionManager,
	)
	aiModelConfigHandler := httpapi.NewAIModelConfigHandler(
		aiModelConfigManager,
		aiClientFactory,
		authSessionManager,
	)
	pipelineHandler := httpapi.NewPipelineHandler(
		syncPipelines,
		usecase.NewQueryPipeline(pipelineRepo, jenkinsClient),
		usecase.NewPipelineBindingManager(pipelineRepo, repo),
		usecase.NewJenkinsPipelineManager(pipelineRepo, jenkinsClient, syncPipelines, syncExecutorParamDefs),
		authSessionManager,
	)
	pipelineScanHandler := httpapi.NewPipelineScanHandler(pipelineScanManager, authSessionManager)
	argocdHandler := httpapi.NewArgoCDHandler(
		syncArgoCDApplications,
		usecase.NewQueryArgoCDApplications(argocdAppRepo),
		argocdInstanceManager,
		authSessionManager,
	)
	gitopsHandler := httpapi.NewGitOpsHandler(
		usecase.NewQueryGitOpsTemplateFields(platformParamRepo),
		usecase.NewQueryGitOpsFieldCandidates(repo, gitopsInstanceManager),
		usecase.NewQueryGitOpsValuesCandidates(repo, gitopsInstanceManager),
		usecase.NewQueryGitOpsScanPathStatus(repo, gitopsInstanceManager),
		gitopsInstanceManager,
		authSessionManager,
	)
	platformParamHandler := httpapi.NewPlatformParamHandler(
		usecase.NewPlatformParamDictManager(platformParamRepo, executorParamRepo),
		authSessionManager,
	)
	artifactRepositoryHandler := httpapi.NewArtifactRepositoryHandler(
		usecase.NewArtifactRepositoryManager(artifactRepositoryRepo),
		authSessionManager,
	)
	gitCommitManager := usecase.NewGitCommitManager(releaseRepo, repo, gitCredentialRepo)
	// 「发布单最近提交」里 auth_type=password 的凭证走 git 协议读取：目标 GitLab 的
	// API v4 对 Basic 账号密码和「PRIVATE-TOKEN: 密码」都返回 401，但同一账号可以正常
	// clone/fetch 仓库。镜像缓存目录使用默认值 os.TempDir()/gos-git-cache。
	gitCommitManager.SetGitCLIClientFactory(func(cfg gitcli.Config) usecase.GitCommitFetcher {
		return gitcli.NewClient(cfg)
	})
	gitCredentialHandler := httpapi.NewGitCredentialHandler(
		usecase.NewGitCredentialManager(gitCredentialRepo),
		gitCommitManager,
		authSessionManager,
	)
	notificationHandler := httpapi.NewNotificationHandler(
		usecase.NewNotificationManager(notificationRepo, platformParamRepo),
		authSessionManager,
	)
	announcementHandler := httpapi.NewAnnouncementHandler(
		usecase.NewAnnouncementManager(announcementRepo),
		authSessionManager,
	)
	executorParamHandler := httpapi.NewExecutorParamHandler(
		usecase.NewExecutorParamDefManager(executorParamRepo, repo, pipelineRepo, platformParamRepo),
		syncExecutorParamDefs,
		authSessionManager,
		authSessionManager,
	)
	releaseOrderManager := usecase.NewReleaseOrderManager(
		releaseRepo,
		repo,
		pipelineRepo,
		executorParamRepo,
		platformParamRepo,
		releaseStore,
		jenkinsClient,
		agentRepo,
		argocdAppRepo,
		notificationRepo,
		argocdClientFactory,
		gitopsRepo,
		gitopsServiceFactory,
		nil,
	)
	handler.SetWorkbenchQuery(usecase.NewApplicationWorkbench(repo, releaseRepo))
	handler.SetApprovalFlowManager(releaseOrderManager)
	releaseTemplateManager := usecase.NewReleaseTemplateManager(releaseRepo, repo, pipelineRepo, executorParamRepo, platformParamRepo, argocdAppRepo, agentRepo, notificationRepo, gitopsInstanceManager)
	releaseOrderManager.SetArtifactRepository(artifactRepositoryRepo)
	releaseOrderManager.SetSystemManagementSettingsStore(releaseStore)
	releaseOrderManager.SetApprovalManagerResolver(userRepo)
	releaseOrderManager.SetPipelineScanRepository(pipelineScanRepo)
	releaseOrderManager.SetAIModelRepository(aiModelConfigRepo)
	releaseOrderManager.SetStageDiagnosisRepository(stageDiagnosisRepo)
	releaseOrderManager.SetAIClientFactory(aiClientFactory)
	releaseTemplateManager.SetPipelineScanRepository(pipelineScanRepo)
	onboardingRepo := sqlrepo.NewOnboardingRepository(db, cfg.Database.Driver)
	if err := bootstrap.InitSchema(onboardingRepo); err != nil {
		log.Fatalf("init onboarding schema: %v", err)
	}
	onboardingManager := usecase.NewOnboardingManager(onboardingRepo, usecase.OnboardingDependencies{
		Apps: repo, Projects: projectRepo, Pipelines: pipelineRepo, Params: executorParamRepo,
		Fields: platformParamRepo, Orders: releaseOrderManager, Templates: releaseTemplateManager,
		Jenkins: jenkinsClient, Users: userManagement, Settings: releaseSettingsQuery, JenkinsEnabled: cfg.Jenkins.Enabled,
	})
	handler.SetOnboardingHandler(httpapi.NewOnboardingHandler(onboardingManager, authSessionManager))
	releaseOrderLogStreamer := usecase.NewReleaseOrderLogStreamer(releaseRepo, pipelineRepo, jenkinsClient)
	releaseOrderHandler := httpapi.NewReleaseOrderHandler(
		releaseOrderManager,
		releaseOrderLogStreamer,
		authSessionManager,
		authSessionManager,
	)
	releaseTemplateHandler := httpapi.NewReleaseTemplateHandler(
		releaseTemplateManager,
		authSessionManager,
	)
	releaseTemplateHandler.SetSyncer(usecase.NewSyncTemplatePipelineParams(releaseRepo, pipelineRepo, executorParamRepo, jenkinsClient))

	releaseTracker := usecase.NewTrackReleaseExecution(
		releaseOrderManager,
		jenkinsClient,
	)
	releaseOrderHandler.SetRealtimeSynchronizer(releaseTracker)

	syncTask := bootstrap.StartJenkinsAutoSyncTask(cfg.Jenkins, func(ctx context.Context) error {
		var (
			pipelineResult usecase.SyncPipelinesOutput
			paramResult    usecase.SyncExecutorParamDefsOutput
			pipelineErr    error
			paramErr       error
			wg             sync.WaitGroup
		)

		wg.Add(2)
		go func() {
			defer wg.Done()
			pipelineResult, pipelineErr = syncPipelines.Execute(ctx)
		}()
		go func() {
			defer wg.Done()
			paramResult, paramErr = syncExecutorParamDefs.Execute(ctx)
		}()
		wg.Wait()

		if pipelineErr != nil {
			return fmt.Errorf("sync pipelines: %w", pipelineErr)
		}
		log.Printf(
			"jenkins auto sync completed: total=%d created=%d updated=%d inactivated=%d skipped=%d",
			pipelineResult.Total,
			pipelineResult.Created,
			pipelineResult.Updated,
			pipelineResult.Inactivated,
			pipelineResult.Skipped,
		)

		if paramErr != nil {
			return fmt.Errorf("sync param defs: %w", paramErr)
		}
		log.Printf(
			"jenkins param auto sync completed: total=%d created=%d updated=%d inactivated=%d skipped=%d",
			paramResult.Total,
			paramResult.Created,
			paramResult.Updated,
			paramResult.Inactivated,
			paramResult.Skipped,
		)
		return nil
	})
	defer syncTask.Stop()

	argocdSyncTask := bootstrap.StartArgoCDAutoSyncTask(cfg.ArgoCD.SyncIntervalSec, func(ctx context.Context) error {
		result, err := syncArgoCDApplications.Execute(ctx)
		if err != nil {
			return err
		}
		log.Printf(
			"argocd auto sync completed: total=%d created=%d updated=%d inactivated=%d",
			result.Total,
			result.Created,
			result.Updated,
			result.Inactivated,
		)
		return nil
	})
	defer argocdSyncTask.Stop()

	releaseTrackTask := bootstrap.StartReleaseTrackTask(cfg.Jenkins.ReleaseTrackIntervalSec, func(ctx context.Context) error {
		releaseResult, err := releaseTracker.Execute(ctx)
		if err != nil {
			return err
		}
		log.Printf(
			"release execution track completed: running=%d updated=%d skipped=%d failed=%d",
			releaseResult.RunningOrders,
			releaseResult.UpdatedOrders,
			releaseResult.SkippedOrders,
			releaseResult.FailedOrders,
		)
		return nil
	})
	defer releaseTrackTask.Stop()

	releaseScheduleTask := bootstrap.StartReleaseScheduleTask(10, func(ctx context.Context) error {
		result, err := releaseOrderManager.RunDueSchedules(ctx, 50)
		if err != nil {
			return err
		}
		if result.Scanned > 0 {
			log.Printf(
				"release schedule dispatch completed: scanned=%d expired=%d dispatched=%d blocked=%d failed=%d skipped=%d",
				result.Scanned,
				result.Expired,
				result.Dispatched,
				result.Blocked,
				result.Failed,
				result.Skipped,
			)
		}
		return nil
	})
	defer releaseScheduleTask.Stop()

	router := httpapi.NewRouter(
		authHandler,
		agentHandler,
		userHandler,
		authSessionManager,
		handler,
		projectHandler,
		systemSettingsHandler,
		aiModelConfigHandler,
		pipelineHandler,
		pipelineScanHandler,
		argocdHandler,
		gitopsHandler,
		artifactRepositoryHandler,
		gitCredentialHandler,
		platformParamHandler,
		notificationHandler,
		executorParamHandler,
		releaseOrderHandler,
		releaseTemplateHandler,
		announcementHandler,
	)

	server := &http.Server{
		Addr:              cfg.Server.Addr,
		Handler:           router,
		ReadHeaderTimeout: time.Duration(cfg.Server.ReadHeaderTimeoutSec) * time.Second,
		ReadTimeout:       time.Duration(cfg.Server.ReadTimeoutSec) * time.Second,
		WriteTimeout:      time.Duration(cfg.Server.WriteTimeoutSec) * time.Second,
		IdleTimeout:       time.Duration(cfg.Server.IdleTimeoutSec) * time.Second,
	}

	serverErr := make(chan error, 1)
	go func() {
		serverErr <- server.Serve(listener)
	}()
	// 冷启动预热：进程刚启动时 git 镜像缓存（os.TempDir()/gos-git-cache）是空的，
	// 列表的第一个请求要为每个仓库冷克隆（大仓库可达一分钟），超过前端 60s 超时后
	// 该列会空白。后台先把最新发布单涉及的仓库拉一遍，用户打开列表即可命中热缓存。
	go func() {
		time.Sleep(5 * time.Second)
		warmCtx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()
		warmReleaseOrderCommits(warmCtx, releaseRepo, gitCommitManager)
	}()
	log.Printf(
		"server listening on %s (env=%s db=%s jenkins_enabled=%t)",
		cfg.Server.Addr,
		cfg.Environment,
		cfg.Database.Driver,
		cfg.Jenkins.Enabled,
	)

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	select {
	case sig := <-stop:
		log.Printf("received signal %s, shutting down", sig)
	case err := <-serverErr:
		if errors.Is(err, http.ErrServerClosed) {
			return
		}
		log.Fatalf("server stopped with error: %v", err)
	}

	shutdownCtx, cancel := contextWithTimeout(time.Duration(cfg.Server.ShutdownTimeoutSec) * time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("graceful shutdown failed: %v", err)
	}

	err = <-serverErr
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Printf("server close error: %v", err)
	}
}

// contextWithTimeout 封装当前模块的业务处理逻辑。
func contextWithTimeout(timeout time.Duration) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	return ctx, cancel
}

type argoCDClientFactory struct{}

type gitOpsServiceFactory struct{}

// Build 组装业务执行所需的输入数据。
func (argoCDClientFactory) Build(instance argocddomain.Instance) usecase.ArgoCDApplicationClient {
	client := argocdinfra.NewClient(argocdinfra.Config{
		BaseURL:            strings.TrimSpace(instance.BaseURL),
		InsecureSkipVerify: instance.InsecureSkipVerify,
		AuthMode:           strings.TrimSpace(instance.AuthMode),
		Token:              strings.TrimSpace(instance.Token),
		Username:           strings.TrimSpace(instance.Username),
		Password:           strings.TrimSpace(instance.Password),
		TimeoutSec:         30,
	})
	return argoCDUsecaseClient{client: client}
}

// Build 组装业务执行所需的输入数据。
func (gitOpsServiceFactory) Build(instance gitopsdomain.Instance) *gitopsinfra.Service {
	return gitopsinfra.NewService(gitopsinfra.Config{
		Enabled:               instance.Status == gitopsdomain.StatusActive && strings.TrimSpace(instance.LocalRoot) != "",
		LocalRoot:             strings.TrimSpace(instance.LocalRoot),
		DefaultBranch:         strings.TrimSpace(instance.DefaultBranch),
		Username:              strings.TrimSpace(instance.Username),
		Password:              strings.TrimSpace(instance.Password),
		Token:                 strings.TrimSpace(instance.Token),
		AuthorName:            strings.TrimSpace(instance.AuthorName),
		AuthorEmail:           strings.TrimSpace(instance.AuthorEmail),
		CommitMessageTemplate: strings.TrimSpace(instance.CommitMessageTemplate),
		CommandTimeoutSec:     instance.CommandTimeoutSec,
	})
}

// ensureDefaultGitOpsInstance 校验前置条件，不满足时写入对应错误响应。
func ensureDefaultGitOpsInstance(ctx context.Context, repo gitopsdomain.Repository, cfg bootstrap.Config) error {
	if repo == nil || !cfg.GitOps.Enabled || strings.TrimSpace(cfg.GitOps.LocalRoot) == "" {
		return nil
	}
	now := time.Now().UTC()
	_, err := repo.UpsertInstance(ctx, gitopsdomain.Instance{
		ID:                    "gitops-instance-default",
		InstanceCode:          "default",
		Name:                  "默认 GitOps",
		LocalRoot:             strings.TrimSpace(cfg.GitOps.LocalRoot),
		DefaultBranch:         strings.TrimSpace(cfg.GitOps.DefaultBranch),
		Username:              strings.TrimSpace(cfg.GitOps.Username),
		Password:              strings.TrimSpace(cfg.GitOps.Password),
		Token:                 strings.TrimSpace(cfg.GitOps.Token),
		AuthorName:            strings.TrimSpace(cfg.GitOps.AuthorName),
		AuthorEmail:           strings.TrimSpace(cfg.GitOps.AuthorEmail),
		CommitMessageTemplate: strings.TrimSpace(cfg.GitOps.CommitMessageTemplate),
		CommandTimeoutSec:     cfg.GitOps.CommandTimeoutSec,
		Status:                gitopsdomain.StatusActive,
		Remark:                "由配置文件自动同步的默认 GitOps 实例",
		CreatedAt:             now,
		UpdatedAt:             now,
	})
	return err
}

// ensureDefaultArgoCDInstance 校验前置条件，不满足时写入对应错误响应。
func ensureDefaultArgoCDInstance(ctx context.Context, repo argocddomain.Repository, cfg bootstrap.Config) error {
	if repo == nil || !cfg.ArgoCD.Enabled || strings.TrimSpace(cfg.ArgoCD.BaseURL) == "" {
		return nil
	}
	now := time.Now().UTC()
	_, err := repo.UpsertInstance(ctx, argocddomain.Instance{
		ID:                 "argocd-instance-default",
		InstanceCode:       "default",
		Name:               "默认 ArgoCD",
		BaseURL:            strings.TrimSpace(cfg.ArgoCD.BaseURL),
		InsecureSkipVerify: cfg.ArgoCD.InsecureSkipVerify,
		AuthMode:           strings.TrimSpace(cfg.ArgoCD.AuthMode),
		Token:              strings.TrimSpace(cfg.ArgoCD.Token),
		Username:           strings.TrimSpace(cfg.ArgoCD.Username),
		Password:           strings.TrimSpace(cfg.ArgoCD.Password),
		GitOpsInstanceID:   defaultGitOpsInstanceID(cfg),
		ClusterName:        "default",
		DefaultNamespace:   "",
		Status:             argocddomain.StatusActive,
		HealthStatus:       "unknown",
		CreatedAt:          now,
		UpdatedAt:          now,
		Remark:             "由配置文件自动同步的默认 ArgoCD 实例",
	})
	return err
}

// defaultGitOpsInstanceID 封装当前模块的业务处理逻辑。
func defaultGitOpsInstanceID(cfg bootstrap.Config) string {
	if cfg.GitOps.Enabled && strings.TrimSpace(cfg.GitOps.LocalRoot) != "" {
		return "gitops-instance-default"
	}
	return ""
}

type argoCDUsecaseClient struct {
	client *argocdinfra.Client
}

// Ping 封装当前模块的业务处理逻辑。
func (c argoCDUsecaseClient) Ping(ctx context.Context) error {
	if c.client == nil {
		return errors.New("argocd client is not configured")
	}
	return c.client.Ping(ctx)
}

// ListApplications 查询并返回列表数据。
func (c argoCDUsecaseClient) ListApplications(ctx context.Context) ([]usecase.ArgoCDApplicationSnapshot, error) {
	if c.client == nil {
		return nil, errors.New("argocd client is not configured")
	}
	items, err := c.client.ListApplications(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]usecase.ArgoCDApplicationSnapshot, 0, len(items))
	for _, item := range items {
		result = append(result, item)
	}
	return result, nil
}

// GetApplication 查询并返回指定资源数据。
func (c argoCDUsecaseClient) GetApplication(ctx context.Context, name string) (usecase.ArgoCDApplicationSnapshot, error) {
	if c.client == nil {
		return nil, errors.New("argocd client is not configured")
	}
	return c.client.GetApplication(ctx, name)
}

// SyncApplication 同步外部或内部状态数据。
func (c argoCDUsecaseClient) SyncApplication(ctx context.Context, name string) error {
	if c.client == nil {
		return errors.New("argocd client is not configured")
	}
	return c.client.SyncApplication(ctx, name)
}

// SyncApplicationWithRevision 同步外部或内部状态数据。
func (c argoCDUsecaseClient) SyncApplicationWithRevision(ctx context.Context, name string, revision string) error {
	if c.client == nil {
		return errors.New("argocd client is not configured")
	}
	return c.client.SyncApplicationWithRevision(ctx, name, revision)
}

// BuildApplicationURL 组装业务执行所需的输入数据。
func (c argoCDUsecaseClient) BuildApplicationURL(name string) string {
	if c.client == nil {
		return ""
	}
	return c.client.BuildApplicationURL(name)
}

// warmReleaseOrderCommits 预热「最近提交」：把最新若干发布单涉及的仓库先拉一遍。
// 进程重启后镜像缓存为空，首次列表请求若为每个仓库冷克隆，很容易超过前端的请求超时；
// 预热只在后台跑一次（最多 10 分钟），失败只记录日志，不影响任何接口。
func warmReleaseOrderCommits(
	ctx context.Context,
	releaseRepo releasedomain.Repository,
	commits *usecase.GitCommitManager,
) {
	if releaseRepo == nil || commits == nil {
		return
	}
	const warmOrderLimit = 20
	orders, _, err := releaseRepo.List(ctx, releasedomain.ListFilter{Page: 1, PageSize: warmOrderLimit})
	if err != nil {
		log.Printf("warm recent commits: list orders failed: %v", err)
		return
	}
	orderIDs := make([]string, 0, len(orders))
	for _, item := range orders {
		orderIDs = append(orderIDs, item.ID)
	}
	if len(orderIDs) == 0 {
		return
	}
	startedAt := time.Now()
	result, err := commits.RecentCommits(ctx, usecase.GitCommitQuery{OrderIDs: orderIDs, Limit: 5})
	if err != nil {
		log.Printf("warm recent commits: failed: %v", err)
		return
	}
	resolved := 0
	failed := 0
	for _, item := range result {
		if item.Error != "" {
			failed++
			continue
		}
		resolved++
	}
	log.Printf(
		"warm recent commits: orders=%d resolved=%d failed=%d elapsed=%s",
		len(orderIDs),
		resolved,
		failed,
		time.Since(startedAt).Truncate(time.Millisecond),
	)
}
