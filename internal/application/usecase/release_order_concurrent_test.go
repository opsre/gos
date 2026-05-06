package usecase

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestBatchExecuteRequiresBatchName(t *testing.T) {
	t.Parallel()

	manager := &ReleaseOrderManager{}
	_, err := manager.BatchExecute(context.Background(), BatchExecuteReleaseOrdersInput{
		OrderIDs: []string{"order-a", "order-b"},
	})
	if err == nil {
		t.Fatal("BatchExecute returned nil error, want invalid input")
	}
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("BatchExecute error = %v, want ErrInvalidInput", err)
	}
	if !strings.Contains(err.Error(), "并发名称") {
		t.Fatalf("BatchExecute error = %q, want mention 并发名称", err.Error())
	}
}
