// The role library (internal/roles): colors and icons a role may take, and
// lookups for the canvas. Terminals refer to roles by id.
import { useMemo } from 'react';
import { api } from '../api/client';
import type { RoleDef, TerminalData } from '../api/types';
import { useSquad } from '../state/store';
import type { IconName } from './icons';
import { toast } from './toast';

/** Same lists as internal/roles (the server refuses others). */
export const ROLE_COLORS = ['plan', 'impl', 'rev', 'test', 'teal', 'indigo', 'magenta', 'orange'] as const;
export const ROLE_ICONS: IconName[] = [
  'account_tree',
  'code',
  'rate_review',
  'science',
  'bug_report',
  'palette',
  'description',
  'shield',
  'database',
  'rocket_launch',
  'psychology',
  'build',
];
export const MAX_ROLE_NAME = 40;
export const MAX_ROLE_PROMPT = 4000;

/** Agent types that receive their role's instructions (the others only show it). */
export const ROLE_AGENTS = new Set(['claude', 'codex', 'devin', 'gemini']);

/** CSS color key of a role (--r-<key>), 'none' without one. */
export function roleColor(r?: RoleDef): string {
  return r?.color ?? 'none';
}

export function roleIcon(r: RoleDef): IconName {
  return (ROLE_ICONS as string[]).includes(r.icon) ? (r.icon as IconName) : 'psychology';
}

/** The role of a terminal by its role id (undefined: none, or removed). */
export function useRole(id?: string): RoleDef | undefined {
  return useSquad((s) => (id ? s.roles.find((r) => r.id === id) : undefined));
}

/** A lookup from role id to role, for lists. */
export function useRoleLookup(): (id?: string) => RoleDef | undefined {
  const roles = useSquad((s) => s.roles);
  return useMemo(() => {
    const byId = new Map(roles.map((r) => [r.id, r]));
    return (id?: string) => (id ? byId.get(id) : undefined);
  }, [roles]);
}

/** The color key of a terminal node's role, by node id. */
export function useNodeRoleColor(): (nodeId?: string) => string {
  const nodes = useSquad((s) => s.workspace?.nodes);
  const roleOf = useRoleLookup();
  return (nodeId?: string) => roleColor(roleOf(nodes?.find((n) => n.id === nodeId)?.terminal?.role));
}

/**
 * Gives a terminal another role (roleId '' takes it away). A running agent
 * that receives instructions restarts to get them, after confirming.
 */
export async function changeRole(nodeId: string, t: TerminalData, roleId: string, running: boolean): Promise<void> {
  const restart = running && ROLE_AGENTS.has(t.agent);
  if (restart) {
    const name = useSquad.getState().roles.find((r) => r.id === roleId)?.name;
    const what = roleId ? `Dar o papel "${name}" a ${t.name}?` : `Tirar o papel de ${t.name}?`;
    if (!window.confirm(`${what} O agente reinicia para receber as instruções, e a sessão atual é encerrada.`)) return;
  }
  try {
    await api.updateNode(nodeId, { terminal: { ...t, role: roleId || undefined } });
    if (restart) await api.nodeAction(nodeId, 'restart');
  } catch (e) {
    toast((e as Error).message);
  }
}
