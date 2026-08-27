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
    <Box className="login-chat-mockup" aria-hidden="true" sx={{ position: 'relative', width: { md: 220, lg: 250, xl: 270 }, height: { md: 300, lg: 335, xl: 365 }, borderRadius: '28px', p: '10px', background: 'linear-gradient(145deg,#2f79ff 0%,#0755e9 55%,#0049d8 100%)', boxShadow: '0 22px 45px rgba(30,91,220,.20)', transform: 'translateY(4px)' }}>
      <Box sx={{ position: 'absolute', inset: 10, borderRadius: '20px', overflow: 'hidden', background: 'linear-gradient(180deg,#0b5bf0 0%,#0b57e9 100%)', p: { md: 2, lg: 2.4 } }}>
        <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, mb: 2.4 }}>
          <Box sx={{ width: 26, height: 26, borderRadius: '50%', bgcolor: 'rgba(255,255,255,.35)' }} />
          <Box sx={{ flex: 1 }}>
            <Box sx={{ width: '58%', height: 5, borderRadius: 5, bgcolor: 'rgba(255,255,255,.55)', mb: .7 }} />
            <Box sx={{ width: '42%', height: 4, borderRadius: 5, bgcolor: 'rgba(255,255,255,.28)' }} />
          </Box>
        </Box>
        <Box sx={{ width: '82%', minHeight: 48, borderRadius: '13px 13px 13px 4px', bgcolor: 'rgba(255,255,255,.20)', p: 1.4, mb: 1.6 }}>
          <Box sx={{ width: '80%', height: 5, borderRadius: 4, bgcolor: 'rgba(255,255,255,.50)', mb: 1 }} />
          <Box sx={{ width: '58%', height: 5, borderRadius: 4, bgcolor: 'rgba(255,255,255,.34)' }} />
        </Box>
        <Box sx={{ width: '70%', ml: 'auto', minHeight: 44, borderRadius: '13px 13px 4px 13px', bgcolor: '#39cfd0', p: 1.35, mb: 1.6 }}>
          <Box sx={{ width: '72%', height: 5, borderRadius: 4, bgcolor: 'rgba(255,255,255,.75)', mb: 1 }} />
          <Box sx={{ width: '48%', height: 5, borderRadius: 4, bgcolor: 'rgba(255,255,255,.52)' }} />
        </Box>
        <Box sx={{ width: '88%', minHeight: 60, borderRadius: '13px 13px 13px 4px', bgcolor: 'rgba(255,255,255,.19)', p: 1.4, mb: 1.8 }}>
          <Box sx={{ width: '84%', height: 5, borderRadius: 4, bgcolor: 'rgba(255,255,255,.50)', mb: 1 }} />
          <Box sx={{ width: '68%', height: 5, borderRadius: 4, bgcolor: 'rgba(255,255,255,.35)', mb: 1 }} />
          <Box sx={{ width: '34%', height: 5, borderRadius: 4, bgcolor: 'rgba(255,255,255,.28)' }} />
        </Box>
        <Box sx={{ display: 'inline-flex', alignItems: 'center', gap: .6, px: 1.1, py: .55, borderRadius: 1.5, bgcolor: 'rgba(255,255,255,.22)' }}>
          <Typography sx={{ color: '#fff', fontSize: { md: 9, lg: 10 }, fontWeight: 800, lineHeight: 1 }}>AI</Typography>
        </Box>
        <Box sx={{ position: 'absolute', left: 18, bottom: 13, display: 'flex', gap: .7 }}>
          {[0, 1, 2].map((dot) => <Box key={dot} sx={{ width: 7, height: 7, borderRadius: '50%', bgcolor: dot === 0 ? '#fff' : 'rgba(255,255,255,.35)' }} />)}
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
  const [turnstileToken] = useState('');
  const [showPassword, setShowPassword] = useState(false);
  const navigate = useNavigate();

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
    '& .MuiOutlinedInput-notchedOutline': { borderColor: '#dfe5ee' },
    '& .MuiOutlinedInput-root': { borderRadius: '12px', bgcolor: '#fff' },
    '& .MuiOutlinedInput-root:hover .MuiOutlinedInput-notchedOutline': { borderColor: '#cbd5e1' },
    '& .MuiOutlinedInput-root.Mui-focused .MuiOutlinedInput-notchedOutline': { borderColor: '#1764ff', borderWidth: 2 },
  };

  const adornmentSx = { width: { xs: 40, md: 40 }, height: { xs: 40, md: 40 }, borderRadius: '10px', bgcolor: '#edf4ff', display: 'flex', alignItems: 'center', justifyContent: 'center', color: '#1764ff', ml: .25 };

  return (
    <Box className="login-page" sx={{ minHeight: '100svh', width: '100%', bgcolor: '#f4f7fc', p: { xs: 0, md: 3 }, fontFamily: 'Inter, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif', overflowX: 'hidden', position: 'relative' }}>
      <Box sx={{ position: 'absolute', top: -180, right: -180, width: 430, height: 430, borderRadius: '50%', bgcolor: 'rgba(76,139,255,.055)', pointerEvents: 'none' }} />
      <Box sx={{ position: 'absolute', bottom: -230, left: { xs: -180, md: 150 }, width: 440, height: 440, borderRadius: '50%', bgcolor: 'rgba(76,139,255,.045)', pointerEvents: 'none' }} />

      <Box className="login-page-container" sx={{ width: '100%', maxWidth: 1360, minHeight: { xs: '100svh', md: 'calc(100svh - 48px)' }, mx: 'auto', display: 'grid', gridTemplateColumns: { xs: '1fr', md: '56% 44%' }, bgcolor: '#fff', borderRadius: { xs: 0, md: '24px' }, overflow: 'hidden', boxShadow: { xs: 'none', md: '0 8px 32px rgba(31,64,112,.08)' }, position: 'relative', zIndex: 1 }}>
        <Box className="login-page-left-panel" sx={{ display: { xs: 'none', md: 'flex' }, position: 'relative', overflow: 'hidden', flexDirection: 'column', bgcolor: '#f3f7fd', px: { md: 5.5, lg: 6.5 }, py: { md: 5, lg: 6 }, justifyContent: 'center' }}>
          <Box sx={{ position: 'relative', zIndex: 1, width: '100%', maxWidth: 720, mx: 'auto' }}>
            <Box sx={{ display: 'grid', gridTemplateColumns: '44% 56%', alignItems: 'center', gap: 2, mb: { md: 3, lg: 4 } }}>
              <Box>
                <Typography sx={{ fontSize: { md: '2rem', lg: '2.25rem', xl: '2.45rem' }, lineHeight: 1.03, fontWeight: 800, letterSpacing: '-.045em', color: '#111b40' }}>WhatsApp</Typography>
                <Typography sx={{ fontSize: { md: '2rem', lg: '2.25rem', xl: '2.45rem' }, lineHeight: 1.03, fontWeight: 800, letterSpacing: '-.045em', color: '#1764ff' }}>AI Assistant</Typography>
                <Typography sx={{ mt: 2, maxWidth: 320, fontSize: { md: '.86rem', lg: '.94rem' }, lineHeight: 1.55, color: '#66748e' }}>Otomatis balasan AI, broadcast massal, dan CRM WhatsApp — semua dalam satu dashboard.</Typography>
              </Box>
              <Box sx={{ display: 'flex', justifyContent: 'center', alignItems: 'center' }}>
                <ChatMockup />
              </Box>
            </Box>

            <Box className="login-feature-list" sx={{ width: '100%', maxWidth: 620 }}>
              {features.map((feature) => (
                <Box key={feature.title} sx={{ display: 'flex', alignItems: 'center', gap: 1.6, py: { md: 1.05, lg: 1.25 }, borderBottom: '1px solid rgba(112,133,166,.18)' }}>
                  <Box sx={{ width: 44, height: 44, flex: '0 0 44px', display: 'flex', alignItems: 'center', justifyContent: 'center', borderRadius: '11px', bgcolor: 'rgba(255,255,255,.78)', color: '#1764ff' }}>{feature.icon}</Box>
                  <Box>
                    <Typography sx={{ fontWeight: 700, fontSize: { md: '.82rem', lg: '.9rem' }, color: '#132044' }}>{feature.title}</Typography>
                    <Typography sx={{ mt: .2, fontSize: { md: '.68rem', lg: '.75rem' }, lineHeight: 1.35, color: '#6b7891' }}>{feature.description}</Typography>
                  </Box>
                </Box>
              ))}
            </Box>

            <Box sx={{ display: 'flex', alignItems: 'center', gap: 1.4, mt: { md: 2.3, lg: 2.8 }, px: 1.6, py: 1.15, maxWidth: 520, borderRadius: '10px', bgcolor: 'rgba(224,237,255,.62)', border: '1px solid rgba(91,135,220,.18)' }}>
              <Box sx={{ width: 38, height: 38, flex: '0 0 38px', borderRadius: '9px', bgcolor: '#1764ff', color: '#fff', display: 'flex', alignItems: 'center', justifyContent: 'center' }}><ShieldOutlined fontSize="small" /></Box>
              <Box><Typography sx={{ fontWeight: 700, fontSize: '.8rem', color: '#14234b' }}>Aman &amp; Terpercaya</Typography><Typography sx={{ mt: .15, fontSize: '.68rem', lineHeight: 1.3, color: '#687590' }}>Data Anda dienkripsi dan terlindungi dengan standar keamanan tinggi.</Typography></Box>
            </Box>
          </Box>
        </Box>

        <Box className="login-page-right-panel" sx={{ minWidth: 0, display: 'flex', flexDirection: 'column', justifyContent: { xs: 'flex-start', md: 'center' }, bgcolor: '#fff', px: { xs: 'clamp(24px, 14vw, 120px)', sm: 'clamp(32px, 10vw, 96px)', md: 6.5, lg: 7.5 }, py: { xs: 'clamp(34px, 6vw, 52px)', sm: 6, md: 5 } }}>
          <Box className="login-page-form-wrapper" sx={{ width: '100%', maxWidth: { xs: 590, md: 425 }, mx: 'auto' }}>
            <Box className="item-logo" sx={{ display: 'flex', justifyContent: 'flex-start', mb: { xs: 'clamp(28px, 5vw, 50px)', md: 3.2 } }}><Box component="img" src={logo} alt="RuangKirim" sx={{ width: { xs: 'clamp(160px, 36.5vw, 310px)', md: 185 }, height: 'auto', display: 'block' }} /></Box>

            <Box className="item-welcome" sx={{ mb: { xs: 'clamp(28px, 4.5vw, 40px)', md: 3.2 } }}>
              <Typography component="h1" sx={{ fontSize: { xs: 'clamp(22px, 3.6vw, 30px)', md: '1.42rem' }, lineHeight: 1.2, fontWeight: 800, letterSpacing: '-.035em', color: '#111a3a' }}>Selamat datang kembali! 👋</Typography>
              <Typography sx={{ mt: 1.25, fontSize: { xs: 'clamp(14px, 2.2vw, 18px)', md: '.8rem' }, lineHeight: 1.55, color: '#687590' }}>Masuk ke dashboard untuk mengelola WhatsApp AI Assistant kamu.</Typography>
            </Box>

            {error && <Alert severity={cooldown > 0 ? 'warning' : 'error'} sx={{ mb: 2.5, borderRadius: '12px' }}>{error}</Alert>}
            {needVerify && <Typography sx={{ mb: 2, fontSize: '.82rem' }}><Link component="button" type="button" underline="hover" onClick={() => navigate('/cek-email', { state: { email: username.trim() } })} sx={{ fontWeight: 600, color: '#1764ff' }}>Kirim ulang link verifikasi</Link></Typography>}

            <Box className="item-form" component="form" onSubmit={(event) => { event.preventDefault(); void handleLogin(); }}>
              <Box sx={{ mb: { xs: 'clamp(18px, 3.2vw, 28px)', md: 2.25 } }}>
                <Typography sx={{ mb: 1.05, fontSize: { xs: 'clamp(14px, 2.35vw, 20px)', md: '.82rem' }, fontWeight: 700, color: '#111a3a' }}>Username</Typography>
                <TextField fullWidth placeholder="Masukkan username" value={username} disabled={loading || cooldown > 0} autoComplete="username" onChange={(e) => { setUsername(e.target.value); if (errors.username) setErrors((p) => ({ ...p, username: '' })); }} error={!!errors.username} helperText={errors.username} sx={fieldSx} slotProps={{ input: { sx: { height: { xs: 'clamp(48px, 9.4vw, 80px)', md: 62 }, borderRadius: '12px', fontSize: { xs: 'clamp(14px, 2.1vw, 18px)', md: '.92rem' }, fontWeight: 500 }, startAdornment: <InputAdornment position="start"><Box sx={adornmentSx}><PersonOutlined fontSize="small" /></Box></InputAdornment> } }} />
              </Box>

              <Box sx={{ mb: { xs: 'clamp(16px, 2.8vw, 24px)', md: 1.55 } }}>
                <Typography sx={{ mb: 1.05, fontSize: { xs: 'clamp(14px, 2.35vw, 20px)', md: '.82rem' }, fontWeight: 700, color: '#111a3a' }}>Password</Typography>
                <TextField fullWidth placeholder="Masukkan password" type={showPassword ? 'text' : 'password'} value={password} disabled={loading || cooldown > 0} autoComplete="current-password" onChange={(e) => { setPassword(e.target.value); if (errors.password) setErrors((p) => ({ ...p, password: '' })); }} error={!!errors.password} helperText={errors.password} sx={fieldSx} slotProps={{ input: { sx: { height: { xs: 'clamp(48px, 9.4vw, 80px)', md: 62 }, borderRadius: '12px', fontSize: { xs: 'clamp(14px, 2.1vw, 18px)', md: '.92rem' }, fontWeight: 500 }, startAdornment: <InputAdornment position="start"><Box sx={adornmentSx}><LockOutlined fontSize="small" /></Box></InputAdornment>, endAdornment: <InputAdornment position="end"><IconButton type="button" onClick={() => setShowPassword((value) => !value)} edge="end" size="small" aria-label={showPassword ? 'Sembunyikan password' : 'Tampilkan password'}><Box sx={{ display: 'flex', color: '#71809c' }}>{showPassword ? <VisibilityOff fontSize="small" /> : <Visibility fontSize="small" />}</Box></IconButton></InputAdornment> } }} />
              </Box>

              <Box sx={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 2, mb: { xs: 'clamp(22px, 3.8vw, 32px)', md: 2.45 } }}>
                <FormControlLabel control={<Checkbox defaultChecked size="small" sx={{ p: .35, color: '#1764ff', '&.Mui-checked': { color: '#1764ff' } }} />} label="Ingat saya" sx={{ m: 0, '& .MuiFormControlLabel-label': { fontSize: { xs: 'clamp(14px, 2.2vw, 18px)', md: '.82rem' }, color: '#66728c' } }} />
                <Link component="button" type="button" underline="hover" onClick={() => navigate('/forgot-password')} sx={{ fontSize: { xs: 'clamp(14px, 2.2vw, 18px)', md: '.82rem' }, fontWeight: 700, color: '#1764ff' }}>Lupa password?</Link>
              </Box>

              <Button fullWidth type="submit" variant="contained" size="large" disabled={loading || cooldown > 0} startIcon={loading ? <CircularProgress size={18} color="inherit" /> : null} sx={{ height: { xs: 'clamp(48px, 9.4vw, 80px)', md: 54 }, borderRadius: '12px', bgcolor: '#1764ff', textTransform: 'none', fontSize: { xs: 'clamp(15px, 2.4vw, 20px)', md: '.95rem' }, fontWeight: 700, boxShadow: 'none', '&:hover': { bgcolor: '#1056e3', boxShadow: 'none' } }}>{loading ? 'Masuk…' : cooldown > 0 ? `Coba lagi ${cooldown}d` : 'Masuk'}</Button>

              <Box sx={{ display: 'flex', alignItems: 'center', gap: 2, my: { xs: 'clamp(20px, 3.8vw, 32px)', md: 2.5 } }}><Divider sx={{ flex: 1, borderColor: '#e0e5ed' }} /><Typography sx={{ color: '#77829a', fontSize: { xs: 'clamp(14px, 2.1vw, 18px)', md: '.8rem' } }}>atau</Typography><Divider sx={{ flex: 1, borderColor: '#e0e5ed' }} /></Box>

              {/* TODO: Google Login — visual placeholder only; OAuth will be implemented later. */}
              <Button fullWidth type="button" variant="outlined" size="large" startIcon={<GoogleIcon />} onClick={() => undefined} sx={{ height: { xs: 'clamp(48px, 9.4vw, 80px)', md: 54 }, borderRadius: '12px', borderColor: '#dfe5ee', color: '#17203f', textTransform: 'none', fontSize: { xs: 'clamp(14px, 2.2vw, 18px)', md: '.92rem' }, fontWeight: 600, boxShadow: 'none', '&:hover': { borderColor: '#cbd5e1', bgcolor: '#fafcff' } }}>Masuk dengan Google</Button>
            </Box>

            <Box className="item-features" sx={{ display: { xs: 'block', md: 'none' }, mt: 4, border: '1px solid #edf0f5', borderRadius: '28px', px: 2.2, py: 1.4 }}>
              {features.map((feature, index) => (
                <Box key={feature.title} sx={{ display: 'flex', alignItems: 'center', gap: 1.8, py: 1.7, borderBottom: index < features.length - 1 ? '1px solid #edf0f5' : 'none' }}>
                  <Box sx={{ width: 48, height: 48, flex: '0 0 48px', borderRadius: '14px', bgcolor: '#edf4ff', color: '#1764ff', display: 'flex', alignItems: 'center', justifyContent: 'center' }}>{feature.icon}</Box>
                  <Box><Typography sx={{ fontWeight: 700, fontSize: '1rem', color: '#132044' }}>{feature.title}</Typography><Typography sx={{ mt: .25, fontSize: '.82rem', lineHeight: 1.45, color: '#687590' }}>{feature.description}</Typography></Box>
                </Box>
              ))}
            </Box>

            <Box className="item-security" sx={{ display: { xs: 'flex', md: 'none' }, alignItems: 'center', gap: 1.8, mt: 2.2, px: 2.2, py: 2, border: '1px solid #edf0f5', borderRadius: '24px' }}>
              <Box sx={{ width: 48, height: 48, flex: '0 0 48px', borderRadius: '14px', bgcolor: '#edf4ff', color: '#1764ff', display: 'flex', alignItems: 'center', justifyContent: 'center' }}><ShieldOutlined /></Box>
              <Box><Typography sx={{ fontWeight: 700, fontSize: '1rem', color: '#132044' }}>Aman &amp; Terpercaya</Typography><Typography sx={{ mt: .25, fontSize: '.82rem', lineHeight: 1.45, color: '#687590' }}>Data Anda dienkripsi dan terlindungi dengan standar keamanan tinggi.</Typography></Box>
            </Box>

            <Typography className="item-footer" sx={{ mt: { xs: 4.5, md: 5 }, textAlign: 'center', color: '#6c7891', fontSize: { xs: '.82rem', md: '.78rem' } }}>© 2024 Ruangkirim. Semua hak dilindungi.</Typography>
          </Box>
        </Box>
      </Box>
    </Box>
  );
}
