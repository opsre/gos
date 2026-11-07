package httpapi

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestMetricsHandlerUsesIndependentBearerAuthentication(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("gos_metrics_collection_success 1\n"))
	})
	for _, tc := range []struct {
		name, method, auth string
		enabled            bool
		want               int
	}{
		{"missing", "GET", "", true, 401},
		{"wrong", "GET", "Bearer wrong", true, 401},
		{"browser session is insufficient", "GET", "Basic user:password", true, 401},
		{"correct", "GET", "Bearer metrics-test-token", true, 200},
		{"head", "HEAD", "Bearer metrics-test-token", true, 200},
		{"read only", "POST", "Bearer metrics-test-token", true, 405},
		{"disabled", "GET", "Bearer metrics-test-token", false, 404},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, "/metrics", nil)
			req.Header.Set("Authorization", tc.auth)
			req.AddCookie(&http.Cookie{Name: "gos_session", Value: "browser-session"})
			rec := httptest.NewRecorder()
			NewMetricsHandler(tc.enabled, "metrics-test-token", next).ServeHTTP(rec, req)
			if rec.Code != tc.want {
				t.Fatalf("status=%d, want %d", rec.Code, tc.want)
			}
		})
	}
}

func TestMetricsHandlerAppliesAuthSwitchWithoutRestart(t *testing.T) {
	var required atomic.Bool
	required.Store(true)
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("metrics 1\n")) })
	handler := NewMetricsHandler(true, "configured-token", next, required.Load)
	check := func(auth string, want int) {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
		req.Header.Set("Authorization", auth)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != want {
			t.Fatalf("status=%d want=%d", rec.Code, want)
		}
	}
	check("", 401)
	check("Bearer configured-token", 200)
	required.Store(false)
	check("", 200)
	check("Bearer wrong-token", 200)
	required.Store(true)
	check("", 401)
	check("Bearer wrong-token", 401)
	check("Bearer configured-token", 200)
}

func TestMetricsHandlerMissingTokenRemainsClosedUntilAuthIsDisabled(t *testing.T) {
	var required atomic.Bool
	required.Store(true)
	handler := NewMetricsHandler(true, "", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) }), required.Load)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest("GET", "/metrics", nil))
	if rec.Code != 503 {
		t.Fatalf("unconfigured protected metrics status=%d", rec.Code)
	}
	required.Store(false)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest("GET", "/metrics", nil))
	if rec.Code != 200 {
		t.Fatalf("anonymous metrics status=%d", rec.Code)
	}
}
