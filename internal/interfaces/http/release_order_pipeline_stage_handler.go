package httpapi

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	domain "gos/internal/domain/release"
)

type ReleaseOrderPipelineStageResponse struct {
	ID             string     `json:"id"`
	ReleaseOrderID string     `json:"release_order_id"`
	PipelineScope  string     `json:"pipeline_scope"`
	ExecutorType   string     `json:"executor_type"`
	StageName      string     `json:"stage_name"`
	Status         string     `json:"status"`
	RawStatus      string     `json:"raw_status"`
	SortNo         int        `json:"sort_no"`
	DurationMillis int64      `json:"duration_millis"`
	StartedAt      *time.Time `json:"started_at"`
	FinishedAt     *time.Time `json:"finished_at"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

type ReleaseOrderPipelineStageListResponse struct {
	ShowModule   bool                                `json:"show_module"`
	ExecutorType string                              `json:"executor_type"`
	Message      string                              `json:"message,omitempty"`
	Data         []ReleaseOrderPipelineStageResponse `json:"data"`
}

type ReleaseOrderPipelineStageLogResponse struct {
	Data struct {
		Stage     ReleaseOrderPipelineStageResponse `json:"stage"`
		Content   string                            `json:"content"`
		HasMore   bool                              `json:"has_more"`
		RawStatus string                            `json:"raw_status"`
		FetchedAt time.Time                         `json:"fetched_at"`
	} `json:"data"`
}

// ListPipelineStages 查询发布单流水线阶段列表。
// @Summary      查询发布单流水线阶段列表
// @Description  查询发布单 CI/CD 流水线阶段状态，并按统一响应结构返回处理结果。
// @Tags         release-orders
// @Produce      json
// @Param        id  path  string  true  "资源 ID"
// @Success      200  {object}  ReleaseOrderPipelineStageListResponse
// @Failure      400  {object}  ErrorResponse
// @Failure      401  {object}  ErrorResponse
// @Failure      403  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Failure      500  {object}  ErrorResponse
// @Router       /release-orders/{id}/pipeline-stages [get]
func (h *ReleaseOrderHandler) ListPipelineStages(c *gin.Context) {
	order, err := h.manager.GetByID(c.Request.Context(), c.Param("id"))
	if err != nil {
		writeReleaseOrderHTTPError(c, err)
		return
	}
	if !ensureReleaseOrderVisible(c, h.authz, order) {
		return
	}

	view, err := h.manager.ListPipelineStagesView(c.Request.Context(), order.ID)
	if err != nil {
		writeReleaseOrderHTTPError(c, err)
		return
	}

	resp := make([]ReleaseOrderPipelineStageResponse, 0, len(view.Stages))
	for _, item := range view.Stages {
		resp = append(resp, toReleaseOrderPipelineStageResponse(item))
	}
	c.JSON(http.StatusOK, ReleaseOrderPipelineStageListResponse{
		ShowModule:   view.ShowModule,
		ExecutorType: view.ExecutorType,
		Message:      view.Message,
		Data:         resp,
	})
}

// GetPipelineStageLog 获取发布单流水线阶段日志。
// @Summary      获取发布单流水线阶段日志
// @Description  获取指定发布单阶段的构建日志内容，并按统一响应结构返回处理结果。
// @Tags         release-orders
// @Produce      json
// @Param        id        path  string  true  "资源 ID"
// @Param        stage_id  path  string  true  "阶段 ID"
// @Success      200  {object}  ReleaseOrderPipelineStageLogResponse
// @Failure      400  {object}  ErrorResponse
// @Failure      401  {object}  ErrorResponse
// @Failure      403  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Failure      500  {object}  ErrorResponse
// @Router       /release-orders/{id}/pipeline-stages/{stage_id}/log [get]
func (h *ReleaseOrderHandler) GetPipelineStageLog(c *gin.Context) {
	order, err := h.manager.GetByID(c.Request.Context(), c.Param("id"))
	if err != nil {
		writeReleaseOrderHTTPError(c, err)
		return
	}
	if !ensureReleaseOrderVisible(c, h.authz, order) {
		return
	}

	stage, stageLog, err := h.manager.GetPipelineStageLog(c.Request.Context(), order.ID, c.Param("stage_id"))
	if err != nil {
		writeReleaseOrderHTTPError(c, err)
		return
	}

	resp := ReleaseOrderPipelineStageLogResponse{}
	resp.Data.Stage = toReleaseOrderPipelineStageResponse(stage)
	resp.Data.Content = stageLog.Content
	resp.Data.HasMore = stageLog.HasMore
	resp.Data.RawStatus = stageLog.RawStatus
	resp.Data.FetchedAt = stageLog.FetchedAt
	c.JSON(http.StatusOK, resp)
}

// toReleaseOrderPipelineStageResponse 将领域对象转换为接口响应结构。
func toReleaseOrderPipelineStageResponse(item domain.ReleaseOrderPipelineStage) ReleaseOrderPipelineStageResponse {
	return ReleaseOrderPipelineStageResponse{
		ID:             item.ID,
		ReleaseOrderID: item.ReleaseOrderID,
		PipelineScope:  item.PipelineScope,
		ExecutorType:   item.ExecutorType,
		StageName:      item.StageName,
		Status:         string(item.Status),
		RawStatus:      item.RawStatus,
		SortNo:         item.SortNo,
		DurationMillis: item.DurationMillis,
		StartedAt:      item.StartedAt,
		FinishedAt:     item.FinishedAt,
		CreatedAt:      item.CreatedAt,
		UpdatedAt:      item.UpdatedAt,
	}
}
