package bootstrap

import (
	"context"
	"log"
	"time"
)

type JenkinsSyncTask struct {
	stop func()
	done <-chan struct{}
}

// Stop 封装当前模块的业务处理逻辑。
func (t JenkinsSyncTask) Stop() {
	if t.stop == nil {
		return
	}
	t.stop()
	if t.done == nil {
		return
	}
	<-t.done
}

// StartJenkinsAutoSyncTask 同步外部或内部状态数据。
func StartJenkinsAutoSyncTask(cfg JenkinsConfig, run func(context.Context) error) JenkinsSyncTask {
	return startJenkinsTask(
		cfg.Enabled && cfg.AutoSyncEnabled,
		time.Duration(cfg.AutoSyncIntervalSec)*time.Second,
		300*time.Second,
		"jenkins auto sync",
		run,
	)
}

// StartJenkinsReleaseTrackTask 封装当前模块的业务处理逻辑。
func StartJenkinsReleaseTrackTask(cfg JenkinsConfig, run func(context.Context) error) JenkinsSyncTask {
	return startJenkinsTask(
		cfg.Enabled && cfg.ReleaseTrackEnabled,
		time.Duration(cfg.ReleaseTrackIntervalSec)*time.Second,
		10*time.Second,
		"jenkins release track",
		run,
	)
}

// StartReleaseTrackTask 封装当前模块的业务处理逻辑。
func StartReleaseTrackTask(intervalSec int, run func(context.Context) error) JenkinsSyncTask {
	return startJenkinsTask(
		true,
		time.Duration(intervalSec)*time.Second,
		10*time.Second,
		"release track",
		run,
	)
}

// startJenkinsTask 封装当前模块的业务处理逻辑。
func startJenkinsTask(
	enabled bool,
	interval time.Duration,
	defaultInterval time.Duration,
	taskName string,
	run func(context.Context) error,
) JenkinsSyncTask {
	done := make(chan struct{})
	if !enabled || run == nil {
		close(done)
		return JenkinsSyncTask{
			stop: func() {},
			done: done,
		}
	}

	if interval <= 0 {
		interval = defaultInterval
	}

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		defer close(done)
		log.Printf("%s task started, interval=%s", taskName, interval)

		execute := func() {
			if err := run(ctx); err != nil {
				log.Printf("%s failed: %v", taskName, err)
			}
		}

		execute()

		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				log.Printf("%s task stopped", taskName)
				return
			case <-ticker.C:
				execute()
			}
		}
	}()

	return JenkinsSyncTask{
		stop: cancel,
		done: done,
	}
}
