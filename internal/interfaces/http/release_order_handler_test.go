package httpapi

import (
	"fmt"
	"testing"

	"gos/internal/application/usecase"
)

func TestNormalizeReleaseOrderErrorMessageStripsInvalidInputPrefix(t *testing.T) {
	err := fmt.Errorf("%w: 重放单不支持再次重放，继续重发请从原始单发起", usecase.ErrInvalidInput)

	got := normalizeReleaseOrderErrorMessage(err)
	want := "重放单不支持再次重放，继续重发请从原始单发起"

	if got != want {
		t.Fatalf("message = %q, want %q", got, want)
	}
}
