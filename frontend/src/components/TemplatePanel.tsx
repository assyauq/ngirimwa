import { useState } from 'react';
import {
  Box, Card, CardContent, Typography, Button, Stack, IconButton,
  Dialog, DialogTitle, DialogContent, DialogActions, TextField, CircularProgress,
  Chip, Alert,
} from '@mui/material';
import AddIcon from '@mui/icons-material/Add';
import EditIcon from '@mui/icons-material/Edit';
import TemplateIcon from '@mui/icons-material/TextSnippetOutlined';
import DeleteIcon from '@mui/icons-material/Delete';
import AttachFileIcon from '@mui/icons-material/AttachFile';
import ImageIcon from '@mui/icons-material/ImageOutlined';
import { useTemplates, useSaveTemplate, useDeleteTemplate } from '../hooks';
import type { Template } from '../types';
import { swalConfirm, swalToast } from '../services/swal';
import PageHeader from './PageHeader';
import EmptyState from './common/EmptyState';
import WhatsAppEditor from './WhatsAppEditor';

const EMPTY: Partial<Template> = { title: '', body: '' };

export default function TemplatePanel({ agentId }: { agentId: number }) {
  const { data: templates, isLoading } = useTemplates(agentId);
  const save = useSaveTemplate(agentId);
  const del = useDeleteTemplate(agentId);
  const [open, setOpen] = useState(false);
  const [form, setForm] = useState<Partial<Template>>(EMPTY);
  const [file, setFile] = useState<File | null>(null);
  const [removeMedia, setRemoveMedia] = useState(false);
  const [errors, setErrors] = useState<Record<string, string>>({});

  const openNew = () => { setForm(EMPTY); setFile(null); setRemoveMedia(false); setErrors({}); setOpen(true); };
  const openEdit = (t: Template) => { setForm(t); setFile(null); setRemoveMedia(false); setErrors({}); setOpen(true); };
  const validate = () => {
    const e: Record<string, string> = {};
    if (!form.title?.trim()) e.title = 'Wajib diisi';
    if (!form.body?.trim()) e.body = 'Wajib diisi';
    setErrors(e);
    return Object.keys(e).length === 0;
  };
  const submit = async () => {
    if (!validate()) return;
    try {
      await save.mutateAsync({ template: form, file, removeMedia });
      setOpen(false);
      swalToast(form.id ? 'Template diperbarui.' : 'Template berhasil dibuat.');
    } catch (error) {
      const message = (error as { response?: { data?: { error?: string } } })?.response?.data?.error || 'Template belum berhasil disimpan.';
      swalToast(message, 'error');
    }
  };
  const remove = async (t: Template) => { if (await swalConfirm('Hapus template ini?')) del.mutate(t.id); };

  if (isLoading) return <Box sx={{ display: 'flex', justifyContent: 'center', mt: 8 }}><CircularProgress /></Box>;

  return (
    <Box>
      <PageHeader title="Template Pesan"
        subtitle="Pesan siap-pakai yang bisa dipanggil cepat di Inbox, Blast, dan Jadwal. Pakai {nama} untuk menyapa otomatis dengan nama kontak."
        action={<Button variant="contained" startIcon={<AddIcon />} onClick={openNew}>Tambah Template</Button>} />

      {(!templates || templates.length === 0) ? (
        <EmptyState
          icon={<TemplateIcon sx={{ fontSize: 48 }} />}
          title="Belum ada template"
          description="Simpan pesan yang sering dipakai sebagai template. Pakai {'{nama}'} untuk personalisasi otomatis."
          actionLabel="Tambah Template"
          onAction={openNew}
        />
      ) : (
        <Stack spacing={1}>
          {templates.map(t => (
            <Card key={t.id}>
              <CardContent>
                <Stack direction="row" sx={{ justifyContent: 'space-between', alignItems: 'flex-start', gap: 1 }}>
                  <Box sx={{ minWidth: 0 }}>
                    <Typography sx={{ fontWeight: 600, mb: 0.5 }}>{t.title}</Typography>
                    <Typography variant="body2" color="text.secondary" sx={{ whiteSpace: 'pre-wrap' }}>{t.body}</Typography>
                    {t.file_name && (
                      <Chip
                        size="small"
                        variant="outlined"
                        icon={t.media_type === 'image' ? <ImageIcon /> : <AttachFileIcon />}
                        label={t.file_name}
                        sx={{ mt: 1, maxWidth: '100%' }}
                      />
                    )}
                  </Box>
                  <Stack direction="row" sx={{ alignItems: 'center', flexShrink: 0 }}>
                    <IconButton size="small" onClick={() => openEdit(t)}><EditIcon fontSize="small" /></IconButton>
                    <IconButton size="small" color="error" onClick={() => remove(t)}><DeleteIcon fontSize="small" /></IconButton>
                  </Stack>
                </Stack>
              </CardContent>
            </Card>
          ))}
        </Stack>
      )}

      <Dialog open={open} onClose={() => setOpen(false)} fullWidth maxWidth="sm">
        <DialogTitle>{form.id ? 'Edit Template' : 'Template Baru'}</DialogTitle>
        <DialogContent>
          <Stack spacing={2} sx={{ mt: 1 }}>
            <TextField label="Judul" value={form.title ?? ''}
              onChange={e => { setForm({ ...form, title: e.target.value }); if (errors.title) setErrors(p => ({ ...p, title: '' })); }} size="small"
              placeholder="Sapaan order" error={!!errors.title} helperText={errors.title || 'Nama singkat untuk mengenali template ini.'} />
            <Box>
              <Typography variant="caption" color="text.secondary" sx={{ mb: 0.5, display: 'block' }}>Isi pesan</Typography>
              <WhatsAppEditor value={form.body ?? ''} onChange={v => { setForm({ ...form, body: v }); if (errors.body) setErrors(p => ({ ...p, body: '' })); }}
                placeholder="Halo {nama}, terima kasih sudah order 🙏" rows={4} error={!!errors.body} helperText={errors.body || 'Tips: {nama} otomatis diganti nama kontak saat dikirim lewat Blast/Jadwal.'} />
            </Box>
            <Box>
              <Typography variant="caption" color="text.secondary" sx={{ mb: 0.75, display: 'block' }}>
                Lampiran (opsional)
              </Typography>
              <Stack direction={{ xs: 'column', sm: 'row' }} spacing={1} sx={{ alignItems: { sm: 'center' } }}>
                <Button component="label" variant="outlined" size="small" startIcon={<AttachFileIcon />}>
                  {file || (form.file_name && !removeMedia) ? 'Ganti lampiran' : 'Pilih lampiran'}
                  <input
                    hidden
                    type="file"
                    accept="image/*,video/*,.pdf,.doc,.docx,.xls,.xlsx,.ppt,.pptx,.txt,.zip"
                    onClick={e => { e.currentTarget.value = ''; }}
                    onChange={e => {
                      const selected = e.target.files?.[0] || null;
                      if (selected && selected.size > 64 * 1024 * 1024) {
                        swalToast('Ukuran lampiran maksimal 64 MB.', 'error');
                        e.target.value = '';
                        return;
                      }
                      setFile(selected);
                      if (selected) setRemoveMedia(false);
                    }}
                  />
                </Button>
                {(file || (form.file_name && !removeMedia)) && (
                  <Chip
                    size="small"
                    label={file?.name || form.file_name}
                    onDelete={() => {
                      if (file) setFile(null);
                      else setRemoveMedia(true);
                    }}
                    sx={{ maxWidth: '100%' }}
                  />
                )}
              </Stack>
              <Alert severity="info" sx={{ mt: 1 }}>
                Foto, video, PDF, atau dokumen otomatis ikut saat template dipakai di Inbox, Blast, atau Jadwal. Maksimal 64 MB.
              </Alert>
            </Box>
          </Stack>
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setOpen(false)}>Batal</Button>
          <Button variant="contained" onClick={submit} disabled={save.isPending}>Simpan</Button>
        </DialogActions>
      </Dialog>
    </Box>
  );
}
