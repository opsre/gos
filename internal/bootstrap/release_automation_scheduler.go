package bootstrap

import (
	"context"
	"time"
)

// StartReleaseAutomationTask starts the release automation poller: it walks the
// enabled automation configs, reads each branch HEAD and creates + dispatches a
// release order when the branch moved.
//
// It runs on every replica like the other schedulers (release schedule, release
// track); the per-config conditional update in the repository is what keeps two
// replicas from advancing the same baseline, so no leader election is needed.
func StartReleaseAutomationTask(intervalSec int, run func(context.Context) error) JenkinsSyncTask {
	return startJenkinsTask(
		true,
		time.Duration(intervalSec)*time.Second,
		30*time.Second,
		"release automation",
		run,
	)
}
