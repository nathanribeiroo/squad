// Labels, icons and tokens for states (docs/torre/RENOMEACAO.md §2, "Estados"); roles: lib/roles.ts.
import type { UIState } from '../api/types';
import type { IconName } from './icons';

export const STATE_META: Record<UIState, { label: string; icon: IconName }> = {
  boot: { label: 'Taxiando', icon: 'progress_activity' },
  idle: { label: 'Pátio', icon: 'radio_button_unchecked' },
  work: { label: 'No ar', icon: 'flight' },
  wait: { label: 'Aguarda', icon: 'hourglass_top' },
  ask: { label: 'Pede você', icon: 'front_hand' },
  done: { label: 'Em solo', icon: 'flight_land' },
  err: { label: 'Pane', icon: 'error' },
  off: { label: 'Hangar', icon: 'bedtime' },
};

export function stateLabel(state: UIState, exitCode?: number): string {
  if (state === 'err' && exitCode && exitCode > 0) return `Pane · saída ${exitCode}`;
  return STATE_META[state].label;
}

/** Elapsed time like 0:42, 1:58 or 1h02. */
export function elapsed(since: string | number | undefined, now = Date.now()): string {
  if (!since) return '';
  const t = typeof since === 'number' ? since : Date.parse(since);
  if (!Number.isFinite(t) || t <= 0) return '';
  let s = Math.max(0, Math.floor((now - t) / 1000));
  const h = Math.floor(s / 3600);
  s -= h * 3600;
  const m = Math.floor(s / 60);
  s -= m * 60;
  if (h > 0) return `${h}h${String(m).padStart(2, '0')}`;
  return `${m}:${String(s).padStart(2, '0')}`;
}

export function clock(t: string | number): string {
  const d = new Date(t);
  return `${String(d.getHours()).padStart(2, '0')}:${String(d.getMinutes()).padStart(2, '0')}`;
}

/** The time, with the day when it isn't today ("17/10 14:32"): the Rádio keeps days of history. */
export function clockDay(t: string | number): string {
  const d = new Date(t);
  if (d.toDateString() === new Date().toDateString()) return clock(t);
  return `${String(d.getDate()).padStart(2, '0')}/${String(d.getMonth() + 1).padStart(2, '0')} ${clock(t)}`;
}

export const AGENTS: { value: string; label: string }[] = [
  { value: 'claude', label: 'Claude Code' },
  { value: 'codex', label: 'Codex' },
  { value: 'devin', label: 'Devin' },
  { value: 'gemini', label: 'Gemini' },
  { value: 'shell', label: 'Shell' },
  { value: 'custom', label: 'Comando próprio' },
];

/** A flight's callsign: its own, else three letters of its name. */
export function callsign(t: { name: string; callsign?: string }): string {
  if (t.callsign) return t.callsign;
  const letters = t.name.normalize('NFD').replace(/[^A-Za-z0-9]/g, '').toUpperCase();
  const cons = letters.replace(/[AEIOU0-9]/g, '');
  return (cons + letters.replace(/[^AEIOU0-9]/g, '')).slice(0, 3) || 'VOO';
}
