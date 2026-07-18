package httpapi

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

type ApplicationApprovalFlowBindingRequest struct {
	ApprovalFlowID string `json:"approval_flow_id"`
}

func (h *ApplicationHandler) GetApprovalFlowBinding(c *gin.Context) {
	if h.approvalFlowManager == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "approval flow manager is not configured"})
		return
	}
	applicationID := strings.TrimSpace(c.Param("id"))
	if !ensureApplicationVisible(c, h.authz, applicationID) {
		return
	}
	flowID, err := h.approvalFlowManager.GetApplicationApprovalFlowID(c.Request.Context(), applicationID)
	if err != nil {
		writeHTTPError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": gin.H{"application_id": applicationID, "approval_flow_id": flowID}})
}

func (h *ApplicationHandler) UpdateApprovalFlowBinding(c *gin.Context) {
	if h.approvalFlowManager == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "approval flow manager is not configured"})
		return
	}
	if !ensurePermission(c, h.authz, "application.manage", "", "") {
		return
	}
	var req ApplicationApprovalFlowBindingRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	applicationID := strings.TrimSpace(c.Param("id"))
	if _, err := h.query.GetByID(c.Request.Context(), applicationID); err != nil {
		writeHTTPError(c, err)
		return
	}
	if err := h.approvalFlowManager.SetApplicationApprovalFlowID(c.Request.Context(), applicationID, req.ApprovalFlowID); err != nil {
		writeHTTPError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": gin.H{"application_id": applicationID, "approval_flow_id": strings.TrimSpace(req.ApprovalFlowID)}})
}
