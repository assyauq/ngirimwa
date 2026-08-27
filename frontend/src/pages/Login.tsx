import { useEffect, useState } from 'react';
import { Box, TextField, Button, Typography, Alert, CircularProgress, Link, InputAdornment, IconButton } from '@mui/material';
import { useNavigate } from 'react-router-dom';
import VisibilityOutlinedIcon from '@mui/icons-material/VisibilityOutlined';
import VisibilityOffOutlinedIcon from '@mui/icons-material/VisibilityOffOutlined';
import PersonOutlineOutlinedIcon from '@mui/icons-material/PersonOutlineOutlined';
import LockOutlinedIcon from '@mui/icons-material/LockOutlined';
import api from '../services/api';
import { unlockInboxSound } from '../services/inboxSound';

function responseStatus(error: unknown) {
  if (typeof error === 'object' && error && 'response' in error) {
    return (error as { response?: { status?: number; headers?: Record<string, string> } }).response;
  }
  return undefined;
}

function Illustration() {
  return (
    <Box className="rk-login-visual">
      <Box className="rk-login-orb rk-login-orb-top" />
      <Box className="rk-login-orb rk-login-orb-bottom" />
      <Box className="rk-login-visual-inner">
        <Box className="rk-login-copy">
          <Typography className="rk-login-eyebrow">WhatsApp</Typography>
          <Typography component="h1" className="rk-login-hero-title">AI Assistant</Typography>
          <Typography className="rk-login-hero-description">
            Otomatis balasan AI, broadcast massal, dan CRM WhatsApp — semua dalam satu dashboard.
          </Typography>
        </Box>

        <Box className="rk-chat-illustration" aria-hidden="true">
          <Box className="rk-chat-spark rk-chat-spark-1">✦</Box>
          <Box className="rk-chat-spark rk-chat-spark-2">✦</Box>
          <Box className="rk-chat-spark rk-chat-spark-3">✦</Box>
          <Box className="rk-chat-phone">
            <Box className="rk-chat-phone-header">
              <Box className="rk-chat-avatar" />
              <Box><span /><span className="short" /></Box>
            </Box>
            <Box className="rk-chat-bubble incoming"><span /><span /><span className="short" /></Box>
            <Box className="rk-chat-bubble outgoing"><span /><span className="short" /></Box>
            <Box className="rk-chat-bubble incoming lower"><span /><span /><span className="short" /><Box className="rk-ai-pill">AI</Box></Box>
            <Box className="rk-chat-dots"><i /><i /><i /></Box>
          </Box>
        </Box>

        <Box className="rk-login-features">
          <Box><span className="rk-feature-icon">◌</span><Box><b>Balasan AI Cerdas</b><small>Menjawab pelanggan secara otomatis 24/7 dengan AI.</small></Box></Box>
          <Box><span className="rk-feature-icon">➤</span><Box><b>Broadcast Massal</b><small>Kirim pesan ke ribuan kontak WhatsApp sekaligus.</small></Box></Box>
          <Box><span className="rk-feature-icon">▥</span><Box><b>CRM Terintegrasi</b><small>Kelola kontak, percakapan, dan data pelanggan dalam satu tempat.</small></Box></Box>
        </Box>

        <Box className="rk-security-note">
          <span>◆</span>
          <Box><b>Aman &amp; Terpercaya</b><small>Data Anda dienkripsi dan terlindungi dengan standar keamanan tinggi.</small></Box>
        </Box>
      </Box>
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
  const [showPassword, setShowPassword] = useState(false);
  const [turnstileToken] = useState('');
  const navigate = useNavigate();

  useEffect(() => {
    if (cooldown <= 0) return;
    const timer = window.setInterval(() => setCooldown((v) => Math.max(0, v - 1)), 1000);
    return () => window.clearInterval(timer);
  }, [cooldown]);

  const handleLogin = async () => {
    if (loading || cooldown > 0) return;

    if (localStorage.getItem('chatloop_inbox_sound') !== 'off') {
      void unlockInboxSound();
    }

    const cleanUsername = username.trim();
    const e: Record<string, string> = {};
    if (!cleanUsername) e.username = 'Wajib diisi';
    if (!password) e.password = 'Wajib diisi';
    setErrors(e);
    if (Object.keys(e).length > 0) return;
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

  return (
    <Box className="rk-login-page">
      <Box className="rk-login-shell">
        <Illustration />

        <Box className="rk-login-panel">
          <Box className="rk-login-form">
            <Box className="rk-login-brand">
              <img src="/ruangkirim-logo.svg" alt="RuangKirim" />
            </Box>

            <Typography component="h2" className="rk-login-title">Selamat datang kembali! 👋</Typography>
            <Typography className="rk-login-subtitle">Masuk ke dashboard untuk mengelola WhatsApp AI Assistant kamu.</Typography>

            {error && <Alert severity={cooldown > 0 ? 'warning' : 'error'} sx={{ mb: 2.5 }}>{error}</Alert>}

            {needVerify && (
              <Typography variant="body2" sx={{ mb: 2, textAlign: 'center' }}>
                <Link component="button" type="button" underline="hover" onClick={() => navigate('/cek-email', { state: { email: username.trim() } })}>
                  Kirim ulang link verifikasi
                </Link>
              </Typography>
            )}

            <Box className="rk-login-field-label">Username</Box>
            <TextField
              fullWidth
              value={username}
              disabled={loading || cooldown > 0}
              autoComplete="username"
              placeholder="Masukkan username"
              onChange={(e) => { setUsername(e.target.value); if (errors.username) setErrors((p) => ({ ...p, username: '' })); }}
              error={!!errors.username}
              helperText={errors.username}
              onKeyDown={(e) => e.key === 'Enter' && handleLogin()}
              className="rk-login-input"
              slotProps={{
                input: { startAdornment: <InputAdornment position="start"><PersonOutlineOutlinedIcon /></InputAdornment> },
              }}
            />

            <Box className="rk-login-field-label rk-login-password-label">Password</Box>
            <TextField
              fullWidth
              type={showPassword ? 'text' : 'password'}
              value={password}
              disabled={loading || cooldown > 0}
              autoComplete="current-password"
              placeholder="Masukkan password"
              onChange={(e) => { setPassword(e.target.value); if (errors.password) setErrors((p) => ({ ...p, password: '' })); }}
              error={!!errors.password}
              helperText={errors.password}
              onKeyDown={(e) => e.key === 'Enter' && handleLogin()}
              className="rk-login-input"
              slotProps={{
                input: {
                  startAdornment: <InputAdornment position="start"><LockOutlinedIcon /></InputAdornment>,
                  endAdornment: <InputAdornment position="end"><IconButton onClick={() => setShowPassword((v) => !v)} edge="end" aria-label={showPassword ? 'Sembunyikan password' : 'Tampilkan password'}>{showPassword ? <VisibilityOffOutlinedIcon /> : <VisibilityOutlinedIcon />}</IconButton></InputAdornment>,
                },
              }}
            />

            <Box className="rk-login-options">
              <label><input type="checkbox" defaultChecked /> <span>Ingat saya</span></label>
              <Link component="button" type="button" underline="hover" onClick={() => navigate('/lupa-password')}>Lupa password?</Link>
            </Box>

            <Button fullWidth variant="contained" size="large" onClick={handleLogin} disabled={loading || cooldown > 0} className="rk-login-submit">
              {loading ? <CircularProgress size={20} color="inherit" /> : cooldown > 0 ? `Coba lagi ${cooldown}d` : 'Masuk'}
            </Button>

            <Box className="rk-login-divider"><span /> <em>atau</em> <span /></Box>
            <Button fullWidth variant="outlined" disabled className="rk-google-button"><span className="rk-google-mark">G</span> Masuk dengan Google</Button>

            <Typography className="rk-login-footer">© 2024 Ruangkirim. Semua hak dilindungi.</Typography>
          </Box>
        </Box>
      </Box>
    </Box>
  );
}
