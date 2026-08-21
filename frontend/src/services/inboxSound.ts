type AudioContextConstructor = new () => AudioContext;

let audioContext: AudioContext | null = null;
let audioPrimed = false;
let nextSoundAt = 0;
let htmlSoundURL = '';
let htmlUnlockAudio: HTMLAudioElement | null = null;
let nextHtmlSoundAt = 0;
const activeHtmlSounds = new Set<HTMLAudioElement>();

function createInboxWaveURL(): string {
  if (htmlSoundURL || typeof window === 'undefined') return htmlSoundURL;

  const sampleRate = 22_050;
  const duration = 0.34;
  const sampleCount = Math.ceil(sampleRate * duration);
  const buffer = new ArrayBuffer(44 + sampleCount * 2);
  const view = new DataView(buffer);
  const writeASCII = (offset: number, value: string) => {
    for (let index = 0; index < value.length; index += 1) {
      view.setUint8(offset + index, value.charCodeAt(index));
    }
  };

  writeASCII(0, 'RIFF');
  view.setUint32(4, 36 + sampleCount * 2, true);
  writeASCII(8, 'WAVE');
  writeASCII(12, 'fmt ');
  view.setUint32(16, 16, true);
  view.setUint16(20, 1, true);
  view.setUint16(22, 1, true);
  view.setUint32(24, sampleRate, true);
  view.setUint32(28, sampleRate * 2, true);
  view.setUint16(32, 2, true);
  view.setUint16(34, 16, true);
  writeASCII(36, 'data');
  view.setUint32(40, sampleCount * 2, true);

  for (let index = 0; index < sampleCount; index += 1) {
    const time = index / sampleRate;
    const firstLocal = time;
    const secondLocal = time - 0.13;
    const firstEnvelope = firstLocal >= 0 && firstLocal <= 0.15
      ? Math.min(1, firstLocal / 0.018, (0.15 - firstLocal) / 0.035)
      : 0;
    const secondEnvelope = secondLocal >= 0 && secondLocal <= 0.19
      ? Math.min(1, secondLocal / 0.018, (0.19 - secondLocal) / 0.045)
      : 0;
    const sample = (
      Math.sin(2 * Math.PI * 660 * firstLocal) * firstEnvelope
      + Math.sin(2 * Math.PI * 880 * secondLocal) * secondEnvelope
    ) * 0.24;
    view.setInt16(44 + index * 2, Math.max(-1, Math.min(1, sample)) * 0x7fff, true);
  }

  htmlSoundURL = URL.createObjectURL(new Blob([buffer], { type: 'audio/wav' }));
  return htmlSoundURL;
}

function createHTMLSound(volume: number): HTMLAudioElement | null {
  const url = createInboxWaveURL();
  if (!url || typeof Audio === 'undefined') return null;
  const audio = new Audio(url);
  audio.preload = 'auto';
  audio.volume = volume;
  return audio;
}

async function unlockHTMLSound(): Promise<boolean> {
  if (!htmlUnlockAudio) htmlUnlockAudio = createHTMLSound(0.0001);
  if (!htmlUnlockAudio) return false;
  try {
    htmlUnlockAudio.currentTime = 0;
    await htmlUnlockAudio.play();
    htmlUnlockAudio.pause();
    htmlUnlockAudio.currentTime = 0;
    return true;
  } catch {
    return false;
  }
}

async function playHTMLSound(): Promise<boolean> {
  const now = window.performance.now();
  const scheduledAt = Math.max(now, nextHtmlSoundAt);
  nextHtmlSoundAt = scheduledAt + 360;
  if (scheduledAt > now) {
    await new Promise<void>((resolve) => {
      window.setTimeout(resolve, scheduledAt - now);
    });
  }

  const audio = createHTMLSound(0.82);
  if (!audio) return false;
  const cleanup = () => {
    activeHtmlSounds.delete(audio);
    audio.removeEventListener('ended', cleanup);
    audio.removeEventListener('error', cleanup);
  };
  activeHtmlSounds.add(audio);
  audio.addEventListener('ended', cleanup, { once: true });
  audio.addEventListener('error', cleanup, { once: true });
  try {
    await audio.play();
    return true;
  } catch {
    cleanup();
    return false;
  }
}

function getAudioContext(): AudioContext | null {
  if (typeof window === 'undefined') return null;
  if (audioContext?.state === 'closed') {
    audioContext = null;
    audioPrimed = false;
    nextSoundAt = 0;
  }
  if (audioContext) return audioContext;

  const audioWindow = window as Window & {
    webkitAudioContext?: AudioContextConstructor;
  };
  const Context = window.AudioContext || audioWindow.webkitAudioContext;
  if (!Context) return null;

  audioContext = new Context();
  return audioContext;
}

async function resumeAudioContext(context: AudioContext): Promise<boolean> {
  if (context.state === 'running') return true;
  if (context.state === 'closed') return false;
  try {
    await context.resume();
  } catch {
    return false;
  }
  // State dapat berubah secara asinkron setelah resume(); String mencegah
  // TypeScript mempertahankan narrowing state sebelum await.
  return String(context.state) === 'running';
}

function primeAudioContext(context: AudioContext): void {
  if (audioPrimed) return;

  // Jadwalkan source saat masih berada di dalam gesture pengguna. Source akan
  // mulai ketika context berhasil di-resume, termasuk pada Safari.
  try {
    const oscillator = context.createOscillator();
    const gain = context.createGain();
    gain.gain.value = 0.0001;
    oscillator.connect(gain);
    gain.connect(context.destination);
    oscillator.start();
    oscillator.stop(context.currentTime + 0.01);
    oscillator.addEventListener('ended', () => {
      oscillator.disconnect();
      gain.disconnect();
    }, { once: true });
    audioPrimed = true;
  } catch {
    audioPrimed = false;
  }
}

export async function unlockInboxSound(): Promise<boolean> {
  const context = getAudioContext();
  const htmlReady = unlockHTMLSound();
  const webAudioReady = context
    ? (primeAudioContext(context), resumeAudioContext(context))
    : Promise.resolve(false);
  const [htmlAllowed, webAudioAllowed] = await Promise.all([htmlReady, webAudioReady]);
  return htmlAllowed || webAudioAllowed;
}

export async function playInboxSound(): Promise<boolean> {
  if (await playHTMLSound()) return true;

  const context = getAudioContext();
  if (!context) return false;
  if (!await resumeAudioContext(context)) return false;

  // Setiap pesan mendapat bunyi sendiri. Jika pesan datang beruntun, jadwalkan
  // nada berikutnya setelah nada sebelumnya alih-alih membuang event kedua.
  const start = Math.max(context.currentTime + 0.01, nextSoundAt);
  nextSoundAt = start + 0.36;
  const gain = context.createGain();
  gain.gain.setValueAtTime(0.0001, start);
  gain.gain.exponentialRampToValueAtTime(0.24, start + 0.018);
  gain.gain.exponentialRampToValueAtTime(0.0001, start + 0.32);
  gain.connect(context.destination);

  const first = context.createOscillator();
  first.type = 'sine';
  first.frequency.setValueAtTime(660, start);
  first.connect(gain);
  first.start(start);
  first.stop(start + 0.15);

  const second = context.createOscillator();
  second.type = 'sine';
  second.frequency.setValueAtTime(880, start + 0.13);
  second.connect(gain);
  second.start(start + 0.13);
  second.stop(start + 0.32);

  second.addEventListener('ended', () => gain.disconnect(), { once: true });
  return true;
}
