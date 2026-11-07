package agent

import (
	"strings"
	"time"
)

const DefaultOfflineAfter = 45 * time.Second

func ResolveRuntimeState(item Instance, now time.Time, offlineAfter time.Duration) RuntimeState {
	switch item.Status {
	case StatusDisabled:
		return RuntimeStateDisabled
	case StatusMaintenance:
		return RuntimeStateMaintenance
	}
	if item.LastHeartbeatAt.IsZero() || now.Sub(item.LastHeartbeatAt) > offlineAfter {
		return RuntimeStateOffline
	}
	if strings.TrimSpace(item.CurrentTaskID) != "" {
		return RuntimeStateBusy
	}
	return RuntimeStateOnline
}
