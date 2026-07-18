package httpapi

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"gos/internal/application/usecase"
	domain "gos/internal/domain/release"
)

type ApprovalFlowNodeRequest struct {
	Code               string   `json:"code"`
	Name               string   `json:"name"`
	Gate               string   `json:"gate"`
	NodeType           string   `json:"node_type"`
	ApplicableEnvCodes []string `json:"applicable_env_codes"`
	ApprovalMode       string   `json:"approval_mode"`
	ApproverSource     string   `json:"approver_source"`
	ManagerLevel       int      `json:"manager_level"`
	ApproverIDs        []string `json:"approver_ids"`
	ApproverNames      []string `json:"approver_names"`
	AgentTaskID        string   `json:"agent_task_id"`
	AgentTaskName      string   `json:"agent_task_name"`
	PositionX          float64  `json:"position_x"`
	PositionY          float64  `json:"position_y"`
}

type ApprovalFlowDefinitionRequest struct {
	Name   string                    `json:"name"`
	Status string                    `json:"status"`
	Nodes  []ApprovalFlowNodeRequest `json:"nodes"`
	Links  []ApprovalFlowLinkRequest `json:"links"`
}

type ApprovalFlowLinkRequest struct {
	FromCode        string   `json:"from_code"`
	ToCode          string   `json:"to_code"`
	ExecutionScopes []string `json:"execution_scopes"`
	Priority        int      `json:"priority"`
}

type ApprovalFlowDefinitionResponse struct {
	ID        string                     `json:"id"`
	Name      string                     `json:"name"`
	Status    string                     `json:"status"`
	Nodes     []ApprovalFlowNodeResponse `json:"nodes"`
	Links     []ApprovalFlowLinkResponse `json:"links"`
	CreatedAt time.Time                  `json:"created_at"`
	UpdatedAt time.Time                  `json:"updated_at"`
}

type ApprovalFlowLinkResponse struct {
	FromCode        string   `json:"from_code"`
	ToCode          string   `json:"to_code"`
	ExecutionScopes []string `json:"execution_scopes"`
	Priority        int      `json:"priority"`
}

type ApprovalFlowNodeResponse struct {
	Code               string   `json:"code"`
	Name               string   `json:"name"`
	Gate               string   `json:"gate"`
	NodeType           string   `json:"node_type"`
	ApplicableEnvCodes []string `json:"applicable_env_codes"`
	ApprovalMode       string   `json:"approval_mode"`
	ApproverSource     string   `json:"approver_source"`
	ManagerLevel       int      `json:"manager_level"`
	ApproverIDs        []string `json:"approver_ids"`
	ApproverNames      []string `json:"approver_names"`
	AgentTaskID        string   `json:"agent_task_id"`
	AgentTaskName      string   `json:"agent_task_name"`
	PositionX          float64  `json:"position_x"`
	PositionY          float64  `json:"position_y"`
	SortNo             int      `json:"sort_no"`
}

type ReleaseOrderApprovalFlowTaskResponse struct {
	ID            string                                       `json:"id"`
	NodeCode      string                                       `json:"node_code"`
	NodeName      string                                       `json:"node_name"`
	Gate          string                                       `json:"gate"`
	NodeType      string                                       `json:"node_type"`
	ApprovalMode  string                                       `json:"approval_mode"`
	ApproverIDs   []string                                     `json:"approver_ids"`
	ApproverNames []string                                     `json:"approver_names"`
	AgentTaskID   string                                       `json:"agent_task_id"`
	AgentTaskName string                                       `json:"agent_task_name"`
	AgentBatchID  string                                       `json:"agent_batch_id"`
	Message       string                                       `json:"message"`
	Status        string                                       `json:"status"`
	Records       []ReleaseOrderApprovalFlowTaskRecordResponse `json:"records"`
	CreatedAt     time.Time                                    `json:"created_at"`
	UpdatedAt     time.Time                                    `json:"updated_at"`
}

type ReleaseOrderApprovalFlowTaskRecordResponse struct {
	ID             string    `json:"id"`
	TaskID         string    `json:"task_id"`
	Action         string    `json:"action"`
	OperatorUserID string    `json:"operator_user_id"`
	OperatorName   string    `json:"operator_name"`
	Comment        string    `json:"comment"`
	CreatedAt      time.Time `json:"created_at"`
}

type ReleaseOrderApprovalFlowResponse struct {
	ID               string                                 `json:"id"`
	FlowDefinitionID string                                 `json:"flow_definition_id"`
	FlowName         string                                 `json:"flow_name"`
	Nodes            []ApprovalFlowNodeResponse             `json:"nodes"`
	Links            []ApprovalFlowLinkResponse             `json:"links"`
	Status           string                                 `json:"status"`
	CurrentGate      string                                 `json:"current_gate"`
	CurrentScope     string                                 `json:"current_scope"`
	CurrentNodeCode  string                                 `json:"current_node_code"`
	CurrentTaskID    string                                 `json:"current_task_id"`
	Tasks            []ReleaseOrderApprovalFlowTaskResponse `json:"tasks"`
	CreatedAt        time.Time                              `json:"created_at"`
	UpdatedAt        time.Time                              `json:"updated_at"`
}

type ApprovalFlowTaskActionRequest struct {
	Comment string `json:"comment"`
}

type ReleaseApprovalWorkbenchTaskResponse struct {
	Source             string    `json:"source"`
	TaskID             string    `json:"task_id"`
	ReleaseOrderID     string    `json:"release_order_id"`
	OrderNo            string    `json:"order_no"`
	ApplicationID      string    `json:"application_id"`
	ApplicationName    string    `json:"application_name"`
	EnvCode            string    `json:"env_code"`
	OperationType      string    `json:"operation_type"`
	TriggeredBy        string    `json:"triggered_by"`
	FlowName           string    `json:"flow_name"`
	NodeName           string    `json:"node_name"`
	Gate               string    `json:"gate"`
	ExecutionScope     string    `json:"execution_scope"`
	ApprovalMode       string    `json:"approval_mode"`
	ApproverIDs        []string  `json:"approver_ids"`
	ApproverNames      []string  `json:"approver_names"`
	ReleaseOrderStatus string    `json:"release_order_status"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}

type ReleaseApprovalWorkbenchRecordResponse struct {
	ID                 string    `json:"id"`
	Source             string    `json:"source"`
	TaskID             string    `json:"task_id"`
	ReleaseOrderID     string    `json:"release_order_id"`
	OrderNo            string    `json:"order_no"`
	ApplicationID      string    `json:"application_id"`
	ApplicationName    string    `json:"application_name"`
	EnvCode            string    `json:"env_code"`
	OperationType      string    `json:"operation_type"`
	TriggeredBy        string    `json:"triggered_by"`
	FlowName           string    `json:"flow_name"`
	NodeName           string    `json:"node_name"`
	Gate               string    `json:"gate"`
	ExecutionScope     string    `json:"execution_scope"`
	Action             string    `json:"action"`
	OperatorUserID     string    `json:"operator_user_id"`
	OperatorName       string    `json:"operator_name"`
	Comment            string    `json:"comment"`
	ReleaseOrderStatus string    `json:"release_order_status"`
	CreatedAt          time.Time `json:"created_at"`
}

func (h *ReleaseOrderHandler) ListApprovalWorkbench(c *gin.Context) {
	if !ensureAnyReleaseOrderDisplayPermission(c, h.authz) {
		return
	}
	user, ok := getCurrentUser(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
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
	input := usecase.ListApprovalWorkbenchInput{UserID: user.ID, Page: page, PageSize: pageSize}
	view := strings.TrimSpace(c.DefaultQuery("view", "pending"))
	if view == "pending" {
		items, total, listErr := h.manager.ListApprovalWorkbenchTasks(c.Request.Context(), input)
		if listErr != nil {
			writeReleaseOrderHTTPError(c, listErr)
			return
		}
		resp := make([]ReleaseApprovalWorkbenchTaskResponse, 0, len(items))
		for _, item := range items {
			resp = append(resp, toReleaseApprovalWorkbenchTaskResponse(item))
		}
		c.JSON(http.StatusOK, gin.H{"data": resp, "page": resolvedPage(page), "page_size": resolvedPageSize(pageSize), "total": total})
		return
	}
	if view == "handled" {
		items, total, listErr := h.manager.ListApprovalWorkbenchRecords(c.Request.Context(), input)
		if listErr != nil {
			writeReleaseOrderHTTPError(c, listErr)
			return
		}
		resp := make([]ReleaseApprovalWorkbenchRecordResponse, 0, len(items))
		for _, item := range items {
			resp = append(resp, toReleaseApprovalWorkbenchRecordResponse(item))
		}
		c.JSON(http.StatusOK, gin.H{"data": resp, "page": resolvedPage(page), "page_size": resolvedPageSize(pageSize), "total": total})
		return
	}
	c.JSON(http.StatusBadRequest, gin.H{"error": "view must be pending or handled"})
}

func (h *ReleaseOrderHandler) ListApprovalFlows(c *gin.Context) {
	if !ensureAnyReleaseOrderDisplayPermission(c, h.authz) {
		return
	}
	status := domain.ApprovalFlowStatus(strings.TrimSpace(c.Query("status")))
	items, err := h.manager.ListApprovalFlowDefinitions(c.Request.Context(), status)
	if err != nil {
		writeReleaseOrderHTTPError(c, err)
		return
	}
	resp := make([]ApprovalFlowDefinitionResponse, 0, len(items))
	for _, item := range items {
		resp = append(resp, toApprovalFlowDefinitionResponse(item))
	}
	c.JSON(http.StatusOK, gin.H{"data": resp})
}

func (h *ReleaseOrderHandler) CreateApprovalFlow(c *gin.Context) {
	if !ensurePermission(c, h.authz, "release.template.manage", "", "") {
		return
	}
	var req ApprovalFlowDefinitionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	item, err := h.manager.CreateApprovalFlowDefinition(c.Request.Context(), toSaveApprovalFlowDefinitionInput("", req))
	if err != nil {
		writeReleaseOrderHTTPError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"data": toApprovalFlowDefinitionResponse(item)})
}

func (h *ReleaseOrderHandler) UpdateApprovalFlow(c *gin.Context) {
	if !ensurePermission(c, h.authz, "release.template.manage", "", "") {
		return
	}
	var req ApprovalFlowDefinitionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	item, err := h.manager.UpdateApprovalFlowDefinition(c.Request.Context(), toSaveApprovalFlowDefinitionInput(c.Param("id"), req))
	if err != nil {
		writeReleaseOrderHTTPError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": toApprovalFlowDefinitionResponse(item)})
}

func (h *ReleaseOrderHandler) GetApprovalFlow(c *gin.Context) {
	existing, err := h.manager.GetByID(c.Request.Context(), c.Param("id"))
	if err != nil {
		writeReleaseOrderHTTPError(c, err)
		return
	}
	if !ensureReleaseOrderVisible(c, h.authz, existing) {
		return
	}
	instance, tasks, err := h.manager.GetApprovalFlowInstance(c.Request.Context(), existing.ID)
	if errors.Is(err, domain.ErrApprovalFlowInstanceNotFound) {
		c.JSON(http.StatusOK, gin.H{"data": nil})
		return
	}
	if err != nil {
		writeReleaseOrderHTTPError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": toReleaseOrderApprovalFlowResponse(instance, tasks)})
}

func (h *ReleaseOrderHandler) ApproveApprovalFlowTask(c *gin.Context) {
	h.actApprovalFlowTask(c, domain.ReleaseOrderApprovalActionApprove)
}

func (h *ReleaseOrderHandler) RejectApprovalFlowTask(c *gin.Context) {
	h.actApprovalFlowTask(c, domain.ReleaseOrderApprovalActionReject)
}

func (h *ReleaseOrderHandler) actApprovalFlowTask(c *gin.Context, action domain.ReleaseOrderApprovalAction) {
	existing, err := h.manager.GetByID(c.Request.Context(), c.Param("id"))
	if err != nil {
		writeReleaseOrderHTTPError(c, err)
		return
	}
	user, ok := getCurrentUser(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	assignedToCurrentTask := false
	_, tasks, flowErr := h.manager.GetApprovalFlowInstance(c.Request.Context(), existing.ID)
	if flowErr == nil {
		for _, task := range tasks {
			if task.ID != c.Param("task_id") {
				continue
			}
			for _, approverID := range task.ApproverIDs {
				if strings.TrimSpace(approverID) == strings.TrimSpace(user.ID) {
					assignedToCurrentTask = true
					break
				}
			}
			break
		}
	}
	if !assignedToCurrentTask && !ensureReleaseOrderVisible(c, h.authz, existing) {
		return
	}
	var req ApprovalFlowTaskActionRequest
	if err := c.ShouldBindJSON(&req); err != nil && !errors.Is(err, io.EOF) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	var task domain.ReleaseOrderApprovalFlowTask
	if action == domain.ReleaseOrderApprovalActionApprove {
		task, err = h.manager.ApproveApprovalFlowTask(c.Request.Context(), existing.ID, c.Param("task_id"), user.ID, resolveTriggeredBy(user), req.Comment)
	} else {
		task, err = h.manager.RejectApprovalFlowTask(c.Request.Context(), existing.ID, c.Param("task_id"), user.ID, resolveTriggeredBy(user), req.Comment)
	}
	if err != nil {
		writeReleaseOrderHTTPError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": toReleaseOrderApprovalFlowTaskResponse(task)})
}

func toSaveApprovalFlowDefinitionInput(id string, req ApprovalFlowDefinitionRequest) usecase.SaveApprovalFlowDefinitionInput {
	nodes := make([]domain.ApprovalFlowNode, 0, len(req.Nodes))
	for _, item := range req.Nodes {
		nodes = append(nodes, domain.ApprovalFlowNode{Code: item.Code, Name: item.Name, Gate: domain.ApprovalFlowGate(item.Gate), NodeType: domain.ApprovalFlowNodeType(item.NodeType), ApplicableEnvCodes: item.ApplicableEnvCodes, ApprovalMode: domain.TemplateApprovalMode(item.ApprovalMode), ApproverSource: domain.ApprovalFlowApproverSource(item.ApproverSource), ManagerLevel: item.ManagerLevel, ApproverIDs: item.ApproverIDs, ApproverNames: item.ApproverNames, AgentTaskID: item.AgentTaskID, AgentTaskName: item.AgentTaskName, PositionX: item.PositionX, PositionY: item.PositionY})
	}
	links := make([]domain.ApprovalFlowLink, 0, len(req.Links))
	for _, item := range req.Links {
		links = append(links, domain.ApprovalFlowLink{FromCode: item.FromCode, ToCode: item.ToCode, ExecutionScopes: item.ExecutionScopes, Priority: item.Priority})
	}
	return usecase.SaveApprovalFlowDefinitionInput{ID: strings.TrimSpace(id), Name: req.Name, Status: domain.ApprovalFlowStatus(req.Status), Nodes: nodes, Links: links}
}

func toApprovalFlowDefinitionResponse(item domain.ApprovalFlowDefinition) ApprovalFlowDefinitionResponse {
	resp := ApprovalFlowDefinitionResponse{ID: item.ID, Name: item.Name, Status: string(item.Status), CreatedAt: item.CreatedAt, UpdatedAt: item.UpdatedAt, Nodes: make([]ApprovalFlowNodeResponse, 0, len(item.Nodes)), Links: make([]ApprovalFlowLinkResponse, 0, len(item.Links))}
	for _, node := range item.Nodes {
		resp.Nodes = append(resp.Nodes, toApprovalFlowNodeResponse(node))
	}
	for _, link := range item.Links {
		resp.Links = append(resp.Links, ApprovalFlowLinkResponse{FromCode: link.FromCode, ToCode: link.ToCode, ExecutionScopes: append([]string(nil), link.ExecutionScopes...), Priority: link.Priority})
	}
	return resp
}

func toApprovalFlowNodeResponse(item domain.ApprovalFlowNode) ApprovalFlowNodeResponse {
	return ApprovalFlowNodeResponse{Code: item.Code, Name: item.Name, Gate: string(item.Gate), NodeType: string(item.NodeType), ApplicableEnvCodes: append([]string(nil), item.ApplicableEnvCodes...), ApprovalMode: string(item.ApprovalMode), ApproverSource: string(item.ApproverSource), ManagerLevel: item.ManagerLevel, ApproverIDs: append([]string(nil), item.ApproverIDs...), ApproverNames: append([]string(nil), item.ApproverNames...), AgentTaskID: item.AgentTaskID, AgentTaskName: item.AgentTaskName, PositionX: item.PositionX, PositionY: item.PositionY, SortNo: item.SortNo}
}

func toReleaseOrderApprovalFlowResponse(instance domain.ReleaseOrderApprovalFlowInstance, tasks []domain.ReleaseOrderApprovalFlowTask) ReleaseOrderApprovalFlowResponse {
	resp := ReleaseOrderApprovalFlowResponse{
		ID: instance.ID, FlowDefinitionID: instance.FlowDefinitionID, FlowName: instance.FlowName,
		Nodes: make([]ApprovalFlowNodeResponse, 0, len(instance.Nodes)), Links: make([]ApprovalFlowLinkResponse, 0, len(instance.Links)),
		Status: string(instance.Status), CurrentGate: string(instance.CurrentGate), CurrentScope: string(instance.CurrentScope),
		CurrentNodeCode: instance.CurrentNodeCode, CurrentTaskID: instance.CurrentTaskID,
		CreatedAt: instance.CreatedAt, UpdatedAt: instance.UpdatedAt, Tasks: make([]ReleaseOrderApprovalFlowTaskResponse, 0, len(tasks)),
	}
	for _, node := range instance.Nodes {
		resp.Nodes = append(resp.Nodes, toApprovalFlowNodeResponse(node))
	}
	for _, link := range instance.Links {
		resp.Links = append(resp.Links, ApprovalFlowLinkResponse{
			FromCode: link.FromCode, ToCode: link.ToCode,
			ExecutionScopes: append([]string(nil), link.ExecutionScopes...), Priority: link.Priority,
		})
	}
	for _, task := range tasks {
		resp.Tasks = append(resp.Tasks, toReleaseOrderApprovalFlowTaskResponse(task))
	}
	return resp
}

func toReleaseOrderApprovalFlowTaskResponse(item domain.ReleaseOrderApprovalFlowTask) ReleaseOrderApprovalFlowTaskResponse {
	response := ReleaseOrderApprovalFlowTaskResponse{
		ID: item.ID, NodeCode: item.NodeCode, NodeName: item.NodeName, Gate: string(item.Gate), NodeType: string(item.NodeType),
		ApprovalMode: string(item.ApprovalMode), ApproverIDs: append([]string(nil), item.ApproverIDs...), ApproverNames: append([]string(nil), item.ApproverNames...),
		AgentTaskID: item.AgentTaskID, AgentTaskName: item.AgentTaskName, AgentBatchID: item.AgentBatchID,
		Message: item.Message, Status: string(item.Status), Records: make([]ReleaseOrderApprovalFlowTaskRecordResponse, 0, len(item.Records)),
		CreatedAt: item.CreatedAt, UpdatedAt: item.UpdatedAt,
	}
	for _, record := range item.Records {
		response.Records = append(response.Records, ReleaseOrderApprovalFlowTaskRecordResponse{
			ID: record.ID, TaskID: record.TaskID, Action: string(record.Action), OperatorUserID: record.OperatorUserID,
			OperatorName: record.OperatorName, Comment: record.Comment, CreatedAt: record.CreatedAt,
		})
	}
	return response
}

func toReleaseApprovalWorkbenchTaskResponse(item domain.ReleaseApprovalWorkbenchTask) ReleaseApprovalWorkbenchTaskResponse {
	return ReleaseApprovalWorkbenchTaskResponse{
		Source: string(item.Source), TaskID: item.TaskID, ReleaseOrderID: item.ReleaseOrderID, OrderNo: item.OrderNo,
		ApplicationID: item.ApplicationID, ApplicationName: item.ApplicationName, EnvCode: item.EnvCode,
		OperationType: string(item.OperationType), TriggeredBy: item.TriggeredBy, FlowName: item.FlowName,
		NodeName: item.NodeName, Gate: string(item.Gate), ExecutionScope: string(item.ExecutionScope),
		ApprovalMode: string(item.ApprovalMode), ApproverIDs: append([]string(nil), item.ApproverIDs...),
		ApproverNames: append([]string(nil), item.ApproverNames...), ReleaseOrderStatus: string(item.ReleaseOrderStatus),
		CreatedAt: item.CreatedAt, UpdatedAt: item.UpdatedAt,
	}
}

func toReleaseApprovalWorkbenchRecordResponse(item domain.ReleaseApprovalWorkbenchRecord) ReleaseApprovalWorkbenchRecordResponse {
	return ReleaseApprovalWorkbenchRecordResponse{
		ID: item.ID, Source: string(item.Source), TaskID: item.TaskID, ReleaseOrderID: item.ReleaseOrderID,
		OrderNo: item.OrderNo, ApplicationID: item.ApplicationID, ApplicationName: item.ApplicationName,
		EnvCode: item.EnvCode, OperationType: string(item.OperationType), TriggeredBy: item.TriggeredBy,
		FlowName: item.FlowName, NodeName: item.NodeName, Gate: string(item.Gate), ExecutionScope: string(item.ExecutionScope),
		Action: string(item.Action), OperatorUserID: item.OperatorUserID, OperatorName: item.OperatorName,
		Comment: item.Comment, ReleaseOrderStatus: string(item.ReleaseOrderStatus), CreatedAt: item.CreatedAt,
	}
}
