// An agent acting on a portal (squad portal click/fill/…): the canvas shows
// its cursor on the page and what it is doing on the wire, for a moment.
import { useEffect, useState } from 'react';
import { roleColor } from '../../lib/roles';
import { PORTAL_ACTION_MS, useSquad, type PortalAction } from '../../state/store';

const VERBS: Record<string, string> = {
  click: 'clicando',
  fill: 'preenchendo',
  type: 'digitando',
  key: 'teclando',
  scroll: 'rolando',
  goto: 'navegando',
  back: 'navegando',
  forward: 'navegando',
  reload: 'recarregando',
};

export function actionVerb(kind: string): string {
  return VERBS[kind] ?? kind;
}

/** The latest agent action on a portal while it is on screen, with the agent's role color key. */
export function usePortalAction(portal: string): { act: PortalAction; role: string } | null {
  const act = useSquad((s) => s.portalActions[portal]);
  const role = useSquad((s) => {
    const id = act && s.workspace?.nodes.find((n) => n.id === act.by)?.terminal?.role;
    return roleColor(id ? s.roles.find((r) => r.id === id) : undefined);
  });
  const [, expire] = useState(0);
  useEffect(() => {
    if (!act) return;
    const left = act.at + PORTAL_ACTION_MS - Date.now();
    if (left <= 0) return;
    const t = window.setTimeout(() => expire((x) => x + 1), left);
    return () => window.clearTimeout(t);
  }, [act]);
  return act && Date.now() - act.at < PORTAL_ACTION_MS ? { act, role } : null;
}
