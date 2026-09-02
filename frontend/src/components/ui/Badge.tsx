import Chip, { type ChipProps } from '@mui/material/Chip';

export type RuangkirimBadgeProps = ChipProps;

/** Compact semantic status primitive. */
export default function Badge({ size = 'small', ...props }: RuangkirimBadgeProps) {
  return <Chip size={size} {...props} />;
}
