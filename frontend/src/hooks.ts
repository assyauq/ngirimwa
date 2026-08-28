import { useEffect, useRef } from 'react';
import { useQuery, useMutation, useQueryClient, type QueryClient } from '@tanstack/react-query';
import api from './services/api';
import { inboxDebugLog } from './services/inboxDebug';
import type { Analytics, AIMetrics, Contact, ChatMsg, ConversationBrief, HistorySyncStatus, LinkPreview, Broadcast, BroadcastDetailData, BroadcastSafetyForm, BroadcastConsentSummary, WAGroup, GroupGuardConfig, GroupModerationLog, LabelInfo, ScheduledMessage, AutoReply, Template, SavedContact, SavedContactsResp, LeadStage, FollowUp, Agent, KnowledgeItem, Handoff, CrawlJob, CrawlPage, KnowledgeUsage, ScheduledStatus, ApiSettings, Flow, Product, ProductOrder, AIForm, AIFormSubmission, TeamUser, CSActivityResp, AgentUnreadSummary, InboxRealtimeEvent, InboxSendResult } from './types';

type ContactList = { number: string; name: string }[];
const INBOX_TEXT_SEND_TIMEOUT_MS = 22_000;
const INBOX_MEDIA_SEND_TIMEOUT_MS = 95_000;
const INBOX_READ_TIMEOUT_MS = 12_000;
const INBOX_TYPING_TIMEOUT_MS = 4_000;
const INBOX_LIVE_INVALIDATION_MS = 120;
const INBOX_SYNC_INVALIDATION_MS = 900;
const LOCAL_CACHE_REALTIME_SUPPRESSION_MS = 10_000;
const LOCAL_CACHE_REDUNDANT_EVENT_KINDS = new Set(['message', 'receipt', 'delivery']);
const CONVERSATION_BRIEF_CONTENT_EVENT_KINDS = new Set([
  'incoming', 'message', 'history', 'history_sync', 'revoke', 'delete', 'reset', 'analysis',
]);
const localCacheHotUntil = new Map<string, number>();
const localCacheMessageHotUntil = new Map<string, number>();

// ---- Tenant ----



// ---- Fitur: analitik, inbox, test chat ----

export function useAgentAnalytics(agentId: number) {
  return useQuery<Analytics>({
    queryKey: ['analytics', agentId],
    queryFn: async () => (await api.get(`/agents/${agentId}/analytics`)).data,
    enabled: !!agentId,
  });
}

export function useAgentAIMetrics(agentId: number) {
  return useQuery<AIMetrics>({
    queryKey: ['ai-metrics', agentId],
    queryFn: async () => (await api.get(`/agents/${agentId}/ai-metrics`)).data,
    enabled: !!agentId,
    refetchInterval: 10000,
  });
}

export function useContacts(agentId: number) {
  return useQuery<Contact[]>({
    queryKey: ['contacts', agentId],
    queryFn: async ({ signal }) =>
      (await api.get(`/agents/${agentId}/contacts`, {
        timeout: INBOX_READ_TIMEOUT_MS,
        signal,
      })).data.data,
    enabled: !!agentId,
    // SSE menjadi jalur utama; refresh daftar berkala hanya aktif saat tab aktif.
    // Notifikasi mempunyai fallback cursor tersendiri yang jauh lebih ringan.
    refetchInterval: 60_000,
    refetchIntervalInBackground: false,
    refetchOnWindowFocus: true,
    staleTime: 30_000,
    retry: false,
    notifyOnChangeProps: ['data', 'isLoading', 'isError'],
    placeholderData: (prev) => prev,
  });
}

export type ConversationPage = {
  data: ChatMsg[];
  sender: string;
  has_more: boolean;
  loaded_count: number;
  next_before_at?: string | null;
  next_before_id?: number;
  needs_human: boolean;
  manual_pause_until?: string | null;
  media_token: string;
};

const INBOX_MESSAGE_PAGE_SIZE = 100;

export function useConversation(agentId: number, sender: string) {
  return useQuery<ConversationPage>({
    queryKey: ['conversation', agentId, sender],
    queryFn: async ({ signal }) =>
      (await api.get(`/agents/${agentId}/conversation`, {
        params: { sender, limit: INBOX_MESSAGE_PAGE_SIZE },
        timeout: INBOX_READ_TIMEOUT_MS,
        signal,
      })).data,
    enabled: !!agentId && !!sender,
    // Hanya halaman terbaru yang disegarkan. Halaman lama mempunyai cache lokal
    // tersendiri agar pesan ribuan tidak diunduh ulang saat ada pesan masuk.
    refetchInterval: 30_000,
    refetchIntervalInBackground: false,
    staleTime: 4_000,
    retry: false,
    // Jangan tampilkan thread chat sebelumnya saat ganti kontak (terasa "lemot"/salah).
    // Cache per-sender tetap dipakai react-query lewat queryKey.
    gcTime: 5 * 60_000,
    placeholderData: (previous) => previous?.sender === sender ? previous : undefined,
  });
}

export function useLoadOlderConversation(agentId: number) {
  return useMutation({
    mutationFn: async (input: {
      sender: string;
      before_at: string;
      before_id: number;
    }) => (await api.get(`/agents/${agentId}/conversation`, {
      params: {
        sender: input.sender,
        limit: INBOX_MESSAGE_PAGE_SIZE,
        before_at: input.before_at,
        before_id: input.before_id,
      },
      timeout: INBOX_READ_TIMEOUT_MS,
    })).data as ConversationPage,
  });
}

function normalizeInboxEventSender(value?: string): string {
  const raw = (value || '').trim();
  if (!raw) return '';
  if (raw.endsWith('@g.us')) return raw;
  const user = raw.split('@', 1)[0].split(':', 1)[0];
  const digits = user.replace(/\D/g, '');
  return digits || raw;
}

function localCacheKey(agentId: number, sender: string): string {
  return `${agentId}:${normalizeInboxEventSender(sender)}`;
}

// Menandai brief tanpa langsung menembakkan request. Query akan menyegarkan
// brief stale dalam interval pendek, sementara composer tetap bebas dari kerja
// jaringan/render tambahan pada jalur keystroke dan send.
function markConversationBriefStale(qc: QueryClient, agentId: number, sender: string) {
  const normalizedSender = normalizeInboxEventSender(sender);
  if (!normalizedSender) return;
  qc.setQueryData<ConversationBrief>(
    ['conversation-brief', agentId, normalizedSender],
    (current) => current && !current.stale ? { ...current, stale: true } : current,
  );
}

function markConversationLocallyCached(agentId: number, sender: string, messageID?: string) {
  const key = localCacheKey(agentId, sender);
  if (key.endsWith(':')) return;
  const hotUntil = Date.now() + LOCAL_CACHE_REALTIME_SUPPRESSION_MS;
  localCacheHotUntil.set(key, hotUntil);
  const normalizedMessageID = (messageID || '').trim();
  if (normalizedMessageID) {
    localCacheMessageHotUntil.set(`${agentId}:${normalizedMessageID}`, hotUntil);
    if (localCacheMessageHotUntil.size > 512) {
      const currentTime = Date.now();
      for (const [messageKey, expiresAt] of localCacheMessageHotUntil) {
        if (expiresAt <= currentTime) localCacheMessageHotUntil.delete(messageKey);
      }
      while (localCacheMessageHotUntil.size > 512) {
        const oldest = localCacheMessageHotUntil.keys().next().value;
        if (!oldest) break;
        localCacheMessageHotUntil.delete(oldest);
      }
    }
  }
}

function isRedundantLocalCacheEvent(
  agentId: number,
  sender: string,
  kind: string,
  messageID?: string,
): boolean {
  if (!LOCAL_CACHE_REDUNDANT_EVENT_KINDS.has(kind)) return false;
  const normalizedMessageID = (messageID || '').trim();
  if (normalizedMessageID) {
    const messageKey = `${agentId}:${normalizedMessageID}`;
    const hotUntil = localCacheMessageHotUntil.get(messageKey) || 0;
    if (hotUntil > Date.now()) return true;
    localCacheMessageHotUntil.delete(messageKey);
  }
  if (!sender) return false;
  const key = localCacheKey(agentId, sender);
  const hotUntil = localCacheHotUntil.get(key) || 0;
  if (hotUntil <= Date.now()) {
    localCacheHotUntil.delete(key);
    return false;
  }
  return true;
}

/**
 * Stream perubahan Inbox dengan Authorization header.
 *
 * Event hanya berfungsi sebagai sinyal invalidasi; API percakapan tetap menjadi
 * sumber data utama sehingga reconnect/duplikasi event tidak menggandakan pesan.
 */
export function useInboxRealtime(
  agentId: number,
  activeSender: string,
  onIncoming?: (event: InboxRealtimeEvent) => void,
  onEvent?: (event: InboxRealtimeEvent) => void,
  refreshData = true,
) {
  const qc = useQueryClient();
  const activeSenderRef = useRef(activeSender);
  const onIncomingRef = useRef(onIncoming);
  const onEventRef = useRef(onEvent);

  useEffect(() => {
    activeSenderRef.current = activeSender;
  }, [activeSender]);

  useEffect(() => {
    onIncomingRef.current = onIncoming;
  }, [onIncoming]);

  useEffect(() => {
    onEventRef.current = onEvent;
  }, [onEvent]);

  useEffect(() => {
    if (!agentId) return undefined;
    const token = localStorage.getItem('token');
    if (!token) return undefined;

    const controller = new AbortController();
    const pendingSenders = new Set<string>();
    const pendingEventKinds = new Map<string, number>();
    const pendingEvents = new Map<string, {
      sender: string;
      kind: string;
      messageID: string;
      count: number;
    }>();
    const handledIncoming = new Set<string>();
    let invalidateTimer = 0;
    let invalidateDueAt = 0;
    let refreshUnreadSummary = false;
    let incomingPollTimer = 0;
    let incomingPollRunning = false;
    let incomingPollStopped = false;

    const flushInvalidations = () => {
      invalidateTimer = 0;
      invalidateDueAt = 0;
      if (!refreshData) {
        pendingSenders.clear();
        pendingEventKinds.clear();
        pendingEvents.clear();
        refreshUnreadSummary = false;
        return;
      }
      const selected = normalizeInboxEventSender(activeSenderRef.current);
      const unsuppressedSenders = new Set<string>();
      const staleBriefSenders = new Set<string>();
      let hasUnsuppressedUnknownSender = false;
      let suppressedCachedEvents = 0;
      for (const event of pendingEvents.values()) {
        if (isRedundantLocalCacheEvent(agentId, event.sender, event.kind, event.messageID)) {
          suppressedCachedEvents += event.count;
          continue;
        }
        if (event.sender) {
          unsuppressedSenders.add(event.sender);
          if (CONVERSATION_BRIEF_CONTENT_EVENT_KINDS.has(event.kind)) {
            staleBriefSenders.add(event.sender);
          }
        }
        else hasUnsuppressedUnknownSender = true;
      }
      inboxDebugLog('realtime.invalidate.flush', {
        pending_senders: pendingSenders.size,
        selected_sender_matched: Array.from(pendingSenders).some(
          (sender) => Boolean(selected) && normalizeInboxEventSender(sender) === selected,
        ),
        suppressed_cached_events: suppressedCachedEvents,
        event_kinds: Object.fromEntries(pendingEventKinds),
      });
      if (hasUnsuppressedUnknownSender || unsuppressedSenders.size > 0) {
        void qc.invalidateQueries(
          { queryKey: ['contacts', agentId], refetchType: 'active' },
          { cancelRefetch: false },
        );
      }
      if (refreshUnreadSummary) {
        void qc.invalidateQueries(
          { queryKey: ['agent-unread-summary'], refetchType: 'active' },
          { cancelRefetch: false },
        );
      }
      refreshUnreadSummary = false;

      for (const staleSender of staleBriefSenders) {
        markConversationBriefStale(qc, agentId, staleSender);
      }

      if (selected && (
        hasUnsuppressedUnknownSender
        || Array.from(unsuppressedSenders).some((sender) => sender === selected)
      )) {
        void qc.invalidateQueries(
          { queryKey: ['conversation', agentId, activeSenderRef.current], refetchType: 'active' },
          { cancelRefetch: false },
        );
      }
      pendingSenders.clear();
      pendingEventKinds.clear();
      pendingEvents.clear();
    };

    const queueInvalidation = (event?: InboxRealtimeEvent) => {
      if (!refreshData) return;
      const sender = normalizeInboxEventSender(event?.sender || event?.number || event?.chat);
      if (sender) pendingSenders.add(sender);
      const kind = event?.kind || '';
      const normalizedKind = kind || 'unknown';
      pendingEventKinds.set(normalizedKind, (pendingEventKinds.get(normalizedKind) || 0) + 1);
      const messageID = (event?.message_id || '').trim();
      const eventKey = `${sender}\u0000${normalizedKind}\u0000${messageID}`;
      const pendingEvent = pendingEvents.get(eventKey);
      if (pendingEvent) pendingEvent.count += 1;
      else pendingEvents.set(eventKey, { sender, kind: normalizedKind, messageID, count: 1 });
      if (['incoming', 'read', 'state', 'reset', 'delete', 'revoke'].includes(kind)) {
        refreshUnreadSummary = true;
      }
      const slowSyncEvent = ['history', 'history_sync', 'state', 'message'].includes(kind);
      const delay = slowSyncEvent ? INBOX_SYNC_INVALIDATION_MS : INBOX_LIVE_INVALIDATION_MS;
      const dueAt = Date.now() + delay;
      if (!invalidateTimer || dueAt < invalidateDueAt) {
        if (invalidateTimer) window.clearTimeout(invalidateTimer);
        invalidateDueAt = dueAt;
        // History sync dapat menghasilkan ratusan event. Throttle per batch,
        // tetapi event live tetap boleh mempercepat refresh yang sudah terjadwal.
        invalidateTimer = window.setTimeout(flushInvalidations, delay);
      }
    };

    const notifyIncoming = (event: InboxRealtimeEvent): boolean => {
      const waMessageID = event.message_id?.trim();
      const identity = waMessageID
        ? `wa:${waMessageID}`
        : event.id
          ? `local:${event.id}`
          : event.revision
            ? `revision:${event.revision}`
            : '';
      if (identity && handledIncoming.has(identity)) return false;
      if (identity) {
        handledIncoming.add(identity);
        if (handledIncoming.size > 512) {
          const oldest = handledIncoming.values().next().value;
          if (oldest) handledIncoming.delete(oldest);
        }
      }
      onIncomingRef.current?.(event);
      return true;
    };

    const waitForRetry = (delay: number) => new Promise<void>((resolve) => {
      let timer = 0;
      const finish = () => {
        if (timer) window.clearTimeout(timer);
        controller.signal.removeEventListener('abort', finish);
        resolve();
      };
      timer = window.setTimeout(finish, delay);
      controller.signal.addEventListener('abort', finish, { once: true });
      if (controller.signal.aborted) finish();
    });

    const consumeStream = async () => {
      let retryDelay = 1_000;
      let lastRevision = 0;
      while (!controller.signal.aborted) {
        try {
          const since = lastRevision > 0 ? `?since=${encodeURIComponent(String(lastRevision))}` : '';
          const response = await fetch(`/api/agents/${agentId}/inbox/events${since}`, {
            method: 'GET',
            headers: {
              Accept: 'text/event-stream',
              Authorization: `Bearer ${token}`,
            },
            cache: 'no-store',
            signal: controller.signal,
          });
          if (response.status === 401 || response.status === 403) return;
          if (!response.ok || !response.body) {
            throw new Error(`Inbox event stream unavailable (${response.status})`);
          }

          retryDelay = 1_000;
          const reader = response.body.getReader();
          const decoder = new TextDecoder();
          let buffer = '';

          while (!controller.signal.aborted) {
            const { done, value } = await reader.read();
            if (done) break;
            buffer += decoder.decode(value, { stream: true });
            buffer = buffer.replace(/\r\n/g, '\n');

            let boundary = buffer.indexOf('\n\n');
            while (boundary >= 0) {
              const frame = buffer.slice(0, boundary);
              buffer = buffer.slice(boundary + 2);
              const payload = frame
                .split('\n')
                .filter((line) => line.startsWith('data:'))
                .map((line) => line.slice(5).trimStart())
                .join('\n')
                .trim();
              if (payload && payload !== '[DONE]') {
                try {
                  const event = JSON.parse(payload) as InboxRealtimeEvent;
                  const revision = Number(event.revision) || 0;
                  if (event.kind === 'ready') {
                    // Backend yang restart memulai revision dari awal.
                    if (revision < lastRevision) lastRevision = revision;
                    else lastRevision = Math.max(lastRevision, revision);
                    boundary = buffer.indexOf('\n\n');
                    continue;
                  }
                  if (revision > 0 && revision <= lastRevision) {
                    boundary = buffer.indexOf('\n\n');
                    continue;
                  }
                  if (revision > 0) lastRevision = revision;
                  onEventRef.current?.(event);
                  if (event.kind === 'typing') {
                    boundary = buffer.indexOf('\n\n');
                    continue;
                  }
                  queueInvalidation(event);
                  if (event.kind === 'incoming') {
                    notifyIncoming(event);
                  }
                } catch {
                  // Heartbeat/non-JSON tidak mengubah data.
                }
              }
              boundary = buffer.indexOf('\n\n');
            }
          }
        } catch (error) {
          if (controller.signal.aborted) return;
          // Polling tetap berjalan; stream dicoba kembali dengan backoff.
          if (error instanceof DOMException && error.name === 'AbortError') return;
        }
        await waitForRetry(retryDelay);
        retryDelay = Math.min(retryDelay * 2, 30_000);
      }
    };

    // Fallback berbasis cursor database. Tidak memakai unread karena unread dapat
    // kembali nol sebelum polling ketika chat aktif atau AI langsung membalas.
    const cursorStorageKey = `chatloop_incoming_cursor_${agentId}`;
    const savedCursor = sessionStorage.getItem(cursorStorageKey);
    let incomingCursor = Number(savedCursor || 0);
    let incomingCursorReady = savedCursor !== null
      && Number.isSafeInteger(incomingCursor)
      && incomingCursor >= 0;
    if (!Number.isSafeInteger(incomingCursor) || incomingCursor < 0) {
      incomingCursor = 0;
      incomingCursorReady = false;
    }

    const pollIncomingCursor = async () => {
      if (controller.signal.aborted || incomingPollRunning) return;
      incomingPollRunning = true;
      let nextDelay = 2_500;
      try {
        const suffix = incomingCursorReady
          ? `?after_id=${encodeURIComponent(String(incomingCursor))}`
          : '';
        const response = await fetch(`/api/agents/${agentId}/inbox/incoming-cursor${suffix}`, {
          method: 'GET',
          headers: {
            Accept: 'application/json',
            Authorization: `Bearer ${token}`,
          },
          cache: 'no-store',
          signal: controller.signal,
        });
        if (response.status === 401 || response.status === 403) {
          incomingPollStopped = true;
          return;
        }
        if (!response.ok) throw new Error(`Inbox cursor unavailable (${response.status})`);
        const payload = await response.json() as {
          data?: {
            cursor?: number;
            has_more?: boolean;
            events?: InboxRealtimeEvent[];
          };
        };
        const data = payload.data;
        if (data) {
          if (incomingCursorReady) {
            for (const event of data.events || []) {
              const incomingEvent: InboxRealtimeEvent = {
                ...event,
                agent_id: Number(event.agent_id) || agentId,
                kind: event.kind || 'incoming',
              };
              if (notifyIncoming(incomingEvent)) {
                onEventRef.current?.(incomingEvent);
                queueInvalidation(incomingEvent);
              }
            }
          }
          const cursor = Number(data.cursor);
          if (Number.isSafeInteger(cursor) && cursor >= 0) {
            incomingCursor = cursor;
            incomingCursorReady = true;
            sessionStorage.setItem(cursorStorageKey, String(cursor));
          }
          if (data.has_more) nextDelay = 50;
        }
      } catch (error) {
        if (controller.signal.aborted) return;
        if (error instanceof DOMException && error.name === 'AbortError') return;
        nextDelay = 5_000;
      } finally {
        incomingPollRunning = false;
        if (!controller.signal.aborted && !incomingPollStopped) {
          incomingPollTimer = window.setTimeout(() => {
            void pollIncomingCursor();
          }, nextDelay);
        }
      }
    };

    void consumeStream();
    void pollIncomingCursor();
    return () => {
      controller.abort();
      if (invalidateTimer) window.clearTimeout(invalidateTimer);
      if (incomingPollTimer) window.clearTimeout(incomingPollTimer);
    };
  }, [agentId, qc, refreshData]);
}

function chatMessageIdentity(message: ChatMsg): string {
  const waMessageID = (message.wa_msg_id || '').trim();
  return waMessageID ? `wa:${waMessageID}` : `local:${message.id}`;
}

function contactPreviewFromMessage(message: ChatMsg): string {
  let preview = (message.message || message.reply || '').trim();
  if (!preview && message.media_type) preview = `[${message.media_type}]`;
  preview = preview.replace(/\s+/g, ' ');
  const chars = Array.from(preview);
  return chars.length > 64 ? `${chars.slice(0, 64).join('')}…` : preview;
}

// Semua endpoint kirim mengembalikan ChatHistory kanonik. Masukkan row langsung
// ke cache thread dan preview kontak; refetch hanya fallback untuk backend lama
// atau kegagalan pencatatan. Echo/receipt terdekat ditahan singkat supaya satu
// balasan tidak menghasilkan request dan render kedua.
function cacheConversationMessages(
  qc: QueryClient,
  agentId: number,
  rawMessages: ChatMsg[],
  source: 'ticket' | 'composer',
  manualPauseUntil?: string | null,
): boolean {
  const cacheStartedAt = window.performance.now();
  const messages = rawMessages.filter((message) => (
    Boolean(message.sender) && Boolean(message.wa_msg_id || message.id)
  ));
  if (messages.length === 0) return false;
  for (const message of messages) {
    markConversationLocallyCached(agentId, message.sender, message.wa_msg_id);
  }
  for (const sender of new Set(messages.map((message) => normalizeInboxEventSender(message.sender)))) {
    markConversationBriefStale(qc, agentId, sender);
  }

  let scannedQueries = 0;
  let matchedQueries = 0;
  let scannedMessages = 0;

  qc.setQueriesData<ConversationPage>(
    { queryKey: ['conversation', agentId] },
    (conversation) => {
      if (!conversation) return conversation;
      scannedQueries += 1;
      scannedMessages += conversation.data.length;
      const sender = normalizeInboxEventSender(conversation.sender);
      const incoming = messages.filter(
        (message) => normalizeInboxEventSender(message.sender) === sender,
      );
      if (incoming.length === 0) return conversation;
      matchedQueries += 1;

      const next = [...conversation.data];
      const indexes = new Map<string, number>();
      next.forEach((message, index) => {
        indexes.set(chatMessageIdentity(message), index);
      });
      for (const message of incoming) {
        const identity = chatMessageIdentity(message);
        const existingIndex = indexes.get(identity);
        if (existingIndex === undefined) {
          indexes.set(identity, next.length);
          next.push(message);
        } else {
          next[existingIndex] = { ...next[existingIndex], ...message };
        }
      }
      next.sort((left, right) => {
        const leftTime = Date.parse(left.created_at) || 0;
        const rightTime = Date.parse(right.created_at) || 0;
        return leftTime - rightTime || left.id - right.id;
      });
      return {
        ...conversation,
        data: next,
        loaded_count: next.length,
        manual_pause_until: manualPauseUntil === undefined
          ? conversation.manual_pause_until
          : manualPauseUntil,
      };
    },
  );

  const latestBySender = new Map<string, ChatMsg>();
  for (const message of messages) {
    const normalizedSender = normalizeInboxEventSender(message.sender);
    const previous = latestBySender.get(normalizedSender);
    if (!previous || (Date.parse(message.created_at) || 0) >= (Date.parse(previous.created_at) || 0)) {
      latestBySender.set(normalizedSender, message);
    }
  }
  qc.setQueryData<Contact[]>(['contacts', agentId], (current) => {
    if (!current) return current;
    let changed = false;
    const next = current.map((contact) => {
      const message = latestBySender.get(normalizeInboxEventSender(contact.sender));
      if (!message) return contact;
      changed = true;
      return {
        ...contact,
        last_at: message.created_at,
        last_msg: contactPreviewFromMessage(message),
        preview_stale: false,
        manual_pause_until: manualPauseUntil === undefined
          ? contact.manual_pause_until
          : manualPauseUntil,
      };
    });
    if (!changed) return current;
    return next.sort((left, right) => {
      const byTime = (Date.parse(right.last_at) || 0) - (Date.parse(left.last_at) || 0);
      return byTime || left.sender.localeCompare(right.sender);
    });
  });

  inboxDebugLog(`${source}.cache-update`, {
    duration_ms: Math.round(window.performance.now() - cacheStartedAt),
    response_messages: messages.length,
    scanned_queries: scannedQueries,
    matched_queries: matchedQueries,
    scanned_messages: scannedMessages,
  });
  return true;
}

export function useHistorySyncStatus(agentId: number) {
  return useQuery<HistorySyncStatus>({
    queryKey: ['history-sync', agentId],
    queryFn: async ({ signal }) => (
      await api.get(`/agents/${agentId}/history-sync`, { signal })
    ).data.data,
    enabled: !!agentId,
    // Mutation sync mengaktifkan query lewat invalidasi. Sesudah selesai tidak
    // perlu polling idle. Saat proses panjang, turunkan frekuensi bertahap agar
    // status sekunder ini tidak mengganggu input/chat utama.
    refetchInterval: (query) => {
      const status = query.state.data;
      if (status?.state !== 'syncing') return false;
      const startedAt = status.started_at ? Date.parse(status.started_at) : Number.NaN;
      const age = Number.isFinite(startedAt) ? Math.max(0, Date.now() - startedAt) : 0;
      if (age < 15_000) return 1_500;
      if (age < 60_000) return 3_000;
      return 7_500;
    },
    refetchIntervalInBackground: false,
    refetchOnWindowFocus: true,
    staleTime: 1_000,
    notifyOnChangeProps: ['data'],
  });
}

/** Preview OpenGraph satu URL untuk Inbox (cache 10 menit di backend + client). */
export function useLinkPreview(agentId: number, url: string, enabled: boolean) {
  return useQuery<LinkPreview>({
    queryKey: ['link-preview', agentId, url],
    queryFn: async ({ signal }) =>
      (await api.get(`/agents/${agentId}/link-preview`, { params: { url }, signal, timeout: 12_000 })).data.data,
    enabled: enabled && !!agentId && !!url,
    staleTime: 10 * 60_000,
    gcTime: 30 * 60_000,
    refetchOnWindowFocus: false,
    retry: 1,
  });
}

export function useMarkConversationRead(agentId: number) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (sender: string) =>
      (await api.post(`/agents/${agentId}/conversation/read`, { sender })).data,
    // Optimis: hilangkan badge unread segera tanpa refetch penuh daftar kontak.
    onMutate: async (sender: string) => {
      await qc.cancelQueries({ queryKey: ['contacts', agentId] });
      const previous = qc.getQueryData<Contact[]>(['contacts', agentId]);
      qc.setQueryData<Contact[]>(['contacts', agentId], (prev) =>
        (prev || []).map((c) => (c.sender === sender ? { ...c, unread_count: 0 } : c)),
      );
      return { previous };
    },
    onError: (_err, _sender, ctx) => {
      if (ctx?.previous) qc.setQueryData(['contacts', agentId], ctx.previous);
    },
  });
}

export function useRequestHistorySync(agentId: number) {
  const qc = useQueryClient();
  return useMutation({
    // deep=true: tarik sedalam data di HP (paginate + full history), bukan cuma 50 pesan.
    mutationFn: async (sender: string) =>
      (await api.post(`/agents/${agentId}/history-sync`, { sender, deep: true })).data as {
        message?: string;
        data?: HistorySyncStatus;
      },
    onSuccess: (result) => {
      // Endpoint mengembalikan status reservasi terkini. Pakai langsung; SSE dan
      // effect finished_at akan menyegarkan chat ketika hasil benar-benar siap.
      // Refetch kontak/thread pada detik pertama hanya mengunduh data lama.
      if (result.data) qc.setQueryData(['history-sync', agentId], result.data);
    },
  });
}

/** Hapus seluruh thread chat satu kontak dari inbox (riwayat + handoff + memory). */
export function useDeleteInboxConversation(agentId: number) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (sender: string) =>
      (await api.delete(`/agents/${agentId}/conversation`, { params: { sender } })).data as {
        message: string;
        sender: string;
        deleted_chats: number;
      },
    onSuccess: (_data, sender) => {
      qc.setQueryData<Contact[]>(['contacts', agentId], (prev) =>
        (prev || []).filter((c) => c.sender !== sender),
      );
      qc.removeQueries({ queryKey: ['conversation', agentId, sender] });
      qc.removeQueries({ queryKey: ['conversation-brief', agentId, sender] });
      qc.invalidateQueries({ queryKey: ['contacts', agentId] });
      qc.invalidateQueries({ queryKey: ['handoffs', agentId] });
    },
  });
}

/** Bersihkan seluruh state Inbox lokal saat data akun WhatsApp sudah tercampur. */
export function useResetAgentInbox(agentId: number) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async () =>
      (await api.post(`/agents/${agentId}/inbox/reset`, { confirm: 'RESET INBOX' })).data as {
        message: string;
        deleted_chats: number;
        deleted_media: number;
      },
    onSuccess: () => {
      qc.setQueryData<Contact[]>(['contacts', agentId], []);
      qc.removeQueries({ queryKey: ['conversation', agentId] });
      qc.removeQueries({ queryKey: ['conversation-brief', agentId] });
      qc.invalidateQueries({ queryKey: ['contacts', agentId] });
      qc.invalidateQueries({ queryKey: ['handoffs', agentId] });
      qc.invalidateQueries({ queryKey: ['history-sync', agentId] });
    },
  });
}

/** Ringkasan CS: fakta penting, open items, risiko — cache di server, auto-rebuild bila stale banyak. */
export function useConversationBrief(agentId: number, sender: string, enabled = true) {
  return useQuery<ConversationBrief>({
    queryKey: ['conversation-brief', agentId, sender],
    queryFn: async ({ signal }) => (
      await api.get(`/agents/${agentId}/conversation/brief`, {
        params: { sender },
        signal,
      })
    ).data.data,
    enabled: enabled && !!agentId && !!sender,
    staleTime: 90_000,
    // Setelah ada chat baru, brief lokal ditandai stale dan dibangun ulang oleh
    // endpoint heuristik ringan maksimal beberapa detik kemudian. Kondisi normal
    // tetap hemat request. AI penuh hanya dijalankan lewat tombol refresh.
    refetchInterval: (query) => query.state.data?.stale ? 4_000 : 120_000,
    refetchIntervalInBackground: false,
    // Brief sekunder: boleh datang belakangan, jangan warisi brief chat lain.
    gcTime: 10 * 60_000,
  });
}

export function useRefreshConversationBrief(agentId: number) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (sender: string) =>
      (await api.post(`/agents/${agentId}/conversation/brief`, { sender })).data.data as ConversationBrief,
    onSuccess: (data, sender) => {
      qc.setQueryData(['conversation-brief', agentId, sender], data);
    },
  });
}

export function useSendMessage(agentId: number) {
  const qc = useQueryClient();
  return useMutation<InboxSendResult, Error, {
    to: string;
    message: string;
    reply_to?: string;
    reply_text?: string;
  }>({
    mutationFn: async (body: { to: string; message: string; reply_to?: string; reply_text?: string }) =>
      (await api.post(`/agents/${agentId}/send`, body, {
        timeout: INBOX_TEXT_SEND_TIMEOUT_MS,
      })).data as InboxSendResult,
    onSuccess: (result, vars) => {
      const cached = Boolean(result.message) && cacheConversationMessages(
        qc,
        agentId,
        result.message ? [result.message] : [],
        'composer',
        result.manual_pause_until,
      );
      if (!cached) {
        void qc.invalidateQueries({ queryKey: ['conversation', agentId, vars.to] });
        void qc.invalidateQueries({ queryKey: ['contacts', agentId] });
      }
      markConversationBriefStale(qc, agentId, vars.to);
    },
  });
}

export function useSendMedia(agentId: number) {
  const qc = useQueryClient();
  return useMutation<InboxSendResult, Error, { to: string; file: File; caption: string }>({
    mutationFn: async ({ to, file, caption }: { to: string; file: File; caption: string }) => {
      const fd = new FormData();
      fd.append('to', to);
      fd.append('caption', caption);
      fd.append('file', file);
      return (await api.post(`/agents/${agentId}/send-media`, fd, {
        timeout: INBOX_MEDIA_SEND_TIMEOUT_MS,
      })).data as InboxSendResult;
    },
    onSuccess: (result, vars) => {
      const cached = Boolean(result.message) && cacheConversationMessages(
        qc,
        agentId,
        result.message ? [result.message] : [],
        'composer',
        result.manual_pause_until,
      );
      if (!cached) {
        void qc.invalidateQueries({ queryKey: ['conversation', agentId, vars.to] });
        void qc.invalidateQueries({ queryKey: ['contacts', agentId] });
      }
      markConversationBriefStale(qc, agentId, vars.to);
    },
  });
}


export function useRevokeMessage(agentId: number) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async ({ msgId, to }: { msgId: string; to: string }) =>
      (await api.delete('/agents/' + agentId + '/messages/' + msgId, { data: { to } })).data,
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['conversation', agentId] });
    },
  });
}

/** Fire-and-forget presence typing — tanpa useMutation agar tidak re-render UI tiap keystroke. */
export function postAgentTyping(agentId: number, to: string, active: boolean, signal?: AbortSignal) {
  return api.post(`/agents/${agentId}/typing`, { to, active }, {
    signal,
    timeout: INBOX_TYPING_TIMEOUT_MS,
  }).catch(() => undefined);
}

/** @deprecated Prefer postAgentTyping agar tidak memicu re-render composer. */
export function useSendTyping(agentId: number) {
  return useMutation({
    mutationFn: async ({ to, active }: { to: string; active: boolean }) =>
      postAgentTyping(agentId, to, active),
  });
}

export function useResumeBot(agentId: number) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (sender: string) => (await api.delete(`/agents/${agentId}/handoffs/${sender}`)).data,
    onSuccess: (_d, sender) => {
      qc.invalidateQueries({ queryKey: ['conversation', agentId, sender] });
      qc.invalidateQueries({ queryKey: ['conversation-brief', agentId, sender] });
      qc.invalidateQueries({ queryKey: ['contacts', agentId] });
    },
  });
}

export function useReanalyzeImage(agentId: number) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async ({ messageId, instruction }: { messageId: number; instruction: string }) =>
      (await api.post(`/agents/${agentId}/messages/${messageId}/analyze`, { instruction })).data,
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['conversation', agentId] });
      qc.invalidateQueries({ queryKey: ['contacts', agentId] });
    },
  });
}

// ---- Broadcast ----

const LIVE_BROADCAST_STATUSES = new Set(['pending', 'running', 'resuming', 'cancel_requested']);

export function useBroadcastConsentSummary(agentId: number) {
  return useQuery<BroadcastConsentSummary>({
    queryKey: ['broadcast-consent-summary', agentId],
    queryFn: async () => (await api.get(`/agents/${agentId}/broadcast/consent-summary`)).data.data,
    enabled: !!agentId,
    staleTime: 30_000,
  });
}

export function useBroadcasts(agentId: number, page: number) {
  return useQuery<{ data: Broadcast[]; total: number; page: number; limit: number }>({
    queryKey: ['broadcasts', agentId, page],
    queryFn: async () => (await api.get(`/agents/${agentId}/broadcasts`, { params: { page } })).data,
    enabled: !!agentId,
    // Respons cepat ketika ada worker aktif, lebih hemat request saat riwayat diam.
    refetchInterval: query => query.state.data?.data.some(b => LIVE_BROADCAST_STATUSES.has(b.status)) ? 2000 : 10000,
    refetchIntervalInBackground: false,
  });
}

export function useChatContacts(agentId: number) {
  return useMutation({
    mutationFn: async () => (await api.get(`/agents/${agentId}/chat-contacts`)).data.data as { number: string; name: string }[],
  });
}

export function useWAContacts(agentId: number) {
  return useMutation({
    mutationFn: async () => (await api.get(`/agents/${agentId}/wa-contacts`)).data.data as ContactList,
  });
}

export function useGroups(agentId: number) {
  return useMutation({ mutationFn: async () => (await api.get(`/agents/${agentId}/groups`)).data.data as WAGroup[] });
}

// useCheckNumbers memvalidasi apakah nomor terdaftar di WhatsApp (pra-blast).
export interface CheckNumbersResult {
  results: Record<string, boolean>;
  registered: string[];
  not_registered: string[];
  total: number;
  registered_count: number;
}
export function useCheckNumbers(agentId: number) {
  return useMutation({
    mutationFn: async (numbers: string[]) =>
      (await api.post(`/agents/${agentId}/check-numbers`, { numbers })).data.data as CheckNumbersResult,
  });
}

// useManagedGroups = query daftar grup (auto-load) untuk halaman Anti-Spam Grup.
export function useManagedGroups(agentId: number, enabled = true) {
  return useQuery({
    queryKey: ['managed-groups', agentId],
    queryFn: async () => (await api.get(`/agents/${agentId}/groups`)).data.data as WAGroup[],
    enabled,
    retry: false,
  });
}

export function useGroupConfig(agentId: number, gjid: string, enabled = true) {
  return useQuery({
    queryKey: ['group-config', agentId, gjid],
    queryFn: async () => (await api.get(`/agents/${agentId}/group-config`, { params: { gjid } })).data.data as GroupGuardConfig,
    enabled: enabled && !!gjid,
    retry: false,
  });
}

export function useSaveGroupConfig(agentId: number) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (body: GroupGuardConfig) => (await api.put(`/agents/${agentId}/group-config`, body)).data.data as GroupGuardConfig,
    onSuccess: (_d, b) => {
      qc.invalidateQueries({ queryKey: ['group-config', agentId, b.group_jid] });
      qc.invalidateQueries({ queryKey: ['managed-groups', agentId] });
    },
  });
}

export function useGroupModeration(agentId: number, enabled = true) {
  return useQuery({
    queryKey: ['group-moderation', agentId],
    queryFn: async () => (await api.get(`/agents/${agentId}/group-moderation`)).data.data as GroupModerationLog[],
    enabled,
    retry: false,
  });
}

export function useConfirmKick(agentId: number) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (logid: number) => (await api.post(`/agents/${agentId}/group-moderation/${logid}/confirm-kick`)).data,
    onSuccess: () => qc.invalidateQueries({ queryKey: ['group-moderation', agentId] }),
  });
}

export function useDismissModeration(agentId: number) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (logid: number) => (await api.post(`/agents/${agentId}/group-moderation/${logid}/dismiss`)).data,
    onSuccess: () => qc.invalidateQueries({ queryKey: ['group-moderation', agentId] }),
  });
}

export function useGroupMembers(agentId: number) {
  return useMutation({ mutationFn: async (jid: string) => (await api.get(`/agents/${agentId}/group-members`, { params: { jid } })).data.data as ContactList });
}

export function useLabels(agentId: number) {
  return useMutation({ mutationFn: async () => (await api.post(`/agents/${agentId}/labels/sync`)).data.data as LabelInfo[] });
}

export function useLabelContacts(agentId: number) {
  return useMutation({ mutationFn: async (labelId: string) => (await api.get(`/agents/${agentId}/label-contacts`, { params: { label_id: labelId } })).data.data as ContactList });
}

// ---- Auto-reply (kata kunci) ----

export function useAutoReplies(agentId: number) {
  return useQuery<AutoReply[]>({
    queryKey: ['autoreplies', agentId],
    queryFn: async () => (await api.get(`/agents/${agentId}/auto-replies`)).data.data,
    enabled: !!agentId,
  });
}

export function useSaveAutoReply(agentId: number) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (r: Partial<AutoReply>) =>
      r.id
        ? (await api.put(`/agents/${agentId}/auto-replies/${r.id}`, r)).data
        : (await api.post(`/agents/${agentId}/auto-replies`, r)).data,
    onSuccess: () => qc.invalidateQueries({ queryKey: ['autoreplies', agentId] }),
  });
}

export function useDeleteAutoReply(agentId: number) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (rid: number) => (await api.delete(`/agents/${agentId}/auto-replies/${rid}`)).data,
    onSuccess: () => qc.invalidateQueries({ queryKey: ['autoreplies', agentId] }),
  });
}

// ---- Template pesan (quick reply) ----

export function useTemplates(agentId: number) {
  return useQuery<Template[]>({
    queryKey: ['templates', agentId],
    queryFn: async () => (await api.get(`/agents/${agentId}/templates`)).data.data,
    enabled: !!agentId,
  });
}

export function useSaveTemplate(agentId: number) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async ({ template, file, removeMedia }: {
      template: Partial<Template>;
      file?: File | null;
      removeMedia?: boolean;
    }) => {
      const form = new FormData();
      form.append('title', template.title || '');
      form.append('body', template.body || '');
      form.append('sort_order', String(template.sort_order || 0));
      if (file) form.append('file', file);
      if (removeMedia) form.append('remove_media', 'true');
      return template.id
        ? (await api.put(`/agents/${agentId}/templates/${template.id}`, form)).data
        : (await api.post(`/agents/${agentId}/templates`, form)).data;
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: ['templates', agentId] }),
  });
}

export function useDeleteTemplate(agentId: number) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (tid: number) => (await api.delete(`/agents/${agentId}/templates/${tid}`)).data,
    onSuccess: () => qc.invalidateQueries({ queryKey: ['templates', agentId] }),
  });
}

// ---- Kontak (CRM ringan) ----

export function useCrmContacts(agentId: number, q: string, tag: string, stage: LeadStage | '', page: number) {
  return useQuery<SavedContactsResp>({
    queryKey: ['crm-contacts', agentId, q, tag, stage, page],
    queryFn: async () => (await api.get(`/agents/${agentId}/crm/contacts`, { params: { q, tag, stage, page } })).data,
    enabled: !!agentId,
    refetchInterval: 15000,
  });
}

export function useSaveCrmContact(agentId: number) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (ct: Partial<SavedContact>) =>
      ct.id
        ? (await api.put(`/agents/${agentId}/crm/contacts/${ct.id}`, ct)).data
        : (await api.post(`/agents/${agentId}/crm/contacts`, ct)).data,
    onSuccess: () => qc.invalidateQueries({ queryKey: ['crm-contacts', agentId] }),
  });
}

export function useDeleteCrmContact(agentId: number) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (cid: number) => (await api.delete(`/agents/${agentId}/crm/contacts/${cid}`)).data,
    onSuccess: () => qc.invalidateQueries({ queryKey: ['crm-contacts', agentId] }),
  });
}

// useCrmContactsExport mengambil SEMUA kontak hasil filter (tanpa paginasi),
// dipakai untuk menjadikan satu tag jadi target broadcast.
export function useCrmContactsExport(agentId: number) {
  return useMutation({
    mutationFn: async ({ q, tag, stage }: { q: string; tag: string; stage: LeadStage | '' }) =>
      (await api.get(`/agents/${agentId}/crm/contacts`, { params: { q, tag, stage, all: 1 } })).data.data as SavedContact[],
  });
}

export function useBulkStageCrmContacts(agentId: number) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (body: { ids: number[]; lead_stage: LeadStage }) =>
      (await api.post(`/agents/${agentId}/crm/contacts/bulk-stage`, body)).data as { updated: number },
    onSuccess: () => qc.invalidateQueries({ queryKey: ['crm-contacts', agentId] }),
  });
}

// useImportCrmContacts memasukkan banyak kontak sekaligus (manual/terkoneksi/CSV).
export function useImportCrmContacts(agentId: number) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (body: { contacts: { number: string; name: string }[]; tag?: string }) =>
      (await api.post(`/agents/${agentId}/crm/contacts/import`, body)).data as { imported: number; skipped: number },
    onSuccess: () => qc.invalidateQueries({ queryKey: ['crm-contacts', agentId] }),
  });
}

// useBulkDeleteCrmContacts menghapus kontak terpilih (ids) atau semua sesuai filter.
export function useBulkDeleteCrmContacts(agentId: number) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (body: { ids?: number[]; all?: boolean; q?: string; tag?: string; stage?: LeadStage | '' }) =>
      (await api.post(`/agents/${agentId}/crm/contacts/bulk-delete`, body)).data as { deleted: number },
    onSuccess: () => qc.invalidateQueries({ queryKey: ['crm-contacts', agentId] }),
  });
}

// ---- Follow-up (drip) ----

export function useFollowUps(agentId: number) {
  return useQuery<FollowUp[]>({
    queryKey: ['follow-ups', agentId],
    queryFn: async () => (await api.get(`/agents/${agentId}/follow-ups`)).data.data,
    enabled: !!agentId,
    staleTime: 5_000,
    refetchInterval: 15_000,
    refetchIntervalInBackground: false,
  });
}

export function useSaveFollowUp(agentId: number) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (fu: Partial<FollowUp>) =>
      fu.id
        ? (await api.put(`/agents/${agentId}/follow-ups/${fu.id}`, fu)).data
        : (await api.post(`/agents/${agentId}/follow-ups`, fu)).data,
    onSuccess: () => qc.invalidateQueries({ queryKey: ['follow-ups', agentId] }),
  });
}

export function useDeleteFollowUp(agentId: number) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (fid: number) => (await api.delete(`/agents/${agentId}/follow-ups/${fid}`)).data,
    onSuccess: () => qc.invalidateQueries({ queryKey: ['follow-ups', agentId] }),
  });
}

export function useEnrollFollowUp(agentId: number) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async ({ fid, recipients }: { fid: number; recipients: { number: string; name: string }[] }) =>
      (await api.post(`/agents/${agentId}/follow-ups/${fid}/enroll`, { recipients })).data as {
        added: number;
        skipped: number;
        details?: {
          invalid: number;
          duplicate: number;
          opted_out: number;
          already_active: number;
          failed: number;
        };
      },
    onSuccess: () => qc.invalidateQueries({ queryKey: ['follow-ups', agentId] }),
  });
}

// ---- Produk ----

export function useProducts(agentId: number) {
  return useQuery<Product[]>({
    queryKey: ['products', agentId],
    queryFn: async () => (await api.get(`/agents/${agentId}/products`)).data.data,
    enabled: !!agentId,
  });
}

export function useSaveProduct(agentId: number) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async ({ id, fd }: { id?: number; fd: FormData }) => {
      if (id) return (await api.put(`/agents/${agentId}/products/${id}`, fd)).data;
      return (await api.post(`/agents/${agentId}/products`, fd)).data;
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['products', agentId] });
      qc.invalidateQueries({ queryKey: ['product-orders', agentId] });
    },
  });
}

export function useGenerateProductAI(agentId: number) {
  return useMutation({
    mutationFn: async (body: {
      name: string;
      product_type: string;
      price: string;
      description: string;
      details_json: string;
      existing_knowledge: string;
      checkout_enabled: boolean;
    }) => (await api.post(`/agents/${agentId}/products/generate-ai`, body)).data as {
      knowledge: string;
      ai_sales_guidance: string;
    },
  });
}

export function useDeleteProduct(agentId: number) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (id: number) => (await api.delete(`/agents/${agentId}/products/${id}`)).data,
    onSuccess: () => { qc.invalidateQueries({ queryKey: ['products', agentId] }); },
  });
}

export function useSendProduct(agentId: number) {
  return useMutation({
    mutationFn: async ({ pid, to }: { pid: number; to: string }) =>
      (await api.post(`/agents/${agentId}/products/${pid}/send`, { to })).data,
  });
}

export function useProductOrders(agentId: number) {
  return useQuery<ProductOrder[]>({
    queryKey: ['product-orders', agentId],
    queryFn: async () => (await api.get(`/agents/${agentId}/product-orders`)).data.data,
    enabled: !!agentId,
    refetchInterval: 15000,
  });
}

export function useAIForms(agentId: number, enabled = true) {
  return useQuery<AIForm[]>({
    queryKey: ['ai-forms', agentId],
    queryFn: async () => (await api.get(`/agents/${agentId}/ai-forms`)).data.data,
    enabled: !!agentId && enabled,
  });
}

export function useSaveAIForm(agentId: number) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (form: Partial<AIForm>) =>
      form.id
        ? (await api.put(`/agents/${agentId}/ai-forms/${form.id}`, form)).data
        : (await api.post(`/agents/${agentId}/ai-forms`, form)).data,
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['ai-forms', agentId] });
      qc.invalidateQueries({ queryKey: ['ai-form-submissions', agentId] });
    },
  });
}

export function useDeleteAIForm(agentId: number) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (id: number) => (await api.delete(`/agents/${agentId}/ai-forms/${id}`)).data,
    onSuccess: () => qc.invalidateQueries({ queryKey: ['ai-forms', agentId] }),
  });
}

export function useAIFormSubmissions(agentId: number, enabled = true) {
  return useQuery<AIFormSubmission[]>({
    queryKey: ['ai-form-submissions', agentId],
    queryFn: async () => (await api.get(`/agents/${agentId}/ai-form-submissions`)).data.data,
    enabled: !!agentId && enabled,
    refetchInterval: enabled ? 15000 : false,
  });
}

// ---- Jadwal ----

export function useSchedules(agentId: number) {
  return useQuery<ScheduledMessage[]>({
    queryKey: ['schedules', agentId],
    queryFn: async () => (await api.get(`/agents/${agentId}/schedules`)).data.data,
    enabled: !!agentId,
    refetchInterval: 10000,
  });
}

export function useCreateSchedule(agentId: number) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (fd: FormData) => (await api.post(`/agents/${agentId}/schedule`, fd)).data,
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['schedules', agentId] });
      qc.invalidateQueries({ queryKey: ['broadcast-consent-summary', agentId] });
    },
  });
}

export function useCancelSchedule(agentId: number) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (sid: number) => (await api.delete(`/agents/${agentId}/schedule/${sid}`)).data,
    onSuccess: () => qc.invalidateQueries({ queryKey: ['schedules', agentId] }),
  });
}

export function useBroadcastDetail(agentId: number, bid: number | null) {
  return useQuery<BroadcastDetailData>({
    queryKey: ['broadcast', agentId, bid],
    queryFn: async () => (await api.get(`/agents/${agentId}/broadcasts/${bid}`)).data.data,
    enabled: !!agentId && !!bid,
    refetchInterval: query => LIVE_BROADCAST_STATUSES.has(query.state.data?.broadcast.status || '') ? 1500 : false,
    refetchIntervalInBackground: false,
  });
}

export function useCreateBroadcast(agentId: number) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (body: { message: string; recipients: { number: string; name: string }[]; min_delay: number; max_delay: number; rest_every: number; rest_duration: number; file: File | null; safety: BroadcastSafetyForm; agent_ids?: number[]; product_id?: number }) => {
      const fd = new FormData();
      fd.append('message', body.message);
      fd.append('recipients', JSON.stringify(body.recipients));
      fd.append('min_delay', String(body.min_delay));
      fd.append('max_delay', String(body.max_delay));
      fd.append('rest_every', String(body.rest_every));
      fd.append('rest_duration', String(body.rest_duration));
      Object.entries(body.safety).forEach(([key, value]) => fd.append(key, String(value)));
      if (body.file) fd.append('file', body.file);
      if (body.agent_ids && body.agent_ids.length) fd.append('agent_ids', JSON.stringify(body.agent_ids));
      if (body.product_id) fd.append('product_id', String(body.product_id));
      return (await api.post(`/agents/${agentId}/broadcast`, fd)).data;
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['broadcasts', agentId] });
      qc.invalidateQueries({ queryKey: ['broadcast-consent-summary', agentId] });
    },
  });
}

export interface BroadcastRotationTestResult {
  pool_size: number;
  sample_size: number;
  failed_agent_id: number;
  reassigned: number;
  messages_sent: number;
  agents: Array<{
    id: number;
    name?: string;
    number?: string;
    initial_count: number;
    after_failover_count: number;
    simulated_failed: boolean;
  }>;
}

export function useTestBroadcastRotation(agentId: number) {
  return useMutation({
    mutationFn: async (agentIds: number[]) =>
      (await api.post(`/agents/${agentId}/broadcast/rotation-test`, { agent_ids: agentIds })).data.data as BroadcastRotationTestResult,
  });
}

export function useCancelBroadcast(agentId: number) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (bid: number) =>
      (await api.post(`/agents/${agentId}/broadcasts/${bid}/cancel`)).data,
    onSuccess: (_data, bid) => {
      qc.invalidateQueries({ queryKey: ['broadcasts', agentId] });
      qc.invalidateQueries({ queryKey: ['broadcast', agentId, bid] });
    },
  });
}

export function useResumeBroadcast(agentId: number) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (bid: number) =>
      (await api.post(`/agents/${agentId}/broadcasts/${bid}/resume`)).data,
    onSuccess: (_data, bid) => {
      qc.invalidateQueries({ queryKey: ['broadcasts', agentId] });
      qc.invalidateQueries({ queryKey: ['broadcast', agentId, bid] });
      qc.invalidateQueries({ queryKey: ['schedules', agentId] });
    },
  });
}

// ---- Agent list & detail (Dashboard) ----

export function useAgents() {
  return useQuery<Agent[]>({
    queryKey: ['agents'],
    queryFn: async () => (await api.get('/agents')).data.data,
    staleTime: 30_000,
  });
}

export function useAgentStatuses() {
  return useQuery<Record<string, string>>({
    queryKey: ['agent-statuses'],
    queryFn: async () => (await api.get('/agents-status')).data.data,
    refetchInterval: 4000,
  });
}

export function useAgentUnreadSummary() {
  return useQuery<AgentUnreadSummary>({
    queryKey: ['agent-unread-summary'],
    queryFn: async () => (await api.get('/agent-unread-summary')).data,
    refetchInterval: 5_000,
    refetchIntervalInBackground: true,
    refetchOnWindowFocus: 'always',
    staleTime: 2_000,
  });
}

export function useTeamUsers(enabled = true) {
  return useQuery<TeamUser[]>({
    queryKey: ['team-users'],
    queryFn: async () => (await api.get('/team/users')).data.data,
    enabled,
  });
}

export function useSaveTeamUser() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (input: Partial<TeamUser> & { password?: string }) =>
      input.id
        ? (await api.put(`/team/users/${input.id}`, input)).data
        : (await api.post('/team/users', input)).data,
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['team-users'] });
      qc.invalidateQueries({ queryKey: ['agents'] });
    },
  });
}

export function useDeleteTeamUser() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (userId: number) => (await api.delete(`/team/users/${userId}`)).data,
    onSuccess: () => qc.invalidateQueries({ queryKey: ['team-users'] }),
  });
}

export function useCSActivity(page = 1, limit = 20, enabled = true) {
  return useQuery<CSActivityResp>({
    queryKey: ['cs-activity', page, limit],
    queryFn: async () => (await api.get('/team/activity', { params: { page, limit } })).data,
    enabled,
    refetchInterval: enabled ? 10_000 : false,
    placeholderData: prev => prev,
  });
}

export function useAgentStatus(agentId: number) {
  return useQuery<{ status: string; qr: string; qr_ttl: number; pair_code: string; pair_error: string; number: string; name: string }>({
    queryKey: ['agent', agentId, 'status'],
    queryFn: async () => (await api.get(`/agents/${agentId}/wa/status`)).data,
    enabled: !!agentId,
    refetchInterval: 4000,
    refetchIntervalInBackground: true,
    refetchOnWindowFocus: 'always',
  });
}

export function useAgentHistory(agentId: number) {
  return useQuery<unknown[]>({
    queryKey: ['agent', agentId, 'history'],
    queryFn: async () => (await api.get(`/agents/${agentId}/chat-history`)).data.data,
    enabled: !!agentId,
    refetchInterval: 4000,
  });
}

export function useAgentKnowledge(agentId: number, enabled = true) {
  return useQuery<KnowledgeItem[]>({
    queryKey: ['agent', agentId, 'knowledge'],
    queryFn: async () => (await api.get(`/agents/${agentId}/knowledge`)).data.data,
    enabled: !!agentId && enabled,
    refetchInterval: enabled ? 4000 : false,
  });
}

export function useAgentHandoffs(agentId: number) {
  return useQuery<Handoff[]>({
    queryKey: ['agent', agentId, 'handoffs'],
    queryFn: async () => (await api.get(`/agents/${agentId}/handoffs`)).data.data,
    enabled: !!agentId,
    refetchInterval: 4000,
  });
}

// ---- Mutasi agent (Dashboard) ----

export function useSaveAgent(agentId: number) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (body: Record<string, unknown>) =>
      (await api.put(`/agents/${agentId}`, body)).data,
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['agents'] });
      qc.invalidateQueries({ queryKey: ['agent', agentId] });
    },
  });
}

export function useCreateAgent() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (body: { name: string; tone: string }) =>
      (await api.post('/agents', body)).data,
    onSuccess: () => qc.invalidateQueries({ queryKey: ['agents'] }),
  });
}

export function useDeleteAgent() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (id: number) => (await api.delete(`/agents/${id}`)).data,
    onSuccess: () => qc.invalidateQueries({ queryKey: ['agents'] }),
  });
}

export function useAgentConnect(agentId: number) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async () => (await api.post(`/agents/${agentId}/wa/connect`)).data,
    onSuccess: () => qc.invalidateQueries({ queryKey: ['agent', agentId, 'status'] }),
  });
}

export function useAgentDisconnect(agentId: number) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async () => (await api.post(`/agents/${agentId}/wa/logout`)).data,
    onSuccess: () => qc.invalidateQueries({ queryKey: ['agent', agentId, 'status'] }),
  });
}

export function useAddKnowledge(agentId: number) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (body: { question: string; answer: string; tags: string; image?: File | null } | FormData) => {
      let payload: unknown = body;
      if (!(body instanceof FormData) && body.image) {
        const fd = new FormData();
        fd.append('question', body.question);
        fd.append('answer', body.answer);
        fd.append('tags', body.tags);
        fd.append('image', body.image);
        payload = fd;
      }
      return (await api.post(`/agents/${agentId}/knowledge`, payload)).data as { data: KnowledgeItem; merged: boolean };
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['agent', agentId, 'knowledge'] });
      qc.invalidateQueries({ queryKey: ['agent', agentId, 'knowledge-usage'] });
    },
  });
}

export function useDeleteKnowledge(agentId: number) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (id: number) => (await api.delete(`/agents/${agentId}/knowledge/${id}`)).data,
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['agent', agentId, 'knowledge'] });
      qc.invalidateQueries({ queryKey: ['agent', agentId, 'knowledge-usage'] });
    },
  });
}

export function useUpdateKnowledge(agentId: number) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (body: { id: number; question: string; answer: string; tags: string; active?: boolean; priority?: number; image?: File | null; remove_image?: boolean } | FormData) => {
      let payload: unknown = body;
      let id: number;
      if (body instanceof FormData) {
        id = Number(body.get('id'));
      } else {
        id = body.id;
        if (body.image || body.remove_image) {
          const fd = new FormData();
          fd.append('question', body.question);
          fd.append('answer', body.answer);
          fd.append('tags', body.tags);
          if (body.active !== undefined) fd.append('active', String(body.active));
          if (body.priority !== undefined) fd.append('priority', String(body.priority));
          if (body.remove_image) fd.append('remove_image', 'true');
          if (body.image) fd.append('image', body.image);
          payload = fd;
        }
      }
      return (await api.put(`/agents/${agentId}/knowledge/${id}`, payload)).data;
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['agent', agentId, 'knowledge'] });
      qc.invalidateQueries({ queryKey: ['agent', agentId, 'knowledge-usage'] });
    },
  });
}

export function useDeleteAllKnowledge(agentId: number) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async () => (await api.delete(`/agents/${agentId}/knowledge-all`)).data,
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['agent', agentId, 'knowledge'] });
      qc.invalidateQueries({ queryKey: ['agent', agentId, 'knowledge-usage'] });
    },
  });
}

export function useGenerateKnowledge(agentId: number) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (body: { text: string; count: number }) =>
      (await api.post(`/agents/${agentId}/knowledge/generate`, body)).data as { data: KnowledgeItem[]; knowledge: number },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['agent', agentId, 'knowledge'] });
      qc.invalidateQueries({ queryKey: ['agent', agentId, 'knowledge-usage'] });
      qc.invalidateQueries({ queryKey: ['agents'] });
    },
  });
}

// ---- Latih dari Website (crawl) ----

export function useCrawlStatus(agentId: number, enabled = true) {
  return useQuery<{ job: CrawlJob | null; pages: CrawlPage[] }>({
    queryKey: ['agent', agentId, 'crawl'],
    queryFn: async () => (await api.get(`/agents/${agentId}/crawl`)).data,
    enabled: !!agentId && enabled,
    // Polling cepat selagi crawl/pelatihan berjalan (termasuk saat dihentikan), berhenti saat idle/selesai.
    refetchInterval: (q) => {
      const s = q.state.data?.job?.status;
      return s === 'pending' || s === 'crawling' || s === 'training' || s === 'stopping' ? 2500 : false;
    },
  });
}

export function useKnowledgeUsage(agentId: number, enabled = true) {
  return useQuery<KnowledgeUsage>({
    queryKey: ['agent', agentId, 'knowledge-usage'],
    queryFn: async () => (await api.get(`/agents/${agentId}/knowledge-usage`)).data,
    enabled: !!agentId && enabled,
  });
}

export function useStartCrawl(agentId: number) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (url: string) => (await api.post(`/agents/${agentId}/crawl`, { url })).data,
    onSuccess: () => qc.invalidateQueries({ queryKey: ['agent', agentId, 'crawl'] }),
  });
}

export function useTrainCrawlPages(agentId: number) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (vars: { jobId: number; pageIds: number[]; updatePersona: boolean }) =>
      (await api.post(`/agents/${agentId}/crawl/${vars.jobId}/train`, { page_ids: vars.pageIds, update_persona: vars.updatePersona })).data,
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['agent', agentId, 'crawl'] });
      qc.invalidateQueries({ queryKey: ['agent', agentId, 'knowledge'] });
      qc.invalidateQueries({ queryKey: ['agent', agentId, 'knowledge-usage'] });
    },
  });
}

export function useStopTraining(agentId: number) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (jobId: number) =>
      (await api.post(`/agents/${agentId}/crawl/${jobId}/train/stop`)).data,
    onSuccess: () => qc.invalidateQueries({ queryKey: ['agent', agentId, 'crawl'] }),
  });
}

export function useRegeneratePersona(agentId: number) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async () =>
      (await api.post(`/agents/${agentId}/persona/regenerate`)).data as { system_prompt: string },
    onSuccess: () => qc.invalidateQueries({ queryKey: ['agents'] }),
  });
}

export function useResumeHandoff(agentId: number) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (sender: string) => (await api.delete(`/agents/${agentId}/handoffs/${sender}`)).data,
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['agent', agentId, 'handoffs'] });
      qc.invalidateQueries({ queryKey: ['conversation', agentId] });
      qc.invalidateQueries({ queryKey: ['contacts', agentId] });
    },
  });
}

export function useTestChat(agentId: number) {
  return useMutation({
    mutationFn: async (vars: { message: string; history: { role: 'user' | 'bot'; text: string }[] }) =>
      (await api.post(`/agents/${agentId}/test-chat`, vars)).data as {
        reply: string; escalate: boolean; model?: string; image_url?: string;
      },
  });
}

// ---- Flow (alur/menu) ----

export function useApiSettings(agentId: number) {
  return useQuery<ApiSettings>({
    queryKey: ['api-settings', agentId],
    queryFn: async () => (await api.get(`/agents/${agentId}/api`)).data.data,
    enabled: !!agentId,
  });
}

export function useRotateApiKey(agentId: number) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async () => (await api.post(`/agents/${agentId}/api/key`)).data as { api_key: string },
    onSuccess: () => qc.invalidateQueries({ queryKey: ['api-settings', agentId] }),
  });
}

export function useRevokeApiKey(agentId: number) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async () => (await api.delete(`/agents/${agentId}/api/key`)).data,
    onSuccess: () => qc.invalidateQueries({ queryKey: ['api-settings', agentId] }),
  });
}

export function useSaveWebhook(agentId: number) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (webhook_url: string) =>
      (await api.put(`/agents/${agentId}/api/webhook`, { webhook_url })).data as { webhook_secret?: string },
    onSuccess: () => qc.invalidateQueries({ queryKey: ['api-settings', agentId] }),
  });
}

export function useRotateWebhookSecret(agentId: number) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async () => (await api.post(`/agents/${agentId}/api/webhook-secret`)).data as { webhook_secret: string },
    onSuccess: () => qc.invalidateQueries({ queryKey: ['api-settings', agentId] }),
  });
}

export function useTestWebhook(agentId: number) {
  return useMutation({
    mutationFn: async () =>
      (await api.post(`/agents/${agentId}/api/webhook/test`)).data as { status: string; http_status: number },
  });
}

/** Uji kirim lewat jalur REST API (JWT dashboard, tanpa mengekspos API key). */
export function useTestApiMessage(agentId: number) {
  return useMutation({
    mutationFn: async (payload: { to: string; text?: string }) =>
      (await api.post(`/agents/${agentId}/api/test-message`, payload)).data as {
        status: string;
        to: string;
        type: string;
        message_id?: string;
        note?: string;
      },
  });
}

export function useFlow(agentId: number) {
  return useQuery<Flow>({
    queryKey: ['flow', agentId],
    queryFn: async () => (await api.get(`/agents/${agentId}/flow`)).data.data,
    enabled: !!agentId,
  });
}

export function useSaveFlow(agentId: number) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (f: Partial<Flow>) => (await api.post(`/agents/${agentId}/flow`, f)).data,
    onSuccess: () => qc.invalidateQueries({ queryKey: ['flow', agentId] }),
  });
}

// ---- Status / Story ----

export function useStatuses(agentId: number) {
  return useQuery<ScheduledStatus[]>({
    queryKey: ['statuses', agentId],
    queryFn: async () => (await api.get(`/agents/${agentId}/statuses`)).data.data,
    enabled: !!agentId,
  });
}

export function useCreateStatus(agentId: number) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (fd: FormData) => (await api.post(`/agents/${agentId}/status`, fd)).data,
    onSuccess: () => qc.invalidateQueries({ queryKey: ['statuses', agentId] }),
  });
}

export function useCancelStatus(agentId: number) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (sid: number) => (await api.delete(`/agents/${agentId}/status/${sid}`)).data,
    onSuccess: () => qc.invalidateQueries({ queryKey: ['statuses', agentId] }),
  });
}

// ---- Pairing ----

export function useAgentConnectPairing(agentId: number) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (phone: string) => (await api.post(`/agents/${agentId}/wa/connect-pairing`, { phone })).data as { status: string },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['agent', agentId, 'status'] });
      qc.invalidateQueries({ queryKey: ['agent-statuses'] });
    },
  });
}

// ---- Usage ----

export function useUsage() {
  return useQuery<{ tenant: { id: number; name: string }; numbers_used: number; max_numbers: number }>({
    queryKey: ['usage'],
    queryFn: async () => (await api.get('/usage')).data,
  });
}
