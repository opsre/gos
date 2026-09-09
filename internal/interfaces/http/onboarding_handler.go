package httpapi

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"gos/internal/application/usecase"
	app "gos/internal/domain/application"
	ob "gos/internal/domain/onboarding"
	pp "gos/internal/domain/platformparam"
	project "gos/internal/domain/project"
	usr "gos/internal/domain/user"
)

type OnboardingHandler struct {
	manager *usecase.OnboardingManager
	authz   RequestAuthorizer
}

type OnboardingCreateRequest struct {
	Mode          string `json:"mode" enums:"create_application,complete_application"`
	ApplicationID string `json:"application_id"`
	ProjectID     string `json:"project_id"`
}
type OnboardingVersionRequest struct {
	ExpectedVersion int64 `json:"expected_version"`
}
type OnboardingDraftRequest struct {
	ExpectedVersion int64    `json:"expected_version"`
	Draft           ob.Draft `json:"draft"`
}
type OnboardingApplyRequest struct {
	ExpectedVersion int64  `json:"expected_version"`
	RequestKey      string `json:"request_key"`
}
type OnboardingFirstReleaseRequest struct {
	ExpectedVersion int64                     `json:"expected_version"`
	RequestKey      string                    `json:"request_key"`
	Order           CreateReleaseOrderRequest `json:"order"`
}
type OnboardingSessionResponse struct {
	Data ob.Session `json:"data"`
}
type OnboardingInspectionResponse struct {
	Data usecase.OnboardingInspection `json:"data"`
}

func NewOnboardingHandler(manager *usecase.OnboardingManager, authz RequestAuthorizer) *OnboardingHandler {
	return &OnboardingHandler{manager, authz}
}
func (h *OnboardingHandler) RegisterRoutes(r gin.IRouter) {
	r.GET("/onboarding/status", h.Status)
	r.POST("/onboarding/sessions", h.Create)
	r.GET("/onboarding/sessions/:id", h.Get)
	r.PUT("/onboarding/sessions/:id/draft", h.SaveDraft)
	r.POST("/onboarding/sessions/:id/inspect", h.Inspect)
	r.POST("/onboarding/sessions/:id/steps/:step/apply", h.Apply)
	r.POST("/onboarding/sessions/:id/check", h.Check)
	r.POST("/onboarding/sessions/:id/first-release", h.FirstRelease)
	r.POST("/onboarding/sessions/:id/abandon", h.Abandon)
	r.GET("/applications/:id/setup-status", h.SetupStatus)
}
func onboardingJSON(c *gin.Context, target any) bool {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 1024*1024)
	if err := c.ShouldBindJSON(target); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求内容无效或过大"})
		return false
	}
	return true
}
func (h *OnboardingHandler) permissions(c *gin.Context, codes ...string) bool {
	for _, code := range codes {
		if !ensurePermission(c, h.authz, code, "", "") {
			return false
		}
	}
	return true
}
func (h *OnboardingHandler) session(c *gin.Context) (ob.Session, bool) {
	if !h.permissions(c, "application.manage") {
		return ob.Session{}, false
	}
	s, err := h.manager.Get(c.Request.Context(), c.Param("id"))
	if err != nil {
		onboardingError(c, s, err)
		return s, false
	}
	user, _ := getCurrentUser(c)
	if s.OwnerUserID != user.ID && user.Role != usr.RoleAdmin {
		onboardingError(c, ob.Session{}, ob.ErrForbidden)
		return ob.Session{}, false
	}
	return s, true
}
func onboardingError(c *gin.Context, s ob.Session, err error) {
	status := http.StatusUnprocessableEntity
	switch {
	case errors.Is(err, ob.ErrNotFound):
		status = http.StatusNotFound
	case errors.Is(err, ob.ErrForbidden):
		status = http.StatusForbidden
	case errors.Is(err, ob.ErrConflict), errors.Is(err, app.ErrKeyDuplicated), errors.Is(err, project.ErrKeyDuplicated), errors.Is(err, pp.ErrParamKeyDuplicated):
		status = http.StatusConflict
	case errors.Is(err, usecase.ErrInvalidInput), errors.Is(err, usecase.ErrInvalidID):
		status = http.StatusBadRequest
	}
	body := gin.H{"error": err.Error()}
	if s.ID != "" {
		body["data"] = s
	}
	c.JSON(status, body)
}

// Status godoc
// @Summary 应用接入基础状态与我的任务
// @Tags onboarding
// @Produce json
// @Success 200 {object} GenericResponse
// @Failure 403 {object} ErrorResponse
// @Router /onboarding/status [get]
func (h *OnboardingHandler) Status(c *gin.Context) {
	if !h.permissions(c, "application.manage", "pipeline.view") {
		return
	}
	user, _ := getCurrentUser(c)
	data, err := h.manager.Status(c.Request.Context(), user.ID)
	if err != nil {
		onboardingError(c, ob.Session{}, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": data})
}

// Create godoc
// @Summary 新增独立应用接入任务或补齐已有应用
// @Tags onboarding
// @Accept json
// @Produce json
// @Param request body OnboardingCreateRequest true "接入模式及可选项目"
// @Success 201 {object} OnboardingSessionResponse
// @Failure 403 {object} ErrorResponse
// @Router /onboarding/sessions [post]
func (h *OnboardingHandler) Create(c *gin.Context) {
	if !h.permissions(c, "application.manage") {
		return
	}
	var req OnboardingCreateRequest
	if !onboardingJSON(c, &req) {
		return
	}
	if req.ApplicationID != "" && !ensureApplicationVisible(c, h.authz, req.ApplicationID) {
		return
	}
	if req.Mode == "complete_application" && !h.permissions(c, "release.template.manage", "pipeline.view", "platform_param.manage", "pipeline_param.manage") {
		return
	}
	user, _ := getCurrentUser(c)
	s, err := h.manager.CreateSession(c.Request.Context(), user.ID, req.Mode, req.ApplicationID, req.ProjectID)
	if err != nil {
		onboardingError(c, ob.Session{}, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"data": s})
}

// Get godoc
// @Summary 读取或恢复接入任务
// @Tags onboarding
// @Produce json
// @Param id path string true "接入任务 ID"
// @Success 200 {object} OnboardingSessionResponse
// @Failure 403 {object} ErrorResponse
// @Router /onboarding/sessions/{id} [get]
func (h *OnboardingHandler) Get(c *gin.Context) {
	s, ok := h.session(c)
	if ok {
		c.JSON(http.StatusOK, gin.H{"data": s})
	}
}

// SaveDraft godoc
// @Summary 按版本保存接入草稿，不创建业务资源
// @Tags onboarding
// @Accept json
// @Produce json
// @Param id path string true "接入任务 ID"
// @Param request body OnboardingDraftRequest true "版本及非敏感草稿"
// @Success 200 {object} OnboardingSessionResponse
// @Failure 409 {object} ErrorResponse
// @Router /onboarding/sessions/{id}/draft [put]
func (h *OnboardingHandler) SaveDraft(c *gin.Context) {
	s, ok := h.session(c)
	if !ok {
		return
	}
	var req OnboardingDraftRequest
	if !onboardingJSON(c, &req) {
		return
	}
	result, err := h.manager.SaveDraft(c.Request.Context(), s.ID, req.ExpectedVersion, req.Draft)
	if err != nil {
		onboardingError(c, ob.Session{}, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": result})
}

// Inspect godoc
// @Summary 只读检查真实管线参数及标准字段建议
// @Tags onboarding
// @Produce json
// @Param id path string true "接入任务 ID"
// @Success 200 {object} OnboardingInspectionResponse
// @Failure 403 {object} ErrorResponse
// @Router /onboarding/sessions/{id}/inspect [post]
func (h *OnboardingHandler) Inspect(c *gin.Context) {
	s, ok := h.session(c)
	if !ok || !h.permissions(c, "pipeline.view", "platform_param.manage", "pipeline_param.manage") {
		return
	}
	result, err := h.manager.Inspect(c.Request.Context(), s.Draft)
	if err != nil {
		onboardingError(c, ob.Session{}, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": result})
}

// Apply godoc
// @Summary 幂等保存一个接入步骤，不执行管线
// @Tags onboarding
// @Accept json
// @Produce json
// @Param id path string true "接入任务 ID"
// @Param step path string true "步骤" Enums(preflight,identity,pipelines,parameters,template_flow,review)
// @Param request body OnboardingApplyRequest true "预期版本与幂等键"
// @Success 200 {object} OnboardingSessionResponse
// @Failure 403 {object} ErrorResponse
// @Failure 409 {object} ErrorResponse
// @Failure 422 {object} ErrorResponse
// @Router /onboarding/sessions/{id}/steps/{step}/apply [post]
func (h *OnboardingHandler) Apply(c *gin.Context) {
	s, ok := h.session(c)
	if !ok {
		return
	}
	step := c.Param("step")
	switch step {
	case "preflight":
		ok = h.permissions(c, "pipeline.view")
	case "pipelines":
		ok = h.permissions(c, "pipeline.manage", "pipeline.view", "pipeline_param.manage", "platform_param.manage")
	case "parameters":
		ok = h.permissions(c, "pipeline_param.manage", "platform_param.manage", "pipeline.view")
	case "template_flow", "review":
		ok = h.permissions(c, "release.template.manage", "pipeline.view", "platform_param.manage", "pipeline_param.manage")
	}
	if !ok {
		return
	}
	var req OnboardingApplyRequest
	if !onboardingJSON(c, &req) {
		return
	}
	result, err := h.manager.ApplyStep(c.Request.Context(), s.ID, req.RequestKey, step, req.ExpectedVersion)
	if err != nil {
		onboardingError(c, result, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": result})
}

// Check godoc
// @Summary 检查已有业务配置并刷新首单验证状态
// @Tags onboarding
// @Produce json
// @Param id path string true "接入任务 ID"
// @Success 200 {object} OnboardingSessionResponse
// @Failure 403 {object} ErrorResponse
// @Router /onboarding/sessions/{id}/check [post]
func (h *OnboardingHandler) Check(c *gin.Context) {
	s, ok := h.session(c)
	if !ok || !h.permissions(c, "release.template.manage", "pipeline.view", "platform_param.manage", "pipeline_param.manage") {
		return
	}
	result, err := h.manager.Check(c.Request.Context(), s.ID)
	if err != nil {
		onboardingError(c, ob.Session{}, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": result})
}

// Abandon godoc
// @Summary 放弃任务并保留已创建的所有业务资源
// @Tags onboarding
// @Accept json
// @Produce json
// @Param id path string true "接入任务 ID"
// @Param request body OnboardingVersionRequest true "预期版本"
// @Success 200 {object} OnboardingSessionResponse
// @Failure 409 {object} ErrorResponse
// @Router /onboarding/sessions/{id}/abandon [post]
func (h *OnboardingHandler) Abandon(c *gin.Context) {
	s, ok := h.session(c)
	if !ok {
		return
	}
	var req OnboardingVersionRequest
	if !onboardingJSON(c, &req) {
		return
	}
	result, err := h.manager.Abandon(c.Request.Context(), s.ID, req.ExpectedVersion)
	if err != nil {
		onboardingError(c, ob.Session{}, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": result})
}

// SetupStatus godoc
// @Summary 按真实模板检查应用接入状态，不依赖引导任务存在
// @Tags onboarding
// @Produce json
// @Param id path string true "应用 ID"
// @Success 200 {object} GenericResponse
// @Failure 403 {object} ErrorResponse
// @Router /applications/{id}/setup-status [get]
func (h *OnboardingHandler) SetupStatus(c *gin.Context) {
	if !h.permissions(c, "application.manage", "release.template.manage", "pipeline.view", "platform_param.manage", "pipeline_param.manage") {
		return
	}
	if !ensureApplicationVisible(c, h.authz, c.Param("id")) {
		return
	}
	result, err := h.manager.SetupStatus(c.Request.Context(), c.Param("id"))
	if err != nil {
		onboardingError(c, ob.Session{}, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": result})
}

// FirstRelease godoc
// @Summary 幂等创建关联首单，不自动执行或绕过发布权限
// @Tags onboarding
// @Accept json
// @Produce json
// @Param id path string true "接入任务 ID"
// @Param request body OnboardingFirstReleaseRequest true "版本、幂等键与标准发布请求"
// @Success 201 {object} OnboardingSessionResponse
// @Failure 403 {object} ErrorResponse
// @Failure 409 {object} ErrorResponse
// @Failure 422 {object} ErrorResponse
// @Router /onboarding/sessions/{id}/first-release [post]
func (h *OnboardingHandler) FirstRelease(c *gin.Context) {
	s, ok := h.session(c)
	if !ok {
		return
	}
	var req OnboardingFirstReleaseRequest
	if !onboardingJSON(c, &req) {
		return
	}
	if !ensureReleaseApplicationPermission(c, h.authz, "release.create", req.Order.ApplicationID, req.Order.EnvCode) {
		return
	}
	user, _ := getCurrentUser(c)
	result, err := h.manager.CreateFirstRelease(c.Request.Context(), s.ID, req.RequestKey, req.ExpectedVersion, buildReleaseOrderInput(req.Order, user))
	if err != nil {
		onboardingError(c, result, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"data": result})
}
