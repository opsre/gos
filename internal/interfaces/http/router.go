package httpapi

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"

	_ "gos/docs"
)

// NewRouter 创建并返回对应组件实例。
func NewRouter(
	authHandler *AuthHandler,
	agentHandler *AgentHandler,
	userHandler *UserHandler,
	sessionResolver SessionUserResolver,
	applicationHandler *ApplicationHandler,
	projectHandler *ProjectHandler,
	systemSettingsHandler *SystemSettingsHandler,
	pipelineHandler *PipelineHandler,
	argocdHandler *ArgoCDHandler,
	gitopsHandler *GitOpsHandler,
	platformParamHandler *PlatformParamHandler,
	notificationHandler *NotificationHandler,
	executorParamHandler *ExecutorParamHandler,
	releaseOrderHandler *ReleaseOrderHandler,
	releaseTemplateHandler *ReleaseTemplateHandler,
	announcementHandler *AnnouncementHandler,
) *gin.Engine {
	router := gin.New()
	router.Use(gin.Logger(), gin.Recovery())
	router.Use(cors())
	registerSystemRoutes(router)
	registerPublicAuthRoutes(router, authHandler)
	registerPublicAgentRoutes(router, agentHandler)
	router.Use(authMiddleware(sessionResolver))
	registerProtectedAuthRoutes(router, authHandler)
	registerAgentRoutes(router, agentHandler)
	registerUserRoutes(router, userHandler)
	registerApplicationRoutes(router, applicationHandler)
	registerProjectRoutes(router, projectHandler)
	registerSystemSettingsRoutes(router, systemSettingsHandler)
	registerPipelineRoutes(router, pipelineHandler)
	registerArgoCDRoutes(router, argocdHandler)
	registerGitOpsRoutes(router, gitopsHandler)
	registerPlatformParamRoutes(router, platformParamHandler)
	registerNotificationRoutes(router, notificationHandler)
	registerExecutorParamRoutes(router, executorParamHandler)
	registerReleaseOrderRoutes(router, releaseOrderHandler)
	registerReleaseTemplateRoutes(router, releaseTemplateHandler)
	registerAnnouncementRoutes(router, announcementHandler)
	return router
}

// registerPublicAgentRoutes 封装当前模块的业务处理逻辑。
func registerPublicAgentRoutes(router gin.IRouter, agentHandler *AgentHandler) {
	if agentHandler == nil {
		return
	}
	agentHandler.RegisterPublicRoutes(router)
}

// registerPublicAuthRoutes 封装当前模块的业务处理逻辑。
func registerPublicAuthRoutes(router gin.IRouter, authHandler *AuthHandler) {
	authHandler.RegisterPublicRoutes(router)
}

// registerProtectedAuthRoutes 封装当前模块的业务处理逻辑。
func registerProtectedAuthRoutes(router gin.IRouter, authHandler *AuthHandler) {
	authHandler.RegisterProtectedRoutes(router)
}

// registerUserRoutes 封装当前模块的业务处理逻辑。
func registerUserRoutes(router gin.IRouter, userHandler *UserHandler) {
	userHandler.RegisterRoutes(router)
}

// registerAgentRoutes 封装当前模块的业务处理逻辑。
func registerAgentRoutes(router gin.IRouter, agentHandler *AgentHandler) {
	if agentHandler == nil {
		return
	}
	agentHandler.RegisterRoutes(router)
}

// registerApplicationRoutes 封装当前模块的业务处理逻辑。
func registerApplicationRoutes(router gin.IRouter, applicationHandler *ApplicationHandler) {
	applicationHandler.RegisterRoutes(router)
}

// registerProjectRoutes 封装当前模块的业务处理逻辑。
func registerProjectRoutes(router gin.IRouter, projectHandler *ProjectHandler) {
	if projectHandler == nil {
		return
	}
	projectHandler.RegisterRoutes(router)
}

// registerSystemSettingsRoutes 封装当前模块的业务处理逻辑。
func registerSystemSettingsRoutes(router gin.IRouter, systemSettingsHandler *SystemSettingsHandler) {
	if systemSettingsHandler == nil {
		return
	}
	systemSettingsHandler.RegisterRoutes(router)
}

// registerPipelineRoutes 封装当前模块的业务处理逻辑。
func registerPipelineRoutes(router gin.IRouter, pipelineHandler *PipelineHandler) {
	pipelineHandler.RegisterRoutes(router)
}

// registerArgoCDRoutes 封装当前模块的业务处理逻辑。
func registerArgoCDRoutes(router gin.IRouter, argocdHandler *ArgoCDHandler) {
	if argocdHandler == nil {
		return
	}
	argocdHandler.RegisterRoutes(router)
}

// registerGitOpsRoutes 封装当前模块的业务处理逻辑。
func registerGitOpsRoutes(router gin.IRouter, gitopsHandler *GitOpsHandler) {
	if gitopsHandler == nil {
		return
	}
	gitopsHandler.RegisterRoutes(router)
}

// registerPlatformParamRoutes 封装当前模块的业务处理逻辑。
func registerPlatformParamRoutes(router gin.IRouter, platformParamHandler *PlatformParamHandler) {
	platformParamHandler.RegisterRoutes(router)
}

// registerNotificationRoutes 封装当前模块的业务处理逻辑。
func registerNotificationRoutes(router gin.IRouter, notificationHandler *NotificationHandler) {
	if notificationHandler == nil {
		return
	}
	notificationHandler.RegisterRoutes(router)
}

// registerExecutorParamRoutes 封装当前模块的业务处理逻辑。
func registerExecutorParamRoutes(router gin.IRouter, executorParamHandler *ExecutorParamHandler) {
	executorParamHandler.RegisterRoutes(router)
}

// registerReleaseOrderRoutes 封装当前模块的业务处理逻辑。
func registerReleaseOrderRoutes(router gin.IRouter, releaseOrderHandler *ReleaseOrderHandler) {
	releaseOrderHandler.RegisterRoutes(router)
}

// registerReleaseTemplateRoutes 封装当前模块的业务处理逻辑。
func registerReleaseTemplateRoutes(router gin.IRouter, releaseTemplateHandler *ReleaseTemplateHandler) {
	releaseTemplateHandler.RegisterRoutes(router)
}

// registerAnnouncementRoutes 封装当前模块的业务处理逻辑。
func registerAnnouncementRoutes(router gin.IRouter, announcementHandler *AnnouncementHandler) {
	if announcementHandler == nil {
		return
	}
	announcementHandler.RegisterRoutes(router)
}

// registerSystemRoutes 封装当前模块的业务处理逻辑。
func registerSystemRoutes(router gin.IRouter) {
	router.GET("/healthz", healthz)
	router.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))
}

// healthz 封装当前模块的业务处理逻辑。
func healthz(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// cors 封装当前模块的业务处理逻辑。
func cors() gin.HandlerFunc {
	return func(c *gin.Context) {
		origin := strings.TrimSpace(c.GetHeader("Origin"))
		if origin != "" {
			c.Header("Access-Control-Allow-Origin", origin)
			c.Header("Vary", "Origin")
		}
		c.Header("Access-Control-Allow-Credentials", "true")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}
