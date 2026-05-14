package httpapi

import (
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"gos/internal/application/usecase"
	scandomain "gos/internal/domain/pipelinescan"
)

type PipelineScanHandler struct {
	manager *usecase.PipelineScanManager
	authz   RequestAuthorizer
}

func NewPipelineScanHandler(manager *usecase.PipelineScanManager, authz RequestAuthorizer) *PipelineScanHandler {
	return &PipelineScanHandler{
		manager: manager,
		authz:   authz,
	}
}

func (h *PipelineScanHandler) RegisterRoutes(router gin.IRouter) {
	if h == nil {
		return
	}
	router.GET("/pipeline-scan/rules", h.ListRules)
	router.POST("/pipeline-scan/rules", h.CreateRule)
	router.GET("/pipeline-scan/rules/:id", h.GetRule)
	router.PUT("/pipeline-scan/rules/:id", h.UpdateRule)
	router.PATCH("/pipeline-scan/rules/:id/enabled", h.SetRuleEnabled)
	router.DELETE("/pipeline-scan/rules/:id", h.DeleteRule)
	router.POST("/pipeline-scan/scan", h.ScanAllPipelines)
	router.GET("/pipeline-scan/results", h.ListResults)
	router.GET("/pipelines/:id/scan-result", h.GetPipelineResult)
	router.POST("/pipelines/:id/scan", h.ScanPipeline)
}

type PipelineScanRuleRequest struct {
	RuleType                 string   `json:"rule_type"`
	RuleCode                 string   `json:"rule_code"`
	RuleName                 string   `json:"rule_name"`
	Category                 string   `json:"category"`
	Severity                 string   `json:"severity"`
	Enabled                  bool     `json:"enabled"`
	TemplateValidationScopes []string `json:"template_validation_scopes"`
	ScopeJSON                string   `json:"scope_json"`
	RuleDSL                  string   `json:"rule_dsl_json"`
	Message                  string   `json:"message"`
	Suggestion               string   `json:"suggestion"`
}

type PipelineScanRuleEnabledRequest struct {
	Enabled bool `json:"enabled"`
}

type PipelineScanRuleResponse struct {
	ID                       string    `json:"id"`
	RuleType                 string    `json:"rule_type"`
	RuleCode                 string    `json:"rule_code"`
	RuleName                 string    `json:"rule_name"`
	Category                 string    `json:"category"`
	Severity                 string    `json:"severity"`
	Enabled                  bool      `json:"enabled"`
	Builtin                  bool      `json:"builtin"`
	TemplateValidationScopes []string  `json:"template_validation_scopes"`
	ScopeJSON                string    `json:"scope_json"`
	RuleDSL                  string    `json:"rule_dsl_json"`
	Message                  string    `json:"message"`
	Suggestion               string    `json:"suggestion"`
	CreatedAt                time.Time `json:"created_at"`
	UpdatedAt                time.Time `json:"updated_at"`
}

type PipelineScanRuleDataResponse struct {
	Data PipelineScanRuleResponse `json:"data"`
}

type PipelineScanRuleListResponse struct {
	Data     []PipelineScanRuleResponse `json:"data"`
	Page     int                        `json:"page"`
	PageSize int                        `json:"page_size"`
	Total    int64                      `json:"total"`
}

type PipelineScanResultResponse struct {
	ID            string    `json:"id"`
	PipelineID    string    `json:"pipeline_id"`
	PipelineName  string    `json:"pipeline_name"`
	ScanStatus    string    `json:"scan_status"`
	TotalFindings int       `json:"total_findings"`
	ErrorCount    int       `json:"error_count"`
	WarningCount  int       `json:"warning_count"`
	InfoCount     int       `json:"info_count"`
	ScriptHash    string    `json:"script_hash"`
	LastScannedAt time.Time `json:"last_scanned_at"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type PipelineScanFindingResponse struct {
	ID          string    `json:"id"`
	PipelineID  string    `json:"pipeline_id"`
	RuleID      string    `json:"rule_id"`
	RuleCode    string    `json:"rule_code"`
	RuleName    string    `json:"rule_name"`
	Severity    string    `json:"severity"`
	LineNo      int       `json:"line_no"`
	MatchedText string    `json:"matched_text"`
	Message     string    `json:"message"`
	Suggestion  string    `json:"suggestion"`
	DetailsJSON string    `json:"details_json"`
	Status      string    `json:"status"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type PipelineScanResultDataResponse struct {
	Data struct {
		Result   PipelineScanResultResponse    `json:"result"`
		Findings []PipelineScanFindingResponse `json:"findings"`
	} `json:"data"`
}

type PipelineScanResultListResponse struct {
	Data     []PipelineScanResultResponse `json:"data"`
	Page     int                          `json:"page"`
	PageSize int                          `json:"page_size"`
	Total    int64                        `json:"total"`
}

type PipelineScanBatchResponse struct {
	Data usecase.ScanPipelinesOutput `json:"data"`
}

func (h *PipelineScanHandler) ListRules(c *gin.Context) {
	if !ensurePermission(c, h.authz, "pipeline.manage", "", "") {
		return
	}
	page, pageSize, ok := parsePipelineScanPage(c)
	if !ok {
		return
	}
	enabled, hasEnabled := parseBoolQuery(c, "enabled")
	filter := scandomain.RuleListFilter{
		Keyword:  c.Query("keyword"),
		Category: scandomain.Category(c.Query("category")),
		Severity: scandomain.Severity(c.Query("severity")),
		Page:     page,
		PageSize: pageSize,
	}
	if hasEnabled {
		filter.Enabled = &enabled
	}
	items, total, err := h.manager.ListRules(c.Request.Context(), filter)
	if err != nil {
		writePipelineScanHTTPError(c, err)
		return
	}
	resp := make([]PipelineScanRuleResponse, 0, len(items))
	for _, item := range items {
		resp = append(resp, toPipelineScanRuleResponse(item))
	}
	c.JSON(http.StatusOK, PipelineScanRuleListResponse{
		Data:     resp,
		Page:     resolvedPage(page),
		PageSize: resolvedPageSize(pageSize),
		Total:    total,
	})
}

func (h *PipelineScanHandler) CreateRule(c *gin.Context) {
	if !ensurePermission(c, h.authz, "pipeline.manage", "", "") {
		return
	}
	var req PipelineScanRuleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	item, err := h.manager.CreateRule(c.Request.Context(), usecase.CreatePipelineScanRuleInput{
		RuleType:                 req.RuleType,
		RuleCode:                 req.RuleCode,
		RuleName:                 req.RuleName,
		Category:                 scandomain.Category(req.Category),
		Severity:                 scandomain.Severity(req.Severity),
		Enabled:                  req.Enabled,
		TemplateValidationScopes: req.TemplateValidationScopes,
		ScopeJSON:                req.ScopeJSON,
		RuleDSL:                  req.RuleDSL,
		Message:                  req.Message,
		Suggestion:               req.Suggestion,
	})
	if err != nil {
		writePipelineScanHTTPError(c, err)
		return
	}
	c.JSON(http.StatusCreated, PipelineScanRuleDataResponse{Data: toPipelineScanRuleResponse(item)})
}

func (h *PipelineScanHandler) GetRule(c *gin.Context) {
	if !ensurePermission(c, h.authz, "pipeline.manage", "", "") {
		return
	}
	item, err := h.manager.GetRule(c.Request.Context(), c.Param("id"))
	if err != nil {
		writePipelineScanHTTPError(c, err)
		return
	}
	c.JSON(http.StatusOK, PipelineScanRuleDataResponse{Data: toPipelineScanRuleResponse(item)})
}

func (h *PipelineScanHandler) UpdateRule(c *gin.Context) {
	if !ensurePermission(c, h.authz, "pipeline.manage", "", "") {
		return
	}
	var req PipelineScanRuleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	item, err := h.manager.UpdateRule(c.Request.Context(), c.Param("id"), usecase.UpdatePipelineScanRuleInput{
		RuleType:                 req.RuleType,
		RuleCode:                 req.RuleCode,
		RuleName:                 req.RuleName,
		Category:                 scandomain.Category(req.Category),
		Severity:                 scandomain.Severity(req.Severity),
		Enabled:                  req.Enabled,
		TemplateValidationScopes: req.TemplateValidationScopes,
		ScopeJSON:                req.ScopeJSON,
		RuleDSL:                  req.RuleDSL,
		Message:                  req.Message,
		Suggestion:               req.Suggestion,
	})
	if err != nil {
		writePipelineScanHTTPError(c, err)
		return
	}
	c.JSON(http.StatusOK, PipelineScanRuleDataResponse{Data: toPipelineScanRuleResponse(item)})
}

func (h *PipelineScanHandler) SetRuleEnabled(c *gin.Context) {
	if !ensurePermission(c, h.authz, "pipeline.manage", "", "") {
		return
	}
	var req PipelineScanRuleEnabledRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	item, err := h.manager.SetRuleEnabled(c.Request.Context(), c.Param("id"), req.Enabled)
	if err != nil {
		writePipelineScanHTTPError(c, err)
		return
	}
	c.JSON(http.StatusOK, PipelineScanRuleDataResponse{Data: toPipelineScanRuleResponse(item)})
}

func (h *PipelineScanHandler) DeleteRule(c *gin.Context) {
	if !ensurePermission(c, h.authz, "pipeline.manage", "", "") {
		return
	}
	if err := h.manager.DeleteRule(c.Request.Context(), c.Param("id")); err != nil {
		writePipelineScanHTTPError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *PipelineScanHandler) ListResults(c *gin.Context) {
	if !ensurePermission(c, h.authz, "pipeline.manage", "", "") {
		return
	}
	page, pageSize, ok := parsePipelineScanPage(c)
	if !ok {
		return
	}
	items, total, err := h.manager.ListResults(c.Request.Context(), scandomain.ResultListFilter{
		PipelineName: c.Query("pipeline_name"),
		ScanStatus:   scandomain.ScanStatus(c.Query("scan_status")),
		Page:         page,
		PageSize:     pageSize,
	})
	if err != nil {
		writePipelineScanHTTPError(c, err)
		return
	}
	resp := make([]PipelineScanResultResponse, 0, len(items))
	for _, item := range items {
		resp = append(resp, toPipelineScanResultResponse(item))
	}
	c.JSON(http.StatusOK, PipelineScanResultListResponse{
		Data:     resp,
		Page:     resolvedPage(page),
		PageSize: resolvedPageSize(pageSize),
		Total:    total,
	})
}

func (h *PipelineScanHandler) ScanAllPipelines(c *gin.Context) {
	if !ensurePermission(c, h.authz, "pipeline.manage", "", "") {
		return
	}
	output, err := h.manager.ScanActiveJenkinsPipelines(c.Request.Context())
	if err != nil {
		writePipelineScanHTTPError(c, err)
		return
	}
	c.JSON(http.StatusOK, PipelineScanBatchResponse{Data: output})
}

func (h *PipelineScanHandler) GetPipelineResult(c *gin.Context) {
	if !ensurePermission(c, h.authz, "pipeline.manage", "", "") {
		return
	}
	result, findings, err := h.manager.GetPipelineResult(c.Request.Context(), c.Param("id"))
	if err != nil {
		writePipelineScanHTTPError(c, err)
		return
	}
	writePipelineScanResult(c, result, findings)
}

func (h *PipelineScanHandler) ScanPipeline(c *gin.Context) {
	if !ensurePermission(c, h.authz, "pipeline.manage", "", "") {
		return
	}
	result, findings, err := h.manager.ScanPipeline(c.Request.Context(), c.Param("id"))
	if err != nil {
		writePipelineScanHTTPError(c, err)
		return
	}
	writePipelineScanResult(c, result, findings)
}

func writePipelineScanResult(c *gin.Context, result scandomain.Result, findings []scandomain.Finding) {
	resp := PipelineScanResultDataResponse{}
	resp.Data.Result = toPipelineScanResultResponse(result)
	resp.Data.Findings = make([]PipelineScanFindingResponse, 0, len(findings))
	for _, item := range findings {
		resp.Data.Findings = append(resp.Data.Findings, toPipelineScanFindingResponse(item))
	}
	c.JSON(http.StatusOK, resp)
}

func toPipelineScanRuleResponse(item scandomain.Rule) PipelineScanRuleResponse {
	return PipelineScanRuleResponse{
		ID:                       item.ID,
		RuleType:                 usecase.PipelineScanRuleTypeFromCode(item.RuleCode),
		RuleCode:                 item.RuleCode,
		RuleName:                 item.RuleName,
		Category:                 string(item.Category),
		Severity:                 string(item.Severity),
		Enabled:                  item.Enabled,
		Builtin:                  item.Builtin,
		TemplateValidationScopes: append([]string(nil), item.TemplateValidationScopes...),
		ScopeJSON:                item.ScopeJSON,
		RuleDSL:                  item.RuleDSL,
		Message:                  item.Message,
		Suggestion:               item.Suggestion,
		CreatedAt:                item.CreatedAt,
		UpdatedAt:                item.UpdatedAt,
	}
}

func toPipelineScanResultResponse(item scandomain.Result) PipelineScanResultResponse {
	return PipelineScanResultResponse{
		ID:            item.ID,
		PipelineID:    item.PipelineID,
		PipelineName:  item.PipelineName,
		ScanStatus:    string(item.ScanStatus),
		TotalFindings: item.TotalFindings,
		ErrorCount:    item.ErrorCount,
		WarningCount:  item.WarningCount,
		InfoCount:     item.InfoCount,
		ScriptHash:    item.ScriptHash,
		LastScannedAt: item.LastScannedAt,
		CreatedAt:     item.CreatedAt,
		UpdatedAt:     item.UpdatedAt,
	}
}

func toPipelineScanFindingResponse(item scandomain.Finding) PipelineScanFindingResponse {
	return PipelineScanFindingResponse{
		ID:          item.ID,
		PipelineID:  item.PipelineID,
		RuleID:      item.RuleID,
		RuleCode:    item.RuleCode,
		RuleName:    item.RuleName,
		Severity:    string(item.Severity),
		LineNo:      item.LineNo,
		MatchedText: item.MatchedText,
		Message:     item.Message,
		Suggestion:  item.Suggestion,
		DetailsJSON: item.DetailsJSON,
		Status:      string(item.Status),
		CreatedAt:   item.CreatedAt,
		UpdatedAt:   item.UpdatedAt,
	}
}

func parsePipelineScanPage(c *gin.Context) (int, int, bool) {
	page, err := parsePositiveInt(c, "page")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return 0, 0, false
	}
	pageSize, err := parsePositiveInt(c, "page_size")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return 0, 0, false
	}
	return page, pageSize, true
}

func parseBoolQuery(c *gin.Context, name string) (bool, bool) {
	raw, exists := c.GetQuery(name)
	if !exists {
		return false, false
	}
	switch raw {
	case "1", "true", "TRUE", "True", "yes", "on":
		return true, true
	default:
		return false, true
	}
}

func writePipelineScanHTTPError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, usecase.ErrInvalidInput),
		errors.Is(err, usecase.ErrInvalidID),
		errors.Is(err, usecase.ErrInvalidStatus),
		errors.Is(err, usecase.ErrInvalidProvider):
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	case errors.Is(err, scandomain.ErrRuleNotFound),
		errors.Is(err, scandomain.ErrResultNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
	case errors.Is(err, scandomain.ErrRuleDuplicated):
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
	}
}
