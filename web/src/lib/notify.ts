// Avisos (Configurações → Avisos): what happens when a flight asks you. Per
// browser, in localStorage.

export interface NoticePrefs {
  /** The count in the window title: "(1) squad · Torre". */
  title: boolean;
  /** A system notification while the window isn't in front. */
  system: boolean;
  /** A short sound. */
  sound: boolean;
}

const KEY = 'torre.notices';
const DEFAULTS: NoticePrefs = { title: true, system: false, sound: false };

export function noticePrefs(): NoticePrefs {
  try {
    return { ...DEFAULTS, ...JSON.parse(localStorage.getItem(KEY) ?? '{}') };
  } catch {
    return DEFAULTS;
  }
}

export function setNoticePrefs(p: NoticePrefs) {
  try {
    localStorage.setItem(KEY, JSON.stringify(p));
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

/** Two short tones (no audio file to ship). */
export function beep() {
  try {
    const ctx = new AudioContext();
    const g = ctx.createGain();
    g.gain.value = 0.06;
    g.connect(ctx.destination);
    [880, 660].forEach((f, i) => {
      const o = ctx.createOscillator();
      o.frequency.value = f;
      o.connect(g);
      o.start(ctx.currentTime + i * 0.14);
      o.stop(ctx.currentTime + i * 0.14 + 0.1);
    });
    setTimeout(() => void ctx.close(), 600);
  } catch {
    /* no audio */
  }
}
