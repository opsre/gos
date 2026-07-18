package httpapi

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"gos/internal/application/usecase"
)

func TestApplicationWriteHTTPErrorMapsReferencedConflictToConflict(t *testing.T) {
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)

	writeHTTPError(c, fmt.Errorf("%w: application key cannot be changed", usecase.ErrReferencedConflict))

	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusConflict)
	}
}
