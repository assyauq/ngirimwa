import type { CardProps } from '@mui/material/Card';
import MuiCard from '@mui/material/Card';

export type RuangkirimCardProps = CardProps;

/** Shared surface primitive. Prefer variant="outlined" for data-heavy content. */
export default function Card({ variant = 'outlined', ...props }: RuangkirimCardProps) {
  return <MuiCard variant={variant} {...props} />;
}
