package gitops

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	gitopsdomain "gos/internal/domain/gitops"
	"gos/internal/support/logx"

	yaml "gopkg.in/yaml.v2"
)

// Config 描述平台在 GitOps 模式下操作声明式仓库所需的最小运行时参数。
//
// 这里刻意不做“通用 Git 客户端”，而是只支持受控的 clone / pull / commit / push：
// 1. local_root 既可以是工作副本根目录，也可以直接就是目标 GitOps 仓库；
// 2. default_branch 用于 ArgoCD Application 配置为 HEAD 时的回退分支；
// 3. author_name / author_email 固定平台提交身份，方便审计；
// 4. username/password 或 token 仅用于远端仓库认证注入。
type Config struct {
	Enabled               bool
	LocalRoot             string
	DefaultBranch         string
	Username              string
	Password              string
	Token                 string
	AuthorName            string
	AuthorEmail           string
	CommitMessageTemplate string
	CommandTimeoutSec     int
	HelmScanPath          string
	KustomizeScanPath     string
}

type Service struct {
	mu                    sync.RWMutex
	enabled               bool
	localRoot             string
	defaultBranch         string
	username              string
	password              string
	token                 string
	authorName            string
	authorEmail           string
	commitMessageTemplate string
	commandTimeoutSec     int
	helmScanPath          string
	kustomizeScanPath     string
}

type Status struct {
	Enabled               bool
	LocalRoot             string
	Mode                  string
	DefaultBranch         string
	Username              string
	AuthorName            string
	AuthorEmail           string
	CommitMessageTemplate string
	CommandTimeoutSec     int
	PathExists            bool
	IsGitRepo             bool
	RemoteOrigin          string
	RemoteReachable       bool
	CurrentBranch         string
	HeadCommit            string
	HeadCommitSubject     string
	WorktreeDirty         bool
	StatusSummary         []string
}

type BindingTarget struct {
	Path                  string
	AppDirectory          string
	DisplayName           string
	HierarchyHint         string
	AvailableEnvironments []string
}

const defaultCommitMessageTemplate = "chore(release): {app_key}/{project_name}/{env} -> {image_version} ({branch})"
const configCandidateScanBranch = "master"

var commitTemplateTokenPattern = regexp.MustCompile(`\{([a-zA-Z0-9_]+)\}`)
var repoBranchLocks sync.Map

// NewService 创建并返回对应组件实例。
func NewService(cfg Config) *Service {
	helmPath := strings.TrimSpace(cfg.HelmScanPath)
	if helmPath == "" {
		helmPath = "apps/helm"
	}
	kustomizePath := strings.TrimSpace(cfg.KustomizeScanPath)
	if kustomizePath == "" {
		kustomizePath = "apps/{app_key}/overlays/{env}"
	}
	return &Service{
		enabled:               cfg.Enabled,
		localRoot:             strings.TrimSpace(cfg.LocalRoot),
		defaultBranch:         strings.TrimSpace(cfg.DefaultBranch),
		username:              strings.TrimSpace(cfg.Username),
		password:              strings.TrimSpace(cfg.Password),
		token:                 strings.TrimSpace(cfg.Token),
		authorName:            strings.TrimSpace(cfg.AuthorName),
		authorEmail:           strings.TrimSpace(cfg.AuthorEmail),
		commitMessageTemplate: NormalizeCommitMessageTemplate(cfg.CommitMessageTemplate),
		commandTimeoutSec:     cfg.CommandTimeoutSec,
		helmScanPath:          strings.TrimRight(helmPath, "/"),
		kustomizeScanPath:     strings.TrimRight(kustomizePath, "/"),
	}
}

// Enabled 封装当前模块的业务处理逻辑。
func (s *Service) Enabled() bool {
	return s != nil && s.enabled && s.localRoot != ""
}

// DefaultCommitMessageTemplate 查询并返回指定资源数据。
func DefaultCommitMessageTemplate() string {
	return defaultCommitMessageTemplate
}

// NormalizeCommitMessageTemplate 标准化输入值，保证后续逻辑使用统一格式。
func NormalizeCommitMessageTemplate(candidate string) string {
	candidate = strings.TrimSpace(candidate)
	if candidate == "" {
		return defaultCommitMessageTemplate
	}
	return candidate
}

// currentCommitMessageTemplate 查询并返回指定资源数据。
func (s *Service) currentCommitMessageTemplate() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return strings.TrimSpace(s.commitMessageTemplate)
}

// UpdateCommitMessageTemplate 查询并返回指定资源数据。
func (s *Service) UpdateCommitMessageTemplate(template string) string {
	normalized := NormalizeCommitMessageTemplate(template)
	s.mu.Lock()
	s.commitMessageTemplate = normalized
	s.mu.Unlock()
	return normalized
}

// UpdateScanPaths 更新业务资源并返回处理结果。
func (s *Service) UpdateScanPaths(helmPath string, kustomizePath string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	helmPath = strings.TrimSpace(helmPath)
	if helmPath == "" {
		helmPath = "apps/helm"
	}
	kustomizePath = strings.TrimSpace(kustomizePath)
	if kustomizePath == "" {
		kustomizePath = "apps/{app_key}/overlays/{env}"
	}
	s.helmScanPath = strings.TrimRight(helmPath, "/")
	s.kustomizeScanPath = strings.TrimRight(kustomizePath, "/")
}

// helmScanPathValue 封装当前模块的业务处理逻辑。
func (s *Service) helmScanPathValue() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.helmScanPath
}

// kustomizeScanPathValue 封装当前模块的业务处理逻辑。
func (s *Service) kustomizeScanPathValue() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.kustomizeScanPath
}

// GetStatus 返回 GitOps 工作目录的当前可见状态，供组件管理页展示。
//
// 这里故意返回“尽量完整的快照”而不是遇错即失败：
// 1. local_root 不存在时，前端也应该能看到配置摘要；
// 2. 目录存在但不是 Git 仓库时，应明确告诉用户当前状态；
// 3. 远端可达性单独探测，方便定位是本地目录问题还是 Git 凭据/网络问题。
func (s *Service) GetStatus(ctx context.Context) (Status, error) {
	status := Status{
		Enabled:               s.Enabled(),
		LocalRoot:             strings.TrimSpace(s.localRoot),
		Mode:                  "workspace_root",
		DefaultBranch:         strings.TrimSpace(s.defaultBranch),
		Username:              strings.TrimSpace(s.username),
		AuthorName:            strings.TrimSpace(s.authorName),
		AuthorEmail:           strings.TrimSpace(s.authorEmail),
		CommitMessageTemplate: s.currentCommitMessageTemplate(),
		CommandTimeoutSec:     s.commandTimeoutSec,
	}
	if !status.Enabled {
		return status, nil
	}

	if _, err := os.Stat(status.LocalRoot); err == nil {
		status.PathExists = true
	} else if os.IsNotExist(err) {
		return status, nil
	} else {
		return status, err
	}

	if _, err := os.Stat(filepath.Join(status.LocalRoot, ".git")); err != nil {
		if os.IsNotExist(err) {
			return status, nil
		}
		return status, err
	}

	status.IsGitRepo = true
	status.Mode = "direct_repo"

	remoteOrigin, err := s.remoteOriginURL(ctx, status.LocalRoot)
	if err == nil {
		status.RemoteOrigin = strings.TrimSpace(remoteOrigin)
	} else {
		status.StatusSummary = append(status.StatusSummary, fmt.Sprintf("warning: 读取 Git remote 失败: %v", err))
	}

	if status.CurrentBranch, err = s.gitSingleLine(ctx, status.LocalRoot, "rev-parse", "--abbrev-ref", "HEAD"); err != nil {
		status.StatusSummary = append(status.StatusSummary, fmt.Sprintf("warning: 读取当前分支失败: %v", err))
	}
	if status.HeadCommit, err = s.gitSingleLine(ctx, status.LocalRoot, "rev-parse", "HEAD"); err != nil {
		status.StatusSummary = append(status.StatusSummary, fmt.Sprintf("warning: 读取 HEAD 提交失败: %v", err))
	}
	if status.HeadCommitSubject, err = s.gitSingleLine(ctx, status.LocalRoot, "log", "-1", "--pretty=%s"); err != nil {
		status.StatusSummary = append(status.StatusSummary, fmt.Sprintf("warning: 读取提交摘要失败: %v", err))
	}

	worktreeOutput, err := s.runGit(ctx, status.LocalRoot, "status", "--short")
	if err != nil {
		status.StatusSummary = append(status.StatusSummary, fmt.Sprintf("warning: 读取工作区状态失败: %v", err))
	} else {
		lines := splitNonEmptyLines(worktreeOutput)
		status.WorktreeDirty = len(lines) > 0
		status.StatusSummary = append(status.StatusSummary, lines...)
	}

	if status.RemoteOrigin != "" {
		status.RemoteReachable = s.checkRemoteReachable(ctx, status.RemoteOrigin)
	}
	return status, nil
}

// ListBindingTargets 扫描 GitOps 仓库里可绑定到 ArgoCD CD 的工作子目录。
//
// 当前约定的目录层级是：
// apps/<应用目录>/overlays/<环境目录>
//
// 例如：
// apps/java_nantong/overlays/dev
// apps/java_nantong/overlays/test
//
// 这里返回给前端的不是 overlay 级目录，而是 apps/<应用目录> 这一层：
//  1. 绑定 CD=ArgoCD 时，用户只需要先选应用目录；
//  2. 实际执行环境通过平台标准 Key `env` 传入；
//  3. 发布执行时平台再拼出真正的 overlay 目录：
//     apps/<应用目录>/overlays/<env>
func (s *Service) ListBindingTargets(ctx context.Context) ([]BindingTarget, error) {
	status, err := s.GetStatus(ctx)
	if err != nil {
		return nil, err
	}
	if !status.Enabled {
		return []BindingTarget{}, nil
	}
	if !status.PathExists || !status.IsGitRepo {
		return []BindingTarget{}, nil
	}

	root := filepath.Join(strings.TrimSpace(status.LocalRoot), "apps")
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return []BindingTarget{}, nil
		}
		return nil, err
	}

	targets := make([]BindingTarget, 0)
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		appDir := strings.TrimSpace(entry.Name())
		if appDir == "" {
			continue
		}
		overlayRoot := filepath.Join(root, appDir, "overlays")
		envEntries, envErr := os.ReadDir(overlayRoot)
		if envErr != nil {
			if os.IsNotExist(envErr) {
				continue
			}
			return nil, envErr
		}
		environments := make([]string, 0, len(envEntries))
		for _, envEntry := range envEntries {
			if !envEntry.IsDir() {
				continue
			}
			environment := strings.TrimSpace(envEntry.Name())
			if environment != "" {
				environments = append(environments, environment)
			}
		}
		if len(environments) == 0 {
			continue
		}
		sort.Strings(environments)
		path := filepath.ToSlash(filepath.Join("apps", appDir))
		targets = append(targets, BindingTarget{
			Path:                  path,
			AppDirectory:          appDir,
			DisplayName:           appDir,
			HierarchyHint:         fmt.Sprintf("apps -> %s -> overlays -> <env>（env 由平台标准 Key 传递）", appDir),
			AvailableEnvironments: append([]string(nil), environments...),
		})
	}

	sort.Slice(targets, func(i, j int) bool {
		return strings.Compare(targets[i].Path, targets[j].Path) < 0
	})
	return targets, nil
}

// ListFieldCandidates 扫描指定应用目录下 overlays 里的 YAML 标量字段，
// 供 ArgoCD/GitOps 替换规则在模板页里做受控选择。
//
// 当前扫描约定：
// 1. 只扫描 apps/<appKey>/overlays/<env> 下的 .yaml/.yml 文件；
// 2. 会自动把实际环境目录替换成 {env}，生成稳定的 file_path_template；
// 3. 只暴露可直接替换的标量叶子节点，避免首版就开放复杂结构编辑。
func (s *Service) ListFieldCandidates(ctx context.Context, appKey string) ([]gitopsdomain.FieldCandidate, error) {
	status, err := s.GetStatus(ctx)
	if err != nil {
		return nil, err
	}
	if !status.Enabled || !status.PathExists || !status.IsGitRepo {
		return []gitopsdomain.FieldCandidate{}, nil
	}

	appKey = strings.TrimSpace(appKey)
	if appKey == "" {
		return []gitopsdomain.FieldCandidate{}, nil
	}

	scanRoot := strings.TrimSpace(status.LocalRoot)
	cleanup := func() {}
	scanRoot, cleanup, err = s.prepareConfigCandidateScanRoot(ctx, scanRoot)
	if err != nil {
		return nil, err
	}
	defer cleanup()

	resolvedAppDir, err := s.resolveAppDirectory(scanRoot, appKey)
	if err != nil {
		return nil, err
	}
	if resolvedAppDir == "" {
		return []gitopsdomain.FieldCandidate{}, nil
	}

	kustomizePathTemplate := s.kustomizeScanPathValue()
	kustomizePathTemplate = strings.ReplaceAll(kustomizePathTemplate, "{app_key}", resolvedAppDir)
	kustomizePathTemplate = strings.Replace(kustomizePathTemplate, "/{env}", "", 1)
	kustomizePathTemplate = strings.Replace(kustomizePathTemplate, "/{env}/", "/", 1)
	overlayRoot := filepath.Join(scanRoot, filepath.FromSlash(kustomizePathTemplate))
	envEntries, err := os.ReadDir(overlayRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return []gitopsdomain.FieldCandidate{}, nil
		}
		return nil, err
	}

	result := make([]gitopsdomain.FieldCandidate, 0)
	seen := make(map[string]struct{})
	for _, envEntry := range envEntries {
		if !envEntry.IsDir() {
			continue
		}
		environment := strings.TrimSpace(envEntry.Name())
		if environment == "" {
			continue
		}
		envRoot := filepath.Join(overlayRoot, environment)
		walkErr := filepath.Walk(envRoot, func(path string, info os.FileInfo, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if info == nil || info.IsDir() {
				return nil
			}
			lowerName := strings.ToLower(info.Name())
			if !strings.HasSuffix(lowerName, ".yaml") && !strings.HasSuffix(lowerName, ".yml") {
				return nil
			}
			items, scanErr := scanYAMLFieldCandidates(path, resolvedAppDir, environment)
			if scanErr != nil {
				return scanErr
			}
			for _, item := range items {
				key := strings.Join([]string{
					item.FilePathTemplate,
					item.DocumentKind,
					item.DocumentName,
					item.TargetPath,
				}, "::")
				if _, exists := seen[key]; exists {
					continue
				}
				seen[key] = struct{}{}
				result = append(result, item)
			}
			return nil
		})
		if walkErr != nil {
			return nil, walkErr
		}
	}

	sort.Slice(result, func(i, j int) bool {
		left := strings.Join([]string{result[i].FilePathTemplate, result[i].DocumentKind, result[i].DocumentName, result[i].TargetPath}, "::")
		right := strings.Join([]string{result[j].FilePathTemplate, result[j].DocumentKind, result[j].DocumentName, result[j].TargetPath}, "::")
		return strings.Compare(left, right) < 0
	})
	return result, nil
}

// ListValuesCandidates 扫描应用目录下的 Helm values 文件标量路径，供模板页做受控选择。
//
// 当前策略保持克制：
// 1. 只扫描文件名包含 values 的 .yaml/.yml；
// 2. 只暴露标量叶子节点；
// 3. 会把实际环境目录替换成 {env}，便于用户生成稳定模板。
func (s *Service) ListValuesCandidates(ctx context.Context, appKey string) ([]gitopsdomain.ValuesCandidate, error) {
	status, err := s.GetStatus(ctx)
	if err != nil {
		return nil, err
	}
	if !status.Enabled || !status.PathExists || !status.IsGitRepo {
		return []gitopsdomain.ValuesCandidate{}, nil
	}

	appKey = strings.TrimSpace(appKey)
	if appKey == "" {
		return []gitopsdomain.ValuesCandidate{}, nil
	}

	scanRoot := strings.TrimSpace(status.LocalRoot)
	cleanup := func() {}
	scanRoot, cleanup, err = s.prepareConfigCandidateScanRoot(ctx, scanRoot)
	if err != nil {
		return nil, err
	}
	defer cleanup()

	resolvedAppDir := ""
	var appRoot string
	helmPathTemplate := s.helmScanPathValue()
	helmPathTemplate = strings.ReplaceAll(helmPathTemplate, "{app_key}", appKey)
	helmResolved := filepath.Join(scanRoot, filepath.FromSlash(helmPathTemplate))
	if info, statErr := os.Stat(helmResolved); statErr == nil && info.IsDir() {
		appRoot = helmResolved
		resolvedAppDir = "helm"
	} else {
		resolvedAppDir, err = s.resolveAppDirectory(scanRoot, appKey)
		if err != nil {
			return nil, err
		}
		if resolvedAppDir == "" {
			return []gitopsdomain.ValuesCandidate{}, nil
		}
		appRoot = filepath.Join(scanRoot, "apps", resolvedAppDir)
	}

	result := make([]gitopsdomain.ValuesCandidate, 0)
	seen := make(map[string]struct{})
	walkErr := filepath.Walk(appRoot, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if info == nil || info.IsDir() {
			return nil
		}
		lowerName := strings.ToLower(info.Name())
		if (!strings.HasSuffix(lowerName, ".yaml") && !strings.HasSuffix(lowerName, ".yml")) ||
			!strings.Contains(lowerName, "values") {
			return nil
		}
		items, scanErr := scanValuesCandidates(path, resolvedAppDir)
		if scanErr != nil {
			return scanErr
		}
		for _, item := range items {
			key := strings.Join([]string{item.FilePathTemplate, item.TargetPath}, "::")
			if _, exists := seen[key]; exists {
				continue
			}
			seen[key] = struct{}{}
			result = append(result, item)
		}
		return nil
	})
	if walkErr != nil {
		return nil, walkErr
	}

	sort.Slice(result, func(i, j int) bool {
		left := strings.Join([]string{result[i].FilePathTemplate, result[i].TargetPath}, "::")
		right := strings.Join([]string{result[j].FilePathTemplate, result[j].TargetPath}, "::")
		return strings.Compare(left, right) < 0
	})
	return result, nil
}

// BuildCommitMessage 使用配置模版渲染 GitOps 提交信息。
//
// 当前只支持轻量的占位符替换，而不引入完整模板引擎：
// 1. 配置简单，便于在 JSON/env 中直接书写；
// 2. 支持直接使用标准平台 Key 作为占位符，发布执行时再从流程参数中取值；
// 3. 未识别的占位符会原样保留，便于快速发现模版配置问题；
// 4. 若渲染结果为空，回退到默认模版，避免生成空提交说明。
func (s *Service) BuildCommitMessage(fields map[string]string) string {
	template := NormalizeCommitMessageTemplate(s.currentCommitMessageTemplate())
	normalizedFields := normalizeCommitMessageFields(fields)
	rendered := renderCommitMessageTemplate(template, normalizedFields)
	rendered = strings.Join(strings.Fields(strings.TrimSpace(rendered)), " ")
	if rendered == "" {
		return strings.Join(strings.Fields(strings.TrimSpace(renderCommitMessageTemplate(defaultCommitMessageTemplate, normalizedFields))), " ")
	}
	return rendered
}

// RenderTemplate 复用与提交信息模版相同的轻量占位符替换规则。
//
// 与 BuildCommitMessage 的区别在于：
// 1. 这里不会回退到默认提交模版；
// 2. 更适合 GitOps YAML 字段值模版渲染；
// 3. 未识别占位符保留原样，方便定位规则配置问题。
func (s *Service) RenderTemplate(template string, fields map[string]string) string {
	return strings.TrimSpace(renderCommitMessageTemplate(strings.TrimSpace(template), normalizeCommitMessageFields(fields)))
}

// UpdateKustomizationImage 只做一件事：更新某个 overlay 下 kustomization.yaml 的镜像 tag。
//
// 这里特意限制为“单镜像条目受控修改”：
// 1. 平台首版只支持更新 images[].newTag，不开放任意 YAML 路径编辑；
// 2. 若文件里存在多条 image 记录，会直接报错，避免错误改动 sidecar 等其他镜像；
// 3. 每次更新后都会以平台固定身份提交，并 push 到指定分支。
func (s *Service) UpdateKustomizationImage(
	ctx context.Context,
	repoURL string,
	sourcePath string,
	branch string,
	newTag string,
	commitMessage string,
) (workspacePath string, manifestPath string, commitSHA string, previousTag string, changed bool, err error) {
	if !s.Enabled() {
		return "", "", "", "", false, fmt.Errorf("gitops service is not configured")
	}

	repoURL = strings.TrimSpace(repoURL)
	sourcePath = strings.TrimSpace(sourcePath)
	branch = resolveBranch(branch, s.defaultBranch)
	newTag = strings.TrimSpace(newTag)
	commitMessage = strings.TrimSpace(commitMessage)
	if repoURL == "" {
		return "", "", "", "", false, fmt.Errorf("gitops repo url is required")
	}
	if sourcePath == "" {
		return "", "", "", "", false, fmt.Errorf("gitops source path is required")
	}
	if newTag == "" {
		return "", "", "", "", false, fmt.Errorf("image_version is required")
	}
	if commitMessage == "" {
		commitMessage = s.BuildCommitMessage(map[string]string{
			"image_version": newTag,
			"source_path":   sourcePath,
		})
	}
	unlock := acquireRepoBranchLock(repoURL, branch)
	defer unlock()

	workspacePath, err = s.resolveWorkspacePath(ctx, repoURL)
	if err != nil {
		return "", "", "", "", false, err
	}
	if err = os.MkdirAll(s.localRoot, 0o755); err != nil {
		return "", "", "", "", false, err
	}
	if err = s.prepareWorkspace(ctx, repoURL, branch, workspacePath); err != nil {
		return "", "", "", "", false, err
	}
	if err = s.configureAuthor(ctx, workspacePath); err != nil {
		return "", "", "", "", false, err
	}

	manifestPath, err = secureJoin(workspacePath, sourcePath, "kustomization.yaml")
	if err != nil {
		return "", "", "", "", false, err
	}
	content, readErr := os.ReadFile(manifestPath)
	if readErr != nil {
		return "", "", "", "", false, readErr
	}

	updatedContent, previousTag, changed, err := updateSingleKustomizeImageTag(content, newTag)
	if err != nil {
		return "", "", "", "", false, err
	}
	if changed {
		if err = os.WriteFile(manifestPath, updatedContent, 0o644); err != nil {
			return "", "", "", "", false, err
		}
		relativeManifestPath, relErr := filepath.Rel(workspacePath, manifestPath)
		if relErr != nil {
			return "", "", "", "", false, relErr
		}
		if _, cmdErr := s.runGit(ctx, workspacePath, "add", relativeManifestPath); cmdErr != nil {
			return "", "", "", "", false, cmdErr
		}
		if _, cmdErr := s.runGit(ctx, workspacePath, "commit", "-m", commitMessage); cmdErr != nil {
			return "", "", "", "", false, cmdErr
		}
		if cmdErr := s.pushWithRetry(ctx, workspacePath, branch); cmdErr != nil {
			return "", "", "", "", false, cmdErr
		}
	}

	commitSHA, err = s.currentCommitSHA(ctx, workspacePath)
	if err != nil {
		return "", "", "", "", false, err
	}
	return workspacePath, manifestPath, commitSHA, previousTag, changed, nil
}

// ApplyManifestRules 将规则化的 YAML 字段替换应用到 GitOps 仓库，并独立提交一次 Git 变更。
//
// 当前版本仍保持保守：
// 1. 只支持基于 file/document/path 的精确字段替换；
// 2. 只改标量叶子节点；
// 3. 如果所有规则都没有实际改动，则不会生成空提交。
func (s *Service) ApplyManifestRules(
	ctx context.Context,
	repoURL string,
	branch string,
	rules []gitopsdomain.ManifestRule,
	commitMessage string,
) (workspacePath string, changedFiles []string, commitSHA string, changed bool, err error) {
	if !s.Enabled() {
		return "", nil, "", false, fmt.Errorf("gitops service is not configured")
	}
	if len(rules) == 0 {
		return "", []string{}, "", false, nil
	}

	repoURL = strings.TrimSpace(repoURL)
	branch = resolveBranch(branch, s.defaultBranch)
	commitMessage = strings.TrimSpace(commitMessage)
	if repoURL == "" {
		return "", nil, "", false, fmt.Errorf("gitops repo url is required")
	}
	if commitMessage == "" {
		commitMessage = "chore(gitops): update manifest fields"
	}
	logx.Info("gitops_service", "apply_manifest_rules_start",
		logx.F("repo_url", normalizeRepoURL(repoURL)),
		logx.F("branch", branch),
		logx.F("rules_count", len(rules)),
	)
	unlock := acquireRepoBranchLock(repoURL, branch)
	defer unlock()

	workspacePath, err = s.resolveWorkspacePath(ctx, repoURL)
	if err != nil {
		return "", nil, "", false, err
	}
	if err = os.MkdirAll(s.localRoot, 0o755); err != nil {
		return "", nil, "", false, err
	}
	if err = s.prepareWorkspace(ctx, repoURL, branch, workspacePath); err != nil {
		return "", nil, "", false, err
	}
	if err = s.configureAuthor(ctx, workspacePath); err != nil {
		return "", nil, "", false, err
	}

	grouped := make(map[string][]gitopsdomain.ManifestRule)
	for _, item := range rules {
		filePath := strings.TrimSpace(item.FilePath)
		if filePath == "" {
			continue
		}
		grouped[filePath] = append(grouped[filePath], item)
	}
	if len(grouped) == 0 {
		logx.Warn("gitops_service", "apply_manifest_rules_skipped",
			logx.F("repo_url", normalizeRepoURL(repoURL)),
			logx.F("branch", branch),
			logx.F("reason", "empty_file_groups"),
		)
		return workspacePath, []string{}, "", false, nil
	}

	relativeChanged := make([]string, 0, len(grouped))
	for relativePath, items := range grouped {
		absolutePath, joinErr := secureJoin(workspacePath, relativePath)
		if joinErr != nil {
			return workspacePath, nil, "", false, joinErr
		}
		content, readErr := os.ReadFile(absolutePath)
		if readErr != nil {
			return workspacePath, nil, "", false, readErr
		}
		updated, fileChanged, applyErr := applyManifestRulesToFile(content, items)
		if applyErr != nil {
			return workspacePath, nil, "", false, applyErr
		}
		if !fileChanged {
			continue
		}
		if writeErr := os.WriteFile(absolutePath, updated, 0o644); writeErr != nil {
			return workspacePath, nil, "", false, writeErr
		}
		relativeChanged = append(relativeChanged, relativePath)
	}

	if len(relativeChanged) == 0 {
		commitSHA, err = s.currentCommitSHA(ctx, workspacePath)
		if err != nil {
			return workspacePath, []string{}, "", false, err
		}
		logx.Info("gitops_service", "apply_manifest_rules_noop",
			logx.F("repo_url", normalizeRepoURL(repoURL)),
			logx.F("branch", branch),
			logx.F("workspace_path", workspacePath),
			logx.F("commit_sha", commitSHA),
		)
		return workspacePath, []string{}, commitSHA, false, nil
	}

	sort.Strings(relativeChanged)
	addArgs := append([]string{"add"}, relativeChanged...)
	if _, err := s.runGit(ctx, workspacePath, addArgs...); err != nil {
		return workspacePath, nil, "", false, err
	}
	if _, err := s.runGit(ctx, workspacePath, "commit", "-m", commitMessage); err != nil {
		return workspacePath, nil, "", false, err
	}
	if err := s.pushWithRetry(ctx, workspacePath, branch); err != nil {
		return workspacePath, nil, "", false, err
	}
	commitSHA, err = s.currentCommitSHA(ctx, workspacePath)
	if err != nil {
		return workspacePath, nil, "", false, err
	}
	logx.Info("gitops_service", "apply_manifest_rules_success",
		logx.F("repo_url", normalizeRepoURL(repoURL)),
		logx.F("branch", branch),
		logx.F("workspace_path", workspacePath),
		logx.F("changed_files_count", len(relativeChanged)),
		logx.F("commit_sha", commitSHA),
	)
	return workspacePath, append([]string(nil), relativeChanged...), commitSHA, true, nil
}

// ApplyValuesRules 将 Helm values 规则写回到 GitOps 仓库，并独立提交一次 Git 变更。
func (s *Service) ApplyValuesRules(
	ctx context.Context,
	repoURL string,
	branch string,
	rules []gitopsdomain.ValuesRule,
	commitMessage string,
) (workspacePath string, changedFiles []string, commitSHA string, changed bool, err error) {
	if !s.Enabled() {
		return "", nil, "", false, fmt.Errorf("gitops service is not configured")
	}
	if len(rules) == 0 {
		return "", []string{}, "", false, nil
	}

	repoURL = strings.TrimSpace(repoURL)
	branch = resolveBranch(branch, s.defaultBranch)
	commitMessage = strings.TrimSpace(commitMessage)
	if repoURL == "" {
		return "", nil, "", false, fmt.Errorf("gitops repo url is required")
	}
	if commitMessage == "" {
		commitMessage = "chore(gitops): update helm values"
	}
	logx.Info("gitops_service", "apply_values_rules_start",
		logx.F("repo_url", normalizeRepoURL(repoURL)),
		logx.F("branch", branch),
		logx.F("rules_count", len(rules)),
	)
	unlock := acquireRepoBranchLock(repoURL, branch)
	defer unlock()

	workspacePath, err = s.resolveWorkspacePath(ctx, repoURL)
	if err != nil {
		return "", nil, "", false, err
	}
	if err = os.MkdirAll(s.localRoot, 0o755); err != nil {
		return "", nil, "", false, err
	}
	if err = s.prepareWorkspace(ctx, repoURL, branch, workspacePath); err != nil {
		return "", nil, "", false, err
	}
	if err = s.configureAuthor(ctx, workspacePath); err != nil {
		return "", nil, "", false, err
	}

	grouped := make(map[string][]gitopsdomain.ValuesRule)
	for _, item := range rules {
		filePath := strings.TrimSpace(item.FilePath)
		if filePath == "" {
			continue
		}
		grouped[filePath] = append(grouped[filePath], item)
	}
	if len(grouped) == 0 {
		logx.Warn("gitops_service", "apply_values_rules_skipped",
			logx.F("repo_url", normalizeRepoURL(repoURL)),
			logx.F("branch", branch),
			logx.F("reason", "empty_file_groups"),
		)
		return workspacePath, []string{}, "", false, nil
	}

	relativeChanged := make([]string, 0, len(grouped))
	for relativePath, items := range grouped {
		absolutePath, joinErr := secureJoin(workspacePath, relativePath)
		if joinErr != nil {
			return workspacePath, nil, "", false, joinErr
		}
		content, readErr := os.ReadFile(absolutePath)
		if readErr != nil {
			return workspacePath, nil, "", false, readErr
		}
		updated, fileChanged, applyErr := applyValuesRulesToFile(content, items)
		if applyErr != nil {
			return workspacePath, nil, "", false, applyErr
		}
		if !fileChanged {
			continue
		}
		if writeErr := os.WriteFile(absolutePath, updated, 0o644); writeErr != nil {
			return workspacePath, nil, "", false, writeErr
		}
		relativeChanged = append(relativeChanged, relativePath)
	}

	if len(relativeChanged) == 0 {
		commitSHA, err = s.currentCommitSHA(ctx, workspacePath)
		if err != nil {
			return workspacePath, []string{}, "", false, err
		}
		logx.Info("gitops_service", "apply_values_rules_noop",
			logx.F("repo_url", normalizeRepoURL(repoURL)),
			logx.F("branch", branch),
			logx.F("workspace_path", workspacePath),
			logx.F("commit_sha", commitSHA),
		)
		return workspacePath, []string{}, commitSHA, false, nil
	}

	sort.Strings(relativeChanged)
	addArgs := append([]string{"add"}, relativeChanged...)
	if _, err := s.runGit(ctx, workspacePath, addArgs...); err != nil {
		return workspacePath, nil, "", false, err
	}
	if _, err := s.runGit(ctx, workspacePath, "commit", "-m", commitMessage); err != nil {
		return workspacePath, nil, "", false, err
	}
	if err := s.pushWithRetry(ctx, workspacePath, branch); err != nil {
		return workspacePath, nil, "", false, err
	}
	commitSHA, err = s.currentCommitSHA(ctx, workspacePath)
	if err != nil {
		return workspacePath, nil, "", false, err
	}
	logx.Info("gitops_service", "apply_values_rules_success",
		logx.F("repo_url", normalizeRepoURL(repoURL)),
		logx.F("branch", branch),
		logx.F("workspace_path", workspacePath),
		logx.F("changed_files_count", len(relativeChanged)),
		logx.F("commit_sha", commitSHA),
	)
	return workspacePath, append([]string(nil), relativeChanged...), commitSHA, true, nil
}

// resolveWorkspacePath 优先复用 local_root 本身作为 GitOps 工作仓库。
//
// 这样可以支持两种模式：
//  1. local_root 是普通目录：平台在其下创建稳定命名的工作副本；
//  2. local_root 本身已经是目标仓库：平台直接在该仓库里 pull / edit / commit / push，
//     避免出现“仓库里再套一层 gitops-xxxx 子仓库”的混乱结构。
func (s *Service) resolveWorkspacePath(ctx context.Context, repoURL string) (string, error) {
	root := strings.TrimSpace(s.localRoot)
	if root == "" {
		return "", fmt.Errorf("gitops local root is required")
	}
	if _, err := os.Stat(filepath.Join(root, ".git")); err == nil {
		remoteURL, remoteErr := s.remoteOriginURL(ctx, root)
		if remoteErr == nil && sameRepo(remoteURL, repoURL) {
			return root, nil
		}
	}
	return filepath.Join(root, repoWorkspaceName(repoURL)), nil
}

// prepareWorkspace 封装当前模块的业务处理逻辑。
func (s *Service) prepareWorkspace(ctx context.Context, repoURL string, branch string, workspacePath string) error {
	authURL, err := s.authRemoteURL(repoURL)
	if err != nil {
		return err
	}
	if _, statErr := os.Stat(filepath.Join(workspacePath, ".git")); statErr == nil {
		logx.Info("gitops_service", "prepare_workspace_reuse",
			logx.F("repo_url", normalizeRepoURL(repoURL)),
			logx.F("branch", branch),
			logx.F("workspace_path", workspacePath),
		)
		if _, err := s.runGit(ctx, workspacePath, "remote", "set-url", "origin", authURL); err != nil {
			return err
		}
		if _, err := s.runGit(ctx, workspacePath, "fetch", "origin", branch); err != nil {
			return err
		}
		if _, err := s.runGit(ctx, workspacePath, "checkout", "-B", branch, "origin/"+branch); err != nil {
			return err
		}
		if _, err := s.runGit(ctx, workspacePath, "clean", "-fd"); err != nil {
			return err
		}
		return nil
	}
	logx.Info("gitops_service", "prepare_workspace_clone",
		logx.F("repo_url", normalizeRepoURL(repoURL)),
		logx.F("branch", branch),
		logx.F("workspace_path", workspacePath),
	)
	if err := os.RemoveAll(workspacePath); err != nil {
		return err
	}
	_, err = s.runCommand(ctx, "", "git", "clone", "--branch", branch, "--single-branch", authURL, workspacePath)
	return err
}

// remoteOriginURL 封装当前模块的业务处理逻辑。
func (s *Service) remoteOriginURL(ctx context.Context, workspacePath string) (string, error) {
	output, err := s.runGit(ctx, workspacePath, "remote", "get-url", "origin")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(output), nil
}

// configureAuthor 封装当前模块的业务处理逻辑。
func (s *Service) configureAuthor(ctx context.Context, workspacePath string) error {
	if s.authorName == "" || s.authorEmail == "" {
		return nil
	}
	if _, err := s.runGit(ctx, workspacePath, "config", "user.name", s.authorName); err != nil {
		return err
	}
	if _, err := s.runGit(ctx, workspacePath, "config", "user.email", s.authorEmail); err != nil {
		return err
	}
	return nil
}

// currentCommitSHA 封装当前模块的业务处理逻辑。
func (s *Service) currentCommitSHA(ctx context.Context, workspacePath string) (string, error) {
	output, err := s.runGit(ctx, workspacePath, "rev-parse", "HEAD")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(output), nil
}

// authRemoteURL 封装当前模块的业务处理逻辑。
func (s *Service) authRemoteURL(rawURL string) (string, error) {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return "", fmt.Errorf("gitops repo url is required")
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "", err
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return rawURL, nil
	}
	switch {
	case s.username != "" && s.password != "":
		parsed.User = url.UserPassword(s.username, s.password)
	case s.token != "":
		username := s.username
		if username == "" {
			username = "oauth2"
		}
		parsed.User = url.UserPassword(username, s.token)
	}
	return parsed.String(), nil
}

// runGit 封装当前模块的业务处理逻辑。
func (s *Service) runGit(ctx context.Context, workspacePath string, args ...string) (string, error) {
	return s.runCommand(ctx, workspacePath, "git", args...)
}

// pushWithRetry 封装当前模块的业务处理逻辑。
func (s *Service) pushWithRetry(ctx context.Context, workspacePath string, branch string) error {
	if _, err := s.runGit(ctx, workspacePath, "push", "origin", branch); err != nil {
		if !isNonFastForwardPushError(err) {
			return err
		}
		logx.Warn("gitops_service", "push_retry_rebase",
			logx.F("workspace_path", workspacePath),
			logx.F("branch", branch),
			logx.F("reason", err.Error()),
		)
		if _, fetchErr := s.runGit(ctx, workspacePath, "fetch", "origin", branch); fetchErr != nil {
			return fmt.Errorf("git push rejected and fetch retry failed: %w", fetchErr)
		}
		if _, rebaseErr := s.runGit(ctx, workspacePath, "rebase", "origin/"+branch); rebaseErr != nil {
			_, _ = s.runGit(context.Background(), workspacePath, "rebase", "--abort")
			return fmt.Errorf("git push rejected because remote branch advanced; automatic rebase failed: %w", rebaseErr)
		}
		if _, pushErr := s.runGit(ctx, workspacePath, "push", "origin", branch); pushErr != nil {
			return fmt.Errorf("git push retry failed after rebasing onto latest origin/%s: %w", branch, pushErr)
		}
		logx.Info("gitops_service", "push_retry_success",
			logx.F("workspace_path", workspacePath),
			logx.F("branch", branch),
		)
	}
	return nil
}

// gitSingleLine 封装当前模块的业务处理逻辑。
func (s *Service) gitSingleLine(ctx context.Context, workspacePath string, args ...string) (string, error) {
	output, err := s.runGit(ctx, workspacePath, args...)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(output), nil
}

// runCommand 封装当前模块的业务处理逻辑。
func (s *Service) runCommand(ctx context.Context, workspacePath string, name string, args ...string) (string, error) {
	timeout := time.Duration(s.commandTimeoutSec) * time.Second
	if timeout <= 0 {
		timeout = 120 * time.Second
	}
	if timeout < 120*time.Second {
		timeout = 120 * time.Second
	}
	lockMaxAge := timeout
	if lockMaxAge < 60*time.Second {
		lockMaxAge = 60 * time.Second
	}
	if name == "git" && strings.TrimSpace(workspacePath) != "" {
		_, _ = recoverStaleGitIndexLock(workspacePath, lockMaxAge)
	}
	cmdCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(cmdCtx, name, args...)
	cmd.Dir = workspacePath
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		message := strings.TrimSpace(stderr.String())
		if message == "" {
			message = strings.TrimSpace(stdout.String())
		}
		if message == "" {
			message = err.Error()
		}
		if name == "git" && strings.TrimSpace(workspacePath) != "" && isGitIndexLockError(message) {
			cleared, clearErr := recoverStaleGitIndexLock(workspacePath, 0)
			if clearErr != nil {
				logx.Warn("gitops_service", "git_index_lock_cleanup_failed",
					logx.F("workspace_path", workspacePath),
					logx.F("reason", clearErr.Error()),
				)
			}
			if cleared {
				logx.Warn("gitops_service", "git_index_lock_retry",
					logx.F("workspace_path", workspacePath),
					logx.F("command", name),
					logx.F("args", strings.Join(args, " ")),
				)
				return s.runCommandWithoutRetry(ctx, workspacePath, timeout, name, args...)
			}
		}
		return "", fmt.Errorf("%s %s failed: %s", name, strings.Join(args, " "), message)
	}
	return stdout.String(), nil
}

// runCommandWithoutRetry 封装当前模块的业务处理逻辑。
func (s *Service) runCommandWithoutRetry(ctx context.Context, workspacePath string, timeout time.Duration, name string, args ...string) (string, error) {
	cmdCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(cmdCtx, name, args...)
	cmd.Dir = workspacePath
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		message := strings.TrimSpace(stderr.String())
		if message == "" {
			message = strings.TrimSpace(stdout.String())
		}
		if message == "" {
			message = err.Error()
		}
		return "", fmt.Errorf("%s %s failed: %s", name, strings.Join(args, " "), message)
	}
	return stdout.String(), nil
}

// recoverStaleGitIndexLock 封装当前模块的业务处理逻辑。
func recoverStaleGitIndexLock(workspacePath string, maxAge time.Duration) (bool, error) {
	lockPath := filepath.Join(strings.TrimSpace(workspacePath), ".git", "index.lock")
	info, err := os.Stat(lockPath)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	if maxAge > 0 && time.Since(info.ModTime()) < maxAge {
		return false, nil
	}
	if err := os.Remove(lockPath); err != nil {
		return false, err
	}
	return true, nil
}

// isGitIndexLockError 封装当前模块的业务处理逻辑。
func isGitIndexLockError(message string) bool {
	text := strings.ToLower(strings.TrimSpace(message))
	return strings.Contains(text, "index.lock") && strings.Contains(text, "file exists")
}

// repoWorkspaceName 封装当前模块的业务处理逻辑。
func repoWorkspaceName(repoURL string) string {
	sum := sha1.Sum([]byte(strings.TrimSpace(repoURL)))
	return "gitops-" + hex.EncodeToString(sum[:8])
}

// checkRemoteReachable 检查业务状态并返回校验结果。
func (s *Service) checkRemoteReachable(ctx context.Context, remoteURL string) bool {
	authURL, err := s.authRemoteURL(remoteURL)
	if err != nil {
		return false
	}
	_, err = s.runCommand(ctx, "", "git", "ls-remote", authURL, "HEAD")
	return err == nil
}

// sameRepo 封装当前模块的业务处理逻辑。
func sameRepo(left string, right string) bool {
	return normalizeRepoURL(left) == normalizeRepoURL(right)
}

// normalizeRepoURL 标准化输入值，保证后续逻辑使用统一格式。
func normalizeRepoURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return strings.TrimSuffix(raw, "/")
	}
	parsed.User = nil
	parsed.Path = strings.TrimSuffix(parsed.Path, "/")
	return parsed.String()
}

// splitNonEmptyLines 封装当前模块的业务处理逻辑。
func splitNonEmptyLines(raw string) []string {
	lines := strings.Split(raw, "\n")
	items := make([]string, 0, len(lines))
	for _, line := range lines {
		value := strings.TrimSpace(line)
		if value != "" {
			items = append(items, value)
		}
	}
	return items
}

// resolveBranch 解析上下文数据，得到后续流程需要的结果。
func resolveBranch(candidate string, fallback string) string {
	candidate = strings.TrimSpace(candidate)
	if candidate == "" || strings.EqualFold(candidate, "HEAD") {
		candidate = strings.TrimSpace(fallback)
	}
	if candidate == "" {
		return "master"
	}
	return candidate
}

// acquireRepoBranchLock 封装当前模块的业务处理逻辑。
func acquireRepoBranchLock(repoURL string, branch string) func() {
	key := normalizeRepoURL(repoURL) + "::" + resolveBranch(branch, "")
	actual, _ := repoBranchLocks.LoadOrStore(key, &sync.Mutex{})
	mu := actual.(*sync.Mutex)
	mu.Lock()
	return mu.Unlock
}

// isNonFastForwardPushError 封装当前模块的业务处理逻辑。
func isNonFastForwardPushError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(strings.TrimSpace(err.Error()))
	return strings.Contains(message, "[rejected]") ||
		strings.Contains(message, "fetch first") ||
		strings.Contains(message, "non-fast-forward") ||
		strings.Contains(message, "failed to push some refs")
}

// resolveAppDirectory 解析上下文数据，得到后续流程需要的结果。
func (s *Service) resolveAppDirectory(localRoot string, appKey string) (string, error) {
	appKey = strings.TrimSpace(appKey)
	if appKey == "" {
		return "", nil
	}
	root := filepath.Join(strings.TrimSpace(localRoot), "apps")
	exactPath := filepath.Join(root, appKey)
	if info, err := os.Stat(exactPath); err == nil && info.IsDir() {
		return appKey, nil
	}

	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	targets := []string{
		normalizeAppDirectoryKey(appKey),
		normalizeAppDirectoryKey(strings.ReplaceAll(appKey, "-", "_")),
		normalizeAppDirectoryKey(strings.ReplaceAll(appKey, "_", "-")),
	}
	targetSet := make(map[string]struct{}, len(targets))
	for _, item := range targets {
		if item != "" {
			targetSet[item] = struct{}{}
		}
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := strings.TrimSpace(entry.Name())
		if name == "" {
			continue
		}
		if _, ok := targetSet[normalizeAppDirectoryKey(name)]; ok {
			return name, nil
		}
	}
	return "", nil
}

// normalizeAppDirectoryKey 标准化输入值，保证后续逻辑使用统一格式。
func normalizeAppDirectoryKey(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	value = strings.ReplaceAll(value, "_", "-")
	for _, suffix := range []string{"-dev", "-test", "-prod", "_dev", "_test", "_prod"} {
		value = strings.TrimSuffix(value, strings.ToLower(suffix))
	}
	return strings.TrimSpace(value)
}

// prepareConfigCandidateScanRoot 封装当前模块的业务处理逻辑。
func (s *Service) prepareConfigCandidateScanRoot(ctx context.Context, localRoot string) (string, func(), error) {
	localRoot = strings.TrimSpace(localRoot)
	if localRoot == "" {
		return "", func() {}, nil
	}
	if !s.branchExists(ctx, localRoot, configCandidateScanBranch) {
		return "", nil, fmt.Errorf("gitops config candidate scan branch %s not found", configCandidateScanBranch)
	}

	tempDir, err := os.MkdirTemp("", "gos-gitops-config-scan-*")
	if err != nil {
		return "", nil, err
	}
	if _, err := s.runGit(ctx, localRoot, "worktree", "add", "--detach", tempDir, configCandidateScanBranch); err != nil {
		_ = os.RemoveAll(tempDir)
		return "", nil, err
	}

	cleanup := func() {
		_, _ = s.runGit(context.Background(), localRoot, "worktree", "remove", "--force", tempDir)
		_ = os.RemoveAll(tempDir)
	}
	return tempDir, cleanup, nil
}

// branchExists 封装当前模块的业务处理逻辑。
func (s *Service) branchExists(ctx context.Context, workspacePath string, branch string) bool {
	branch = strings.TrimSpace(branch)
	if branch == "" {
		return false
	}
	_, err := s.runGit(ctx, workspacePath, "rev-parse", "--verify", branch+"^{commit}")
	return err == nil
}

// secureJoin 封装当前模块的业务处理逻辑。
func secureJoin(root string, parts ...string) (string, error) {
	joined := filepath.Join(append([]string{root}, parts...)...)
	cleanRoot := filepath.Clean(root)
	cleanJoined := filepath.Clean(joined)
	relative, err := filepath.Rel(cleanRoot, cleanJoined)
	if err != nil {
		return "", err
	}
	if strings.HasPrefix(relative, "..") {
		return "", fmt.Errorf("gitops source path escapes workspace")
	}
	return cleanJoined, nil
}

// updateSingleKustomizeImageTag 查询并返回指定资源数据。
func updateSingleKustomizeImageTag(content []byte, newTag string) ([]byte, string, bool, error) {
	var doc yaml.MapSlice
	if err := yaml.Unmarshal(content, &doc); err != nil {
		return nil, "", false, fmt.Errorf("parse kustomization.yaml failed: %w", err)
	}

	imagesValue, ok := mapSliceGet(doc, "images")
	if !ok {
		return nil, "", false, fmt.Errorf("kustomization.yaml is missing images section")
	}
	images, ok := imagesValue.([]interface{})
	if !ok || len(images) == 0 {
		return nil, "", false, fmt.Errorf("kustomization.yaml has no image entries")
	}
	if len(images) != 1 {
		return nil, "", false, fmt.Errorf("kustomization.yaml contains %d image entries, current GitOps strategy only supports one", len(images))
	}

	imageItem, ok := images[0].(yaml.MapSlice)
	if !ok {
		return nil, "", false, fmt.Errorf("unsupported images item structure")
	}
	currentTag, ok := mapSliceString(imageItem, "newTag")
	if !ok {
		return nil, "", false, fmt.Errorf("images[0].newTag is missing")
	}
	if strings.TrimSpace(currentTag) == strings.TrimSpace(newTag) {
		return content, currentTag, false, nil
	}

	imageItem = mapSliceSet(imageItem, "newTag", newTag)
	images[0] = imageItem
	doc = mapSliceSet(doc, "images", images)

	updated, err := yaml.Marshal(doc)
	if err != nil {
		return nil, "", false, fmt.Errorf("encode kustomization.yaml failed: %w", err)
	}
	return updated, currentTag, true, nil
}

// mapSliceGet 查询并返回指定资源数据。
func mapSliceGet(items yaml.MapSlice, key string) (interface{}, bool) {
	for _, item := range items {
		if strings.EqualFold(fmt.Sprint(item.Key), key) {
			return item.Value, true
		}
	}
	return nil, false
}

// mapSliceString 封装当前模块的业务处理逻辑。
func mapSliceString(items yaml.MapSlice, key string) (string, bool) {
	value, ok := mapSliceGet(items, key)
	if !ok {
		return "", false
	}
	return strings.TrimSpace(fmt.Sprint(value)), true
}

// scanYAMLFieldCandidates 封装当前模块的业务处理逻辑。
func scanYAMLFieldCandidates(path string, appKey string, environment string) ([]gitopsdomain.FieldCandidate, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	documents := splitYAMLDocuments(string(content))
	if len(documents) == 0 {
		return []gitopsdomain.FieldCandidate{}, nil
	}

	relativeWithinEnv, err := filepath.Rel(filepath.Join(filepath.Dir(path), ".."), path)
	if err != nil {
		// fallback to path relative from env root if the above shape is not met
		relativeWithinEnv, err = filepath.Rel(filepath.Join(filepath.Dir(path[:len(path)-len(filepath.Base(path))])), path)
		if err != nil {
			relativeWithinEnv = filepath.Base(path)
		}
	}
	// The path we actually need is relative to overlays/<env>/...
	overlayIndex := strings.Index(filepath.ToSlash(path), filepath.ToSlash(filepath.Join("apps", appKey, "overlays", environment)))
	if overlayIndex >= 0 {
		relativeWithinEnv = strings.TrimPrefix(filepath.ToSlash(path)[overlayIndex+len(filepath.ToSlash(filepath.Join("apps", appKey, "overlays", environment))):], "/")
	}
	filePathTemplate := filepath.ToSlash(filepath.Join("apps", appKey, "overlays", "{env}", relativeWithinEnv))

	result := make([]gitopsdomain.FieldCandidate, 0)
	for _, rawDoc := range documents {
		var node interface{}
		if err := yaml.Unmarshal([]byte(rawDoc), &node); err != nil {
			return nil, fmt.Errorf("parse yaml candidate file failed: %w", err)
		}
		kind, name := extractDocumentIdentity(node)
		name = normalizeEnvironmentPlaceholder(name, environment)
		fields := make([]gitopsdomain.FieldCandidate, 0)
		collectScalarCandidates(node, "", filePathTemplate, kind, name, &fields)
		result = append(result, fields...)
	}
	return result, nil
}

// scanValuesCandidates 封装当前模块的业务处理逻辑。
func scanValuesCandidates(path string, appKey string) ([]gitopsdomain.ValuesCandidate, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var node interface{}
	if err := yaml.Unmarshal(content, &node); err != nil {
		return nil, fmt.Errorf("parse values file failed: %w", err)
	}

	filePathTemplate := filepath.ToSlash(path)
	appPrefix := filepath.ToSlash(filepath.Join("apps", appKey)) + "/"
	if idx := strings.Index(filePathTemplate, appPrefix); idx >= 0 {
		filePathTemplate = "apps/" + filePathTemplate[idx+len("apps/"):]
	}
	filePathTemplate = normalizeValuesFilePathTemplate(filePathTemplate)
	filePathTemplate = normalizeHoistedHelmValuesFilePathTemplate(filePathTemplate)

	result := make([]gitopsdomain.ValuesCandidate, 0)
	collectValuesScalarCandidates(node, nil, filePathTemplate, &result)
	return result, nil
}

// normalizeValuesFilePathTemplate 标准化输入值，保证后续逻辑使用统一格式。
func normalizeValuesFilePathTemplate(value string) string {
	value = filepath.ToSlash(strings.TrimSpace(value))
	for _, item := range []struct {
		old string
		new string
	}{
		{"/dev/", "/{env}/"},
		{"/test/", "/{env}/"},
		{"/prod/", "/{env}/"},
		{"-dev.", "-{env}."},
		{"-test.", "-{env}."},
		{"-prod.", "-{env}."},
		{"_dev.", "_{env}."},
		{"_test.", "_{env}."},
		{"_prod.", "_{env}."},
		{".dev.", ".{env}."},
		{".test.", ".{env}."},
		{".prod.", ".{env}."},
	} {
		value = strings.ReplaceAll(value, item.old, item.new)
	}
	return value
}

// normalizeHoistedHelmValuesFilePathTemplate 标准化输入值，保证后续逻辑使用统一格式。
func normalizeHoistedHelmValuesFilePathTemplate(value string) string {
	value = filepath.ToSlash(strings.TrimSpace(value))
	if value == "" {
		return ""
	}
	const prefix = "apps/"
	if !strings.HasPrefix(value, prefix) {
		return value
	}
	rest := strings.TrimPrefix(value, prefix)
	parts := strings.Split(rest, "/")
	if len(parts) < 3 {
		return value
	}
	if strings.EqualFold(parts[0], "helm") {
		return value
	}
	if strings.EqualFold(parts[1], "helm") {
		return filepath.ToSlash(filepath.Join("apps", "helm", filepath.Join(parts[2:]...)))
	}
	return value
}

// collectValuesScalarCandidates 封装当前模块的业务处理逻辑。
func collectValuesScalarCandidates(
	node interface{},
	segments []string,
	filePathTemplate string,
	items *[]gitopsdomain.ValuesCandidate,
) {
	switch typed := node.(type) {
	case map[interface{}]interface{}:
		keys := make([]string, 0, len(typed))
		values := make(map[string]interface{}, len(typed))
		for key, value := range typed {
			text := strings.TrimSpace(fmt.Sprint(key))
			if text == "" {
				continue
			}
			keys = append(keys, text)
			values[text] = value
		}
		sort.Strings(keys)
		for _, key := range keys {
			collectValuesScalarCandidates(values[key], append(append([]string(nil), segments...), key), filePathTemplate, items)
		}
	case map[string]interface{}:
		keys := make([]string, 0, len(typed))
		for key := range typed {
			key = strings.TrimSpace(key)
			if key != "" {
				keys = append(keys, key)
			}
		}
		sort.Strings(keys)
		for _, key := range keys {
			collectValuesScalarCandidates(typed[key], append(append([]string(nil), segments...), key), filePathTemplate, items)
		}
	case []interface{}:
		for idx, value := range typed {
			collectValuesScalarCandidates(value, append(append([]string(nil), segments...), fmt.Sprintf("[%d]", idx)), filePathTemplate, items)
		}
	case string, bool, int, int64, float64, float32, uint, uint64:
		if len(segments) == 0 {
			return
		}
		targetPath := strings.Join(segments, ".")
		sample := strings.TrimSpace(fmt.Sprint(typed))
		if len(sample) > 120 {
			sample = sample[:120] + "..."
		}
		*items = append(*items, gitopsdomain.ValuesCandidate{
			FilePathTemplate: filePathTemplate,
			TargetPath:       targetPath,
			ValueType:        scalarTypeName(typed),
			SampleValue:      sample,
			DisplayName:      fmt.Sprintf("%s / %s", filepath.Base(filePathTemplate), targetPath),
		})
	}
}

// applyValuesRulesToFile 封装当前模块的业务处理逻辑。
func applyValuesRulesToFile(content []byte, rules []gitopsdomain.ValuesRule) ([]byte, bool, error) {
	var node interface{}
	if err := yaml.Unmarshal(content, &node); err != nil {
		return nil, false, fmt.Errorf("parse values file failed: %w", err)
	}

	changed := false
	current := node
	for _, rule := range rules {
		updated, applied, err := setNodeValueByDotPath(current, strings.TrimSpace(rule.TargetPath), strings.TrimSpace(rule.Value))
		if err != nil {
			return nil, false, err
		}
		if applied {
			changed = true
			current = updated
		}
	}

	if !changed {
		return content, false, nil
	}
	encoded, err := yaml.Marshal(current)
	if err != nil {
		return nil, false, fmt.Errorf("encode values file failed: %w", err)
	}
	return encoded, true, nil
}

// normalizeEnvironmentPlaceholder 将环境相关的资源名归一成 {env} 模版。
//
// 这么做的原因是 GitOps 替换规则配置发生在“模板层”，而模板层还没有具体环境值：
// 1. 扫描 overlays/dev、overlays/test、overlays/prod 时，文件路径已经被统一成 {env}；
// 2. 如果 metadata.name 仍然保留具体环境后缀，前端会看到一堆看似重复的资源；
// 3. 统一成 {env} 后，模板保存的是稳定规则，执行时再结合发布单环境渲染成真实名称。
func normalizeEnvironmentPlaceholder(value string, environment string) string {
	value = strings.TrimSpace(value)
	environment = strings.TrimSpace(environment)
	if value == "" || environment == "" {
		return value
	}
	pattern := regexp.MustCompile(`(^|[-_./])` + regexp.QuoteMeta(environment) + `($|[-_./])`)
	return pattern.ReplaceAllString(value, "${1}{env}${2}")
}

// collectScalarCandidates 封装当前模块的业务处理逻辑。
func collectScalarCandidates(
	node interface{},
	pointer string,
	filePathTemplate string,
	documentKind string,
	documentName string,
	items *[]gitopsdomain.FieldCandidate,
) {
	switch typed := node.(type) {
	case map[interface{}]interface{}:
		for key, value := range typed {
			token := encodeJSONPointerToken(strings.TrimSpace(fmt.Sprint(key)))
			collectScalarCandidates(value, pointer+"/"+token, filePathTemplate, documentKind, documentName, items)
		}
	case map[string]interface{}:
		for key, value := range typed {
			token := encodeJSONPointerToken(strings.TrimSpace(key))
			collectScalarCandidates(value, pointer+"/"+token, filePathTemplate, documentKind, documentName, items)
		}
	case []interface{}:
		for idx, value := range typed {
			collectScalarCandidates(value, fmt.Sprintf("%s/%d", pointer, idx), filePathTemplate, documentKind, documentName, items)
		}
	case string, bool, int, int64, float64, float32, uint, uint64:
		path := pointer
		if path == "" {
			path = "/"
		}
		sample := strings.TrimSpace(fmt.Sprint(typed))
		if len(sample) > 120 {
			sample = sample[:120] + "..."
		}
		display := strings.Join([]string{
			filepath.Base(filePathTemplate),
			firstNonEmpty(documentKind, "Document"),
			firstNonEmpty(documentName, "-"),
			path,
		}, " / ")
		*items = append(*items, gitopsdomain.FieldCandidate{
			FilePathTemplate: filePathTemplate,
			DocumentKind:     strings.TrimSpace(documentKind),
			DocumentName:     strings.TrimSpace(documentName),
			TargetPath:       path,
			ValueType:        scalarTypeName(typed),
			SampleValue:      sample,
			DisplayName:      display,
		})
	}
}

// firstNonEmpty 封装当前模块的业务处理逻辑。
func firstNonEmpty(values ...string) string {
	for _, item := range values {
		value := strings.TrimSpace(item)
		if value != "" {
			return value
		}
	}
	return ""
}

// applyManifestRulesToFile 封装当前模块的业务处理逻辑。
func applyManifestRulesToFile(content []byte, rules []gitopsdomain.ManifestRule) ([]byte, bool, error) {
	documents := splitYAMLDocuments(string(content))
	if len(documents) == 0 {
		return content, false, nil
	}

	changed := false
	encodedDocs := make([]string, 0, len(documents))
	for _, rawDoc := range documents {
		var doc yaml.MapSlice
		if err := yaml.Unmarshal([]byte(rawDoc), &doc); err != nil {
			return nil, false, fmt.Errorf("parse yaml manifest failed: %w", err)
		}
		kind := strings.TrimSpace(extractKindFromMapSlice(doc))
		name := strings.TrimSpace(extractMetadataNameFromMapSlice(doc))
		docChanged := false
		for _, rule := range rules {
			if !strings.EqualFold(strings.TrimSpace(rule.DocumentKind), kind) {
				continue
			}
			if strings.TrimSpace(rule.DocumentName) != "" && strings.TrimSpace(rule.DocumentName) != name {
				continue
			}
			updated, applied, err := setMapSliceValueByPointer(doc, strings.TrimSpace(rule.TargetPath), strings.TrimSpace(rule.Value))
			if err != nil {
				return nil, false, err
			}
			if applied {
				doc = updated
				docChanged = true
				changed = true
			}
		}
		encoded, err := yaml.Marshal(doc)
		if err != nil {
			return nil, false, fmt.Errorf("encode yaml manifest failed: %w", err)
		}
		if docChanged {
			encodedDocs = append(encodedDocs, strings.TrimRight(string(encoded), "\n"))
			continue
		}
		encodedDocs = append(encodedDocs, strings.TrimRight(string(encoded), "\n"))
	}
	return []byte(strings.Join(encodedDocs, "\n---\n") + "\n"), changed, nil
}

// splitYAMLDocuments 封装当前模块的业务处理逻辑。
func splitYAMLDocuments(raw string) []string {
	normalized := strings.ReplaceAll(raw, "\r\n", "\n")
	lines := strings.Split(normalized, "\n")
	current := make([]string, 0, len(lines))
	result := make([]string, 0)
	flush := func() {
		doc := strings.TrimSpace(strings.Join(current, "\n"))
		if doc != "" {
			result = append(result, doc)
		}
		current = current[:0]
	}
	for _, line := range lines {
		if strings.TrimSpace(line) == "---" {
			flush()
			continue
		}
		current = append(current, line)
	}
	flush()
	return result
}

// setNodeValueByDotPath 封装当前模块的业务处理逻辑。
func setNodeValueByDotPath(node interface{}, path string, newValue string) (interface{}, bool, error) {
	segments := splitValuesPath(path)
	if len(segments) == 0 {
		return node, false, fmt.Errorf("values_path is required")
	}
	return setNodeValueBySegments(node, segments, newValue)
}

// splitValuesPath 封装当前模块的业务处理逻辑。
func splitValuesPath(path string) []string {
	parts := strings.Split(strings.TrimSpace(path), ".")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			result = append(result, part)
		}
	}
	return result
}

// setNodeValueBySegments 封装当前模块的业务处理逻辑。
func setNodeValueBySegments(node interface{}, segments []string, newValue string) (interface{}, bool, error) {
	if len(segments) == 0 {
		current := strings.TrimSpace(fmt.Sprint(node))
		if current == strings.TrimSpace(newValue) {
			return node, false, nil
		}
		return newValue, true, nil
	}

	switch typed := node.(type) {
	case map[interface{}]interface{}:
		key := segments[0]
		for mapKey, mapValue := range typed {
			if strings.TrimSpace(fmt.Sprint(mapKey)) != key {
				continue
			}
			updated, changed, err := setNodeValueBySegments(mapValue, segments[1:], newValue)
			if err != nil {
				return node, false, err
			}
			if changed {
				typed[mapKey] = updated
			}
			return typed, changed, nil
		}
		return node, false, fmt.Errorf("values_path not found: %s", strings.Join(segments, "."))
	case map[string]interface{}:
		key := segments[0]
		value, ok := typed[key]
		if !ok {
			return node, false, fmt.Errorf("values_path not found: %s", strings.Join(segments, "."))
		}
		updated, changed, err := setNodeValueBySegments(value, segments[1:], newValue)
		if err != nil {
			return node, false, err
		}
		if changed {
			typed[key] = updated
		}
		return typed, changed, nil
	case []interface{}:
		indexToken := strings.Trim(strings.TrimSpace(segments[0]), "[]")
		indexValue, err := parseSliceIndex(indexToken)
		if err != nil {
			return node, false, err
		}
		if indexValue < 0 || indexValue >= len(typed) {
			return node, false, fmt.Errorf("values_path index out of range: %s", segments[0])
		}
		updated, changed, err := setNodeValueBySegments(typed[indexValue], segments[1:], newValue)
		if err != nil {
			return node, false, err
		}
		if changed {
			typed[indexValue] = updated
		}
		return typed, changed, nil
	default:
		return node, false, fmt.Errorf("values_path is not replaceable: %s", strings.Join(segments, "."))
	}
}

// encodeJSONPointerToken 封装当前模块的业务处理逻辑。
func encodeJSONPointerToken(value string) string {
	return strings.NewReplacer("~", "~0", "/", "~1").Replace(value)
}

// decodeJSONPointerToken 封装当前模块的业务处理逻辑。
func decodeJSONPointerToken(value string) string {
	return strings.NewReplacer("~1", "/", "~0", "~").Replace(value)
}

// extractDocumentIdentity 封装当前模块的业务处理逻辑。
func extractDocumentIdentity(node interface{}) (kind string, name string) {
	switch typed := node.(type) {
	case map[interface{}]interface{}:
		for key, value := range typed {
			keyText := strings.TrimSpace(fmt.Sprint(key))
			switch keyText {
			case "kind":
				kind = strings.TrimSpace(fmt.Sprint(value))
			case "metadata":
				switch metadata := value.(type) {
				case map[interface{}]interface{}:
					for mk, mv := range metadata {
						if strings.TrimSpace(fmt.Sprint(mk)) == "name" {
							name = strings.TrimSpace(fmt.Sprint(mv))
						}
					}
				}
			}
		}
	}
	return strings.TrimSpace(kind), strings.TrimSpace(name)
}

// scalarTypeName 封装当前模块的业务处理逻辑。
func scalarTypeName(value interface{}) string {
	switch value.(type) {
	case bool:
		return "bool"
	case int, int64, float64, float32, uint, uint64:
		return "number"
	default:
		return "string"
	}
}

// extractKindFromMapSlice 封装当前模块的业务处理逻辑。
func extractKindFromMapSlice(doc yaml.MapSlice) string {
	value, ok := mapSliceString(doc, "kind")
	if !ok {
		return ""
	}
	return value
}

// extractMetadataNameFromMapSlice 封装当前模块的业务处理逻辑。
func extractMetadataNameFromMapSlice(doc yaml.MapSlice) string {
	value, ok := mapSliceGet(doc, "metadata")
	if !ok {
		return ""
	}
	items, ok := value.(yaml.MapSlice)
	if !ok {
		return ""
	}
	name, _ := mapSliceString(items, "name")
	return name
}

// setMapSliceValueByPointer 封装当前模块的业务处理逻辑。
func setMapSliceValueByPointer(doc yaml.MapSlice, pointer string, newValue string) (yaml.MapSlice, bool, error) {
	if strings.TrimSpace(pointer) == "" || strings.TrimSpace(pointer) == "/" {
		return doc, false, fmt.Errorf("yaml target path is required")
	}
	segments := strings.Split(strings.TrimPrefix(strings.TrimSpace(pointer), "/"), "/")
	decoded := make([]string, 0, len(segments))
	for _, item := range segments {
		if strings.TrimSpace(item) == "" {
			continue
		}
		decoded = append(decoded, decodeJSONPointerToken(item))
	}
	if len(decoded) == 0 {
		return doc, false, fmt.Errorf("yaml target path is required")
	}

	updated, changed, err := setNodeValueByPointer(doc, decoded, newValue)
	if err != nil {
		return doc, false, err
	}
	result, ok := updated.(yaml.MapSlice)
	if !ok {
		return doc, false, fmt.Errorf("yaml root document must be a map")
	}
	return result, changed, nil
}

// setNodeValueByPointer 封装当前模块的业务处理逻辑。
func setNodeValueByPointer(node interface{}, segments []string, newValue string) (interface{}, bool, error) {
	if len(segments) == 0 {
		current := strings.TrimSpace(fmt.Sprint(node))
		if current == strings.TrimSpace(newValue) {
			return node, false, nil
		}
		return newValue, true, nil
	}

	switch typed := node.(type) {
	case yaml.MapSlice:
		key := segments[0]
		for idx, item := range typed {
			if strings.TrimSpace(fmt.Sprint(item.Key)) != key {
				continue
			}
			updated, changed, err := setNodeValueByPointer(item.Value, segments[1:], newValue)
			if err != nil {
				return node, false, err
			}
			if changed {
				typed[idx].Value = updated
			}
			return typed, changed, nil
		}
		return node, false, fmt.Errorf("yaml target path not found: %s", "/"+strings.Join(segments, "/"))
	case []interface{}:
		indexValue, err := parseSliceIndex(segments[0])
		if err != nil {
			return node, false, err
		}
		if indexValue < 0 || indexValue >= len(typed) {
			return node, false, fmt.Errorf("yaml target index out of range: %s", segments[0])
		}
		updated, changed, err := setNodeValueByPointer(typed[indexValue], segments[1:], newValue)
		if err != nil {
			return node, false, err
		}
		if changed {
			typed[indexValue] = updated
		}
		return typed, changed, nil
	default:
		if len(segments) > 0 {
			return node, false, fmt.Errorf("yaml target path is not replaceable: %s", "/"+strings.Join(segments, "/"))
		}
		current := strings.TrimSpace(fmt.Sprint(node))
		if current == strings.TrimSpace(newValue) {
			return node, false, nil
		}
		return newValue, true, nil
	}
}

// parseSliceIndex 解析输入内容并返回结构化结果。
func parseSliceIndex(value string) (int, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, fmt.Errorf("yaml target index is required")
	}
	var index int
	_, err := fmt.Sscanf(value, "%d", &index)
	if err != nil {
		return 0, fmt.Errorf("invalid yaml target index: %s", value)
	}
	return index, nil
}

// mapSliceSet 封装当前模块的业务处理逻辑。
func mapSliceSet(items yaml.MapSlice, key string, value interface{}) yaml.MapSlice {
	for idx, item := range items {
		if strings.EqualFold(fmt.Sprint(item.Key), key) {
			items[idx].Value = value
			return items
		}
	}
	return append(items, yaml.MapItem{Key: key, Value: value})
}

// normalizeCommitMessageFields 标准化输入值，保证后续逻辑使用统一格式。
func normalizeCommitMessageFields(fields map[string]string) map[string]string {
	result := make(map[string]string, len(fields))
	for key, value := range fields {
		normalizedKey := strings.ToLower(strings.TrimSpace(key))
		if normalizedKey == "" {
			continue
		}
		result[normalizedKey] = strings.TrimSpace(value)
	}
	return result
}

// renderCommitMessageTemplate 查询并返回指定资源数据。
func renderCommitMessageTemplate(template string, fields map[string]string) string {
	return commitTemplateTokenPattern.ReplaceAllStringFunc(template, func(token string) string {
		matches := commitTemplateTokenPattern.FindStringSubmatch(token)
		if len(matches) != 2 {
			return token
		}
		key := strings.ToLower(strings.TrimSpace(matches[1]))
		if key == "" {
			return token
		}
		if value, ok := fields[key]; ok {
			return value
		}
		return token
	})
}

// CheckScanPath 检查配置的扫描路径在 master 分支中是否存在。
// 返回解析后的路径模板、是否存在、可能的错误。
func (s *Service) CheckScanPath(ctx context.Context, appKey string, gitopsType string) (string, bool, error) {
	status, err := s.GetStatus(ctx)
	if err != nil {
		return "", false, err
	}
	if !status.Enabled || !status.PathExists || !status.IsGitRepo {
		return "", false, nil
	}

	appKey = strings.TrimSpace(appKey)
	if appKey == "" {
		return "", false, nil
	}

	scanRoot := strings.TrimSpace(status.LocalRoot)
	cleanup := func() {}
	scanRoot, cleanup, err = s.prepareConfigCandidateScanRoot(ctx, scanRoot)
	if err != nil {
		return "", false, err
	}
	defer cleanup()

	switch strings.ToLower(strings.TrimSpace(gitopsType)) {
	case "helm":
		helmPathTemplate := s.helmScanPathValue()
		helmPathTemplate = strings.ReplaceAll(helmPathTemplate, "{app_key}", appKey)
		helmResolved := filepath.Join(scanRoot, filepath.FromSlash(helmPathTemplate))
		if info, statErr := os.Stat(helmResolved); statErr == nil && info.IsDir() {
			return helmPathTemplate, true, nil
		}
		return helmPathTemplate, false, nil
	default:
		resolvedAppDir, resolveErr := s.resolveAppDirectory(scanRoot, appKey)
		if resolveErr != nil {
			return "", false, resolveErr
		}
		if resolvedAppDir == "" {
			return "", false, nil
		}
		kustomizePathTemplate := s.kustomizeScanPathValue()
		kustomizePathTemplate = strings.ReplaceAll(kustomizePathTemplate, "{app_key}", resolvedAppDir)
		kustomizePathTemplate = strings.Replace(kustomizePathTemplate, "/{env}", "", 1)
		kustomizePathTemplate = strings.Replace(kustomizePathTemplate, "/{env}/", "/", 1)
		kustomizeResolved := filepath.Join(scanRoot, filepath.FromSlash(kustomizePathTemplate))
		if info, statErr := os.Stat(kustomizeResolved); statErr == nil && info.IsDir() {
			return kustomizePathTemplate, true, nil
		}
		return kustomizePathTemplate, false, nil
	}
}
