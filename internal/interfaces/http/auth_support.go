package httpapi

import (
	"context"
	"net/http"
	"sort"
	"strings"

	"github.com/gin-gonic/gin"

	releasedomain "gos/internal/domain/release"
	userdomain "gos/internal/domain/user"
)

const currentUserContextKey = "current_user"
const applicationEnvScopeSeparator = "::"

type RequestAuthorizer interface {
	HasPermission(ctx context.Context, user userdomain.User, permissionCode string, scopeType string, scopeValue string) (bool, error)
	ListEffectivePermissions(ctx context.Context, user userdomain.User) ([]userdomain.UserPermission, error)
}

// setCurrentUser 封装当前模块的业务处理逻辑。
func setCurrentUser(c *gin.Context, user userdomain.User) {
	c.Set(currentUserContextKey, user)
}

// getCurrentUser 查询并返回指定资源数据。
func getCurrentUser(c *gin.Context) (userdomain.User, bool) {
	value, exists := c.Get(currentUserContextKey)
	if !exists {
		return userdomain.User{}, false
	}
	user, ok := value.(userdomain.User)
	if !ok {
		return userdomain.User{}, false
	}
	return user, true
}

// extractBearerToken 封装当前模块的业务处理逻辑。
func extractBearerToken(value string) string {
	text := strings.TrimSpace(value)
	if text == "" {
		return ""
	}
	const prefix = "Bearer "
	if len(text) < len(prefix) || !strings.EqualFold(text[:len(prefix)], prefix) {
		return ""
	}
	return strings.TrimSpace(text[len(prefix):])
}

// ensurePermission 校验前置条件，不满足时写入对应错误响应。
func ensurePermission(
	c *gin.Context,
	authz RequestAuthorizer,
	permissionCode string,
	scopeType string,
	scopeValue string,
) bool {
	return ensurePermissionWithMessage(c, authz, permissionCode, scopeType, scopeValue, "")
}

// ensurePermissionWithMessage 校验前置条件，不满足时写入对应错误响应。
func ensurePermissionWithMessage(
	c *gin.Context,
	authz RequestAuthorizer,
	permissionCode string,
	scopeType string,
	scopeValue string,
	forbiddenMessage string,
) bool {
	user, ok := getCurrentUser(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return false
	}
	if authz == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "authorizer is not configured"})
		return false
	}
	allowed, err := authz.HasPermission(c.Request.Context(), user, permissionCode, scopeType, scopeValue)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return false
	}
	if !allowed {
		message := strings.TrimSpace(forbiddenMessage)
		if message == "" {
			message = "forbidden: permission denied"
		}
		c.JSON(http.StatusForbidden, gin.H{"error": message})
		return false
	}
	return true
}

// ensureAnyPermission 校验前置条件，不满足时写入对应错误响应。
func ensureAnyPermission(
	c *gin.Context,
	authz RequestAuthorizer,
	permissionCodes ...string,
) bool {
	user, ok := getCurrentUser(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return false
	}
	if authz == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "authorizer is not configured"})
		return false
	}
	for _, code := range permissionCodes {
		allowed, err := authz.HasPermission(c.Request.Context(), user, strings.TrimSpace(code), "", "")
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
			return false
		}
		if allowed {
			return true
		}
	}
	c.JSON(http.StatusForbidden, gin.H{"error": "forbidden: permission denied"})
	return false
}

// ensureAnyApplicationPermission 校验前置条件，不满足时写入对应错误响应。
func ensureAnyApplicationPermission(
	c *gin.Context,
	authz RequestAuthorizer,
	applicationID string,
	permissionCodes ...string,
) bool {
	user, ok := getCurrentUser(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return false
	}
	if authz == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "authorizer is not configured"})
		return false
	}
	appID := strings.TrimSpace(applicationID)
	for _, code := range permissionCodes {
		allowed, err := authz.HasPermission(c.Request.Context(), user, strings.TrimSpace(code), "application", appID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
			return false
		}
		if allowed {
			return true
		}
	}
	c.JSON(http.StatusForbidden, gin.H{"error": "forbidden: permission denied"})
	return false
}

// buildApplicationEnvScopeValue 组装业务执行所需的输入数据。
func buildApplicationEnvScopeValue(applicationID string, envCode string) string {
	appID := strings.TrimSpace(applicationID)
	env := strings.TrimSpace(envCode)
	if appID == "" || env == "" {
		return ""
	}
	return appID + applicationEnvScopeSeparator + env
}

// parseApplicationEnvScopeValue 解析输入内容并返回结构化结果。
func parseApplicationEnvScopeValue(value string) (applicationID string, envCode string, ok bool) {
	text := strings.TrimSpace(value)
	if text == "" {
		return "", "", false
	}
	parts := strings.SplitN(text, applicationEnvScopeSeparator, 2)
	if len(parts) != 2 {
		return "", "", false
	}
	applicationID = strings.TrimSpace(parts[0])
	envCode = strings.TrimSpace(parts[1])
	if applicationID == "" || envCode == "" {
		return "", "", false
	}
	return applicationID, envCode, true
}

// collectApplicationScopesFromPermissions 封装当前模块的业务处理逻辑。
func collectApplicationScopesFromPermissions(
	items []userdomain.UserPermission,
	acceptedCodes map[string]struct{},
) ([]string, []releasedomain.ApplicationEnvScope) {
	appSeen := make(map[string]struct{})
	scopeSeen := make(map[string]struct{})
	applicationIDs := make([]string, 0)
	envScopes := make([]releasedomain.ApplicationEnvScope, 0)

	for _, item := range items {
		if !item.Enabled {
			continue
		}
		code := strings.ToLower(strings.TrimSpace(item.PermissionCode))
		if len(acceptedCodes) > 0 {
			if _, exists := acceptedCodes[code]; !exists {
				continue
			}
		}
		switch strings.ToLower(strings.TrimSpace(item.ScopeType)) {
		case "application":
			applicationID := strings.TrimSpace(item.ScopeValue)
			if applicationID == "" {
				continue
			}
			if _, exists := appSeen[applicationID]; exists {
				continue
			}
			appSeen[applicationID] = struct{}{}
			applicationIDs = append(applicationIDs, applicationID)
		case "application_env":
			applicationID, envCode, ok := parseApplicationEnvScopeValue(item.ScopeValue)
			if !ok {
				continue
			}
			scopeKey := applicationID + applicationEnvScopeSeparator + envCode
			if _, exists := scopeSeen[scopeKey]; !exists {
				scopeSeen[scopeKey] = struct{}{}
				envScopes = append(envScopes, releasedomain.ApplicationEnvScope{
					ApplicationID: applicationID,
					EnvCode:       envCode,
				})
			}
		}
	}

	sort.Strings(applicationIDs)
	sort.Slice(envScopes, func(i, j int) bool {
		if envScopes[i].ApplicationID == envScopes[j].ApplicationID {
			return envScopes[i].EnvCode < envScopes[j].EnvCode
		}
		return envScopes[i].ApplicationID < envScopes[j].ApplicationID
	})
	return applicationIDs, envScopes
}
