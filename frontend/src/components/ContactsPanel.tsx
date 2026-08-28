import { useState } from 'react';
import { useQueryClient } from '@tanstack/react-query';
import {
  Box, Typography, Button, Stack, Chip, IconButton, Checkbox, Card, CardContent, Alert, Collapse, Divider, Tooltip, Avatar,
  Dialog, DialogTitle, DialogContent, DialogActions, TextField, CircularProgress, InputAdornment, FormControlLabel, Switch,
  Table, TableBody, TableCell, TableContainer, TableHead, TableRow, Paper, Pagination, MenuItem,
} from '@mui/material';
import EmptyState from './common/EmptyState';
import PeopleIcon from '@mui/icons-material/PeopleOutlined';
import AddIcon from '@mui/icons-material/Add';
import EditIcon from '@mui/icons-material/Edit';
import DeleteIcon from '@mui/icons-material/Delete';
import SearchIcon from '@mui/icons-material/Search';
import ChatIcon from '@mui/icons-material/ChatBubbleOutlineOutlined';
import CampaignIcon from '@mui/icons-material/CampaignOutlined';
import ScheduleIcon from '@mui/icons-material/ScheduleOutlined';
import LocalOfferIcon from '@mui/icons-material/LocalOfferOutlined';
import CloseIcon from '@mui/icons-material/Close';
import UploadFileIcon from '@mui/icons-material/UploadFileOutlined';
import AutoAwesomeIcon from '@mui/icons-material/AutoAwesomeOutlined';
import LockIcon from '@mui/icons-material/LockOutlined';
import { useCrmContacts, useSaveCrmContact, useDeleteCrmContact, useCrmContactsExport, useBulkDeleteCrmContacts, useBulkStageCrmContacts } from '../hooks';
import type { LeadStage, SavedContact } from '../types';
import api from '../services/api';
import PageHeader from './PageHeader';
import ContactImportDialog from './contacts/ContactImportDialog';
import { swalConfirm, swalToast } from '../services/swal';

const EMPTY: Partial<SavedContact> = { number: '', name: '', notes: '', tags: '', lead_stage: 'new' };
const AI_INFO_DISMISSED_KEY = 'chatloop.contacts.ai-info-dismissed';

const LEAD_STAGES: { value: LeadStage; label: string; color: string; bg: string }[] = [
  { value: 'new', label: 'Baru', color: '#455a64', bg: '#eceff1' },
  { value: 'cold', label: 'Cold', color: '#1565c0', bg: '#e3f2fd' },
  { value: 'warm', label: 'Warm', color: '#a15c00', bg: '#fff3e0' },
  { value: 'hot', label: 'Hot', color: '#c62828', bg: '#ffebee' },
  { value: 'customer', label: 'Pelanggan', color: '#2e7d32', bg: '#e8f5e9' },
  { value: 'unqualified', label: 'Tidak potensial', color: '#616161', bg: '#eeeeee' },
];

const stageMeta = (stage: LeadStage) => LEAD_STAGES.find(item => item.value === stage) || LEAD_STAGES[0];
const safeLeadStage = (value: unknown): LeadStage =>
  LEAD_STAGES.some(item => item.value === value) ? value as LeadStage : 'new';
const apiStatus = (error: unknown): number | undefined =>
  (error as { response?: { status?: number } } | null)?.response?.status;

const stageSourceLabel = (contact: SavedContact): string => {
  if (contact.lead_stage_locked) return 'Diatur manual';
  if (contact.lead_stage_source === 'ai') return `Dinilai AI${contact.lead_stage_confidence ? ` · ${Math.round(contact.lead_stage_confidence * 100)}%` : ''}`;
  if (contact.lead_stage_source === 'activity') return 'Dari aktivitas';
  if (contact.lead_stage_source === 'manual') return 'AI aktif · menunggu penilaian';
  return 'Otomatis';
};

function StageAssessment({ contact }: { contact: SavedContact }) {
  const manual = contact.lead_stage_locked;
  return (
    <Tooltip title={contact.lead_stage_reason || (manual ? 'AI tidak akan mengubah status ini.' : 'Status diperbarui otomatis.')} arrow>
      <Stack direction="row" spacing={0.4} sx={{ mt: 0.35, alignItems: 'center', color: 'text.secondary', width: 'fit-content' }}>
        {manual ? <LockIcon sx={{ fontSize: 12 }} /> : <AutoAwesomeIcon sx={{ fontSize: 12 }} />}
        <Typography variant="caption" sx={{ fontSize: 10.5 }}>{stageSourceLabel(contact)}</Typography>
      </Stack>
    </Tooltip>
  );
}

function StageSelect({ value, onChange, fullWidth = false, disabled = false }: {
  value: LeadStage | undefined | null;
  onChange: (stage: LeadStage) => void;
  fullWidth?: boolean;
  disabled?: boolean;
}) {
  const safeValue = safeLeadStage(value);
  const meta = stageMeta(safeValue);
  return (
    <TextField select size="small" value={safeValue} onChange={e => onChange(e.target.value as LeadStage)} fullWidth={fullWidth} disabled={disabled}
      aria-label="Status CRM"
      sx={{ minWidth: fullWidth ? undefined : 122, '& .MuiInputBase-root': { height: 30, color: meta.color, bgcolor: meta.bg, fontSize: 12, fontWeight: 750 } }}>
      {LEAD_STAGES.map(item => <MenuItem key={item.value} value={item.value}>{item.label}</MenuItem>)}
    </TextField>
  );
}

function PipelineOverview({ counts, active, onPick, disabled }: {
  counts: Record<LeadStage, number>;
  active: LeadStage | '';
  onPick: (stage: LeadStage | '') => void;
  disabled: boolean;
}) {
  const total = LEAD_STAGES.reduce((sum, item) => sum + (counts[item.value] || 0), 0);
  return (
    <Paper variant="outlined" sx={{ p: { xs: 1, sm: 1.25 }, borderRadius: 1.25, minWidth: 0 }}>
      <Stack direction="row" sx={{ alignItems: 'center', justifyContent: 'space-between', gap: 1, mb: 0.85, minHeight: 34 }}>
        <Box sx={{ minWidth: 0 }}>
          <Typography sx={{ fontSize: 13.5, lineHeight: 1.25, fontWeight: 750 }}>Pipeline CRM</Typography>
          <Typography variant="caption" color="text.secondary" sx={{ display: 'block', mt: 0.15, fontSize: 11.25 }}>
            {total} kontak{active ? ` · filter ${stageMeta(active).label}` : ''}
          </Typography>
        </Box>
        {active && (
          <Button size="small" color="inherit" onClick={() => onPick('')}
            sx={{ minHeight: 28, px: 0.85, fontSize: 11.5, whiteSpace: 'nowrap' }}>
            Reset filter
          </Button>
        )}
      </Stack>
      <Box sx={{
        display: { xs: 'flex', sm: 'grid' },
        gridTemplateColumns: { sm: 'repeat(3, minmax(0, 1fr))' },
        gap: 0.625,
        overflowX: { xs: 'auto', sm: 'visible' },
        pb: { xs: 0.25, sm: 0 },
        scrollSnapType: { xs: 'x proximity', sm: 'none' },
        scrollbarWidth: 'thin',
        '& > *': { flex: { xs: '0 0 126px', sm: 'initial' }, scrollSnapAlign: 'start' },
      }}>
        {LEAD_STAGES.map(item => {
          const selected = active === item.value;
          return (
            <Button key={item.value} variant="outlined" disabled={disabled} onClick={() => onPick(item.value)}
              aria-pressed={selected}
              sx={{
                px: 0.9, py: 0.65, minWidth: 0, minHeight: 49, justifyContent: 'flex-start',
                textTransform: 'none', borderColor: selected ? item.color : 'divider',
                bgcolor: selected ? item.bg : 'background.paper', color: item.color,
              }}>
              <Box sx={{ textAlign: 'left', minWidth: 0 }}>
                <Typography sx={{ fontSize: 16.5, lineHeight: 1.05, fontWeight: 800, fontVariantNumeric: 'tabular-nums' }}>
                  {counts[item.value] || 0}
                </Typography>
                <Typography variant="caption" sx={{ mt: 0.2, color: selected ? 'inherit' : 'text.secondary', fontSize: 11.25, whiteSpace: 'nowrap', overflow: 'hidden', textOverflow: 'ellipsis', display: 'block' }}>
                  {item.label}
                </Typography>
              </Box>
            </Button>
          );
        })}
      </Box>
    </Paper>
  );
}

export default function ContactsPanel({ agentId, onBroadcast, onScheduleBroadcast, onOpenChat, canBroadcast = true }: {
  agentId: number;
  onBroadcast: (recipients: string) => void;
  /** Kirim daftar penerima ke tab Jadwal Blast (opsional). */
  onScheduleBroadcast?: (recipients: string) => void;
  onOpenChat: (number: string) => void;
  canBroadcast?: boolean;
}) {
  const [addOpen, setAddOpen] = useState(false);
  const [edit, setEdit] = useState<SavedContact | null>(null);
  const [open, setOpen] = useState(false);
  const [form, setForm] = useState<Partial<SavedContact>>(EMPTY);
  const [formErrors, setFormErrors] = useState<Record<string, string>>({});
  const [q, setQ] = useState('');
  const [tag, setTag] = useState('');
  const [stage, setStage] = useState<LeadStage | ''>('');
  const [page, setPage] = useState(1);
  const [selected, setSelected] = useState<Set<number>>(new Set());
  const [bulkTag, setBulkTag] = useState('');
  const [bulkApplying, setBulkApplying] = useState(false);
  const [tagModalOpen, setTagModalOpen] = useState(false);
  const [stageModalOpen, setStageModalOpen] = useState(false);
  const [bulkStage, setBulkStage] = useState<LeadStage>('warm');
  const [importOpen, setImportOpen] = useState(false);
  const [aiInfoDismissed, setAiInfoDismissed] = useState(() => {
    try {
      return window.localStorage.getItem(AI_INFO_DISMISSED_KEY) === '1';
    } catch {
      return false;
    }
  });

  const { data, isLoading } = useCrmContacts(agentId, q, tag, stage, page);
  const saveCrmContact = useSaveCrmContact(agentId);
  const deleteCrmContact = useDeleteCrmContact(agentId);
  const bulkDelete = useBulkDeleteCrmContacts(agentId);
  const bulkStageMutation = useBulkStageCrmContacts(agentId);
  const crmExport = useCrmContactsExport(agentId);
  const queryClient = useQueryClient();

  const contacts = data?.data || [];
  const allTags = data?.all_tags || [];
  const totalContacts = data?.total ?? 0;
  const crmBackendReady = !data || !!data.stage_counts;
  const stageCounts: Record<LeadStage, number> = data?.stage_counts || {
    new: totalContacts, cold: 0, warm: 0, hot: 0, customer: 0, unqualified: 0,
  };
  const totalPages = Math.max(1, Math.ceil(totalContacts / (data?.limit ?? 20)));
  const selectedContacts = contacts.filter(c => selected.has(c.id));
  const selectedIDs = selectedContacts.map(contact => contact.id);
  const hasFilter = !!q.trim() || !!tag || !!stage;
  const allPageSelected = contacts.length > 0 && contacts.every(contact => selected.has(contact.id));
  const somePageSelected = contacts.some(contact => selected.has(contact.id));

  const openAdd = () => { setForm(EMPTY); setFormErrors({}); setAddOpen(true); };
  const openEdit = (ct: SavedContact) => {
    const normalized = { ...ct, lead_stage: safeLeadStage(ct.lead_stage) };
    setForm(normalized); setFormErrors({}); setEdit(normalized); setOpen(true);
  };
  const closeDialog = () => { setAddOpen(false); setOpen(false); setEdit(null); setFormErrors({}); };

  const validate = (): boolean => {
    const errs: Record<string, string> = {};
    if (!form.number?.trim()) errs.number = 'Nomor WhatsApp wajib diisi';
    setFormErrors(errs);
    return Object.keys(errs).length === 0;
  };

  const save = async () => {
    if (!validate()) return;
    try {
      const payload = { ...form };
      if (edit && payload.lead_stage === edit.lead_stage) delete payload.lead_stage;
      if (edit && payload.lead_stage_locked === edit.lead_stage_locked) delete payload.lead_stage_locked;
      await saveCrmContact.mutateAsync(payload);
      swalToast(addOpen ? 'Kontak ditambahkan' : 'Kontak disimpan');
      closeDialog();
    } catch {
      swalToast('Kontak belum bisa disimpan', 'error');
    }
  };

  const remove = async (ct: SavedContact) => {
    if (!await swalConfirm(`Hapus kontak ${ct.name || ct.number}?`, 'Kontak yang dihapus tidak muncul lagi di daftar CRM.')) return;
    try {
      await deleteCrmContact.mutateAsync(ct.id);
      setSelected(prev => {
        const next = new Set(prev);
        next.delete(ct.id);
        return next;
      });
      swalToast('Kontak dihapus');
    } catch {
      swalToast('Kontak belum bisa dihapus', 'error');
    }
  };

  const pickStage = (next: LeadStage | '') => { setStage(prev => prev === next ? '' : next); setPage(1); setSelected(new Set()); };

  const resolveBroadcastRecipients = async () => {
    const list = selectedContacts.length > 0 ? selectedContacts : await crmExport.mutateAsync({ q, tag, stage });
    return {
      list,
      lines: list.map(c => `${c.number},${c.name || ''}`).join('\n'),
    };
  };

  const handleBroadcast = async () => {
    try {
      const { list, lines } = await resolveBroadcastRecipients();
      onBroadcast(lines);
      swalToast(`${list.length} kontak dikirim ke Blast`);
    } catch {
      swalToast('Kontak belum bisa dikirim ke Blast', 'error');
    }
  };

  const handleScheduleBroadcast = async () => {
    if (!onScheduleBroadcast) return;
    try {
      const { list, lines } = await resolveBroadcastRecipients();
      onScheduleBroadcast(lines);
      swalToast(`${list.length} kontak dikirim ke Jadwal Blast`);
    } catch {
      swalToast('Kontak belum bisa dikirim ke Jadwal Blast', 'error');
    }
  };

  const toggleSelect = (id: number) => {
    setSelected(prev => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  };

  const toggleSelectAll = () => {
    if (allPageSelected) {
      setSelected(new Set());
    } else {
      setSelected(new Set(contacts.map(c => c.id)));
    }
  };

  const handleBulkTag = async () => {
    if (!bulkTag.trim() || selectedIDs.length === 0) return;
    setBulkApplying(true);
    try {
      await api.post(`/agents/${agentId}/crm/contacts/bulk-tag`, {
        ids: selectedIDs,
        tag: bulkTag.trim(),
      });
      queryClient.invalidateQueries({ queryKey: ['crm-contacts', agentId] });
      setSelected(new Set());
      setBulkTag('');
      swalToast('Tag ditambahkan');
    } catch {
      swalToast('Tag belum bisa ditambahkan', 'error');
    } finally {
      setBulkApplying(false);
    }
  };

  const changeStage = async (ids: number[], leadStage: LeadStage, clearSelection = false): Promise<boolean> => {
    if (ids.length === 0) return false;
    try {
      const res = await bulkStageMutation.mutateAsync({ ids, lead_stage: leadStage });
      if (clearSelection) setSelected(new Set());
      else setSelected(prev => {
        const next = new Set(prev);
        ids.forEach(id => next.delete(id));
        return next;
      });
      swalToast(`${res.updated} kontak dipindahkan ke ${stageMeta(leadStage).label}`);
      return true;
    } catch (error) {
      swalToast(apiStatus(error) === 404
        ? 'Backend belum dimuat ulang. Restart backend agar fitur status CRM aktif.'
        : 'Status CRM belum bisa diubah', 'error');
      return false;
    }
  };

  const handleBulkDeleteSelected = async () => {
    if (selectedIDs.length === 0) return;
    if (!await swalConfirm(`Hapus ${selectedIDs.length} kontak terpilih?`, 'Kontak yang dihapus tidak muncul lagi di daftar CRM.')) return;
    try {
      const res = await bulkDelete.mutateAsync({ ids: selectedIDs });
      setSelected(new Set());
      swalToast(`${res.deleted} kontak dihapus`);
    } catch {
      swalToast('Kontak belum bisa dihapus', 'error');
    }
  };

  const handleDeleteAll = async () => {
    const scope = hasFilter ? 'semua kontak yang cocok filter ini' : 'SEMUA kontak';
    if (!await swalConfirm(`Hapus ${scope}?`, 'Tindakan ini tidak bisa dibatalkan. Kontak akan hilang dari daftar CRM.')) return;
    try {
      const res = await bulkDelete.mutateAsync({ all: true, q, tag, stage });
      setSelected(new Set());
      setPage(1);
      swalToast(`${res.deleted} kontak dihapus`);
    } catch {
      swalToast('Kontak belum bisa dihapus', 'error');
    }
  };

  const contactInitial = (ct: SavedContact) => {
    const base = (ct.name || ct.number || '?').trim();
    return base.slice(0, 1).toUpperCase();
  };
  const contactPhoto = (number: string) => {
    if (!data?.media_token) return undefined;
    return `/api/agents/${agentId}/profile-picture?sender=${encodeURIComponent(number)}&token=${encodeURIComponent(data.media_token)}`;
  };

  const dismissAIInfo = () => {
    setAiInfoDismissed(true);
    try {
      window.localStorage.setItem(AI_INFO_DISMISSED_KEY, '1');
    } catch {
      // Penyimpanan browser dapat dinonaktifkan; state sesi tetap cukup.
    }
  };

  return (
    <Box>
      <PageHeader
        dense
        title="Kontak"
        subtitle="Kelola prospek, tentukan prioritas, lalu tindak lanjuti dari satu tempat."
        action={
          <Stack direction="row" spacing={0.75} sx={{ width: '100%' }}>
            <Button variant="outlined" startIcon={<UploadFileIcon />} onClick={() => setImportOpen(true)}
              sx={{ flex: { xs: 1, sm: 'initial' }, whiteSpace: 'nowrap' }}>
              Impor
            </Button>
            <Button variant="contained" startIcon={<AddIcon />} onClick={openAdd}
              sx={{ flex: { xs: 1, sm: 'initial' }, whiteSpace: 'nowrap' }}>
              Tambah Kontak
            </Button>
          </Stack>
        }
      />

      {!crmBackendReady && (
        <Alert severity="warning" sx={{ mb: 1.25 }}>
          Backend masih memakai versi lama. Restart backend terlebih dahulu agar status CRM dapat disimpan.
        </Alert>
      )}

      {crmBackendReady && (
        <Collapse in={!aiInfoDismissed}>
          <Alert severity="info" icon={<AutoAwesomeIcon fontSize="inherit" />}
            sx={{ mb: 1, py: 0.55, '& .MuiAlert-message': { fontSize: 12.25, lineHeight: 1.4 } }}
            action={
              <IconButton size="small" color="inherit" onClick={dismissAIInfo} aria-label="Tutup info">
                <CloseIcon sx={{ fontSize: 17 }} />
              </IconButton>
            }>
            AI dapat memperbarui status CRM dari konteks chat. Status yang Anda ubah manual akan tetap dikunci.
          </Alert>
        </Collapse>
      )}

      <Box sx={{
        display: 'grid',
        gridTemplateColumns: { xs: 'minmax(0, 1fr)', lg: 'repeat(2, minmax(0, 1fr))' },
        gap: 1,
        mb: 1.25,
      }}>
        <PipelineOverview counts={stageCounts} active={stage} onPick={pickStage} disabled={isLoading || !crmBackendReady} />
      </Box>

      <Paper variant="outlined" sx={{ mb: 1.25, borderRadius: 1.25, overflow: 'hidden' }}>
        <Box sx={{ p: 1 }}>
          <Stack direction={{ xs: 'column', lg: 'row' }} spacing={0.75} sx={{ alignItems: { xs: 'stretch', lg: 'center' } }}>
            <TextField
              fullWidth size="small" placeholder="Cari nama atau nomor"
              value={q} onChange={e => { setQ(e.target.value); setPage(1); setSelected(new Set()); }}
              slotProps={{
                input: {
                  startAdornment: <InputAdornment position="start"><SearchIcon fontSize="small" /></InputAdornment>,
                  endAdornment: q ? (
                    <InputAdornment position="end">
                      <IconButton size="small" aria-label="Hapus pencarian"
                        onClick={() => { setQ(''); setPage(1); setSelected(new Set()); }}>
                        <CloseIcon fontSize="small" />
                      </IconButton>
                    </InputAdornment>
                  ) : undefined,
                },
              }}
            />
            <TextField select size="small" label="Tag" value={tag} disabled={allTags.length === 0}
              onChange={e => { setTag(e.target.value); setPage(1); setSelected(new Set()); }}
              sx={{ minWidth: { lg: 165 } }}>
              <MenuItem value="">Semua tag</MenuItem>
              {allTags.map(item => <MenuItem key={item} value={item}>{item}</MenuItem>)}
            </TextField>
            {canBroadcast && (
              <Box sx={{
                display: 'grid',
                gridTemplateColumns: onScheduleBroadcast
                  ? { xs: 'minmax(0, 1fr)', sm: 'repeat(2, minmax(0, 1fr))' }
                  : 'minmax(0, 1fr)',
                gap: 0.75,
                flexShrink: 0,
              }}>
                <Button
                  variant="outlined"
                  startIcon={<CampaignIcon />}
                  onClick={handleBroadcast}
                  disabled={(selectedContacts.length === 0 && totalContacts === 0) || crmExport.isPending}
                  sx={{ minWidth: { lg: 148 }, whiteSpace: 'nowrap' }}
                >
                  {selectedContacts.length
                    ? `Blast ${selectedContacts.length} dipilih`
                    : hasFilter ? 'Blast hasil filter' : 'Blast semua'}
                </Button>
                {onScheduleBroadcast && (
                  <Button
                    variant="outlined"
                    startIcon={<ScheduleIcon />}
                    onClick={handleScheduleBroadcast}
                    disabled={(selectedContacts.length === 0 && totalContacts === 0) || crmExport.isPending}
                    sx={{ minWidth: { lg: 148 }, whiteSpace: 'nowrap' }}
                  >
                    {selectedContacts.length
                      ? `Jadwalkan ${selectedContacts.length}`
                      : hasFilter ? 'Jadwalkan hasil' : 'Jadwalkan semua'}
                  </Button>
                )}
              </Box>
            )}
          </Stack>
        </Box>
      </Paper>

      {isLoading ? (
        <Paper variant="outlined" sx={{ textAlign: 'center', py: 4 }}>
          <CircularProgress size={24} />
          <Typography variant="body2" color="text.secondary" sx={{ mt: 1 }}>Memuat kontak...</Typography>
        </Paper>
      ) : contacts.length === 0 ? (
        <EmptyState
          icon={<PeopleIcon sx={{ fontSize: 48 }} />}
          title={hasFilter ? 'Tidak ada kontak' : 'Belum ada kontak'}
          description={hasFilter
            ? 'Coba ubah filter atau kata kunci.'
            : 'Kontak masuk otomatis saat pelanggan chat. Atau impor manual, dari nomor terkoneksi, maupun file CSV.'}
          actionLabel={hasFilter ? undefined : 'Impor Kontak'}
          onAction={hasFilter ? undefined : () => setImportOpen(true)}
        />
      ) : (
        <>
          {selectedContacts.length > 0 && (
            <Paper variant="outlined" sx={{ p: 0.75, mb: 1, borderColor: 'primary.light', bgcolor: 'background.paper', position: 'sticky', top: 8, zIndex: 3, boxShadow: 1 }}>
              <Stack direction="row" sx={{ alignItems: 'center', gap: 0.5, flexWrap: 'wrap' }}>
                <Chip label={`${selectedContacts.length} dipilih`} size="small" color="primary" onDelete={() => setSelected(new Set())} />
                <Box sx={{ flex: 1 }} />
                <Button variant="outlined" size="small" onClick={() => setStageModalOpen(true)} disabled={!crmBackendReady}>
                  Ubah status
                </Button>
                <Button variant="outlined" size="small" startIcon={<LocalOfferIcon />} onClick={() => setTagModalOpen(true)}>
                  Tambah Tag
                </Button>
                <Button variant="text" size="small" color="error" startIcon={<DeleteIcon />}
                  onClick={handleBulkDeleteSelected} disabled={bulkDelete.isPending}>
                  Hapus
                </Button>
              </Stack>
            </Paper>
          )}

          <TableContainer component={Paper} sx={{ mb: 1, display: { xs: 'none', lg: 'block' }, overflowX: 'auto', bgcolor: 'background.paper' }}>
              <Table size="small" aria-label="Daftar kontak" sx={{
                minWidth: 1160,
                '& .MuiTableCell-root': { verticalAlign: 'middle', py: 0.75, borderColor: 'divider' },
                '& .MuiTableHead-root .MuiTableCell-root': {
                  fontWeight: 700, fontSize: 11.5, color: 'text.secondary', bgcolor: 'background.paper', py: 0.75, whiteSpace: 'nowrap',
                },
              }}>
                <TableHead>
                  <TableRow>
                    <TableCell sx={{ width: 40, p: 0.5 }}>
                      <Checkbox
                        size="small"
                        checked={allPageSelected}
                        indeterminate={somePageSelected && !allPageSelected}
                        onChange={toggleSelectAll}
                        slotProps={{ input: { 'aria-label': 'Pilih semua kontak pada halaman ini' } }}
                      />
                    </TableCell>
                    <TableCell>Kontak</TableCell>
                    <TableCell>CRM</TableCell>
                    <TableCell>Chat</TableCell>
                    <TableCell sx={{ width: 112 }}>Aksi</TableCell>
                  </TableRow>
                </TableHead>
                <TableBody>
                  {contacts.map(ct => {
                    return (
                    <TableRow key={ct.id} hover selected={selected.has(ct.id)}>
                      <TableCell sx={{ p: 0.5 }}>
                        <Checkbox size="small" checked={selected.has(ct.id)} onChange={() => toggleSelect(ct.id)}
                          slotProps={{ input: { 'aria-label': `Pilih ${ct.name || ct.number}` } }} />
                      </TableCell>
                      <TableCell>
                        <Stack direction="row" spacing={1} sx={{ alignItems: 'center', minWidth: 0 }}>
                          <Avatar
                            src={contactPhoto(ct.number)}
                            slotProps={{ img: { loading: 'lazy' } }}
                            sx={{ width: 28, height: 28, fontSize: 12, bgcolor: 'primary.main' }}
                          >
                            {contactInitial(ct)}
                          </Avatar>
                          <Box sx={{ minWidth: 0 }}>
                            <Typography sx={{ fontWeight: 700, fontSize: 13, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap', lineHeight: 1.3 }}>
                              {ct.name || `+${ct.number}`}
                            </Typography>
                            <Typography variant="caption" color="text.secondary" sx={{ fontSize: 11.5, fontVariantNumeric: 'tabular-nums' }}>
                              +{ct.number}
                            </Typography>
                          </Box>
                        </Stack>
                      </TableCell>
                      <TableCell>
                        <StageSelect value={ct.lead_stage} onChange={next => changeStage([ct.id], next)} disabled={!crmBackendReady} />
                        <StageAssessment contact={ct} />
                      </TableCell>
                      <TableCell>
                        <Typography variant="caption" color="text.secondary" sx={{ whiteSpace: 'nowrap' }}>
                          {ct.last_at ? lastChatLabel(ct.last_at) : '—'}
                        </Typography>
                      </TableCell>
                      <TableCell>
                        <Stack direction="row" spacing={0.15}>
                          <Tooltip title="Buka chat"><IconButton size="small" aria-label={`Buka chat ${ct.name || ct.number}`} onClick={() => onOpenChat(ct.number)}><ChatIcon fontSize="small" /></IconButton></Tooltip>
                          <Tooltip title="Edit kontak"><IconButton size="small" aria-label={`Edit ${ct.name || ct.number}`} onClick={() => openEdit(ct)}><EditIcon fontSize="small" /></IconButton></Tooltip>
                          <Tooltip title="Hapus kontak"><IconButton size="small" aria-label={`Hapus ${ct.name || ct.number}`} color="error" onClick={() => remove(ct)}><DeleteIcon fontSize="small" /></IconButton></Tooltip>
                        </Stack>
                      </TableCell>
                    </TableRow>
                    );
                  })}
                </TableBody>
              </Table>
          </TableContainer>

          <Stack spacing={1} sx={{ display: { xs: 'flex', lg: 'none' }, mb: 1 }}>
            {contacts.map(ct => (
              <Card key={ct.id} variant="outlined" sx={{ borderColor: selected.has(ct.id) ? 'primary.main' : 'divider' }}>
                <CardContent sx={{ p: 1.25, '&:last-child': { pb: 1.25 } }}>
                  <Stack direction="row" spacing={1} sx={{ alignItems: 'flex-start' }}>
                    <Checkbox size="small" checked={selected.has(ct.id)} onChange={() => toggleSelect(ct.id)}
                      slotProps={{ input: { 'aria-label': `Pilih ${ct.name || ct.number}` } }} sx={{ p: 0.25 }} />
                    <Avatar
                      src={contactPhoto(ct.number)}
                      slotProps={{ img: { loading: 'lazy' } }}
                      sx={{ width: 32, height: 32, fontSize: 13, bgcolor: 'primary.main', flexShrink: 0 }}
                    >
                      {contactInitial(ct)}
                    </Avatar>
                    <Box sx={{ flex: 1, minWidth: 0 }}>
                      <Stack direction={{ xs: 'column', sm: 'row' }}
                        sx={{ alignItems: { xs: 'stretch', sm: 'flex-start' }, justifyContent: 'space-between', gap: 0.75 }}>
                        <Box sx={{ minWidth: 0, flex: 1 }}>
                          <Typography sx={{ fontSize: 13.5, fontWeight: 750, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{ct.name || `+${ct.number}`}</Typography>
                          <Typography variant="caption" color="text.secondary">+{ct.number}</Typography>
                        </Box>
                        <Box sx={{ width: { xs: '100%', sm: 132 }, flexShrink: 0 }}>
                          <StageSelect value={ct.lead_stage} onChange={next => changeStage([ct.id], next)} fullWidth disabled={!crmBackendReady} />
                          <StageAssessment contact={ct} />
                        </Box>
                      </Stack>
                      <Typography variant="caption" color="text.secondary" sx={{ display: 'block', mt: 0.25 }}>
                        {ct.last_at ? lastChatLabel(ct.last_at) : 'Belum ada riwayat chat'}
                      </Typography>
                      {ct.tags && (
                        <Stack direction="row" sx={{ flexWrap: 'wrap', gap: 0.5, mt: 0.75 }}>
                          {ct.tags.split(',').map(t => t.trim()).filter(Boolean).slice(0, 4).map((t, i) => (
                            <Chip key={i} label={t} size="small" variant="outlined" sx={{ height: 20, fontSize: '0.65rem' }} />
                          ))}
                        </Stack>
                      )}
                      {ct.notes && (
                        <Box sx={{ mt: 0.75, px: 0.75, py: 0.5, bgcolor: 'action.hover', borderRadius: 0.75 }}>
                          <Typography variant="caption" color="text.secondary" sx={{ display: 'block', fontWeight: 700 }}>Catatan</Typography>
                          <Typography variant="caption" sx={{ display: '-webkit-box', WebkitLineClamp: 3, WebkitBoxOrient: 'vertical', overflow: 'hidden', whiteSpace: 'pre-wrap' }}>
                            {ct.notes}
                          </Typography>
                        </Box>
                      )}
                    </Box>
                  </Stack>
                  <Divider sx={{ my: 1 }} />
                  <Stack direction="row" spacing={0.5} sx={{ justifyContent: 'flex-end', flexWrap: 'wrap' }}>
                    <Button size="small" startIcon={<ChatIcon />} onClick={() => onOpenChat(ct.number)} sx={{ flex: { xs: 1, sm: 'initial' } }}>Chat</Button>
                    <Button size="small" startIcon={<EditIcon />} onClick={() => openEdit(ct)} sx={{ flex: { xs: 1, sm: 'initial' } }}>Edit</Button>
                    <Button size="small" color="error" startIcon={<DeleteIcon />} onClick={() => remove(ct)} sx={{ flex: { xs: 1, sm: 'initial' } }}>Hapus</Button>
                  </Stack>
                </CardContent>
              </Card>
            ))}
          </Stack>

          <Stack direction={{ xs: 'column', sm: 'row' }} sx={{ alignItems: { xs: 'stretch', sm: 'center' }, justifyContent: 'space-between', mb: 1, gap: 1 }}>
            <Stack direction="row" spacing={1.5} sx={{ alignItems: 'center', flexWrap: 'wrap' }}>
              <Typography variant="body2" color="text.secondary">
                Menampilkan {contacts.length} dari {totalContacts} kontak
              </Typography>
              <Button size="small" color="error" startIcon={<DeleteIcon />} onClick={handleDeleteAll} disabled={bulkDelete.isPending}>
                {hasFilter ? 'Hapus hasil filter' : 'Hapus semua'}
              </Button>
            </Stack>
            <Pagination
              count={totalPages}
              page={page}
              onChange={(_e, p) => { setPage(p); setSelected(new Set()); }}
              size="small"
              siblingCount={0}
              boundaryCount={1}
            />
          </Stack>
        </>
      )}

      <Dialog open={stageModalOpen} onClose={() => setStageModalOpen(false)} maxWidth="xs" fullWidth>
        <DialogTitle>Ubah Status CRM</DialogTitle>
        <DialogContent>
          <Stack spacing={1.5} sx={{ mt: 1 }}>
            <Alert severity="info" icon={false}>
              Status baru akan diterapkan ke {selectedContacts.length} kontak terpilih.
            </Alert>
            <TextField select label="Status CRM" size="small" value={bulkStage}
              onChange={e => setBulkStage(e.target.value as LeadStage)}>
              {LEAD_STAGES.map(item => <MenuItem key={item.value} value={item.value}>{item.label}</MenuItem>)}
            </TextField>
          </Stack>
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setStageModalOpen(false)}>Batal</Button>
          <Button variant="contained" disabled={bulkStageMutation.isPending || selectedIDs.length === 0}
            onClick={async () => { if (await changeStage(selectedIDs, bulkStage, true)) setStageModalOpen(false); }}>
            Terapkan
          </Button>
        </DialogActions>
      </Dialog>

      <Dialog open={tagModalOpen} onClose={() => { setTagModalOpen(false); setBulkTag(''); }} maxWidth="xs" fullWidth>
        <DialogTitle>Tambah Tag</DialogTitle>
        <DialogContent>
          <Stack spacing={2} sx={{ mt: 1 }}>
            <Alert severity="info" icon={false}>
              Tag akan ditambahkan ke {selectedContacts.length} kontak yang sedang dipilih.
            </Alert>
            <TextField
              label="Tag"
              size="small"
              value={bulkTag}
              onChange={e => setBulkTag(e.target.value)}
              placeholder="vip, pelanggan tetap"
              autoFocus
            />
            {allTags.length > 0 && (
              <Box>
                <Typography variant="caption" color="text.secondary" sx={{ mb: 0.5, display: 'block' }}>
                  Tag yang sudah ada:
                </Typography>
                <Stack direction="row" sx={{ gap: 0.5, flexWrap: 'wrap' }}>
                  {allTags.map(t => (
                    <Chip key={t} label={t} size="small" variant="outlined" onClick={() => setBulkTag(t)}
                      sx={{ cursor: 'pointer', '&:hover': { opacity: 0.8 } }} />
                  ))}
                </Stack>
              </Box>
            )}
          </Stack>
        </DialogContent>
        <DialogActions>
          <Button onClick={() => { setTagModalOpen(false); setBulkTag(''); }}>Batal</Button>
          <Button variant="contained" onClick={async () => { await handleBulkTag(); setTagModalOpen(false); }} disabled={!bulkTag.trim() || bulkApplying}>
            {bulkApplying ? '...' : 'Terapkan'}
          </Button>
        </DialogActions>
      </Dialog>

      <Dialog open={addOpen || open} onClose={closeDialog} maxWidth="sm" fullWidth>
        <DialogTitle>{addOpen ? 'Tambah Kontak' : 'Edit Kontak'}</DialogTitle>
        <DialogContent>
          <Stack spacing={1.5} sx={{ mt: 1 }}>
            <Alert severity="info" icon={false}>
              Kontak dari chat WhatsApp akan masuk otomatis. Form ini dipakai untuk menambah atau merapikan kontak manual.
            </Alert>
            <TextField label="Nama kontak" size="small" value={form.name || ''} onChange={e => setForm({...form, name: e.target.value})}
              placeholder="Budi, Sinta, Toko Maju" />
            <TextField label="Nomor WhatsApp" size="small" value={form.number || ''}
              onChange={e => { setForm({...form, number: e.target.value}); if (formErrors.number) setFormErrors(p => ({ ...p, number: '' })); }}
              disabled={!!edit} error={!!formErrors.number}
              helperText={formErrors.number || (edit ? 'Nomor tidak bisa diubah setelah kontak dibuat.' : 'Boleh pakai format 08xx atau 62xx.')} />
            <TextField select label="Status CRM" size="small" value={safeLeadStage(form.lead_stage)} disabled={!crmBackendReady}
              onChange={e => setForm({ ...form, lead_stage: e.target.value as LeadStage, lead_stage_locked: true })}
              helperText={crmBackendReady ? 'Gunakan satu status utama; tag tetap untuk segmentasi tambahan.' : 'Restart backend untuk mengaktifkan status CRM.'}>
              {LEAD_STAGES.map(item => <MenuItem key={item.value} value={item.value}>{item.label}</MenuItem>)}
            </TextField>
            {!!edit && form.lead_stage_reason && (
              <Alert severity="info" icon={form.lead_stage_locked ? <LockIcon fontSize="inherit" /> : <AutoAwesomeIcon fontSize="inherit" />}>
                <Typography variant="caption" sx={{ display: 'block', fontWeight: 800 }}>{stageSourceLabel(form as SavedContact)}</Typography>
                <Typography variant="body2">{form.lead_stage_reason}</Typography>
              </Alert>
            )}
            {!!edit && (
              <FormControlLabel
                control={<Switch checked={!form.lead_stage_locked} onChange={e => setForm({ ...form, lead_stage_locked: !e.target.checked })} />}
                label={
                  <Box>
                    <Typography variant="body2" sx={{ fontWeight: 700 }}>Izinkan AI memperbarui status</Typography>
                    <Typography variant="caption" color="text.secondary">
                      Nonaktifkan bila status pilihan Anda tidak boleh diubah otomatis.
                    </Typography>
                  </Box>
                }
              />
            )}
            <TextField label="Tag" size="small" value={form.tags || ''} onChange={e => setForm({...form, tags: e.target.value})}
              placeholder="vip, pelanggan tetap" helperText="Pisahkan beberapa tag dengan koma." />
            <TextField label="Catatan" size="small" multiline rows={2} value={form.notes || ''} onChange={e => setForm({...form, notes: e.target.value})}
              placeholder="Contoh: suka produk A, follow up bulan depan." />
          </Stack>
        </DialogContent>
        <DialogActions>
          <Button onClick={closeDialog}>Batal</Button>
          <Button variant="contained" onClick={save} disabled={saveCrmContact.isPending}>Simpan</Button>
        </DialogActions>
      </Dialog>

      <ContactImportDialog agentId={agentId} open={importOpen} onClose={() => setImportOpen(false)} />
    </Box>
  );
}

function lastChatLabel(d: string | undefined | null): string {
  if (!d) return '';
  const now = Date.now();
  const then = new Date(d).getTime();
  const diff = now - then;
  const mins = Math.floor(diff / 60000);
  if (mins < 1) return 'Baru saja';
  if (mins < 60) return `${mins} menit lalu`;
  const hours = Math.floor(mins / 60);
  if (hours < 24) return `${hours} jam lalu`;
  const days = Math.floor(hours / 24);
  if (days < 30) return `${days} hari lalu`;
  return new Date(d).toLocaleDateString('id-ID');
}
