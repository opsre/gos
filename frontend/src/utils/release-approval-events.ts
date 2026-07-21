export const releaseApprovalTasksChangedEvent = 'gos:release-approval-tasks-changed'

export function notifyReleaseApprovalTasksChanged() {
  if (typeof window === 'undefined') return
  window.dispatchEvent(new Event(releaseApprovalTasksChangedEvent))
}
