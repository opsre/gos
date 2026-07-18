package httpapi

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"gos/internal/application/usecase"
	domain "gos/internal/domain/release"
	"gos/internal/support/logx"
)

const (
	releaseOrderRealtimeInterval          = 2 * time.Second
	releaseOrderRealtimeIdleGrace         = 30 * time.Second
	releaseOrderRealtimeHeartbeatInterval = 15 * time.Second
	releaseOrderRealtimeWriteTimeout      = 10 * time.Second
	releaseOrderRealtimeMaxLoadFailures   = 3
)

// ReleaseOrderRealtimeSnapshotPayload 是版本摘要覆盖的纯业务数据。
// Version 和 GeneratedAt 位于外层，避免时间字段导致无业务变化时仍持续广播。
type ReleaseOrderRealtimeSnapshotPayload struct {
	Order                   ReleaseOrderResponse                               `json:"order"`
	Executions              []ReleaseOrderExecutionResponse                    `json:"executions"`
	Steps                   []ReleaseOrderStepResponse                         `json:"steps"`
	ValueProgress           []ReleaseOrderValueProgressResponse                `json:"value_progress"`
	PipelineStageView       ReleaseOrderPipelineStageListResponse              `json:"pipeline_stage_view"`
	ArtifactMetadata        []ReleaseOrderArtifactMetadataResponse             `json:"artifact_metadata"`
	ApprovalRecords         []ReleaseOrderApprovalRecordResponse               `json:"approval_records"`
	ConcurrentBatchProgress *usecase.ReleaseOrderConcurrentBatchProgressOutput `json:"concurrent_batch_progress"`
}

type releaseOrderRealtimeSnapshot struct {
	Version     string
	GeneratedAt time.Time
	Payload     ReleaseOrderRealtimeSnapshotPayload
}

// ReleaseOrderRealtimeSnapshotData 是 GET 与 SSE snapshot 事件共享的数据结构。
type ReleaseOrderRealtimeSnapshotData struct {
	Version              string    `json:"version"`
	GeneratedAt          time.Time `json:"generated_at"`
	ValueProgressVisible bool      `json:"value_progress_visible"`
	ReleaseOrderRealtimeSnapshotPayload
}

type ReleaseOrderRealtimeSnapshotResponse struct {
	Data ReleaseOrderRealtimeSnapshotData `json:"data"`
}

type releaseOrderRealtimeSynchronizer interface {
	SyncOrder(ctx context.Context, orderID string) error
}

type releaseOrderRealtimeSnapshotLoader func(
	ctx context.Context,
	orderID string,
) (ReleaseOrderRealtimeSnapshotPayload, error)

type releaseOrderRealtimeCoordinator struct {
	loader       releaseOrderRealtimeSnapshotLoader
	synchronizer releaseOrderRealtimeSynchronizer
	interval     time.Duration
	idleGrace    time.Duration
	heartbeat    time.Duration
	now          func() time.Time

	mu               sync.Mutex
	watchers         map[string]*releaseOrderRealtimeWatcher
	nextSubscriberID uint64
}

type releaseOrderRealtimeWatcher struct {
	refreshMu    sync.Mutex
	orderID      string
	ctx          context.Context
	cancel       context.CancelFunc
	ready        chan struct{}
	initialErr   error
	latest       releaseOrderRealtimeSnapshot
	hasLatest    bool
	subscribers  map[uint64]chan releaseOrderRealtimeSnapshot
	idleTimer    *time.Timer
	idleVersion  uint64
	loadFailures int
}

func newReleaseOrderRealtimeCoordinator(
	loader releaseOrderRealtimeSnapshotLoader,
	synchronizer releaseOrderRealtimeSynchronizer,
) *releaseOrderRealtimeCoordinator {
	return &releaseOrderRealtimeCoordinator{
		loader:       loader,
		synchronizer: synchronizer,
		interval:     releaseOrderRealtimeInterval,
		idleGrace:    releaseOrderRealtimeIdleGrace,
		heartbeat:    releaseOrderRealtimeHeartbeatInterval,
		now: func() time.Time {
			return time.Now().UTC()
		},
		watchers: make(map[string]*releaseOrderRealtimeWatcher),
	}
}

func (c *releaseOrderRealtimeCoordinator) setSynchronizer(syncer releaseOrderRealtimeSynchronizer) {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.synchronizer = syncer
	c.mu.Unlock()
}

func (c *releaseOrderRealtimeCoordinator) snapshot(
	ctx context.Context,
	orderID string,
) (releaseOrderRealtimeSnapshot, error) {
	if c == nil || c.loader == nil {
		return releaseOrderRealtimeSnapshot{}, errors.New("release order realtime snapshot is not configured")
	}
	orderID = strings.TrimSpace(orderID)
	if orderID == "" {
		return releaseOrderRealtimeSnapshot{}, usecase.ErrInvalidID
	}

	c.mu.Lock()
	watcher := c.watchers[orderID]
	var ready <-chan struct{}
	if watcher != nil {
		ready = watcher.ready
	}
	c.mu.Unlock()
	if watcher != nil {
		select {
		case <-ctx.Done():
			return releaseOrderRealtimeSnapshot{}, ctx.Err()
		case <-ready:
		}
		c.mu.Lock()
		active := c.watchers[orderID] == watcher && watcher.initialErr == nil
		initialErr := watcher.initialErr
		c.mu.Unlock()
		if initialErr != nil {
			return releaseOrderRealtimeSnapshot{}, initialErr
		}
		if active {
			// A manual snapshot must never return the watcher's cached value: action handlers
			// use this endpoint immediately after a mutation and the old cache could overwrite it.
			return c.refreshWatcher(ctx, watcher, false, false)
		}
	}
	// Pure HTTP fallback has no fast watcher. Best-effort synchronize this order once so a
	// five-second fallback poll is not limited by the normal ten-second background tracker.
	c.mu.Lock()
	syncer := c.synchronizer
	c.mu.Unlock()
	if syncer != nil {
		if err := syncer.SyncOrder(ctx, orderID); err != nil && !errors.Is(err, context.Canceled) {
			logx.Warn("release_realtime", "fallback_sync_order_failed",
				logx.F("order_id", orderID),
				logx.F("error", err.Error()),
			)
		}
	}
	return c.loadSnapshot(ctx, orderID)
}

func (c *releaseOrderRealtimeCoordinator) subscribe(
	ctx context.Context,
	orderID string,
) (<-chan releaseOrderRealtimeSnapshot, func(), error) {
	if c == nil || c.loader == nil {
		return nil, nil, errors.New("release order realtime snapshot is not configured")
	}
	orderID = strings.TrimSpace(orderID)
	if orderID == "" {
		return nil, nil, usecase.ErrInvalidID
	}

	c.mu.Lock()
	watcher := c.watchers[orderID]
	if watcher == nil {
		watcherCtx, cancel := context.WithCancel(context.Background())
		watcher = &releaseOrderRealtimeWatcher{
			orderID:     orderID,
			ctx:         watcherCtx,
			cancel:      cancel,
			ready:       make(chan struct{}),
			subscribers: make(map[uint64]chan releaseOrderRealtimeSnapshot),
		}
		c.watchers[orderID] = watcher
		go c.initializeWatcher(watcher)
	}
	ready := watcher.ready
	c.mu.Unlock()

	select {
	case <-ctx.Done():
		return nil, nil, ctx.Err()
	case <-ready:
	}

	c.mu.Lock()
	if c.watchers[orderID] != watcher {
		err := watcher.initialErr
		c.mu.Unlock()
		if err == nil {
			err = errors.New("release order realtime watcher stopped")
		}
		return nil, nil, err
	}
	if watcher.initialErr != nil {
		err := watcher.initialErr
		c.mu.Unlock()
		return nil, nil, err
	}
	if watcher.idleTimer != nil {
		watcher.idleTimer.Stop()
		watcher.idleTimer = nil
	}
	watcher.idleVersion++
	c.nextSubscriberID++
	subscriberID := c.nextSubscriberID
	events := make(chan releaseOrderRealtimeSnapshot, 1)
	watcher.subscribers[subscriberID] = events
	if watcher.hasLatest {
		events <- watcher.latest
	}
	c.mu.Unlock()

	var once sync.Once
	unsubscribe := func() {
		once.Do(func() {
			c.unsubscribe(orderID, watcher, subscriberID)
		})
	}
	return events, unsubscribe, nil
}

func (c *releaseOrderRealtimeCoordinator) initializeWatcher(watcher *releaseOrderRealtimeWatcher) {
	snapshot, err := c.loadSnapshot(watcher.ctx, watcher.orderID)

	c.mu.Lock()
	if c.watchers[watcher.orderID] != watcher {
		c.mu.Unlock()
		return
	}
	watcher.initialErr = err
	if err == nil {
		watcher.latest = snapshot
		watcher.hasLatest = true
	} else {
		delete(c.watchers, watcher.orderID)
		watcher.cancel()
	}
	close(watcher.ready)
	if err == nil && len(watcher.subscribers) == 0 {
		c.scheduleIdleStopLocked(watcher)
	}
	c.mu.Unlock()

	if err != nil {
		return
	}
	go c.runWatcher(watcher)
}

func (c *releaseOrderRealtimeCoordinator) runWatcher(watcher *releaseOrderRealtimeWatcher) {
	interval := c.interval
	if interval <= 0 {
		interval = releaseOrderRealtimeInterval
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-watcher.ctx.Done():
			return
		case <-ticker.C:
			_, _ = c.refreshWatcher(watcher.ctx, watcher, true, true)
		}
	}
}

func (c *releaseOrderRealtimeCoordinator) refreshWatcher(
	ctx context.Context,
	watcher *releaseOrderRealtimeWatcher,
	synchronize bool,
	countFailure bool,
) (releaseOrderRealtimeSnapshot, error) {
	watcher.refreshMu.Lock()
	defer watcher.refreshMu.Unlock()

	if synchronize {
		c.mu.Lock()
		syncer := c.synchronizer
		c.mu.Unlock()
		if syncer != nil {
			if err := syncer.SyncOrder(ctx, watcher.orderID); err != nil && !errors.Is(err, context.Canceled) {
				logx.Warn("release_realtime", "sync_order_failed",
					logx.F("order_id", watcher.orderID),
					logx.F("error", err.Error()),
				)
			}
		}
	}

	snapshot, err := c.loadSnapshot(ctx, watcher.orderID)
	if err != nil {
		if !errors.Is(err, context.Canceled) {
			logx.Warn("release_realtime", "load_snapshot_failed",
				logx.F("order_id", watcher.orderID),
				logx.F("error", err.Error()),
			)
		}
		c.mu.Lock()
		if countFailure && c.watchers[watcher.orderID] == watcher && watcher.ctx.Err() == nil {
			watcher.loadFailures++
			if watcher.loadFailures >= releaseOrderRealtimeMaxLoadFailures {
				logx.Warn("release_realtime", "watcher_stopped_after_load_failures",
					logx.F("order_id", watcher.orderID),
					logx.F("consecutive_failures", watcher.loadFailures),
				)
				c.stopWatcherLocked(watcher)
			}
		}
		c.mu.Unlock()
		return releaseOrderRealtimeSnapshot{}, err
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	if c.watchers[watcher.orderID] != watcher || watcher.ctx.Err() != nil {
		return snapshot, nil
	}
	watcher.loadFailures = 0
	changed := !watcher.hasLatest || watcher.latest.Version != snapshot.Version
	watcher.latest = snapshot
	watcher.hasLatest = true
	if !changed {
		return snapshot, nil
	}
	for _, events := range watcher.subscribers {
		select {
		case events <- snapshot:
		default:
			// 慢连接只保留最新快照，避免反压阻塞同一发布单的其他订阅者。
			select {
			case <-events:
			default:
			}
			select {
			case events <- snapshot:
			default:
			}
		}
	}
	return snapshot, nil
}

func (c *releaseOrderRealtimeCoordinator) loadSnapshot(
	ctx context.Context,
	orderID string,
) (releaseOrderRealtimeSnapshot, error) {
	payload, err := c.loader(ctx, orderID)
	if err != nil {
		return releaseOrderRealtimeSnapshot{}, err
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return releaseOrderRealtimeSnapshot{}, err
	}
	digest := sha256.Sum256(encoded)
	return releaseOrderRealtimeSnapshot{
		Version:     hex.EncodeToString(digest[:]),
		GeneratedAt: c.now().UTC(),
		Payload:     payload,
	}, nil
}

func (c *releaseOrderRealtimeCoordinator) unsubscribe(
	orderID string,
	watcher *releaseOrderRealtimeWatcher,
	subscriberID uint64,
) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.watchers[orderID] != watcher {
		return
	}
	delete(watcher.subscribers, subscriberID)
	if len(watcher.subscribers) == 0 {
		c.scheduleIdleStopLocked(watcher)
	}
}

func (c *releaseOrderRealtimeCoordinator) scheduleIdleStopLocked(watcher *releaseOrderRealtimeWatcher) {
	if watcher.idleTimer != nil {
		watcher.idleTimer.Stop()
	}
	watcher.idleVersion++
	version := watcher.idleVersion
	grace := c.idleGrace
	if grace <= 0 {
		grace = releaseOrderRealtimeIdleGrace
	}
	watcher.idleTimer = time.AfterFunc(grace, func() {
		c.mu.Lock()
		defer c.mu.Unlock()
		if c.watchers[watcher.orderID] != watcher || len(watcher.subscribers) != 0 || watcher.idleVersion != version {
			return
		}
		c.stopWatcherLocked(watcher)
	})
}

func (c *releaseOrderRealtimeCoordinator) stopWatcherLocked(watcher *releaseOrderRealtimeWatcher) {
	if watcher == nil || c.watchers[watcher.orderID] != watcher {
		return
	}
	delete(c.watchers, watcher.orderID)
	if watcher.idleTimer != nil {
		watcher.idleTimer.Stop()
		watcher.idleTimer = nil
	}
	watcher.cancel()
	for subscriberID, events := range watcher.subscribers {
		close(events)
		delete(watcher.subscribers, subscriberID)
	}
}

// SetRealtimeSynchronizer 在服务启动时接入单发布单快速追踪器。
func (h *ReleaseOrderHandler) SetRealtimeSynchronizer(syncer releaseOrderRealtimeSynchronizer) {
	if h == nil {
		return
	}
	if h.realtime == nil {
		h.realtime = newReleaseOrderRealtimeCoordinator(h.loadReleaseOrderRealtimeSnapshot, syncer)
		return
	}
	h.realtime.setSynchronizer(syncer)
}

// GetRealtimeSnapshot 返回一次轻量动态快照。
func (h *ReleaseOrderHandler) GetRealtimeSnapshot(c *gin.Context) {
	if _, ok := h.authorizeRealtimeOrder(c); !ok {
		return
	}
	snapshot, err := h.realtime.snapshot(c.Request.Context(), c.Param("id"))
	if err != nil {
		writeReleaseOrderHTTPError(c, err)
		return
	}
	data, err := h.realtimeSnapshotForCurrentUser(c, snapshot)
	if err != nil {
		writeReleaseOrderHTTPError(c, err)
		return
	}
	c.JSON(http.StatusOK, ReleaseOrderRealtimeSnapshotResponse{Data: data})
}

// StreamRealtimeEvents 以 SSE 推送发布单动态快照；同一进程内同一 order 共用 watcher。
func (h *ReleaseOrderHandler) StreamRealtimeEvents(c *gin.Context) {
	if _, ok := h.authorizeRealtimeOrder(c); !ok {
		return
	}
	_, ok := c.Writer.(http.Flusher)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "streaming is not supported"})
		return
	}
	events, unsubscribe, err := h.realtime.subscribe(c.Request.Context(), c.Param("id"))
	if err != nil {
		writeReleaseOrderHTTPError(c, err)
		return
	}
	defer unsubscribe()

	// SSE 是长连接，先清除 http.Server 的整段响应写超时；每帧写入时再设置滚动 deadline。
	_ = http.NewResponseController(c.Writer).SetWriteDeadline(time.Time{})
	c.Header("Content-Type", "text/event-stream; charset=utf-8")
	c.Header("Cache-Control", "no-cache, no-transform")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")
	c.Status(http.StatusOK)

	heartbeatInterval := h.realtime.heartbeat
	if heartbeatInterval <= 0 {
		heartbeatInterval = releaseOrderRealtimeHeartbeatInterval
	}
	heartbeat := time.NewTicker(heartbeatInterval)
	defer heartbeat.Stop()
	var (
		latestSnapshot     releaseOrderRealtimeSnapshot
		hasLatestSnapshot  bool
		lastVisibleVersion string
	)
	emitVisibleSnapshot := func(snapshot releaseOrderRealtimeSnapshot) error {
		if !h.canCurrentUserViewRealtimeSnapshot(c, snapshot) {
			return errors.New("release order visibility was revoked")
		}
		data, visibleErr := h.realtimeSnapshotForCurrentUser(c, snapshot)
		if visibleErr != nil {
			return visibleErr
		}
		if data.Version == lastVisibleVersion {
			return nil
		}
		if writeErr := writeReleaseOrderRealtimeSSE(c, "snapshot", data.Version, data); writeErr != nil {
			return writeErr
		}
		lastVisibleVersion = data.Version
		return nil
	}
	for {
		select {
		case <-c.Request.Context().Done():
			return
		case snapshot, open := <-events:
			if !open {
				return
			}
			latestSnapshot = snapshot
			hasLatestSnapshot = true
			if err := emitVisibleSnapshot(snapshot); err != nil {
				return
			}
		case timestamp := <-heartbeat.C:
			// Permission may change while the SSE connection remains open. Recompute the visible
			// representation before heartbeat so a downgrade actively overwrites old sensitive data.
			if hasLatestSnapshot {
				if err := emitVisibleSnapshot(latestSnapshot); err != nil {
					return
				}
			}
			if err := writeReleaseOrderRealtimeSSE(c, "heartbeat", "", gin.H{"timestamp": timestamp.UTC()}); err != nil {
				return
			}
		}
	}
}

func (h *ReleaseOrderHandler) canCurrentUserViewRealtimeSnapshot(
	c *gin.Context,
	snapshot releaseOrderRealtimeSnapshot,
) bool {
	user, ok := getCurrentUser(c)
	if !ok || h == nil || h.authz == nil {
		return false
	}
	visible, err := canCurrentUserViewReleaseOrder(c.Request.Context(), h.authz, user, domain.ReleaseOrder{
		ID:                    snapshot.Payload.Order.ID,
		ApplicationID:         snapshot.Payload.Order.ApplicationID,
		EnvCode:               snapshot.Payload.Order.EnvCode,
		CreatorUserID:         snapshot.Payload.Order.CreatorUserID,
		ApprovalApproverIDs:   append([]string(nil), snapshot.Payload.Order.ApprovalApproverIDs...),
		ApprovalApproverNames: append([]string(nil), snapshot.Payload.Order.ApprovalApproverNames...),
	})
	return err == nil && visible
}

func (h *ReleaseOrderHandler) authorizeRealtimeOrder(c *gin.Context) (domain.ReleaseOrder, bool) {
	if !ensureAnyReleaseOrderDisplayPermission(c, h.authz) {
		return domain.ReleaseOrder{}, false
	}
	if h == nil || h.manager == nil || h.realtime == nil {
		c.JSON(http.StatusNotImplemented, gin.H{"error": "release order realtime is not configured"})
		return domain.ReleaseOrder{}, false
	}
	order, err := h.manager.GetStoredReleaseOrderByID(c.Request.Context(), c.Param("id"))
	if err != nil {
		writeReleaseOrderHTTPError(c, err)
		return domain.ReleaseOrder{}, false
	}
	if !ensureReleaseOrderVisible(c, h.authz, order) {
		return domain.ReleaseOrder{}, false
	}
	return order, true
}

func (h *ReleaseOrderHandler) realtimeSnapshotForCurrentUser(
	c *gin.Context,
	snapshot releaseOrderRealtimeSnapshot,
) (ReleaseOrderRealtimeSnapshotData, error) {
	canViewProgress := h.canCurrentUserViewReleaseValueProgress(c)
	payload := snapshot.Payload
	if !canViewProgress {
		payload.ValueProgress = []ReleaseOrderValueProgressResponse{}
	}
	fingerprint := struct {
		ValueProgressVisible bool `json:"value_progress_visible"`
		ReleaseOrderRealtimeSnapshotPayload
	}{
		ValueProgressVisible:                canViewProgress,
		ReleaseOrderRealtimeSnapshotPayload: payload,
	}
	encoded, err := json.Marshal(fingerprint)
	if err != nil {
		return ReleaseOrderRealtimeSnapshotData{}, err
	}
	digest := sha256.Sum256(encoded)
	return ReleaseOrderRealtimeSnapshotData{
		Version:                             hex.EncodeToString(digest[:]),
		GeneratedAt:                         snapshot.GeneratedAt,
		ValueProgressVisible:                canViewProgress,
		ReleaseOrderRealtimeSnapshotPayload: payload,
	}, nil
}

func (h *ReleaseOrderHandler) canCurrentUserViewReleaseValueProgress(c *gin.Context) bool {
	user, ok := getCurrentUser(c)
	if !ok || h == nil || h.authz == nil {
		return false
	}
	allowed, err := h.authz.HasPermission(c.Request.Context(), user, "release.param_snapshot.view", "", "")
	return err == nil && allowed
}

func (h *ReleaseOrderHandler) loadReleaseOrderRealtimeSnapshot(
	ctx context.Context,
	orderID string,
) (ReleaseOrderRealtimeSnapshotPayload, error) {
	aggregate, err := h.manager.LoadStoredReleaseOrderRealtimeAggregate(ctx, orderID)
	if err != nil {
		return ReleaseOrderRealtimeSnapshotPayload{}, err
	}
	order := enrichRealtimeReleaseOrder(aggregate.Order, aggregate.Executions, aggregate.ConcurrentBatchProgress)
	executionResponses := make([]ReleaseOrderExecutionResponse, 0, len(aggregate.Executions))
	for _, item := range aggregate.Executions {
		executionResponses = append(executionResponses, toReleaseOrderExecutionResponse(item))
	}
	stepResponses := make([]ReleaseOrderStepResponse, 0, len(aggregate.Steps))
	for _, item := range aggregate.Steps {
		stepResponses = append(stepResponses, toReleaseOrderStepResponse(item))
	}
	valueProgressResponses := make([]ReleaseOrderValueProgressResponse, 0, len(aggregate.ValueProgress))
	for _, item := range aggregate.ValueProgress {
		valueProgressResponses = append(valueProgressResponses, ReleaseOrderValueProgressResponse{
			PipelineScope:     string(item.PipelineScope),
			ParamKey:          item.ParamKey,
			ParamName:         item.ParamName,
			ExecutorParamName: item.ExecutorParamName,
			Required:          item.Required,
			PipelineParam:     item.PipelineParam,
			ValueKind:         string(item.ValueKind),
			Status:            string(item.Status),
			Value:             item.Value,
			ValueSource:       item.ValueSource,
			Message:           item.Message,
			UpdatedAt:         item.UpdatedAt,
			SortNo:            item.SortNo,
		})
	}
	stageResponses := make([]ReleaseOrderPipelineStageResponse, 0, len(aggregate.PipelineStageView.Stages))
	for _, item := range aggregate.PipelineStageView.Stages {
		stageResponses = append(stageResponses, toReleaseOrderPipelineStageResponse(item))
	}
	artifactResponses := make([]ReleaseOrderArtifactMetadataResponse, 0, len(aggregate.ArtifactMetadata))
	for _, item := range aggregate.ArtifactMetadata {
		artifactResponses = append(artifactResponses, toReleaseOrderArtifactMetadataResponse(item))
	}
	approvalResponses := make([]ReleaseOrderApprovalRecordResponse, 0, len(aggregate.ApprovalRecords))
	for _, item := range aggregate.ApprovalRecords {
		approvalResponses = append(approvalResponses, toReleaseOrderApprovalRecordResponse(item))
	}
	orderResponse := toReleaseOrderResponse(order, aggregate.AppReleaseState)
	orderResponse.LiveStateCanConfirm = aggregate.LiveStateCanConfirm
	if len(aggregate.DeploySnapshots) > 0 {
		orderResponse.DeploySnapshots = make([]ReleaseOrderDeploySnapshotResponse, 0, len(aggregate.DeploySnapshots))
		for _, item := range aggregate.DeploySnapshots {
			orderResponse.DeploySnapshots = append(orderResponse.DeploySnapshots, toReleaseOrderDeploySnapshotResponse(item))
		}
	}

	return ReleaseOrderRealtimeSnapshotPayload{
		Order:         orderResponse,
		Executions:    executionResponses,
		Steps:         stepResponses,
		ValueProgress: valueProgressResponses,
		PipelineStageView: ReleaseOrderPipelineStageListResponse{
			ShowModule:   aggregate.PipelineStageView.ShowModule,
			ExecutorType: aggregate.PipelineStageView.ExecutorType,
			Message:      aggregate.PipelineStageView.Message,
			Data:         stageResponses,
		},
		ArtifactMetadata:        artifactResponses,
		ApprovalRecords:         approvalResponses,
		ConcurrentBatchProgress: aggregate.ConcurrentBatchProgress,
	}, nil
}

func enrichRealtimeReleaseOrder(
	order domain.ReleaseOrder,
	executions []domain.ReleaseOrderExecution,
	progress *usecase.ReleaseOrderConcurrentBatchProgressOutput,
) domain.ReleaseOrder {
	hasRunningExecution := false
	for _, execution := range executions {
		if execution.Status == domain.ExecutionStatusRunning {
			hasRunningExecution = true
		}
		if execution.PipelineScope == domain.PipelineScopeCI {
			order.HasCIExecution = true
		}
		if execution.PipelineScope == domain.PipelineScopeCD {
			order.HasCDExecution = true
			order.CDProvider = strings.TrimSpace(execution.Provider)
		}
	}
	order.BusinessStatus = deriveReleaseBusinessStatus(order.Status, hasRunningExecution)
	if progress == nil {
		return order
	}
	for _, item := range progress.Items {
		if strings.TrimSpace(item.OrderID) != strings.TrimSpace(order.ID) {
			continue
		}
		switch item.QueueState {
		case usecase.ReleaseOrderConcurrentBatchQueueStateQueued:
			if order.BusinessStatus != domain.ReleaseBusinessStatusBuilding {
				order.BusinessStatus = domain.ReleaseBusinessStatusQueued
			}
			order.QueuePosition = item.QueuePosition
			if item.QueuePosition > 0 {
				order.QueuedReason = fmt.Sprintf("并发批次排队中，当前位次 %d", item.QueuePosition)
			}
		case usecase.ReleaseOrderConcurrentBatchQueueStateExecuting:
			if order.BusinessStatus != domain.ReleaseBusinessStatusBuilding {
				order.BusinessStatus = domain.ReleaseBusinessStatusDeploying
			}
		case usecase.ReleaseOrderConcurrentBatchQueueStateSuccess:
			order.BusinessStatus = domain.ReleaseBusinessStatusDeploySuccess
		case usecase.ReleaseOrderConcurrentBatchQueueStateFailed:
			order.BusinessStatus = domain.ReleaseBusinessStatusDeployFailed
		case usecase.ReleaseOrderConcurrentBatchQueueStateCancelled:
			order.BusinessStatus = domain.ReleaseBusinessStatusCancelled
		}
		break
	}
	return order
}

func writeReleaseOrderRealtimeSSE(
	c *gin.Context,
	eventName string,
	eventID string,
	payload any,
) error {
	controller := http.NewResponseController(c.Writer)
	_ = controller.SetWriteDeadline(time.Now().Add(releaseOrderRealtimeWriteTimeout))
	defer func() {
		_ = controller.SetWriteDeadline(time.Time{})
	}()
	encoded, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	if strings.TrimSpace(eventID) != "" {
		if _, err := fmt.Fprintf(c.Writer, "id: %s\n", strings.TrimSpace(eventID)); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintf(c.Writer, "event: %s\n", strings.TrimSpace(eventName)); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(c.Writer, "data: %s\n\n", encoded); err != nil {
		return err
	}
	return controller.Flush()
}
