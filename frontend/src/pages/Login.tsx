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
    <Box className="login-chat-mockup" aria-hidden="true" sx={{ width: { md: 205, lg: 245, xl: 285 }, height: { md: 285, lg: 340, xl: 390 }, flex: '0 0 auto', p: '9px', borderRadius: { md: '25px', lg: '29px' }, background: 'linear-gradient(155deg,#2f7dff 0%,#1261ee 48%,#0349d6 100%)', boxShadow: '0 22px 42px rgba(22,92,226,.22)' }}>
      <Box sx={{ height: '100%', borderRadius: { md: '18px', lg: '22px' }, overflow: 'hidden', position: 'relative', background: 'linear-gradient(180deg,#0d62f4 0%,#0751dd 100%)', p: { md: 1.6, lg: 2 } }}>
        <Box sx={{ display: 'flex', alignItems: 'center', gap: .8, mb: { md: 2, lg: 2.6 } }}>
          <Box sx={{ width: { md: 23, lg: 27 }, height: { md: 23, lg: 27 }, borderRadius: '50%', bgcolor: 'rgba(255,255,255,.38)' }} />
          <Box sx={{ flex: 1 }}>
            <Box sx={{ width: '56%', height: 4, borderRadius: 4, bgcolor: 'rgba(255,255,255,.58)', mb: .6 }} />
            <Box sx={{ width: '38%', height: 3, borderRadius: 4, bgcolor: 'rgba(255,255,255,.30)' }} />
          </Box>
        </Box>
        <Box sx={{ width: '82%', borderRadius: '12px 12px 12px 4px', bgcolor: 'rgba(255,255,255,.20)', p: { md: 1, lg: 1.3 }, mb: { md: 1.2, lg: 1.5 } }}>
          <Box sx={{ width: '84%', height: 5, borderRadius: 4, bgcolor: 'rgba(255,255,255,.60)', mb: .8 }} />
          <Box sx={{ width: '62%', height: 5, borderRadius: 4, bgcolor: 'rgba(255,255,255,.35)' }} />
          <Typography sx={{ mt: .8, color: 'rgba(255,255,255,.60)', fontSize: { md: 6, lg: 7 } }}>09.30</Typography>
        </Box>
        <Box sx={{ width: '72%', ml: 'auto', borderRadius: '12px 12px 4px 12px', bgcolor: '#32ced0', p: { md: 1, lg: 1.3 }, mb: { md: 1.2, lg: 1.5 } }}>
          <Box sx={{ width: '86%', height: 5, borderRadius: 4, bgcolor: 'rgba(255,255,255,.78)', mb: .8 }} />
          <Box sx={{ width: '66%', height: 5, borderRadius: 4, bgcolor: 'rgba(255,255,255,.52)' }} />
          <Typography sx={{ mt: .8, color: 'rgba(255,255,255,.70)', fontSize: { md: 6, lg: 7 }, textAlign: 'right' }}>09.31 ✓✓</Typography>
        </Box>
        <Box sx={{ width: '80%', borderRadius: '12px 12px 12px 4px', bgcolor: 'rgba(255,255,255,.19)', p: { md: 1, lg: 1.3 }, mb: { md: 1.4, lg: 1.8 } }}>
          <Box sx={{ width: '78%', height: 5, borderRadius: 4, bgcolor: 'rgba(255,255,255,.52)', mb: .8 }} />
          <Box sx={{ width: '48%', height: 5, borderRadius: 4, bgcolor: 'rgba(255,255,255,.32)' }} />
          <Typography sx={{ mt: .8, color: 'rgba(255,255,255,.55)', fontSize: { md: 6, lg: 7 } }}>09.32</Typography>
        </Box>
        <Box sx={{ display: 'inline-flex', alignItems: 'center', gap: .6, px: 1, py: .5, borderRadius: 1.5, bgcolor: 'rgba(255,255,255,.20)' }}>
          <Typography sx={{ color: '#fff', fontWeight: 800, fontSize: { md: 8, lg: 10 }, lineHeight: 1 }}>AI</Typography>
          {[0, 1, 2].map((dot) => <Box key={dot} sx={{ width: 5, height: 5, borderRadius: '50%', bgcolor: dot === 0 ? '#fff' : 'rgba(255,255,255,.45)' }} />)}
        </Box>
        <Box sx={{ position: 'absolute', left: { md: 15, lg: 18 }, bottom: { md: 11, lg: 14 }, display: 'flex', gap: .6 }}>
          {[0, 1, 2].map((dot) => <Box key={dot} sx={{ width: 7, height: 7, borderRadius: '50%', bgcolor: dot === 0 ? '#fff' : 'rgba(255,255,255,.35)' }} />)}
        </Box>
      </Box>
    </Box>
  );
}

function FeatureList({ mobile = false }: { mobile?: boolean }) {
  return (
    <Box className="item-features" sx={{ width: '100%', maxWidth: mobile ? 390 : 690 }}>
      {features.map((feature) => (
        <Box key={feature.title} sx={{ display: 'flex', alignItems: 'center', gap: { xs: 1.5, md: 1.6 }, py: { xs: 1.35, md: 1.05, lg: 1.2 }, borderBottom: '1px solid rgba(108,128,159,.16)' }}>
          <Box sx={{ width: { xs: 42, md: 44 }, height: { xs: 42, md: 44 }, flex: '0 0 auto', borderRadius: { xs: '11px', md: '10px' }, display: 'flex', alignItems: 'center', justifyContent: 'center', bgcolor: '#edf4ff', color: '#1764ff' }}>{feature.icon}</Box>
          <Box sx={{ minWidth: 0 }}>
            <Typography sx={{ fontWeight: 700, fontSize: { xs: '.86rem', md: '.82rem', lg: '.9rem' }, color: '#132044' }}>{feature.title}</Typography>
            <Typography sx={{ mt: .25, fontSize: { xs: '.69rem', md: '.68rem', lg: '.75rem' }, lineHeight: 1.4, color: '#68768f' }}>{feature.description}</Typography>
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
    '& .MuiOutlinedInput-root': { borderRadius: '12px', bgcolor: '#fff', minHeight: { xs: 54, md: 54 } },
    '& .MuiOutlinedInput-notchedOutline': { borderColor: '#dfe5ee' },
    '& .MuiOutlinedInput-root:hover .MuiOutlinedInput-notchedOutline': { borderColor: '#cbd5e1' },
    '& .MuiOutlinedInput-root.Mui-focused .MuiOutlinedInput-notchedOutline': { borderColor: '#1764ff', borderWidth: 2 },
  };

  const inputIcon = { width: 38, height: 38, borderRadius: '10px', bgcolor: '#edf4ff', display: 'flex', alignItems: 'center', justifyContent: 'center', color: '#1764ff' };

  const formContent = (
    <>
      <Box className="item-logo" sx={{ mb: { xs: 2.5, md: 3.2 }, textAlign: 'left' }}>
        <Box component="img" src={logo} alt="Ruangkirim" sx={{ width: { xs: 190, md: 190, lg: 210 }, height: 'auto', display: 'block', objectFit: 'contain' }} />
      </Box>

      <Box className="item-welcome" sx={{ mb: { xs: 3.1, md: 3.5 } }}>
        <Typography sx={{ fontSize: { xs: '1.32rem', md: '1.22rem', lg: '1.35rem' }, lineHeight: 1.2, fontWeight: 700, letterSpacing: '-.02em', color: '#111b40' }}>Selamat datang kembali! 👋</Typography>
        <Typography sx={{ mt: 1.05, maxWidth: 480, fontSize: { xs: '.88rem', md: '.75rem', lg: '.82rem' }, lineHeight: 1.5, color: '#68768f' }}>Masuk ke dashboard untuk mengelola WhatsApp AI Assistant kamu.</Typography>
      </Box>

      <Box className="item-form">
        {error && (
          <Alert severity="error" sx={{ mb: 2, borderRadius: '10px', fontSize: '.8rem' }}>{error}</Alert>
        )}
        {needVerify && (
          <Box sx={{ mb: 2, fontSize: '.78rem', color: '#1764ff' }}>
            <Link component="button" underline="hover" onClick={() => navigate('/cek-email', { state: { email: username.trim() } })}>Buka halaman verifikasi email</Link>
          </Box>
        )}

        <Typography component="label" className="login-page-input-label" sx={{ display: 'block', mb: .75, fontSize: { xs: '.8rem', md: '.72rem', lg: '.78rem' }, fontWeight: 600, color: '#111b40' }}>Username</Typography>
        <TextField
          fullWidth
          size="small"
          value={username}
          placeholder="Masukkan username"
          error={Boolean(errors.username)}
          helperText={errors.username || ' '}
          onChange={(e) => {
            setUsername(e.target.value);
            if (errors.username) setErrors((prev) => { const next = { ...prev }; delete next.username; return next; });
          }}
          onKeyDown={(e) => e.key === 'Enter' && handleLogin()}
          sx={{ ...fieldSx, mb: .45, '& .MuiFormHelperText-root': { minHeight: 17, mt: .45, fontSize: '.68rem' } }}
          slotProps={{ input: { startAdornment: <InputAdornment position="start"><Box sx={inputIcon}><PersonOutlined fontSize="small" /></Box></InputAdornment> } }}
        />

        <Typography component="label" className="login-page-input-label" sx={{ display: 'block', mb: .75, mt: .55, fontSize: { xs: '.8rem', md: '.72rem', lg: '.78rem' }, fontWeight: 600, color: '#111b40' }}>Password</Typography>
        <TextField
          fullWidth
          size="small"
          type={showPassword ? 'text' : 'password'}
          value={password}
          placeholder="Masukkan password"
          error={Boolean(errors.password)}
          helperText={errors.password || ' '}
          onChange={(e) => {
            setPassword(e.target.value);
            if (errors.password) setErrors((prev) => { const next = { ...prev }; delete next.password; return next; });
          }}
          onKeyDown={(e) => e.key === 'Enter' && handleLogin()}
          sx={{ ...fieldSx, '& .MuiFormHelperText-root': { minHeight: 17, mt: .45, fontSize: '.68rem' } }}
          slotProps={{
            input: {
              startAdornment: <InputAdornment position="start"><Box sx={inputIcon}><LockOutlined fontSize="small" /></Box></InputAdornment>,
              endAdornment: <InputAdornment position="end"><IconButton aria-label={showPassword ? 'Sembunyikan password' : 'Tampilkan password'} onClick={() => setShowPassword((v) => !v)} edge="end" size="small" sx={{ color: '#74839d' }}>{showPassword ? <VisibilityOff fontSize="small" /> : <Visibility fontSize="small" />}</IconButton></InputAdornment>,
            },
          }}
        />

        <Box sx={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', mt: .25, mb: 2.2 }}>
          <FormControlLabel control={<Checkbox size="small" defaultChecked sx={{ p: .3, color: '#1764ff', '&.Mui-checked': { color: '#1764ff' } }} />} label="Ingat saya" sx={{ m: 0, '& .MuiFormControlLabel-label': { fontSize: { xs: '.8rem', md: '.7rem', lg: '.76rem' }, color: '#68768f' } }} />
          <Link component="button" variant="body2" underline="hover" onClick={() => navigate('/forgot-password')} sx={{ border: 0, background: 'none', p: 0, cursor: 'pointer', fontWeight: 600, color: '#1764ff', fontSize: { xs: '.8rem', md: '.7rem', lg: '.76rem' } }}>Lupa password?</Link>
        </Box>

        <Button fullWidth variant="contained" onClick={handleLogin} disabled={loading || cooldown > 0} sx={{ minHeight: { xs: 54, md: 50 }, borderRadius: '12px', textTransform: 'none', fontWeight: 700, fontSize: { xs: '1rem', md: '.86rem', lg: '.92rem' }, bgcolor: '#1764ff', boxShadow: 'none', '&:hover': { bgcolor: '#0e56e8', boxShadow: 'none' } }}>
          {loading ? <CircularProgress size={22} sx={{ color: '#fff' }} /> : cooldown > 0 ? `Coba lagi dalam ${cooldown}s` : 'Masuk'}
        </Button>

        <Divider sx={{ my: { xs: 2.6, md: 2.3 }, '&::before, &::after': { borderColor: '#e4e8ee' }, color: '#7b879b', fontSize: { xs: '.8rem', md: '.7rem' } }}>atau</Divider>

        <Button fullWidth variant="outlined" type="button" onClick={() => undefined} startIcon={<GoogleIcon />} sx={{ minHeight: { xs: 54, md: 48 }, borderRadius: '12px', textTransform: 'none', fontWeight: 600, color: '#1c2945', borderColor: '#dfe5ee', bgcolor: '#fff', fontSize: { xs: '.9rem', md: '.78rem', lg: '.84rem' }, '&:hover': { borderColor: '#dfe5ee', bgcolor: '#fff' } }}>Masuk dengan Google</Button>
      </Box>

      <Box className="item-features-mobile" sx={{ display: { xs: 'block', md: 'none' }, mt: 3.6 }}>
        <FeatureList mobile />
      </Box>

      <Box className="item-security-mobile" sx={{ display: { xs: 'flex', md: 'none' }, alignItems: 'center', gap: 1.4, mt: 1.2, p: 1.5, border: '1px solid #e6eaf0', borderRadius: '18px' }}>
        <Box sx={{ width: 42, height: 42, flex: '0 0 auto', borderRadius: '11px', bgcolor: '#edf4ff', color: '#1764ff', display: 'flex', alignItems: 'center', justifyContent: 'center' }}><ShieldOutlined /></Box>
        <Box><Typography sx={{ fontWeight: 700, fontSize: '.86rem', color: '#132044' }}>Aman &amp; Terpercaya</Typography><Typography sx={{ mt: .2, fontSize: '.69rem', lineHeight: 1.45, color: '#68768f' }}>Data Anda dienkripsi dan terlindungi dengan standar keamanan tinggi.</Typography></Box>
      </Box>

      <Typography className="item-footer" sx={{ mt: { xs: 5, md: 5 }, textAlign: 'center', fontSize: { xs: '.68rem', md: '.64rem', lg: '.7rem' }, color: '#68768f' }}>© 2024 Ruangkirim. Semua hak dilindungi.</Typography>
    </>
  );

  return (
    <Box className="login-page" sx={{ minHeight: '100svh', width: '100%', bgcolor: '#f4f7fc', p: { xs: 0, md: 3 }, fontFamily: 'Inter, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif', overflowX: 'hidden', position: 'relative' }}>
      <Box sx={{ position: 'absolute', top: -180, right: -180, width: 430, height: 430, borderRadius: '50%', bgcolor: 'rgba(76,139,255,.055)', pointerEvents: 'none' }} />
      <Box sx={{ position: 'absolute', bottom: -230, left: { xs: -180, md: 150 }, width: 440, height: 440, borderRadius: '50%', bgcolor: 'rgba(76,139,255,.045)', pointerEvents: 'none' }} />

      <Box className="login-page-container" sx={{ width: '100%', maxWidth: 1360, minHeight: { xs: '100svh', md: 'calc(100svh - 48px)' }, mx: 'auto', display: 'grid', gridTemplateColumns: { xs: '1fr', md: '58% 42%' }, bgcolor: '#fff', borderRadius: { xs: 0, md: '24px' }, overflow: 'hidden', boxShadow: { xs: 'none', md: '0 8px 32px rgba(31,64,112,.08)' }, position: 'relative', zIndex: 1 }}>
        <Box className="login-page-left-panel" sx={{ display: { xs: 'flex', md: 'flex' }, position: 'relative', overflow: 'hidden', flexDirection: 'column', justifyContent: 'center', bgcolor: '#f3f7fd', px: { md: 5, lg: 6, xl: 7 }, py: { md: 5, lg: 6 }, '&::before': { content: '""', position: 'absolute', width: 430, height: 430, borderRadius: '50%', top: -250, right: -170, bgcolor: 'rgba(86,143,255,.06)' }, '&::after': { content: '""', position: 'absolute', width: 430, height: 430, borderRadius: '50%', bottom: -300, left: -80, bgcolor: 'rgba(86,143,255,.055)' } }}>
          <Box sx={{ position: 'relative', zIndex: 1, width: '100%', maxWidth: 790, mx: 'auto' }}>
            <Box className="item-branding" sx={{ display: { xs: 'none', md: 'block' } }}>
              <Box sx={{ display: 'grid', gridTemplateColumns: { md: '42% 58%', lg: '40% 60%' }, alignItems: 'center', gap: { md: 2, lg: 3 }, mb: { md: 3, lg: 3.5 } }}>
                <Box>
                  <Typography sx={{ fontSize: { md: '2rem', lg: '2.35rem', xl: '2.65rem' }, lineHeight: 1.02, fontWeight: 800, letterSpacing: '-.05em', color: '#111b40' }}>WhatsApp</Typography>
                  <Typography sx={{ fontSize: { md: '2rem', lg: '2.35rem', xl: '2.65rem' }, lineHeight: 1.02, fontWeight: 800, letterSpacing: '-.05em', color: '#1764ff' }}>AI Assistant</Typography>
                  <Typography sx={{ mt: 1.8, maxWidth: 315, fontSize: { md: '.83rem', lg: '.92rem' }, lineHeight: 1.55, color: '#66748e' }}>Otomatis balasan AI, broadcast massal, dan CRM WhatsApp — semua dalam satu dashboard.</Typography>
                </Box>
                <Box className="item-illustration" sx={{ display: 'flex', justifyContent: 'center', alignItems: 'center' }}><ChatMockup /></Box>
              </Box>
            </Box>

            <Box className="item-features-desktop" sx={{ display: { xs: 'none', md: 'block' } }}><FeatureList /></Box>

            <Box className="item-security" sx={{ display: { xs: 'none', md: 'flex' }, alignItems: 'center', gap: 1.3, mt: { md: 2.2, lg: 2.8 }, maxWidth: 610, px: 1.5, py: 1.15, borderRadius: '11px', bgcolor: 'rgba(226,238,255,.62)', border: '1px solid rgba(91,135,220,.16)' }}>
              <Box sx={{ width: 38, height: 38, flex: '0 0 auto', borderRadius: '9px', bgcolor: '#1764ff', color: '#fff', display: 'flex', alignItems: 'center', justifyContent: 'center' }}><ShieldOutlined fontSize="small" /></Box>
              <Box><Typography sx={{ fontWeight: 700, fontSize: { md: '.8rem', lg: '.88rem' }, color: '#132044' }}>Aman &amp; Terpercaya</Typography><Typography sx={{ mt: .15, fontSize: { md: '.66rem', lg: '.72rem' }, color: '#68768f' }}>Data Anda dienkripsi dan terlindungi dengan standar keamanan tinggi.</Typography></Box>
            </Box>
          </Box>
        </Box>

        <Box className="login-page-right-panel" sx={{ bgcolor: '#fff', display: { xs: 'contents', md: 'flex' }, alignItems: 'center', justifyContent: 'center', px: { md: 4, lg: 6, xl: 8 }, py: { md: 5, lg: 6 } }}>
          <Box className="login-page-form-wrapper" sx={{ width: '100%', maxWidth: { xs: 390, md: 500 }, mx: 'auto' }}>
            {formContent}
          </Box>
        </Box>
      </Box>
    </Box>
  );
}
