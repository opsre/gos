package bootstrap

import (
	"context"
	"log"
	"time"
)

type ArgoCDSyncTask struct {
	stop func()
	done <-chan struct{}
}

// Stop 封装当前模块的业务处理逻辑。
func (t ArgoCDSyncTask) Stop() {
	if t.stop == nil {
		return
	}
	t.stop()
	if t.done == nil {
		return
	}
	<-t.done
}

// StartArgoCDAutoSyncTask 同步外部或内部状态数据。
func StartArgoCDAutoSyncTask(intervalSec int, run func(context.Context) error) ArgoCDSyncTask {
	done := make(chan struct{})
	if run == nil {
		close(done)
		return ArgoCDSyncTask{stop: func() {}, done: done}
	}
	interval := time.Duration(intervalSec) * time.Second
	if interval <= 0 {
		interval = 300 * time.Second
	}
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		defer close(done)
		log.Printf("argocd auto sync task started, interval=%s", interval)
		execute := func() {
			if err := run(ctx); err != nil {
				log.Printf("argocd auto sync failed: %v", err)
			}
		}
		execute()
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				log.Printf("argocd auto sync task stopped")
				return
			case <-ticker.C:
				execute()
			}
		}
	}()
	return ArgoCDSyncTask{stop: cancel, done: done}
}
