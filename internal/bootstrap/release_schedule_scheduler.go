package bootstrap

import (
	"context"
	"time"
)

// StartReleaseScheduleTask starts the scheduled release dispatcher.
func StartReleaseScheduleTask(intervalSec int, run func(context.Context) error) JenkinsSyncTask {
	return startJenkinsTask(
		true,
		time.Duration(intervalSec)*time.Second,
		10*time.Second,
		"release schedule",
		run,
	)
}
