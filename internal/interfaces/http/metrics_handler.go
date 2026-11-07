package httpapi

import (
	"crypto/sha256"
	"crypto/subtle"
	"net/http"
	"strings"
)

// NewMetricsHandler is independent of browser sessions. The optional token is a
// deployment-wide read credential because the endpoint contains global metrics.
func NewMetricsHandler(enabled bool, token string, next http.Handler, authRequired ...func() bool) http.Handler {
	expected := sha256.Sum256([]byte(token))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !enabled || next == nil {
			http.NotFound(w, r)
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		requireAuth := token != ""
		if len(authRequired) > 0 && authRequired[0] != nil {
			requireAuth = authRequired[0]()
		}
		if requireAuth {
			if token == "" {
				http.Error(w, "metrics authentication token is not configured", http.StatusServiceUnavailable)
				return
			}
			parts := strings.Fields(r.Header.Get("Authorization"))
			valid := false
			if len(parts) == 2 && strings.EqualFold(parts[0], "Bearer") {
				actual := sha256.Sum256([]byte(parts[1]))
				valid = subtle.ConstantTimeCompare(actual[:], expected[:]) == 1
			}
			if !valid {
				w.Header().Set("WWW-Authenticate", `Bearer realm="metrics"`)
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
		}
		w.Header().Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)
	})
}
