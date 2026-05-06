package usecase

import (
	"context"
	"errors"
	"strings"

	agentdomain "gos/internal/domain/agent"
)

type managedScriptCache struct {
	items   map[string]agentdomain.Script
	missing map[string]struct{}
}

// newManagedScriptCache 封装当前模块的业务处理逻辑。
func newManagedScriptCache() *managedScriptCache {
	return &managedScriptCache{
		items:   make(map[string]agentdomain.Script),
		missing: make(map[string]struct{}),
	}
}

// syncManagedScriptSnapshotsForTasks 同步外部或内部状态数据。
func syncManagedScriptSnapshotsForTasks(
	ctx context.Context,
	repo agentdomain.Repository,
	items []agentdomain.Task,
	cache *managedScriptCache,
) ([]agentdomain.Task, error) {
	if len(items) == 0 {
		return items, nil
	}
	if cache == nil {
		cache = newManagedScriptCache()
	}
	result := make([]agentdomain.Task, 0, len(items))
	for _, item := range items {
		synced, err := syncManagedScriptSnapshotForTask(ctx, repo, item, cache)
		if err != nil {
			return nil, err
		}
		result = append(result, synced)
	}
	return result, nil
}

// syncManagedScriptSnapshotForTask 同步外部或内部状态数据。
func syncManagedScriptSnapshotForTask(
	ctx context.Context,
	repo agentdomain.Repository,
	item agentdomain.Task,
	cache *managedScriptCache,
) (agentdomain.Task, error) {
	scriptID := strings.TrimSpace(item.ScriptID)
	if scriptID == "" || repo == nil {
		return item, nil
	}
	if cache == nil {
		cache = newManagedScriptCache()
	}
	script, found, err := loadManagedScriptSnapshot(ctx, repo, scriptID, cache)
	if err != nil {
		return item, err
	}
	if !found {
		return item, nil
	}
	applyManagedScriptSnapshot(&item, script)
	return item, nil
}

// loadManagedScriptSnapshot 封装当前模块的业务处理逻辑。
func loadManagedScriptSnapshot(
	ctx context.Context,
	repo agentdomain.Repository,
	scriptID string,
	cache *managedScriptCache,
) (agentdomain.Script, bool, error) {
	scriptID = strings.TrimSpace(scriptID)
	if scriptID == "" || repo == nil {
		return agentdomain.Script{}, false, nil
	}
	if cache == nil {
		cache = newManagedScriptCache()
	}
	if item, ok := cache.items[scriptID]; ok {
		return item, true, nil
	}
	if _, ok := cache.missing[scriptID]; ok {
		return agentdomain.Script{}, false, nil
	}
	item, err := repo.GetScriptByID(ctx, scriptID)
	if err != nil {
		if errors.Is(err, agentdomain.ErrScriptNotFound) {
			cache.missing[scriptID] = struct{}{}
			return agentdomain.Script{}, false, nil
		}
		return agentdomain.Script{}, false, err
	}
	cache.items[scriptID] = item
	return item, true, nil
}

// applyManagedScriptSnapshot 封装当前模块的业务处理逻辑。
func applyManagedScriptSnapshot(task *agentdomain.Task, script agentdomain.Script) bool {
	if task == nil {
		return false
	}
	changed := false
	taskType := strings.TrimSpace(script.TaskType)
	shellType := firstNonEmptyAgentString(strings.TrimSpace(script.ShellType), "sh")
	scriptName := strings.TrimSpace(script.Name)
	scriptPath := strings.TrimSpace(script.ScriptPath)
	scriptText := strings.TrimSpace(script.ScriptText)

	if task.TaskType != taskType {
		task.TaskType = taskType
		changed = true
	}
	if task.ShellType != shellType {
		task.ShellType = shellType
		changed = true
	}
	if task.ScriptName != scriptName {
		task.ScriptName = scriptName
		changed = true
	}
	if task.ScriptPath != scriptPath {
		task.ScriptPath = scriptPath
		changed = true
	}
	if strings.TrimSpace(task.ScriptText) != scriptText {
		task.ScriptText = scriptText
		changed = true
	}
	return changed
}
