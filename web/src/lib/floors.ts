// Floors: isolated copies of the workspace's repository (git worktrees on
// their own branch), each with its own canvas (internal/floors).
import type { CheckKind, Floor, FloorGit, Workspace } from '../api/types';
import { ago } from './sessions';
import { useSquad } from '../state/store';

/** Name of the ground floor (the runway) when its branch isn't known. */
export const GROUND = 'pista';

const NO_FLOORS: Floor[] = [];

/** The workspace's floors, in creation order. */
export function useFloors(): Floor[] {
  return useSquad((s) => s.workspace?.floors ?? NO_FLOORS);
}

/** The floor on screen (undefined on the ground floor). */
export function useActiveFloor(): Floor | undefined {
  return useSquad((s) => (s.activeFloor ? s.workspace?.floors?.find((f) => f.id === s.activeFloor) : undefined));
}

/** The folder a floor's terminals start from ("" the ground floor): the
 * floor's copy, else the session's copy, else the workspace folder. */
export function workdir(ws: Workspace | null, floor?: string): string | undefined {
  if (floor) return ws?.floors?.find((f) => f.id === floor)?.cwd;
  return ws?.copy?.cwd ?? ws?.cwd;
}

/** A floor's name, "pista" for the ground floor ("" or unknown). */
export function floorName(floors: Floor[] | undefined, id?: string): string {
  return (id && floors?.find((f) => f.id === id)?.name) || GROUND;
}

/**
 * The branch a floor ("" the ground floor) is on: from the polled git status,
 * else the floor's own; "" when the workspace isn't a git repository.
 */
export function useBranch(floor: string): string {
  return useSquad((s) => {
    const st = s.floorStatus;
    if (!floor) return st?.repo ? st.ground.branch || 'HEAD' : '';
    return st?.floors[floor]?.branch || s.workspace?.floors?.find((f) => f.id === floor)?.branch || '';
  });
}

/** What a floor has that isn't landed: "limpo", "3 alt.", "2 commits". */
export function gitSummary(g: FloorGit | undefined): { text: string; dirty: boolean } | null {
  if (!g) return null;
  if (g.missing) return { text: 'pasta sumiu', dirty: true };
  if (g.changes > 0) return { text: `${g.changes} alt.`, dirty: true };
  if (g.ahead) return { text: `${g.ahead} ${g.ahead === 1 ? 'commit' : 'commits'}`, dirty: false };
  return { text: 'limpo', dirty: false };
}

/** The branch a new floor gets from its name, as the server derives it
 * (accents folded, ASCII letters and digits joined by "-"). */
export function slugify(name: string): string {
  return name
    .normalize('NFD')
    .replace(/[̀-ͯ]/g, '')
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-+|-+$/g, '');
}

/** "3 alterações", "1 commit"… */
export function plural(n: number, one: string, many: string): string {
  return `${n} ${n === 1 ? one : many}`;
}

/** "FL2" for a floor, "PISTA" for the ground floor. */
export function flightLevel(f?: Floor): string {
  return f ? `FL${f.number || '?'}` : 'PISTA';
}

export const CHECK_NAMES: Record<CheckKind, string> = { tests: 'testes', review: 'revisão' };

/** A floor's tests or review as the landing queue shows it. */
export interface CheckView {
  kind: CheckKind;
  /** stale: marked on an older commit than the floor's latest. */
  state: 'pass' | 'fail' | 'stale' | 'none';
  label: string;
  /** Who marked it, the note, when and on which commit. */
  detail: string;
}

/** The floor's checks, against its latest commit (head; unknown: as marked). */
export function checkViews(f: Floor, head?: string): CheckView[] {
  return (['tests', 'review'] as CheckKind[]).map((kind) => {
    const c = f.checks?.[kind];
    const name = CHECK_NAMES[kind];
    if (!c) return { kind, state: 'none', label: `○ ${name}`, detail: 'sem marca' };
    const stale = !!head && c.commit !== head;
    const state = stale ? 'stale' : c.result;
    const mark = stale ? '⟳' : c.result === 'pass' ? '✓' : '✕';
    const who = c.callsign || c.byName;
    const detail = [
      `${who} ${c.result === 'pass' ? 'passou' : 'falhou'}${c.note ? `: ${c.note}` : ''}`,
      `${ago(c.at)} · commit ${c.commit.slice(0, 7)}`,
      stale ? 'chegou commit depois: marcar de novo' : '',
    ]
      .filter(Boolean)
      .join(' · ');
    return { kind, state, label: `${mark} ${name}`, detail };
  });
}
