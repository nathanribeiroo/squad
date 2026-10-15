// /ws/workspaces with automatic reconnection: the workspace list, live.
import { useSquad } from '../state/store';
import { wsURL } from './client';
import type { ServerEvent, WorkspaceSummary } from './types';

export function connectWorkspaces(): () => void {
  let ws: WebSocket | null = null;
  let stopped = false;
  let retry = 0;
  let timer: number | undefined;

  const open = () => {
    if (stopped) return;
    ws = new WebSocket(wsURL('/ws/workspaces'));
    ws.onmessage = (m) => {
      try {
        const ev = JSON.parse(m.data as string) as ServerEvent<WorkspaceSummary[]>;
        if (ev.type !== 'workspaces') return;
        retry = 0;
        useSquad.getState().setWorkspaces(ev.data);
      } catch {
        /* ignore malformed */
      }
    };
    ws.onclose = () => {
      if (stopped) return;
      retry++;
      timer = window.setTimeout(open, Math.min(5000, 300 * retry));
    };
  };
  open();
  return () => {
    stopped = true;
    window.clearTimeout(timer);
    ws?.close();
  };
}
