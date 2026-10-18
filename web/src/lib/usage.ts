// Consumo: numbers and labels for what the flights used and the plan limits.
import type { UsageAccount, UsageWindow } from '../api/types';

/** Agents whose usage Torre reads. */
export const USAGE_AGENTS = ['claude', 'devin', 'codex', 'gemini'];

/** "820", "44,1k", "312k", "1,2M": token counts, short. */
export function tokens(n: number): string {
  if (n < 1000) return String(n);
  const [v, unit] = n < 1e6 ? [n / 1e3, 'k'] : [n / 1e6, 'M'];
  const s = v < 100 ? v.toFixed(1).replace(/\.0$/, '') : Math.round(v).toString();
  return s.replace('.', ',') + unit;
}

/** "US$ 0,42", "< US$ 0,01". */
export function usd(n: number): string {
  if (n > 0 && n < 0.01) return '< US$ 0,01';
  return `US$ ${n.toFixed(2).replace('.', ',')}`;
}

/** "3,2 ACU", "0,25 ACU". */
export function acus(n: number): string {
  return `${(n < 1 ? n.toFixed(2) : n.toFixed(1)).replace('.', ',')} ACU`;
}

export function percent(n: number): string {
  return `${Math.round(n)}%`;
}

const WINDOW_LABEL: Record<UsageWindow['kind'], string> = {
  five_hour: 'Sessão de 5 h',
  seven_day: 'Semana',
  daily: 'Cota diária',
  weekly: 'Cota semanal',
};

const WINDOW_SHORT: Record<UsageWindow['kind'], string> = { five_hour: '5H', seven_day: 'SEM', daily: 'DIA', weekly: 'SEM' };

export function windowLabel(w: UsageWindow): string {
  return WINDOW_LABEL[w.kind] ?? w.kind;
}

export function windowShort(w: UsageWindow): string {
  return WINDOW_SHORT[w.kind] ?? '';
}

/** The window renewed since it was seen: its share is no longer known. */
export function renewed(w: UsageWindow, now: number): boolean {
  return !!w.resetsAt && Date.parse(w.resetsAt) <= now;
}

/** "renova às 17:40", "renova sex. 09:00"; "renovou às 17:40" once past. */
export function resets(w: UsageWindow, now: number): string {
  if (!w.resetsAt) return '';
  const t = Date.parse(w.resetsAt);
  if (!Number.isFinite(t)) return '';
  const d = new Date(t);
  const hm = `${String(d.getHours()).padStart(2, '0')}:${String(d.getMinutes()).padStart(2, '0')}`;
  if (t <= now) return `renovou às ${hm}`;
  if (t - now < 20 * 3600_000) return `renova às ${hm}`;
  return `renova ${d.toLocaleDateString('pt-BR', { weekday: 'short' })} ${hm}`;
}

/** warn from 80%, crit from 95%. */
export function level(used: number): '' | 'warn' | 'crit' {
  return used >= 95 ? 'crit' : used >= 80 ? 'warn' : '';
}

/** The fullest window still current among the accounts (the top bar shows it). */
export function tightest(accounts: UsageAccount[], now: number): UsageWindow | null {
  let best: UsageWindow | null = null;
  for (const a of accounts) {
    for (const w of a.windows ?? []) {
      if (renewed(w, now)) continue;
      if (!best || w.used > best.used) best = w;
    }
  }
  return best;
}
