import { Box, Typography } from '@mui/material';
import type { ReactNode } from 'react';

type EmptyStateProps = {
  title: string;
  description?: string;
  action?: ReactNode;
  icon?: ReactNode;
  minHeight?: number;
};

export default function EmptyState({ title, description, action, icon, minHeight = 220 }: EmptyStateProps) {
  return (
    <Box sx={{ minHeight, display: "flex", flexDirection: "column", alignItems: "center", justifyContent: "center", textAlign: "center", gap: 1.5, px: 3 }}>
      {icon && <Box color="text.secondary">{icon}</Box>}
      <Typography variant="h6">{title}</Typography>
      {description && <Typography variant="body2" color="text.secondary" sx={{ maxWidth: 420 }}>{description}</Typography>}
      {action && <Box sx={{ mt: 0.5 }}>{action}</Box>}
    </Box>
  );
}
