type InboxDebugDetails = Record<string, unknown>;

type InboxDebugEntry = {
  at: string;
  elapsed_ms: number;
  event: string;
  details: InboxDebugDetails;
};

type InboxDebugAPI = {
  enable: () => void;
  disable: () => void;
  clear: () => void;
  dump: () => InboxDebugEntry[];
  snapshot: (reason?: string) => InboxDebugDetails;
};

declare global {
  interface Window {
    __chatloopInboxDebug?: InboxDebugAPI;
  }
}

const DEBUG_STORAGE_KEY = 'chatloop_inbox_debug';
const DEBUG_SESSION_STORAGE_KEY = 'chatloop_inbox_debug_session';
const MAX_DEBUG_ENTRIES = 240;
const HEARTBEAT_INTERVAL_MS = 250;
const MAIN_THREAD_STALL_MS = 180;
const DEBUG_WINDOW_MS = 5 * 60_000;
const WORKER_STALL_GRACE_MS = 10_000;
const CONSOLE_LOGS_PER_SECOND = 40;
const startedAt = window.performance.now();
const entries: InboxDebugEntry[] = [];

function createDebugSessionID() {
  try {
    const existing = window.sessionStorage.getItem(DEBUG_SESSION_STORAGE_KEY);
    if (existing) return existing;
    const created = typeof window.crypto.randomUUID === 'function'
      ? window.crypto.randomUUID()
      : `${Date.now()}-${Math.random().toString(36).slice(2)}`;
    window.sessionStorage.setItem(DEBUG_SESSION_STORAGE_KEY, created);
    return created;
  } catch {
    return `${Date.now()}-${Math.random().toString(36).slice(2)}`;
  }
}

const debugSessionID = createDebugSessionID();

let enabled = import.meta.env.DEV;
let activeUntil = 0;
let heartbeatTimer = 0;
let expectedHeartbeat = 0;
let inputFramePending = false;
let inputFrameStartedAt = 0;
let inputEventsInFrame = 0;
let inputEventsTotal = 0;
let lastInputSampleAt = 0;
let lastKeyboardSampleAt = 0;
let lastTextInputSampleAt = 0;
let lastFrameDelayLogAt = 0;
let lastFrameDelaySnapshotAt = 0;
let consoleWindowStartedAt = 0;
let consoleLogsInWindow = 0;
let suppressedConsoleLogs = 0;
let consoleEnabled = false;
let activeAgentID = 0;
let reportedAgentID = 0;
let remoteFlushTimer = 0;
let remoteFlushRunning = false;
let remoteFailureCount = 0;
let stallWorker: Worker | null = null;
const componentCommits = new Map<string, { count: number; reportedAt: number }>();
const remoteEntries: InboxDebugEntry[] = [];

try {
  const explicitlyEnabled = window.localStorage.getItem(DEBUG_STORAGE_KEY) === '1'
    || new URLSearchParams(window.location.search).get('inboxDebug') === '1';
  enabled = enabled || explicitlyEnabled;
  // Log server tetap otomatis di development. Menulis ratusan object ke
  // console Safari dapat menahan object/DOM dan menambah beban paint, sehingga
  // console hanya dinyalakan bila debug diminta eksplisit.
  consoleEnabled = explicitlyEnabled;
} catch {
  // Storage dapat ditolak browser; mode development tetap bisa dipakai.
}

function now() {
  return window.performance.now();
}

function describeElement(target: EventTarget | Element | null): string {
  if (!(target instanceof Element)) return target ? target.constructor.name : 'none';
  const role = target.getAttribute('data-kirimwa-role') || target.getAttribute('role') || '';
  const semanticParent = target.closest<HTMLElement>('[data-kirimwa-role]');
  const parentRole = !role && semanticParent
    ? semanticParent.getAttribute('data-kirimwa-role') || ''
    : '';
  const id = target.id ? `#${target.id}` : '';
  const classes = Array.from(target.classList).slice(0, 3).join('.');
  return `${target.tagName.toLowerCase()}${id}${classes ? `.${classes}` : ''}${role ? `[${role}]` : ''}${parentRole ? `[within:${parentRole}]` : ''}`;
}

function debugErrorLabel(value: unknown): string {
  const message = value instanceof Error
    ? `${value.name}: ${value.message}`
    : typeof value === 'string'
      ? value
      : Object.prototype.toString.call(value);
  return message.replace(/[\r\n\t]+/g, ' ').slice(0, 320);
}

function currentSnapshot(): InboxDebugDetails {
  const overlaySelectors = [
    '.MuiBackdrop-root',
    '.MuiModal-root',
    '.MuiPopover-root',
    '.swal2-container',
    '.swal2-popup',
    '[role="dialog"]',
  ].join(',');
  const overlays = Array.from(document.querySelectorAll(overlaySelectors))
    .slice(0, 12)
    .map((element) => {
      const style = window.getComputedStyle(element);
      const rect = element.getBoundingClientRect();
      return {
        element: describeElement(element),
        display: style.display,
        visibility: style.visibility,
        pointer_events: style.pointerEvents,
        z_index: style.zIndex,
        width: Math.round(rect.width),
        height: Math.round(rect.height),
        aria_hidden: element.getAttribute('aria-hidden'),
      };
    });
  return {
    visibility: document.visibilityState,
    active_element: describeElement(document.activeElement),
    body_classes: document.body.className,
    body_overflow: window.getComputedStyle(document.body).overflow,
    overlays,
  };
}

function emit(event: string, details: InboxDebugDetails = {}) {
  if (!enabled) return;
  const entry: InboxDebugEntry = {
    at: new Date().toISOString(),
    elapsed_ms: Math.round(now() - startedAt),
    event,
    details,
  };
  entries.push(entry);
  if (entries.length > MAX_DEBUG_ENTRIES) entries.splice(0, entries.length - MAX_DEBUG_ENTRIES);
  if (consoleEnabled) {
    const emittedAt = now();
    if (consoleWindowStartedAt === 0 || emittedAt - consoleWindowStartedAt >= 1_000) {
      if (suppressedConsoleLogs > 0) {
        console.info('[InboxDebug] console-log.throttled', { suppressed: suppressedConsoleLogs });
      }
      consoleWindowStartedAt = emittedAt;
      consoleLogsInWindow = 0;
      suppressedConsoleLogs = 0;
    }
    if (consoleLogsInWindow < CONSOLE_LOGS_PER_SECOND) {
      consoleLogsInWindow += 1;
      console.info(`[InboxDebug] ${event}`, details);
    } else {
      suppressedConsoleLogs += 1;
    }
  }
  queueRemoteEntry(entry);
}

function scheduleRemoteFlush(delayMs: number) {
  if (!enabled || activeAgentID <= 0 || remoteFlushRunning || remoteFlushTimer) return;
  remoteFlushTimer = window.setTimeout(() => {
    remoteFlushTimer = 0;
    void flushRemoteEntries();
  }, delayMs);
}

function ensureStallWorker() {
  if (stallWorker || typeof Worker === 'undefined') return stallWorker;
  try {
    const source = `
      let config = null;
      let lastPingAt = Date.now();
      let lastVisibility = 'visible';
      let lastReportAt = 0;
      self.onmessage = (event) => {
        const message = event.data || {};
        if (message.type === 'activate') {
          config = message;
          lastPingAt = Date.now();
          lastVisibility = message.visibility || 'visible';
          lastReportAt = 0;
        } else if (message.type === 'ping') {
          lastPingAt = Date.now();
          lastVisibility = message.visibility || lastVisibility;
        } else if (message.type === 'stop') {
          config = null;
        }
      };
      setInterval(() => {
        if (!config || Date.now() > config.activeUntil || lastVisibility !== 'visible') return;
        const blockedMs = Date.now() - lastPingAt;
        if (blockedMs < 900 || Date.now() - lastReportAt < 1500) return;
        lastReportAt = Date.now();
        const entry = {
          at: new Date().toISOString(),
          elapsed_ms: 0,
          event: 'worker.main-thread-unresponsive',
          details: { blocked_ms: blockedMs, detector: 'web-worker' },
        };
        fetch(config.endpoint, {
          method: 'POST',
          headers: {
            'Content-Type': 'application/json',
            Authorization: 'Bearer ' + config.token,
          },
          body: JSON.stringify({ session_id: config.sessionID, entries: [entry] }),
          cache: 'no-store',
        }).catch(() => undefined);
      }, 400);
    `;
    const workerURL = URL.createObjectURL(new Blob([source], { type: 'text/javascript' }));
    stallWorker = new Worker(workerURL, { name: 'kirimwa-inbox-debug' });
    URL.revokeObjectURL(workerURL);
  } catch {
    stallWorker = null;
  }
  return stallWorker;
}

function activateStallWorker(durationMs: number) {
  if (!enabled || activeAgentID <= 0) return;
  const token = window.localStorage.getItem('token');
  if (!token) return;
  const worker = ensureStallWorker();
  if (!worker) return;
  worker.postMessage({
    type: 'activate',
    endpoint: new URL(`/api/agents/${activeAgentID}/inbox/client-debug`, window.location.origin).toString(),
    token,
    sessionID: debugSessionID,
    // Grace memastikan hang yang mulai persis di ujung jendela debug tetap
    // dilaporkan oleh worker walau timer main thread sudah tidak sempat hidup.
    activeUntil: Date.now() + durationMs + WORKER_STALL_GRACE_MS,
    visibility: document.visibilityState,
  });
}

function pingStallWorker() {
  stallWorker?.postMessage({ type: 'ping', visibility: document.visibilityState });
}

function queueRemoteEntry(entry: InboxDebugEntry) {
  if (!enabled || activeAgentID <= 0) return;
  remoteEntries.push(entry);
  if (remoteEntries.length > MAX_DEBUG_ENTRIES) {
    remoteEntries.splice(0, remoteEntries.length - MAX_DEBUG_ENTRIES);
  }
  const urgent = entry.event === 'debug-window.start'
    || entry.event === 'ticket.group-request.success'
    || entry.event === 'main-thread.stall';
  scheduleRemoteFlush(urgent ? 100 : 750);
}

async function flushRemoteEntries() {
  if (!enabled || remoteFlushRunning || activeAgentID <= 0 || remoteEntries.length === 0) return;
  const token = window.localStorage.getItem('token');
  if (!token) return;
  const agentID = activeAgentID;
  // Safari membatasi total body fetch keepalive sekitar 64 KiB. Batch kecil
  // menjaga snapshot overlay yang cukup besar tetap dapat terkirim.
  const batch = remoteEntries.splice(0, 24);
  remoteFlushRunning = true;
  try {
    const response = await window.fetch(`/api/agents/${agentID}/inbox/client-debug`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        Authorization: `Bearer ${token}`,
      },
      body: JSON.stringify({ session_id: debugSessionID, entries: batch }),
      cache: 'no-store',
      keepalive: true,
    });
    if (!response.ok) throw new Error(`debug endpoint ${response.status}`);
    remoteFailureCount = 0;
  } catch {
    remoteFailureCount += 1;
    if (remoteFailureCount <= 3) {
      remoteEntries.unshift(...batch);
      if (remoteEntries.length > MAX_DEBUG_ENTRIES) remoteEntries.length = MAX_DEBUG_ENTRIES;
    }
  } finally {
    remoteFlushRunning = false;
    if (remoteEntries.length > 0) {
      scheduleRemoteFlush(remoteFailureCount > 0
        ? Math.min(5_000, 500 * (2 ** remoteFailureCount))
        : 750);
    }
  }
}

function debugActive() {
  return enabled && now() <= activeUntil;
}

export function isInboxDebugActive() {
  return debugActive();
}

export function configureInboxDebugAgent(agentID: number) {
  if (!Number.isSafeInteger(agentID) || agentID <= 0) return;
  activeAgentID = agentID;
  if (enabled && reportedAgentID !== agentID) {
    reportedAgentID = agentID;
    emit('debug.transport.ready', { agent_id: agentID });
  }
}

function armHeartbeat() {
  if (!enabled || heartbeatTimer) return;
  pingStallWorker();
  expectedHeartbeat = now() + HEARTBEAT_INTERVAL_MS;
  const tick = () => {
    heartbeatTimer = 0;
    const tickAt = now();
    pingStallWorker();
    // Safari menahan setTimeout ketika tab berada di background. Drift tersebut
    // bukan hang aplikasi, jadi reset baseline sebelum melakukan pengukuran.
    if (document.visibilityState !== 'visible') {
      if (tickAt > activeUntil) return;
      expectedHeartbeat = tickAt + HEARTBEAT_INTERVAL_MS;
      heartbeatTimer = window.setTimeout(tick, HEARTBEAT_INTERVAL_MS);
      return;
    }
    const drift = tickAt - expectedHeartbeat;
    if (drift >= MAIN_THREAD_STALL_MS) {
      emit('main-thread.stall', {
        blocked_ms: Math.round(drift),
        react_commits_pending: Object.fromEntries(
          Array.from(componentCommits, ([component, sample]) => [component, sample.count]),
        ),
        ...currentSnapshot(),
      });
    }
    if (tickAt > activeUntil) return;
    expectedHeartbeat = tickAt + HEARTBEAT_INTERVAL_MS;
    heartbeatTimer = window.setTimeout(tick, HEARTBEAT_INTERVAL_MS);
  };
  heartbeatTimer = window.setTimeout(tick, HEARTBEAT_INTERVAL_MS);
}

export function activateInboxDebugWindow(
  reason: string,
  details: InboxDebugDetails = {},
  durationMs = DEBUG_WINDOW_MS,
) {
  if (!enabled) return;
  const detailAgentID = Number(details.agent_id);
  if (Number.isSafeInteger(detailAgentID) && detailAgentID > 0) activeAgentID = detailAgentID;
  activeUntil = Math.max(activeUntil, now() + durationMs);
  emit('debug-window.start', { reason, duration_ms: durationMs, ...details });
  activateStallWorker(durationMs);
  armHeartbeat();
}

export function inboxDebugLog(event: string, details: InboxDebugDetails = {}, force = false) {
  if (!enabled || (!force && !debugActive())) return;
  emit(event, details);
}

export function captureInboxDebugSnapshot(reason: string) {
  const snapshot = currentSnapshot();
  inboxDebugLog('ui.snapshot', { reason, ...snapshot });
  return snapshot;
}

// Sampling dikoales per animation frame. Isi pesan tidak pernah dicatat.
export function sampleComposerInput(details: InboxDebugDetails) {
  if (!debugActive()) return;
  const inputAt = now();
  inputEventsTotal += 1;
  inputEventsInFrame += 1;
  if (inputAt - lastInputSampleAt >= 750) {
    lastInputSampleAt = inputAt;
    emit('composer.input-sample', { sequence: inputEventsTotal, ...details });
  }
  if (inputFramePending) return;
  inputFramePending = true;
  inputFrameStartedAt = inputAt;
  window.requestAnimationFrame(() => {
    const frameDelay = now() - inputFrameStartedAt;
    const coalescedEvents = inputEventsInFrame;
    inputFramePending = false;
    inputEventsInFrame = 0;
    const frameAt = now();
    // Sampling maksimal dua kali per detik. Snapshot overlay memanggil style /
    // layout API, jadi hanya diambil berkala agar diagnostik tidak memperparah
    // repaint textarea yang sedang diukur.
    if (frameDelay >= 50 && frameAt - lastFrameDelayLogAt >= 500) {
      lastFrameDelayLogAt = frameAt;
      const details: InboxDebugDetails = {
        delay_ms: Math.round(frameDelay),
        input_events: coalescedEvents,
      };
      if (frameAt - lastFrameDelaySnapshotAt >= 1_500 || frameDelay >= 250) {
        lastFrameDelaySnapshotAt = frameAt;
        Object.assign(details, currentSnapshot());
      }
      emit('composer.frame-delay', details);
    }
  });
}

export function sampleInboxComponentCommit(component: string, details: InboxDebugDetails = {}) {
  if (!debugActive()) return;
  const commitAt = now();
  const sample = componentCommits.get(component) || { count: 0, reportedAt: 0 };
  sample.count += 1;
  if (sample.reportedAt === 0 || commitAt - sample.reportedAt >= 1_000) {
    emit('react.commit-rate', {
      component,
      commits_since_sample: sample.count,
      sample_window_ms: sample.reportedAt ? Math.round(commitAt - sample.reportedAt) : 0,
      ...details,
    });
    sample.count = 0;
    sample.reportedAt = commitAt;
  }
  componentCommits.set(component, sample);
}

if (typeof PerformanceObserver !== 'undefined'
  && PerformanceObserver.supportedEntryTypes?.includes('longtask')) {
  try {
    const observer = new PerformanceObserver((list) => {
      if (!debugActive()) return;
      for (const entry of list.getEntries()) {
        if (entry.duration < 50) continue;
        emit('performance.long-task', {
          duration_ms: Math.round(entry.duration),
          start_ms: Math.round(entry.startTime),
        });
      }
    });
    observer.observe({ entryTypes: ['longtask'] });
  } catch {
    // Safari versi tertentu belum menyediakan Long Tasks API; heartbeat tetap aktif.
  }
}

window.addEventListener('focusin', (event) => {
  inboxDebugLog('focus.in', { target: describeElement(event.target) });
}, true);

window.addEventListener('pointerdown', (event) => {
  inboxDebugLog('pointer.down', { target: describeElement(event.target) });
}, true);

window.addEventListener('keydown', (event) => {
  if (!debugActive()) return;
  const eventAt = now();
  if (eventAt - lastKeyboardSampleAt < 750) return;
  lastKeyboardSampleAt = eventAt;
  const keyType = event.key.length === 1 ? 'character' : event.key;
  emit('keyboard.sample', {
    key_type: keyType,
    target: describeElement(event.target),
    default_prevented: event.defaultPrevented,
    composing: event.isComposing,
  });
}, true);

window.addEventListener('input', (event) => {
  if (!debugActive()) return;
  if (!(event.target instanceof HTMLInputElement || event.target instanceof HTMLTextAreaElement)) return;
  const eventAt = now();
  if (eventAt - lastTextInputSampleAt < 750) return;
  lastTextInputSampleAt = eventAt;
  emit('text-input.sample', {
    target: describeElement(event.target),
    length: event.target.value.length,
    input_type: event.target instanceof HTMLTextAreaElement ? 'textarea' : event.target.type,
  });
}, true);

window.addEventListener('error', (event) => {
  inboxDebugLog('window.error', {
    message: debugErrorLabel(event.error || event.message),
    filename: (event.filename || '').split('/').pop() || '',
    line: event.lineno || 0,
    column: event.colno || 0,
  }, true);
});

window.addEventListener('unhandledrejection', (event) => {
  inboxDebugLog('window.unhandled-rejection', {
    message: debugErrorLabel(event.reason),
  }, true);
});

document.addEventListener('visibilitychange', () => {
  // Beri tahu worker segera saat tab disembunyikan/ditampilkan dan buang drift
  // timer yang terakumulasi selama Safari melakukan background throttling.
  expectedHeartbeat = now() + HEARTBEAT_INTERVAL_MS;
  pingStallWorker();
  inboxDebugLog('page.lifecycle', {
    kind: 'visibilitychange',
    visibility: document.visibilityState,
  });
  if (document.visibilityState === 'visible' && debugActive()) armHeartbeat();
});

window.addEventListener('pagehide', (event) => {
  inboxDebugLog('page.lifecycle', {
    kind: 'pagehide',
    persisted: event.persisted,
    visibility: document.visibilityState,
  });
  if (remoteFlushTimer) window.clearTimeout(remoteFlushTimer);
  remoteFlushTimer = 0;
  void flushRemoteEntries();
});

try {
  configureInboxDebugAgent(Number(window.localStorage.getItem('wai_agent')) || 0);
} catch {
  // InboxPanel akan mengonfigurasi agent setelah aplikasi siap.
}

window.__chatloopInboxDebug = {
  enable: () => {
    enabled = true;
    consoleEnabled = true;
    try {
      window.localStorage.setItem(DEBUG_STORAGE_KEY, '1');
    } catch {
      // Tetap aktif sampai reload walau storage ditolak.
    }
    activateInboxDebugWindow('manual-enable');
  },
  disable: () => {
    enabled = false;
    activeUntil = 0;
    if (heartbeatTimer) window.clearTimeout(heartbeatTimer);
    heartbeatTimer = 0;
    if (remoteFlushTimer) window.clearTimeout(remoteFlushTimer);
    remoteFlushTimer = 0;
    remoteEntries.length = 0;
    stallWorker?.postMessage({ type: 'stop' });
    stallWorker?.terminate();
    stallWorker = null;
    try {
      window.localStorage.removeItem(DEBUG_STORAGE_KEY);
    } catch {
      // Tidak ada tindakan tambahan.
    }
  },
  clear: () => {
    entries.length = 0;
    remoteEntries.length = 0;
  },
  dump: () => {
    const copy = entries.map((entry) => ({ ...entry, details: { ...entry.details } }));
    console.table(copy.map((entry) => ({
      elapsed_ms: entry.elapsed_ms,
      event: entry.event,
      details: JSON.stringify(entry.details),
    })));
    return copy;
  },
  snapshot: (reason = 'manual') => {
    const snapshot = currentSnapshot();
    emit('ui.snapshot', { reason, ...snapshot });
    return snapshot;
  },
};

export {};
