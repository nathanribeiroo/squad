import { useEffect, useRef } from 'react';
import { useReactFlow } from '@xyflow/react';
import { hub } from '../api/client';
import type { WNode } from '../api/types';
import { useSquad } from '../state/store';
import { Icon } from './Icon';
import './NoLink.css';
import './WakeOverlay.css';

const AGENTS = new Set(['claude', 'codex', 'devin', 'gemini']);

/**
 * A workspace coming back on screen starts its agents a few at a time
 * (internal/hub launch.go), each continuing its conversation: the canvas
 * shows underneath, with the flights' last screens, as with "Sem enlace".
 * The top bar stays usable (another workspace can be picked meanwhile).
 */
export function WakeOverlay() {
  const ws = useSquad((s) => s.workspaces.find((w) => w.id === s.activeWs));
  const linkDown = useSquad((s) => s.connection === 'reconnecting');
  useWakeOrder(ws?.id, ws?.state === 'loading' || ws?.state === 'inactive');
  if (!ws || linkDown || (ws.state !== 'loading' && ws.state !== 'inactive')) return null;
  const { ready = 0, total = 0 } = ws.loading ?? {};
  return (
    <div className="nolink wake" role="status" aria-live="polite">
      <div className="nolink-card wake-card">
        <Icon name="progress_activity" size={20} className="spin" />
        <div>
          <strong>Carregando o workspace</strong>
          <span>{total ? `${ready} de ${total} ${total === 1 ? 'voo de volta' : 'voos de volta'}` : 'Os voos estão voltando…'}</span>
          {total > 0 && (
            <span className="wake-bar" aria-hidden="true">
              <span style={{ transform: `scaleX(${ready / total})` }} />
            </span>
          )}
          <span className="nolink-sub">Cada voo continua a sua conversa. Eles voltam poucos de cada vez, para não travar o computador.</span>
        </div>
      </div>
    </div>
  );
}

/** While the workspace on screen starts its agents, tells the server which
 * go first: the flight in focus or docked, then those in view, nearest the
 * middle first. Once per workspace and session. */
function useWakeOrder(ws: string | undefined, waking: boolean) {
  const rf = useReactFlow();
  const sent = useRef('');
  const session = useSquad((s) => s.activeSession);
  const loaded = useSquad((s) => !!s.workspace);
  useEffect(() => {
    const key = `${ws}:${session}`;
    if (!ws || !waking || !loaded || sent.current === key) return;
    sent.current = key;
    const st = useSquad.getState();
    const nodes = (st.workspace?.nodes ?? []).filter((n) => n.terminal && AGENTS.has(n.terminal.agent));
    const first: string[] = [];
    for (const id of [st.focusPane, st.dock?.id, st.focusedTerminal]) {
      if (id && nodes.some((n) => n.id === id) && !first.includes(id)) first.push(id);
    }
    const a = rf.screenToFlowPosition({ x: 0, y: 0 });
    const b = rf.screenToFlowPosition({ x: window.innerWidth, y: window.innerHeight });
    const mid = { x: (a.x + b.x) / 2, y: (a.y + b.y) / 2 };
    const inView = (n: WNode) => (n.floor ?? '') === st.activeFloor && n.x < b.x && n.x + n.w > a.x && n.y < b.y && n.y + n.h > a.y;
    const dist = (n: WNode) => Math.hypot(n.x + n.w / 2 - mid.x, n.y + n.h / 2 - mid.y);
    for (const n of nodes.filter(inView).sort((p, q) => dist(p) - dist(q))) {
      if (!first.includes(n.id)) first.push(n.id);
    }
    if (first.length) hub.wakeOrder(ws, first).catch(() => {});
  }, [ws, session, waking, loaded, rf]);
}
