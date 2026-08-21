import { useMemo, useState } from 'react';
import {
  Alert, Box, Button, Card, CardContent, Checkbox, Chip, CircularProgress,
  Dialog, DialogActions, DialogContent, DialogTitle, Divider, FormControl,
  FormControlLabel, InputLabel, ListItemText, MenuItem, Pagination, Paper, Select,
  Stack, Switch, TextField, Typography,
} from '@mui/material';
import AddIcon from '@mui/icons-material/Add';
import EditIcon from '@mui/icons-material/EditOutlined';
import DeleteIcon from '@mui/icons-material/PersonOffOutlined';
import ManageAccountsIcon from '@mui/icons-material/ManageAccountsOutlined';
import { useAgents, useCSActivity, useDeleteTeamUser, useSaveTeamUser, useTeamUsers } from '../hooks';
import type { CSActivity, TeamUser } from '../types';
import { swalConfirm, swalToast } from '../services/swal';
import PageHeader from './PageHeader';

type TeamForm = {
  id?: number;
  name: string;
  username: string;
  password: string;
  active: boolean;
  agent_ids: number[];
};

const EMPTY_FORM: TeamForm = {
  name: '',
  username: '',
  password: '',
  active: true,
  agent_ids: [],
};

const activityLabel = (action: string) => {
  if (action === 'read') return 'Membaca chat';
  if (action === 'reply') return 'Membalas pesan';
  if (action === 'reply_media') return 'Mengirim media';
  return action;
};

function ActivityRow({ item }: { item: CSActivity }) {
  return (
    <Stack direction={{ xs: 'column', sm: 'row' }} sx={{ gap: 0.75, py: 1, alignItems: { sm: 'center' } }}>
      <Box sx={{ minWidth: { sm: 150 } }}>
        <Typography sx={{ fontSize: 13.5, fontWeight: 700 }}>{item.user_name || `User ${item.user_id}`}</Typography>
        <Typography variant="caption" color="text.secondary">
          {new Date(item.created_at).toLocaleString('id-ID', {
            day: '2-digit', month: 'short', year: 'numeric', hour: '2-digit', minute: '2-digit',
          })}
        </Typography>
      </Box>
      <Chip size="small" variant="outlined" label={activityLabel(item.action)} sx={{ width: 'fit-content' }} />
      <Box sx={{ flex: 1, minWidth: 0 }}>
        <Typography sx={{ fontSize: 13 }}>{item.agent_name || `CS ${item.agent_id}`} · +{item.sender}</Typography>
        {item.detail && (
          <Typography noWrap variant="caption" color="text.secondary" sx={{ display: 'block' }}>
            {item.detail}
          </Typography>
        )}
      </Box>
    </Stack>
  );
}

const ACTIVITY_PAGE_SIZE = 20;

export default function TeamPanel() {
  const { data: users = [], isLoading: usersLoading } = useTeamUsers();
  const { data: agents = [] } = useAgents();
  const [activityPage, setActivityPage] = useState(1);
  const { data: activityResp, isLoading: activityLoading, isFetching: activityFetching } = useCSActivity(activityPage, ACTIVITY_PAGE_SIZE);
  const activity = activityResp?.data || [];
  const activityTotal = activityResp?.total ?? 0;
  const activityTotalPages = Math.max(1, Math.ceil(activityTotal / (activityResp?.limit || ACTIVITY_PAGE_SIZE)));
  const saveUser = useSaveTeamUser();
  const deleteUser = useDeleteTeamUser();
  const [dialogOpen, setDialogOpen] = useState(false);
  const [form, setForm] = useState<TeamForm>(EMPTY_FORM);
  const [error, setError] = useState('');

  const agentNames = useMemo(() => new Map(agents.map(agent => [agent.id, agent.name || `CS ${agent.id}`])), [agents]);

  const openCreate = () => {
    setForm({ ...EMPTY_FORM, agent_ids: agents.length === 1 ? [agents[0].id] : [] });
    setError('');
    setDialogOpen(true);
  };

  const openEdit = (user: TeamUser) => {
    setForm({
      id: user.id,
      name: user.name,
      username: user.username,
      password: '',
      active: user.active,
      agent_ids: user.agent_ids || [],
    });
    setError('');
    setDialogOpen(true);
  };

  const submit = async () => {
    setError('');
    if (!form.name.trim() || !form.username.trim()) {
      setError('Nama dan username wajib diisi.');
      return;
    }
    if (!form.id && form.password.length < 8) {
      setError('Password minimal 8 karakter.');
      return;
    }
    if (form.agent_ids.length === 0) {
      setError('Pilih minimal satu nomor WhatsApp.');
      return;
    }
    try {
      await saveUser.mutateAsync({
        id: form.id,
        name: form.name.trim(),
        username: form.username.trim(),
        password: form.password || undefined,
        active: form.active,
        agent_ids: form.agent_ids,
      });
      setDialogOpen(false);
      swalToast(form.id ? 'Akun CS diperbarui.' : 'Akun CS berhasil dibuat.');
    } catch (requestError: unknown) {
      setError((requestError as { response?: { data?: { error?: string } } })?.response?.data?.error || 'Akun CS belum dapat disimpan.');
    }
  };

  const deactivate = async (user: TeamUser) => {
    const confirmed = await swalConfirm(
      `Nonaktifkan akun ${user.name || user.username}?`,
      'Akun langsung tidak dapat login dan assignment nomor WhatsApp akan dilepas. Riwayat audit tetap disimpan.',
    );
    if (!confirmed) return;
    try {
      await deleteUser.mutateAsync(user.id);
      swalToast('Akun CS dinonaktifkan.');
    } catch {
      swalToast('Akun CS belum dapat dinonaktifkan.', 'error');
    }
  };

  return (
    <Box>
      <PageHeader
        title="Pengguna CS"
        subtitle="Buat login terpisah dan tentukan nomor WhatsApp yang boleh dipantau setiap CS."
        action={(
          <Button variant="contained" startIcon={<AddIcon />} onClick={openCreate}>
            Tambah Akun CS
          </Button>
        )}
      />

      <Alert severity="info" icon={<ManageAccountsIcon />} sx={{ mb: 1.5 }}>
        Akun CS hanya mendapat akses Inbox operasional pada nomor yang ditugaskan. Pengaturan sistem, koneksi WhatsApp, AI, dan kampanye tetap khusus admin.
      </Alert>

      <Stack spacing={1}>
        {usersLoading ? (
          <Stack direction="row" spacing={1} sx={{ alignItems: 'center', py: 2 }}>
            <CircularProgress size={18} /><Typography variant="body2">Memuat pengguna…</Typography>
          </Stack>
        ) : users.length === 0 ? (
          <Paper variant="outlined" sx={{ p: 3, textAlign: 'center' }}>
            <Typography sx={{ fontWeight: 700 }}>Belum ada akun CS</Typography>
            <Typography variant="body2" color="text.secondary">Tambahkan akun agar staf dapat login memakai username sendiri.</Typography>
          </Paper>
        ) : users.map(user => (
          <Card key={user.id} variant="outlined">
            <CardContent sx={{ py: '12px !important' }}>
              <Stack direction={{ xs: 'column', sm: 'row' }} sx={{ gap: 1, alignItems: { sm: 'center' } }}>
                <Box sx={{ flex: 1 }}>
                  <Stack direction="row" spacing={0.75} sx={{ alignItems: 'center', flexWrap: 'wrap' }}>
                    <Typography sx={{ fontWeight: 700 }}>{user.name || user.username}</Typography>
                    <Chip size="small" label={user.role === 'owner' ? 'Owner' : 'CS'} color={user.role === 'owner' ? 'primary' : 'default'} />
                    <Chip size="small" label={user.active ? 'Aktif' : 'Nonaktif'} color={user.active ? 'success' : 'default'} variant="outlined" />
                  </Stack>
                  <Typography variant="caption" color="text.secondary">@{user.username}</Typography>
                  <Stack direction="row" sx={{ gap: 0.5, mt: 0.75, flexWrap: 'wrap' }}>
                    {(user.agent_ids || []).map(agentID => (
                      <Chip key={agentID} size="small" variant="outlined" label={agentNames.get(agentID) || `Nomor ${agentID}`} />
                    ))}
                    {user.role === 'owner' && <Chip size="small" variant="outlined" label="Semua nomor" />}
                  </Stack>
                </Box>
                {user.role === 'cs' && (
                  <Stack direction="row" spacing={0.5}>
                    <Button size="small" startIcon={<EditIcon />} onClick={() => openEdit(user)}>Edit</Button>
                    <Button size="small" color="error" startIcon={<DeleteIcon />} onClick={() => void deactivate(user)} disabled={!user.active || deleteUser.isPending}>
                      Nonaktifkan
                    </Button>
                  </Stack>
                )}
              </Stack>
            </CardContent>
          </Card>
        ))}
      </Stack>

      <Paper variant="outlined" sx={{ mt: 2, p: 1.5 }}>
        <Stack direction={{ xs: 'column', sm: 'row' }} sx={{ alignItems: { sm: 'center' }, justifyContent: 'space-between', gap: 0.5 }}>
          <Box>
            <Typography sx={{ fontWeight: 700 }}>Aktivitas CS terbaru</Typography>
            <Typography variant="caption" color="text.secondary">
              Audit saat percakapan dibuka dan saat pesan dibalas dari dashboard.
            </Typography>
          </Box>
          {activityTotal > 0 && (
            <Typography variant="caption" color="text.secondary" sx={{ whiteSpace: 'nowrap' }}>
              {activityTotal} aktivitas
              {activityFetching && !activityLoading ? ' · memperbarui…' : ''}
            </Typography>
          )}
        </Stack>
        <Divider sx={{ mt: 1 }} />
        {activityLoading && activity.length === 0 ? (
          <CircularProgress size={18} sx={{ mt: 2 }} />
        ) : activity.length === 0 ? (
          <Typography variant="body2" color="text.secondary" sx={{ py: 2 }}>Belum ada aktivitas CS.</Typography>
        ) : (
          <>
            {activity.map((item, index) => (
              <Box key={item.id}>
                <ActivityRow item={item} />
                {index < activity.length - 1 && <Divider />}
              </Box>
            ))}
            {activityTotalPages > 1 && (
              <Stack
                direction={{ xs: 'column', sm: 'row' }}
                sx={{ mt: 1.5, pt: 1, borderTop: 1, borderColor: 'divider', alignItems: { sm: 'center' }, justifyContent: 'space-between', gap: 1 }}
              >
                <Typography variant="caption" color="text.secondary">
                  Halaman {activityPage} dari {activityTotalPages}
                  {' · '}
                  menampilkan {(activityPage - 1) * ACTIVITY_PAGE_SIZE + 1}
                  –
                  {Math.min(activityPage * ACTIVITY_PAGE_SIZE, activityTotal)}
                </Typography>
                <Pagination
                  count={activityTotalPages}
                  page={activityPage}
                  onChange={(_e, p) => setActivityPage(p)}
                  size="small"
                  color="primary"
                  siblingCount={0}
                  boundaryCount={1}
                  disabled={activityFetching}
                />
              </Stack>
            )}
          </>
        )}
      </Paper>

      <Dialog open={dialogOpen} onClose={() => !saveUser.isPending && setDialogOpen(false)} maxWidth="sm" fullWidth>
        <DialogTitle>{form.id ? 'Edit Akun CS' : 'Tambah Akun CS'}</DialogTitle>
        <DialogContent>
          <Stack spacing={1.5} sx={{ mt: 0.5 }}>
            {error && <Alert severity="error">{error}</Alert>}
            <TextField label="Nama CS" size="small" value={form.name} onChange={event => setForm(current => ({ ...current, name: event.target.value }))} autoFocus />
            <TextField label="Username login" size="small" value={form.username} onChange={event => setForm(current => ({ ...current, username: event.target.value }))} />
            <TextField
              label={form.id ? 'Password baru (opsional)' : 'Password'}
              type="password"
              size="small"
              value={form.password}
              onChange={event => setForm(current => ({ ...current, password: event.target.value }))}
              helperText={form.id ? 'Kosongkan jika password tidak diubah.' : 'Minimal 8 karakter.'}
            />
            <FormControl size="small" fullWidth>
              <InputLabel id="team-agent-label">Nomor yang dapat diakses</InputLabel>
              <Select
                labelId="team-agent-label"
                multiple
                value={form.agent_ids}
                label="Nomor yang dapat diakses"
                onChange={event => setForm(current => ({ ...current, agent_ids: event.target.value as number[] }))}
                renderValue={selected => selected.map(id => agentNames.get(id) || `Nomor ${id}`).join(', ')}
              >
                {agents.map(agent => (
                  <MenuItem key={agent.id} value={agent.id}>
                    <Checkbox checked={form.agent_ids.includes(agent.id)} />
                    <ListItemText primary={agent.name || `CS ${agent.id}`} secondary={agent.number ? `+${agent.number}` : 'Belum tertaut'} />
                  </MenuItem>
                ))}
              </Select>
            </FormControl>
            {form.id && (
              <FormControlLabel
                control={<Switch checked={form.active} onChange={event => setForm(current => ({ ...current, active: event.target.checked }))} />}
                label="Akun aktif"
              />
            )}
          </Stack>
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setDialogOpen(false)} disabled={saveUser.isPending}>Batal</Button>
          <Button variant="contained" onClick={() => void submit()} disabled={saveUser.isPending}>
            {saveUser.isPending ? <CircularProgress size={18} /> : 'Simpan'}
          </Button>
        </DialogActions>
      </Dialog>
    </Box>
  );
}
