import Swal from 'sweetalert2';

type ToastIcon = 'success' | 'error' | 'warning' | 'info';

const LIGHTWEIGHT_TOAST_HOST_ID = 'kirimwa-toast-host';
const toastTone: Record<ToastIcon, { accent: string; background: string; glyph: string }> = {
  success: { accent: '#008069', background: '#f2fbf7', glyph: '✓' },
  error: { accent: '#c62828', background: '#fff6f6', glyph: '!' },
  warning: { accent: '#b45309', background: '#fffaf0', glyph: '!' },
  info: { accent: '#2563a6', background: '#f4f8ff', glyph: 'i' },
};

const swal = Swal.mixin({
  confirmButtonColor: '#25D366',
  cancelButtonColor: '#757575',
  denyButtonColor: '#e53935',
});

/** Konfirmasi ya/tidak. Return true kalau user klik Ya. */
export async function swalConfirm(title: string, text?: string): Promise<boolean> {
  const result = await swal.fire({
    title,
    text,
    icon: 'question',
    showCancelButton: true,
    confirmButtonText: 'Ya',
    cancelButtonText: 'Batal',
  });
  return result.isConfirmed;
}

/** Prompt input teks. Return string atau null kalau batal. */
export async function swalPrompt(title: string, placeholder?: string): Promise<string | null> {
  const result = await swal.fire({
    title,
    input: 'text',
    inputPlaceholder: placeholder,
    showCancelButton: true,
    confirmButtonText: 'OK',
    cancelButtonText: 'Batal',
  });
  return result.isConfirmed ? (result.value || '') : null;
}

/** Alert info/error. `text` opsional untuk detail di bawah judul. */
export async function swalAlert(title: string, icon: 'success' | 'error' | 'warning' | 'info' = 'info', text?: string) {
  await swal.fire({ title, text, icon, confirmButtonText: 'OK' });
}

function removeLegacyToastArtifacts() {
  // SweetAlert toast lama memasang class global `swal2-shown` pada body.
  // Safari dapat menahan timer animasinya saat tab berpindah sehingga body dan
  // container full-screen tertinggal. Bersihkan hanya artefak toast; dialog
  // konfirmasi/prompt yang memang modal tidak disentuh.
  document.querySelectorAll<HTMLElement>('.swal2-container').forEach((container) => {
    if (container.querySelector('.swal2-toast')) container.remove();
  });
  document.body.classList.remove('swal2-toast-shown');
  if (!document.querySelector('.swal2-container')) {
    document.body.classList.remove('swal2-shown');
  }
}

function lightweightToastHost() {
  const current = document.getElementById(LIGHTWEIGHT_TOAST_HOST_ID);
  if (current) return current;
  const host = document.createElement('div');
  host.id = LIGHTWEIGHT_TOAST_HOST_ID;
  host.setAttribute('aria-live', 'polite');
  Object.assign(host.style, {
    position: 'fixed',
    top: '16px',
    right: '16px',
    zIndex: '20000',
    display: 'flex',
    flexDirection: 'column',
    alignItems: 'flex-end',
    gap: '8px',
    maxWidth: 'calc(100vw - 24px)',
    pointerEvents: 'none',
  });
  document.body.appendChild(host);
  return host;
}

/**
 * Toast non-modal. Tidak membuat backdrop/portal framework, tidak memindahkan
 * fokus, dan tidak mengubah overflow/class body sehingga aman untuk composer.
 */
export function swalToast(title: string, icon: ToastIcon = 'success') {
  if (typeof document === 'undefined') return;
  removeLegacyToastArtifacts();
  const host = lightweightToastHost();
  const tone = toastTone[icon];
  const item = document.createElement('div');
  item.setAttribute('role', icon === 'error' ? 'alert' : 'status');
  Object.assign(item.style, {
    display: 'grid',
    gridTemplateColumns: '24px minmax(0, 1fr)',
    alignItems: 'start',
    gap: '9px',
    width: 'min(380px, calc(100vw - 24px))',
    padding: '11px 13px',
    border: `1px solid ${tone.accent}33`,
    borderLeft: `4px solid ${tone.accent}`,
    borderRadius: '8px',
    background: tone.background,
    color: '#243138',
    boxShadow: '0 8px 24px rgba(15, 35, 45, .16)',
    fontFamily: 'inherit',
    fontSize: '13px',
    fontWeight: '650',
    lineHeight: '1.38',
    overflowWrap: 'anywhere',
    opacity: '0',
    transform: 'translateY(-6px)',
    transition: 'opacity 140ms ease, transform 140ms ease',
    pointerEvents: 'none',
  });

  const glyph = document.createElement('span');
  glyph.textContent = tone.glyph;
  Object.assign(glyph.style, {
    display: 'grid',
    placeItems: 'center',
    width: '22px',
    height: '22px',
    borderRadius: '50%',
    background: tone.accent,
    color: '#fff',
    fontSize: '13px',
    fontWeight: '800',
  });
  const message = document.createElement('span');
  message.textContent = title;
  item.append(glyph, message);
  host.appendChild(item);

  while (host.childElementCount > 4) host.firstElementChild?.remove();
  window.requestAnimationFrame(() => {
    item.style.opacity = '1';
    item.style.transform = 'translateY(0)';
  });

  const lifetime = icon === 'error' || icon === 'warning' ? 4200 : 2800;
  window.setTimeout(() => {
    item.style.opacity = '0';
    item.style.transform = 'translateY(-4px)';
    window.setTimeout(() => {
      item.remove();
      if (host.childElementCount === 0) host.remove();
    }, 180);
  }, lifetime);
}

if (typeof document !== 'undefined') queueMicrotask(removeLegacyToastArtifacts);
