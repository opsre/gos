package httpapi

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"gos/internal/application/usecase"
	agentdomain "gos/internal/domain/agent"
)

type AgentHandler struct {
	manager       *usecase.AgentManager
	taskManager   *usecase.AgentTaskManager
	scriptManager *usecase.AgentScriptManager
	authz         RequestAuthorizer
}

// NewAgentHandler 创建并返回对应组件实例。
func NewAgentHandler(manager *usecase.AgentManager, taskManager *usecase.AgentTaskManager, scriptManager *usecase.AgentScriptManager, authz RequestAuthorizer) *AgentHandler {
	return &AgentHandler{manager: manager, taskManager: taskManager, scriptManager: scriptManager, authz: authz}
}

// RegisterPublicRoutes 封装当前模块的业务处理逻辑。
func (h *AgentHandler) RegisterPublicRoutes(router gin.IRouter) {
	if h == nil {
		return
	}
	router.POST("/agent/register", h.Register)
	router.POST("/agent/heartbeat", h.Heartbeat)
	router.POST("/agent/tasks/poll", h.PollTask)
	router.POST("/agent/tasks/:id/start", h.StartTask)
	router.POST("/agent/tasks/:id/finish", h.FinishTask)
}

// RegisterRoutes 封装当前模块的业务处理逻辑。
func (h *AgentHandler) RegisterRoutes(router gin.IRouter) {
	if h == nil {
		return
	}
	router.GET("/agents", h.List)
	router.GET("/agents/bootstrap-config", h.BootstrapConfig)
	router.GET("/agents/:id", h.Get)
	router.GET("/agents/:id/config", h.Config)
	router.GET("/agent-tasks", h.ListAllTasks)
	router.GET("/agents/:id/tasks", h.ListTasks)
	router.GET("/agent-scripts", h.ListScripts)
	router.GET("/agent-scripts/:id", h.GetScript)
	router.POST("/agents", h.Create)
	router.PUT("/agents/:id", h.Update)
	router.DELETE("/agents/:id", h.Delete)
	router.POST("/agent-tasks", h.CreateUnassignedTask)
	router.POST("/agent-tasks/:taskID/execute", h.ExecuteStandaloneTask)
	router.POST("/agents/:id/tasks", h.CreateTask)
	router.POST("/agents/:id/tasks/:taskID/execute", h.ExecuteTask)
	router.POST("/agents/:id/tasks/:taskID/stop", h.StopTask)
	router.POST("/agents/:id/tasks/:taskID/resume", h.ResumeTask)
	router.DELETE("/agents/:id/tasks/:taskID", h.DeleteTask)
	router.PUT("/agents/:id/tasks/:taskID", h.UpdateTask)
	router.PUT("/agent-tasks/:taskID", h.UpdateTemporaryTask)
	router.DELETE("/agent-tasks/:taskID", h.DeleteTemporaryTask)
	router.PUT("/resident-tasks/:taskID", h.UpdateResidentTask)
	router.DELETE("/resident-tasks/:taskID", h.DeleteResidentTask)
	router.POST("/agent-scripts", h.CreateScript)
	router.PUT("/agent-scripts/:id", h.UpdateScript)
	router.DELETE("/agent-scripts/:id", h.DeleteScript)
	router.POST("/agents/bootstrap-token/reset", h.ResetBootstrapToken)
	router.POST("/agents/:id/reset-token", h.ResetToken)
	router.POST("/agents/:id/enable", h.Enable)
	router.POST("/agents/:id/disable", h.Disable)
	router.POST("/agents/:id/maintenance", h.Maintenance)
}

type AgentListResponse struct {
	Data     []usecase.AgentOutput `json:"data"`
	Page     int                   `json:"page"`
	PageSize int                   `json:"page_size"`
	Total    int64                 `json:"total"`
}

type AgentDataResponse struct {
	Data usecase.AgentOutput `json:"data"`
}

type AgentRegisterDataResponse struct {
	Data usecase.AgentRegisterOutput `json:"data"`
}

type AgentConfigResponse struct {
	Data usecase.AgentInstallConfigOutput `json:"data"`
}

type AgentTaskListResponse struct {
	Data     []usecase.AgentTaskOutput `json:"data"`
	Page     int                       `json:"page"`
	PageSize int                       `json:"page_size"`
	Total    int64                     `json:"total"`
}

type AgentTaskDataResponse struct {
	Data any `json:"data"`
}

type AgentScriptListResponse struct {
	Data     []usecase.AgentScriptOutput `json:"data"`
	Page     int                         `json:"page"`
	PageSize int                         `json:"page_size"`
	Total    int64                       `json:"total"`
}

type AgentScriptDataResponse struct {
	Data usecase.AgentScriptOutput `json:"data"`
}

type upsertAgentRequest struct {
	AgentCode       string   `json:"agent_code"`
	Name            string   `json:"name"`
	EnvironmentCode string   `json:"environment_code"`
	WorkDir         string   `json:"work_dir"`
	Tags            []string `json:"tags"`
	Status          string   `json:"status"`
	Remark          string   `json:"remark"`
}

type agentHeartbeatRequest struct {
	AgentCode          string   `json:"agent_code"`
	Token              string   `json:"token"`
	Hostname           string   `json:"hostname"`
	HostIP             string   `json:"host_ip"`
	AgentVersion       string   `json:"agent_version"`
	OS                 string   `json:"os"`
	Arch               string   `json:"arch"`
	WorkDir            string   `json:"work_dir"`
	Tags               []string `json:"tags"`
	CurrentTaskID      string   `json:"current_task_id"`
	CurrentTaskName    string   `json:"current_task_name"`
	CurrentTaskType    string   `json:"current_task_type"`
	CurrentTaskStarted string   `json:"current_task_started_at"`
	LastTaskStatus     string   `json:"last_task_status"`
	LastTaskSummary    string   `json:"last_task_summary"`
	LastTaskFinishedAt string   `json:"last_task_finished_at"`
}

type agentRegisterRequest struct {
	RegistrationToken string   `json:"registration_token"`
	MachineID         string   `json:"machine_id"`
	Name              string   `json:"name"`
	EnvironmentCode   string   `json:"environment_code"`
	Hostname          string   `json:"hostname"`
	HostIP            string   `json:"host_ip"`
	AgentVersion      string   `json:"agent_version"`
	OS                string   `json:"os"`
	Arch              string   `json:"arch"`
	WorkDir           string   `json:"work_dir"`
	Tags              []string `json:"tags"`
}

type createAgentTaskRequest struct {
	Name           string            `json:"name"`
	TaskMode       string            `json:"task_mode"`
	TaskType       string            `json:"task_type"`
	ShellType      string            `json:"shell_type"`
	WorkDir        string            `json:"work_dir"`
	ScriptID       string            `json:"script_id"`
	ScriptPath     string            `json:"script_path"`
	ScriptText     string            `json:"script_text"`
	Variables      map[string]string `json:"variables"`
	TargetAgentIDs []string          `json:"target_agent_ids"`
	TimeoutSec     int               `json:"timeout_sec"`
}

type upsertAgentScriptRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	TaskType    string `json:"task_type"`
	ShellType   string `json:"shell_type"`
	ScriptPath  string `json:"script_path"`
	ScriptText  string `json:"script_text"`
}

type agentTaskPollRequest struct {
	AgentCode string `json:"agent_code"`
	Token     string `json:"token"`
}

type finishAgentTaskRequest struct {
	AgentCode     string `json:"agent_code"`
	Token         string `json:"token"`
	Status        string `json:"status"`
	ExitCode      int    `json:"exit_code"`
	StdoutText    string `json:"stdout_text"`
	StderrText    string `json:"stderr_text"`
	FailureReason string `json:"failure_reason"`
}

// List 查询资源列表。
// @Summary      查询资源列表
// @Description  查询资源列表，并按统一响应结构返回处理结果。
// @Tags         agents
// @Produce      json
// @Success      200  {object}  GenericResponse
// @Failure      400  {object}  ErrorResponse
// @Failure      401  {object}  ErrorResponse
// @Failure      403  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Failure      500  {object}  ErrorResponse
// @Router       /agents [get]
func (h *AgentHandler) List(c *gin.Context) {
	if !ensureAnyPermission(c, h.authz, "component.agent.view", "component.agent.manage") {
		return
	}
	if h.manager == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "agent manager is not configured"})
		return
	}
	page, err := parsePositiveInt(c, "page")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	pageSize, err := parsePositiveInt(c, "page_size")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	output, err := h.manager.List(c.Request.Context(), agentdomain.ListFilter{
		Keyword:      strings.TrimSpace(c.Query("keyword")),
		Status:       agentdomain.Status(strings.TrimSpace(c.Query("status"))),
		RuntimeState: agentdomain.RuntimeState(strings.TrimSpace(c.Query("runtime_state"))),
		Page:         page,
		PageSize:     pageSize,
	})
	if err != nil {
		writeAgentHTTPError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"data":      output.Items,
		"page":      resolvedPage(page),
		"page_size": resolvedPageSize(pageSize),
		"total":     output.Total,
	})
}

// Get 获取资源详情。
// @Summary      获取资源详情
// @Description  获取资源详情，并按统一响应结构返回处理结果。
// @Tags         agents
// @Produce      json
// @Param        id  path  string  true  "资源 ID"
// @Success      200  {object}  GenericResponse
// @Failure      400  {object}  ErrorResponse
// @Failure      401  {object}  ErrorResponse
// @Failure      403  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Failure      500  {object}  ErrorResponse
// @Router       /agents/{id} [get]
func (h *AgentHandler) Get(c *gin.Context) {
	if !ensureAnyPermission(c, h.authz, "component.agent.view", "component.agent.manage") {
		return
	}
	if h.manager == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "agent manager is not configured"})
		return
	}
	output, err := h.manager.Get(c.Request.Context(), c.Param("id"))
	if err != nil {
		writeAgentHTTPError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": output})
}

// Create 创建资源。
// @Summary      创建资源
// @Description  创建资源，并按统一响应结构返回处理结果。
// @Tags         agents
// @Accept       json
// @Produce      json
// @Success      200  {object}  GenericResponse
// @Failure      400  {object}  ErrorResponse
// @Failure      401  {object}  ErrorResponse
// @Failure      403  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Failure      500  {object}  ErrorResponse
// @Router       /agents [post]
func (h *AgentHandler) Create(c *gin.Context) {
	if !ensurePermission(c, h.authz, "component.agent.manage", "", "") {
		return
	}
	if h.manager == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "agent manager is not configured"})
		return
	}
	var req upsertAgentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	output, err := h.manager.Create(c.Request.Context(), usecase.CreateAgentInput{
		AgentCode:       req.AgentCode,
		Name:            req.Name,
		EnvironmentCode: req.EnvironmentCode,
		WorkDir:         req.WorkDir,
		Tags:            req.Tags,
		Status:          agentdomain.Status(strings.TrimSpace(req.Status)),
		Remark:          req.Remark,
	})
	if err != nil {
		writeAgentHTTPError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": output})
}

// Update 更新资源。
// @Summary      更新资源
// @Description  更新资源，并按统一响应结构返回处理结果。
// @Tags         agents
// @Accept       json
// @Produce      json
// @Param        id  path  string  true  "资源 ID"
// @Success      200  {object}  GenericResponse
// @Failure      400  {object}  ErrorResponse
// @Failure      401  {object}  ErrorResponse
// @Failure      403  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Failure      500  {object}  ErrorResponse
// @Router       /agents/{id} [put]
func (h *AgentHandler) Update(c *gin.Context) {
	if !ensurePermission(c, h.authz, "component.agent.manage", "", "") {
		return
	}
	if h.manager == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "agent manager is not configured"})
		return
	}
	var req upsertAgentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	output, err := h.manager.Update(c.Request.Context(), c.Param("id"), usecase.UpdateAgentInput{
		AgentCode:       req.AgentCode,
		Name:            req.Name,
		EnvironmentCode: req.EnvironmentCode,
		WorkDir:         req.WorkDir,
		Tags:            req.Tags,
		Status:          agentdomain.Status(strings.TrimSpace(req.Status)),
		Remark:          req.Remark,
	})
	if err != nil {
		writeAgentHTTPError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": output})
}

// Delete 删除资源。
// @Summary      删除资源
// @Description  删除资源，并按统一响应结构返回处理结果。
// @Tags         agents
// @Produce      json
// @Param        id  path  string  true  "资源 ID"
// @Success      200  {object}  GenericResponse
// @Failure      400  {object}  ErrorResponse
// @Failure      401  {object}  ErrorResponse
// @Failure      403  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Failure      500  {object}  ErrorResponse
// @Router       /agents/{id} [delete]
func (h *AgentHandler) Delete(c *gin.Context) {
	if !ensurePermission(c, h.authz, "component.agent.manage", "", "") {
		return
	}
	if h.manager == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "agent manager is not configured"})
		return
	}
	if err := h.manager.Delete(c.Request.Context(), c.Param("id")); err != nil {
		writeAgentHTTPError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// Config 处理Config接口。
// @Summary      处理Config接口
// @Description  处理Config接口，并按统一响应结构返回处理结果。
// @Tags         agents
// @Produce      json
// @Param        id  path  string  true  "资源 ID"
// @Success      200  {object}  GenericResponse
// @Failure      400  {object}  ErrorResponse
// @Failure      401  {object}  ErrorResponse
// @Failure      403  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Failure      500  {object}  ErrorResponse
// @Router       /agents/{id}/config [get]
func (h *AgentHandler) Config(c *gin.Context) {
	if !ensureAnyPermission(c, h.authz, "component.agent.view", "component.agent.manage") {
		return
	}
	if h.manager == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "agent manager is not configured"})
		return
	}
	output, err := h.manager.BuildInstallConfig(c.Request.Context(), c.Param("id"), resolveAgentBaseURL(c))
	if err != nil {
		writeAgentHTTPError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": output})
}

// BootstrapConfig 处理Bootstrap Config接口。
// @Summary      处理Bootstrap Config接口
// @Description  处理Bootstrap Config接口，并按统一响应结构返回处理结果。
// @Tags         agents
// @Produce      json
// @Success      200  {object}  GenericResponse
// @Failure      400  {object}  ErrorResponse
// @Failure      401  {object}  ErrorResponse
// @Failure      403  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Failure      500  {object}  ErrorResponse
// @Router       /agents/bootstrap-config [get]
func (h *AgentHandler) BootstrapConfig(c *gin.Context) {
	if !ensurePermission(c, h.authz, "component.agent.manage", "", "") {
		return
	}
	if h.manager == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "agent manager is not configured"})
		return
	}
	output, err := h.manager.BuildBootstrapConfig(c.Request.Context(), resolveAgentBaseURL(c))
	if err != nil {
		writeAgentHTTPError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": output})
}

// ResetBootstrapToken 重置Bootstrap Token。
// @Summary      重置Bootstrap Token
// @Description  重置Bootstrap Token，并按统一响应结构返回处理结果。
// @Tags         agents
// @Accept       json
// @Produce      json
// @Success      200  {object}  GenericResponse
// @Failure      400  {object}  ErrorResponse
// @Failure      401  {object}  ErrorResponse
// @Failure      403  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Failure      500  {object}  ErrorResponse
// @Router       /agents/bootstrap-token/reset [post]
func (h *AgentHandler) ResetBootstrapToken(c *gin.Context) {
	if !ensurePermission(c, h.authz, "component.agent.manage", "", "") {
		return
	}
	if h.manager == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "agent manager is not configured"})
		return
	}
	output, err := h.manager.ResetBootstrapToken(c.Request.Context(), resolveAgentBaseURL(c))
	if err != nil {
		writeAgentHTTPError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": output})
}

// ResetToken 重置Token。
// @Summary      重置Token
// @Description  重置Token，并按统一响应结构返回处理结果。
// @Tags         agents
// @Accept       json
// @Produce      json
// @Param        id  path  string  true  "资源 ID"
// @Success      200  {object}  GenericResponse
// @Failure      400  {object}  ErrorResponse
// @Failure      401  {object}  ErrorResponse
// @Failure      403  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Failure      500  {object}  ErrorResponse
// @Router       /agents/{id}/reset-token [post]
func (h *AgentHandler) ResetToken(c *gin.Context) {
	if !ensurePermission(c, h.authz, "component.agent.manage", "", "") {
		return
	}
	if h.manager == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "agent manager is not configured"})
		return
	}
	output, err := h.manager.ResetToken(c.Request.Context(), c.Param("id"))
	if err != nil {
		writeAgentHTTPError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": output})
}

// ListTasks 查询Tasks列表。
// @Summary      查询Tasks列表
// @Description  查询Tasks列表，并按统一响应结构返回处理结果。
// @Tags         agents
// @Produce      json
// @Param        id  path  string  true  "资源 ID"
// @Success      200  {object}  GenericResponse
// @Failure      400  {object}  ErrorResponse
// @Failure      401  {object}  ErrorResponse
// @Failure      403  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Failure      500  {object}  ErrorResponse
// @Router       /agents/{id}/tasks [get]
func (h *AgentHandler) ListTasks(c *gin.Context) {
	if !ensureAnyPermission(c, h.authz, "component.agent.view", "component.agent.manage") {
		return
	}
	if h.taskManager == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "agent task manager is not configured"})
		return
	}
	page, err := parsePositiveInt(c, "page")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	pageSize, err := parsePositiveInt(c, "page_size")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	output, err := h.taskManager.ListByAgent(c.Request.Context(), c.Param("id"), page, pageSize)
	if err != nil {
		writeAgentHTTPError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"data":      output.Items,
		"page":      resolvedPage(page),
		"page_size": resolvedPageSize(pageSize),
		"total":     output.Total,
	})
}

// ListAllTasks 查询All Tasks列表。
// @Summary      查询All Tasks列表
// @Description  查询All Tasks列表，并按统一响应结构返回处理结果。
// @Tags         agents
// @Produce      json
// @Success      200  {object}  GenericResponse
// @Failure      400  {object}  ErrorResponse
// @Failure      401  {object}  ErrorResponse
// @Failure      403  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Failure      500  {object}  ErrorResponse
// @Router       /agent-tasks [get]
func (h *AgentHandler) ListAllTasks(c *gin.Context) {
	if !ensureAnyPermission(c, h.authz, "component.agent.view", "component.agent.manage") {
		return
	}
	if h.taskManager == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "agent task manager is not configured"})
		return
	}
	page, err := parsePositiveInt(c, "page")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	pageSize, err := parsePositiveInt(c, "page_size")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	output, err := h.taskManager.List(c.Request.Context(), page, pageSize)
	if err != nil {
		writeAgentHTTPError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"data":      output.Items,
		"page":      resolvedPage(page),
		"page_size": resolvedPageSize(pageSize),
		"total":     output.Total,
	})
}

// ListScripts 查询Scripts列表。
// @Summary      查询Scripts列表
// @Description  查询Scripts列表，并按统一响应结构返回处理结果。
// @Tags         agents
// @Produce      json
// @Success      200  {object}  GenericResponse
// @Failure      400  {object}  ErrorResponse
// @Failure      401  {object}  ErrorResponse
// @Failure      403  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Failure      500  {object}  ErrorResponse
// @Router       /agent-scripts [get]
func (h *AgentHandler) ListScripts(c *gin.Context) {
	if !ensureAnyPermission(c, h.authz, "component.agent.view", "component.agent.manage") {
		return
	}
	if h.scriptManager == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "agent script manager is not configured"})
		return
	}
	page, err := parsePositiveInt(c, "page")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	pageSize, err := parsePositiveInt(c, "page_size")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	output, err := h.scriptManager.List(c.Request.Context(), agentdomain.ScriptListFilter{
		Keyword:  strings.TrimSpace(c.Query("keyword")),
		TaskType: agentdomain.TaskType(strings.TrimSpace(c.Query("task_type"))),
		Page:     page,
		PageSize: pageSize,
	})
	if err != nil {
		writeAgentHTTPError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"data":      output.Items,
		"page":      resolvedPage(page),
		"page_size": resolvedPageSize(pageSize),
		"total":     output.Total,
	})
}

// GetScript 获取Script详情。
// @Summary      获取Script详情
// @Description  获取Script详情，并按统一响应结构返回处理结果。
// @Tags         agents
// @Produce      json
// @Param        id  path  string  true  "资源 ID"
// @Success      200  {object}  GenericResponse
// @Failure      400  {object}  ErrorResponse
// @Failure      401  {object}  ErrorResponse
// @Failure      403  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Failure      500  {object}  ErrorResponse
// @Router       /agent-scripts/{id} [get]
func (h *AgentHandler) GetScript(c *gin.Context) {
	if !ensureAnyPermission(c, h.authz, "component.agent.view", "component.agent.manage") {
		return
	}
	if h.scriptManager == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "agent script manager is not configured"})
		return
	}
	output, err := h.scriptManager.Get(c.Request.Context(), c.Param("id"))
	if err != nil {
		writeAgentHTTPError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": output})
}

// CreateTask 创建Task。
// @Summary      创建Task
// @Description  创建Task，并按统一响应结构返回处理结果。
// @Tags         agents
// @Accept       json
// @Produce      json
// @Param        id  path  string  true  "资源 ID"
// @Success      200  {object}  GenericResponse
// @Failure      400  {object}  ErrorResponse
// @Failure      401  {object}  ErrorResponse
// @Failure      403  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Failure      500  {object}  ErrorResponse
// @Router       /agents/{id}/tasks [post]
func (h *AgentHandler) CreateTask(c *gin.Context) {
	if !ensurePermission(c, h.authz, "component.agent.manage", "", "") {
		return
	}
	if h.taskManager == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "agent task manager is not configured"})
		return
	}
	var req createAgentTaskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	createdBy := ""
	if currentUser, ok := getCurrentUser(c); ok {
		createdBy = strings.TrimSpace(currentUser.DisplayName)
		if createdBy == "" {
			createdBy = strings.TrimSpace(currentUser.Username)
		}
		if createdBy == "" {
			createdBy = strings.TrimSpace(currentUser.ID)
		}
	}
	output, err := h.taskManager.Create(c.Request.Context(), usecase.CreateAgentTaskInput{
		AgentID:        c.Param("id"),
		TargetAgentIDs: req.TargetAgentIDs,
		Name:           req.Name,
		TaskMode:       req.TaskMode,
		TaskType:       req.TaskType,
		ShellType:      req.ShellType,
		WorkDir:        req.WorkDir,
		ScriptID:       req.ScriptID,
		ScriptPath:     req.ScriptPath,
		ScriptText:     req.ScriptText,
		Variables:      req.Variables,
		TimeoutSec:     req.TimeoutSec,
		CreatedBy:      createdBy,
	})
	if err != nil {
		writeAgentHTTPError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": output})
}

// CreateUnassignedTask 创建Unassigned Task。
// @Summary      创建Unassigned Task
// @Description  创建Unassigned Task，并按统一响应结构返回处理结果。
// @Tags         agents
// @Accept       json
// @Produce      json
// @Success      200  {object}  GenericResponse
// @Failure      400  {object}  ErrorResponse
// @Failure      401  {object}  ErrorResponse
// @Failure      403  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Failure      500  {object}  ErrorResponse
// @Router       /agent-tasks [post]
func (h *AgentHandler) CreateUnassignedTask(c *gin.Context) {
	if !ensurePermission(c, h.authz, "component.agent.manage", "", "") {
		return
	}
	if h.taskManager == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "agent task manager is not configured"})
		return
	}
	var req createAgentTaskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	createdBy := ""
	if currentUser, ok := getCurrentUser(c); ok {
		createdBy = strings.TrimSpace(currentUser.DisplayName)
		if createdBy == "" {
			createdBy = strings.TrimSpace(currentUser.Username)
		}
		if createdBy == "" {
			createdBy = strings.TrimSpace(currentUser.ID)
		}
	}
	output, err := h.taskManager.Create(c.Request.Context(), usecase.CreateAgentTaskInput{
		TargetAgentIDs: req.TargetAgentIDs,
		Name:           req.Name,
		TaskMode:       req.TaskMode,
		TaskType:       req.TaskType,
		ShellType:      req.ShellType,
		WorkDir:        req.WorkDir,
		ScriptID:       req.ScriptID,
		ScriptPath:     req.ScriptPath,
		ScriptText:     req.ScriptText,
		Variables:      req.Variables,
		TimeoutSec:     req.TimeoutSec,
		CreatedBy:      createdBy,
	})
	if err != nil {
		writeAgentHTTPError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": output})
}

// UpdateTask 更新Task。
// @Summary      更新Task
// @Description  更新Task，并按统一响应结构返回处理结果。
// @Tags         agents
// @Accept       json
// @Produce      json
// @Param        id  path  string  true  "资源 ID"
// @Param        taskID  path  string  true  "任务 ID"
// @Success      200  {object}  GenericResponse
// @Failure      400  {object}  ErrorResponse
// @Failure      401  {object}  ErrorResponse
// @Failure      403  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Failure      500  {object}  ErrorResponse
// @Router       /agents/{id}/tasks/{taskID} [put]
func (h *AgentHandler) UpdateTask(c *gin.Context) {
	if !ensurePermission(c, h.authz, "component.agent.manage", "", "") {
		return
	}
	if h.taskManager == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "agent task manager is not configured"})
		return
	}
	var req createAgentTaskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	output, err := h.taskManager.Update(c.Request.Context(), c.Param("taskID"), usecase.UpdateAgentTaskInput{
		AgentID:    c.Param("id"),
		Name:       req.Name,
		TaskMode:   req.TaskMode,
		WorkDir:    req.WorkDir,
		ScriptID:   req.ScriptID,
		Variables:  req.Variables,
		TimeoutSec: req.TimeoutSec,
	})
	if err != nil {
		writeAgentHTTPError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": output})
}

// ExecuteTask 执行Task。
// @Summary      执行Task
// @Description  执行Task，并按统一响应结构返回处理结果。
// @Tags         agents
// @Accept       json
// @Produce      json
// @Param        id  path  string  true  "资源 ID"
// @Param        taskID  path  string  true  "任务 ID"
// @Success      200  {object}  GenericResponse
// @Failure      400  {object}  ErrorResponse
// @Failure      401  {object}  ErrorResponse
// @Failure      403  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Failure      500  {object}  ErrorResponse
// @Router       /agents/{id}/tasks/{taskID}/execute [post]
func (h *AgentHandler) ExecuteTask(c *gin.Context) {
	if !ensurePermission(c, h.authz, "component.agent.manage", "", "") {
		return
	}
	if h.taskManager == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "agent task manager is not configured"})
		return
	}
	output, err := h.taskManager.Execute(c.Request.Context(), c.Param("taskID"), usecase.ExecuteAgentTaskInput{
		AgentID: c.Param("id"),
	})
	if err != nil {
		writeAgentHTTPError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": output})
}

// ExecuteStandaloneTask 执行Standalone Task。
// @Summary      执行Standalone Task
// @Description  执行Standalone Task，并按统一响应结构返回处理结果。
// @Tags         agents
// @Accept       json
// @Produce      json
// @Param        taskID  path  string  true  "任务 ID"
// @Success      200  {object}  GenericResponse
// @Failure      400  {object}  ErrorResponse
// @Failure      401  {object}  ErrorResponse
// @Failure      403  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Failure      500  {object}  ErrorResponse
// @Router       /agent-tasks/{taskID}/execute [post]
func (h *AgentHandler) ExecuteStandaloneTask(c *gin.Context) {
	if !ensurePermission(c, h.authz, "component.agent.manage", "", "") {
		return
	}
	if h.taskManager == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "agent task manager is not configured"})
		return
	}
	output, err := h.taskManager.Execute(c.Request.Context(), c.Param("taskID"), usecase.ExecuteAgentTaskInput{})
	if err != nil {
		writeAgentHTTPError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": output})
}

// StopTask 停止Task。
// @Summary      停止Task
// @Description  停止Task，并按统一响应结构返回处理结果。
// @Tags         agents
// @Accept       json
// @Produce      json
// @Param        id  path  string  true  "资源 ID"
// @Param        taskID  path  string  true  "任务 ID"
// @Success      200  {object}  GenericResponse
// @Failure      400  {object}  ErrorResponse
// @Failure      401  {object}  ErrorResponse
// @Failure      403  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Failure      500  {object}  ErrorResponse
// @Router       /agents/{id}/tasks/{taskID}/stop [post]
func (h *AgentHandler) StopTask(c *gin.Context) {
	if !ensurePermission(c, h.authz, "component.agent.manage", "", "") {
		return
	}
	if h.taskManager == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "agent task manager is not configured"})
		return
	}
	output, err := h.taskManager.Stop(c.Request.Context(), c.Param("taskID"), usecase.StopAgentTaskInput{
		AgentID: c.Param("id"),
	})
	if err != nil {
		writeAgentHTTPError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": output})
}

// ResumeTask 恢复Task。
// @Summary      恢复Task
// @Description  恢复Task，并按统一响应结构返回处理结果。
// @Tags         agents
// @Accept       json
// @Produce      json
// @Param        id  path  string  true  "资源 ID"
// @Param        taskID  path  string  true  "任务 ID"
// @Success      200  {object}  GenericResponse
// @Failure      400  {object}  ErrorResponse
// @Failure      401  {object}  ErrorResponse
// @Failure      403  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Failure      500  {object}  ErrorResponse
// @Router       /agents/{id}/tasks/{taskID}/resume [post]
func (h *AgentHandler) ResumeTask(c *gin.Context) {
	if !ensurePermission(c, h.authz, "component.agent.manage", "", "") {
		return
	}
	if h.taskManager == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "agent task manager is not configured"})
		return
	}
	output, err := h.taskManager.Resume(c.Request.Context(), c.Param("taskID"), usecase.ResumeAgentTaskInput{
		AgentID: c.Param("id"),
	})
	if err != nil {
		writeAgentHTTPError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": output})
}

// DeleteTask 删除Task。
// @Summary      删除Task
// @Description  删除Task，并按统一响应结构返回处理结果。
// @Tags         agents
// @Produce      json
// @Param        id  path  string  true  "资源 ID"
// @Param        taskID  path  string  true  "任务 ID"
// @Success      200  {object}  GenericResponse
// @Failure      400  {object}  ErrorResponse
// @Failure      401  {object}  ErrorResponse
// @Failure      403  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Failure      500  {object}  ErrorResponse
// @Router       /agents/{id}/tasks/{taskID} [delete]
func (h *AgentHandler) DeleteTask(c *gin.Context) {
	if !ensurePermission(c, h.authz, "component.agent.manage", "", "") {
		return
	}
	if h.taskManager == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "agent task manager is not configured"})
		return
	}
	if err := h.taskManager.Delete(c.Request.Context(), c.Param("taskID"), usecase.StopAgentTaskInput{
		AgentID: c.Param("id"),
	}); err != nil {
		writeAgentHTTPError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// UpdateTemporaryTask 更新Temporary Task。
// @Summary      更新Temporary Task
// @Description  更新Temporary Task，并按统一响应结构返回处理结果。
// @Tags         agents
// @Accept       json
// @Produce      json
// @Param        taskID  path  string  true  "任务 ID"
// @Success      200  {object}  GenericResponse
// @Failure      400  {object}  ErrorResponse
// @Failure      401  {object}  ErrorResponse
// @Failure      403  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Failure      500  {object}  ErrorResponse
// @Router       /agent-tasks/{taskID} [put]
func (h *AgentHandler) UpdateTemporaryTask(c *gin.Context) {
	if !ensurePermission(c, h.authz, "component.agent.manage", "", "") {
		return
	}
	if h.taskManager == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "agent task manager is not configured"})
		return
	}
	var req createAgentTaskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	output, err := h.taskManager.UpdateTemporaryTask(c.Request.Context(), c.Param("taskID"), usecase.UpdateAgentTaskInput{
		TargetAgentIDs: req.TargetAgentIDs,
		Name:           req.Name,
		TaskMode:       req.TaskMode,
		WorkDir:        req.WorkDir,
		ScriptID:       req.ScriptID,
		Variables:      req.Variables,
		TimeoutSec:     req.TimeoutSec,
	})
	if err != nil {
		writeAgentHTTPError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": output})
}

// DeleteTemporaryTask 删除Temporary Task。
// @Summary      删除Temporary Task
// @Description  删除Temporary Task，并按统一响应结构返回处理结果。
// @Tags         agents
// @Produce      json
// @Param        taskID  path  string  true  "任务 ID"
// @Success      200  {object}  GenericResponse
// @Failure      400  {object}  ErrorResponse
// @Failure      401  {object}  ErrorResponse
// @Failure      403  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Failure      500  {object}  ErrorResponse
// @Router       /agent-tasks/{taskID} [delete]
func (h *AgentHandler) DeleteTemporaryTask(c *gin.Context) {
	if !ensurePermission(c, h.authz, "component.agent.manage", "", "") {
		return
	}
	if h.taskManager == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "agent task manager is not configured"})
		return
	}
	if err := h.taskManager.DeleteTemporaryTask(c.Request.Context(), c.Param("taskID")); err != nil {
		writeAgentHTTPError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// UpdateResidentTask 更新Resident Task。
// @Summary      更新Resident Task
// @Description  更新Resident Task，并按统一响应结构返回处理结果。
// @Tags         agents
// @Accept       json
// @Produce      json
// @Param        taskID  path  string  true  "任务 ID"
// @Success      200  {object}  GenericResponse
// @Failure      400  {object}  ErrorResponse
// @Failure      401  {object}  ErrorResponse
// @Failure      403  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Failure      500  {object}  ErrorResponse
// @Router       /resident-tasks/{taskID} [put]
func (h *AgentHandler) UpdateResidentTask(c *gin.Context) {
	if !ensurePermission(c, h.authz, "component.agent.manage", "", "") {
		return
	}
	if h.taskManager == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "agent task manager is not configured"})
		return
	}
	var req createAgentTaskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	output, err := h.taskManager.UpdateResidentTask(c.Request.Context(), c.Param("taskID"), usecase.UpdateAgentTaskInput{
		Name:       req.Name,
		TaskMode:   req.TaskMode,
		WorkDir:    req.WorkDir,
		ScriptID:   req.ScriptID,
		Variables:  req.Variables,
		TimeoutSec: req.TimeoutSec,
	})
	if err != nil {
		writeAgentHTTPError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": output})
}

// DeleteResidentTask 删除Resident Task。
// @Summary      删除Resident Task
// @Description  删除Resident Task，并按统一响应结构返回处理结果。
// @Tags         agents
// @Produce      json
// @Param        taskID  path  string  true  "任务 ID"
// @Success      200  {object}  GenericResponse
// @Failure      400  {object}  ErrorResponse
// @Failure      401  {object}  ErrorResponse
// @Failure      403  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Failure      500  {object}  ErrorResponse
// @Router       /resident-tasks/{taskID} [delete]
func (h *AgentHandler) DeleteResidentTask(c *gin.Context) {
	if !ensurePermission(c, h.authz, "component.agent.manage", "", "") {
		return
	}
	if h.taskManager == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "agent task manager is not configured"})
		return
	}
	if err := h.taskManager.DeleteResidentTask(c.Request.Context(), c.Param("taskID")); err != nil {
		writeAgentHTTPError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// CreateScript 创建Script。
// @Summary      创建Script
// @Description  创建Script，并按统一响应结构返回处理结果。
// @Tags         agents
// @Accept       json
// @Produce      json
// @Success      200  {object}  GenericResponse
// @Failure      400  {object}  ErrorResponse
// @Failure      401  {object}  ErrorResponse
// @Failure      403  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Failure      500  {object}  ErrorResponse
// @Router       /agent-scripts [post]
func (h *AgentHandler) CreateScript(c *gin.Context) {
	if !ensurePermission(c, h.authz, "component.agent.manage", "", "") {
		return
	}
	if h.scriptManager == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "agent script manager is not configured"})
		return
	}
	var req upsertAgentScriptRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	createdBy := ""
	if currentUser, ok := getCurrentUser(c); ok {
		createdBy = strings.TrimSpace(currentUser.DisplayName)
		if createdBy == "" {
			createdBy = strings.TrimSpace(currentUser.Username)
		}
		if createdBy == "" {
			createdBy = strings.TrimSpace(currentUser.ID)
		}
	}
	output, err := h.scriptManager.Create(c.Request.Context(), usecase.CreateAgentScriptInput{
		Name:        req.Name,
		Description: req.Description,
		TaskType:    req.TaskType,
		ShellType:   req.ShellType,
		ScriptPath:  req.ScriptPath,
		ScriptText:  req.ScriptText,
		CreatedBy:   createdBy,
	})
	if err != nil {
		writeAgentHTTPError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": output})
}

// UpdateScript 更新Script。
// @Summary      更新Script
// @Description  更新Script，并按统一响应结构返回处理结果。
// @Tags         agents
// @Accept       json
// @Produce      json
// @Param        id  path  string  true  "资源 ID"
// @Success      200  {object}  GenericResponse
// @Failure      400  {object}  ErrorResponse
// @Failure      401  {object}  ErrorResponse
// @Failure      403  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Failure      500  {object}  ErrorResponse
// @Router       /agent-scripts/{id} [put]
func (h *AgentHandler) UpdateScript(c *gin.Context) {
	if !ensurePermission(c, h.authz, "component.agent.manage", "", "") {
		return
	}
	if h.scriptManager == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "agent script manager is not configured"})
		return
	}
	var req upsertAgentScriptRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	updatedBy := ""
	if currentUser, ok := getCurrentUser(c); ok {
		updatedBy = strings.TrimSpace(currentUser.DisplayName)
		if updatedBy == "" {
			updatedBy = strings.TrimSpace(currentUser.Username)
		}
		if updatedBy == "" {
			updatedBy = strings.TrimSpace(currentUser.ID)
		}
	}
	output, err := h.scriptManager.Update(c.Request.Context(), c.Param("id"), usecase.UpdateAgentScriptInput{
		Name:        req.Name,
		Description: req.Description,
		TaskType:    req.TaskType,
		ShellType:   req.ShellType,
		ScriptPath:  req.ScriptPath,
		ScriptText:  req.ScriptText,
		UpdatedBy:   updatedBy,
	})
	if err != nil {
		writeAgentHTTPError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": output})
}

// DeleteScript 删除Script。
// @Summary      删除Script
// @Description  删除Script，并按统一响应结构返回处理结果。
// @Tags         agents
// @Produce      json
// @Param        id  path  string  true  "资源 ID"
// @Success      200  {object}  GenericResponse
// @Failure      400  {object}  ErrorResponse
// @Failure      401  {object}  ErrorResponse
// @Failure      403  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Failure      500  {object}  ErrorResponse
// @Router       /agent-scripts/{id} [delete]
func (h *AgentHandler) DeleteScript(c *gin.Context) {
	if !ensurePermission(c, h.authz, "component.agent.manage", "", "") {
		return
	}
	if h.scriptManager == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "agent script manager is not configured"})
		return
	}
	if err := h.scriptManager.Delete(c.Request.Context(), c.Param("id")); err != nil {
		writeAgentHTTPError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// PollTask 处理Poll Task接口。
// @Summary      处理Poll Task接口
// @Description  处理Poll Task接口，并按统一响应结构返回处理结果。
// @Tags         agents
// @Accept       json
// @Produce      json
// @Success      200  {object}  GenericResponse
// @Failure      400  {object}  ErrorResponse
// @Failure      401  {object}  ErrorResponse
// @Failure      403  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Failure      500  {object}  ErrorResponse
// @Router       /agent/tasks/poll [post]
func (h *AgentHandler) PollTask(c *gin.Context) {
	if h.taskManager == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "agent task manager is not configured"})
		return
	}
	var req agentTaskPollRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	output, err := h.taskManager.Poll(c.Request.Context(), usecase.AgentTaskPollInput{
		AgentCode: req.AgentCode,
		Token:     req.Token,
	})
	if err != nil {
		writeAgentHTTPError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": output})
}

// StartTask 启动Task。
// @Summary      启动Task
// @Description  启动Task，并按统一响应结构返回处理结果。
// @Tags         agents
// @Accept       json
// @Produce      json
// @Param        id  path  string  true  "资源 ID"
// @Success      200  {object}  GenericResponse
// @Failure      400  {object}  ErrorResponse
// @Failure      401  {object}  ErrorResponse
// @Failure      403  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Failure      500  {object}  ErrorResponse
// @Router       /agent/tasks/{id}/start [post]
func (h *AgentHandler) StartTask(c *gin.Context) {
	if h.taskManager == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "agent task manager is not configured"})
		return
	}
	var req agentTaskPollRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	output, err := h.taskManager.Start(c.Request.Context(), req.AgentCode, req.Token, c.Param("id"))
	if err != nil {
		writeAgentHTTPError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": output})
}

// FinishTask 完成Task。
// @Summary      完成Task
// @Description  完成Task，并按统一响应结构返回处理结果。
// @Tags         agents
// @Accept       json
// @Produce      json
// @Param        id  path  string  true  "资源 ID"
// @Success      200  {object}  GenericResponse
// @Failure      400  {object}  ErrorResponse
// @Failure      401  {object}  ErrorResponse
// @Failure      403  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Failure      500  {object}  ErrorResponse
// @Router       /agent/tasks/{id}/finish [post]
func (h *AgentHandler) FinishTask(c *gin.Context) {
	if h.taskManager == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "agent task manager is not configured"})
		return
	}
	var req finishAgentTaskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	output, err := h.taskManager.Finish(c.Request.Context(), usecase.FinishAgentTaskInput{
		AgentCode:     req.AgentCode,
		Token:         req.Token,
		TaskID:        c.Param("id"),
		Status:        agentdomain.TaskStatus(strings.TrimSpace(req.Status)),
		ExitCode:      req.ExitCode,
		StdoutText:    req.StdoutText,
		StderrText:    req.StderrText,
		FailureReason: req.FailureReason,
	})
	if err != nil {
		writeAgentHTTPError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": output})
}

// Enable 处理Enable接口。
// @Summary      处理Enable接口
// @Description  处理Enable接口，并按统一响应结构返回处理结果。
// @Tags         agents
// @Accept       json
// @Produce      json
// @Param        id  path  string  true  "资源 ID"
// @Success      200  {object}  GenericResponse
// @Failure      400  {object}  ErrorResponse
// @Failure      401  {object}  ErrorResponse
// @Failure      403  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Failure      500  {object}  ErrorResponse
// @Router       /agents/{id}/enable [post]
func (h *AgentHandler) Enable(c *gin.Context) {
	h.updateStatus(c, agentdomain.StatusActive)
}

// Disable 处理Disable接口。
// @Summary      处理Disable接口
// @Description  处理Disable接口，并按统一响应结构返回处理结果。
// @Tags         agents
// @Accept       json
// @Produce      json
// @Param        id  path  string  true  "资源 ID"
// @Success      200  {object}  GenericResponse
// @Failure      400  {object}  ErrorResponse
// @Failure      401  {object}  ErrorResponse
// @Failure      403  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Failure      500  {object}  ErrorResponse
// @Router       /agents/{id}/disable [post]
func (h *AgentHandler) Disable(c *gin.Context) {
	h.updateStatus(c, agentdomain.StatusDisabled)
}

// Maintenance 处理Maintenance接口。
// @Summary      处理Maintenance接口
// @Description  处理Maintenance接口，并按统一响应结构返回处理结果。
// @Tags         agents
// @Accept       json
// @Produce      json
// @Param        id  path  string  true  "资源 ID"
// @Success      200  {object}  GenericResponse
// @Failure      400  {object}  ErrorResponse
// @Failure      401  {object}  ErrorResponse
// @Failure      403  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Failure      500  {object}  ErrorResponse
// @Router       /agents/{id}/maintenance [post]
func (h *AgentHandler) Maintenance(c *gin.Context) {
	h.updateStatus(c, agentdomain.StatusMaintenance)
}

// updateStatus 更新业务资源并返回处理结果。
func (h *AgentHandler) updateStatus(c *gin.Context, status agentdomain.Status) {
	if !ensurePermission(c, h.authz, "component.agent.manage", "", "") {
		return
	}
	if h.manager == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "agent manager is not configured"})
		return
	}
	output, err := h.manager.UpdateStatus(c.Request.Context(), c.Param("id"), status)
	if err != nil {
		writeAgentHTTPError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": output})
}

// Heartbeat 处理Heartbeat接口。
// @Summary      处理Heartbeat接口
// @Description  处理Heartbeat接口，并按统一响应结构返回处理结果。
// @Tags         agents
// @Accept       json
// @Produce      json
// @Success      200  {object}  GenericResponse
// @Failure      400  {object}  ErrorResponse
// @Failure      401  {object}  ErrorResponse
// @Failure      403  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Failure      500  {object}  ErrorResponse
// @Router       /agent/heartbeat [post]
func (h *AgentHandler) Heartbeat(c *gin.Context) {
	if h.manager == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "agent manager is not configured"})
		return
	}
	var req agentHeartbeatRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	output, err := h.manager.Heartbeat(c.Request.Context(), usecase.AgentHeartbeatInput{
		AgentCode:          req.AgentCode,
		Token:              req.Token,
		Hostname:           req.Hostname,
		HostIP:             req.HostIP,
		AgentVersion:       req.AgentVersion,
		OS:                 req.OS,
		Arch:               req.Arch,
		WorkDir:            req.WorkDir,
		Tags:               req.Tags,
		CurrentTaskID:      req.CurrentTaskID,
		CurrentTaskName:    req.CurrentTaskName,
		CurrentTaskType:    req.CurrentTaskType,
		CurrentTaskStarted: parseOptionalTime(req.CurrentTaskStarted),
		LastTaskStatus:     agentdomain.LastTaskStatus(strings.TrimSpace(req.LastTaskStatus)),
		LastTaskSummary:    req.LastTaskSummary,
		LastTaskFinishedAt: parseOptionalTime(req.LastTaskFinishedAt),
	})
	if err != nil {
		writeAgentHTTPError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": output})
}

// Register 处理Register接口。
// @Summary      处理Register接口
// @Description  处理Register接口，并按统一响应结构返回处理结果。
// @Tags         agents
// @Accept       json
// @Produce      json
// @Success      200  {object}  GenericResponse
// @Failure      400  {object}  ErrorResponse
// @Failure      401  {object}  ErrorResponse
// @Failure      403  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Failure      500  {object}  ErrorResponse
// @Router       /agent/register [post]
func (h *AgentHandler) Register(c *gin.Context) {
	if h.manager == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "agent manager is not configured"})
		return
	}
	var req agentRegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	output, err := h.manager.Register(c.Request.Context(), usecase.AgentRegisterInput{
		RegistrationToken: req.RegistrationToken,
		MachineID:         req.MachineID,
		Name:              req.Name,
		EnvironmentCode:   req.EnvironmentCode,
		Hostname:          req.Hostname,
		HostIP:            req.HostIP,
		AgentVersion:      req.AgentVersion,
		OS:                req.OS,
		Arch:              req.Arch,
		WorkDir:           req.WorkDir,
		Tags:              req.Tags,
	})
	if err != nil {
		writeAgentHTTPError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": output})
}

// parseOptionalTime 解析输入内容并返回结构化结果。
func parseOptionalTime(raw string) *time.Time {
	text := strings.TrimSpace(raw)
	if text == "" {
		return nil
	}
	value, err := time.Parse(time.RFC3339, text)
	if err != nil {
		return nil
	}
	value = value.UTC()
	return &value
}

// writeAgentHTTPError 写入处理结果或错误信息。
func writeAgentHTTPError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, usecase.ErrInvalidInput), errors.Is(err, usecase.ErrInvalidID), errors.Is(err, usecase.ErrInvalidStatus):
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	case errors.Is(err, agentdomain.ErrInstanceNotFound), errors.Is(err, agentdomain.ErrTaskNotFound), errors.Is(err, agentdomain.ErrScriptNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
	case errors.Is(err, agentdomain.ErrInstanceDeleteBlocked):
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
	case errors.Is(err, agentdomain.ErrTaskNotClaimable):
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
	case errors.Is(err, agentdomain.ErrAgentCodeDuplicated):
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
	case errors.Is(err, agentdomain.ErrHeartbeatAuthRejected), errors.Is(err, agentdomain.ErrInvalidAgentToken), errors.Is(err, agentdomain.ErrBootstrapTokenInvalid):
		c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
	}
}

// resolveAgentBaseURL 解析上下文数据，得到后续流程需要的结果。
func resolveAgentBaseURL(c *gin.Context) string {
	if c == nil || c.Request == nil {
		return "http://127.0.0.1:8081"
	}
	scheme := strings.TrimSpace(c.GetHeader("X-Forwarded-Proto"))
	if scheme == "" {
		if c.Request.TLS != nil {
			scheme = "https"
		} else {
			scheme = "http"
		}
	}
	host := strings.TrimSpace(c.GetHeader("X-Forwarded-Host"))
	if host == "" {
		host = strings.TrimSpace(c.Request.Host)
	}
	if host == "" {
		host = "127.0.0.1:8081"
	}
	return fmt.Sprintf("%s://%s", scheme, host)
}
