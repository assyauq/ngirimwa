import { memo, useState } from 'react';
import { Button, Menu, MenuItem, ListItemText, Typography, Chip, CircularProgress, Stack } from '@mui/material';
import TextSnippetIcon from '@mui/icons-material/TextSnippetOutlined';
import AttachFileIcon from '@mui/icons-material/AttachFile';
import { useTemplates } from '../hooks';
import api from '../services/api';
import { swalToast } from '../services/swal';
import type { Template } from '../types';

interface Props {
  agentId: number;
  onPick: (body: string, attachment?: File) => void;
  supportsAttachment?: boolean;
  size?: 'small' | 'medium';
  variant?: 'text' | 'outlined' | 'contained';
  label?: string;
}

// TemplatePicker = tombol kecil untuk menyisipkan isi template ke composer pesan.
// Dipakai di Inbox, Broadcast, dan Jadwal.
const TemplatePicker = memo(function TemplatePicker({ agentId, onPick, supportsAttachment = false, size = 'small', variant = 'outlined', label = 'Template' }: Props) {
  const { data: templates } = useTemplates(agentId);
  const [anchor, setAnchor] = useState<null | HTMLElement>(null);
  const [loadingId, setLoadingId] = useState<number | null>(null);

  const pick = async (template: Template) => {
    if (!template.file_name || !supportsAttachment) {
      onPick(template.body);
      setAnchor(null);
      return;
    }
    setLoadingId(template.id);
    try {
      const response = await api.get(`/agents/${agentId}/templates/${template.id}/media`, { responseType: 'blob' });
      const attachment = new File(
        [response.data],
        template.file_name,
        { type: template.mimetype || response.data.type || 'application/octet-stream' },
      );
      onPick(template.body, attachment);
      setAnchor(null);
    } catch {
      swalToast('Lampiran template belum berhasil dimuat. Coba pilih lagi.', 'error');
    } finally {
      setLoadingId(null);
    }
  };

  return (
    <>
      <Button size={size} variant={variant} startIcon={<TextSnippetIcon fontSize="small" />}
        onClick={e => setAnchor(e.currentTarget)} sx={{ flexShrink: 0 }}>
        {label}
      </Button>
      <Menu anchorEl={anchor} open={!!anchor} onClose={() => setAnchor(null)}
        slotProps={{ paper: { sx: { maxWidth: 360, maxHeight: 360 } } }}>
        {(!templates || templates.length === 0) ? (
          <MenuItem disabled>
            <Typography variant="body2" color="text.secondary">Belum ada template. Buat di menu Template.</Typography>
          </MenuItem>
        ) : templates.map(t => (
          <MenuItem key={t.id} disabled={loadingId !== null} onClick={() => void pick(t)} sx={{ display: 'block', py: 1 }}>
            <ListItemText
              primary={(
                <Stack direction="row" spacing={1} sx={{ alignItems: 'center' }}>
                  <Typography sx={{ fontWeight: 600, flex: 1 }} noWrap>{t.title}</Typography>
                  {loadingId === t.id && <CircularProgress size={14} />}
                  {t.file_name && (
                    <Chip
                      size="small"
                      icon={<AttachFileIcon />}
                      label={supportsAttachment ? 'Ada lampiran' : 'Teks saja'}
                      variant="outlined"
                    />
                  )}
                </Stack>
              )}
              secondary={<Typography variant="caption" color="text.secondary" sx={{ display: '-webkit-box', WebkitLineClamp: 2, WebkitBoxOrient: 'vertical', overflow: 'hidden' }}>{t.body}</Typography>}
            />
          </MenuItem>
        ))}
      </Menu>
    </>
  );
});

export default TemplatePicker;
