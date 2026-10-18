// Avisos (Configurações → Avisos): what happens when a flight asks you, or
// finishes a turn of yours and waits for you. Per browser, in localStorage.

export interface NoticePrefs {
  /** The count in the window title: "(1) squad · Torre". */
  title: boolean;
  /** A system notification while the window isn't in front. */
  system: boolean;
  /** A short sound. */
  sound: boolean;
  /** The sound again while a flight keeps asking (every 2 min, 3 times). */
  repeat: boolean;
  /**
   * A flight finished a turn of yours and waits (SUA VEZ): minutes before
   * the notice ("0": at once), or "off". Only while the window is behind.
   */
  turnAfter: 'off' | '0' | '1' | '3' | '10';
}

const KEY = 'torre.notices';
const DEFAULTS: NoticePrefs = { title: true, system: false, sound: true, repeat: false, turnAfter: '1' };

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

const toldTurns = new Set<string>();

/**
 * Tells of the flights waiting for you after a turn of yours (Avisos → SUA
 * VEZ), once per turn, after the chosen time, while the window is behind:
 * each entry is one turn (key), since when it waits, who and what it said.
 */
export function remindYourTurns(turns: { key: string; since: string; who: string; said: string }[], now = Date.now()) {
  for (const k of [...toldTurns]) if (!turns.some((t) => t.key === k)) toldTurns.delete(k);
  const p = noticePrefs();
  if (p.turnAfter === 'off') return;
  const after = Number(p.turnAfter) * 60_000;
  const due = turns.filter((t) => !toldTurns.has(t.key) && now - Date.parse(t.since) >= after);
  if (due.length === 0 || document.hasFocus()) return;
  for (const t of due) toldTurns.add(t.key);
  if (p.system && typeof Notification === 'function' && Notification.permission === 'granted') {
    for (const t of due) {
      try {
        const n = new Notification(`${t.who} terminou: sua vez`, { body: t.said || 'O voo espera você.', tag: `torre-turn-${t.key}` });
        n.onclick = () => {
          window.focus();
          n.close();
        };
      } catch {
        /* some browsers only notify from a service worker */
      }
    }
  }
  if (p.sound) tone();
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

/** One soft tone: a flight waits for you, not asking anything (SUA VEZ). */
export function tone() {
  const ac = audio();
  if (!ac) return;
  try {
    const g = ac.createGain();
    g.gain.value = 0.045;
    g.connect(ac.destination);
    const o = ac.createOscillator();
    o.frequency.value = 523;
    o.connect(g);
    o.start(ac.currentTime);
    o.stop(ac.currentTime + 0.16);
  } catch {
    /* no audio */
  }
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
