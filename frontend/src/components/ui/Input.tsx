import TextField, { type TextFieldProps } from '@mui/material/TextField';

export type RuangkirimInputProps = TextFieldProps;

/** Shared form input primitive. */
export default function Input({ fullWidth = true, size = 'small', ...props }: RuangkirimInputProps) {
  return <TextField fullWidth={fullWidth} size={size} {...props} />;
}
