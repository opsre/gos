package httpapi

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

type ReleaseOrderValueProgressResponse struct {
	PipelineScope     string     `json:"pipeline_scope"`
	ParamKey          string     `json:"param_key"`
	ParamName         string     `json:"param_name"`
	ExecutorParamName string     `json:"executor_param_name"`
	Required          bool       `json:"required"`
	PipelineParam     bool       `json:"pipeline_param"`
	ValueKind         string     `json:"value_kind"`
	Status            string     `json:"status"`
	Value             string     `json:"value"`
	ValueSource       string     `json:"value_source"`
	Message           string     `json:"message"`
	UpdatedAt         *time.Time `json:"updated_at"`
	SortNo            int        `json:"sort_no"`
}

type ReleaseOrderValueProgressListResponse struct {
	Data []ReleaseOrderValueProgressResponse `json:"data"`
}

// ListValueProgress godoc
// @Summary      List release order value progress
// @Tags         release-orders
// @Produce      json
// @Success      200  {object}  ReleaseOrderValueProgressListResponse
// @Failure      400  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Failure      500  {object}  ErrorResponse
// @Router       /release-orders/{id}/value-progress [get]
func (h *ReleaseOrderHandler) ListValueProgress(c *gin.Context) {
	if !ensureAnyReleaseOrderDisplayPermission(c, h.authz) {
		return
	}
	if !ensurePermission(c, h.authz, "release.param_snapshot.view", "", "") {
		return
	}
	order, err := h.manager.GetByID(c.Request.Context(), c.Param("id"))
	if err != nil {
		writeReleaseOrderHTTPError(c, err)
		return
	}
	if !ensureReleaseOrderVisible(c, h.authz, order) {
		return
	}

	items, err := h.manager.ListValueProgress(c.Request.Context(), order.ID)
	if err != nil {
		writeReleaseOrderHTTPError(c, err)
		return
	}
	resp := make([]ReleaseOrderValueProgressResponse, 0, len(items))
	for _, item := range items {
		resp = append(resp, ReleaseOrderValueProgressResponse{
			PipelineScope:     string(item.PipelineScope),
			ParamKey:          item.ParamKey,
			ParamName:         item.ParamName,
			ExecutorParamName: item.ExecutorParamName,
			Required:          item.Required,
			PipelineParam:     item.PipelineParam,
			ValueKind:         string(item.ValueKind),
			Status:            string(item.Status),
			Value:             item.Value,
			ValueSource:       item.ValueSource,
			Message:           item.Message,
			UpdatedAt:         item.UpdatedAt,
			SortNo:            item.SortNo,
		})
	}
	c.JSON(http.StatusOK, ReleaseOrderValueProgressListResponse{Data: resp})
}
