import {
  Fragment, memo, useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState,
  type ReactNode, type RefObject, type UIEvent,
} from 'react';
import {
  Box, Typography, TextField, IconButton, Stack, Chip, Button, CircularProgress,
  Avatar, Dialog, DialogTitle, DialogContent, DialogActions, Alert, Collapse,
  Tooltip, InputAdornment, useMediaQuery, Popover,
} from '@mui/material';
import { useTheme, alpha, styled } from '@mui/material/styles';
import SendIcon from '@mui/icons-material/Send';
import SmartToyIcon from '@mui/icons-material/SmartToy';
import TaskAltIcon from '@mui/icons-material/TaskAlt';
import AttachFileIcon from '@mui/icons-material/AttachFile';
import InsertDriveFileOutlinedIcon from '@mui/icons-material/InsertDriveFileOutlined';
import CloseIcon from '@mui/icons-material/Close';
import DeleteIcon from '@mui/icons-material/Delete';
import ReplyIcon from '@mui/icons-material/Reply';
import AutoAwesomeIcon from '@mui/icons-material/AutoAwesome';
import RefreshIcon from '@mui/icons-material/Refresh';
import ExpandMoreIcon from '@mui/icons-material/ExpandMore';
import ExpandLessIcon from '@mui/icons-material/ExpandLess';
import SearchIcon from '@mui/icons-material/Search';
import ArrowBackIcon from '@mui/icons-material/ArrowBack';
import InsertEmoticonIcon from '@mui/icons-material/InsertEmoticon';
import ContentCopyIcon from '@mui/icons-material/ContentCopy';
import PhoneOutlinedIcon from '@mui/icons-material/PhoneOutlined';
import InfoOutlinedIcon from '@mui/icons-material/InfoOutlined';
import SyncIcon from '@mui/icons-material/Sync';
import DoneAllIcon from '@mui/icons-material/DoneAll';
import DoneIcon from '@mui/icons-material/Done';
import GroupsOutlinedIcon from '@mui/icons-material/GroupsOutlined';
import LabelOutlinedIcon from '@mui/icons-material/LabelOutlined';
import WifiIcon from '@mui/icons-material/Wifi';
import WifiOffIcon from '@mui/icons-material/WifiOff';
import VolumeUpOutlinedIcon from '@mui/icons-material/VolumeUpOutlined';
import VolumeOffOutlinedIcon from '@mui/icons-material/VolumeOffOutlined';
import PersonOutlineOutlinedIcon from '@mui/icons-material/PersonOutlineOutlined';
import {
  useContacts, useConversation, useConversationBrief, useRefreshConversationBrief,
  useSendMessage, useSendMedia, postAgentTyping, useRevokeMessage, useResumeBot, useReanalyzeImage,
  useDeleteInboxConversation,
  useHistorySyncStatus, useRequestHistorySync,
  useMarkConversationRead,
  useLoadOlderConversation,
  useLabels,
  useAgentStatus,
  useLinkPreview,
} from '../hooks';
import TemplatePicker from './TemplatePicker';
import { swalConfirm, swalToast } from '../services/swal';
import {
  activateInboxDebugWindow,
  captureInboxDebugSnapshot,
  configureInboxDebugAgent,
  inboxDebugLog,
  sampleComposerInput,
  sampleInboxComponentCommit,
} from '../services/inboxDebug';
import type { ChatMsg, Contact, ConversationBrief, HistorySyncStatus } from '../types';

/* ─── WhatsApp Web palette ─────────────────────────────────────────────── */
const WA = {
  panel: '#ffffff',
  panelHeader: '#f0f2f5',
  listHover: '#f5f6f6',
  listActive: '#f0f2f5',
  chatBg: '#efeae2',
  bubbleIn: '#ffffff',
  bubbleOut: '#d9fdd3',
  bubbleOutCS: '#d9fdd3',
  green: '#00a884',
  greenDark: '#008069',
  meta: '#667781',
  border: '#e9edef',
  searchBg: '#f0f2f5',
  tick: '#53bdeb',
};

const CHAT_EMOJIS = [
  '😀', '😃', '😄', '😁', '😊', '🙂', '😉', '😍',
  '🥰', '😘', '😎', '🤗', '🤔', '😅', '😂', '🤣',
  '🙏', '👍', '👎', '👏', '🙌', '🤝', '👌', '💪',
  '❤️', '🧡', '💛', '💚', '💙', '💜', '🤍', '✨',
  '🎉', '🔥', '✅', '❌', '⚠️', '📌', '📦', '🚚',
  '💬', '📞', '💳', '💰', '🧾', '⏰', '📍', '🏠',
];

type InboxFilter = 'all' | 'unread' | 'read' | 'handoff' | 'groups';
type ContactSidePanel = 'details' | null;

const CONTACT_RENDER_BATCH = 80;
const EMPTY_REVOKED_MESSAGE_IDS: string[] = [];
const MAX_COMPOSER_FILE_BYTES = 64 * 1024 * 1024;
const MAX_COMPOSER_TEXT_LENGTH = 65_536;
const COMPOSER_TYPING_IDLE_MS = 2_800;
const COMPOSER_FILE_EXTENSIONS = new Set([
  'pdf', 'doc', 'docx', 'xls', 'xlsx', 'ppt', 'pptx', 'txt', 'zip',
]);
const COMPOSER_MEDIA_MIME_BY_EXTENSION: Record<string, string> = {
  jpg: 'image/jpeg',
  jpeg: 'image/jpeg',
  png: 'image/png',
  gif: 'image/gif',
  webp: 'image/webp',
  heic: 'image/heic',
  heif: 'image/heif',
  mp4: 'video/mp4',
  mov: 'video/quicktime',
  m4v: 'video/x-m4v',
  '3gp': 'video/3gpp',
  webm: 'video/webm',
};

// Native textarea dengan tinggi tetap sengaja dipakai untuk composer. MUI
// TextareaAutosize membaca getComputedStyle + scrollHeight textarea bayangan
// pada setiap render/karakter; pada thread panjang itu dapat memaksa layout
// seluruh Inbox berulang kali dan membuat main thread browser tersendat.
const ComposerTextarea = styled('textarea')({
  display: 'block',
  width: '100%',
  height: 42,
  minHeight: 42,
  maxHeight: 42,
  boxSizing: 'border-box',
  padding: '9px 4px',
  border: 0,
  outline: 0,
  resize: 'none',
  overflowY: 'auto',
  overscrollBehavior: 'contain',
  WebkitAppearance: 'none',
  WebkitTextFillColor: '#111b21',
  background: 'transparent',
  color: '#111b21',
  caretColor: '#008069',
  font: 'inherit',
  fontSize: 15,
  lineHeight: '22px',
  scrollbarWidth: 'thin',
  '&::placeholder': {
    color: '#667781',
    opacity: 1,
  },
  '&:disabled': {
    color: '#667781',
    cursor: 'not-allowed',
  },
});

type ComposerAttachment = {
  file: File;
  previewURL: string;
  kind: 'image' | 'video' | 'document';
};

function composerAttachmentKind(file: File): ComposerAttachment['kind'] {
  if (file.type.startsWith('image/')) return 'image';
  if (file.type.startsWith('video/')) return 'video';
  return 'document';
}

function composerFileExtension(file: File): string {
  return file.name.split('.').pop()?.toLowerCase() || '';
}

function normalizeComposerFile(file: File): File {
  if (file.type && file.type !== 'application/octet-stream') return file;
  const inferredType = COMPOSER_MEDIA_MIME_BY_EXTENSION[composerFileExtension(file)];
  if (!inferredType) return file;
  return new File([file], file.name, {
    type: inferredType,
    lastModified: file.lastModified,
  });
}

function composerFileAllowed(file: File): boolean {
  if (file.type.startsWith('image/') || file.type.startsWith('video/')) return true;
  const extension = composerFileExtension(file);
  return COMPOSER_FILE_EXTENSIONS.has(extension) || Boolean(COMPOSER_MEDIA_MIME_BY_EXTENSION[extension]);
}

function formatComposerFileSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${Math.ceil(bytes / 1024)} KB`;
  return `${(bytes / (1024 * 1024)).toFixed(bytes >= 10 * 1024 * 1024 ? 0 : 1)} MB`;
}

function composerSendErrorMessage(error: unknown, hasFile: boolean): string {
  const requestError = error as {
    code?: string;
    message?: string;
    response?: { status?: number; data?: { error?: string } };
  };
  const serverMessage = requestError.response?.data?.error;
  if (serverMessage) return serverMessage;
  if (
    requestError.code === 'ECONNABORTED'
    || requestError.code === 'ETIMEDOUT'
    || requestError.response?.status === 504
    || requestError.message?.toLowerCase().includes('timeout')
  ) {
    return 'Pengiriman terlalu lama. Composer sudah diaktifkan kembali; periksa chat sebelum mencoba ulang agar pesan tidak terkirim ganda.';
  }
  return hasFile
    ? 'Media belum berhasil dikirim. File dan caption tetap tersedia untuk dicoba kembali.'
    : 'Pesan belum berhasil dikirim. Teks tetap tersedia untuk dicoba kembali.';
}

type ConversationHistoryWindow = {
  sender: string;
  messages: ChatMsg[];
  nextBeforeAt?: string | null;
  nextBeforeID?: number;
  hasMore: boolean;
};

const labelPalette = ['#00a884', '#53bdeb', '#a855f7', '#f59e0b', '#ef4444', '#06b6d4', '#84cc16', '#ec4899'];
function waLabelColor(value?: number) {
  return labelPalette[Math.abs(value || 0) % labelPalette.length];
}

function HistorySyncNotice({
  status,
}: {
  status?: HistorySyncStatus;
}) {
  const [hiddenStatusKey, setHiddenStatusKey] = useState('');
  const syncing = status?.state === 'syncing';
  const failed = status?.state === 'failed';
  const stillStale = Boolean(status?.still_stale);
  const statusKey = [
    status?.state,
    status?.started_at,
    status?.finished_at,
    status?.message,
    status?.error,
    status?.imported,
    status?.still_stale,
  ].join('|');
  const hidden = Boolean(statusKey) && hiddenStatusKey === statusKey;

  // Banner adalah umpan balik singkat, bukan keadaan modal. Kegagalan diberi
  // waktu baca lebih panjang tetapi tetap hilang agar area chat tidak terasa
  // terkunci; percobaan sinkron berikutnya memiliki statusKey baru.
  useEffect(() => {
    if (!status || syncing) return undefined;
    const delay = failed ? 12_000 : stillStale ? 8_000 : 4_000;
    const t = window.setTimeout(() => setHiddenStatusKey(statusKey), delay);
    return () => window.clearTimeout(t);
  }, [failed, status, statusKey, stillStale, syncing]);

  if (!status || hidden) return null;
  if (status.state !== 'syncing' && status.state !== 'failed' && status.state !== 'completed') return null;

  let text: string;
  if (syncing) {
    const progress = status.progress > 0 ? ` ${Math.min(status.progress, 99)}%` : '';
    text = `Menyinkronkan…${progress}`;
    if (status.imported > 0) text += ` · ${status.imported} pesan`;
  } else if (failed) {
    text = status.error || status.message || 'Sinkronisasi gagal. Pastikan HP online.';
  } else if (stillStale) {
    text = status.message || 'Riwayat belum lengkap. Pastikan WhatsApp tersambung, buka chat di HP, lalu sinkronkan lagi.';
  } else if (status.imported > 0) {
    text = status.message || `${status.imported} pesan ditambahkan.`;
  } else {
    text = status.message || 'Chat sudah diperbarui.';
  }
  const warn = failed || (stillStale && !syncing);
  return (
    <Alert
      severity={failed ? 'error' : warn ? 'warning' : 'success'}
      icon={
        syncing
          ? (
            <CircularProgress
              size={16}
              thickness={4.5}
              sx={{ color: 'inherit', display: 'block' }}
            />
          )
          : undefined
      }
      onClose={syncing ? undefined : () => setHiddenStatusKey(statusKey)}
      sx={{
        py: 0,
        px: 1.5,
        minHeight: 34,
        borderRadius: 0,
        alignItems: 'center',
        bgcolor: failed ? '#fff4f4' : warn ? '#fff8e6' : '#f0f9f6',
        color: failed ? 'error.dark' : warn ? '#8a6116' : WA.greenDark,
        borderBottom: `1px solid ${failed ? alpha('#d32f2f', 0.18) : warn ? alpha('#ed6c02', 0.22) : alpha(WA.green, 0.18)}`,
        '& .MuiAlert-icon': {
          p: 0,
          m: 0,
          mr: 1,
          opacity: 1,
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'center',
          alignSelf: 'center',
          lineHeight: 0,
        },
        '& .MuiAlert-message': {
          py: 0.7,
          fontSize: 12.5,
          lineHeight: 1.35,
          display: 'flex',
          alignItems: 'center',
        },
        '& .MuiAlert-action': {
          pt: 0,
          alignItems: 'center',
        },
      }}
    >
      {text}
    </Alert>
  );
}

function WhatsAppConnectionNotice({ status }: { status?: string }) {
  if (!status || status === 'connected') return null;
  const reconnecting = status === 'connecting' || status === 'pairing';
  return (
    <Alert
      severity={reconnecting ? 'warning' : 'error'}
      icon={reconnecting ? <WifiIcon fontSize="small" /> : <WifiOffIcon fontSize="small" />}
      sx={{
        py: 0,
        px: 1.25,
        borderRadius: 0,
        borderBottom: `1px solid ${alpha(reconnecting ? '#ed6c02' : '#d32f2f', 0.2)}`,
        '& .MuiAlert-icon': { py: 0.8, mr: 0.8 },
        '& .MuiAlert-message': { py: 0.75, fontSize: 12, lineHeight: 1.35 },
      }}
    >
      {reconnecting
        ? 'WhatsApp sedang menyambung ulang. Pesan baru dapat terlambat; sinkronisasi tersedia setelah status online.'
        : 'WhatsApp tidak terhubung. Inbox tidak menerima pesan realtime dan data terakhir mungkin belum lengkap. Tautkan kembali di menu Koneksi WhatsApp.'}
    </Alert>
  );
}

function mediaTypeLabel(mediaType?: string) {
  if (mediaType === 'image') return 'foto';
  if (mediaType === 'sticker') return 'stiker';
  if (mediaType === 'video') return 'video';
  if (mediaType === 'audio') return 'audio';
  if (mediaType === 'document') return 'dokumen';
  return 'media';
}

function MissingMediaNotice({ m }: { m: ChatMsg }) {
  const label = mediaTypeLabel(m.media_type);
  const pending = m.media_fetch_status === 'pending';
  const failed = m.media_fetch_status === 'failed';
  const detail = pending
    ? `File ${label} tercatat dan sedang menunggu pengambilan dari WhatsApp.`
    : failed
      ? `File ${label} tercatat, tetapi belum berhasil diambil dari WhatsApp.`
      : `Pesan ${label} tercatat, tetapi filenya belum tersedia di server.`;
  return (
    <Box
      sx={{
        minWidth: 190,
        maxWidth: 240,
        px: 1.1,
        py: 0.85,
        borderRadius: 1,
        bgcolor: alpha(WA.meta, 0.07),
        border: `1px solid ${alpha(WA.meta, 0.13)}`,
      }}
    >
      <Typography sx={{ fontSize: 12.5, fontWeight: 650, color: '#3b4a54' }}>
        {m.media_type === 'sticker' ? '🌟 Stiker belum tersedia' : `📎 ${label.charAt(0).toUpperCase()}${label.slice(1)} belum tersedia`}
      </Typography>
      <Typography sx={{ mt: 0.2, fontSize: 11, lineHeight: 1.35, color: WA.meta }}>
        {detail} Pastikan WhatsApp online, lalu sinkronkan lagi.
      </Typography>
    </Box>
  );
}

function MediaView({ agentId, m, token }: { agentId: number; m: ChatMsg; token: string }) {
  const [zoom, setZoom] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);
  const [errorText, setErrorText] = useState('');
  const [imageURL, setImageURL] = useState('');
  const [retry, setRetry] = useState(0);
  const [inView, setInView] = useState(false);
  const hostRef = useRef<HTMLDivElement | null>(null);
  const url = `/api/agents/${agentId}/media/${m.id}?token=${token}`;
  const isVisual = m.media_type === 'image' || m.media_type === 'sticker';

  // Hanya unduh media yang terlihat di viewport — buka chat 200 pesan tidak membanjiri server.
  useEffect(() => {
    const el = hostRef.current;
    if (!el) return;
    if (typeof IntersectionObserver === 'undefined') {
      const fallback = window.setTimeout(() => setInView(true), 0);
      return () => window.clearTimeout(fallback);
    }
    const io = new IntersectionObserver(
      (entries) => {
        if (entries.some((e) => e.isIntersecting)) {
          setInView(true);
          io.disconnect();
        }
      },
      { rootMargin: '180px 0px' },
    );
    io.observe(el);
    return () => io.disconnect();
  }, []);

  useEffect(() => {
    if (!isVisual || !inView || !token) return;
    const controller = new AbortController();
    const timeout = window.setTimeout(() => controller.abort(), 15_000);
    let objectURL = '';
    let active = true;
    queueMicrotask(() => {
      if (!active) return;
      setLoading(true);
      setErrorText('');
      setImageURL('');
    });
    void fetch(url, { signal: controller.signal })
      .then(async response => {
        if (!response.ok) {
          const payload = await response.json().catch(() => ({})) as { error?: string };
          throw new Error(payload.error || (response.status === 503 ? 'WhatsApp belum terhubung.' : 'Media lama tidak tersedia.'));
        }
        return response.blob();
      })
      .then(blob => {
        if (!active) return;
        objectURL = URL.createObjectURL(blob);
        setImageURL(objectURL);
        setLoading(false);
      })
      .catch(error => {
        if (!active) return;
        const aborted = error instanceof DOMException && error.name === 'AbortError';
        setErrorText(aborted ? 'Pengambilan media terlalu lama. Silakan coba lagi.' : (error instanceof Error ? error.message : 'Media gagal dimuat.'));
        setLoading(false);
      })
      .finally(() => window.clearTimeout(timeout));
    return () => {
      active = false;
      controller.abort();
      window.clearTimeout(timeout);
      if (objectURL) URL.revokeObjectURL(objectURL);
    };
  }, [agentId, inView, isVisual, m.id, retry, token, url]);

  if (m.media_type === 'image' || m.media_type === 'sticker') {
    return (
      <>
        <Box
          ref={hostRef}
          sx={{ position: 'relative', width: { xs: 200, sm: 220 }, height: { xs: 150, sm: 165 }, borderRadius: 1, overflow: 'hidden', bgcolor: alpha(WA.meta, 0.08) }}
        >
          {imageURL && (
            <Box
              component="img"
              src={imageURL}
              alt="Media WhatsApp"
              onClick={() => setZoom(imageURL)}
              sx={{ width: '100%', height: '100%', objectFit: 'contain', borderRadius: 1, display: 'block', cursor: 'pointer' }}
            />
          )}
          {!token && (
            <Stack spacing={0.4} sx={{ position: 'absolute', inset: 0, px: 1.5, alignItems: 'center', justifyContent: 'center', textAlign: 'center', color: WA.meta }}>
              <Typography sx={{ fontSize: 12.5, fontWeight: 650 }}>Media belum dapat dibuka</Typography>
              <Typography sx={{ fontSize: 11 }}>Token media belum tersedia. Muat ulang Inbox.</Typography>
            </Stack>
          )}
          {token && (loading || (!inView && !imageURL && !errorText)) && (
            <Stack spacing={0.75} sx={{ position: 'absolute', inset: 0, alignItems: 'center', justifyContent: 'center', color: WA.meta }}>
              <CircularProgress size={22} sx={{ color: WA.green }} />
              <Typography sx={{ fontSize: 11.5 }}>{inView ? 'Mengambil media…' : 'Media…'}</Typography>
            </Stack>
          )}
          {errorText && (
            <Stack spacing={0.4} sx={{ width: 210, minHeight: 92, px: 1.5, py: 1.25, alignItems: 'center', justifyContent: 'center', textAlign: 'center' }}>
              <Typography sx={{ fontSize: 12.5, fontWeight: 600, color: WA.meta }}>Media belum bisa dimuat</Typography>
              <Typography sx={{ fontSize: 11, color: WA.meta }}>{errorText}</Typography>
              <Button size="small" startIcon={<RefreshIcon />} onClick={() => setRetry(value => value + 1)} sx={{ mt: 0.35, fontSize: 11.5 }}>Coba lagi</Button>
            </Stack>
          )}
        </Box>
        <Dialog open={!!zoom} onClose={() => setZoom(null)} maxWidth="md" onClick={() => setZoom(null)}>
          <Box component="img" src={zoom || ''} alt="" sx={{ maxWidth: '90vw', maxHeight: '85vh', display: 'block' }} />
        </Dialog>
      </>
    );
  }
  if (!token) return <MissingMediaNotice m={m} />;
  if (!inView) {
    const reservedHeight = m.media_type === 'video' ? 180 : m.media_type === 'audio' ? 54 : 48;
    return (
      <Box
        ref={hostRef}
        sx={{ width: 240, height: reservedHeight, borderRadius: 1, bgcolor: alpha(WA.meta, 0.06) }}
      />
    );
  }
  if (m.media_type === 'audio') {
    return (
      <Box ref={hostRef} sx={{ width: 240, height: 54, display: 'flex', alignItems: 'center', overflow: 'hidden' }}>
        <audio src={url} controls preload="none" style={{ width: 240, height: 40, display: 'block' }} />
      </Box>
    );
  }
  if (m.media_type === 'video') {
    return (
      <Box ref={hostRef} sx={{ width: 240, height: 180, borderRadius: 1, overflow: 'hidden', bgcolor: '#111' }}>
        <video
          src={url}
          controls
          preload="metadata"
          style={{ width: 240, height: 180, objectFit: 'contain', borderRadius: 8, display: 'block' }}
        />
      </Box>
    );
  }
  return (
    <Box ref={hostRef} sx={{ minWidth: 190, maxWidth: 240, minHeight: 48, display: 'flex', alignItems: 'center' }}>
      <a href={url} target="_blank" rel="noreferrer" style={{ color: 'inherit', fontSize: 13 }}>
        📎 {m.file_name || 'Unduh file'}
      </a>
    </Box>
  );
}

/** Zona waktu tampilan Inbox — selaras pengguna Indonesia / WA Web. */
const INBOX_TZ = 'Asia/Jakarta';

function parseMsgDate(ts?: string): Date | null {
  if (!ts) return null;
  const d = new Date(ts);
  if (Number.isNaN(d.getTime()) || d.getFullYear() < 2000) return null;
  return d;
}

/** YYYY-MM-DD di Asia/Jakarta (bukan timezone browser mentah). */
function calendarDayJakarta(d: Date): string {
  return new Intl.DateTimeFormat('en-CA', {
    timeZone: INBOX_TZ,
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
  }).format(d);
}

function fmtTime(ts?: string) {
  const d = parseMsgDate(ts);
  if (!d) return '';
  return d.toLocaleTimeString('id-ID', { hour: '2-digit', minute: '2-digit', timeZone: INBOX_TZ });
}

function fmtListTime(ts?: string) {
  const d = parseMsgDate(ts);
  if (!d) return '';
  const today = calendarDayJakarta(new Date());
  const key = calendarDayJakarta(d);
  if (key === today) {
    return d.toLocaleTimeString('id-ID', { hour: '2-digit', minute: '2-digit', timeZone: INBOX_TZ });
  }
  const yest = new Date();
  yest.setDate(yest.getDate() - 1);
  if (key === calendarDayJakarta(yest)) return 'Kemarin';
  const diffDays = (Date.now() - d.getTime()) / 86400000;
  if (diffDays < 7) {
    return d.toLocaleDateString('id-ID', { weekday: 'short', timeZone: INBOX_TZ });
  }
  return d.toLocaleDateString('id-ID', { day: '2-digit', month: 'short', timeZone: INBOX_TZ });
}

/** Kunci hari (YYYY-MM-DD, Asia/Jakarta) untuk memisah chip tanggal di thread. */
function dayKey(ts?: string): string {
  const d = parseMsgDate(ts);
  if (!d) return '';
  return calendarDayJakarta(d);
}

/** Label chip tanggal ala WhatsApp: "Hari ini" / "Kemarin" / "28 Juli 2026". */
function fmtDayChip(ts?: string): string {
  const d = parseMsgDate(ts);
  if (!d) return '';
  const key = calendarDayJakarta(d);
  const today = calendarDayJakarta(new Date());
  if (key === today) return 'Hari ini';
  const yest = new Date();
  yest.setDate(yest.getDate() - 1);
  if (key === calendarDayJakarta(yest)) return 'Kemarin';
  const sameYear = key.slice(0, 4) === today.slice(0, 4);
  return d.toLocaleDateString('id-ID', {
    day: 'numeric',
    month: 'long',
    timeZone: INBOX_TZ,
    ...(sameYear ? {} : { year: 'numeric' as const }),
  });
}

const historyMediaPrefix = /^(?:📷 Foto|🎥 Video|🎵 Audio|🌟 Stiker|📄 Dokumen|Pesan) dari riwayat WhatsApp\s*/u;

function cleanHistoryMediaText(value?: string) {
  return (value || '').replace(historyMediaPrefix, '').trim();
}

function mediaPreviewLabel(m: Pick<ChatMsg, 'message' | 'media_type' | 'file_name' | 'reply'>) {
  const message = cleanHistoryMediaText(m.message);
  const reply = cleanHistoryMediaText(m.reply);
  if (message) return message;
  if (reply) return reply;
  if (m.media_type === 'image') return '📷 Foto';
  if (m.media_type === 'sticker') return '🌟 Stiker';
  if (m.media_type === 'video') return '🎥 Video';
  if (m.media_type === 'audio') return '🎵 Audio';
  if (m.media_type === 'document') return `📄 ${m.file_name || 'Dokumen'}`;
  return 'Pesan';
}

function avatarColor(seed: string) {
  const colors = ['#00a884', '#53bdeb', '#a855f7', '#f59e0b', '#ef4444', '#06b6d4', '#84cc16', '#ec4899'];
  let h = 0;
  for (let i = 0; i < seed.length; i++) h = (h * 31 + seed.charCodeAt(i)) >>> 0;
  return colors[h % colors.length];
}

function profilePictureURL(agentId: number, sender: string, token: string) {
  if (!agentId || !sender || !token) return undefined;
  return `/api/agents/${agentId}/profile-picture?sender=${encodeURIComponent(sender)}&token=${encodeURIComponent(token)}`;
}

/** Avatar foto profil: baru fetch saat baris masuk viewport (hemat bandwidth + WA API). */
const LazyContactAvatar = memo(function LazyContactAvatar({
  agentId, sender, mediaToken, label, isGroup,
}: {
  agentId: number;
  sender: string;
  mediaToken: string;
  label: string;
  isGroup: boolean;
}) {
  const ref = useRef<HTMLDivElement | null>(null);
  const [active, setActive] = useState(false);
  useEffect(() => {
    if (isGroup || active) return;
    const el = ref.current;
    if (!el) return;
    if (typeof IntersectionObserver === 'undefined') {
      const fallback = window.setTimeout(() => setActive(true), 0);
      return () => window.clearTimeout(fallback);
    }
    const io = new IntersectionObserver(
      (entries) => {
        if (entries.some((e) => e.isIntersecting)) {
          setActive(true);
          io.disconnect();
        }
      },
      { rootMargin: '120px 0px' },
    );
    io.observe(el);
    return () => io.disconnect();
  }, [active, isGroup]);
  const initial = label.charAt(0).toUpperCase();
  return (
    <Avatar
      ref={ref}
      src={active && !isGroup ? profilePictureURL(agentId, sender, mediaToken) : undefined}
      slotProps={{ img: { loading: 'lazy', decoding: 'async' } }}
      sx={{
        width: 49,
        height: 49,
        fontSize: 18,
        fontWeight: 600,
        bgcolor: avatarColor(sender),
        color: '#fff',
        flexShrink: 0,
      }}
    >
      {isGroup ? <GroupsOutlinedIcon sx={{ fontSize: 23 }} /> : initial}
    </Avatar>
  );
});

/* ─── Bubble (memo) ─────────────────────────────────────────────────────── */

type ReplyPreview = {
  id: string;
  text: string;
  mediaType?: string;
  localId?: number;
  mediaDownloadable?: boolean;
  /** true bila pesan yang di-quote adalah pesan kita (CS/bot) → label "Anda". */
  fromHuman?: boolean;
};

/** Thumbnail mini untuk quote reply media — lazy, tidak memblok UI. */
const ReplyThumb = memo(function ReplyThumb({
  agentId, messageId, token, mediaType,
}: {
  agentId: number;
  messageId: number;
  token: string;
  mediaType?: string;
}) {
  const [src, setSrc] = useState('');
  useEffect(() => {
    if (!token || !messageId) return;
    if (mediaType !== 'image' && mediaType !== 'sticker' && mediaType !== 'video') return;
    const controller = new AbortController();
    let objectURL = '';
    void fetch(`/api/agents/${agentId}/media/${messageId}?token=${encodeURIComponent(token)}`, {
      signal: controller.signal,
    })
      .then((r) => (r.ok ? r.blob() : Promise.reject()))
      .then((blob) => {
        objectURL = URL.createObjectURL(blob);
        setSrc(objectURL);
      })
      .catch(() => undefined);
    return () => {
      controller.abort();
      if (objectURL) URL.revokeObjectURL(objectURL);
    };
  }, [agentId, mediaType, messageId, token]);

  const isVideo = mediaType === 'video';
  return (
    <Box
      sx={{
        width: 42,
        height: 42,
        borderRadius: 0.75,
        overflow: 'hidden',
        flexShrink: 0,
        bgcolor: alpha(WA.meta, 0.12),
        display: 'grid',
        placeItems: 'center',
        fontSize: 16,
      }}
    >
      {src ? (
        isVideo ? (
          <Box component="video" src={src} muted sx={{ width: '100%', height: '100%', objectFit: 'cover' }} />
        ) : (
          <Box component="img" src={src} alt="" sx={{ width: '100%', height: '100%', objectFit: 'cover', display: 'block' }} />
        )
      ) : (
        <span aria-hidden>
          {mediaType === 'video' ? '🎥' : mediaType === 'audio' ? '🎵' : mediaType === 'document' ? '📄' : mediaType === 'sticker' ? '🌟' : '📷'}
        </span>
      )}
    </Box>
  );
});

const Bubble = memo(function Bubble({
  side, tag, time, name, replyPreview, onReply, onOpenReply, children, deliveryStatus, agentId, mediaToken,
}: {
  side: 'left' | 'right';
  tag?: string;
  time?: string;
  name?: string;
  replyPreview?: ReplyPreview | null;
  onReply?: () => void;
  onOpenReply?: () => void;
  children: ReactNode;
  deliveryStatus?: string;
  agentId?: number;
  mediaToken?: string;
}) {
  const isLeft = side === 'left';
  const normalizedDelivery = (deliveryStatus || '').toLowerCase();
  const isRead = normalizedDelivery === 'read' || normalizedDelivery === 'read_inferred' || normalizedDelivery === 'played';
  const isSent = normalizedDelivery === 'sent' || normalizedDelivery === 'delivered' || isRead;
  const quoteText = replyPreview?.text || '';
  // Hanya label "Anda" bila mengutip pesan CS — tanpa nama kontak di bubble.
  const quoteLabel = replyPreview?.fromHuman ? 'Anda' : undefined;
  const showThumb = Boolean(
    replyPreview?.localId
    && replyPreview.mediaDownloadable
    && mediaToken
    && agentId
    && (replyPreview.mediaType === 'image' || replyPreview.mediaType === 'sticker' || replyPreview.mediaType === 'video'),
  );
  return (
    <Box
      sx={{
        alignSelf: isLeft ? 'flex-start' : 'flex-end',
        maxWidth: { xs: '90%', sm: '76%', md: 720 },
        display: 'flex',
        flexDirection: isLeft ? 'row' : 'row-reverse',
        alignItems: 'flex-end',
        gap: 0.5,
        '&:hover .reply-btn': { opacity: 1 },
      }}
    >
      <Box sx={{ position: 'relative', minWidth: 0 }}>
        {tag && (
          <Typography
            sx={{
              display: 'block',
              textAlign: isLeft ? 'left' : 'right',
              mb: 0.2,
              fontWeight: 600,
              fontSize: 10,
              color: tag === 'Bot' ? WA.greenDark : WA.meta,
              px: 0.25,
            }}
          >
            {tag === 'Bot' ? 'Asisten AI' : tag === 'CS' ? 'CS' : tag}
          </Typography>
        )}
        {name && isLeft && !tag && (
          <Typography
            sx={{
              display: 'block',
              textAlign: 'left',
              mb: 0.15,
              fontWeight: 600,
              fontSize: 12.5,
              color: avatarColor(name),
              px: 0.25,
            }}
          >
            {name}
          </Typography>
        )}
        <Box
          sx={{
            px: 1.1,
            pt: 0.55,
            pb: 0.35,
            borderRadius: '7.5px',
            borderTopLeftRadius: isLeft ? '0' : '7.5px',
            borderTopRightRadius: isLeft ? '7.5px' : '0',
            bgcolor: isLeft ? WA.bubbleIn : WA.bubbleOut,
            color: '#111b21',
            whiteSpace: 'pre-wrap',
            wordBreak: 'break-word',
            fontSize: '0.925rem',
            lineHeight: 1.4,
            boxShadow: '0 1px 0.5px rgba(11,20,26,0.13)',
          }}
        >
          {replyPreview && quoteText && (
            <Box
              role={onOpenReply ? 'button' : undefined}
              tabIndex={onOpenReply ? 0 : undefined}
              onClick={(event) => {
                event.stopPropagation();
                onOpenReply?.();
              }}
              onKeyDown={event => {
                if (onOpenReply && (event.key === 'Enter' || event.key === ' ')) {
                  event.preventDefault();
                  onOpenReply();
                }
              }}
              sx={{
                borderLeft: '4px solid',
                borderColor: isLeft ? WA.green : '#06cf9c',
                pl: 0.75,
                pr: 0.45,
                py: 0.35,
                mb: 0.5,
                bgcolor: isLeft ? 'rgba(0,0,0,0.04)' : 'rgba(0,0,0,0.05)',
                borderRadius: '0 4px 4px 0',
                fontSize: '0.8rem',
                lineHeight: 1.3,
                color: WA.meta,
                maxHeight: 56,
                overflow: 'hidden',
                cursor: onOpenReply ? 'pointer' : 'default',
                transition: 'background-color 0.15s',
                display: 'flex',
                alignItems: 'center',
                gap: 0.75,
                '&:hover': onOpenReply ? { bgcolor: 'rgba(0,0,0,0.09)' } : undefined,
              }}
            >
              {showThumb && replyPreview.localId && agentId && mediaToken ? (
                <ReplyThumb
                  agentId={agentId}
                  messageId={replyPreview.localId}
                  token={mediaToken}
                  mediaType={replyPreview.mediaType}
                />
              ) : null}
              <Box sx={{ flex: 1, minWidth: 0 }}>
                {quoteLabel && (
                  <Typography
                    component="div"
                    sx={{
                      fontSize: '0.78rem',
                      fontWeight: 700,
                      lineHeight: 1.25,
                      color: isLeft ? WA.greenDark : '#027a55',
                      mb: 0.1,
                    }}
                  >
                    {quoteLabel}
                  </Typography>
                )}
                <Typography
                  component="div"
                  noWrap={!showThumb}
                  sx={{
                    fontSize: '0.8rem',
                    lineHeight: 1.3,
                    color: WA.meta,
                    display: '-webkit-box',
                    WebkitLineClamp: quoteLabel ? 2 : 2,
                    WebkitBoxOrient: 'vertical',
                    overflow: 'hidden',
                    whiteSpace: showThumb ? 'normal' : undefined,
                  }}
                >
                  {quoteText}
                </Typography>
              </Box>
            </Box>
          )}
          {children}
          <Stack
            direction="row"
            spacing={0.4}
            sx={{ width: '100%', justifyContent: 'flex-end', alignItems: 'center', mt: 0.15, ml: 'auto', minHeight: 16, gap: 0.35 }}
          >
            {time && (
              <Typography component="span" sx={{ fontSize: 11, color: WA.meta, lineHeight: 1, userSelect: 'none' }}>
                {time}
              </Typography>
            )}
            {!isLeft && deliveryStatus && (
              isSent ? (
                <DoneAllIcon
                  aria-label={isRead ? 'Pesan sudah dibaca' : 'Pesan sudah terkirim'}
                  sx={{ width: 16, height: 16, color: isRead ? WA.tick : WA.meta, ml: -0.1, flexShrink: 0 }}
                />
              ) : (
                <DoneIcon
                  aria-label={normalizedDelivery === 'failed_send' ? 'Pesan gagal dikirim' : 'Pesan sudah dikirim'}
                  sx={{
                    width: 16,
                    height: 16,
                    color: normalizedDelivery === 'failed_send' ? 'error.main' : WA.meta,
                    ml: -0.1,
                    flexShrink: 0,
                  }}
                />
              )
            )}
          </Stack>
          <IconButton
            size="small"
            className="reply-btn"
            onClick={onReply}
            aria-label="Balas"
            sx={{
              position: 'absolute', top: 3,
              right: isLeft ? -24 : 'auto', left: isLeft ? 'auto' : -24,
              opacity: 0, transition: 'opacity 0.12s', p: 0.15, width: 20, height: 20,
            }}
          >
            <ReplyIcon sx={{ fontSize: 14, color: WA.meta }} />
          </IconButton>
        </Box>
      </Box>
      {/* name unused visually in WA web — keep for a11y */}
      {name ? <Box component="span" sx={{ display: 'none' }}>{name}</Box> : null}
    </Box>
  );
});

function TypingIndicator() {
  return (
    <Box
      sx={{
        alignSelf: 'flex-start',
        display: 'flex',
        alignItems: 'center',
        gap: 0.5,
        px: 1.5,
        py: 1.1,
        bgcolor: WA.bubbleIn,
        borderRadius: '7.5px',
        borderTopLeftRadius: 0,
        boxShadow: '0 1px 0.5px rgba(11,20,26,0.13)',
        maxWidth: 72,
      }}
    >
      {[0, 1, 2].map((i) => (
        <Box
          key={i}
          sx={{
            width: 7,
            height: 7,
            borderRadius: '50%',
            bgcolor: '#90a4ae',
            animation: 'typingBounce 1.4s ease-in-out infinite',
            animationDelay: `${i * 0.2}s`,
            '@keyframes typingBounce': {
              '0%,60%,100%': { transform: 'translateY(0)', opacity: 0.4 },
              '30%': { transform: 'translateY(-5px)', opacity: 1 },
            },
          }}
        />
      ))}
    </Box>
  );
}

function InfoRow({ label, value }: { label: string; value: string }) {
  return (
    <Box sx={{ py: 0.25 }}>
      <Typography sx={{ fontSize: 12, color: WA.meta, mb: 0.25, fontWeight: 500 }}>{label}</Typography>
      <Typography
        noWrap
        title={value}
        sx={{ fontSize: 14, color: '#111b21', lineHeight: 1.4 }}
      >
        {value}
      </Typography>
    </Box>
  );
}

/* ─── Brief (compact, collapsed by default) ─────────────────────────────── */

const STAGE_LABEL: Record<string, string> = {
  new: 'Baru',
  info: 'Tanya info',
  interest: 'Minat',
  transaction: 'Order',
  issue: 'Keluhan',
  done: 'Selesai',
};

function stageColor(stage: string): 'default' | 'info' | 'success' | 'warning' | 'error' | 'primary' {
  switch (stage) {
    case 'issue': return 'error';
    case 'transaction': return 'primary';
    case 'interest': return 'info';
    case 'done': return 'success';
    default: return 'default';
  }
}

function ConversationBriefBar({
  brief, loading, refreshing, onRefresh, error,
}: {
  brief?: ConversationBrief;
  loading: boolean;
  refreshing: boolean;
  onRefresh: () => void;
  error?: string;
}) {
  const [open, setOpen] = useState(false);

  if (loading && !brief) {
    return (
      <Stack direction="row" spacing={1} sx={{ px: 1.5, py: 0.85, alignItems: 'center', bgcolor: WA.panelHeader, borderBottom: `1px solid ${WA.border}` }}>
        <CircularProgress size={14} thickness={5} sx={{ color: WA.green }} />
        <Typography sx={{ fontSize: 12.5, color: WA.meta }}>Menyusun ringkasan…</Typography>
      </Stack>
    );
  }
  if (!brief && error) {
    return (
      <Alert
        severity="warning"
        sx={{ borderRadius: 0, py: 0.25 }}
        action={<Button size="small" onClick={onRefresh} disabled={refreshing}>Coba lagi</Button>}
      >
        Ringkasan belum bisa dimuat
      </Alert>
    );
  }
  if (!brief) return null;

  return (
    <Box sx={{ borderBottom: `1px solid ${WA.border}`, bgcolor: '#fff', flexShrink: 0 }}>
      <Stack
        direction="row"
        spacing={0.85}
        onClick={() => setOpen((o) => !o)}
        sx={{
          px: { xs: 1, md: 1.25 },
          py: 0.7,
          alignItems: 'center',
          cursor: 'pointer',
          bgcolor: brief.needs_human || brief.stage === 'issue' ? alpha('#ed6c02', 0.045) : '#fbfcfc',
          '&:hover': { bgcolor: brief.needs_human || brief.stage === 'issue' ? alpha('#ed6c02', 0.075) : alpha(WA.green, 0.045) },
        }}
      >
        <Box
          sx={{
            width: 32,
            height: 32,
            flexShrink: 0,
            display: 'grid',
            placeItems: 'center',
            borderRadius: 1,
            color: brief.needs_human || brief.stage === 'issue' ? '#b45309' : WA.greenDark,
            bgcolor: brief.needs_human || brief.stage === 'issue' ? alpha('#ed6c02', 0.1) : alpha(WA.green, 0.09),
          }}
        >
          <AutoAwesomeIcon sx={{ fontSize: 17 }} />
        </Box>
        <Box sx={{ flex: 1, minWidth: 0 }}>
          <Box sx={{ display: 'flex', alignItems: 'center', flexWrap: 'wrap', columnGap: 0.5, rowGap: 0.35 }}>
            <Typography
              noWrap
              sx={{ minWidth: 100, flex: '1 1 180px', fontWeight: 750, fontSize: 12.75, color: '#111b21', lineHeight: 1.25 }}
            >
              {brief.intent || 'Ringkasan percakapan'}
            </Typography>
            <Chip
              size="small"
              color={stageColor(brief.stage)}
              label={STAGE_LABEL[brief.stage] || brief.stage}
              sx={{ height: 19, flexShrink: 0, borderRadius: '4px !important', fontSize: 9.75, fontWeight: 700, '.MuiChip-label': { px: 0.75 } }}
            />
            {brief.needs_human && (
              <Chip
                size="small"
                color="warning"
                label="Butuh CS"
                sx={{ height: 19, flexShrink: 0, borderRadius: '4px !important', fontSize: 9.75, '.MuiChip-label': { px: 0.75 } }}
              />
            )}
            {brief.stale && (
              <Chip
                size="small"
                color="warning"
                variant="outlined"
                label="Memperbarui"
                sx={{ height: 19, flexShrink: 0, borderRadius: '4px !important', fontSize: 9.75, '.MuiChip-label': { px: 0.75 } }}
              />
            )}
          </Box>
          {!open && brief.summary && (
            <Typography
              noWrap
              title={brief.summary}
              sx={{ fontSize: 11.5, color: WA.meta, mt: 0.2, lineHeight: 1.25 }}
            >
              {brief.summary}
            </Typography>
          )}
        </Box>
        <Stack direction="row" spacing={0.1} sx={{ flexShrink: 0 }} onClick={(e) => e.stopPropagation()}>
          <Tooltip title="Perbarui ringkasan">
            <span>
              <IconButton
                size="small"
                onClick={onRefresh}
                disabled={refreshing}
                sx={{ width: 30, height: 30, color: WA.meta, borderRadius: '50% !important' }}
              >
                {refreshing ? <CircularProgress size={14} /> : <RefreshIcon sx={{ fontSize: 16 }} />}
              </IconButton>
            </span>
          </Tooltip>
          <IconButton
            size="small"
            onClick={() => setOpen((o) => !o)}
            aria-label={open ? 'Tutup ringkasan' : 'Buka ringkasan'}
            sx={{ width: 30, height: 30, color: WA.meta, borderRadius: '50% !important' }}
          >
            {open ? <ExpandLessIcon sx={{ fontSize: 18 }} /> : <ExpandMoreIcon sx={{ fontSize: 18 }} />}
          </IconButton>
        </Stack>
      </Stack>
      <Collapse in={open}>
        <Box sx={{ pl: { xs: 5.85, md: 6.1 }, pr: 1.5, pb: 1.1, maxHeight: 200, overflowY: 'auto' }}>
          {brief.current_state && (
            <Stack
              direction={{ xs: 'column', sm: 'row' }}
              spacing={{ xs: 0.15, sm: 0.75 }}
              sx={{ alignItems: { xs: 'flex-start', sm: 'center' }, mb: 0.7 }}
            >
              <Typography sx={{ fontSize: 10.75, fontWeight: 750, color: WA.meta, textTransform: 'uppercase', letterSpacing: 0.25 }}>
                Kondisi sekarang
              </Typography>
              <Typography
                sx={{
                  fontSize: 12.25,
                  fontWeight: 700,
                  color: brief.waiting_for === 'cs' ? '#b45309' : brief.waiting_for === 'customer' ? '#0277bd' : WA.greenDark,
                }}
              >
                {brief.current_state}
              </Typography>
            </Stack>
          )}
          {brief.summary && (
            <Typography sx={{ fontSize: 13, color: '#3b4a54', lineHeight: 1.45, mb: 0.75, whiteSpace: 'pre-wrap' }}>
              {brief.summary}
            </Typography>
          )}
          {(brief.open_items?.length || 0) > 0 && (
            <Box sx={{ mb: 0.75 }}>
              <Typography sx={{ fontSize: 11, fontWeight: 700, color: WA.meta, mb: 0.35, textTransform: 'uppercase' }}>
                Perlu dikerjakan
              </Typography>
              {brief.open_items.map((item, i) => (
                <Typography key={i} sx={{ fontSize: 12.5, color: '#111b21', pl: 1, lineHeight: 1.4 }}>
                  {i + 1}. {item}
                </Typography>
              ))}
            </Box>
          )}
          {(brief.risk_flags?.length || 0) > 0 && (
            <Box sx={{ mb: 0.75 }}>
              <Typography sx={{ fontSize: 11, fontWeight: 700, color: '#b45309', mb: 0.35, textTransform: 'uppercase' }}>
                Perlu perhatian
              </Typography>
              <Typography sx={{ fontSize: 12.5, color: '#7c2d12', lineHeight: 1.4 }}>
                {brief.risk_flags.join(' · ')}
              </Typography>
            </Box>
          )}
          {(brief.key_facts?.length || 0) > 0 && (
            <Box>
              <Typography sx={{ fontSize: 11, fontWeight: 700, color: WA.meta, mb: 0.35, textTransform: 'uppercase' }}>
                Fakta
              </Typography>
              {brief.key_facts.map((item, i) => (
                <Typography key={i} sx={{ fontSize: 12.5, color: '#3b4a54', pl: 1, lineHeight: 1.4 }}>
                  • {item}
                </Typography>
              ))}
            </Box>
          )}
          {brief.enhancement_note && (
            <Typography sx={{ mt: 0.7, fontSize: 10.75, color: WA.meta, lineHeight: 1.35 }}>
              {brief.enhancement_note}
            </Typography>
          )}
        </Box>
      </Collapse>
    </Box>
  );
}


/* ─── Contact row (memo) ────────────────────────────────────────────────── */

const ContactRow = memo(function ContactRow({
  ct, selected, onSelect, onDelete, deleting, agentId, mediaToken, aiEnabled,
}: {
  ct: Contact;
  selected: boolean;
  onSelect: (sender: string) => void;
  onDelete?: (sender: string) => void;
  deleting?: boolean;
  agentId: number;
  mediaToken: string;
  aiEnabled: boolean;
}) {
  const isGroup = Boolean(ct.is_group || ct.sender.endsWith('@g.us'));
  const label = ct.name || (isGroup ? 'Grup WhatsApp' : `+${ct.sender}`);
  const unreadCount = ct.unread_count ?? 0;
  const canDelete = Boolean(onDelete && !isGroup);
  return (
    <Box
      sx={{
        width: '100%',
        borderBottom: `1px solid ${WA.border}`,
        bgcolor: selected ? WA.listActive : 'transparent',
        position: 'relative',
        contentVisibility: 'auto',
        containIntrinsicSize: '76px',
        '&:hover': { bgcolor: selected ? WA.listActive : WA.listHover },
        '&:hover .inbox-del-btn': { opacity: 1 },
        '&:hover .inbox-time': { opacity: { xs: 1, md: 0 } },
      }}
    >
      <Box
        component="button"
        type="button"
        onClick={() => onSelect(ct.sender)}
        sx={{
          appearance: 'none',
          width: '100%',
          flex: 1,
          minWidth: 0,
          textAlign: 'left',
          border: 0,
          bgcolor: 'transparent',
          display: 'flex',
          alignItems: 'center',
          gap: 1.25,
          pl: 1.5,
          pr: { xs: canDelete ? 6 : 1.5, md: 1.5 },
          py: 1.1,
          cursor: 'pointer',
        }}
      >
        <LazyContactAvatar
          agentId={agentId}
          sender={ct.sender}
          mediaToken={mediaToken}
          label={label}
          isGroup={isGroup}
        />
        <Box sx={{ minWidth: 0, flex: 1, border: 0 }}>
          <Stack direction="row" sx={{ width: '100%', justifyContent: 'space-between', alignItems: 'baseline', gap: 1.5, mb: 0.2 }}>
            <Typography
              noWrap
              sx={{ minWidth: 0, flex: 1, overflow: 'hidden', textOverflow: 'ellipsis', fontWeight: 500, fontSize: 16, color: '#111b21', lineHeight: 1.25 }}
            >
              {label}
            </Typography>
            <Typography
              className="inbox-time"
              sx={{ fontSize: 12, color: ct.needs_human ? WA.green : WA.meta, flexShrink: 0, transition: 'opacity 0.12s' }}
            >
              {fmtListTime(ct.last_at)}
            </Typography>
          </Stack>
          <Stack direction="row" sx={{ justifyContent: 'space-between', alignItems: 'center', gap: 0.75 }}>
            <Typography
              noWrap
              sx={{ width: 0, fontSize: 13.5, color: WA.meta, flex: 1, minWidth: 0, overflow: 'hidden', textOverflow: 'ellipsis', lineHeight: 1.3 }}
            >
              {ct.preview_stale ? '⚠ ' : ''}
              {ct.last_msg || (ct.name ? `+${ct.sender}` : ' ')}
              {ct.preview_stale ? ' · riwayat belum lengkap' : ''}
            </Typography>
            {ct.labels?.[0] && (
              <Box
                title={ct.labels.map((label) => label.name).join(', ')}
                sx={{
                  maxWidth: 82,
                  height: 19,
                  px: 0.65,
                  borderRadius: 1,
                  display: 'flex',
                  alignItems: 'center',
                  gap: 0.4,
                  bgcolor: alpha(waLabelColor(ct.labels[0].color), 0.11),
                  color: waLabelColor(ct.labels[0].color),
                  flexShrink: 0,
                }}
              >
                <Box sx={{ width: 6, height: 6, borderRadius: '50%', bgcolor: 'currentColor', flexShrink: 0 }} />
                <Typography noWrap sx={{ minWidth: 0, fontSize: 10.5, fontWeight: 700 }}>
                  {ct.labels[0].name}{ct.labels.length > 1 ? ` +${ct.labels.length - 1}` : ''}
                </Typography>
              </Box>
            )}
            {unreadCount > 0 && (
              <Box
                sx={{
                  minWidth: 20,
                  height: 20,
                  px: 0.6,
                  borderRadius: 10,
                  bgcolor: WA.green,
                  color: '#fff',
                  fontSize: 11,
                  fontWeight: 700,
                  display: 'grid',
                  placeItems: 'center',
                  flexShrink: 0,
                }}
              >
                {unreadCount > 99 ? '99+' : unreadCount}
              </Box>
            )}
            {ct.needs_human ? (
              <Box
                sx={{
                  minWidth: 20,
                  height: 20,
                  px: 0.6,
                  borderRadius: 10,
                  bgcolor: WA.green,
                  color: '#fff',
                  fontSize: 11,
                  fontWeight: 700,
                  display: 'grid',
                  placeItems: 'center',
                  flexShrink: 0,
                }}
              >
                !
              </Box>
            ) : aiEnabled && ct.manual_pause_until ? (
              <Chip size="small" label="AI off" sx={{ height: 18, fontSize: 10, bgcolor: alpha('#53bdeb', 0.15), color: '#0288d1' }} />
            ) : null}
          </Stack>
        </Box>
      </Box>
      {canDelete && onDelete && <Tooltip title="Hapus chat dari inbox">
        <span>
          <IconButton
            className="inbox-del-btn"
            size="small"
            disabled={deleting}
            onClick={(e) => {
              e.stopPropagation();
              onDelete(ct.sender);
            }}
            aria-label={`Hapus chat ${label}`}
            sx={{
              opacity: { xs: 0.85, md: 0 },
              transition: 'opacity 0.12s',
              color: 'error.main',
              position: 'absolute',
              right: 8,
              top: '50%',
              transform: 'translateY(-50%)',
              zIndex: 1,
              bgcolor: selected ? WA.listActive : WA.panel,
              '&:hover': { bgcolor: alpha('#d32f2f', 0.08) },
            }}
          >
            {deleting ? <CircularProgress size={16} color="inherit" /> : <DeleteIcon sx={{ fontSize: 18 }} />}
          </IconButton>
        </span>
      </Tooltip>}
    </Box>
  );
});

/* ─── Message list item ─────────────────────────────────────────────────── */

// Deteksi URL untuk linkifikasi + preview. Sama konservatif dengan backend.
const LINK_URL_RE = /https?:\/\/[^\s<>"')\]]+/g;

/** Potong tanda baca trailing yang bukan bagian URL (titik, koma, kurung tutup). */
function trimLinkPunct(url: string): string {
  return url.replace(/[.,;:!?）】」』"')\]]+$/, '');
}

/** Render teks dengan URL jadi link klikable berwarna (WA palette). */
function LinkifiedText({ text, sx }: { text: string; sx?: object }) {
  const parts: ReactNode[] = [];
  let lastIndex = 0;
  let key = 0;
  for (const match of text.matchAll(LINK_URL_RE)) {
    const index = match.index ?? 0;
    if (index > lastIndex) parts.push(text.slice(lastIndex, index));
    const raw = match[0];
    const url = trimLinkPunct(raw);
    parts.push(
      <Box
        key={key++}
        component="a"
        href={url}
        target="_blank"
        rel="noreferrer"
        onClick={(e) => e.stopPropagation()}
        sx={{ color: '#53bdeb', textDecoration: 'underline', wordBreak: 'break-all', display: 'inline' }}
      >
        {url}
      </Box>,
    );
    lastIndex = index + raw.length;
  }
  if (lastIndex < text.length) parts.push(text.slice(lastIndex));
  if (parts.length === 0) return null;
  return <Typography component="span" sx={sx}>{parts}</Typography>;
}

/** Kartu preview OpenGraph untuk URL pertama dalam pesan (auto, lazy viewport). */
function LinkPreviewCard({
  agentId, url, sx,
}: {
  agentId: number;
  url: string;
  sx?: object;
}) {
  const [inView, setInView] = useState(false);
  const hostRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    const el = hostRef.current;
    if (!el) return;
    if (typeof IntersectionObserver === 'undefined') {
      setInView(true);
      return;
    }
    const io = new IntersectionObserver(
      (entries) => {
        if (entries.some((e) => e.isIntersecting)) {
          setInView(true);
          io.disconnect();
        }
      },
      { rootMargin: '200px 0px' },
    );
    io.observe(el);
    return () => io.disconnect();
  }, []);

  const previewQ = useLinkPreview(agentId, url, inView && !!url);
  const preview = previewQ.data;

  // Host Box selalu dirender agar IntersectionObserver punya elemen untuk
  // di-observe (kalau return null duluan, inView tidak pernah true dan query
  // tidak pernah di-fetch). Isi kartu baru muncul saat preview tersedia.
  return (
    <Box ref={hostRef} sx={{ minWidth: 0, ...sx }}>
      {previewQ.isError || !preview || (!preview.image && !preview.title) ? null : (
        <Box sx={{ mt: 0.5, mb: 0.25, maxWidth: 340 }}>
          <Box
            component="a"
            href={preview.url || url}
            target="_blank"
            rel="noreferrer"
            onClick={(e) => e.stopPropagation()}
            sx={{ textDecoration: 'none', display: 'block' }}
          >
        <Box
          sx={{
            display: 'flex',
            overflow: 'hidden',
            borderRadius: 1,
            border: '1px solid',
            borderColor: alpha(WA.meta, 0.16),
            bgcolor: 'rgba(0,0,0,0.03)',
            '&:hover': { borderColor: alpha(WA.meta, 0.32) },
          }}
        >
          {preview.image && (
            <Box
              component="img"
              src={preview.image}
              alt=""
              loading="lazy"
              referrerPolicy="no-referrer"
              onError={(e) => { (e.target as HTMLImageElement).style.display = 'none'; }}
              sx={{
                width: 96,
                height: 96,
                objectFit: 'cover',
                flexShrink: 0,
                bgcolor: 'action.hover',
              }}
            />
          )}
          <Box sx={{ minWidth: 0, flex: 1, p: 0.75, display: 'flex', flexDirection: 'column', gap: 0.25 }}>
            {preview.site_name && (
              <Typography noWrap sx={{ fontSize: 10.5, fontWeight: 700, color: WA.meta, textTransform: 'uppercase', letterSpacing: 0.4 }}>
                {preview.site_name}
              </Typography>
            )}
            <Typography noWrap sx={{ fontSize: 13, fontWeight: 600, color: '#111b21', lineHeight: 1.3 }} title={preview.title}>
              {preview.title}
            </Typography>
            {preview.description && (
              <Typography
                noWrap
                sx={{ fontSize: 11.5, color: WA.meta, lineHeight: 1.35 }}
                title={preview.description}
              >
                {preview.description}
              </Typography>
            )}
          </Box>
        </Box>
      </Box>
        </Box>
      )}
    </Box>
  );
}

/** URL pertama dalam teks (untuk preview), atau null. */
function firstLinkInText(text: string): string | null {
  const match = text.match(LINK_URL_RE);
  if (!match) return null;
  return trimLinkPunct(match[0]);
}

const MessageBlock = memo(function MessageBlock({
  m,
  agentId,
  mediaToken,
  inboundQuote,
  outboundQuote,
  onReply,
  onNavigateReply,
  onRevoke,
  onVision,
  compacted,
}: {
  m: ChatMsg;
  agentId: number;
  mediaToken: string;
  compacted?: boolean;
  inboundQuote: ReplyPreview | null;
  outboundQuote: ReplyPreview | null;
  onReply: (id: string, text: string) => void;
  onNavigateReply: (id: string) => void;
  onRevoke: (msgId: string) => void;
  onVision: (m: ChatMsg) => void;
}) {
  const rawMessage = (m.message || '').trim();
  const rawReply = (m.reply || '').trim();
  const visibleMessage = cleanHistoryMediaText(rawMessage)
    || (!m.media_type && historyMediaPrefix.test(rawMessage) ? rawMessage : '');
  const visibleReply = cleanHistoryMediaText(rawReply)
    || (!m.media_type && historyMediaPrefix.test(rawReply) ? rawReply : '');
  const hasDownloadableAttachment = (
    m.media_type === 'image'
    || m.media_type === 'sticker'
    || m.media_type === 'video'
    || m.media_type === 'audio'
    || m.media_type === 'document'
  );
  const openInboundQuote = inboundQuote
    ? () => onNavigateReply(
      inboundQuote.localId
        ? `local:${inboundQuote.localId}`
        : (inboundQuote.id || m.reply_to || ''),
    )
    : undefined;
  const openOutboundQuote = outboundQuote
    ? () => onNavigateReply(
      outboundQuote.localId
        ? `local:${outboundQuote.localId}`
        : (outboundQuote.id || m.reply_to || ''),
    )
    : undefined;

  return (
    <Box
      data-message-id={m.wa_msg_id || String(m.id)}
      data-wa-msg-id={m.wa_msg_id || ''}
      data-local-message-id={String(m.id)}
      sx={{
        display: 'flex',
        flexDirection: 'column',
        // Spasi antar bubble ala WA Web — jangan mepet.
        gap: compacted ? 0.4 : 1.1,
        pt: compacted ? 0.2 : 0.85,
        pb: compacted ? 0.05 : 0.15,
        px: { xs: 1, sm: 2.5, md: 4 },
        borderRadius: 1,
      }}
    >
      {(m.message || (m.media_type && !m.from_human)) && (
        <Bubble
          side="left"
          time={fmtTime(m.created_at)}
          replyPreview={inboundQuote}
          agentId={agentId}
          mediaToken={mediaToken}
          onOpenReply={openInboundQuote}
          onReply={() => onReply(m.wa_msg_id || String(m.id), mediaPreviewLabel(m))}
        >
          {m.revoked ? (
            <Typography sx={{ fontStyle: 'italic', color: WA.meta, fontSize: 13 }}>Pesan ini dihapus</Typography>
          ) : (
            <>
              {hasDownloadableAttachment && m.media_downloadable && !m.from_human && <MediaView agentId={agentId} m={m} token={mediaToken} />}
              {hasDownloadableAttachment && !m.media_downloadable && !m.from_human && <MissingMediaNotice m={m} />}
              {visibleMessage && (
                <Box sx={{ mt: hasDownloadableAttachment ? 0.5 : 0, fontSize: 'inherit', lineHeight: 1.4, whiteSpace: 'pre-wrap' }}>
                  {!m.from_human && firstLinkInText(visibleMessage) && (
                    <LinkPreviewCard agentId={agentId} url={firstLinkInText(visibleMessage) as string} />
                  )}
                  <LinkifiedText text={visibleMessage} />
                </Box>
              )}
              {m.image_analysis && (
                <Box
                  sx={{
                    mt: 0.75,
                    p: 0.75,
                    borderRadius: 1,
                    bgcolor: m.image_analysis_status === 'completed' ? alpha(WA.green, 0.08) : alpha('#ed6c02', 0.08),
                    border: '1px solid',
                    borderColor: m.image_analysis_status === 'completed' ? alpha(WA.green, 0.25) : alpha('#ed6c02', 0.25),
                  }}
                >
                  <Stack direction="row" spacing={0.5} sx={{ alignItems: 'center', mb: 0.3 }}>
                    <SmartToyIcon sx={{ fontSize: 13, color: WA.greenDark }} />
                    <Typography sx={{ fontSize: 11, fontWeight: 700 }}>Analisis gambar</Typography>
                  </Stack>
                  <Typography sx={{ fontSize: 12, lineHeight: 1.4 }}>{m.image_analysis}</Typography>
                </Box>
              )}
              {(m.media_type === 'image' || m.media_type === 'sticker') && m.media_available && !m.from_human && (
                <Button
                  size="small"
                  startIcon={<AutoAwesomeIcon sx={{ fontSize: 14 }} />}
                  onClick={() => onVision(m)}
                  sx={{ mt: 0.5, px: 0.5, fontSize: 11, color: WA.greenDark }}
                >
                  {m.image_analysis ? 'Analisis ulang' : 'Analisis gambar'}
                </Button>
              )}
            </>
          )}
        </Bubble>
      )}

      {(m.reply || (m.media_type && m.from_human)) && (
        <Box sx={{ display: 'flex', flexDirection: 'column', alignItems: 'flex-end' }}>
          <Bubble
            side="right"
            tag={m.from_human ? 'CS' : 'Bot'}
            time={fmtTime(m.created_at)}
            deliveryStatus={m.wa_msg_id ? (m.delivery_status || 'sent') : undefined}
            replyPreview={outboundQuote}
            agentId={agentId}
            mediaToken={mediaToken}
            onOpenReply={openOutboundQuote}
            onReply={() => onReply(m.wa_msg_id || String(m.id), mediaPreviewLabel(m))}
          >
            {m.revoked ? (
              <Typography sx={{ fontStyle: 'italic', color: WA.meta, fontSize: 13 }}>Pesan ini dihapus</Typography>
            ) : (
              <>
                {hasDownloadableAttachment && m.media_downloadable && m.from_human && <MediaView agentId={agentId} m={m} token={mediaToken} />}
                {hasDownloadableAttachment && !m.media_downloadable && m.from_human && <MissingMediaNotice m={m} />}
                {visibleReply && (
                  <Box sx={{ whiteSpace: 'pre-wrap' }}>
                    {firstLinkInText(visibleReply) && (
                      <LinkPreviewCard agentId={agentId} url={firstLinkInText(visibleReply) as string} />
                    )}
                    <LinkifiedText text={visibleReply} />
                  </Box>
                )}
              </>
            )}
          </Bubble>
          {m.from_human && m.wa_msg_id && (
            <IconButton
              size="small"
              onClick={() => onRevoke(m.wa_msg_id || String(m.id))}
              sx={{ p: 0.25, opacity: 0.45, '&:hover': { opacity: 1 } }}
              aria-label="Hapus pesan"
            >
              <DeleteIcon sx={{ fontSize: 14, color: 'error.main' }} />
            </IconButton>
          )}
        </Box>
      )}
    </Box>
  );
});

/* ─── Message thread (memo) — tidak ikut re-render saat mengetik ───────── */

const MessageThread = memo(function MessageThread({
  messages,
  agentId,
  mediaToken,
  resolveReplyPreview,
  onReply,
  onNavigateReply,
  onRevoke,
  onVision,
  showTyping,
  chatRef,
  bottomRef,
  onScrollPosition,
  onUserScrollStart,
  hasOlder,
  loadingOlder,
}: {
  messages: ChatMsg[];
  agentId: number;
  mediaToken: string;
  resolveReplyPreview: (replyTo?: string, replyText?: string) => ReplyPreview | null;
  onReply: (id: string, text: string) => void;
  onNavigateReply: (id: string) => void;
  onRevoke: (msgId: string) => void;
  onVision: (m: ChatMsg) => void;
  showTyping: boolean;
  chatRef: RefObject<HTMLDivElement | null>;
  bottomRef: RefObject<HTMLDivElement | null>;
  onScrollPosition: () => void;
  onUserScrollStart: () => void;
  hasOlder: boolean;
  loadingOlder: boolean;
}) {
  useEffect(() => {
    sampleInboxComponentCommit('MessageThread', {
      message_count: messages.length,
      show_typing: showTyping,
    });
  });

  // Identitas kanonik adalah stanza ID WhatsApp. Data lama tanpa stanza ID hanya
  // boleh dideduplikasi lewat primary key lokal yang sama. Jangan pernah menebak
  // duplikat dari isi/jenis/waktu karena dua stiker atau teks sama tetap dua pesan.
  const visibleMessages = useMemo(() => {
    type CanonicalMessage = { message: ChatMsg; firstIndex: number };
    const byIdentity = new Map<string, CanonicalMessage>();
    const richness = (message: ChatMsg) =>
      (message.media_available ? 16 : 0)
      + (message.media_downloadable ? 8 : 0)
      + (message.media_type ? 4 : 0)
      + (cleanHistoryMediaText(message.message) ? 2 : 0)
      + (cleanHistoryMediaText(message.reply) ? 2 : 0)
      + (message.reply_to ? 1 : 0)
      + (message.revoked ? 32 : 0);

    messages.forEach((message, index) => {
      const waID = (message.wa_msg_id || '').trim();
      const identity = waID ? `wa:${waID}` : `local:${message.id}`;
      const current = byIdentity.get(identity);
      if (!current) {
        byIdentity.set(identity, { message, firstIndex: index });
        return;
      }

      const preferred = richness(message) >= richness(current.message) ? message : current.message;
      const other = preferred === message ? current.message : message;
      byIdentity.set(identity, {
        firstIndex: current.firstIndex,
        message: {
          ...other,
          ...preferred,
          message: preferred.message || other.message,
          reply: preferred.reply || other.reply,
          media_type: preferred.media_type || other.media_type,
          file_name: preferred.file_name || other.file_name,
          mimetype: preferred.mimetype || other.mimetype,
          reply_to: preferred.reply_to || other.reply_to,
          reply_text: preferred.reply_text || other.reply_text,
          media_available: Boolean(preferred.media_available || other.media_available),
          media_downloadable: Boolean(preferred.media_downloadable || other.media_downloadable),
          revoked: Boolean(preferred.revoked || other.revoked),
        },
      });
    });

    return Array.from(byIdentity.values())
      .sort((left, right) => {
        const leftTime = parseMsgDate(left.message.created_at)?.getTime();
        const rightTime = parseMsgDate(right.message.created_at)?.getTime();
        if (leftTime !== undefined && rightTime !== undefined && leftTime !== rightTime) {
          return leftTime - rightTime;
        }
        if (leftTime !== undefined && rightTime === undefined) return -1;
        if (leftTime === undefined && rightTime !== undefined) return 1;
        if (left.message.id !== right.message.id) return left.message.id - right.message.id;
        return left.firstIndex - right.firstIndex;
      })
      .map((entry) => entry.message);
  }, [messages]);

  // Group consecutive messages from the same side (WA-style) — hanya burst singkat.
  // Jangan compact lintas hari; window 90s (bukan 5 menit) biar bubble tidak mepet.
  const compactedFlags = useMemo(() => {
    const flags = new Array(visibleMessages.length).fill(false);
    const isOutbound = (m: ChatMsg) => Boolean(m.from_human || (m.reply && !m.message));
    for (let i = 1; i < visibleMessages.length; i++) {
      const prev = visibleMessages[i - 1];
      const cur = visibleMessages[i];
      if (dayKey(prev.created_at) !== dayKey(cur.created_at)) continue;
      if (isOutbound(prev) !== isOutbound(cur)) continue;
      const prevTime = new Date(prev.created_at || '').getTime();
      const curTime = new Date(cur.created_at || '').getTime();
      if (Number.isFinite(prevTime) && Number.isFinite(curTime) && Math.abs(curTime - prevTime) < 90_000) {
        flags[i] = true;
      }
    }
    return flags;
  }, [visibleMessages]);

  return (
    <Box
      ref={chatRef}
      onScroll={onScrollPosition}
      onWheel={onUserScrollStart}
      onTouchStart={onUserScrollStart}
      onPointerDown={onUserScrollStart}
      sx={{
        flex: 1,
        minHeight: 0,
        overflowY: 'auto',
        py: { xs: 1.5, md: 2 },
        display: 'flex',
        flexDirection: 'column',
        overscrollBehavior: 'contain',
        overflowAnchor: 'none',
        scrollbarGutter: 'stable',
        touchAction: 'pan-y',
      }}
    >
      {(hasOlder || loadingOlder) && (
        <Box
          aria-live="polite"
          sx={{
            minHeight: 28,
            display: 'flex',
            justifyContent: 'center',
            alignItems: 'center',
            gap: 0.75,
            px: 2,
            pb: 1,
            color: WA.meta,
          }}
        >
          {loadingOlder && <CircularProgress size={13} sx={{ color: WA.green }} />}
          <Typography sx={{ fontSize: 11.5, color: 'inherit' }}>
            {loadingOlder ? 'Memuat 100 pesan lama…' : 'Scroll ke atas untuk memuat pesan lama'}
          </Typography>
        </Box>
      )}
      {visibleMessages.map((m, idx) => {
        const prev = idx > 0 ? visibleMessages[idx - 1] : null;
        const curDay = dayKey(m.created_at);
        const showDayChip = Boolean(curDay && (!prev || dayKey(prev.created_at) !== curDay));
        const dayLabel = showDayChip ? fmtDayChip(m.created_at) : '';
        // Preview dihitung di thread. MessageBlock tanpa quote menerima `null`
        // yang stabil, sehingga penambahan satu bubble tidak merender ulang
        // ratusan bubble lama hanya karena fungsi lookup berubah identitas.
        const inboundQuote = (!m.from_human || Boolean(m.message))
          ? resolveReplyPreview(m.reply_to, m.reply_text)
          : null;
        const outboundQuote = m.from_human
          ? resolveReplyPreview(m.reply_to, m.reply_text)
          : null;
        return (
          <Fragment key={m.id}>
            {showDayChip && dayLabel && (
              <Box
                sx={{
                  display: 'flex',
                  justifyContent: 'center',
                  pt: 1.75,
                  pb: 1.25,
                  px: { xs: 1, sm: 2.5, md: 4 },
                }}
              >
                <Box
                  component="span"
                  sx={{
                    px: 1.4,
                    py: 0.45,
                    borderRadius: 1.5,
                    bgcolor: alpha('#e9edef', 0.98),
                    color: '#54656f',
                    fontSize: 12.5,
                    fontWeight: 600,
                    letterSpacing: 0.15,
                    boxShadow: '0 1px 1.5px rgba(11,20,26,0.12)',
                    userSelect: 'none',
                  }}
                >
                  {dayLabel}
                </Box>
              </Box>
            )}
            <MessageBlock
              m={m}
              agentId={agentId}
              mediaToken={mediaToken}
              inboundQuote={inboundQuote}
              outboundQuote={outboundQuote}
              onReply={onReply}
              onNavigateReply={onNavigateReply}
              onRevoke={onRevoke}
              onVision={onVision}
              compacted={compactedFlags[idx]}
            />
          </Fragment>
        );
      })}
      {showTyping && (
        <Box sx={{ px: { xs: 1, sm: 2.5, md: 4 } }}>
          <TypingIndicator />
        </Box>
      )}
      <div ref={bottomRef} />
    </Box>
  );
});

/* ─── Composer (state teks lokal) — mengetik tidak re-render daftar chat ─ */

const ChatComposer = memo(function ChatComposer({
  agentId,
  sender,
  selectedName,
  replyTo,
  onClearReply,
  onSend,
  dropTargetRef,
  onInputNode,
}: {
  agentId: number;
  sender: string;
  selectedName?: string;
  replyTo: { id: string; text: string } | null;
  onClearReply: () => void;
  onSend: (payload: {
    text: string;
    file: File | null;
    replyTo: { id: string; text: string } | null;
  }) => Promise<void>;
  dropTargetRef: RefObject<HTMLDivElement | null>;
  onInputNode: (node: HTMLTextAreaElement | null) => void;
}) {
  // Draft disimpan di ref/uncontrolled textarea: karakter biasa tidak memicu
  // render React. State hanya berubah saat transisi kosong <-> berisi agar
  // tombol Kirim dapat diperbarui.
  const [hasText, setHasText] = useState(false);
  const [attachment, setAttachment] = useState<ComposerAttachment | null>(null);
  const [dragActive, setDragActive] = useState(false);
  const [previewFailed, setPreviewFailed] = useState(false);
  const [sending, setSending] = useState(false);
  const sendingRef = useRef(false);
  const [emojiAnchor, setEmojiAnchor] = useState<HTMLElement | null>(null);
  const fileInput = useRef<HTMLInputElement>(null);
  const messageInput = useRef<HTMLTextAreaElement>(null);
  const captionInput = useRef<HTMLTextAreaElement>(null);
  const attachmentRef = useRef<ComposerAttachment | null>(null);
  const draftRef = useRef('');
  const draftRevision = useRef(0);
  const hasTextRef = useRef(false);
  const dragDepth = useRef(0);
  const typingActive = useRef(false);
  const typingTimer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const typingDeadline = useRef(0);
  const typingRequest = useRef<AbortController | null>(null);
  const senderRef = useRef(sender);
  const file = attachment?.file || null;

  useEffect(() => {
    sampleInboxComponentCommit('ChatComposer', {
      has_text: hasText,
      sending,
      has_attachment: Boolean(attachment),
    });
  });

  const bindCaptionInput = useCallback((node: HTMLTextAreaElement | null) => {
    captionInput.current = node;
    if (node && node.value !== draftRef.current) node.value = draftRef.current;
  }, []);

  const bindMessageInput = useCallback((node: HTMLTextAreaElement | null) => {
    messageInput.current = node;
    onInputNode(node);
  }, [onInputNode]);

  const syncDraftAvailability = useCallback((value: string) => {
    const available = Boolean(value.trim());
    if (hasTextRef.current === available) return;
    hasTextRef.current = available;
    setHasText(available);
  }, []);

  const writeDraftValue = useCallback((nextValue: string) => {
    const value = nextValue.slice(0, MAX_COMPOSER_TEXT_LENGTH);
    draftRef.current = value;
    draftRevision.current += 1;
    if (messageInput.current && messageInput.current.value !== value) {
      messageInput.current.value = value;
    }
    if (captionInput.current && captionInput.current.value !== value) {
      captionInput.current.value = value;
    }
    syncDraftAvailability(value);
  }, [syncDraftAvailability]);

  const sendTypingPresence = useCallback((to: string, active: boolean) => {
    if (!to) return;
    // Hanya satu request presence boleh hidup. Request lama dibatalkan supaya
    // koneksi lambat tidak menumpuk ketika operator berpindah fokus/chat.
    typingRequest.current?.abort();
    const controller = new AbortController();
    typingRequest.current = controller;
    const requestStartedAt = window.performance.now();
    inboxDebugLog('composer.presence.start', { active });
    void postAgentTyping(agentId, to, active, controller.signal).finally(() => {
      inboxDebugLog('composer.presence.finish', {
        active,
        aborted: controller.signal.aborted,
        duration_ms: Math.round(window.performance.now() - requestStartedAt),
      });
      if (typingRequest.current === controller) typingRequest.current = null;
    });
  }, [agentId]);

  const replaceAttachment = useCallback((nextFile: File | null) => {
    const previous = attachmentRef.current;
    if (previous?.previewURL) URL.revokeObjectURL(previous.previewURL);

    let next: ComposerAttachment | null = null;
    if (nextFile) {
      const kind = composerAttachmentKind(nextFile);
      let previewURL = '';
      if (kind === 'image' || kind === 'video') {
        try {
          previewURL = URL.createObjectURL(nextFile);
        } catch {
          previewURL = '';
        }
      }
      next = { file: nextFile, previewURL, kind };
    }

    attachmentRef.current = next;
    setAttachment(next);
    setPreviewFailed(false);
    if (!next && fileInput.current) fileInput.current.value = '';
  }, []);

  const acceptAttachment = useCallback((nextFile: File): boolean => {
    if (nextFile.size <= 0) {
      swalToast('File kosong tidak dapat dikirim.', 'error');
      return false;
    }
    if (nextFile.size > MAX_COMPOSER_FILE_BYTES) {
      swalToast('Ukuran media maksimal 64 MB.', 'error');
      return false;
    }
    if (!composerFileAllowed(nextFile)) {
      swalToast('Format file belum didukung. Pilih gambar, video, PDF, dokumen Office, TXT, atau ZIP.', 'error');
      return false;
    }
    replaceAttachment(normalizeComposerFile(nextFile));
    return true;
  }, [replaceAttachment]);

  useEffect(() => {
    const mountedSender = sender;
    inboxDebugLog('composer.mount', {
      agent_id: agentId,
      sender_kind: sender.endsWith('@g.us') ? 'group' : 'customer',
    });
    return () => {
      inboxDebugLog('composer.unmount', {
        agent_id: agentId,
        sender_kind: sender.endsWith('@g.us') ? 'group' : 'customer',
      });
      if (typingTimer.current) clearTimeout(typingTimer.current);
      typingTimer.current = null;
      typingRequest.current?.abort();
      typingRequest.current = null;
      if (typingActive.current && mountedSender) {
        // Best effort saat komponen dilepas; tidak menunggu respons jaringan.
        void postAgentTyping(agentId, mountedSender, false);
      }
      typingActive.current = false;
      const current = attachmentRef.current;
      if (current?.previewURL) URL.revokeObjectURL(current.previewURL);
      attachmentRef.current = null;
    };
  }, [agentId, sender]);

  useEffect(() => {
    const target = dropTargetRef.current;
    if (!target) return undefined;
    const hasFiles = (event: DragEvent) => Array.from(event.dataTransfer?.types || []).includes('Files');
    const handleDragEnter = (event: DragEvent) => {
      if (!hasFiles(event)) return;
      event.preventDefault();
      dragDepth.current += 1;
      setDragActive(true);
    };
    const handleDragOver = (event: DragEvent) => {
      if (!hasFiles(event)) return;
      event.preventDefault();
      if (event.dataTransfer) event.dataTransfer.dropEffect = 'copy';
    };
    const handleDragLeave = (event: DragEvent) => {
      if (dragDepth.current === 0) return;
      event.preventDefault();
      dragDepth.current = Math.max(0, dragDepth.current - 1);
      if (dragDepth.current === 0) setDragActive(false);
    };
    const handleDrop = (event: DragEvent) => {
      if (!hasFiles(event)) return;
      event.preventDefault();
      dragDepth.current = 0;
      setDragActive(false);
      const files = Array.from(event.dataTransfer?.files || []);
      if (!files.length) return;
      if (files.length > 1) swalToast('Saat ini kirim satu lampiran dalam satu pesan.', 'info');
      acceptAttachment(files[0]);
    };

    target.addEventListener('dragenter', handleDragEnter);
    target.addEventListener('dragover', handleDragOver);
    target.addEventListener('dragleave', handleDragLeave);
    target.addEventListener('drop', handleDrop);
    return () => {
      target.removeEventListener('dragenter', handleDragEnter);
      target.removeEventListener('dragover', handleDragOver);
      target.removeEventListener('dragleave', handleDragLeave);
      target.removeEventListener('drop', handleDrop);
    };
  }, [acceptAttachment, dropTargetRef]);

  const stopTyping = useCallback(() => {
    if (typingTimer.current) {
      clearTimeout(typingTimer.current);
      typingTimer.current = null;
    }
    typingDeadline.current = 0;
    if (typingActive.current) {
      typingActive.current = false;
      sendTypingPresence(senderRef.current, false);
    }
  }, [sendTypingPresence]);

  // Deadline scheduler: keystroke hanya memperpanjang deadline pada ref.
  // Tidak ada clearTimeout/setTimeout baru pada setiap karakter; maksimal satu
  // timer dan satu request presence aktif untuk seluruh burst pengetikan.
  const pulseTyping = useCallback(() => {
    const to = senderRef.current;
    if (!to) return;
    typingDeadline.current = window.performance.now() + COMPOSER_TYPING_IDLE_MS;
    if (!typingActive.current) {
      typingActive.current = true;
      sendTypingPresence(to, true);
    }
    if (typingTimer.current) return;

    const finishWhenIdle = () => {
      const remaining = typingDeadline.current - window.performance.now();
      if (remaining > 0) {
        typingTimer.current = setTimeout(finishWhenIdle, Math.max(80, remaining));
        return;
      }
      typingTimer.current = null;
      if (!typingActive.current) return;
      typingActive.current = false;
      sendTypingPresence(senderRef.current, false);
    };
    typingTimer.current = setTimeout(finishWhenIdle, COMPOSER_TYPING_IDLE_MS);
  }, [sendTypingPresence]);

  const handleDraftInput = useCallback((source: HTMLTextAreaElement) => {
    const value = source.value;
    draftRef.current = value;
    draftRevision.current += 1;
    const mirror = source === messageInput.current ? captionInput.current : messageInput.current;
    if (mirror && mirror.value !== value) mirror.value = value;
    syncDraftAvailability(value);
    sampleComposerInput({
      input: source === messageInput.current ? 'message' : 'caption',
      length: value.length,
      has_attachment: Boolean(attachmentRef.current),
      typing_active: typingActive.current,
    });
    pulseTyping();
  }, [pulseTyping, syncDraftAvailability]);

  const handleTemplatePick = useCallback((body: string, attachment?: File) => {
    const filled = body.replace(/\{nama\}/g, selectedName || 'kak');
    const current = draftRef.current;
    writeDraftValue(current ? `${current} ${filled}` : filled);
    if (attachment) acceptAttachment(attachment);
  }, [acceptAttachment, selectedName, writeDraftValue]);

  const insertEmoji = useCallback((emoji: string) => {
    const input = document.activeElement === captionInput.current
      ? captionInput.current
      : messageInput.current;
    const current = draftRef.current;
    const start = input?.selectionStart ?? current.length;
    const end = input?.selectionEnd ?? start;
    const next = `${current.slice(0, start)}${emoji}${current.slice(end)}`;
    writeDraftValue(next);
    pulseTyping();
    window.requestAnimationFrame(() => {
      const cursor = start + emoji.length;
      input?.focus();
      input?.setSelectionRange(cursor, cursor);
    });
  }, [pulseTyping, writeDraftValue]);

  const doSend = useCallback(async () => {
    if (sendingRef.current) return;
    const sentRevision = draftRevision.current;
    const sentDraft = draftRef.current;
    const payload = {
      text: draftRef.current.trim(),
      file,
      replyTo,
    };
    if (!payload.file && !payload.text) return;
    activateInboxDebugWindow('composer.send', {
      sender_kind: sender.endsWith('@g.us') ? 'group' : 'customer',
      text_length: payload.text.length,
      has_file: Boolean(payload.file),
    });
    inboxDebugLog('composer.send.start', {
      text_length: payload.text.length,
      has_file: Boolean(payload.file),
    });
    stopTyping();
    sendingRef.current = true;
    setSending(true);
    // Lepaskan draft teks dari request yang sedang berjalan. Textarea langsung
    // kosong dan tetap menerima karakter untuk draft berikutnya seperti WA Web.
    let clearedRevision = sentRevision;
    if (!payload.file) {
      writeDraftValue('');
      clearedRevision = draftRevision.current;
      window.requestAnimationFrame(() => messageInput.current?.focus({ preventScroll: true }));
    }
    try {
      await onSend(payload);
      inboxDebugLog('composer.send.success');
      if (payload.file) {
        // Caption hanya dihapus bila belum diedit selama upload.
        if (draftRevision.current === sentRevision) writeDraftValue('');
        replaceAttachment(null);
      }
    } catch (error) {
      inboxDebugLog('composer.send.error', {
        message: composerSendErrorMessage(error, Boolean(payload.file)),
      });
      // Response request lama tidak boleh menimpa draft berikutnya.
      if (!payload.file && draftRevision.current === clearedRevision) {
        writeDraftValue(sentDraft);
      }
      swalToast(composerSendErrorMessage(error, Boolean(payload.file)), 'error');
    } finally {
      sendingRef.current = false;
      setSending(false);
    }
  }, [file, onSend, replaceAttachment, replyTo, sender, stopTyping, writeDraftValue]);

  const isBusy = sending;
  const canSend = Boolean(file || hasText) && !isBusy;

  return (
    <>
      {dragActive && (
        <Box
          sx={{
            position: 'absolute',
            inset: 10,
            zIndex: 20,
            display: 'grid',
            placeItems: 'center',
            borderRadius: 2,
            border: `2px dashed ${WA.green}`,
            bgcolor: alpha('#ffffff', 0.94),
            color: WA.greenDark,
            pointerEvents: 'none',
            boxShadow: '0 10px 36px rgba(11,20,26,0.18)',
          }}
        >
          <Stack spacing={0.75} sx={{ alignItems: 'center', textAlign: 'center', px: 2 }}>
            <AttachFileIcon sx={{ fontSize: 38 }} />
            <Typography sx={{ fontSize: 16, fontWeight: 700 }}>Lepaskan file untuk melihat preview</Typography>
            <Typography sx={{ fontSize: 12.5, color: WA.meta }}>Gambar, video, atau dokumen · maksimal 64 MB</Typography>
          </Stack>
        </Box>
      )}

      <Dialog
        open={Boolean(attachment)}
        onClose={() => {
          if (!isBusy) replaceAttachment(null);
        }}
        fullWidth
        maxWidth="sm"
        slotProps={{
          paper: {
            sx: {
              m: { xs: 1, sm: 2 },
              width: { xs: 'calc(100% - 16px)', sm: '100%' },
              maxHeight: 'calc(100dvh - 32px)',
              borderRadius: 2,
              overflow: 'hidden',
              bgcolor: '#f0f2f5',
            },
          },
        }}
      >
        {attachment && (
          <>
            <DialogTitle
              sx={{
                display: 'flex',
                alignItems: 'center',
                gap: 1,
                py: 1,
                px: 1.5,
                minHeight: 56,
                bgcolor: WA.panelHeader,
                borderBottom: `1px solid ${WA.border}`,
              }}
            >
              <Box sx={{ minWidth: 0, flex: 1 }}>
                <Typography noWrap sx={{ fontSize: 14, fontWeight: 700, color: '#111b21' }}>
                  {attachment.file.name}
                </Typography>
                <Typography sx={{ fontSize: 11.5, color: WA.meta }}>
                  {formatComposerFileSize(attachment.file.size)}
                </Typography>
              </Box>
              <IconButton
                size="small"
                onClick={() => replaceAttachment(null)}
                disabled={isBusy}
                aria-label="Batalkan lampiran"
              >
                <CloseIcon />
              </IconButton>
            </DialogTitle>
            <DialogContent sx={{ p: 0, display: 'flex', flexDirection: 'column', minHeight: 0 }}>
              <Box
                sx={{
                  minHeight: { xs: 260, sm: 420 },
                  maxHeight: 'calc(100dvh - 210px)',
                  flex: 1,
                  display: 'grid',
                  placeItems: 'center',
                  p: { xs: 1.25, sm: 2 },
                  overflow: 'hidden',
                  bgcolor: '#dfe3e5',
                  backgroundImage: `
                    linear-gradient(45deg, ${alpha('#ffffff', 0.32)} 25%, transparent 25%),
                    linear-gradient(-45deg, ${alpha('#ffffff', 0.32)} 25%, transparent 25%),
                    linear-gradient(45deg, transparent 75%, ${alpha('#ffffff', 0.32)} 75%),
                    linear-gradient(-45deg, transparent 75%, ${alpha('#ffffff', 0.32)} 75%)
                  `,
                  backgroundSize: '20px 20px',
                  backgroundPosition: '0 0, 0 10px, 10px -10px, -10px 0px',
                }}
              >
                {attachment.kind === 'image' && attachment.previewURL && !previewFailed ? (
                  <Box
                    component="img"
                    src={attachment.previewURL}
                    alt={`Preview ${attachment.file.name}`}
                    decoding="async"
                    onError={() => {
                      if (attachmentRef.current?.previewURL === attachment.previewURL) {
                        setPreviewFailed(true);
                      }
                    }}
                    sx={{
                      display: 'block',
                      maxWidth: '100%',
                      maxHeight: '100%',
                      objectFit: 'contain',
                      borderRadius: 1,
                      boxShadow: '0 4px 18px rgba(11,20,26,0.18)',
                    }}
                  />
                ) : attachment.kind === 'video' && attachment.previewURL && !previewFailed ? (
                  <Box
                    component="video"
                    src={attachment.previewURL}
                    controls
                    preload="metadata"
                    onError={() => {
                      if (attachmentRef.current?.previewURL === attachment.previewURL) {
                        setPreviewFailed(true);
                      }
                    }}
                    sx={{
                      display: 'block',
                      width: '100%',
                      maxHeight: '100%',
                      borderRadius: 1,
                      bgcolor: '#111',
                      boxShadow: '0 4px 18px rgba(11,20,26,0.18)',
                    }}
                  />
                ) : (
                  <Stack spacing={1} sx={{ alignItems: 'center', maxWidth: 360, textAlign: 'center', px: 2 }}>
                    <Box
                      sx={{
                        width: 76,
                        height: 76,
                        display: 'grid',
                        placeItems: 'center',
                        borderRadius: 2,
                        bgcolor: WA.panel,
                        color: WA.greenDark,
                        boxShadow: '0 4px 16px rgba(11,20,26,0.12)',
                      }}
                    >
                      <InsertDriveFileOutlinedIcon sx={{ fontSize: 40 }} />
                    </Box>
                    <Typography sx={{ fontSize: 14, fontWeight: 700, color: '#111b21', wordBreak: 'break-word' }}>
                      {attachment.file.name}
                    </Typography>
                    {previewFailed && (
                      <Typography sx={{ fontSize: 12, color: WA.meta }}>
                        Browser tidak dapat menampilkan preview file ini, tetapi file tetap dapat dikirim.
                      </Typography>
                    )}
                  </Stack>
                )}
              </Box>

              <Stack
                spacing={1}
                sx={{ p: 1.25, bgcolor: WA.panelHeader, borderTop: `1px solid ${WA.border}` }}
              >
                <ComposerTextarea
                  ref={bindCaptionInput}
                  autoFocus
                  rows={3}
                  maxLength={MAX_COMPOSER_TEXT_LENGTH}
                  aria-label="Caption lampiran"
                  placeholder="Tambahkan caption…"
                  defaultValue=""
                  onInput={(event) => handleDraftInput(event.currentTarget)}
                  onKeyDown={(event) => {
                    if (event.key === 'Enter' && !event.shiftKey && !event.nativeEvent.isComposing) {
                      event.preventDefault();
                      void doSend();
                    }
                  }}
                  onBlur={stopTyping}
                  disabled={isBusy}
                  sx={{
                    height: 88,
                    minHeight: 88,
                    maxHeight: 88,
                    px: 1.25,
                    py: 1,
                    bgcolor: WA.panel,
                    border: `1px solid ${WA.border}`,
                    borderRadius: 2,
                    fontSize: 14.5,
                    lineHeight: '21px',
                    '&:focus': { borderColor: WA.green },
                  }}
                />
                <Stack direction="row" sx={{ justifyContent: 'center' }}>
                  <IconButton
                    onClick={() => void doSend()}
                    disabled={!canSend}
                    aria-label="Kirim lampiran"
                    sx={{
                      width: 46,
                      height: 46,
                      bgcolor: WA.green,
                      color: '#fff',
                      '&:hover': { bgcolor: WA.greenDark },
                      '&.Mui-disabled': { bgcolor: alpha(WA.green, 0.35), color: '#fff' },
                    }}
                  >
                    {isBusy ? <CircularProgress size={20} color="inherit" /> : <SendIcon sx={{ fontSize: 20 }} />}
                  </IconButton>
                </Stack>
              </Stack>
            </DialogContent>
          </>
        )}
      </Dialog>

      {replyTo && (
        <Stack
          direction="row"
          sx={{
            mx: 1.25,
            mb: 0.5,
            px: 1.25,
            py: 0.75,
            alignItems: 'center',
            gap: 1,
            bgcolor: WA.panel,
            borderRadius: 1,
            borderLeft: `4px solid ${WA.green}`,
            boxShadow: '0 1px 2px rgba(11,20,26,0.06)',
          }}
        >
          {/📷|🎥|🎵|🌟|📄|Foto|Video|Audio|Stiker|Dokumen/i.test(replyTo.text) && (
            <Box
              sx={{
                width: 40,
                height: 40,
                borderRadius: 1,
                bgcolor: alpha(WA.green, 0.1),
                color: WA.greenDark,
                display: 'grid',
                placeItems: 'center',
                fontSize: 18,
                flexShrink: 0,
              }}
              aria-hidden
            >
              {/🎥|Video/i.test(replyTo.text) ? '🎥'
                : /🎵|🎤|Audio|suara/i.test(replyTo.text) ? '🎵'
                  : /📄|Dokumen/i.test(replyTo.text) ? '📄'
                    : /🌟|Stiker/i.test(replyTo.text) ? '🌟'
                      : '📷'}
            </Box>
          )}
          <Box sx={{ flex: 1, minWidth: 0 }}>
            <Typography sx={{ fontSize: 12, fontWeight: 700, color: WA.greenDark }}>Membalas</Typography>
            <Typography noWrap sx={{ fontSize: 13, color: WA.meta }}>
              {replyTo.text}
            </Typography>
          </Box>
          <IconButton size="small" onClick={onClearReply} aria-label="Batalkan balasan">
            <CloseIcon sx={{ fontSize: 18 }} />
          </IconButton>
        </Stack>
      )}

      <Stack
        direction="row"
        spacing={{ xs: 0.5, sm: 0.75 }}
        sx={{
          px: { xs: 0.75, sm: 1, md: 1.25 },
          py: { xs: 0.75, md: 0.9 },
          alignItems: 'center',
          bgcolor: WA.panelHeader,
          flexShrink: 0,
          borderTop: `1px solid ${WA.border}`,
          // Jangan memakai CSS paint containment di sini. Safari/WebKit dapat
          // gagal menginvalidasi layer glyph textarea saat input sangat cepat,
          // sehingga value DOM ada tetapi area composer terlihat kosong.
        }}
      >
        <input
          ref={fileInput}
          type="file"
          accept="image/*,video/*,.pdf,.doc,.docx,.xls,.xlsx,.ppt,.pptx,.txt,.zip"
          hidden
          onChange={(e) => {
            const selected = e.target.files?.[0] || null;
            if (selected && !acceptAttachment(selected)) e.target.value = '';
          }}
        />
        <IconButton
          onClick={() => fileInput.current?.click()}
          title="Lampirkan foto, video, atau dokumen"
          aria-label="Lampirkan foto, video, atau dokumen"
          sx={{
            width: 42,
            height: 42,
            flexShrink: 0,
            color: WA.meta,
            bgcolor: alpha('#fff', 0.76),
            border: `1px solid ${alpha(WA.meta, 0.1)}`,
            borderRadius: '50% !important',
            '&:hover': { bgcolor: '#fff', color: WA.greenDark },
          }}
        >
          <AttachFileIcon sx={{ fontSize: 22 }} />
        </IconButton>
        <Box
          sx={{
            flexShrink: 0,
            '& .MuiButton-root': {
              height: 42,
              minWidth: 0,
              px: { xs: 0.9, sm: 1.15 },
              color: WA.greenDark,
              bgcolor: alpha('#fff', 0.76),
              border: `1px solid ${alpha(WA.meta, 0.1)}`,
              borderRadius: '12px !important',
              fontWeight: 700,
              fontSize: 13,
              textTransform: 'none',
              '&:hover': { bgcolor: '#fff', borderColor: alpha(WA.green, 0.28) },
            },
            '& .MuiButton-startIcon': { mr: 0.65 },
          }}
        >
          <TemplatePicker
            agentId={agentId}
            variant="text"
            supportsAttachment
            onPick={handleTemplatePick}
          />
        </Box>
        <Box
          sx={{
            flex: 1,
            minWidth: 0,
            display: 'flex',
            alignItems: 'center',
            bgcolor: '#fff',
            borderRadius: '999px',
            px: 0.55,
            py: 0.25,
            minHeight: 48,
            border: `1px solid ${alpha(WA.meta, 0.16)}`,
            boxShadow: '0 1px 2px rgba(11,20,26,0.04)',
            transition: 'border-color 120ms ease, box-shadow 120ms ease',
            '&:focus-within': {
              borderColor: alpha(WA.green, 0.65),
              boxShadow: `0 0 0 3px ${alpha(WA.green, 0.1)}`,
            },
          }}
        >
          <IconButton
            size="small"
            onClick={(event) => setEmojiAnchor(event.currentTarget)}
            sx={{
              width: 38,
              height: 38,
              flexShrink: 0,
              color: emojiAnchor ? WA.greenDark : WA.meta,
              borderRadius: '50% !important',
              '&:hover': { bgcolor: alpha(WA.green, 0.08), color: WA.greenDark },
            }}
            aria-label="Pilih emoji"
            aria-haspopup="dialog"
            aria-expanded={emojiAnchor ? 'true' : undefined}
          >
            <InsertEmoticonIcon fontSize="small" />
          </IconButton>
          <Popover
            open={!!emojiAnchor}
            anchorEl={emojiAnchor}
            onClose={() => setEmojiAnchor(null)}
            anchorOrigin={{ vertical: 'top', horizontal: 'left' }}
            transformOrigin={{ vertical: 'bottom', horizontal: 'left' }}
            slotProps={{
              paper: {
                sx: {
                  width: 304,
                  maxWidth: 'calc(100vw - 24px)',
                  p: 1,
                  borderRadius: 2,
                  boxShadow: '0 8px 28px rgba(11,20,26,0.18)',
                },
              },
            }}
          >
            <Typography sx={{ px: 0.5, pb: 0.75, fontSize: 12, fontWeight: 700, color: WA.meta }}>
              Emoji
            </Typography>
            <Box sx={{ display: 'grid', gridTemplateColumns: 'repeat(8, 1fr)', gap: 0.25 }}>
              {CHAT_EMOJIS.map((emoji) => (
                <IconButton
                  key={emoji}
                  size="small"
                  onClick={() => insertEmoji(emoji)}
                  aria-label={`Emoji ${emoji}`}
                  sx={{ width: 34, height: 34, fontSize: 21, borderRadius: 1.25 }}
                >
                  {emoji}
                </IconButton>
              ))}
            </Box>
          </Popover>
          <ComposerTextarea
            ref={bindMessageInput}
            data-chatloop-role="composer-input"
            rows={1}
            maxLength={MAX_COMPOSER_TEXT_LENGTH}
            aria-label="Pesan WhatsApp"
            placeholder={file ? 'Caption (opsional)' : 'Ketik pesan'}
            defaultValue=""
            onInput={(event) => handleDraftInput(event.currentTarget)}
            onKeyDown={(event) => {
              if (event.key === 'Enter' && !event.shiftKey && !event.nativeEvent.isComposing) {
                event.preventDefault();
                void doSend();
              }
            }}
            onFocus={() => {
              activateInboxDebugWindow('composer.focus', {
                agent_id: agentId,
                sender_kind: sender.endsWith('@g.us') ? 'group' : 'customer',
              });
              inboxDebugLog('composer.focus', {
                length: messageInput.current?.value.length || 0,
              });
              captureInboxDebugSnapshot('composer-focus');
            }}
            onBlur={() => {
              inboxDebugLog('composer.blur', {
                length: messageInput.current?.value.length || 0,
              });
              stopTyping();
              window.requestAnimationFrame(() => captureInboxDebugSnapshot('composer-after-blur'));
            }}
            sx={{
              height: 42,
              minHeight: 42,
              maxHeight: 42,
              px: 0.65,
              py: '10px',
              fontSize: 14.5,
            }}
          />
        </Box>
        <IconButton
          onClick={() => void doSend()}
          disabled={!canSend}
          aria-label="Kirim pesan"
          sx={{
            bgcolor: WA.green,
            color: '#fff',
            width: 48,
            height: 48,
            flexShrink: 0,
            borderRadius: '50% !important',
            boxShadow: '0 2px 5px rgba(0,128,105,0.2)',
            '&:hover': { bgcolor: WA.greenDark, boxShadow: '0 3px 7px rgba(0,128,105,0.26)' },
            '&.Mui-disabled': { bgcolor: alpha(WA.green, 0.35), color: '#fff' },
          }}
        >
          {isBusy ? <CircularProgress size={20} color="inherit" /> : <SendIcon sx={{ fontSize: 20 }} />}
        </IconButton>
      </Stack>
    </>
  );
});

/* ─── Main panel ────────────────────────────────────────────────────────── */

export default function InboxPanel({
  agentId,
  aiEnabled,
  seed,
  canManageInbox = true,
  notificationSoundEnabled = true,
  onToggleNotificationSound,
  onActiveSenderChange,
  customerTyping = false,
  revokedMessageIDs = EMPTY_REVOKED_MESSAGE_IDS,
}: {
  agentId: number;
  aiEnabled: boolean;
  seed?: { value: string; n: number } | null;
  canManageInbox?: boolean;
  notificationSoundEnabled?: boolean;
  onToggleNotificationSound?: () => void;
  onActiveSenderChange?: (sender: string) => void;
  customerTyping?: boolean;
  revokedMessageIDs?: string[];
}) {
  const theme = useTheme();
  const isMobile = useMediaQuery(theme.breakpoints.down('md'));
  useEffect(() => {
    configureInboxDebugAgent(agentId);
  }, [agentId]);

  const {
    data: contacts,
    isLoading,
    isError: contactsError,
    refetch: refetchContacts,
  } = useContacts(agentId);
  const [sender, setSender] = useState('');
  useEffect(() => {
    sampleInboxComponentCommit('InboxPanel', {
      sender_kind: sender.endsWith('@g.us') ? 'group' : sender ? 'customer' : 'none',
    });
  });
  const { data: agentStatus } = useAgentStatus(agentId);
  const waStatus = agentStatus?.status || '';
  const waStatusKnown = Boolean(waStatus);
  const whatsappConnected = waStatus === 'connected';
  const historySyncUnavailable = waStatusKnown && !whatsappConnected;
  const senderIsGroup = sender.endsWith('@g.us');
  const [mobileShowChat, setMobileShowChat] = useState(false);
  const [search, setSearch] = useState('');
  const [inboxFilter, setInboxFilter] = useState<InboxFilter>('all');
  const [labelFilter, setLabelFilter] = useState('');
  const [contactWindow, setContactWindow] = useState({ key: '', limit: CONTACT_RENDER_BATCH });
  const [olderHistory, setOlderHistory] = useState<ConversationHistoryWindow>({
    sender: '',
    messages: [],
    hasMore: false,
  });
  // Tunda brief sedikit agar request chat utama tidak berebut bandwidth.
  const [secondaryReadySender, setSecondaryReadySender] = useState('');
  const secondaryReady = Boolean(sender) && secondaryReadySender === sender;

  const {
    data: latestConvo,
    isLoading: convoLoading,
    isFetching: convoFetching,
    isError: convoError,
    isPlaceholderData: convoPlaceholder,
    refetch: refetchConversation,
  } = useConversation(agentId, sender);
  const loadOlderConversation = useLoadOlderConversation(agentId);
  const loadOlderConversationAsync = loadOlderConversation.mutateAsync;
  const historySyncQ = useHistorySyncStatus(agentId);
  const requestHistorySync = useRequestHistorySync(agentId);
  const markConversationRead = useMarkConversationRead(agentId);
  const briefQ = useConversationBrief(
    agentId,
    senderIsGroup ? '' : sender,
    secondaryReady && !!sender && !senderIsGroup,
  );
  const refreshBrief = useRefreshConversationBrief(agentId);
  const revokeMsg = useRevokeMessage(agentId);
  const revokeMessage = revokeMsg.mutate;
  const sendMsg = useSendMessage(agentId);
  const sendMedia = useSendMedia(agentId);
  const sendMessage = sendMsg.mutateAsync;
  const sendMediaMessage = sendMedia.mutateAsync;
  const markConversationReadNow = markConversationRead.mutate;
  const resumeBot = useResumeBot(agentId);
  const reanalyzeImage = useReanalyzeImage(agentId);
  const deleteConvo = useDeleteInboxConversation(agentId);
  const deleteConversationNow = deleteConvo.mutateAsync;
  const syncLabels = useLabels(agentId);
  const [deletingSender, setDeletingSender] = useState<string | null>(null);

  const [replyTo, setReplyTo] = useState<{ id: string; text: string } | null>(null);
  const composerInputRef = useRef<HTMLTextAreaElement>(null);
  const observedHistorySyncFinishRef = useRef('');
  const historySyncObserverReadyRef = useRef(false);
  const setComposerInputNode = useCallback((node: HTMLTextAreaElement | null) => {
    composerInputRef.current = node;
  }, []);
  const [visionTarget, setVisionTarget] = useState<ChatMsg | null>(null);
  const [visionInstruction, setVisionInstruction] = useState('');
  const [visionError, setVisionError] = useState('');
  const [contactSidePanel, setContactSidePanel] = useState<ContactSidePanel>(null);
  // Panel sekunder benar-benar null saat ditutup.
  const panelTab = contactSidePanel;
  const selectedContact = useMemo(
    () => contacts?.find((ct) => ct.sender === sender),
    [contacts, sender],
  );
  const selectedName = selectedContact?.name;
  const [copyHint, setCopyHint] = useState('');
  const bottomRef = useRef<HTMLDivElement>(null);
  const chatPaneRef = useRef<HTMLDivElement>(null);
  const chatRef = useRef<HTMLDivElement>(null);
  const olderLoadInFlight = useRef(false);
  const olderScrollAnchor = useRef<{
    sender: string;
    scrollHeight: number;
    scrollTop: number;
  } | null>(null);
  const didFirstScroll = useRef(false);
  const stickToBottom = useRef(true);
  const autoScrollUntil = useRef(0);
  const initialScrollCancelled = useRef(false);
  const revokedMessageIDSet = useMemo(
    () => new Set(revokedMessageIDs.map((messageID) => messageID.trim()).filter(Boolean)),
    [revokedMessageIDs],
  );
  const activeOlderHistory = olderHistory.sender === sender ? olderHistory : undefined;
  const convo = useMemo(() => {
    if (!latestConvo) return undefined;
    const combined = [
      ...(activeOlderHistory?.messages || []),
      ...latestConvo.data,
    ];
    const byIdentity = new Map<string, ChatMsg>();
    for (const message of combined) {
      const waID = (message.wa_msg_id || '').trim();
      const resolvedMessage = waID && revokedMessageIDSet.has(waID)
        ? { ...message, revoked: true }
        : message;
      byIdentity.set(waID ? `wa:${waID}` : `local:${message.id}`, resolvedMessage);
    }
    return {
      ...latestConvo,
      data: Array.from(byIdentity.values()),
      has_more: activeOlderHistory?.hasMore ?? latestConvo.has_more,
      loaded_count: byIdentity.size,
      next_before_at: activeOlderHistory
        ? activeOlderHistory.nextBeforeAt
        : latestConvo.next_before_at,
      next_before_id: activeOlderHistory
        ? activeOlderHistory.nextBeforeID
        : latestConvo.next_before_id,
    };
  }, [activeOlderHistory, latestConvo, revokedMessageIDSet]);
  const loadOlderMessages = useCallback(async () => {
    if (
      !sender
      || olderLoadInFlight.current
      || !convo?.has_more
      || !convo.next_before_at
      || !convo.next_before_id
    ) return;

    const el = chatRef.current;
    if (el) {
      olderScrollAnchor.current = {
        sender,
        scrollHeight: el.scrollHeight,
        scrollTop: el.scrollTop,
      };
    }
    olderLoadInFlight.current = true;
    try {
      const page = await loadOlderConversationAsync({
        sender,
        before_at: convo.next_before_at,
        before_id: convo.next_before_id,
      });
      if (page.sender !== sender) return;
      setOlderHistory((previous) => {
        const previousMessages = previous.sender === sender ? previous.messages : [];
        return {
          sender,
          messages: [...page.data, ...previousMessages],
          nextBeforeAt: page.next_before_at,
          nextBeforeID: page.next_before_id,
          hasMore: page.has_more,
        };
      });
    } catch (error: unknown) {
      olderScrollAnchor.current = null;
      const message = (error as { response?: { data?: { error?: string } } })?.response?.data?.error;
      swalToast(message || 'Pesan lama belum dapat dimuat.', 'error');
    } finally {
      olderLoadInFlight.current = false;
    }
  }, [convo, loadOlderConversationAsync, sender]);
  // Hanya identitas pesan terakhir yang boleh memicu auto-scroll. Panjang array
  // berubah ketika pesan lama ditambahkan di atas dan object convo berubah pada
  // polling meski isinya sama; keduanya tidak boleh menarik viewport pengguna.
  const lastMessage = convo?.data?.length ? convo.data[convo.data.length - 1] : undefined;
  const lastMessageKey = lastMessage
    ? (lastMessage.wa_msg_id || `local:${lastMessage.id}`)
    : 'empty';
  const conversationReady = Boolean(convo && !convoPlaceholder);

  useEffect(() => {
    if (sender || !contacts?.length) return undefined;
    const initial = contacts[0];
    const selectInitial = window.setTimeout(() => {
      setSender(initial.sender);
      onActiveSenderChange?.(initial.sender);
      // Fire-and-forget; hanya jika masih ada unread.
      if ((initial.unread_count ?? 0) > 0) {
        markConversationRead.mutate(initial.sender);
      }
    }, 0);
    return () => window.clearTimeout(selectInitial);
  }, [contacts, isMobile, markConversationRead, onActiveSenderChange, sender]);

  useEffect(() => {
    if (!seed?.value) return undefined;
    const selectSeed = window.setTimeout(() => {
      setSender(seed.value);
      onActiveSenderChange?.(seed.value);
      setMobileShowChat(true);
    }, 0);
    return () => window.clearTimeout(selectSeed);
  }, [seed?.n]); // eslint-disable-line react-hooks/exhaustive-deps

  useEffect(() => {
    didFirstScroll.current = false;
    stickToBottom.current = true;
    initialScrollCancelled.current = false;
    olderScrollAnchor.current = null;
  }, [sender]);

  useLayoutEffect(() => {
    const anchor = olderScrollAnchor.current;
    const el = chatRef.current;
    if (!anchor || !el || anchor.sender !== sender) return;
    const addedHeight = el.scrollHeight - anchor.scrollHeight;
    autoScrollUntil.current = window.performance.now() + 150;
    el.scrollTop = anchor.scrollTop + Math.max(0, addedHeight);
    stickToBottom.current = false;
    olderScrollAnchor.current = null;
  }, [activeOlderHistory?.hasMore, activeOlderHistory?.messages.length, sender]);

  useEffect(() => {
    if (!sender || senderIsGroup || !convo || convoPlaceholder) return;
    // Brief baru dimuat setelah thread utama sudah tampil. Ini mencegah
    // request berat berebut saat operator baru membuka Inbox/chat.
    const timer = window.setTimeout(() => setSecondaryReadySender(sender), 600);
    return () => window.clearTimeout(timer);
  }, [convo, convoPlaceholder, sender, senderIsGroup]);

  useEffect(() => {
    const el = chatRef.current;
    if (!el) return;
    if (!didFirstScroll.current && !conversationReady) return;
    let releaseTimer = 0;
    let verifyTimer = 0;
    let secondFrame = 0;
    const frame = window.requestAnimationFrame(() => {
      secondFrame = window.requestAnimationFrame(() => {
        if (!didFirstScroll.current) {
          autoScrollUntil.current = window.performance.now() + 500;
          // Scroll pertama harus instan. Smooth scroll pada ratusan/ribuan bubble
          // membuat Inbox terlihat macet walaupun datanya sudah selesai dimuat.
          el.scrollTop = el.scrollHeight;
          didFirstScroll.current = true;
          stickToBottom.current = true;
          // Thread pertama kadang baru mencapai tinggi final setelah bubble/media
          // selesai dilukis. Konfirmasi sekali lagi, kecuali user sudah scroll manual.
          verifyTimer = window.setTimeout(() => {
            if (initialScrollCancelled.current) return;
            const distance = el.scrollHeight - el.scrollTop - el.clientHeight;
            if (distance > 4) {
              el.scrollTop = el.scrollHeight;
            }
            stickToBottom.current = true;
          }, 350);
          releaseTimer = window.setTimeout(() => {
            autoScrollUntil.current = 0;
          }, 500);
        } else if (stickToBottom.current) {
          // Pesan baru saat operator berada di dasar: tempel instan. Animasi
          // smooth programatik beradu dengan wheel/trackpad dan terasa bergetar.
          autoScrollUntil.current = window.performance.now() + 120;
          el.scrollTop = el.scrollHeight;
          releaseTimer = window.setTimeout(() => {
            autoScrollUntil.current = 0;
          }, 120);
        }
      });
    });
    return () => {
      window.cancelAnimationFrame(frame);
      if (secondFrame) window.cancelAnimationFrame(secondFrame);
      if (releaseTimer) window.clearTimeout(releaseTimer);
      if (verifyTimer) window.clearTimeout(verifyTimer);
    };
  }, [conversationReady, lastMessageKey, sender]);

  const handleChatScroll = useCallback(() => {
    const el = chatRef.current;
    if (!el) return;
    if (window.performance.now() < autoScrollUntil.current) return;
    // Ambang kecil: ketika operator baru naik sedikit, pesan/refetch baru tidak
    // boleh memaksa viewport kembali ke bawah.
    stickToBottom.current = el.scrollHeight - el.scrollTop - el.clientHeight < 40;
    if (
      didFirstScroll.current
      && el.scrollTop <= 140
      && el.scrollHeight > el.clientHeight + 20
      && convo?.has_more
      && !loadOlderConversation.isPending
    ) {
      void loadOlderMessages();
    }
  }, [convo?.has_more, loadOlderConversation.isPending, loadOlderMessages]);

  const handleUserScrollStart = useCallback(() => {
    autoScrollUntil.current = 0;
    initialScrollCancelled.current = true;
  }, []);

  // Index pesan untuk quote preview + navigasi (wa_msg_id, id lokal, lowercase, local:).
  const replyIndex = useMemo(() => {
    const byKey = new Map<string, ChatMsg>();
    const msgs = convo?.data || [];
    for (const m of msgs) {
      byKey.set(String(m.id), m);
      byKey.set(`local:${m.id}`, m);
      const wa = (m.wa_msg_id || '').trim();
      if (wa) {
        byKey.set(wa, m);
        byKey.set(wa.toLowerCase(), m);
      }
    }
    return byKey;
  }, [convo?.data]);

  const resolveReplyPreview = useCallback((replyTo?: string, replyText?: string): ReplyPreview | null => {
    const rawId = (replyTo || '').trim();
    if (!rawId && !replyText) return null;
    const localKey = rawId.startsWith('local:') ? rawId.slice(6) : rawId;
    let hit =
      (rawId && (replyIndex.get(rawId) || replyIndex.get(rawId.toLowerCase()) || replyIndex.get(localKey)))
      || undefined;
    // Fallback fuzzy: cocokkan suffix stanza id (WA kadang beda casing/prefix).
    if (!hit && rawId.length >= 6 && !rawId.startsWith('local:')) {
      for (const m of convo?.data || []) {
        const wa = (m.wa_msg_id || '').trim();
        if (!wa) continue;
        if (wa === rawId || wa.toLowerCase() === rawId.toLowerCase() || wa.endsWith(rawId) || rawId.endsWith(wa)) {
          hit = m;
          break;
        }
      }
    }
    // Fallback posisi: data lama — cari media/teks terdekat sebelum pesan yang mereply id ini.
    if (!hit && rawId) {
      const msgs = convo?.data || [];
      let quoterIdx = -1;
      for (let i = 0; i < msgs.length; i++) {
        const rt = (msgs[i].reply_to || '').trim();
        if (!rt) continue;
        const norm = rt.startsWith('local:') ? rt.slice(6) : rt;
        if (norm === rawId || norm === localKey || rt === rawId || rt.toLowerCase() === rawId.toLowerCase()) {
          quoterIdx = i;
          break;
        }
      }
      if (quoterIdx < 0 && (replyText || '').trim()) {
        // Cari quoter lewat reply_text sama (kadang reply_to rusak).
        for (let i = 0; i < msgs.length; i++) {
          if ((msgs[i].reply_text || '').trim() === (replyText || '').trim() && msgs[i].reply_to) {
            quoterIdx = i;
            break;
          }
        }
      }
      if (quoterIdx > 0) {
        const wantMedia = !replyText || /📷|🎥|🎵|🌟|📄|Foto|Video|Audio|Stiker|Dokumen|Pesan/i.test(replyText);
        for (let j = quoterIdx - 1; j >= 0 && j >= quoterIdx - 15; j--) {
          if (wantMedia && (msgs[j].media_type || msgs[j].media_downloadable)) {
            hit = msgs[j];
            break;
          }
          if (!wantMedia && mediaPreviewLabel(msgs[j]) === (replyText || '').trim()) {
            hit = msgs[j];
            break;
          }
        }
        if (!hit) {
          for (let j = quoterIdx - 1; j >= 0 && j >= quoterIdx - 8; j--) {
            hit = msgs[j];
            break;
          }
        }
      }
    }
    if (hit) {
      return {
        id: hit.wa_msg_id || `local:${hit.id}`,
        text: mediaPreviewLabel(hit) || (replyText || '').trim() || '💬 Pesan',
        mediaType: hit.media_type || undefined,
        localId: hit.id,
        mediaDownloadable: Boolean(hit.media_downloadable),
        fromHuman: Boolean(hit.from_human),
      };
    }
    const text = (replyText || '').trim() || '💬 Pesan';
    const mediaGuess = /^📷|^🎥|^🎵|^🌟|^📄|Foto|Video|Audio|Stiker|Dokumen/i.test(text)
      ? (/video/i.test(text) ? 'video' : /audio|suara/i.test(text) ? 'audio' : /dokumen|📄/i.test(text) ? 'document' : /stiker|🌟/i.test(text) ? 'sticker' : 'image')
      : undefined;
    return {
      id: rawId,
      text,
      mediaType: mediaGuess,
    };
  }, [convo?.data, replyIndex]);

  const scrollToMessageNode = useCallback((target: HTMLElement) => {
    stickToBottom.current = false;
    autoScrollUntil.current = 0;
    initialScrollCancelled.current = true;
    window.requestAnimationFrame(() => {
      target.scrollIntoView({ behavior: 'smooth', block: 'center' });
      target.animate(
        [
          { backgroundColor: 'rgba(0, 168, 132, 0)' },
          { backgroundColor: 'rgba(0, 168, 132, 0.28)' },
          { backgroundColor: 'rgba(0, 168, 132, 0)' },
        ],
        { duration: 1400, easing: 'ease-out' },
      );
    });
  }, []);

  // Handler navigasi diteruskan ke setiap bubble. Simpan data lookup terbaru
  // di ref agar identitas handler tetap stabil ketika satu pesan ditambahkan;
  // tanpa ini React.memo pada seluruh MessageBlock menjadi tidak efektif.
  const replyNavigationRef = useRef({
    messages: convo?.data || [],
    index: replyIndex,
    resolve: resolveReplyPreview,
  });
  useLayoutEffect(() => {
    replyNavigationRef.current = {
      messages: convo?.data || [],
      index: replyIndex,
      resolve: resolveReplyPreview,
    };
  }, [convo?.data, replyIndex, resolveReplyPreview]);

  const handleNavigateReply = useCallback((messageID: string) => {
    if (!messageID || !chatRef.current) return;
    const navigation = replyNavigationRef.current;
    let needle = messageID.trim();
    if (needle.startsWith('local:')) needle = needle.slice(6);
    const needleLower = needle.toLowerCase();

    const fromIndex =
      navigation.index.get(messageID.trim())
      || navigation.index.get(needle)
      || navigation.index.get(needleLower)
      || navigation.index.get(`local:${needle}`);
    let fuzzyMsg: ChatMsg | undefined;
    if (!fromIndex && needle.length >= 6) {
      for (const m of navigation.messages) {
        const wa = (m.wa_msg_id || '').trim();
        if (!wa) continue;
        if (
          wa === needle
          || wa.toLowerCase() === needleLower
          || wa.endsWith(needle)
          || needle.endsWith(wa)
        ) {
          fuzzyMsg = m;
          break;
        }
      }
    }
    const resolved = fromIndex || fuzzyMsg;
    // Prefer id lokal dari preview resolve (termasuk fallback posisi).
    const preview = navigation.resolve(messageID, '');
    const preferredLocal = String(resolved?.id || preview?.localId || '');

    const nodes = Array.from(chatRef.current.querySelectorAll<HTMLElement>('[data-local-message-id]'));
    const target = nodes.find((element) => {
      const localId = (element.dataset.localMessageId || '').trim();
      const waId = (element.dataset.waMsgId || element.dataset.messageId || '').trim();
      if (preferredLocal && localId === preferredLocal) return true;
      if (localId === needle || waId === needle) return true;
      if (localId && localId.toLowerCase() === needleLower) return true;
      if (waId && waId.toLowerCase() === needleLower) return true;
      if (waId && needle.length >= 6 && (waId.endsWith(needle) || needle.endsWith(waId))) return true;
      return false;
    });
    if (target) {
      scrollToMessageNode(target);
      return;
    }

    const msgs = navigation.messages;
    let quoterIdx = -1;
    for (let i = 0; i < msgs.length; i++) {
      const rt = (msgs[i].reply_to || '').trim();
      if (!rt) continue;
      const norm = rt.startsWith('local:') ? rt.slice(6) : rt;
      if (norm === needle || norm.toLowerCase() === needleLower || rt === messageID.trim()) {
        quoterIdx = i;
        break;
      }
    }
    if (quoterIdx > 0) {
      for (let j = quoterIdx - 1; j >= 0 && j >= quoterIdx - 15; j--) {
        if (!msgs[j].media_type && !msgs[j].media_downloadable) continue;
        const local = String(msgs[j].id);
        const node = nodes.find((el) => el.dataset.localMessageId === local);
        if (node) {
          scrollToMessageNode(node);
          return;
        }
      }
      // Teks terdekat sebelum quote.
      for (let j = quoterIdx - 1; j >= 0 && j >= quoterIdx - 5; j--) {
        const node = nodes.find((el) => el.dataset.localMessageId === String(msgs[j].id));
        if (node) {
          scrollToMessageNode(node);
          return;
        }
      }
    }

    swalToast('Pesan yang dirujuk tidak ditemukan di 200 pesan terbaru.', 'info');
  }, [scrollToMessageNode]);

  useEffect(() => {
    const finishedAt = historySyncQ.data?.finished_at || '';
    // Status `completed` tetap dipolling dan `finished_at` lama tersimpan di
    // response. Tanpa guard, perubahan sender menjalankan effect ini lagi lalu
    // menembakkan refetch kedua bersamaan dengan fetch chat yang baru dipilih.
    if (!historySyncObserverReadyRef.current) {
      historySyncObserverReadyRef.current = true;
      observedHistorySyncFinishRef.current = finishedAt;
      return;
    }
    if (!finishedAt || observedHistorySyncFinishRef.current === finishedAt) return;
    observedHistorySyncFinishRef.current = finishedAt;
    inboxDebugLog('history-sync.finish-refresh', {
      sync_sender_matches_active: Boolean(
        !historySyncQ.data?.sender || historySyncQ.data.sender === sender,
      ),
    });
    void refetchContacts();
    if (sender && (!historySyncQ.data?.sender || historySyncQ.data.sender === sender)) {
      void refetchConversation();
    }
  }, [
    historySyncQ.data?.finished_at,
    historySyncQ.data?.sender,
    refetchContacts,
    refetchConversation,
    sender,
  ]);

  const availableLabels = useMemo(() => {
    const map = new Map<string, { label_id: string; name: string; color: number; count: number }>();
    for (const contact of contacts || []) {
      for (const label of contact.labels || []) {
        const current = map.get(label.label_id);
        map.set(label.label_id, { ...label, count: (current?.count || 0) + 1 });
      }
    }
    return Array.from(map.values()).sort((a, b) => a.name.localeCompare(b.name, 'id'));
  }, [contacts]);

  const filterCounts = useMemo(() => {
    const list = contacts || [];
    return {
      unread: list.filter((contact) => (contact.unread_count || 0) > 0).length,
      read: list.filter((contact) => (contact.unread_count || 0) === 0).length,
      handoff: list.filter((contact) => contact.needs_human).length,
      groups: list.filter((contact) => contact.is_group).length,
    };
  }, [contacts]);

  const filteredContacts = useMemo(() => {
    const list = contacts || [];
    const sourceOrder = new Map(list.map((contact, index) => [contact.sender, index]));
    const q = search.trim().toLowerCase();
    return list
      .filter((c) => {
        const unread = c.unread_count || 0;
        if (inboxFilter === 'unread' && unread === 0) return false;
        if (inboxFilter === 'read' && unread > 0) return false;
        if (inboxFilter === 'handoff' && !c.needs_human) return false;
        if (inboxFilter === 'groups' && !c.is_group) return false;
        if (labelFilter && !(c.labels || []).some((label) => label.label_id === labelFilter)) return false;
        if (!q) return true;
        const name = (c.name || '').toLowerCase();
        const num = c.sender.toLowerCase();
        const msg = (c.last_msg || '').toLowerCase();
        return name.includes(q) || num.includes(q) || msg.includes(q);
      })
      .sort((left, right) => {
        const leftTime = parseMsgDate(left.last_at)?.getTime() ?? Number.NEGATIVE_INFINITY;
        const rightTime = parseMsgDate(right.last_at)?.getTime() ?? Number.NEGATIVE_INFINITY;
        if (leftTime !== rightTime) return rightTime - leftTime;
        return (sourceOrder.get(left.sender) ?? 0) - (sourceOrder.get(right.sender) ?? 0);
      });
  }, [contacts, inboxFilter, labelFilter, search]);
  const contactWindowKey = `${inboxFilter}\u0000${labelFilter}\u0000${search.trim().toLowerCase()}`;
  const contactRenderLimit = contactWindow.key === contactWindowKey
    ? contactWindow.limit
    : CONTACT_RENDER_BATCH;
  const visibleContacts = useMemo(
    () => filteredContacts.slice(0, contactRenderLimit),
    [contactRenderLimit, filteredContacts],
  );
  const handleContactListScroll = useCallback((event: UIEvent<HTMLDivElement>) => {
    const el = event.currentTarget;
    const remaining = el.scrollHeight - el.scrollTop - el.clientHeight;
    if (remaining > 220 || contactRenderLimit >= filteredContacts.length) return;
    setContactWindow((previous) => {
      const currentLimit = previous.key === contactWindowKey
        ? previous.limit
        : CONTACT_RENDER_BATCH;
      return {
        key: contactWindowKey,
        limit: Math.min(filteredContacts.length, currentLimit + CONTACT_RENDER_BATCH),
      };
    });
  }, [contactRenderLimit, contactWindowKey, filteredContacts.length]);

  const headerSubtitle = useMemo(() => {
    if (historySyncUnavailable) {
      return waStatus === 'connecting' || waStatus === 'pairing'
        ? 'WhatsApp menyambung ulang · data dapat terlambat'
        : 'WhatsApp offline · riwayat mungkin belum lengkap';
    }
    if (historySyncQ.data?.state === 'syncing' && (!historySyncQ.data.sender || historySyncQ.data.sender === sender)) {
      return 'menyinkronkan chat lama…';
    }
    if (convoFetching) return 'memperbarui…';
    if (senderIsGroup) return 'Grup WhatsApp · AI tidak membalas otomatis';
    if (selectedName) return `+${sender}`;
    return `+${sender}`;
  }, [convoFetching, historySyncQ.data, historySyncUnavailable, selectedName, sender, senderIsGroup, waStatus]);

  const syncStatusForChat = useMemo(() => {
    const status = historySyncQ.data;
    if (!status || (status.sender && status.sender !== sender)) return undefined;
    return status;
  }, [historySyncQ.data, sender]);

  const startHistorySync = useCallback(async () => {
    if (!sender) return;
    if (historySyncUnavailable) {
      swalToast('WhatsApp belum online. Tautkan atau tunggu tersambung kembali sebelum menyinkronkan riwayat.', 'warning');
      return;
    }
    try {
      const res = await requestHistorySync.mutateAsync(sender) as {
        message?: string;
        data?: HistorySyncStatus;
      };
      const st = res?.data;
      const msg = st?.message || res?.message;
      if (st?.state === 'failed') {
        swalToast(msg || 'Sinkronisasi gagal.', 'error');
      } else if ((st?.imported || 0) > 0) {
        swalToast(msg || `${st?.imported} pesan ditambahkan.`);
      } else if (msg && !st?.still_stale) {
        swalToast(msg, 'info');
      }
      // still_stale: cukup banner singkat di header (auto-hide), jangan toast menakutkan.
    } catch (error: unknown) {
      const msg = (error as { response?: { data?: { error?: string } } })?.response?.data?.error;
      swalToast(msg || 'Sinkronisasi chat belum dapat dimulai.', 'error');
    }
  }, [historySyncUnavailable, requestHistorySync, sender]);

  const copyNumber = useCallback(async () => {
    if (!sender) return;
    try {
      await navigator.clipboard.writeText(senderIsGroup ? sender : (sender.startsWith('+') ? sender : `+${sender}`));
      setCopyHint(senderIsGroup ? 'ID grup disalin' : 'Nomor disalin');
      window.setTimeout(() => setCopyHint(''), 1800);
    } catch {
      setCopyHint('Gagal menyalin');
      window.setTimeout(() => setCopyHint(''), 1800);
    }
  }, [sender, senderIsGroup]);

  // Event contact harus mempunyai identitas stabil. Jika callback berganti pada
  // setiap perubahan sender/query, React.memo pada seluruh ContactRow menjadi
  // tidak berguna dan satu perpindahan chat merender ulang hingga 80 baris.
  const contactActionRuntimeRef = useRef({
    sender,
    contacts,
    onActiveSenderChange,
    markRead: markConversationReadNow,
    deleteConversation: deleteConversationNow,
  });
  useLayoutEffect(() => {
    contactActionRuntimeRef.current = {
      sender,
      contacts,
      onActiveSenderChange,
      markRead: markConversationReadNow,
      deleteConversation: deleteConversationNow,
    };
  }, [
    contacts,
    deleteConversationNow,
    markConversationReadNow,
    onActiveSenderChange,
    sender,
  ]);

  const selectContact = useCallback((s: string) => {
    const runtime = contactActionRuntimeRef.current;
    const debugDetails = {
      changed: s !== runtime.sender,
      from_kind: runtime.sender.endsWith('@g.us') ? 'group' : runtime.sender ? 'customer' : 'none',
      to_kind: s.endsWith('@g.us') ? 'group' : 'customer',
    };
    activateInboxDebugWindow('contact.select', debugDetails);
    inboxDebugLog('contact.select', debugDetails);
    if (s === runtime.sender && chatRef.current) {
      initialScrollCancelled.current = false;
      autoScrollUntil.current = window.performance.now() + 250;
      stickToBottom.current = true;
      chatRef.current.scrollTop = chatRef.current.scrollHeight;
      window.setTimeout(() => { autoScrollUntil.current = 0; }, 250);
      return;
    }
    // UI dulu — request menyusul; terasa instan.
    setSender(s);
    runtime.onActiveSenderChange?.(s);
    setMobileShowChat(true);
    setReplyTo(null);
    setContactSidePanel(null);
    const unread = runtime.contacts?.find((c) => c.sender === s)?.unread_count ?? 0;
    // Skip mark-read bila sudah dibaca: hemat round-trip server + WA.
    if (unread > 0) {
      runtime.markRead(s);
    }
  }, [isMobile]);

  const deleteConversation = useCallback(async (target: string) => {
    const startRuntime = contactActionRuntimeRef.current;
    const label = startRuntime.contacts?.find((c) => c.sender === target)?.name || `+${target}`;
    const ok = await swalConfirm(
      `Hapus chat ${label}?`,
      'Riwayat percakapan dihapus dari Inbox (termasuk media di server). Data kontak CRM tidak dihapus. Tindakan ini tidak bisa dibatalkan.',
    );
    if (!ok) return;
    setDeletingSender(target);
    try {
      await startRuntime.deleteConversation(target);
      setOlderHistory((previous) => previous.sender === target
        ? { sender: '', messages: [], hasMore: false }
        : previous);
      const latestRuntime = contactActionRuntimeRef.current;
      if (latestRuntime.sender === target) {
        setSender('');
        latestRuntime.onActiveSenderChange?.('');
        setMobileShowChat(false);
        setContactSidePanel(null);
        setReplyTo(null);
      }
      swalToast('Chat dihapus dari inbox.');
    } catch (error: unknown) {
      const msg = (error as { response?: { data?: { error?: string } } })?.response?.data?.error;
      swalToast(msg || 'Gagal menghapus chat.', 'error');
    } finally {
      setDeletingSender(null);
    }
  }, []);

  const handleReply = useCallback((id: string, t: string) => {
    setReplyTo({ id, text: t });
  }, []);

  const handleRevoke = useCallback(
    (msgId: string) => {
      if (!sender) return;
      revokeMessage({ msgId, to: sender });
    },
    [revokeMessage, sender],
  );

  const handleVision = useCallback((m: ChatMsg) => {
    setVisionTarget(m);
    setVisionInstruction('');
    setVisionError('');
  }, []);

  const clearReply = useCallback(() => setReplyTo(null), []);

  const handleComposerSend = useCallback(async (payload: {
    text: string;
    file: File | null;
    replyTo: { id: string; text: string } | null;
  }) => {
    if (!sender) return;
    if (payload.file) {
      await sendMediaMessage({ to: sender, file: payload.file, caption: payload.text });
      setReplyTo(null);
      return;
    }
    if (!payload.text) return;
    await sendMessage({
      to: sender,
      message: payload.text,
      reply_to: payload.replyTo?.id || '',
      reply_text: payload.replyTo?.text || '',
    });
    setReplyTo(null);
  }, [sender, sendMediaMessage, sendMessage]);

  const runReanalysis = async () => {
    if (!visionTarget) return;
    setVisionError('');
    try {
      await reanalyzeImage.mutateAsync({ messageId: visionTarget.id, instruction: visionInstruction.trim() });
      setVisionTarget(null);
      setVisionInstruction('');
    } catch (error: unknown) {
      const msg = (error as { response?: { data?: { error?: string } } })?.response?.data?.error;
      setVisionError(msg || 'Analisis ulang belum berhasil.');
    }
  };

  const showList = !isMobile || !mobileShowChat;
  const showChat = !isMobile || mobileShowChat;

  if (isLoading && !contacts) {
    return (
      <Box sx={{ flex: 1, display: 'grid', placeItems: 'center', bgcolor: '#f0f2f5' }}>
        <Stack spacing={1.25} sx={{ alignItems: 'center' }}>
          <CircularProgress size={28} sx={{ color: WA.green }} />
          <Typography sx={{ color: WA.meta, fontSize: 13.5 }}>Memuat inbox…</Typography>
        </Stack>
      </Box>
    );
  }

  return (
    <Box
      sx={{
        flex: 1,
        minHeight: 0,
        display: 'flex',
        position: 'relative',
        bgcolor: '#f0f2f5',
        overflow: 'hidden',
      }}
    >
      {/* ── Sidebar ───────────────────────────────────────────────────── */}
      {showList && (
        <Box
          sx={{
            width: { xs: '100%', md: 360 },
            maxWidth: { md: 420 },
            flexShrink: 0,
            display: 'flex',
            flexDirection: 'column',
            bgcolor: WA.panel,
            borderRight: { md: `1px solid ${WA.border}` },
            minHeight: 0,
          }}
        >
          <Stack
            direction="row"
            sx={{
              px: 1.5,
              py: 1.25,
              alignItems: 'center',
              justifyContent: 'space-between',
              bgcolor: WA.panelHeader,
              borderBottom: `1px solid ${WA.border}`,
              minHeight: 60,
            }}
          >
            <Typography sx={{ fontWeight: 750, fontSize: 18, color: '#111b21' }}>Inbox</Typography>
            <Stack direction="row" spacing={0.5} sx={{ alignItems: 'center' }}>
              <Tooltip title={notificationSoundEnabled ? 'Suara notifikasi aktif' : 'Suara notifikasi nonaktif'}>
                <IconButton
                  size="small"
                  onClick={onToggleNotificationSound}
                  aria-label={notificationSoundEnabled ? 'Nonaktifkan suara notifikasi' : 'Aktifkan suara notifikasi'}
                  sx={{
                    width: 32,
                    height: 32,
                    color: notificationSoundEnabled ? WA.greenDark : WA.meta,
                    bgcolor: notificationSoundEnabled ? alpha(WA.green, 0.08) : 'transparent',
                  }}
                >
                  {notificationSoundEnabled
                    ? <VolumeUpOutlinedIcon sx={{ fontSize: 19 }} />
                    : <VolumeOffOutlinedIcon sx={{ fontSize: 19 }} />}
                </IconButton>
              </Tooltip>
            </Stack>
          </Stack>

          <WhatsAppConnectionNotice status={waStatus} />

          {historySyncQ.data && historySyncQ.data.state !== 'idle' && !historySyncQ.data.sender && (
            <HistorySyncNotice status={historySyncQ.data} />
          )}

          <Box sx={{ px: 1.5, py: 1.1, bgcolor: WA.panel, borderBottom: `1px solid ${WA.border}` }}>
            <TextField
              fullWidth
              size="small"
              placeholder="Cari atau mulai chat baru"
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              slotProps={{
                input: {
                  startAdornment: (
                    <InputAdornment position="start">
                      <SearchIcon sx={{ fontSize: 18, color: WA.meta }} />
                    </InputAdornment>
                  ),
                },
              }}
              sx={{
                '& .MuiOutlinedInput-root': {
                  bgcolor: WA.searchBg,
                  borderRadius: 2,
                  fontSize: 14,
                  '& fieldset': { border: 'none' },
                },
              }}
            />
            <Box
              sx={{
                display: 'flex',
                gap: 0.65,
                mt: 0.8,
                pb: 0.15,
                overflowX: 'auto',
                scrollbarWidth: 'none',
                '&::-webkit-scrollbar': { display: 'none' },
              }}
            >
              {([
                ['all', 'Semua', contacts?.length || 0],
                ['unread', 'Belum dibaca', filterCounts.unread],
                ['read', 'Sudah dibaca', filterCounts.read],
                ['handoff', 'Butuh CS', filterCounts.handoff],
                ['groups', 'Grup', filterCounts.groups],
              ] as Array<[InboxFilter, string, number]>).map(([value, label, count]) => (
                <Chip
                  key={value}
                  size="small"
                  label={`${label}${count ? ` ${count}` : ''}`}
                  onClick={() => {
                    setInboxFilter(value);
                    if (value === 'all') setLabelFilter('');
                  }}
                  variant={inboxFilter === value && (!labelFilter || value !== 'all') ? 'filled' : 'outlined'}
                  sx={{
                    height: 27,
                    flexShrink: 0,
                    fontSize: 11.5,
                    fontWeight: inboxFilter === value ? 700 : 500,
                    bgcolor: inboxFilter === value ? alpha(WA.green, 0.12) : '#fff',
                    color: inboxFilter === value ? WA.greenDark : WA.meta,
                    borderColor: inboxFilter === value ? alpha(WA.green, 0.35) : WA.border,
                  }}
                />
              ))}
              {availableLabels.map((label) => {
                const active = labelFilter === label.label_id;
                const color = waLabelColor(label.color);
                return (
                  <Chip
                    key={label.label_id}
                    size="small"
                    icon={<LabelOutlinedIcon sx={{ fontSize: '14px !important', color: `${color} !important` }} />}
                    label={`${label.name} ${label.count}`}
                    onClick={() => setLabelFilter(active ? '' : label.label_id)}
                    variant={active ? 'filled' : 'outlined'}
                    sx={{
                      height: 27,
                      flexShrink: 0,
                      maxWidth: 150,
                      fontSize: 11.5,
                      fontWeight: active ? 700 : 500,
                      bgcolor: active ? alpha(color, 0.12) : '#fff',
                      color: active ? color : WA.meta,
                      borderColor: active ? alpha(color, 0.4) : WA.border,
                    }}
                  />
                );
              })}
              <Chip
                size="small"
                icon={syncLabels.isPending
                  ? <CircularProgress size={12} sx={{ color: WA.meta }} />
                  : <SyncIcon sx={{ fontSize: '14px !important' }} />}
                label="Sinkron label"
                disabled={syncLabels.isPending}
                onClick={() => {
                  void syncLabels.mutateAsync()
                    .then(() => refetchContacts())
                    .catch(() => swalToast('Label WhatsApp belum dapat disinkronkan.', 'warning'));
                }}
                variant="outlined"
                sx={{ height: 27, flexShrink: 0, fontSize: 11.5, color: WA.meta, borderColor: WA.border }}
              />
            </Box>
          </Box>

          <Box
            onScroll={handleContactListScroll}
            sx={{ flex: 1, minHeight: 0, overflowY: 'auto', overscrollBehavior: 'contain' }}
          >
            {contactsError && !contacts ? (
              <Stack spacing={1} sx={{ alignItems: 'center', px: 2, py: 3, textAlign: 'center' }}>
                <Typography sx={{ color: WA.meta, fontSize: 13.5 }}>
                  Inbox belum dapat dimuat. Tampilan tetap aktif dan bisa dicoba kembali.
                </Typography>
                <Button size="small" variant="outlined" onClick={() => void refetchContacts()}>
                  Coba lagi
                </Button>
              </Stack>
            ) : filteredContacts.length === 0 ? (
              <Typography sx={{ p: 3, color: WA.meta, fontSize: 14, textAlign: 'center' }}>
                {search || inboxFilter !== 'all' || labelFilter ? 'Tidak ada chat yang cocok dengan filter.' : 'Belum ada percakapan.'}
              </Typography>
            ) : (
              <>
                {visibleContacts.map((ct) => (
                  <ContactRow
                    key={ct.sender}
                    ct={ct}
                    selected={ct.sender === sender && (!isMobile || mobileShowChat)}
                    onSelect={selectContact}
                    onDelete={canManageInbox ? deleteConversation : undefined}
                    deleting={deletingSender === ct.sender}
                    agentId={agentId}
                    mediaToken={convo?.media_token || ''}
                    aiEnabled={aiEnabled}
                  />
                ))}
                {visibleContacts.length < filteredContacts.length && (
                  <Typography
                    sx={{
                      py: 1,
                      textAlign: 'center',
                      color: WA.meta,
                      fontSize: 11.5,
                      borderBottom: `1px solid ${WA.border}`,
                    }}
                  >
                    Scroll ke bawah untuk memuat chat berikutnya
                  </Typography>
                )}
              </>
            )}
          </Box>
        </Box>
      )}

      {/* ── Chat pane ─────────────────────────────────────────────────── */}
      {showChat && (
        <Box
          ref={chatPaneRef}
          sx={{
            flex: 1,
            minWidth: 0,
            minHeight: 0,
            display: 'flex',
            flexDirection: 'column',
            position: 'relative',
            bgcolor: WA.chatBg,
            // Subtle WA wallpaper noise via layered gradient
            backgroundImage: `
              linear-gradient(${alpha('#d1d7db', 0.35)}, ${alpha('#d1d7db', 0.35)}),
              url("data:image/svg+xml,%3Csvg width='60' height='60' viewBox='0 0 60 60' xmlns='http://www.w3.org/2000/svg'%3E%3Cg fill='none' fill-rule='evenodd'%3E%3Cg fill='%23c5bbb0' fill-opacity='0.18'%3E%3Cpath d='M36 34v-4h-2v4h-4v2h4v4h2v-4h4v-2h-4zm0-30V0h-2v4h-4v2h4v4h2V6h4V4h-4zM6 34v-4H4v4H0v2h4v4h2v-4h4v-2H6zM6 4V0H4v4H0v2h4v4h2V6h4V4H6z'/%3E%3C/g%3E%3C/g%3E%3C/svg%3E")
            `,
          }}
        >
          {!sender ? (
            <Box
              sx={{
                flex: 1,
                display: 'flex',
                flexDirection: 'column',
                alignItems: 'center',
                justifyContent: 'center',
                bgcolor: '#f0f2f5',
                borderBottom: { md: '6px solid #00a884' },
                px: 3,
                textAlign: 'center',
              }}
            >
              <Box
                sx={{
                  width: 72,
                  height: 72,
                  borderRadius: '50%',
                  bgcolor: alpha(WA.green, 0.12),
                  display: 'grid',
                  placeItems: 'center',
                  mb: 2,
                }}
              >
                <SmartToyIcon sx={{ fontSize: 36, color: WA.greenDark }} />
              </Box>
              <Typography sx={{ fontWeight: 300, fontSize: 28, color: '#41525d', mb: 1 }}>
                ChatLoop Inbox
              </Typography>
              <Typography sx={{ maxWidth: 420, color: WA.meta, fontSize: 14, lineHeight: 1.5 }}>
                Pilih percakapan di kiri untuk membalas pelanggan. Chat AI, CS, dan pelanggan digabung dalam satu thread.
              </Typography>
            </Box>
          ) : (
            <>
              {/* Header — area nama/avatar bisa dibuka untuk info kontak */}
              <Stack
                direction="row"
                sx={{
                  px: { xs: 1.25, md: 1.75 },
                  py: 1,
                  alignItems: 'center',
                  gap: 1,
                  bgcolor: WA.panelHeader,
                  borderBottom: `1px solid ${WA.border}`,
                  minHeight: 64,
                  flexShrink: 0,
                }}
              >
                {isMobile && (
                  <IconButton size="small" onClick={() => setMobileShowChat(false)} aria-label="Kembali">
                    <ArrowBackIcon />
                  </IconButton>
                )}
                <Box
                  component="button"
                  type="button"
                  onClick={() => setContactSidePanel('details')}
                  aria-label={senderIsGroup ? 'Buka info grup' : 'Buka detail pelanggan'}
                  sx={{
                    appearance: 'none',
                    border: 0,
                    bgcolor: 'transparent',
                    display: 'flex',
                    alignItems: 'center',
                    gap: 1,
                    flex: 1,
                    minWidth: 0,
                    textAlign: 'left',
                    cursor: 'pointer',
                    borderRadius: 1,
                    py: 0.25,
                    px: 0.25,
                    '&:hover': { bgcolor: alpha('#000', 0.04) },
                  }}
                >
                  <Avatar
                    src={senderIsGroup ? undefined : profilePictureURL(agentId, sender, convo?.media_token || '')}
                    sx={{
                      width: 40,
                      height: 40,
                      fontSize: 16,
                      fontWeight: 600,
                      bgcolor: avatarColor(sender),
                      color: '#fff',
                      flexShrink: 0,
                    }}
                  >
                    {senderIsGroup ? <GroupsOutlinedIcon /> : (selectedName || sender).charAt(0).toUpperCase()}
                  </Avatar>
                  <Box sx={{ flex: 1, minWidth: 0 }}>
                    <Typography noWrap sx={{ fontWeight: 600, fontSize: 16, color: '#111b21', lineHeight: 1.25 }}>
                      {selectedName || (senderIsGroup ? 'Grup WhatsApp' : `+${sender}`)}
                    </Typography>
                    <Typography noWrap sx={{ fontSize: 12.5, color: WA.meta, lineHeight: 1.2 }}>
                      {headerSubtitle}
                    </Typography>
                  </Box>
                </Box>
                <Tooltip
                  title={historySyncUnavailable
                    ? 'WhatsApp harus online sebelum riwayat dapat disinkronkan'
                    : senderIsGroup
                      ? 'Sinkronkan hingga 100 pesan grup terbaru dari HP'
                      : 'Sinkronkan riwayat lengkap dari HP (semua pesan yang tersedia, bukan cuma 50)'}
                >
                  <span>
                    <IconButton
                      size="small"
                      onClick={() => { void startHistorySync(); }}
                      disabled={historySyncUnavailable || requestHistorySync.isPending || historySyncQ.data?.state === 'syncing'}
                      aria-label={senderIsGroup ? 'Sinkronkan pesan grup terbaru' : 'Sinkronkan riwayat lengkap'}
                      sx={{ color: WA.meta }}
                    >
                      {requestHistorySync.isPending || (historySyncQ.data?.state === 'syncing' && historySyncQ.data.sender === sender)
                        ? <CircularProgress size={18} sx={{ color: WA.green }} />
                        : <SyncIcon fontSize="small" />}
                    </IconButton>
                  </span>
                </Tooltip>
                <Tooltip title="Info kontak">
                  <IconButton size="small" onClick={() => setContactSidePanel('details')} aria-label="Info kontak" sx={{ color: WA.meta }}>
                    <InfoOutlinedIcon fontSize="small" />
                  </IconButton>
                </Tooltip>
                {senderIsGroup ? (
                  <Chip
                    size="small"
                    icon={<GroupsOutlinedIcon sx={{ fontSize: '15px !important' }} />}
                    label="Grup · balasan manual"
                    sx={{ height: 24, fontSize: 11, fontWeight: 600, bgcolor: alpha(WA.green, 0.1), color: WA.greenDark }}
                  />
                ) : convo?.needs_human ? (
                  <Button
                    size="small"
                    variant="contained"
                    startIcon={<TaskAltIcon />}
                    onClick={() => resumeBot.mutate(sender)}
                    disabled={resumeBot.isPending}
                    sx={{ bgcolor: WA.green, '&:hover': { bgcolor: WA.greenDark }, textTransform: 'none', boxShadow: 'none' }}
                  >
                    Selesai
                  </Button>
                ) : aiEnabled && convo?.manual_pause_until ? (
                  <Stack direction="row" spacing={0.5} sx={{ alignItems: 'center' }}>
                    <Chip
                      size="small"
                      label={`AI off · ${fmtTime(convo.manual_pause_until)}`}
                      sx={{ height: 24, fontSize: 11, bgcolor: alpha('#53bdeb', 0.12) }}
                    />
                    <Button size="small" onClick={() => resumeBot.mutate(sender)} disabled={resumeBot.isPending} sx={{ textTransform: 'none' }}>
                      Aktifkan AI
                    </Button>
                  </Stack>
                ) : (
                  <Chip
                    size="small"
                    label={aiEnabled ? 'AI aktif' : 'AI nonaktif'}
                    sx={{
                      height: 24,
                      fontSize: 11,
                      fontWeight: 600,
                      bgcolor: aiEnabled ? alpha(WA.green, 0.12) : alpha('#000', 0.06),
                      color: aiEnabled ? WA.greenDark : WA.meta,
                    }}
                  />
                )}
              </Stack>

              {isMobile && (
                <Stack
                  direction="row"
                  spacing={1}
                  sx={{ px: 1.5, py: 1, bgcolor: WA.panel, borderBottom: `1px solid ${WA.border}` }}
                >
                  <Button
                    fullWidth
                    size="small"
                    variant="outlined"
                    startIcon={<PersonOutlineOutlinedIcon />}
                    onClick={() => setContactSidePanel('details')}
                    sx={{ height: 38, borderRadius: '4px !important', color: WA.greenDark, borderColor: alpha(WA.green, 0.3), textTransform: 'none', fontWeight: 700 }}
                  >
                    Detail
                  </Button>
                </Stack>
              )}

              {!showList && <WhatsAppConnectionNotice status={waStatus} />}

              {/* Satu area status di bawah header — jangan tumpuk banner penuh. */}
              <Collapse
                in={Boolean(
                  syncStatusForChat
                  && (syncStatusForChat.state === 'syncing'
                    || syncStatusForChat.state === 'failed'
                    || syncStatusForChat.state === 'completed'),
                )}
                unmountOnExit
              >
                <HistorySyncNotice
                  status={syncStatusForChat}
                />
              </Collapse>


              {!senderIsGroup && secondaryReady && (
                <ConversationBriefBar
                  brief={briefQ.data}
                  loading={briefQ.isLoading}
                  refreshing={refreshBrief.isPending}
                  onRefresh={() => {
                    if (!sender) return;
                    refreshBrief.mutate(sender, {
                      onSuccess: (brief) => {
                        if (brief.enhancement === 'ai') {
                          swalToast('Ringkasan berhasil diperbarui dan diperkaya AI.');
                        } else {
                          swalToast(brief.enhancement_note || 'Ringkasan lokal berhasil diperbarui.', 'info');
                        }
                      },
                    });
                  }}
                  error={
                    (briefQ.error as { response?: { data?: { error?: string } } })?.response?.data?.error
                    || (briefQ.isError ? 'Gagal memuat ringkasan' : undefined)
                  }
                />
              )}

              {convoLoading && !convo ? (
                <Box sx={{ flex: 1, display: 'grid', placeItems: 'center', minHeight: 0 }}>
                  <Stack spacing={1} sx={{ alignItems: 'center' }}>
                    <CircularProgress size={26} sx={{ color: WA.green }} />
                    <Typography sx={{ fontSize: 13, color: WA.meta }}>Memuat percakapan…</Typography>
                  </Stack>
                </Box>
              ) : convoError && !convo ? (
                <Box sx={{ flex: 1, display: 'grid', placeItems: 'center', minHeight: 0, px: 2 }}>
                  <Stack spacing={1} sx={{ alignItems: 'center', textAlign: 'center' }}>
                    <Typography sx={{ fontSize: 13.5, color: WA.meta }}>
                      Percakapan belum dapat diperbarui. Inbox tidak dikosongkan dan bisa dicoba kembali.
                    </Typography>
                    <Button size="small" variant="outlined" onClick={() => void refetchConversation()}>
                      Muat ulang percakapan
                    </Button>
                  </Stack>
                </Box>
              ) : senderIsGroup && (convo?.data.length || 0) === 0 ? (
                <Box sx={{ flex: 1, display: 'grid', placeItems: 'center', minHeight: 0, px: 3 }}>
                  <Stack spacing={1.25} sx={{ alignItems: 'center', textAlign: 'center', maxWidth: 390 }}>
                    <GroupsOutlinedIcon sx={{ fontSize: 42, color: alpha(WA.greenDark, 0.72) }} />
                    <Typography sx={{ fontSize: 15, fontWeight: 700, color: '#34434b' }}>
                      Belum ada pesan grup yang tersimpan
                    </Typography>
                    <Typography sx={{ fontSize: 13, color: WA.meta, lineHeight: 1.5 }}>
                      Nama grup sudah tersedia dari daftar WhatsApp. Ambil pesan terbaru dari HP; setelah itu pesan baru akan masuk otomatis.
                    </Typography>
                    <Button
                      size="small"
                      variant="contained"
                      startIcon={requestHistorySync.isPending || syncStatusForChat?.state === 'syncing'
                        ? <CircularProgress size={14} color="inherit" />
                        : <SyncIcon />}
                      onClick={() => { void startHistorySync(); }}
                      disabled={historySyncUnavailable || requestHistorySync.isPending || syncStatusForChat?.state === 'syncing'}
                      sx={{ mt: 0.5, bgcolor: WA.greenDark, textTransform: 'none', boxShadow: 'none' }}
                    >
                      Sinkronkan pesan terbaru
                    </Button>
                  </Stack>
                </Box>
              ) : (
                <MessageThread
                  messages={convo?.data || []}
                  agentId={agentId}
                  mediaToken={convo?.media_token || ''}
                  resolveReplyPreview={resolveReplyPreview}
                  onReply={handleReply}
                  onNavigateReply={handleNavigateReply}
                  onRevoke={handleRevoke}
                  onVision={handleVision}
                  showTyping={customerTyping}
                  chatRef={chatRef}
                  bottomRef={bottomRef}
                  onScrollPosition={handleChatScroll}
                  onUserScrollStart={handleUserScrollStart}
                  hasOlder={Boolean(convo?.has_more)}
                  loadingOlder={loadOlderConversation.isPending}
                />
              )}

              <ChatComposer
                key={`${agentId}:${sender}`}
                agentId={agentId}
                sender={sender}
                selectedName={selectedName}
                replyTo={replyTo}
                onClearReply={clearReply}
                onSend={handleComposerSend}
                dropTargetRef={chatPaneRef}
                onInputNode={setComposerInputNode}
              />
            </>
          )}
        </Box>
      )}

      {/* Panel sekunder on-demand. Saat ditutup subtree benar-benar di-unmount. */}
      {(() => {
        const panel = (
          <>
        <Stack
          direction="row"
          sx={{
            px: 0.75,
            py: 0.6,
            alignItems: 'center',
            gap: 0.65,
            bgcolor: WA.greenDark,
            color: '#fff',
            minHeight: 50,
            borderBottom: `1px solid ${alpha('#fff', 0.14)}`,
          }}
        >
          <IconButton
            size="small"
            onClick={() => setContactSidePanel(null)}
            sx={{ color: '#fff', width: 36, height: 36, flexShrink: 0, borderRadius: '4px !important' }}
            aria-label="Tutup panel pelanggan"
          >
            <CloseIcon />
          </IconButton>
          <Stack
            direction="row"
            spacing={0.5}
            sx={{ flex: 1, minWidth: 0, maxWidth: 250, mx: 'auto' }}
          >
            <Button
              size="small"
              onClick={() => setContactSidePanel('details')}
              startIcon={<PersonOutlineOutlinedIcon sx={{ fontSize: '17px !important' }} />}
              sx={{
                flex: 1,
                minWidth: 0,
                height: 36,
                px: 1,
                color: panelTab === 'details' ? WA.greenDark : 'rgba(255,255,255,0.85)',
                bgcolor: panelTab === 'details' ? '#fff' : 'rgba(255,255,255,0.14)',
                textTransform: 'none',
                fontWeight: 700,
                fontSize: 13,
                borderRadius: '4px !important',
                boxShadow: 'none',
                '&:hover': { bgcolor: panelTab === 'details' ? '#fff' : 'rgba(255,255,255,0.22)' },
              }}
            >
              Detail
            </Button>
          </Stack>
          <Box aria-hidden sx={{ width: 36, height: 36, flexShrink: 0 }} />
        </Stack>

        {panelTab === 'details' && (
          <>
            <Box sx={{ bgcolor: WA.panel, pt: 3, pb: 2.5, px: 2, textAlign: 'center', borderBottom: `1px solid ${WA.border}` }}>
              <Avatar
                src={senderIsGroup ? undefined : profilePictureURL(agentId, sender, convo?.media_token || '')}
                sx={{
                  width: 120,
                  height: 120,
                  fontSize: 48,
                  fontWeight: 600,
                  bgcolor: avatarColor(sender),
                  color: '#fff',
                  mx: 'auto',
                  mb: 1.5,
                }}
              >
                {senderIsGroup ? <GroupsOutlinedIcon sx={{ fontSize: 54 }} /> : (selectedName || sender).charAt(0).toUpperCase()}
              </Avatar>
              <Typography sx={{ fontWeight: 500, fontSize: 22, color: '#111b21', lineHeight: 1.25 }}>
                {selectedName || (senderIsGroup ? 'Grup WhatsApp' : `+${sender}`)}
              </Typography>
              <Typography sx={{ fontSize: 15, color: WA.meta, mt: 0.35 }}>
                {senderIsGroup ? 'Percakapan grup' : `+${sender.replace(/^\+/, '')}`}
              </Typography>
            </Box>

            <Box sx={{ bgcolor: WA.panel, mt: 1.25, px: 2, py: 1.5 }}>
              <Typography sx={{ fontSize: 13, fontWeight: 600, color: WA.greenDark, mb: 1 }}>Tentang</Typography>
              <Stack spacing={1.25}>
                <Stack direction="row" spacing={1.25} sx={{ alignItems: 'center' }}>
                  {senderIsGroup
                    ? <GroupsOutlinedIcon sx={{ fontSize: 20, color: WA.meta }} />
                    : <PhoneOutlinedIcon sx={{ fontSize: 20, color: WA.meta }} />}
                  <Box sx={{ flex: 1, minWidth: 0 }}>
                    <Typography sx={{ fontSize: 15, color: '#111b21' }}>
                      {senderIsGroup ? selectedName || 'Grup WhatsApp' : `+${sender.replace(/^\+/, '')}`}
                    </Typography>
                    <Typography sx={{ fontSize: 12.5, color: WA.meta }}>
                      {senderIsGroup ? 'Balasan manual · AI tidak aktif di grup' : 'WhatsApp'}
                    </Typography>
                  </Box>
                  <Tooltip title={senderIsGroup ? 'Salin ID grup' : 'Salin nomor'}>
                    <IconButton size="small" onClick={() => void copyNumber()} aria-label="Salin nomor">
                      <ContentCopyIcon sx={{ fontSize: 18, color: WA.meta }} />
                    </IconButton>
                  </Tooltip>
                </Stack>
                {copyHint && (
                  <Typography sx={{ fontSize: 12, color: WA.greenDark, pl: 4.5 }}>{copyHint}</Typography>
                )}
              </Stack>
            </Box>

            <Box sx={{ bgcolor: WA.panel, mt: 1.5, px: 2, py: 1.75 }}>
              <Typography sx={{ fontSize: 13.5, fontWeight: 700, color: WA.greenDark, mb: 1.25 }}>Info chat</Typography>
              <Stack spacing={0.85}>
                <InfoRow
                  label="Asisten AI"
                  value={
                    aiEnabled && convo?.manual_pause_until
                      ? `Off sampai ${fmtTime(convo.manual_pause_until)}`
                      : aiEnabled
                        ? 'Aktif'
                        : 'Nonaktif'
                  }
                />
                <InfoRow
                  label="Aktivitas terakhir"
                  value={
                    selectedContact?.last_at
                      ? new Date(selectedContact.last_at).toLocaleString('id-ID', {
                          day: '2-digit',
                          month: 'short',
                          year: 'numeric',
                          hour: '2-digit',
                          minute: '2-digit',
                          timeZone: INBOX_TZ,
                        })
                      : '—'
                  }
                />
                {selectedContact?.last_msg && (
                  <Box>
                    <Typography sx={{ fontSize: 12.5, color: WA.meta, mb: 0.25 }}>Pesan terakhir</Typography>
                    <Typography sx={{ fontSize: 14, color: '#111b21', lineHeight: 1.4 }}>
                      {selectedContact.last_msg}
                    </Typography>
                  </Box>
                )}
              </Stack>
            </Box>

            {briefQ.data?.summary && (
              <Box sx={{ bgcolor: WA.panel, mt: 1.5, px: 2, py: 1.75 }}>
                <Typography sx={{ fontSize: 13.5, fontWeight: 700, color: WA.greenDark, mb: 1 }}>Ringkasan</Typography>
                <Typography sx={{ fontSize: 13.5, color: '#3b4a54', lineHeight: 1.45 }}>
                  {briefQ.data.summary}
                </Typography>
              </Box>
            )}

            <Box sx={{ px: 2, py: 2, display: 'flex', flexDirection: 'column', gap: 1, mb: 1 }}>
              {convo?.needs_human && (
                <Button
                  fullWidth
                  variant="contained"
                  startIcon={<TaskAltIcon />}
                  onClick={() => {
                    resumeBot.mutate(sender);
                    setContactSidePanel(null);
                  }}
                  disabled={resumeBot.isPending}
                  sx={{ bgcolor: WA.green, '&:hover': { bgcolor: WA.greenDark }, textTransform: 'none', boxShadow: 'none' }}
                >
                  Selesai ditangani CS
                </Button>
              )}
              {canManageInbox && (
                <Button
                  fullWidth
                  color="error"
                  variant="outlined"
                  startIcon={deleteConvo.isPending ? <CircularProgress size={16} color="inherit" /> : <DeleteIcon />}
                  onClick={() => { void deleteConversation(sender); }}
                  disabled={deleteConvo.isPending || deletingSender === sender}
                  sx={{ textTransform: 'none' }}
                >
                  Hapus chat dari inbox
                </Button>
              )}
              <Typography sx={{ fontSize: 11.5, color: WA.meta, lineHeight: 1.4, textAlign: 'center' }}>
                Hanya menghapus di Inbox aplikasi. Chat di HP pelanggan tetap aman.
              </Typography>
            </Box>
          </>
        )}

          </>
        );
        if (isMobile) {
          if (!contactSidePanel || !sender) return null;
          // Panel lokal tanpa MUI Modal/Drawer: tidak menyentuh body, tidak
          // memasang backdrop global, dan langsung hilang saat ditutup.
          return (
            <Box
              data-chatloop-role="inbox-mobile-side-panel"
              sx={{
                position: 'absolute',
                inset: 0,
                zIndex: 12,
                display: 'flex',
                flexDirection: 'column',
                minHeight: 0,
                bgcolor: '#f0f2f5',
              }}
            >
              {panel}
            </Box>
          );
        }
        return sender && contactSidePanel ? (
          <Box
            data-chatloop-role="inbox-desktop-side-panel"
            sx={{
              width: { md: 340, lg: 380, xl: 400 },
              flexShrink: 0,
              display: 'flex',
              flexDirection: 'column',
              minHeight: 0,
              bgcolor: '#f0f2f5',
              borderLeft: `1px solid ${WA.border}`,
            }}
          >
            {panel}
          </Box>
        ) : null;
      })()}

      <Dialog open={!!visionTarget} onClose={() => !reanalyzeImage.isPending && setVisionTarget(null)} fullWidth maxWidth="sm">
        <DialogTitle>Analisis ulang gambar</DialogTitle>
        <DialogContent>
          <Typography variant="body2" color="text.secondary" sx={{ mb: 1.5 }}>
            Kosongkan instruksi untuk analisis umum, atau jelaskan detail yang perlu diperiksa. Hasil baru menggantikan
            analisis lama tanpa mengirim pesan ke pelanggan.
          </Typography>
          <TextField
            fullWidth
            multiline
            minRows={3}
            autoFocus
            label="Instruksi khusus (opsional)"
            placeholder="Contoh: Baca warna dan ukuran yang terlihat, lalu cocokkan dengan katalog produk."
            value={visionInstruction}
            onChange={(event) => setVisionInstruction(event.target.value.slice(0, 800))}
            helperText={`${visionInstruction.length}/800`}
          />
          {visionError && (
            <Alert severity="error" sx={{ mt: 1.5 }}>
              {visionError}
            </Alert>
          )}
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setVisionTarget(null)} disabled={reanalyzeImage.isPending}>
            Batal
          </Button>
          <Button
            variant="contained"
            startIcon={
              reanalyzeImage.isPending ? <CircularProgress size={16} color="inherit" /> : <AutoAwesomeIcon />
            }
            onClick={() => void runReanalysis()}
            disabled={reanalyzeImage.isPending}
          >
            Analisis sekarang
          </Button>
        </DialogActions>
      </Dialog>
    </Box>
  );
}
