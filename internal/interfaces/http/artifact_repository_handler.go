package httpapi

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"gos/internal/application/usecase"
	domain "gos/internal/domain/artifactrepo"
)

type ArtifactRepositoryHandler struct {
	manager *usecase.ArtifactRepositoryManager
	authz   RequestAuthorizer
}

func NewArtifactRepositoryHandler(manager *usecase.ArtifactRepositoryManager, authz RequestAuthorizer) *ArtifactRepositoryHandler {
	return &ArtifactRepositoryHandler{
		manager: manager,
		authz:   authz,
	}
}

func (h *ArtifactRepositoryHandler) RegisterRoutes(router gin.IRouter) {
	router.POST("/artifact-repositories/actions/test-connection", h.TestConnection)
	router.POST("/artifact-repositories", h.Create)
	router.GET("/artifact-repositories", h.List)
	router.GET("/artifact-repositories/:id", h.GetByID)
	router.PUT("/artifact-repositories/:id", h.Update)
	router.DELETE("/artifact-repositories/:id", h.Delete)
}

type ArtifactRepositoryRequest struct {
	Name               string `json:"name"`
	RepositoryType     string `json:"type"`
	Endpoint           string `json:"endpoint"`
	Port               int    `json:"port"`
	Bucket             string `json:"bucket"`
	Directory          string `json:"directory"`
	AccessKeyID        string `json:"access_key_id"`
	AccessKeySecret    string `json:"access_key_secret"`
	Username           string `json:"username"`
	Password           string `json:"password"`
	PrivateKey         string `json:"private_key"`
	DisableEPSV        bool   `json:"disable_epsv"`
	HostKeyFingerprint string `json:"host_key_fingerprint"`
	ACL                string `json:"acl"`
	Status             string `json:"status"`
}

// ArtifactRepositoryResponse deliberately carries no credential value. This
// module used to echo the object storage secret to the browser, which is the one
// place the platform diverged from the "never return a secret" convention that
// GitOps, ArgoCD, Agent, notification and AI model config all follow. The two
// configured flags let the UI show credential state without the secret itself,
// and a blank credential on update means "keep the stored one".
type ArtifactRepositoryResponse struct {
	ID                   string    `json:"id"`
	Name                 string    `json:"name"`
	RepositoryType       string    `json:"type"`
	Endpoint             string    `json:"endpoint"`
	Port                 int       `json:"port"`
	Bucket               string    `json:"bucket"`
	Directory            string    `json:"directory"`
	AccessKeyID          string    `json:"access_key_id"`
	Username             string    `json:"username"`
	DisableEPSV          bool      `json:"disable_epsv"`
	HostKeyFingerprint   string    `json:"host_key_fingerprint"`
	SecretConfigured     bool      `json:"secret_configured"`
	PrivateKeyConfigured bool      `json:"private_key_configured"`
	ACL                  string    `json:"acl"`
	Status               string    `json:"status"`
	CreatedAt            time.Time `json:"created_at"`
	UpdatedAt            time.Time `json:"updated_at"`
}

type ArtifactRepositoryDataResponse struct {
	Data ArtifactRepositoryResponse `json:"data"`
}

type ArtifactRepositoryListResponse struct {
	Data     []ArtifactRepositoryResponse `json:"data"`
	Page     int                          `json:"page"`
	PageSize int                          `json:"page_size"`
	Total    int64                        `json:"total"`
}

type ArtifactRepositoryConnectionTestResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

func (h *ArtifactRepositoryHandler) Create(c *gin.Context) {
	if !ensurePermission(c, h.authz, "artifact_repo.manage", "", "") {
		return
	}
	var req ArtifactRepositoryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	item, err := h.manager.Create(c.Request.Context(), toArtifactRepositoryInput(req))
	if err != nil {
		writeArtifactRepositoryHTTPError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"data": toArtifactRepositoryResponse(item)})
}

func (h *ArtifactRepositoryHandler) TestConnection(c *gin.Context) {
	if !ensurePermission(c, h.authz, "artifact_repo.manage", "", "") {
		return
	}
	var req ArtifactRepositoryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	result, err := h.manager.TestConnection(c.Request.Context(), toArtifactRepositoryInput(req))
	if err != nil {
		writeArtifactRepositoryHTTPError(c, err)
		return
	}
	c.JSON(http.StatusOK, ArtifactRepositoryConnectionTestResponse{
		Success: result.Success,
		Message: result.Message,
	})
}

func (h *ArtifactRepositoryHandler) List(c *gin.Context) {
	if !ensurePermission(c, h.authz, "artifact_repo.manage", "", "") {
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
	items, total, err := h.manager.List(c.Request.Context(), domain.ListFilter{
		Keyword:        c.Query("keyword"),
		RepositoryType: domain.RepositoryType(strings.TrimSpace(c.Query("type"))),
		Status:         domain.Status(strings.TrimSpace(c.Query("status"))),
		Page:           page,
		PageSize:       pageSize,
	})
	if err != nil {
		writeArtifactRepositoryHTTPError(c, err)
		return
	}
	resp := make([]ArtifactRepositoryResponse, 0, len(items))
	for _, item := range items {
		resp = append(resp, toArtifactRepositoryResponse(item))
	}
	c.JSON(http.StatusOK, gin.H{
		"data":      resp,
		"page":      resolvedPage(page),
		"page_size": resolvedPageSize(pageSize),
		"total":     total,
	})
}

func (h *ArtifactRepositoryHandler) GetByID(c *gin.Context) {
	if !ensurePermission(c, h.authz, "artifact_repo.manage", "", "") {
		return
	}
	item, err := h.manager.GetByID(c.Request.Context(), c.Param("id"))
	if err != nil {
		writeArtifactRepositoryHTTPError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": toArtifactRepositoryResponse(item)})
}

func (h *ArtifactRepositoryHandler) Update(c *gin.Context) {
	if !ensurePermission(c, h.authz, "artifact_repo.manage", "", "") {
		return
	}
	var req ArtifactRepositoryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	item, err := h.manager.Update(c.Request.Context(), c.Param("id"), toArtifactRepositoryInput(req))
	if err != nil {
		writeArtifactRepositoryHTTPError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": toArtifactRepositoryResponse(item)})
}

func (h *ArtifactRepositoryHandler) Delete(c *gin.Context) {
	if !ensurePermission(c, h.authz, "artifact_repo.manage", "", "") {
		return
	}
	if err := h.manager.Delete(c.Request.Context(), c.Param("id")); err != nil {
		writeArtifactRepositoryHTTPError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func toArtifactRepositoryInput(req ArtifactRepositoryRequest) usecase.ArtifactRepositoryInput {
	return usecase.ArtifactRepositoryInput{
		Name:               req.Name,
		RepositoryType:     domain.RepositoryType(strings.TrimSpace(req.RepositoryType)),
		Endpoint:           req.Endpoint,
		Port:               req.Port,
		Bucket:             req.Bucket,
		Directory:          req.Directory,
		AccessKeyID:        req.AccessKeyID,
		AccessKeySecret:    req.AccessKeySecret,
		Username:           req.Username,
		Password:           req.Password,
		PrivateKey:         req.PrivateKey,
		DisableEPSV:        req.DisableEPSV,
		HostKeyFingerprint: req.HostKeyFingerprint,
		ACL:                domain.ACL(strings.TrimSpace(req.ACL)),
		Status:             domain.Status(strings.TrimSpace(req.Status)),
	}
}

func toArtifactRepositoryResponse(item domain.ArtifactRepository) ArtifactRepositoryResponse {
	return ArtifactRepositoryResponse{
		ID:                   item.ID,
		Name:                 item.Name,
		RepositoryType:       string(item.RepositoryType),
		Endpoint:             item.Endpoint,
		Port:                 item.Port,
		Bucket:               item.Bucket,
		Directory:            item.Directory,
		AccessKeyID:          item.AccessKeyID,
		Username:             item.Username,
		DisableEPSV:          item.DisableEPSV,
		HostKeyFingerprint:   item.HostKeyFingerprint,
		SecretConfigured:     artifactRepositorySecretConfigured(item),
		PrivateKeyConfigured: strings.TrimSpace(item.PrivateKey) != "",
		ACL:                  string(item.ACL),
		Status:               string(item.Status),
		CreatedAt:            item.CreatedAt,
		UpdatedAt:            item.UpdatedAt,
	}
}

// artifactRepositorySecretConfigured reports whether the credential this
// repository type actually uses is present, so the UI can show one state tag
// next to the matching input.
func artifactRepositorySecretConfigured(item domain.ArtifactRepository) bool {
	if item.RepositoryType.UsesObjectStorage() {
		return strings.TrimSpace(item.AccessKeySecret) != ""
	}
	return strings.TrimSpace(item.Password) != ""
}

func writeArtifactRepositoryHTTPError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, usecase.ErrInvalidInput),
		errors.Is(err, usecase.ErrInvalidID),
		errors.Is(err, usecase.ErrInvalidStatus):
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	case errors.Is(err, domain.ErrNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
	case errors.Is(err, domain.ErrNameDuplicated):
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
	case errors.Is(err, usecase.ErrArtifactConnectionFailed):
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
	}
}
