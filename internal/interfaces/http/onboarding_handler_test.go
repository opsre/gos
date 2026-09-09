package httpapi

import (
	"context"
	"database/sql"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"gos/internal/application/usecase"
	ob "gos/internal/domain/onboarding"
	usr "gos/internal/domain/user"
	"gos/internal/infrastructure/persistence/sqlrepo"
	_ "modernc.org/sqlite"
)

type onboardingAuthFake struct{ denied string }

func (a onboardingAuthFake) HasPermission(_ context.Context, _ usr.User, code, _, _ string) (bool, error) {
	return code != a.denied, nil
}
func (a onboardingAuthFake) ListEffectivePermissions(context.Context, usr.User) ([]usr.UserPermission, error) {
	return nil, nil
}

func TestOnboardingHTTPPermissionAndSessionOwnership(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	repo := sqlrepo.NewOnboardingRepository(db, "sqlite")
	if err := repo.InitSchema(context.Background()); err != nil {
		t.Fatal(err)
	}
	session := ob.Session{ID: "session", OwnerUserID: "owner", Version: 1, UpdatedAt: time.Now(), Status: "draft", Refs: ob.References{ApplicationID: "app", TemplateID: "template"}}
	if err := repo.Create(context.Background(), session); err != nil {
		t.Fatal(err)
	}
	manager := usecase.NewOnboardingManager(repo, usecase.OnboardingDependencies{})
	for _, tc := range []struct {
		name, user, role, denied, method, path, body string
		want                                         int
	}{
		{name: "own session", user: "owner", method: "GET", path: "/onboarding/sessions/session", want: 200},
		{name: "other owner", user: "other", method: "GET", path: "/onboarding/sessions/session", want: 403},
		{name: "administrator", user: "other", role: "admin", method: "GET", path: "/onboarding/sessions/session", want: 200},
		{name: "missing app management", user: "owner", denied: "application.manage", method: "GET", path: "/onboarding/sessions/session", want: 403},
		{name: "unauthenticated", method: "GET", path: "/onboarding/sessions/session", want: 401},
		{name: "field write permission", user: "owner", denied: "platform_param.manage", method: "POST", path: "/onboarding/sessions/session/steps/parameters/apply", body: `{"expected_version":1,"request_key":"key"}`, want: 403},
		{name: "binding write permission", user: "owner", denied: "pipeline.manage", method: "POST", path: "/onboarding/sessions/session/steps/pipelines/apply", body: `{"expected_version":1,"request_key":"key"}`, want: 403},
		{name: "template write permission", user: "owner", denied: "release.template.manage", method: "POST", path: "/onboarding/sessions/session/steps/template_flow/apply", body: `{"expected_version":1,"request_key":"key"}`, want: 403},
		{name: "first order permission", user: "owner", denied: "release.create", method: "POST", path: "/onboarding/sessions/session/first-release", body: `{"expected_version":1,"request_key":"key","order":{"application_id":"app","template_id":"template","env_code":"dev"}}`, want: 403},
	} {
		t.Run(tc.name, func(t *testing.T) {
			router := gin.New()
			router.Use(func(c *gin.Context) {
				if tc.user != "" {
					setCurrentUser(c, usr.User{ID: tc.user, Role: usr.Role(tc.role)})
				}
				c.Next()
			})
			NewOnboardingHandler(manager, onboardingAuthFake{tc.denied}).RegisterRoutes(router)
			req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)
			if w.Code != tc.want {
				t.Fatalf("status=%d want %d: %s", w.Code, tc.want, w.Body.String())
			}
		})
	}
	current, err := repo.Get(context.Background(), session.ID)
	if err != nil || current.Version != 1 {
		t.Fatal("denied requests mutated session")
	}
}
