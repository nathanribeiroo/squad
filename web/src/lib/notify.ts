// Avisos (Configurações → Avisos): what happens when a flight asks you. Per
// browser, in localStorage.

export interface NoticePrefs {
  /** The count in the window title: "(1) squad · Torre". */
  title: boolean;
  /** A system notification while the window isn't in front. */
  system: boolean;
  /** A short sound. */
  sound: boolean;
  /** The sound again while a flight keeps asking (every 2 min, 3 times). */
  repeat: boolean;
}

const KEY = 'torre.notices';
const DEFAULTS: NoticePrefs = { title: true, system: false, sound: true, repeat: false };

export function noticePrefs(): NoticePrefs {
  try {
    return { ...DEFAULTS, ...JSON.parse(localStorage.getItem(KEY) ?? '{}') };
  } catch {
    return DEFAULTS;
  }
}

/** Saves only what the user changed: an option never touched follows its default. */
export function setNoticePrefs(patch: Partial<NoticePrefs>) {
  try {
    const saved = JSON.parse(localStorage.getItem(KEY) ?? '{}');
    localStorage.setItem(KEY, JSON.stringify({ ...saved, ...patch }));
  } catch {
    /* storage may be unavailable */
  }
}

/** A flight started asking you: a notification and a sound, as chosen. */
export function noticeAsk(who: string, what: string) {
  const p = noticePrefs();
  if (p.system && typeof Notification === 'function' && Notification.permission === 'granted' && !document.hasFocus()) {
    try {
      const n = new Notification(`${who} pede você`, { body: what, tag: `torre-ask-${who}` });
      n.onclick = () => {
        window.focus();
        n.close();
      };
    } catch {
      /* some browsers only notify from a service worker */
    }
  }
  if (p.sound) beep();
}

const REPEAT_MS = 120_000;
const REPEATS = 3;
const reminders = new Map<string, { next: number; left: number }>();

/**
 * Repeats the sound while asks stay unanswered (Avisos → Repetir): each
 * key is one ask, still pending; at most one sound per call.
 */
export function remindAsks(keys: string[], now = Date.now()) {
  for (const k of [...reminders.keys()]) if (!keys.includes(k)) reminders.delete(k);
  let due = false;
  for (const k of keys) {
    const r = reminders.get(k);
    if (!r) reminders.set(k, { next: now + REPEAT_MS, left: REPEATS });
    else if (r.left > 0 && now >= r.next) {
      r.left--;
      r.next = now + REPEAT_MS;
      due = true;
    }
  }
  const p = noticePrefs();
  if (due && p.sound && p.repeat) beep();
}

// One audio context for the page. Browsers let it play only after the user
// touched the page (the app window that torre serve opens lets it play from
// the start): the first click or key prepares it, so a flight asking later
// is heard even with the window behind others.
let ctx: AudioContext | null = null;

function audio(): AudioContext | null {
  try {
    ctx ??= new AudioContext();
  } catch {
    return null;
  }
  if (ctx.state === 'suspended') void ctx.resume().catch(() => undefined);
  return ctx;
}

/** Prepares the sound at the first click or key on the page. */
export function unlockAudio() {
  const on = () => {
    audio();
    removeEventListener('pointerdown', on, true);
    removeEventListener('keydown', on, true);
  };
  addEventListener('pointerdown', on, true);
  addEventListener('keydown', on, true);
}

/** Two short tones (no audio file to ship). */
export function beep() {
  const ac = audio();
  if (!ac) return;
  try {
    const g = ac.createGain();
    g.gain.value = 0.06;
    g.connect(ac.destination);
    [880, 660].forEach((f, i) => {
      const o = ac.createOscillator();
      o.frequency.value = f;
      o.connect(g);
      o.start(ac.currentTime + i * 0.14);
      o.stop(ac.currentTime + i * 0.14 + 0.1);
    });
  } catch {
    /* no audio */
  }
}
