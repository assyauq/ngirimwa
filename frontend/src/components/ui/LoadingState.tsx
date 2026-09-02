import { Box, CircularProgress, Typography } from '@mui/material';

type LoadingStateProps = { message?: string; minHeight?: number };

export default function LoadingState({ message = 'Memuat data...', minHeight = 180 }: LoadingStateProps) {
  return (
    <Box minHeight={minHeight} display="flex" flexDirection="column" alignItems="center" justifyContent="center" gap={2}>
      <CircularProgress size={28} />
      <Typography variant="body2" color="text.secondary">{message}</Typography>
    </Box>
  );
}
