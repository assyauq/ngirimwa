import { useEffect, useState } from 'react';
import { Box, TextField, Button, Typography, Alert, CircularProgress, Link, InputAdornment, IconButton, Divider } from '@mui/material';
import { Visibility, VisibilityOff, LockOutlined, PersonOutlined, MessageOutlined, SendOutlined, ContactsOutlined, ShieldOutlined } from '@mui/icons-material';
import { useNavigate } from 'react-router-dom';
import api from '../services/api';
import { unlockInboxSound } from '../services/inboxSound';
// We use the new assets
import logo from '../assets/logo-ruangkirim.png';

function GoogleIcon() {
  return (
    <svg width="20" height="20" viewBox="0 0 24 24" xmlns="http://www.w3.org/2000/svg">
      <path d="M22.56 12.25c0-.78-.07-1.53-.2-2.25H12v4.26h5.92c-.26 1.37-1.04 2.53-2.21 3.31v2.77h3.57c2.08-1.92 3.28-4.74 3.28-8.09z" fill="#4285F4"/>
      <path d="M12 23c2.97 0 5.46-.98 7.28-2.66l-3.57-2.77c-.98.66-2.23 1.06-3.71 1.06-2.86 0-5.29-1.93-6.16-4.53H2.18v2.84C3.99 20.53 7.7 23 12 23z" fill="#34A853"/>
      <path d="M5.84 14.09c-.22-.66-.35-1.36-.35-2.09s.13-1.43.35-2.09V7.07H2.18C1.43 8.55 1 10.22 1 12s.43 3.45 1.18 4.93l2.85-2.22.81-.62z" fill="#FBBC05"/>
      <path d="M12 5.38c1.62 0 3.06.56 4.21 1.64l3.15-3.15C17.45 2.09 14.97 1 12 1 7.7 1 3.99 3.47 2.18 7.07l3.66 2.84c.87-2.6 3.3-4.53 6.16-4.53z" fill="#EA4335"/>
    </svg>
  );
}

function responseStatus(error: unknown) {
  if (typeof error === 'object' && error && 'response' in error) {
    return (error as { response?: { status?: number; headers?: Record<string, string> } }).response;
  }
  return undefined;
}

export default function Login() {
  const [username, setUsername] = useState('');
  const [password, setPassword] = useState('');
  const [error, setError] = useState('');
  const [errors, setErrors] = useState<Record<string, string>>({});
  const [loading, setLoading] = useState(false);
  const [cooldown, setCooldown] = useState(0);
  const [needVerify, setNeedVerify] = useState(false);
  const [turnstileToken] = useState(''); // Retained for compatibility
  const [showPassword, setShowPassword] = useState(false);
  const navigate = useNavigate();

  // Turnstile — disembunyikan dulu
  // useEffect(() => {
  //   const render = () => {
  //     if (window.turnstile) {
  //       window.turnstile.render('#turnstile-login', {
  //         sitekey: '0x4AAAAAADrLaq7r2pyIGOYs',
  //         callback: (token: string) => { setTurnstileToken(token); },
  //       });
  //     } else {
  //       setTimeout(render, 200);
  //     }
  //   };
  //   render();
  // }, []);

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
      // if (window.turnstile) window.turnstile.reset();
    }
  };

  return (
    <Box className="login-page login-page-container">
      
      {/* Left Panel - Branding & Features */}
      <Box className="login-page-left-panel">
        <Box className="item-branding" sx={{ mb: 4 }}>
          <Typography variant="h3" sx={{ fontWeight: 800, mb: 2, color: 'var(--rk-navy)', letterSpacing: '-0.5px' }}>
            WhatsApp<br />
            <span style={{ color: 'var(--rk-primary)' }}>AI Assistant</span>
          </Typography>
          <Typography sx={{ color: 'var(--rk-text-muted)', fontSize: '1.1rem', maxWidth: '380px', lineHeight: 1.6 }}>
            Solusi otomatisasi WhatsApp cerdas untuk meningkatkan layanan pelanggan dan interaksi bisnis Anda.
          </Typography>
        </Box>

        <Box className="item-features login-page-features-grid">
          <Box className="login-page-feature-card">
            <Box className="login-page-icon-wrapper">
              <MessageOutlined />
            </Box>
            <Typography sx={{ fontWeight: 600, fontSize: '0.95rem', mb: 0.5 }}>Balasan AI Cerdas</Typography>
            <Typography sx={{ fontSize: '0.85rem', color: 'var(--rk-text-muted)' }}>Merespons pelanggan otomatis 24/7</Typography>
          </Box>
          <Box className="login-page-feature-card">
            <Box className="login-page-icon-wrapper">
              <SendOutlined />
            </Box>
            <Typography sx={{ fontWeight: 600, fontSize: '0.95rem', mb: 0.5 }}>Broadcast Massal</Typography>
            <Typography sx={{ fontSize: '0.85rem', color: 'var(--rk-text-muted)' }}>Kirim pesan ke ribuan kontak sekaligus</Typography>
          </Box>
          <Box className="login-page-feature-card">
            <Box className="login-page-icon-wrapper">
              <ContactsOutlined />
            </Box>
            <Typography sx={{ fontWeight: 600, fontSize: '0.95rem', mb: 0.5 }}>CRM Terintegrasi</Typography>
            <Typography sx={{ fontSize: '0.85rem', color: 'var(--rk-text-muted)' }}>Kelola data pelanggan dalam satu tempat</Typography>
          </Box>
        </Box>

        <Box className="item-security" sx={{ mt: 4 }}>
          <Box sx={{ display: 'flex', alignItems: 'center', gap: 2, background: 'rgba(26,86,255,0.05)', p: 2, borderRadius: '12px', border: '1px solid rgba(26,86,255,0.1)' }}>
            <ShieldOutlined sx={{ color: 'var(--rk-primary)', fontSize: 32 }} />
            <Box>
              <Typography sx={{ fontWeight: 600, fontSize: '0.9rem', color: 'var(--rk-navy)' }}>Aman & Terpercaya</Typography>
              <Typography sx={{ fontSize: '0.8rem', color: 'var(--rk-text-muted)' }}>Data Anda dienkripsi end-to-end</Typography>
            </Box>
          </Box>
        </Box>

        <Box className="item-illustration" sx={{ mt: 6, display: 'flex', justifyContent: 'center' }}>
          <img 
            src="/assets/illustration-ruangkirim.png" 
            alt="AI Assistant Illustration" 
            style={{ maxWidth: '100%', height: 'auto', maxHeight: '350px', objectFit: 'contain' }}
            onError={(e) => { e.currentTarget.style.display = 'none'; }}
          />
        </Box>
      </Box>

      {/* Right Panel - Login Form */}
      <Box className="login-page-right-panel">
        
        <Box className="item-logo">
          <img
            src={logo}
            alt="RuangKirim"
            style={{
              height: '36px',
              width: 'auto',
              objectFit: 'contain',
              display: 'block'
            }}
            onError={(e) => {
              // Fallback jika asset hilang
              e.currentTarget.src = '/assets/logo-ruangkirim.png';
            }}
          />
        </Box>

        <Box className="item-welcome">
          <Typography variant="h4" sx={{ fontWeight: 700, mb: 1, color: 'var(--rk-text-main)', letterSpacing: '-0.5px', fontSize: '1.75rem' }}>
            Selamat datang kembali! 👋
          </Typography>
          <Typography sx={{ color: 'var(--rk-text-muted)', fontSize: '0.95rem' }}>
            Masuk ke akun Anda untuk melanjutkan
          </Typography>
        </Box>

        <Box className="item-form login-page-form-wrapper">
          {error && (
            <Alert severity={cooldown > 0 ? 'warning' : 'error'} sx={{ mb: 3, borderRadius: '12px', '& .MuiAlert-message': { fontSize: '0.9rem' } }}>
              {error}
            </Alert>
          )}

          {needVerify && (
            <Alert severity="info" sx={{ mb: 3, borderRadius: '12px', '& .MuiAlert-message': { fontSize: '0.9rem' } }}>
              Email belum diverifikasi.{' '}
              <Link
                component="button"
                type="button"
                underline="hover"
                onClick={() => navigate('/cek-email', { state: { email: username.trim() } })}
                sx={{ fontWeight: 600 }}
              >
                Kirim ulang
              </Link>
            </Alert>
          )}

          <Box sx={{ mb: 2.5 }}>
            <Typography className="login-page-input-label">Username</Typography>
            <TextField
              fullWidth
              placeholder="Masukkan username"
              value={username}
              disabled={loading || cooldown > 0}
              autoComplete="username"
              onChange={(e) => {
                setUsername(e.target.value);
                if (errors.username) setErrors((p) => ({ ...p, username: '' }));
              }}
              error={!!errors.username}
              helperText={errors.username}
              onKeyDown={(e) => e.key === 'Enter' && handleLogin()}
              // @ts-expect-error type is buggy in MUI v5/v6 mixed
              InputProps={{
                startAdornment: (
                  <InputAdornment position="start">
                    <PersonOutlined sx={{ color: 'var(--rk-text-muted)', fontSize: 20 }} />
                  </InputAdornment>
                ),
                sx: { borderRadius: '12px', bgcolor: '#fff', '&.Mui-focused': { boxShadow: '0 0 0 3px rgba(26,86,255,0.1)' } }
              }}
            />
          </Box>

          <Box sx={{ mb: 3 }}>
            <Typography className="login-page-input-label">Password</Typography>
            <TextField
              fullWidth
              placeholder="Masukkan password"
              type={showPassword ? 'text' : 'password'}
              value={password}
              disabled={loading || cooldown > 0}
              autoComplete="current-password"
              onChange={(e) => {
                setPassword(e.target.value);
                if (errors.password) setErrors((p) => ({ ...p, password: '' }));
              }}
              error={!!errors.password}
              helperText={errors.password}
              onKeyDown={(e) => e.key === 'Enter' && handleLogin()}
              // @ts-expect-error type is buggy in MUI v5/v6 mixed
              InputProps={{
                startAdornment: (
                  <InputAdornment position="start">
                    <LockOutlined sx={{ color: 'var(--rk-text-muted)', fontSize: 20 }} />
                  </InputAdornment>
                ),
                endAdornment: (
                  <InputAdornment position="end">
                    <IconButton onClick={() => setShowPassword(!showPassword)} edge="end" size="small">
                      {showPassword ? <VisibilityOff sx={{ fontSize: 20, color: 'var(--rk-text-muted)' }} /> : <Visibility sx={{ fontSize: 20, color: 'var(--rk-text-muted)' }} />}
                    </IconButton>
                  </InputAdornment>
                ),
                sx: { borderRadius: '12px', bgcolor: '#fff', '&.Mui-focused': { boxShadow: '0 0 0 3px rgba(26,86,255,0.1)' } }
              }}
            />
          </Box>

          {/* Turnstile — disembunyikan dulu */}
          {/* <Box id="turnstile-login" sx={{ mb: 2, display: 'flex', justifyContent: 'center' }} /> */}

          <Button
            fullWidth
            variant="contained"
            size="large"
            onClick={handleLogin}
            disabled={loading || cooldown > 0}
            startIcon={loading ? <CircularProgress size={18} color="inherit" /> : null}
            sx={{
              py: 1.5,
              fontWeight: 600,
              fontSize: '1rem',
              borderRadius: '12px',
              textTransform: 'none',
              bgcolor: 'var(--rk-primary)',
              boxShadow: '0 4px 12px rgba(26,86,255,0.2)',
              '&:hover': { bgcolor: 'var(--rk-primary-hover)', boxShadow: '0 6px 16px rgba(26,86,255,0.3)' },
            }}
          >
            {loading ? 'Masuk…' : cooldown > 0 ? `Coba lagi ${cooldown}d` : 'Masuk'}
          </Button>

          <Box sx={{ display: 'flex', alignItems: 'center', my: 3 }}>
            <Divider sx={{ flex: 1, borderColor: 'var(--rk-border)' }} />
            <Typography sx={{ px: 2, color: 'var(--rk-text-muted)', fontSize: '0.85rem' }}>atau</Typography>
            <Divider sx={{ flex: 1, borderColor: 'var(--rk-border)' }} />
          </Box>

          {/* TODO: Google Login — implement in future phase. */}
          <Button
            fullWidth
            variant="outlined"
            size="large"
            startIcon={<GoogleIcon />}
            onClick={() => {}}
            sx={{
              py: 1.5,
              fontWeight: 600,
              fontSize: '1rem',
              borderRadius: '12px',
              textTransform: 'none',
              color: 'var(--rk-text-main)',
              borderColor: 'var(--rk-border)',
              bgcolor: '#fff',
              '&:hover': { bgcolor: '#f8fafc', borderColor: '#cbd5e1' },
            }}
          >
            Masuk dengan Google
          </Button>
        </Box>

        <Box className="item-footer">
          <Typography sx={{ mt: 4, textAlign: 'center', color: '#94a3b8', fontSize: '0.75rem' }}>
            © {new Date().getFullYear()} Ruangkirim. Semua hak dilindungi.
          </Typography>
        </Box>

      </Box>

    </Box>
  );
}
