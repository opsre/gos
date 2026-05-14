package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"gos/internal/application/usecase"
	aidomain "gos/internal/domain/ai"
)

type AIModelConfigHandler struct {
	manager       *usecase.AIModelConfigManager
	clientFactory usecase.AIModelClientFactory
	authz         RequestAuthorizer
}

func NewAIModelConfigHandler(
	manager *usecase.AIModelConfigManager,
	clientFactory usecase.AIModelClientFactory,
	authz RequestAuthorizer,
) *AIModelConfigHandler {
	return &AIModelConfigHandler{
		manager:       manager,
		clientFactory: clientFactory,
		authz:         authz,
	}
}

func (h *AIModelConfigHandler) RegisterRoutes(router gin.IRouter) {
	if h == nil {
		return
	}
	router.GET("/system/ai-model-configs", h.List)
	router.POST("/system/ai-model-configs", h.Create)
	router.GET("/system/ai-model-configs/:id", h.Get)
	router.PUT("/system/ai-model-configs/:id", h.Update)
	router.DELETE("/system/ai-model-configs/:id", h.Delete)
	router.POST("/system/ai-model-configs/:id/test", h.Test)
	router.POST("/system/ai-model-configs/:id/set-diagnosis-model", h.SetDiagnosisModel)
	router.POST("/system/ai-model-configs/:id/unset-diagnosis-model", h.UnsetDiagnosisModel)
}

type AIModelConfigRequest struct {
	Name        string   `json:"name"`
	Provider    string   `json:"provider"`
	BaseURL     string   `json:"base_url"`
	Model       string   `json:"model"`
	APIKey      *string  `json:"api_key"`
	Temperature *float64 `json:"temperature"`
	MaxTokens   int      `json:"max_tokens"`
	TimeoutSec  int      `json:"timeout_sec"`
	Enabled     bool     `json:"enabled"`
}

func (h *AIModelConfigHandler) List(c *gin.Context) {
	if !ensurePermission(c, h.authz, "system.permission.manage", "", "") {
		return
	}
	items, err := h.manager.List(c.Request.Context())
	if err != nil {
		writeAIModelConfigHTTPError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": items})
}

func (h *AIModelConfigHandler) Get(c *gin.Context) {
	if !ensurePermission(c, h.authz, "system.permission.manage", "", "") {
		return
	}
	item, err := h.manager.Get(c.Request.Context(), c.Param("id"))
	if err != nil {
		writeAIModelConfigHTTPError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": item})
}

func (h *AIModelConfigHandler) Create(c *gin.Context) {
	if !ensurePermission(c, h.authz, "system.permission.manage", "", "") {
		return
	}
	var req AIModelConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	user, _ := getCurrentUser(c)
	item, err := h.manager.Create(c.Request.Context(), toAIModelConfigInput(req, user.ID))
	if err != nil {
		writeAIModelConfigHTTPError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"data": item})
}

func (h *AIModelConfigHandler) Update(c *gin.Context) {
	if !ensurePermission(c, h.authz, "system.permission.manage", "", "") {
		return
	}
	var req AIModelConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	item, err := h.manager.Update(c.Request.Context(), c.Param("id"), toAIModelConfigInput(req, ""))
	if err != nil {
		writeAIModelConfigHTTPError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": item})
}

func (h *AIModelConfigHandler) Delete(c *gin.Context) {
	if !ensurePermission(c, h.authz, "system.permission.manage", "", "") {
		return
	}
	if err := h.manager.Delete(c.Request.Context(), c.Param("id")); err != nil {
		writeAIModelConfigHTTPError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": gin.H{"deleted": true}})
}

func (h *AIModelConfigHandler) SetDiagnosisModel(c *gin.Context) {
	if !ensurePermission(c, h.authz, "system.permission.manage", "", "") {
		return
	}
	item, err := h.manager.SetDiagnosisModel(c.Request.Context(), c.Param("id"))
	if err != nil {
		writeAIModelConfigHTTPError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": item})
}

func (h *AIModelConfigHandler) UnsetDiagnosisModel(c *gin.Context) {
	if !ensurePermission(c, h.authz, "system.permission.manage", "", "") {
		return
	}
	item, err := h.manager.UnsetDiagnosisModel(c.Request.Context(), c.Param("id"))
	if err != nil {
		writeAIModelConfigHTTPError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": item})
}

func (h *AIModelConfigHandler) Test(c *gin.Context) {
	if !ensurePermission(c, h.authz, "system.permission.manage", "", "") {
		return
	}
	if h.clientFactory == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "ai client factory is not configured"})
		return
	}
	config, err := h.manager.GetDomainConfig(c.Request.Context(), c.Param("id"))
	if err != nil {
		writeAIModelConfigHTTPError(c, err)
		return
	}
	if !config.Enabled {
		c.JSON(http.StatusBadRequest, gin.H{"error": "disabled ai model cannot be tested"})
		return
	}
	if !config.HasAPIKey() {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ai model api key is required"})
		return
	}
	client, err := h.clientFactory.NewClient(config)
	if err != nil {
		writeAIModelConfigHTTPError(c, err)
		return
	}
	_, err = client.DiagnoseStageLog(c.Request.Context(), sampleAIModelTestInput())
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": gin.H{"ok": true}})
}

func toAIModelConfigInput(req AIModelConfigRequest, createdBy string) usecase.AIModelConfigInput {
	temperature := 0.2
	if req.Temperature != nil {
		temperature = *req.Temperature
	}
	return usecase.AIModelConfigInput{
		Name:        req.Name,
		Provider:    firstNonEmptyString(req.Provider, string(aidomain.ProviderOpenAICompatible)),
		BaseURL:     req.BaseURL,
		Model:       req.Model,
		APIKey:      req.APIKey,
		Temperature: temperature,
		MaxTokens:   req.MaxTokens,
		TimeoutSec:  req.TimeoutSec,
		Enabled:     req.Enabled,
		CreatedBy:   createdBy,
	}
}

func sampleAIModelTestInput() usecase.AIChatInput {
	return usecase.AIChatInput{
		ReleaseOrder: usecase.AIChatReleaseOrder{ID: "test", OrderNo: "TEST", ApplicationName: "test", EnvCode: "test", OperationType: "deploy", TriggerType: "manual"},
		Pipeline:     usecase.AIChatPipeline{Scope: "ci", Provider: "jenkins", StageID: "stage-test", StageName: "Test", StageStatus: "failed", RawStatus: "FAILED"},
		Log:          usecase.AIChatLog{Hash: "test", TotalChars: 16, Strategy: "test", Content: "ERROR test log"},
		Rules:        usecase.AIChatRules{Language: "zh-CN", OutputSchema: "release_stage_diagnosis_v1"},
	}
}

func writeAIModelConfigHTTPError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, usecase.ErrInvalidInput), errors.Is(err, usecase.ErrInvalidID):
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	case errors.Is(err, aidomain.ErrModelConfigNotFound), errors.Is(err, aidomain.ErrDiagnosisModelNotConfigured):
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
	case errors.Is(err, aidomain.ErrDiagnosisModelInUse):
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
	default:
		if strings.TrimSpace(err.Error()) == "" {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
			return
		}
		var js json.RawMessage
		if json.Unmarshal([]byte(err.Error()), &js) == nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
	}
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
