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

  const adornmentSx = {
    width: { xs: 'clamp(32px, 5vw, 40px)', md: 40 },
    height: { xs: 'clamp(32px, 5vw, 40px)', md: 40 },
    borderRadius: '10px',
    bgcolor: '#edf4ff',
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'center',
    color: '#1764ff',
    ml: 0.25,
  };

  return (
    <Box sx={{ minHeight: '100svh', width: '100%', bgcolor: '#f5f8fd', p: { xs: 0, md: 3 }, fontFamily: 'Inter, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif', overflowX: 'hidden', position: 'relative' }}>
      <Box sx={{ position: 'absolute', top: { xs: -120, md: -180 }, right: { xs: -150, md: -180 }, width: { xs: 330, md: 430 }, height: { xs: 330, md: 430 }, borderRadius: '50%', bgcolor: 'rgba(100,149,255,0.055)', pointerEvents: 'none' }} />
      <Box sx={{ position: 'absolute', bottom: { xs: -170, md: -220 }, left: { xs: -170, md: 180 }, width: { xs: 350, md: 430 }, height: { xs: 350, md: 430 }, borderRadius: '50%', bgcolor: 'rgba(100,149,255,0.045)', pointerEvents: 'none' }} />

      <Box sx={{ width: '100%', maxWidth: 1360, minHeight: { xs: '100svh', md: 'calc(100svh - 48px)' }, mx: 'auto', display: 'grid', gridTemplateColumns: { xs: '1fr', md: '56% 44%' }, bgcolor: '#fff', borderRadius: { xs: 0, md: '24px' }, overflow: 'hidden', boxShadow: { xs: 'none', md: '0 8px 32px rgba(31,64,112,0.08)' }, position: 'relative', zIndex: 1 }}>
        <Box sx={{ display: { xs: 'none', md: 'flex' }, position: 'relative', overflow: 'hidden', flexDirection: 'column', justifyContent: 'center', bgcolor: '#f4f7fc', p: { md: 5, lg: 5.5 }, '&::before': { content: '""', position: 'absolute', width: 460, height: 460, borderRadius: '50%', bgcolor: 'rgba(76,139,255,0.055)', top: -180, right: -130 }, '&::after': { content: '""', position: 'absolute', width: 430, height: 430, borderRadius: '50%', bgcolor: 'rgba(76,139,255,0.045)', bottom: -260, left: 140 } }}>
          <Box sx={{ position: 'relative', zIndex: 1, display: 'grid', gridTemplateColumns: '46% 54%', gap: 1, alignItems: 'start' }}>
            <Box sx={{ pt: 1 }}>
              <Typography sx={{ fontSize: { md: '2.05rem', lg: '2.2rem' }, lineHeight: 1.05, fontWeight: 800, letterSpacing: '-0.04em', color: '#101a3d' }}>WhatsApp</Typography>
              <Typography sx={{ fontSize: { md: '2.05rem', lg: '2.2rem' }, lineHeight: 1.05, fontWeight: 800, letterSpacing: '-0.04em', color: '#1764ff' }}>AI Assistant</Typography>
              <Typography sx={{ mt: 2.2, maxWidth: 325, fontSize: { md: '0.92rem', lg: '0.98rem' }, lineHeight: 1.55, color: '#64718d' }}>Otomatis balasan AI, broadcast massal, dan CRM WhatsApp — semua dalam satu dashboard.</Typography>
            </Box>
            <Box sx={{ display: 'flex', justifyContent: 'center', alignItems: 'center', minHeight: 350 }}>
              <Box component="img" src="/assets/illustration-ruangkirim.png" alt="WhatsApp AI Assistant" sx={{ width: '100%', maxWidth: 350, maxHeight: 365, objectFit: 'contain' }} />
            </Box>
          </Box>

          <Box sx={{ position: 'relative', zIndex: 1, mt: 1, width: '100%', maxWidth: 520 }}>
            {features.map((feature, index) => (
              <Box key={feature.title} sx={{ display: 'flex', alignItems: 'center', gap: 1.8, py: 1.35, borderBottom: index < features.length - 1 ? '1px solid #e3e9f2' : 'none' }}>
                <Box sx={{ width: 48, height: 48, flex: '0 0 48px', display: 'flex', alignItems: 'center', justifyContent: 'center', borderRadius: '12px', bgcolor: '#fff', border: '1px solid #e1e8f3', color: '#1764ff', boxShadow: '0 4px 10px rgba(45,86,148,0.06)' }}>{feature.icon}</Box>
                <Box>
                  <Typography sx={{ fontWeight: 700, fontSize: '0.9rem', color: '#132044' }}>{feature.title}</Typography>
                  <Typography sx={{ mt: 0.3, fontSize: '0.76rem', lineHeight: 1.4, color: '#6d7891' }}>{feature.description}</Typography>
                </Box>
              </Box>
            ))}
          </Box>

          <Box sx={{ position: 'relative', zIndex: 1, mt: 3, width: '100%', maxWidth: 520 }}>
            <Box sx={{ display: 'flex', alignItems: 'center', gap: 1.5, px: 2, py: 1.45, borderRadius: '12px', border: '1px solid #cddcf7', bgcolor: 'rgba(229,239,255,0.58)' }}>
              <Box sx={{ width: 42, height: 42, flex: '0 0 42px', borderRadius: '10px', bgcolor: '#1764ff', color: '#fff', display: 'flex', alignItems: 'center', justifyContent: 'center' }}><ShieldOutlined /></Box>
              <Box><Typography sx={{ fontWeight: 700, fontSize: '0.86rem', color: '#14234b' }}>Aman &amp; Terpercaya</Typography><Typography sx={{ mt: 0.2, fontSize: '0.74rem', color: '#687590' }}>Data Anda dienkripsi dan terlindungi dengan standar keamanan tinggi.</Typography></Box>
            </Box>
          </Box>
        </Box>

        <Box sx={{ minWidth: 0, display: 'flex', flexDirection: 'column', justifyContent: { xs: 'flex-start', md: 'center' }, bgcolor: '#fff', px: { xs: 'clamp(24px, 14vw, 120px)', sm: 'clamp(32px, 10vw, 96px)', md: 6.5, lg: 7.5 }, py: { xs: 'clamp(34px, 6vw, 52px)', sm: 6, md: 5 } }}>
          <Box sx={{ width: '100%', maxWidth: { xs: 590, md: 425 }, mx: 'auto' }}>
            <Box sx={{ display: 'flex', justifyContent: 'flex-start', mb: { xs: 'clamp(28px, 5vw, 50px)', md: 3.2 } }}>
              <Box component="img" src={logo} alt="RuangKirim" sx={{ width: { xs: 'clamp(160px, 36.5vw, 310px)', md: 185 }, height: 'auto', display: 'block', objectFit: 'contain' }} />
            </Box>

            <Box sx={{ mb: { xs: 'clamp(28px, 4.5vw, 40px)', md: 3.2 } }}>
              <Typography component="h1" sx={{ fontSize: { xs: 'clamp(22px, 3.6vw, 30px)', md: '1.42rem' }, lineHeight: 1.2, fontWeight: 800, letterSpacing: '-0.035em', color: '#111a3a' }}>Selamat datang kembali! 👋</Typography>
              <Typography sx={{ mt: 1.25, fontSize: { xs: 'clamp(14px, 2.2vw, 18px)', md: '0.8rem' }, lineHeight: 1.55, color: '#687590' }}>Masuk ke dashboard untuk mengelola WhatsApp AI Assistant kamu.</Typography>
            </Box>

            {error && <Alert severity={cooldown > 0 ? 'warning' : 'error'} sx={{ mb: 2.5, borderRadius: '12px' }}>{error}</Alert>}
            {needVerify && <Typography sx={{ mb: 2, fontSize: '0.82rem' }}><Link component="button" type="button" underline="hover" onClick={() => navigate('/cek-email', { state: { email: username.trim() } })} sx={{ fontWeight: 600, color: '#1764ff' }}>Kirim ulang link verifikasi</Link></Typography>}

            <Box component="form" onSubmit={(event) => { event.preventDefault(); void handleLogin(); }}>
              <Box sx={{ mb: { xs: 'clamp(18px, 3.2vw, 28px)', md: 2.25 } }}>
                <Typography sx={{ mb: 1.05, fontSize: { xs: 'clamp(14px, 2.35vw, 20px)', md: '0.82rem' }, fontWeight: 700, color: '#111a3a' }}>Username</Typography>
                <TextField fullWidth placeholder="Masukkan username" value={username} disabled={loading || cooldown > 0} autoComplete="username" onChange={(e) => { setUsername(e.target.value); if (errors.username) setErrors((p) => ({ ...p, username: '' })); }} error={!!errors.username} helperText={errors.username} sx={fieldSx} slotProps={{ input: { sx: { height: { xs: 'clamp(48px, 9.4vw, 80px)', md: 62 }, borderRadius: '12px', fontSize: { xs: 'clamp(14px, 2.1vw, 18px)', md: '0.92rem' }, fontWeight: 500 }, startAdornment: <InputAdornment position="start"><Box sx={adornmentSx}><PersonOutlined fontSize="small" /></Box></InputAdornment> } }} />
              </Box>

              <Box sx={{ mb: { xs: 'clamp(16px, 2.8vw, 24px)', md: 1.55 } }}>
                <Typography sx={{ mb: 1.05, fontSize: { xs: 'clamp(14px, 2.35vw, 20px)', md: '0.82rem' }, fontWeight: 700, color: '#111a3a' }}>Password</Typography>
                <TextField fullWidth placeholder="Masukkan password" type={showPassword ? 'text' : 'password'} value={password} disabled={loading || cooldown > 0} autoComplete="current-password" onChange={(e) => { setPassword(e.target.value); if (errors.password) setErrors((p) => ({ ...p, password: '' })); }} error={!!errors.password} helperText={errors.password} sx={fieldSx} slotProps={{ input: { sx: { height: { xs: 'clamp(48px, 9.4vw, 80px)', md: 62 }, borderRadius: '12px', fontSize: { xs: 'clamp(14px, 2.1vw, 18px)', md: '0.92rem' }, fontWeight: 500 }, startAdornment: <InputAdornment position="start"><Box sx={adornmentSx}><LockOutlined fontSize="small" /></Box></InputAdornment>, endAdornment: <InputAdornment position="end"><IconButton type="button" onClick={() => setShowPassword((value) => !value)} edge="end" size="small" aria-label={showPassword ? 'Sembunyikan password' : 'Tampilkan password'}><Box sx={{ display: 'flex', color: '#71809c' }}>{showPassword ? <VisibilityOff fontSize="small" /> : <Visibility fontSize="small" />}</Box></IconButton></InputAdornment> } }} />
              </Box>

              <Box sx={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 2, mb: { xs: 'clamp(22px, 3.8vw, 32px)', md: 2.45 } }}>
                <FormControlLabel control={<Checkbox defaultChecked size="small" sx={{ p: 0.35, color: '#1764ff', '&.Mui-checked': { color: '#1764ff' } }} />} label="Ingat saya" sx={{ m: 0, '& .MuiFormControlLabel-label': { fontSize: { xs: 'clamp(14px, 2.2vw, 18px)', md: '0.82rem' }, color: '#66728c' } }} />
                <Link component="button" type="button" underline="hover" onClick={() => navigate('/forgot-password')} sx={{ fontSize: { xs: 'clamp(14px, 2.2vw, 18px)', md: '0.82rem' }, fontWeight: 700, color: '#1764ff' }}>Lupa password?</Link>
              </Box>

              <Button fullWidth type="submit" variant="contained" size="large" disabled={loading || cooldown > 0} startIcon={loading ? <CircularProgress size={18} color="inherit" /> : null} sx={{ height: { xs: 'clamp(48px, 9.4vw, 80px)', md: 54 }, borderRadius: '12px', bgcolor: '#1764ff', textTransform: 'none', fontSize: { xs: 'clamp(15px, 2.4vw, 20px)', md: '0.95rem' }, fontWeight: 700, boxShadow: 'none', '&:hover': { bgcolor: '#1056e3', boxShadow: 'none' } }}>{loading ? 'Masuk…' : cooldown > 0 ? `Coba lagi ${cooldown}d` : 'Masuk'}</Button>

              <Box sx={{ display: 'flex', alignItems: 'center', gap: 2, my: { xs: 'clamp(20px, 3.8vw, 32px)', md: 2.5 } }}><Divider sx={{ flex: 1, borderColor: '#e0e5ed' }} /><Typography sx={{ color: '#77829a', fontSize: { xs: 'clamp(14px, 2.1vw, 18px)', md: '0.8rem' } }}>atau</Typography><Divider sx={{ flex: 1, borderColor: '#e0e5ed' }} /></Box>

              {/* TODO: Google Login — visual placeholder only; OAuth will be implemented later. */}
              <Button fullWidth type="button" variant="outlined" size="large" startIcon={<GoogleIcon />} onClick={() => undefined} sx={{ height: { xs: 'clamp(48px, 9.4vw, 80px)', md: 54 }, borderRadius: '12px', borderColor: '#dfe5ee', color: '#17203f', textTransform: 'none', fontSize: { xs: 'clamp(14px, 2.3vw, 18px)', md: '0.9rem' }, fontWeight: 700, bgcolor: '#fff', '&:hover': { borderColor: '#cbd5e1', bgcolor: '#fff' } }}>Masuk dengan Google</Button>
            </Box>

            <Box sx={{ display: { xs: 'block', md: 'none' }, mt: 'clamp(20px, 4.5vw, 40px)', borderRadius: '28px', bgcolor: '#fff', border: '1px solid #edf0f5', px: 'clamp(14px, 4vw, 24px)', py: 1.1, boxShadow: '0 10px 32px rgba(31,64,112,0.035)' }}>
              {features.map((feature, index) => <Box key={feature.title} sx={{ display: 'flex', alignItems: 'center', gap: 'clamp(12px, 3.5vw, 20px)', py: 'clamp(14px, 3.2vw, 24px)', borderBottom: index < features.length - 1 ? '1px solid #edf0f5' : 'none' }}><Box sx={{ width: 'clamp(32px, 7.5vw, 64px)', height: 'clamp(32px, 7.5vw, 64px)', flex: '0 0 auto', borderRadius: '14px', bgcolor: '#edf4ff', color: '#1764ff', display: 'flex', alignItems: 'center', justifyContent: 'center' }}>{feature.icon}</Box><Box sx={{ minWidth: 0 }}><Typography sx={{ fontWeight: 800, fontSize: { xs: 'clamp(13px, 2.1vw, 17px)' }, color: '#132044' }}>{feature.title}</Typography><Typography sx={{ mt: 0.4, fontSize: { xs: 'clamp(11px, 1.8vw, 15px)' }, lineHeight: 1.45, color: '#66728c' }}>{feature.description}</Typography></Box></Box>)}
            </Box>

            <Box sx={{ display: { xs: 'flex', md: 'none' }, alignItems: 'center', gap: 'clamp(12px, 3.5vw, 20px)', mt: 'clamp(12px, 2.5vw, 22px)', px: 'clamp(14px, 4vw, 24px)', py: 'clamp(14px, 3.2vw, 24px)', borderRadius: '28px', bgcolor: '#fff', border: '1px solid #edf0f5', boxShadow: '0 10px 32px rgba(31,64,112,0.035)' }}><Box sx={{ width: 'clamp(32px, 7.5vw, 64px)', height: 'clamp(32px, 7.5vw, 64px)', flex: '0 0 auto', borderRadius: '14px', bgcolor: '#edf4ff', color: '#1764ff', display: 'flex', alignItems: 'center', justifyContent: 'center' }}><ShieldOutlined sx={{ fontSize: 'clamp(18px, 4vw, 34px)' }} /></Box><Box><Typography sx={{ fontWeight: 800, fontSize: { xs: 'clamp(13px, 2.1vw, 17px)' }, color: '#132044' }}>Aman &amp; Terpercaya</Typography><Typography sx={{ mt: 0.4, fontSize: { xs: 'clamp(11px, 1.8vw, 15px)' }, lineHeight: 1.45, color: '#66728c' }}>Data Anda dienkripsi dan terlindungi dengan standar keamanan tinggi.</Typography></Box></Box>

            <Typography sx={{ mt: 'clamp(28px, 5vw, 44px)', mb: { xs: 1, md: 0 }, textAlign: 'center', fontSize: { xs: 'clamp(11px, 1.9vw, 15px)', md: '0.74rem' }, color: '#65718b' }}>© 2024 Ruangkirim. Semua hak dilindungi.</Typography>
          </Box>
        </Box>
      </Box>
    </Box>
  );
}
