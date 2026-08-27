import { useEffect, useState } from 'react';
import {
  Alert,
  Box,
  Button,
  Checkbox,
  CircularProgress,
  Divider,
  FormControlLabel,
  IconButton,
  InputAdornment,
  Link,
  TextField,
  Typography,
} from '@mui/material';
import {
  ContactsOutlined,
  LockOutlined,
  MessageOutlined,
  PersonOutlined,
  SendOutlined,
  ShieldOutlined,
  Visibility,
  VisibilityOff,
} from '@mui/icons-material';
import { useNavigate } from 'react-router-dom';
import api from '../services/api';
import { unlockInboxSound } from '../services/inboxSound';
import logo from '../assets/logo-ruangkirim.png';
import './Login.css';

function responseStatus(error: unknown) {
  if (typeof error === 'object' && error && 'response' in error) {
    return (error as { response?: { status?: number; headers?: Record<string, string> } }).response;
  }
  return undefined;
}

function GoogleIcon() {
  return (
    <svg width="22" height="22" viewBox="0 0 24 24" aria-hidden="true">
      <path d="M22.56 12.25c0-.78-.07-1.53-.2-2.25H12v4.26h5.92c-.26 1.37-1.04 2.53-2.21 3.31v2.77h3.57c2.08-1.92 3.28-4.74 3.28-8.09z" fill="#4285F4" />
      <path d="M12 23c2.97 0 5.46-.98 7.28-2.66l-3.57-2.77c-.98.66-2.23 1.06-3.71 1.06-2.86 0-5.29-1.93-6.16-4.53H2.18v2.84C3.99 20.53 7.7 23 12 23z" fill="#34A853" />
      <path d="M5.84 14.09c-.22-.66-.35-1.36-.35-2.09s.13-1.43.35-2.09V7.07H2.18C1.43 8.55 1 10.22 1 12s.43 3.45 1.18 4.93l2.85-2.22.81-.62z" fill="#FBBC05" />
      <path d="M12 5.38c1.62 0 3.06.56 4.21 1.64l3.15-3.15C17.45 2.09 14.97 1 12 1 7.7 1 3.99 3.47 2.18 7.07l3.66 2.84c.87-2.6 3.3-4.53 6.16-4.53z" fill="#EA4335" />
    </svg>
  );
}

const features = [
  { title: 'Balasan AI Cerdas', description: 'Menjawab pelanggan secara otomatis 24/7 dengan AI.', icon: <MessageOutlined /> },
  { title: 'Broadcast Massal', description: 'Kirim pesan ke ribuan kontak WhatsApp sekaligus.', icon: <SendOutlined /> },
  { title: 'CRM Terintegrasi', description: 'Kelola kontak, percakapan, dan data pelanggan dalam satu tempat.', icon: <ContactsOutlined /> },
];

function ChatMockup() {
  return (
    <Box className="login-chat-mockup" aria-hidden="true">
      <Box className="login-chat-mockup__screen">
        <Box className="login-chat-mockup__header">
          <Box className="login-chat-mockup__avatar" />
          <Box sx={{ flex: 1 }}>
            <Box className="login-chat-mockup__line login-chat-mockup__line--wide" />
            <Box className="login-chat-mockup__line login-chat-mockup__line--short" />
          </Box>
        </Box>
        <Box className="login-chat-message login-chat-message--left">
          <Box className="login-chat-message__line login-chat-message__line--wide" />
          <Box className="login-chat-message__line login-chat-message__line--medium" />
          <Typography>09.30</Typography>
        </Box>
        <Box className="login-chat-message login-chat-message--right">
          <Box className="login-chat-message__line login-chat-message__line--wide" />
          <Box className="login-chat-message__line login-chat-message__line--medium" />
          <Typography>09.31 ✓✓</Typography>
        </Box>
        <Box className="login-chat-message login-chat-message--left login-chat-message--last">
          <Box className="login-chat-message__line login-chat-message__line--wide" />
          <Box className="login-chat-message__line login-chat-message__line--shorter" />
          <Typography>09.32</Typography>
        </Box>
        <Box className="login-chat-ai-badge">
          <Typography>AI</Typography>
          {[0, 1, 2].map((dot) => <Box key={dot} className={`login-chat-ai-dot${dot === 0 ? ' is-active' : ''}`} />)}
        </Box>
        <Box className="login-chat-dots">
          {[0, 1, 2].map((dot) => <Box key={dot} />)}
        </Box>
      </Box>
    </Box>
  );
}

function FeatureList() {
  return (
    <Box className="login-feature-list">
      {features.map((feature) => (
        <Box key={feature.title} className="login-feature-item">
          <Box className="login-feature-icon">{feature.icon}</Box>
          <Box className="login-feature-copy">
            <Typography className="login-feature-title">{feature.title}</Typography>
            <Typography className="login-feature-description">{feature.description}</Typography>
          </Box>
        </Box>
      ))}
    </Box>
  );
}

export default function Login() {
  const [username, setUsername] = useState('');
  const [password, setPassword] = useState('');
  const [error, setError] = useState('');
  const [errors, setErrors] = useState<Record<string, string>>({});
  const [loading, setLoading] = useState(false);
  const [cooldown, setCooldown] = useState(0);
  const [needVerify, setNeedVerify] = useState(false);
  const [turnstileToken] = useState('');
  const [showPassword, setShowPassword] = useState(false);
  const navigate = useNavigate();
  const currentYear = new Date().getFullYear();

  useEffect(() => {
    if (cooldown <= 0) return;
    const timer = window.setInterval(() => setCooldown((v) => Math.max(0, v - 1)), 1000);
    return () => window.clearInterval(timer);
  }, [cooldown]);

  const handleLogin = async () => {
    if (loading || cooldown > 0) return;
    if (localStorage.getItem('chatloop_inbox_sound') !== 'off') void unlockInboxSound();

    const cleanUsername = username.trim();
    const nextErrors: Record<string, string> = {};
    if (!cleanUsername) nextErrors.username = 'Wajib diisi';
    if (!password) nextErrors.password = 'Wajib diisi';
    setErrors(nextErrors);
    if (Object.keys(nextErrors).length > 0) return;

    setError('');
    setNeedVerify(false);
    setLoading(true);
    try {
      const res = await api.post('/login', { username: cleanUsername, password, turnstile: turnstileToken });
      localStorage.setItem('token', res.data.token);
      localStorage.setItem('user', JSON.stringify(res.data.user));
      navigate('/app');
    } catch (e) {
      const response = responseStatus(e);
      if (response?.status === 429) {
        const retryAfter = Number(response.headers?.['retry-after'] || 60);
        setCooldown(Number.isFinite(retryAfter) ? Math.min(Math.max(retryAfter, 30), 300) : 60);
        setError('Terlalu banyak percobaan. Tunggu sebentar lalu coba lagi.');
      } else if (response?.status === 403) {
        setError('Email kamu belum diverifikasi. Cek inbox atau folder spam untuk link aktivasi.');
        setNeedVerify(true);
      } else if (!response || (response.status ?? 0) >= 500) {
        setError('Server belum siap. Coba lagi sebentar lagi.');
      } else {
        setError('Login belum berhasil. Periksa kembali data yang kamu masukkan.');
      }
      setLoading(false);
    }
  };

  const fieldSx = {
    '& .MuiOutlinedInput-root': {
      borderRadius: '11px',
      bgcolor: 'rgba(255,255,255,.88)',
      minHeight: 52,
      boxSizing: 'border-box',
    },
    '& .MuiOutlinedInput-notchedOutline': { borderColor: '#e1e8f2' },
    '& .MuiOutlinedInput-root:hover .MuiOutlinedInput-notchedOutline': { borderColor: '#cbd8e8' },
    '& .MuiOutlinedInput-root.Mui-focused .MuiOutlinedInput-notchedOutline': { borderColor: '#246bfd', borderWidth: 2 },
  };

  const inputIcon = {
    width: 36,
    height: 36,
    borderRadius: '10px',
    bgcolor: '#edf4ff',
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'center',
    color: '#246bfd',
  };

  return (
    <Box component="main" className="login-page">
      <Box aria-hidden="true" className="login-page-decoration login-page-decoration--top" />
      <Box aria-hidden="true" className="login-page-decoration login-page-decoration--bottom" />

      <Box component="section" className="login-promo">
        <Box className="login-promo-content">
          <Box className="login-promo-heading">
            <Box className="login-promo-copy">
              <Typography component="h2" className="login-promo-title">WhatsApp</Typography>
              <Typography component="h2" className="login-promo-title login-promo-title--accent">AI Assistant</Typography>
              <Typography className="login-promo-description">Otomatis balasan AI, broadcast massal, dan CRM WhatsApp — semua dalam satu dashboard.</Typography>
            </Box>
            <ChatMockup />
          </Box>
          <FeatureList />
          <Box className="login-security">
            <Box className="login-security-icon"><ShieldOutlined fontSize="small" /></Box>
            <Box sx={{ minWidth: 0 }}>
              <Typography className="login-security-title">Aman &amp; Terpercaya</Typography>
              <Typography className="login-security-description">Data Anda dienkripsi dan terlindungi dengan standar keamanan tinggi.</Typography>
            </Box>
          </Box>
        </Box>
      </Box>

      <Box component="section" className="login-form">
        <Box className="login-form-content">
          <Box className="login-logo">
            <Box component="img" src={logo} alt="Ruangkirim" />
          </Box>

          <Box className="login-welcome">
            <Typography component="h1">Selamat datang kembali! 👋</Typography>
            <Typography>Masuk ke dashboard untuk mengelola WhatsApp AI Assistant kamu.</Typography>
          </Box>

          <Box className="login-form-fields">
            {error && <Alert severity="error" className="login-error">{error}</Alert>}
            {needVerify && (
              <Box className="login-verification-link">
                <Link component="button" underline="hover" onClick={() => navigate('/cek-email', { state: { email: username.trim() } })}>Buka halaman verifikasi email</Link>
              </Box>
            )}

            <Typography component="label" className="login-input-label">Username</Typography>
            <TextField
              fullWidth
              size="small"
              value={username}
              placeholder="Masukkan username"
              disabled={loading || cooldown > 0}
              autoComplete="username"
              error={Boolean(errors.username)}
              helperText={errors.username || ' '}
              onChange={(e) => {
                setUsername(e.target.value);
                if (errors.username) setErrors((prev) => { const next = { ...prev }; delete next.username; return next; });
              }}
              onKeyDown={(e) => e.key === 'Enter' && handleLogin()}
              sx={{ ...fieldSx, mb: .25, '& .MuiFormHelperText-root': { minHeight: 15, mt: .35, fontSize: '.68rem' } }}
              slotProps={{ input: { startAdornment: <InputAdornment position="start"><Box sx={inputIcon}><PersonOutlined fontSize="small" /></Box></InputAdornment> } }}
            />

            <Typography component="label" className="login-input-label login-input-label--password">Password</Typography>
            <TextField
              fullWidth
              size="small"
              type={showPassword ? 'text' : 'password'}
              value={password}
              placeholder="Masukkan password"
              disabled={loading || cooldown > 0}
              autoComplete="current-password"
              error={Boolean(errors.password)}
              helperText={errors.password || ' '}
              onChange={(e) => {
                setPassword(e.target.value);
                if (errors.password) setErrors((prev) => { const next = { ...prev }; delete next.password; return next; });
              }}
              onKeyDown={(e) => e.key === 'Enter' && handleLogin()}
              sx={{ ...fieldSx, '& .MuiFormHelperText-root': { minHeight: 15, mt: .35, fontSize: '.68rem' } }}
              slotProps={{
                input: {
                  startAdornment: <InputAdornment position="start"><Box sx={inputIcon}><LockOutlined fontSize="small" /></Box></InputAdornment>,
                  endAdornment: <InputAdornment position="end"><IconButton aria-label={showPassword ? 'Sembunyikan password' : 'Tampilkan password'} onClick={() => setShowPassword((v) => !v)} edge="end" size="small" sx={{ color: '#74839d' }}>{showPassword ? <VisibilityOff fontSize="small" /> : <Visibility fontSize="small" />}</IconButton></InputAdornment>,
                },
              }}
            />

            <Box className="login-options">
              <FormControlLabel
                control={<Checkbox size="small" defaultChecked sx={{ p: .3, color: '#246bfd', '&.Mui-checked': { color: '#246bfd' } }} />}
                label="Ingat saya"
                sx={{ m: 0, minWidth: 0, flexShrink: 1 }}
              />
              <Link component="button" variant="body2" underline="hover" onClick={() => navigate('/forgot-password')} className="login-forgot">Lupa password?</Link>
            </Box>

            <Button fullWidth variant="contained" onClick={handleLogin} disabled={loading || cooldown > 0} className="login-submit">
              {loading ? <CircularProgress size={22} sx={{ color: '#fff' }} /> : cooldown > 0 ? `Coba lagi dalam ${cooldown}s` : 'Masuk'}
            </Button>

            <Divider className="login-divider">atau</Divider>

            {/* TODO: Integrate Google OAuth when the backend/provider is available. */}
            <Button fullWidth variant="outlined" type="button" onClick={() => undefined} startIcon={<GoogleIcon />} className="login-google">Masuk dengan Google</Button>
          </Box>

          <Typography className="login-footer">© {currentYear} Ruangkirim. Semua hak dilindungi.</Typography>
        </Box>
      </Box>
    </Box>
  );
}
