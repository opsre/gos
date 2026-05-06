package httpapi

import (
	"context"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	userdomain "gos/internal/domain/user"
)

type SessionUserResolver interface {
	ResolveUserByToken(ctx context.Context, token string) (userdomain.User, userdomain.UserSession, error)
}

// authMiddleware 封装当前模块的业务处理逻辑。
func authMiddleware(resolver SessionUserResolver) gin.HandlerFunc {
	return func(c *gin.Context) {
		if isPublicPath(c.Request.URL.Path) {
			c.Next()
			return
		}
		if resolver == nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "auth resolver is not configured"})
			c.Abort()
			return
		}

		token := extractBearerToken(c.GetHeader("Authorization"))
		if token == "" {
			// EventSource cannot set custom Authorization header in browser.
			// Fallback to query token for SSE endpoints.
			token = strings.TrimSpace(c.Query("access_token"))
		}
		if token == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			c.Abort()
			return
		}

		user, _, err := resolver.ResolveUserByToken(c.Request.Context(), token)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			c.Abort()
			return
		}

		setCurrentUser(c, user)
		c.Next()
	}
}

// isPublicPath 封装当前模块的业务处理逻辑。
func isPublicPath(path string) bool {
	clean := strings.TrimSpace(path)
	if clean == "" {
		return false
	}
	switch clean {
	case "/healthz", "/auth/login":
		return true
	default:
		return strings.HasPrefix(clean, "/swagger/")
	}
}
