import { useMemo, useState, type MouseEvent } from 'react';
import {
  Box, Card, CardContent, Typography, Button, Stack, Chip, Switch, IconButton,
  Dialog, DialogTitle, DialogContent, DialogActions, TextField, Select, MenuItem,
  FormControl, InputLabel, FormControlLabel, CircularProgress, Alert, Paper, useMediaQuery,
  ToggleButton, ToggleButtonGroup, Tooltip, Menu, Divider,
} from '@mui/material';
import { useTheme } from '@mui/material/styles';
import AddIcon from '@mui/icons-material/Add';
import EditIcon from '@mui/icons-material/Edit';
import DeleteIcon from '@mui/icons-material/Delete';
import ScheduleSendIcon from '@mui/icons-material/ScheduleSendOutlined';
import PersonAddIcon from '@mui/icons-material/PersonAddAlt1Outlined';
import AutoAwesomeIcon from '@mui/icons-material/AutoAwesome';
import AccessTimeIcon from '@mui/icons-material/AccessTimeOutlined';
import MoreVertIcon from '@mui/icons-material/MoreVert';
import ExpandMoreIcon from '@mui/icons-material/ExpandMore';
import {
  useFollowUps, useSaveFollowUp, useDeleteFollowUp, useEnrollFollowUp,
  useCrmContacts, useCrmContactsExport,
} from '../hooks';
import type { FollowUp, LeadStage } from '../types';
import { normalizePhone } from '../types';
import { swalConfirm, swalAlert } from '../services/swal';
import PageHeader from './PageHeader';
import EmptyState from './common/EmptyState';
import WhatsAppEditor from './WhatsAppEditor';
import TemplatePicker from './TemplatePicker';
import RecipientField from './RecipientField';

type StepForm = { delay_value: number; delay_unit: 'hari' | 'jam'; message: string; ai_generated: boolean; ai_instruction: string };

const CRM_STAGES: { value: LeadStage; label: string }[] = [
  { value: 'new', label: 'Baru' }, { value: 'cold', label: 'Cold' },
  { value: 'warm', label: 'Warm' }, { value: 'hot', label: 'Hot' },
  { value: 'customer', label: 'Pelanggan' }, { value: 'unqualified', label: 'Tidak potensial' },
];

// jam tersimpan -> {nilai, satuan} untuk editor.
function hoursToParts(h: number): { delay_value: number; delay_unit: 'hari' | 'jam' } {
  if (h > 0 && h % 24 === 0) return { delay_value: h / 24, delay_unit: 'hari' };
  return { delay_value: h, delay_unit: 'jam' };
}
function partsToHours(s: StepForm): number {
  const v = Math.max(0, Math.floor(s.delay_value || 0));
  return s.delay_unit === 'hari' ? v * 24 : v;
}
function stepBadge(h: number): string {
  if (h === 0) return 'Segera setelah dimulai';
  if (h % 24 === 0) return `${h / 24} hari setelah dimulai`;
  return `${h} jam setelah dimulai`;
}

function followUpTiming(followUp: FollowUp): string {
  if (!followUp.enabled) return 'Follow-up dijeda — pesan tidak dikirim';
  if (!followUp.counts.active) return 'Belum ada kontak aktif';
  if ((followUp.counts.due || 0) > 0) return `${followUp.counts.due} pesan jatuh tempo dan menunggu diproses`;
  if (!followUp.next_send_at) return 'Menunggu jadwal berikutnya';
  const next = new Date(followUp.next_send_at);
  if (Number.isNaN(next.getTime())) return 'Menunggu jadwal berikutnya';
  return `Pengiriman berikutnya ${next.toLocaleString('id-ID', {
    day: 'numeric', month: 'short', hour: '2-digit', minute: '2-digit',
  })}`;
}

function requestError(error: unknown, fallback: string): string {
  return (error as { response?: { data?: { error?: string } } })?.response?.data?.error || fallback;
}

const NEW_STEP: StepForm = { delay_value: 1, delay_unit: 'hari', message: '', ai_generated: false, ai_instruction: '' };

export default function FollowUpPanel({ agentId }: { agentId: number }) {
  const theme = useTheme();
  const mobileDialog = useMediaQuery(theme.breakpoints.down('sm'));
  const { data: flows, isLoading } = useFollowUps(agentId);
  const save = useSaveFollowUp(agentId);
  const del = useDeleteFollowUp(agentId);
  const enroll = useEnrollFollowUp(agentId);
  const exportContacts = useCrmContactsExport(agentId);
  const { data: crm } = useCrmContacts(agentId, '', '', '', 1);
  const allTags = crm?.all_tags || [];
  const [expandedId, setExpandedId] = useState<number | null>(null);
  const [menuAnchor, setMenuAnchor] = useState<HTMLElement | null>(null);
  const [menuFollowUp, setMenuFollowUp] = useState<FollowUp | null>(null);

  // ---- form urutan ----
  const [open, setOpen] = useState(false);
  const [editId, setEditId] = useState<number | null>(null);
  const [name, setName] = useState('');
  const [stopOnReply, setStopOnReply] = useState(true);
  const [steps, setSteps] = useState<StepForm[]>([{ ...NEW_STEP, delay_value: 0, delay_unit: 'jam' }]);

  const openNew = () => {
    setEditId(null); setName(''); setStopOnReply(true);
    setSteps([{ delay_value: 0, delay_unit: 'jam', message: '', ai_generated: false, ai_instruction: '' }]);
    setOpen(true);
  };
  const openEdit = (fu: FollowUp) => {
    setEditId(fu.id); setName(fu.name); setStopOnReply(fu.stop_on_reply);
    setSteps(fu.steps.length ? fu.steps.map(s => ({ ...hoursToParts(s.delay_hours), message: s.message, ai_generated: s.ai_generated || false, ai_instruction: s.ai_instruction || '' })) : [{ delay_value: 0, delay_unit: 'jam', message: '', ai_generated: false, ai_instruction: '' }]);
    setOpen(true);
  };

  const setStep = (i: number, patch: Partial<StepForm>) => setSteps(steps.map((s, j) => j === i ? { ...s, ...patch } : s));
  const addStep = () => {
    const previousHours = steps.length ? partsToHours(steps[steps.length - 1]) : 0;
    const next = hoursToParts(previousHours + 24);
    setSteps([...steps, { ...next, message: '', ai_generated: false, ai_instruction: '' }]);
  };
  const removeStep = (i: number) => setSteps(steps.filter((_, j) => j !== i));

  const submit = async () => {
    if (!name.trim()) { await swalAlert('Nama Follow-up wajib diisi.', 'warning'); return; }
    const payloadSteps = steps.filter(s => s.message.trim()).map(s => ({ delay_hours: partsToHours(s), message: s.message, ai_generated: s.ai_generated, ai_instruction: s.ai_generated ? s.ai_instruction : '' }));
    if (payloadSteps.length === 0) { await swalAlert('Minimal satu langkah dengan pesan.', 'warning'); return; }
    const invalidOrder = payloadSteps.findIndex((step, index) => index > 0 && step.delay_hours < payloadSteps[index - 1].delay_hours);
    if (invalidOrder >= 0) { await swalAlert(`Waktu langkah ${invalidOrder + 1} tidak boleh lebih awal dari langkah sebelumnya.`, 'warning'); return; }
    try {
      await save.mutateAsync({ id: editId ?? undefined, name: name.trim(), stop_on_reply: stopOnReply, steps: payloadSteps } as Partial<FollowUp>);
      setOpen(false);
      await swalAlert(editId ? 'Follow-up diperbarui.' : 'Follow-up berhasil dibuat.', 'success');
    } catch (error) {
      await swalAlert(requestError(error, 'Follow-up belum bisa disimpan.'), 'error');
    }
  };

  const toggle = async (fu: FollowUp) => {
    try {
      await save.mutateAsync({ id: fu.id, enabled: !fu.enabled } as Partial<FollowUp>);
    } catch (error) {
      await swalAlert(requestError(error, 'Status Follow-up belum bisa diubah.'), 'error');
    }
  };
  const remove = async (fu: FollowUp) => {
    if (!await swalConfirm(`Hapus Follow-up "${fu.name}"?`, 'Kontak yang sedang mengikuti Follow-up ini juga akan dihapus.')) return;
    try {
      await del.mutateAsync(fu.id);
    } catch (error) {
      await swalAlert(requestError(error, 'Follow-up belum bisa dihapus.'), 'error');
    }
  };

  // ---- dialog daftarkan kontak ----
  const [enrollFu, setEnrollFu] = useState<FollowUp | null>(null);
  const [recipients, setRecipients] = useState('');
  const [enrollTag, setEnrollTag] = useState('');
  const [enrollStage, setEnrollStage] = useState<LeadStage | ''>('');
  const recipientSummary = useMemo(() => {
    const lines = recipients.split('\n').map(line => line.trim()).filter(Boolean);
    const unique = new Set<string>();
    let valid = 0;
    let invalid = 0;
    let duplicate = 0;
    for (const line of lines) {
      const number = normalizePhone(line.split(',')[0]);
      if (!number) {
        invalid++;
      } else if (unique.has(number)) {
        duplicate++;
      } else {
        unique.add(number);
        valid++;
      }
    }
    return { total: lines.length, valid, invalid, duplicate };
  }, [recipients]);

  const openEnroll = (fu: FollowUp) => { setEnrollFu(fu); setRecipients(''); setEnrollTag(''); setEnrollStage(''); };

  const openActions = (event: MouseEvent<HTMLElement>, followUp: FollowUp) => {
    setMenuAnchor(event.currentTarget);
    setMenuFollowUp(followUp);
  };
  const closeActions = () => {
    setMenuAnchor(null);
    setMenuFollowUp(null);
  };

  const appendCrmContacts = (list: { number: string; name: string }[]) => {
    const lines = list.map(c => (c.name ? `${c.number},${c.name}` : c.number));
    setRecipients(prev => {
      const have = new Set(prev.split('\n').map(l => normalizePhone(l.split(',')[0])).filter(Boolean));
      const fresh = lines.filter(l => !have.has(normalizePhone(l.split(',')[0])));
      return [prev.trim(), ...fresh].filter(Boolean).join('\n');
    });
  };

  const fillFromTag = async (tag: string) => {
    setEnrollTag(tag);
    if (!tag) return;
    try {
      const list = await exportContacts.mutateAsync({ q: '', tag, stage: '' });
      appendCrmContacts(list);
    } catch { await swalAlert('Gagal mengambil kontak tag.', 'error'); }
  };

  const fillFromStage = async (stage: LeadStage | '') => {
    setEnrollStage(stage);
    if (!stage) return;
    try {
      appendCrmContacts(await exportContacts.mutateAsync({ q: '', tag: '', stage }));
    } catch { await swalAlert('Gagal mengambil kontak dari status CRM.', 'error'); }
  };

  const doEnroll = async () => {
    if (!enrollFu) return;
    const parsed = recipients.split('\n').map(l => l.trim()).filter(Boolean).map(line => {
      const [num, ...rest] = line.split(',');
      return { number: normalizePhone(num), name: rest.join(',').trim() };
    }).filter(r => r.number);
    if (parsed.length === 0) { await swalAlert('Masukkan minimal satu nomor.', 'warning'); return; }
    try {
      const res = await enroll.mutateAsync({ fid: enrollFu.id, recipients: parsed });
      setEnrollFu(null);
      const details = res.details;
      const skippedDetails = details ? [
        details.already_active ? `${details.already_active} sudah aktif` : '',
        details.opted_out ? `${details.opted_out} memilih STOP` : '',
        details.duplicate ? `${details.duplicate} duplikat` : '',
        details.invalid ? `${details.invalid} tidak valid` : '',
        details.failed ? `${details.failed} gagal disimpan` : '',
      ].filter(Boolean).join(', ') : '';
      await swalAlert(
        `${res.added} kontak mulai mengikuti Follow-up${res.skipped ? `. ${res.skipped} dilewati${skippedDetails ? `: ${skippedDetails}` : ''}` : ''}.`,
        res.added > 0 ? 'success' : 'warning',
      );
    } catch (error) {
      await swalAlert(requestError(error, 'Kontak belum bisa ditambahkan ke Follow-up.'), 'error');
    }
  };

  if (isLoading) return <Box sx={{ display: 'flex', justifyContent: 'center', mt: 8 }}><CircularProgress /></Box>;

  return (
    <Box>
      <PageHeader title="Follow-up Otomatis"
        subtitle="Kirim pesan lanjutan secara bertahap. Follow-up berhenti otomatis ketika pelanggan membalas atau memilih STOP."
        action={<Button variant="contained" startIcon={<AddIcon />} onClick={openNew}>Buat Follow-up</Button>} />

      {(!flows || flows.length === 0) ? (
        <EmptyState
          icon={<ScheduleSendIcon sx={{ fontSize: 48 }} />}
          title="Belum ada Follow-up"
          description="Buat rangkaian pesan untuk menindaklanjuti pelanggan secara otomatis, misalnya ucapan terima kasih lalu pengingat beberapa hari kemudian."
          actionLabel="Buat Follow-up"
          onAction={openNew}
        />
      ) : (
        <Stack spacing={1.5}>
          {flows.map(fu => (
            <Card
              key={fu.id}
              sx={{
                opacity: fu.enabled ? 1 : 0.72,
                borderColor: (fu.counts.due || 0) > 0 ? 'warning.main' : 'divider',
              }}
            >
              <CardContent>
                <Stack direction={{ xs: 'column', md: 'row' }} sx={{ justifyContent: 'space-between', alignItems: { xs: 'stretch', md: 'flex-start' }, gap: 1.5 }}>
                  <Box sx={{ minWidth: 0, flex: 1 }}>
                    <Stack direction="row" spacing={1} sx={{ alignItems: 'center', flexWrap: 'wrap', gap: 0.75 }}>
                      <Typography sx={{ fontWeight: 700, fontSize: 15 }}>{fu.name}</Typography>
                      <Chip
                        size="small"
                        label={fu.enabled ? 'Aktif' : 'Dijeda'}
                        color={fu.enabled ? 'success' : 'warning'}
                        variant="outlined"
                      />
                    </Stack>

                    <Stack direction="row" sx={{ flexWrap: 'wrap', gap: 0.6, mt: 1, alignItems: 'center' }}>
                      <Chip size="small" label={`${fu.counts.active} sedang berjalan`} color={fu.counts.active ? 'success' : 'default'} variant="outlined" />
                      {(fu.counts.due || 0) > 0 && <Chip size="small" label={`${fu.counts.due} menunggu dikirim`} color="warning" variant="outlined" />}
                      <Chip size="small" label={`${fu.counts.completed} selesai`} variant="outlined" />
                      <Chip size="small" label={`${fu.counts.stopped} dihentikan`} variant="outlined" />
                    </Stack>

                    <Stack direction="row" spacing={0.75} sx={{ alignItems: 'center', mt: 1 }}>
                      <AccessTimeIcon sx={{ fontSize: 16, color: fu.enabled ? 'text.secondary' : 'warning.main' }} />
                      <Typography variant="caption" color={fu.enabled ? 'text.secondary' : 'warning.main'} sx={{ fontWeight: 600 }}>
                        {followUpTiming(fu)}
                      </Typography>
                    </Stack>
                  </Box>

                  <Box
                    sx={{
                      display: 'grid',
                      gridTemplateColumns: { xs: '1fr auto', sm: 'auto auto auto' },
                      gridTemplateAreas: {
                        xs: '"status menu" "button button"',
                        sm: '"status button menu"',
                      },
                      alignItems: 'center',
                      justifyContent: { xs: 'stretch', sm: 'end' },
                      gap: 0.75,
                      width: { xs: '100%', md: 'auto' },
                      flexShrink: 0,
                    }}
                  >
                    <FormControlLabel
                      sx={{ m: 0, gridArea: 'status', justifySelf: 'start' }}
                      control={<Switch checked={fu.enabled} onChange={() => { void toggle(fu); }} size="small" disabled={save.isPending} />}
                      label={<Typography variant="caption" sx={{ fontWeight: 700 }}>{fu.enabled ? 'Aktif' : 'Dijeda'}</Typography>}
                    />
                    <Tooltip title={fu.enabled ? 'Tambahkan kontak ke Follow-up' : 'Aktifkan Follow-up sebelum menambahkan kontak'}>
                      <Box component="span" sx={{ gridArea: 'button', width: { xs: '100%', sm: 'auto' } }}>
                        <Button
                          fullWidth
                          size="small"
                          variant="contained"
                          startIcon={<PersonAddIcon />}
                          disabled={!fu.enabled}
                          onClick={() => openEnroll(fu)}
                          sx={{ minWidth: { sm: 152 } }}
                        >
                          Tambah Kontak
                        </Button>
                      </Box>
                    </Tooltip>
                    <Tooltip title="Tindakan lainnya">
                      <IconButton
                        size="small"
                        onClick={event => openActions(event, fu)}
                        aria-label={`Tindakan untuk ${fu.name}`}
                        sx={{ gridArea: 'menu', justifySelf: 'end' }}
                      >
                        <MoreVertIcon />
                      </IconButton>
                    </Tooltip>
                  </Box>
                </Stack>

                <Divider sx={{ my: 1.5 }} />

                <Stack direction={{ xs: 'column', sm: 'row' }} spacing={1} sx={{ alignItems: { xs: 'stretch', sm: 'center' }, justifyContent: 'space-between' }}>
                  <Box>
                    <Typography variant="caption" color="text.secondary" sx={{ display: 'block', fontWeight: 700 }}>
                      {fu.steps.length} langkah pesan
                    </Typography>
                    <Typography variant="caption" color="text.secondary">
                      {fu.stop_on_reply ? 'Berhenti otomatis saat pelanggan membalas.' : 'Tetap berjalan meskipun pelanggan membalas.'}
                    </Typography>
                  </Box>
                  <Button
                    size="small"
                    variant="text"
                    endIcon={<ExpandMoreIcon sx={{ transform: expandedId === fu.id ? 'rotate(180deg)' : 'none', transition: 'transform 160ms ease' }} />}
                    onClick={() => setExpandedId(expandedId === fu.id ? null : fu.id)}
                    sx={{ alignSelf: { xs: 'flex-start', sm: 'center' } }}
                  >
                    {expandedId === fu.id ? 'Tutup detail' : 'Lihat alur pesan'}
                  </Button>
                </Stack>

                {expandedId === fu.id && (
                  <Stack spacing={0} sx={{ mt: 1.5, border: '1px solid', borderColor: 'divider', borderRadius: 1.25, overflow: 'hidden' }}>
                    {fu.steps.map((step, index) => {
                      const content = step.ai_generated ? (step.ai_instruction || step.message) : step.message;
                      return (
                        <Stack
                          key={step.id || index}
                          direction="row"
                          spacing={1.25}
                          sx={{
                            p: 1.25,
                            alignItems: 'flex-start',
                            bgcolor: index % 2 === 0 ? 'background.default' : 'background.paper',
                            borderTop: index > 0 ? '1px solid' : 0,
                            borderColor: 'divider',
                          }}
                        >
                          <Box sx={{ width: 26, height: 26, borderRadius: '50%', bgcolor: 'primary.main', color: 'primary.contrastText', display: 'grid', placeItems: 'center', fontSize: 12, fontWeight: 700, flexShrink: 0 }}>
                            {index + 1}
                          </Box>
                          <Box sx={{ minWidth: 0, flex: 1 }}>
                            <Stack direction="row" spacing={0.75} sx={{ alignItems: 'center', flexWrap: 'wrap', gap: 0.5 }}>
                              <Typography variant="body2" sx={{ fontWeight: 700 }}>{stepBadge(step.delay_hours)}</Typography>
                              <Chip size="small" label={step.ai_generated ? 'Ditulis AI' : 'Pesan tetap'} color={step.ai_generated ? 'success' : 'default'} variant="outlined" sx={{ height: 20 }} />
                            </Stack>
                            <Typography variant="caption" color="text.secondary" sx={{ display: 'block', mt: 0.5, whiteSpace: 'pre-wrap', overflowWrap: 'anywhere' }}>
                              {content || 'Belum ada isi pesan'}
                            </Typography>
                          </Box>
                        </Stack>
                      );
                    })}
                  </Stack>
                )}
              </CardContent>
            </Card>
          ))}
        </Stack>
      )}

      <Menu anchorEl={menuAnchor} open={!!menuAnchor} onClose={closeActions}>
        <MenuItem onClick={() => {
          const followUp = menuFollowUp;
          closeActions();
          if (followUp) openEdit(followUp);
        }}>
          <EditIcon fontSize="small" sx={{ mr: 1 }} /> Edit Follow-up
        </MenuItem>
        <MenuItem sx={{ color: 'error.main' }} onClick={() => {
          const followUp = menuFollowUp;
          closeActions();
          if (followUp) void remove(followUp);
        }}>
          <DeleteIcon fontSize="small" sx={{ mr: 1 }} /> Hapus Follow-up
        </MenuItem>
      </Menu>

      {/* Buat / edit urutan */}
      <Dialog open={open} onClose={() => setOpen(false)} fullWidth maxWidth="md" fullScreen={mobileDialog}>
        <DialogTitle>{editId ? 'Edit Follow-up' : 'Buat Follow-up'}</DialogTitle>
        <DialogContent dividers>
          <Stack spacing={1.5}>
            <Alert severity="info" icon={false}>
              Waktu setiap langkah dihitung sejak kontak mulai mengikuti Follow-up ini. Contoh: “2 hari” berarti dua hari setelah kontak ditambahkan.
            </Alert>
            {!!editId && (flows?.find(flow => flow.id === editId)?.counts.active || 0) > 0 && (
              <Alert severity="warning">
                Follow-up ini sedang berjalan untuk {flows?.find(flow => flow.id === editId)?.counts.active} kontak. Perubahan isi dan waktu berlaku pada langkah yang belum terkirim.
              </Alert>
            )}

            <Paper variant="outlined" sx={{ p: 1.25 }}>
              <Typography variant="subtitle2" sx={{ fontWeight: 800, mb: 1 }}>Informasi Follow-up</Typography>
              <TextField fullWidth label="Nama Follow-up" value={name} onChange={e => setName(e.target.value)} size="small"
                placeholder="Contoh: Pembeli baru" helperText="Gunakan nama yang mudah dikenali. Nama ini hanya terlihat di dashboard." />
              <FormControlLabel sx={{ mt: 0.75, alignItems: 'flex-start' }}
                control={<Switch checked={stopOnReply} onChange={e => setStopOnReply(e.target.checked)} />}
                label={
                  <Box sx={{ pt: 0.65 }}>
                    <Typography variant="body2" sx={{ fontWeight: 700 }}>Hentikan saat kontak membalas</Typography>
                    <Typography variant="caption" color="text.secondary">Pesan berikutnya tidak dikirim setelah kontak membalas.</Typography>
                  </Box>
                } />
            </Paper>

            <Box>
              <Typography variant="subtitle2" sx={{ fontWeight: 800 }}>Alur Pesan</Typography>
              <Typography variant="caption" color="text.secondary">Susun kapan pesan dikirim dan pilih apakah isinya tetap atau dibuat personal oleh AI.</Typography>
            </Box>
            {steps.map((s, i) => (
              <Paper key={i} variant="outlined" sx={{ p: 1.25 }}>
                <Stack direction="row" sx={{ alignItems: 'center', justifyContent: 'space-between', gap: 1, mb: 1 }}>
                  <Stack direction="row" sx={{ alignItems: 'center', gap: 0.75 }}>
                    <Chip size="small" color="primary" label={`Langkah ${i + 1}`} />
                    <Typography variant="caption" color="text.secondary">{stepBadge(partsToHours(s))}</Typography>
                  </Stack>
                  {steps.length > 1 && <IconButton aria-label={`Hapus langkah ${i + 1}`} size="small" color="error" onClick={() => removeStep(i)}><DeleteIcon fontSize="small" /></IconButton>}
                </Stack>

                <Typography variant="body2" sx={{ fontWeight: 700, mb: 0.75 }}>Waktu kirim</Typography>
                <Stack direction="row" spacing={1} sx={{ alignItems: 'flex-start', mb: 0.5 }}>
                  <TextField type="number" size="small" label="Jeda" value={s.delay_value}
                    onChange={e => setStep(i, { delay_value: Math.max(0, Number(e.target.value) || 0) })} sx={{ width: 110 }}
                    slotProps={{ htmlInput: { min: 0 } }} />
                  <FormControl size="small" sx={{ width: 120 }}>
                    <InputLabel>Satuan</InputLabel>
                    <Select label="Satuan" value={s.delay_unit} onChange={e => setStep(i, { delay_unit: e.target.value as 'hari' | 'jam' })}>
                      <MenuItem value="jam">jam</MenuItem>
                      <MenuItem value="hari">hari</MenuItem>
                    </Select>
                  </FormControl>
                </Stack>
                <Typography variant="caption" color="text.secondary" sx={{ display: 'block', mb: 1.25 }}>
                  {partsToHours(s) === 0
                    ? 'Pesan pertama masuk antrean segera setelah kontak ditambahkan.'
                    : `Pesan dikirim ${s.delay_value} ${s.delay_unit} setelah kontak ditambahkan ke Follow-up.`}
                </Typography>

                <Stack direction={{ xs: 'column', sm: 'row' }} sx={{ alignItems: { xs: 'flex-start', sm: 'center' }, justifyContent: 'space-between', gap: 0.5, mb: 1 }}>
                  <Stack direction="row" spacing={1} sx={{ alignItems: 'center' }}>
                    <Typography variant="body2" sx={{ fontWeight: 700 }}>
                      {s.ai_generated ? 'Arahan untuk AI' : 'Isi pesan'}
                    </Typography>
                    {s.ai_generated && <Chip size="small" icon={<AutoAwesomeIcon />} label="AI" color="success" variant="outlined" sx={{ height: 20, fontSize: '0.6rem' }} />}
                  </Stack>
                  <Stack direction="row" spacing={1} sx={{ alignItems: 'center' }}>
                    <ToggleButtonGroup
                      size="small"
                      value={s.ai_generated ? 'ai' : 'manual'}
                      exclusive
                      onChange={(_, v) => v && setStep(i, { ai_generated: v === 'ai', ai_instruction: v === 'ai' ? (s.ai_instruction || s.message) : '' })}
                    >
                      <ToggleButton value="manual" sx={{ px: 1.5, py: 0.25, fontSize: '0.7rem' }}>Pesan Tetap</ToggleButton>
                      <ToggleButton value="ai" sx={{ px: 1.5, py: 0.25, fontSize: '0.7rem' }}>
                        <AutoAwesomeIcon sx={{ fontSize: '0.85rem', mr: 0.25 }} />Ditulis AI
                      </ToggleButton>
                    </ToggleButtonGroup>
                    {!s.ai_generated && <TemplatePicker label="Template" agentId={agentId} variant="text" onPick={b => setStep(i, { message: s.message ? s.message + '\n' + b : b })} />}
                  </Stack>
                </Stack>

                {s.ai_generated ? (
                  <>
                    <Alert severity="success" icon={<AutoAwesomeIcon />} sx={{ mb: 1, py: 0.25, '& .MuiAlert-message': { py: 0.25 } }}>
                      <Typography variant="caption">
                        AI akan menulis pesan berbeda untuk <b>setiap kontak</b> — disesuaikan dengan nama, riwayat chat terakhir, dan konteks bisnis. Hasil tidak akan sama antar kontak.
                      </Typography>
                    </Alert>
                    <TextField
                      fullWidth
                      multiline
                      rows={3}
                      size="small"
                      label="Arahan untuk AI"
                      value={s.ai_instruction}
                      onChange={e => setStep(i, { ai_instruction: e.target.value, message: e.target.value })}
                      placeholder="Contoh: Tanyakan apakah barang sudah sampai dengan baik, tawarkan promo repeat order diskon 10% untuk pembelian berikutnya. Jangan terlalu memaksa."
                      helperText="Tulis apa yang harus dilakukan AI. AI akan membaca instruksi + riwayat chat kontak untuk menulis pesan yang personal."
                    />
                  </>
                ) : (
                  <WhatsAppEditor value={s.message} onChange={v => setStep(i, { message: v })}
                    placeholder="Halo {nama}, gimana kabarnya? ..." rows={3} />
                )}
              </Paper>
            ))}
            <Button startIcon={<AddIcon />} variant="outlined" onClick={addStep} size="small" sx={{ alignSelf: 'flex-start' }}>Tambah Pesan Berikutnya</Button>
          </Stack>
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setOpen(false)}>Batal</Button>
          <Button variant="contained" onClick={submit} disabled={save.isPending}>
            {save.isPending ? 'Menyimpan...' : 'Simpan Follow-up'}
          </Button>
        </DialogActions>
      </Dialog>

      {/* Tambahkan kontak agar mulai mengikuti urutan */}
      <Dialog open={!!enrollFu} onClose={() => setEnrollFu(null)} fullWidth maxWidth="sm" fullScreen={mobileDialog}>
        <DialogTitle>Mulai Follow-up untuk Kontak</DialogTitle>
        <DialogContent dividers>
          <Stack spacing={1.5}>
            <Alert severity="info" icon={false}>
              Kontak akan mulai mengikuti <b>{enrollFu?.name}</b> setelah Anda menekan tombol Mulai Follow-up.
              {enrollFu?.steps[0]?.delay_hours === 0
                ? ' Pesan pertama akan masuk antrean segera.'
                : enrollFu?.steps[0]
                  ? ` Pesan pertama dijadwalkan ${stepBadge(enrollFu.steps[0].delay_hours).toLowerCase()}.`
                  : ''}
            </Alert>
            {allTags.length > 0 && (
              <FormControl size="small" fullWidth>
                <InputLabel>Tambahkan dari tag</InputLabel>
                <Select label="Tambahkan dari tag" value={enrollTag} onChange={e => fillFromTag(e.target.value)}>
                  <MenuItem value=""><em>— pilih tag —</em></MenuItem>
                  {allTags.map(t => <MenuItem key={t} value={t}>{t}</MenuItem>)}
                </Select>
              </FormControl>
            )}
            <FormControl size="small" fullWidth>
              <InputLabel>Tambahkan dari status CRM</InputLabel>
              <Select label="Tambahkan dari status CRM" value={enrollStage} onChange={e => fillFromStage(e.target.value as LeadStage | '')}>
                <MenuItem value=""><em>— pilih status —</em></MenuItem>
                {CRM_STAGES.map(item => (
                  <MenuItem key={item.value} value={item.value}>
                    {item.label} ({crm?.stage_counts?.[item.value] || 0})
                  </MenuItem>
                ))}
              </Select>
            </FormControl>
            <RecipientField agentId={agentId} value={recipients} onChange={setRecipients} />

            <Paper variant="outlined" sx={{ p: 1.25, bgcolor: 'background.default' }}>
              <Typography variant="body2" sx={{ fontWeight: 700, mb: 1 }}>Ringkasan kontak</Typography>
              <Box sx={{ display: 'grid', gridTemplateColumns: 'repeat(3, minmax(0, 1fr))', gap: 1 }}>
                {[
                  { label: 'Siap dimulai', value: recipientSummary.valid, color: 'success.main' },
                  { label: 'Duplikat', value: recipientSummary.duplicate, color: 'warning.main' },
                  { label: 'Tidak valid', value: recipientSummary.invalid, color: 'error.main' },
                ].map(item => (
                  <Box key={item.label}>
                    <Typography sx={{ fontSize: 18, fontWeight: 700, color: item.color, lineHeight: 1.2 }}>{item.value}</Typography>
                    <Typography variant="caption" color="text.secondary">{item.label}</Typography>
                  </Box>
                ))}
              </Box>
            </Paper>

            <Typography variant="caption" color="text.secondary">
              Sistem tetap memeriksa kontak yang sudah aktif atau memilih STOP. Kontak tersebut otomatis dilewati dan ditampilkan pada hasil.
            </Typography>
          </Stack>
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setEnrollFu(null)}>Batal</Button>
          <Button variant="contained" onClick={doEnroll} disabled={enroll.isPending || recipientSummary.valid === 0}>
            {enroll.isPending ? 'Memulai...' : `Mulai untuk ${recipientSummary.valid} kontak`}
          </Button>
        </DialogActions>
      </Dialog>
    </Box>
  );
}
