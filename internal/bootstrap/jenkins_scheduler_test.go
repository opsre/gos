package bootstrap

import (
	"context"
	"testing"
	"time"
)

func TestStartJenkinsAutoSyncTaskRunsImmediately(t *testing.T) {
	called := make(chan struct{}, 1)
	task := StartJenkinsAutoSyncTask(JenkinsConfig{
		Enabled:             true,
		AutoSyncEnabled:     true,
		AutoSyncIntervalSec: 3600,
	}, func(context.Context) error {
		select {
		case called <- struct{}{}:
		default:
		}
		return nil
	})
	defer task.Stop()

	select {
	case <-called:
	case <-time.After(time.Second):
		t.Fatal("Jenkins auto sync did not run immediately after task startup")
	}
}
