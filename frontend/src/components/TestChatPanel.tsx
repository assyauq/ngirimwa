import { Fragment, useEffect, useRef, useState } from 'react';
import {
  Avatar, Box, Button, Card, CardContent, Chip, CircularProgress, IconButton,
  Stack, TextField, Tooltip, Typography,
} from '@mui/material';
import SendIcon from '@mui/icons-material/Send';
import SmartToyIcon from '@mui/icons-material/SmartToyOutlined';
import PersonIcon from '@mui/icons-material/Person';
import RestartAltIcon from '@mui/icons-material/RestartAlt';
import ScienceIcon from '@mui/icons-material/ScienceOutlined';
import { useTestChat } from '../hooks';
import PageHeader from './PageHeader';

type Msg = {
  role: 'user' | 'bot';
  text: string;
  escalate?: boolean;
  model?: string;
  error?: boolean;
};

const STARTER_PROMPTS = [
  'Produk atau layanan apa saja yang tersedia?',
  'Berapa harga dan cara pemesanannya?',
  'Apakah ada promo hari ini?',
];

// Ubah URL (mis. https://wa.me/62...) di teks jadi tautan yang bisa diklik.
function renderWithLinks(text: string) {
  return text.split(/(https?:\/\/[^\s]+)/g).map((part, index) =>
    /^https?:\/\//.test(part)
      ? <a key={index} href={part} target="_blank" rel="noopener noreferrer" style={{ color: 'inherit', fontWeight: 700, textDecoration: 'underline' }}>{part}</a>
      : <Fragment key={index}>{part}</Fragment>
  );
}

export default function TestChatPanel({ agentId }: { agentId: number }) {
  const [msgs, setMsgs] = useState<Msg[]>([]);
  const [input, setInput] = useState('');
  const testChat = useTestChat(agentId);
  const messagesBoxRef = useRef<HTMLDivElement | null>(null);
  const inputRef = useRef<HTMLInputElement | null>(null);

  useEffect(() => {
    const messagesBox = messagesBoxRef.current;
    if (!messagesBox) return;
    messagesBox.scrollTo({
      top: messagesBox.scrollHeight,
      behavior: window.matchMedia('(prefers-reduced-motion: reduce)').matches ? 'auto' : 'smooth',
    });
  }, [msgs, testChat.isPending]);

  const send = async (preset?: string) => {
    const text = (preset ?? input).trim();
    if (!text || testChat.isPending) return;

    const history = msgs.map(message => ({ role: message.role, text: message.text }));
    setMsgs(current => [...current, { role: 'user', text }]);
    setInput('');

    try {
      const response = await testChat.mutateAsync({ message: text, history });
      setMsgs(current => [...current, {
        role: 'bot',
        text: response.reply,
        escalate: response.escalate,
        model: response.model,
      }]);
    } catch {
      setMsgs(current => [...current, {
        role: 'bot',
        text: 'Simulasi belum berhasil dijalankan. Periksa konfigurasi AI, lalu coba lagi.',
        error: true,
      }]);
    } finally {
      window.setTimeout(() => inputRef.current?.focus(), 0);
    }
  };

  const reset = () => {
    setMsgs([]);
    setInput('');
    window.setTimeout(() => inputRef.current?.focus(), 0);
  };

  return (
    <Box
      sx={{
        width: '100%',
        maxWidth: 960,
        height: { xs: 'calc(100dvh - 160px)', md: 'calc(100vh - 142px)' },
        minHeight: { xs: 460, md: 520 },
        maxHeight: 840,
        display: 'flex',
        flexDirection: 'column',
      }}
    >
      <PageHeader
        title="Simulasi AI"
        subtitle="Uji cara Asisten AI menjawab sebelum digunakan untuk percakapan pelanggan."
        action={<Chip size="small" icon={<ScienceIcon />} label="Tidak mengirim ke WhatsApp" variant="outlined" />}
      />

      <Card sx={{ flex: 1, minHeight: 0, overflow: 'hidden' }}>
        <CardContent sx={{ p: '0 !important', height: '100%', display: 'flex', flexDirection: 'column' }}>
          <Stack
            direction="row"
            spacing={1.25}
            sx={{
              alignItems: 'center',
              justifyContent: 'space-between',
              px: { xs: 1.25, sm: 1.75 },
              py: 1.25,
              borderBottom: '1px solid',
              borderColor: 'divider',
              bgcolor: 'background.paper',
              flexShrink: 0,
            }}
          >
            <Stack direction="row" spacing={1} sx={{ alignItems: 'center', minWidth: 0 }}>
              <Avatar sx={{ width: 36, height: 36, bgcolor: 'action.selected', color: 'primary.main' }}>
                <SmartToyIcon fontSize="small" />
              </Avatar>
              <Box sx={{ minWidth: 0 }}>
                <Typography noWrap variant="body2" sx={{ fontWeight: 700 }}>Asisten AI</Typography>
                <Stack direction="row" spacing={0.75} sx={{ alignItems: 'center' }}>
                  <Box sx={{ width: 7, height: 7, borderRadius: '50%', bgcolor: 'success.main' }} />
                  <Typography variant="caption" color="text.secondary">Mode pengujian</Typography>
                </Stack>
              </Box>
            </Stack>

            <Tooltip title={msgs.length > 0 ? 'Mulai percakapan baru' : 'Percakapan masih kosong'}>
              <span>
                <Button
                  size="small"
                  variant="text"
                  startIcon={<RestartAltIcon />}
                  onClick={reset}
                  disabled={msgs.length === 0 || testChat.isPending}
                >
                  Mulai ulang
                </Button>
              </span>
            </Tooltip>
          </Stack>

          <Box
            ref={messagesBoxRef}
            role="log"
            aria-live="polite"
            sx={{
              flex: 1,
              minHeight: 0,
              overflowY: 'auto',
              overscrollBehavior: 'contain',
              scrollbarGutter: 'stable',
              px: { xs: 1.25, sm: 2 },
              py: 1.75,
              display: 'flex',
              flexDirection: 'column',
              gap: 1.25,
              bgcolor: '#F7F9F7',
            }}
          >
            {msgs.length === 0 && (
              <Box
                sx={{
                  flex: 1,
                  minHeight: 280,
                  display: 'grid',
                  placeItems: 'center',
                  textAlign: 'center',
                  py: 3,
                }}
              >
                <Box sx={{ maxWidth: 520 }}>
                  <Avatar sx={{ width: 48, height: 48, mx: 'auto', mb: 1.5, bgcolor: 'action.selected', color: 'primary.main' }}>
                    <SmartToyIcon />
                  </Avatar>
                  <Typography variant="subtitle1" sx={{ fontWeight: 700 }}>Mulai percakapan sebagai calon pelanggan</Typography>
                  <Typography variant="body2" color="text.secondary" sx={{ mt: 0.5, mb: 2 }}>
                    Pilih contoh pertanyaan atau tulis skenario sendiri untuk mengecek kualitas jawaban AI.
                  </Typography>
                  <Stack direction="row" spacing={0.75} sx={{ justifyContent: 'center', flexWrap: 'wrap', gap: 0.75 }}>
                    {STARTER_PROMPTS.map(prompt => (
                      <Chip
                        key={prompt}
                        label={prompt}
                        variant="outlined"
                        onClick={() => { void send(prompt); }}
                        disabled={testChat.isPending}
                        sx={{ height: 'auto', maxWidth: '100%', '& .MuiChip-label': { whiteSpace: 'normal', py: 0.75 } }}
                      />
                    ))}
                  </Stack>
                </Box>
              </Box>
            )}

            {msgs.map((message, index) => (
              <Stack
                key={`${message.role}-${index}`}
                direction={message.role === 'user' ? 'row-reverse' : 'row'}
                spacing={0.75}
                sx={{
                  alignItems: 'flex-end',
                  alignSelf: message.role === 'user' ? 'flex-end' : 'flex-start',
                  maxWidth: { xs: '94%', sm: '82%' },
                }}
              >
                <Avatar
                  sx={{
                    width: 28,
                    height: 28,
                    bgcolor: message.role === 'user' ? 'primary.main' : 'background.paper',
                    color: message.role === 'user' ? 'primary.contrastText' : 'primary.main',
                    border: '1px solid',
                    borderColor: message.role === 'user' ? 'primary.main' : 'divider',
                    flexShrink: 0,
                  }}
                >
                  {message.role === 'user' ? <PersonIcon sx={{ fontSize: 17 }} /> : <SmartToyIcon sx={{ fontSize: 17 }} />}
                </Avatar>
                <Box sx={{ minWidth: 0 }}>
                  <Box
                    sx={{
                      px: 1.35,
                      py: 0.9,
                      borderRadius: message.role === 'user' ? '12px 12px 4px 12px' : '12px 12px 12px 4px',
                      bgcolor: message.error ? 'error.main' : message.role === 'user' ? 'primary.main' : 'background.paper',
                      color: message.role === 'user' || message.error ? 'primary.contrastText' : 'text.primary',
                      border: message.role === 'bot' && !message.error ? '1px solid' : 0,
                      borderColor: 'divider',
                      whiteSpace: 'pre-wrap',
                      overflowWrap: 'anywhere',
                      fontSize: 13.5,
                      lineHeight: 1.55,
                    }}
                  >
                    {renderWithLinks(message.text)}
                  </Box>
                  {message.escalate && (
                    <Chip label="AI menyarankan bantuan CS" size="small" color="warning" variant="outlined" sx={{ mt: 0.5 }} />
                  )}
                </Box>
              </Stack>
            ))}

            {testChat.isPending && (
              <Stack direction="row" spacing={0.75} sx={{ alignItems: 'center', alignSelf: 'flex-start' }}>
                <Avatar sx={{ width: 28, height: 28, bgcolor: 'background.paper', color: 'primary.main', border: '1px solid', borderColor: 'divider' }}>
                  <SmartToyIcon sx={{ fontSize: 17 }} />
                </Avatar>
                <Stack direction="row" spacing={0.75} sx={{ alignItems: 'center', px: 1.25, py: 1, bgcolor: 'background.paper', border: '1px solid', borderColor: 'divider', borderRadius: '12px 12px 12px 4px' }}>
                  <CircularProgress size={14} />
                  <Typography variant="caption" color="text.secondary">AI sedang menyusun jawaban…</Typography>
                </Stack>
              </Stack>
            )}
          </Box>

          <Box
            sx={{
              p: { xs: 1, sm: 1.25 },
              borderTop: '1px solid',
              borderColor: 'divider',
              bgcolor: 'background.paper',
              flexShrink: 0,
            }}
          >
            <Stack direction="row" spacing={1} sx={{ alignItems: 'center' }}>
              <TextField
                fullWidth
                multiline
                minRows={1}
                maxRows={4}
                inputRef={inputRef}
                placeholder="Tulis pesan sebagai calon pelanggan…"
                value={input}
                disabled={testChat.isPending}
                onChange={event => setInput(event.target.value)}
                onKeyDown={event => {
                  if (event.key === 'Enter' && !event.shiftKey) {
                    event.preventDefault();
                    void send();
                  }
                }}
                slotProps={{
                  htmlInput: {
                    'aria-label': 'Pesan simulasi',
                  },
                }}
              />
              <Tooltip title={input.trim() ? 'Kirim pesan' : 'Tulis pesan terlebih dahulu'}>
                <span>
                  <IconButton
                    color="primary"
                    onClick={() => { void send(); }}
                    disabled={!input.trim() || testChat.isPending}
                    aria-label="Kirim pesan simulasi"
                    sx={{
                      width: 42,
                      height: 42,
                      p: 0,
                      display: 'grid',
                      placeItems: 'center',
                      bgcolor: 'primary.main',
                      color: 'primary.contrastText',
                      '&:hover': { bgcolor: 'primary.dark' },
                      '&.Mui-disabled': { bgcolor: 'action.disabledBackground' },
                      '& .MuiSvgIcon-root': {
                        display: 'block',
                        transform: 'translateX(1px)',
                      },
                    }}
                  >
                    {testChat.isPending ? <CircularProgress size={18} /> : <SendIcon fontSize="small" />}
                  </IconButton>
                </span>
              </Tooltip>
            </Stack>
            <Typography variant="caption" color="text.secondary" sx={{ display: { xs: 'none', sm: 'block' }, mt: 0.75, px: 0.25 }}>
              Enter untuk mengirim · Shift + Enter untuk membuat baris baru
            </Typography>
          </Box>
        </CardContent>
      </Card>
    </Box>
  );
}
