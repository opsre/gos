package httpapi

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"gos/internal/application/usecase"
	aidomain "gos/internal/domain/ai"
)

type CreateStageDiagnosisRequest struct {
	ForceRefresh bool `json:"force_refresh"`
}

type StageDiagnosisDataResponse struct {
	Data usecase.StageDiagnosisOutput `json:"data"`
}

type FollowUpStageDiagnosisRequest struct {
	Question string                                  `json:"question"`
	Messages []usecase.StageDiagnosisFollowUpMessage `json:"messages"`
}

type StageDiagnosisFollowUpDataResponse struct {
	Data usecase.StageDiagnosisFollowUpOutput `json:"data"`
}

func (h *ReleaseOrderHandler) CreatePipelineStageDiagnosis(c *gin.Context) {
	order, err := h.manager.GetByID(c.Request.Context(), c.Param("id"))
	if err != nil {
		writeReleaseOrderHTTPError(c, err)
		return
	}
	if !ensureReleaseOrderVisible(c, h.authz, order) {
		return
	}
	var req CreateStageDiagnosisRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	user, _ := getCurrentUser(c)
	output, err := h.manager.DiagnosePipelineStage(c.Request.Context(), order.ID, c.Param("stage_id"), usecase.StageDiagnosisInput{
		ForceRefresh: req.ForceRefresh,
		CreatedBy:    user.ID,
	})
	if err != nil {
		writeStageDiagnosisHTTPError(c, err)
		return
	}
	c.JSON(http.StatusOK, StageDiagnosisDataResponse{Data: output})
}

func (h *ReleaseOrderHandler) FollowUpPipelineStageDiagnosis(c *gin.Context) {
	order, err := h.manager.GetByID(c.Request.Context(), c.Param("id"))
	if err != nil {
		writeReleaseOrderHTTPError(c, err)
		return
	}
	if !ensureReleaseOrderVisible(c, h.authz, order) {
		return
	}
	var req FollowUpStageDiagnosisRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	user, _ := getCurrentUser(c)
	output, err := h.manager.FollowUpPipelineStageDiagnosis(c.Request.Context(), order.ID, c.Param("stage_id"), c.Param("diagnosis_id"), usecase.StageDiagnosisFollowUpInput{
		Question:  req.Question,
		Messages:  req.Messages,
		CreatedBy: user.ID,
	})
	if err != nil {
		writeStageDiagnosisHTTPError(c, err)
		return
	}
	c.JSON(http.StatusOK, StageDiagnosisFollowUpDataResponse{Data: output})
}

func (h *ReleaseOrderHandler) GetLatestPipelineStageDiagnosis(c *gin.Context) {
	order, err := h.manager.GetByID(c.Request.Context(), c.Param("id"))
	if err != nil {
		writeReleaseOrderHTTPError(c, err)
		return
	}
	if !ensureReleaseOrderVisible(c, h.authz, order) {
		return
	}
	output, err := h.manager.GetLatestPipelineStageDiagnosis(c.Request.Context(), order.ID, c.Param("stage_id"))
	if err != nil {
		writeStageDiagnosisHTTPError(c, err)
		return
	}
	c.JSON(http.StatusOK, StageDiagnosisDataResponse{Data: output})
}

func (h *ReleaseOrderHandler) GetPipelineStageDiagnosis(c *gin.Context) {
	order, err := h.manager.GetByID(c.Request.Context(), c.Param("id"))
	if err != nil {
		writeReleaseOrderHTTPError(c, err)
		return
	}
	if !ensureReleaseOrderVisible(c, h.authz, order) {
		return
	}
	output, err := h.manager.GetPipelineStageDiagnosisByID(c.Request.Context(), order.ID, c.Param("diagnosis_id"))
	if err != nil {
		writeStageDiagnosisHTTPError(c, err)
		return
	}
	c.JSON(http.StatusOK, StageDiagnosisDataResponse{Data: output})
}

func writeStageDiagnosisHTTPError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, usecase.ErrInvalidInput), errors.Is(err, usecase.ErrInvalidID):
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	case errors.Is(err, aidomain.ErrDiagnosisModelNotConfigured):
		c.JSON(http.StatusBadRequest, gin.H{"error": "AI 诊断模型未配置，请先到系统设置中设置诊断模型"})
	case errors.Is(err, aidomain.ErrStageDiagnosisNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
	default:
		writeReleaseOrderHTTPError(c, err)
	}
}
