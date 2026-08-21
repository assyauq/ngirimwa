import { Box, Typography } from '@mui/material';

// Header ringkas untuk halaman dashboard — hierarki tipografi konsisten.
export default function PageHeader({ title, subtitle, action, dense = false }: {
  title: React.ReactNode;
  subtitle?: React.ReactNode;
  action?: React.ReactNode;
  dense?: boolean;
}) {
  return (
    <Box
      sx={{
        display: 'flex',
        flexDirection: { xs: 'column', sm: 'row' },
        alignItems: { xs: 'stretch', sm: 'flex-start' },
        justifyContent: 'space-between',
        gap: { xs: 1.25, sm: 2 },
        mb: dense ? 1.25 : 2.25,
        pb: 0.25,
      }}
    >
      <Box sx={{ minWidth: 0 }}>
        <Typography
          variant="h5"
          sx={{
            fontWeight: 600,
            fontSize: { xs: 18, md: 20 },
            letterSpacing: '-0.01em',
            lineHeight: 1.3,
            color: 'text.primary',
          }}
        >
          {title}
        </Typography>
        {subtitle && (
          <Typography
            variant="body2"
            color="text.secondary"
            sx={{ mt: 0.5, maxWidth: 640, lineHeight: 1.5 }}
          >
            {subtitle}
          </Typography>
        )}
      </Box>
      {action && (
        <Box sx={{ flexShrink: 0, width: { xs: '100%', sm: 'auto' }, display: 'flex', justifyContent: { xs: 'flex-start', sm: 'flex-end' }, gap: 1 }}>
          {action}
        </Box>
      )}
    </Box>
  );
}
