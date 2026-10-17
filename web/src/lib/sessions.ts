// Sessions (internal/hub sessions.go): texts for the panel and the selector,
// and the "agents are working" refusal that leaving a session can get.
import { ApiError } from '../api/client';
import type { SessionSummary } from '../api/types';

/** Why leaving a session waits for a confirmation (a 409 from the server). */
export interface Busy {
  working: string[];
  pending: number;
}

/** The working agents of a refused switch, or null for another error. */
export function busyOf(err: unknown): Busy | null {
  if (!(err instanceof ApiError) || err.status !== 409) return null;
  const working = Array.isArray(err.body.working) ? (err.body.working as string[]) : [];
  const pending = typeof err.body.pending === 'number' ? err.body.pending : 0;
  return { working, pending };
}

function list(names: string[]): string {
  try {
    return new Intl.ListFormat('pt-BR', { style: 'long', type: 'conjunction' }).format(names);
  } catch {
    return names.join(', ');
  }
}

/** "Planejador e Revisor estão trabalhando, e 1 pedido entre agentes está em andamento." */
export function busyText(b: Busy): string {
  const parts: string[] = [];
  if (b.working.length) parts.push(`${list(b.working)} ${b.working.length === 1 ? 'está trabalhando' : 'estão trabalhando'}`);
  if (b.pending) parts.push(b.pending === 1 ? '1 pedido entre agentes está em andamento' : `${b.pending} pedidos entre agentes estão em andamento`);
  return parts.join(', e ') + '.';
}

function pad(n: number): string {
  return String(n).padStart(2, '0');
}

/** "17/10 14:32" (with the year when it isn't this one). */
export function when(t: string): string {
  const d = new Date(t);
  if (!Number.isFinite(d.getTime())) return '';
  const year = d.getFullYear() === new Date().getFullYear() ? '' : `/${d.getFullYear()}`;
  return `${pad(d.getDate())}/${pad(d.getMonth() + 1)}${year} ${pad(d.getHours())}:${pad(d.getMinutes())}`;
}

/** "agora", "há 5 min", "há 2 h", "ontem", "há 3 dias", or the date. */
export function ago(t: string, now = Date.now()): string {
  const ms = now - Date.parse(t);
  if (!Number.isFinite(ms)) return '';
  const min = Math.floor(ms / 60_000);
  if (min < 1) return 'agora';
  if (min < 60) return `há ${min} min`;
  const h = Math.floor(min / 60);
  if (h < 24) return `há ${h} h`;
  const days = Math.floor(h / 24);
  if (days === 1) return 'ontem';
  if (days < 7) return `há ${days} dias`;
  return when(t).split(' ')[0];
}

function count(n: number, one: string, many: string): string {
  return `${n} ${n === 1 ? one : many}`;
}

/** "3 terminais · 2 notas · 1 portal · 1 andar", or "vazia". */
export function contents(s: SessionSummary): string {
  const parts = [];
  if (s.terminals) parts.push(count(s.terminals, 'terminal', 'terminais'));
  if (s.notes) parts.push(count(s.notes, 'nota', 'notas'));
  if (s.portals) parts.push(count(s.portals, 'portal', 'portais'));
  if (s.floors) parts.push(count(s.floors, 'andar', 'andares'));
  return parts.length ? parts.join(' · ') : 'vazia';
}
