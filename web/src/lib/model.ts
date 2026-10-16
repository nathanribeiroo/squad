// Labels, icons and tokens for states (docs/design_handoff_squad §2); roles: lib/roles.ts.
import type { UIState } from '../api/types';
import type { IconName } from './icons';

export const STATE_META: Record<UIState, { label: string; icon: IconName }> = {
  boot: { label: 'Iniciando', icon: 'progress_activity' },
  idle: { label: 'Ocioso', icon: 'radio_button_unchecked' },
  work: { label: 'Trabalhando', icon: 'autorenew' },
  wait: { label: 'Esperando resposta', icon: 'hourglass_top' },
  done: { label: 'Terminou', icon: 'check_circle' },
  err: { label: 'Parado · erro', icon: 'error' },
  off: { label: 'Descarregado', icon: 'bedtime' },
};

export function stateLabel(state: UIState, exitCode?: number): string {
  if (state === 'err' && exitCode && exitCode > 0) return `Parado · saída ${exitCode}`;
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

export const AGENTS: { value: string; label: string }[] = [
  { value: 'claude', label: 'Claude Code' },
  { value: 'codex', label: 'Codex' },
  { value: 'devin', label: 'Devin' },
  { value: 'gemini', label: 'Gemini' },
  { value: 'shell', label: 'Shell' },
  { value: 'custom', label: 'Comando próprio' },
];
