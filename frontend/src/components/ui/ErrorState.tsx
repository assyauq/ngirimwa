import { Alert, Box } from '@mui/material';
import type { ReactNode } from 'react';

type ErrorStateProps = {
  title?: string;
  message: string;
  action?: ReactNode;
};

export default function ErrorState({ title = 'Terjadi kesalahan', message, action }: ErrorStateProps) {
  return (
    <Box sx={{ display: "flex", flexDirection: "column", gap: 1.5 }}>
      <Alert severity="error">
        <strong>{title}</strong><br />
        {message}
      </Alert>
      {action}
    </Box>
  );
}
