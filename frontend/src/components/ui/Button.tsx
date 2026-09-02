import type { ButtonProps } from '@mui/material/Button';
import MuiButton from '@mui/material/Button';

export type RuangkirimButtonProps = ButtonProps;

/** Shared Phase 2 button primitive. */
export default function Button(props: RuangkirimButtonProps) {
  return <MuiButton disableElevation {...props} />;
}
