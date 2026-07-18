import {
  onBeforeUnmount,
  ref,
  toValue,
  watch,
  type MaybeRefOrGetter,
} from "vue";
import {
  buildReleaseOrderRealtimeEventsURL,
  getReleaseOrderRealtimeSnapshot,
} from "../api/release";
import type { ReleaseOrderRealtimeSnapshot } from "../types/release";

export type ReleaseOrderRealtimeSnapshotSource =
  | "sse"
  | "fallback"
  | "manual";

export interface UseReleaseOrderRealtimeOptions {
  orderID: MaybeRefOrGetter<string>;
  accessToken: MaybeRefOrGetter<string>;
  enabled: MaybeRefOrGetter<boolean>;
  onSnapshot: (
    snapshot: ReleaseOrderRealtimeSnapshot,
    source: ReleaseOrderRealtimeSnapshotSource,
  ) => void;
  fallbackIntervalMs?: number;
}

const DEFAULT_FALLBACK_INTERVAL_MS = 5_000;
const CONNECT_TIMEOUT_MS = 10_000;
const STREAM_SILENCE_TIMEOUT_MS = 40_000;
const STABLE_CONNECTION_MS = 20_000;
const RECONNECT_DELAYS_MS = [1_000, 2_000, 5_000, 10_000] as const;

function isAbortError(error: unknown) {
  const value = error as { name?: string; code?: string } | null;
  return value?.name === "AbortError" || value?.code === "ERR_CANCELED";
}

function errorMessage(error: unknown) {
  if (error instanceof Error) {
    return error.message;
  }
  return String(error || "实时连接异常");
}

function hasDocumentVisibility() {
  return typeof document !== "undefined";
}

function isPageVisible() {
  return !hasDocumentVisibility() || !document.hidden;
}

function isNetworkOnline() {
  return typeof navigator === "undefined" || navigator.onLine !== false;
}

export function useReleaseOrderRealtime(
  options: UseReleaseOrderRealtimeOptions,
) {
  const connected = ref(false);
  const connecting = ref(false);
  const fallbackPolling = ref(false);
  const lastAppliedVersion = ref("");
  const lastSnapshotAt = ref("");
  const lastError = ref("");

  const fallbackIntervalMs = Math.max(
    1_000,
    Number(options.fallbackIntervalMs || DEFAULT_FALLBACK_INTERVAL_MS),
  );

  let active = false;
  let disposed = false;
  let generation = 0;
  let appliedRevision = 0;
  let reconnectAttempt = 0;
  let activeOrderID = "";
  let activeAccessToken = "";
  let streamController: AbortController | null = null;
  let snapshotController: AbortController | null = null;
  let snapshotRequestSource: ReleaseOrderRealtimeSnapshotSource | null = null;
  let streamTask: Promise<void> | null = null;
  let snapshotTask: Promise<void> | null = null;
  let reconnectTimer: number | null = null;
  let fallbackTimer: number | null = null;

  function currentOrderID() {
    return String(toValue(options.orderID) || "").trim();
  }

  function shouldRun() {
    return (
      active &&
      !disposed &&
      Boolean(currentOrderID()) &&
      Boolean(toValue(options.enabled)) &&
      isPageVisible() &&
      isNetworkOnline()
    );
  }

  function clearReconnectTimer() {
    if (reconnectTimer !== null) {
      window.clearTimeout(reconnectTimer);
      reconnectTimer = null;
    }
  }

  function clearFallbackTimer() {
    if (fallbackTimer !== null) {
      window.clearTimeout(fallbackTimer);
      fallbackTimer = null;
    }
  }

  function stopFallbackPolling(abortRequest = false) {
    fallbackPolling.value = false;
    clearFallbackTimer();
    if (
      abortRequest &&
      snapshotRequestSource === "fallback" &&
      snapshotController
    ) {
      snapshotController.abort();
    }
  }

  function closeNetwork(resetVersion = false) {
    generation += 1;
    clearReconnectTimer();
    stopFallbackPolling();
    streamController?.abort();
    snapshotController?.abort();
    streamController = null;
    snapshotController = null;
    streamTask = null;
    snapshotTask = null;
    snapshotRequestSource = null;
    connected.value = false;
    connecting.value = false;
    if (resetVersion) {
      lastAppliedVersion.value = "";
      lastSnapshotAt.value = "";
      appliedRevision = 0;
      reconnectAttempt = 0;
    }
  }

  function applySnapshot(
    snapshot: ReleaseOrderRealtimeSnapshot,
    source: ReleaseOrderRealtimeSnapshotSource,
    requestGeneration: number,
    requestOrderID: string,
    expectedRevision: number | null = null,
  ) {
    if (
      requestGeneration !== generation ||
      requestOrderID !== currentOrderID()
    ) {
      return false;
    }
    const version = String(snapshot?.version || "").trim();
    // Version is a SHA-256 content digest, not a sortable sequence number.
    if (
      !version ||
      version === lastAppliedVersion.value ||
      (expectedRevision !== null && expectedRevision !== appliedRevision)
    ) {
      return false;
    }
    options.onSnapshot(snapshot, source);
    lastAppliedVersion.value = version;
    lastSnapshotAt.value = String(snapshot.generated_at || "").trim();
    appliedRevision += 1;
    return true;
  }

  function requestSnapshot(source: ReleaseOrderRealtimeSnapshotSource) {
    if (snapshotTask) {
      return snapshotTask;
    }
    const requestOrderID = currentOrderID();
    if (!requestOrderID) {
      return Promise.resolve();
    }
    const requestGeneration = generation;
    const expectedRevision = appliedRevision;
    const controller = new AbortController();
    snapshotController = controller;
    snapshotRequestSource = source;

    const task = (async () => {
      try {
        const response = await getReleaseOrderRealtimeSnapshot(requestOrderID, {
          signal: controller.signal,
        });
        applySnapshot(
          response.data,
          source,
          requestGeneration,
          requestOrderID,
          expectedRevision,
        );
      } finally {
        if (snapshotController === controller) {
          snapshotController = null;
          snapshotRequestSource = null;
        }
        if (snapshotTask === task) {
          snapshotTask = null;
        }
      }
    })();
    snapshotTask = task;
    return task;
  }

  function scheduleFallbackPoll(delayMs: number) {
    if (!shouldRun() || connected.value || fallbackTimer !== null) {
      return;
    }
    fallbackTimer = window.setTimeout(() => {
      fallbackTimer = null;
      void runFallbackPoll();
    }, delayMs);
  }

  async function runFallbackPoll() {
    if (!shouldRun() || connected.value) {
      stopFallbackPolling();
      return;
    }
    fallbackPolling.value = true;
    try {
      await requestSnapshot("fallback");
      lastError.value = "";
    } catch (error) {
      if (!isAbortError(error)) {
        lastError.value = errorMessage(error);
      }
    } finally {
      if (shouldRun() && !connected.value) {
        scheduleFallbackPoll(fallbackIntervalMs);
      } else {
        stopFallbackPolling();
      }
    }
  }

  function startFallbackPolling(immediate = false) {
    if (!shouldRun() || connected.value) {
      return;
    }
    fallbackPolling.value = true;
    if (fallbackTimer !== null) {
      return;
    }
    if (immediate && !snapshotTask) {
      void runFallbackPoll();
      return;
    }
    scheduleFallbackPoll(fallbackIntervalMs);
  }

  function scheduleReconnect() {
    if (!shouldRun() || reconnectTimer !== null || streamTask) {
      return;
    }
    const delay =
      RECONNECT_DELAYS_MS[
        Math.min(reconnectAttempt, RECONNECT_DELAYS_MS.length - 1)
      ];
    reconnectAttempt += 1;
    reconnectTimer = window.setTimeout(() => {
      reconnectTimer = null;
      if (shouldRun()) {
        openEventStream();
      }
    }, delay);
  }

  function dispatchSSEEvent(
    eventName: string,
    data: string,
    requestGeneration: number,
    requestOrderID: string,
  ) {
    if (eventName === "heartbeat") {
      if (
        requestGeneration === generation &&
        requestOrderID === currentOrderID()
      ) {
        reconnectAttempt = 0;
      }
      return;
    }
    if (eventName !== "snapshot" || !data.trim()) {
      return;
    }
    try {
      const snapshot = JSON.parse(data) as ReleaseOrderRealtimeSnapshot;
      applySnapshot(snapshot, "sse", requestGeneration, requestOrderID);
    } catch (error) {
      lastError.value = `实时快照解析失败：${errorMessage(error)}`;
    }
  }

  async function consumeEventStream(
    response: Response,
    requestGeneration: number,
    requestOrderID: string,
    onStreamActivity: () => void,
  ) {
    if (!response.body) {
      throw new Error("浏览器未提供实时响应流");
    }
    const reader = response.body.getReader();
    const decoder = new TextDecoder();
    let buffer = "";
    let eventName = "message";
    let dataLines: string[] = [];

    function dispatchPendingEvent() {
      if (dataLines.length > 0 || eventName !== "message") {
        dispatchSSEEvent(
          eventName,
          dataLines.join("\n"),
          requestGeneration,
          requestOrderID,
        );
      }
      eventName = "message";
      dataLines = [];
    }

    function processLine(rawLine: string) {
      const line = rawLine.endsWith("\r") ? rawLine.slice(0, -1) : rawLine;
      if (!line) {
        dispatchPendingEvent();
        return;
      }
      if (line.startsWith(":")) {
        return;
      }
      const separatorIndex = line.indexOf(":");
      const field = separatorIndex >= 0 ? line.slice(0, separatorIndex) : line;
      let value = separatorIndex >= 0 ? line.slice(separatorIndex + 1) : "";
      if (value.startsWith(" ")) {
        value = value.slice(1);
      }
      if (field === "event") {
        eventName = value || "message";
      } else if (field === "data") {
        dataLines.push(value);
      }
    }

    try {
      while (requestGeneration === generation) {
        const { done, value } = await reader.read();
        if (done) {
          break;
        }
        onStreamActivity();
        buffer += decoder.decode(value, { stream: true });
        let newlineIndex = buffer.indexOf("\n");
        while (newlineIndex >= 0) {
          processLine(buffer.slice(0, newlineIndex));
          buffer = buffer.slice(newlineIndex + 1);
          newlineIndex = buffer.indexOf("\n");
        }
      }
      buffer += decoder.decode();
      if (buffer) {
        processLine(buffer);
      }
      dispatchPendingEvent();
    } finally {
      reader.releaseLock();
    }
  }

  function openEventStream() {
    if (!shouldRun() || streamTask) {
      return;
    }
    const requestOrderID = currentOrderID();
    const requestGeneration = generation;
    const controller = new AbortController();
    streamController = controller;
    connecting.value = true;

    const task = (async () => {
      const timeout = window.setTimeout(
        () => controller.abort(),
        CONNECT_TIMEOUT_MS,
      );
      let silenceTimeout: number | null = null;
      let stableConnectionTimer: number | null = null;
      let silenceTimedOut = false;
      const clearStreamTimers = () => {
        if (silenceTimeout !== null) {
          window.clearTimeout(silenceTimeout);
          silenceTimeout = null;
        }
        if (stableConnectionTimer !== null) {
          window.clearTimeout(stableConnectionTimer);
          stableConnectionTimer = null;
        }
      };
      const armSilenceWatchdog = () => {
        if (silenceTimeout !== null) {
          window.clearTimeout(silenceTimeout);
        }
        silenceTimeout = window.setTimeout(() => {
          if (
            requestGeneration === generation &&
            streamController === controller &&
            connected.value
          ) {
            silenceTimedOut = true;
            controller.abort();
          }
        }, STREAM_SILENCE_TIMEOUT_MS);
      };
      try {
        const token = String(toValue(options.accessToken) || "").trim();
        const headers: Record<string, string> = {
          Accept: "text/event-stream",
        };
        if (token) {
          headers.Authorization = `Bearer ${token}`;
        }
        const response = await fetch(
          buildReleaseOrderRealtimeEventsURL(requestOrderID),
          {
            method: "GET",
            headers,
            cache: "no-store",
            signal: controller.signal,
          },
        );
        window.clearTimeout(timeout);
        if (!response.ok) {
          throw new Error(`实时连接失败（HTTP ${response.status}）`);
        }
        if (
          requestGeneration !== generation ||
          requestOrderID !== currentOrderID()
        ) {
          return;
        }
        connecting.value = false;
        connected.value = true;
        lastError.value = "";
        stopFallbackPolling(true);
        armSilenceWatchdog();
        stableConnectionTimer = window.setTimeout(() => {
          if (
            requestGeneration === generation &&
            streamController === controller &&
            connected.value
          ) {
            reconnectAttempt = 0;
          }
        }, STABLE_CONNECTION_MS);
        await consumeEventStream(
          response,
          requestGeneration,
          requestOrderID,
          armSilenceWatchdog,
        );
        if (requestGeneration === generation && shouldRun()) {
          throw new Error("实时连接已结束");
        }
      } catch (error) {
        window.clearTimeout(timeout);
        if (
          requestGeneration !== generation ||
          !shouldRun()
        ) {
          return;
        }
        connected.value = false;
        lastError.value = silenceTimedOut
          ? "实时连接超过 40 秒未收到数据"
          : errorMessage(error);
        startFallbackPolling(true);
      } finally {
        window.clearTimeout(timeout);
        clearStreamTimers();
        if (streamController === controller) {
          streamController = null;
        }
        if (streamTask === task) {
          streamTask = null;
        }
        if (requestGeneration === generation) {
          connecting.value = false;
          if (!connected.value && shouldRun()) {
            scheduleReconnect();
          }
        }
      }
    })();
    streamTask = task;
  }

  function reconcile() {
    const nextOrderID = currentOrderID();
    const nextAccessToken = String(toValue(options.accessToken) || "").trim();
    if (nextOrderID !== activeOrderID) {
      activeOrderID = nextOrderID;
      closeNetwork(true);
    }
    if (nextAccessToken !== activeAccessToken) {
      activeAccessToken = nextAccessToken;
      closeNetwork(true);
    }
    if (!shouldRun()) {
      closeNetwork();
      return;
    }
    if (!connected.value && !connecting.value && !streamTask) {
      openEventStream();
    }
  }

  function start() {
    active = true;
    activeOrderID = currentOrderID();
    activeAccessToken = String(toValue(options.accessToken) || "").trim();
    reconcile();
  }

  function stop() {
    active = false;
    closeNetwork();
  }

  async function refreshNow() {
    closeNetwork();
    try {
      await requestSnapshot("manual");
    } finally {
      if (active && !disposed) {
        reconcile();
      }
    }
  }

  const stopStateWatch = watch(
    [
      () => currentOrderID(),
      () => Boolean(toValue(options.enabled)),
      () => String(toValue(options.accessToken) || "").trim(),
    ],
    () => {
      if (active) {
        reconcile();
      }
    },
    { flush: "sync" },
  );

  const handleVisibilityChange = () => reconcile();
  const handleOnline = () => reconcile();
  const handleOffline = () => reconcile();
  if (typeof document !== "undefined") {
    document.addEventListener("visibilitychange", handleVisibilityChange);
  }
  if (typeof window !== "undefined") {
    window.addEventListener("online", handleOnline);
    window.addEventListener("offline", handleOffline);
  }

  function dispose() {
    if (disposed) {
      return;
    }
    disposed = true;
    stop();
    stopStateWatch();
    if (typeof document !== "undefined") {
      document.removeEventListener("visibilitychange", handleVisibilityChange);
    }
    if (typeof window !== "undefined") {
      window.removeEventListener("online", handleOnline);
      window.removeEventListener("offline", handleOffline);
    }
  }

  onBeforeUnmount(dispose);

  return {
    connected,
    connecting,
    fallbackPolling,
    lastAppliedVersion,
    lastSnapshotAt,
    lastError,
    start,
    stop,
    refreshNow,
  };
}
