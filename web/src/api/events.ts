// /ws/events with automatic reconnection; events feed the store.
import { useSquad } from '../state/store';
import { wsPath, wsURL } from './client';
import type { ServerEvent } from './types';

export function connectEvents(): () => void {
  let ws: WebSocket | null = null;
  let stopped = false;
  let retry = 0;
  let timer: number | undefined;

  const open = () => {
    if (stopped) return;
    useSquad.getState().setConnection(retry === 0 ? 'connecting' : 'reconnecting');
    ws = new WebSocket(wsURL(wsPath('/ws/events')));
    ws.onmessage = (m) => {
      if (stopped) return; // a late event of the workspace left behind
      try {
        const ev = JSON.parse(m.data as string) as ServerEvent;
        if (ev.type === 'hello') {
          retry = 0;
          useSquad.getState().setConnection('connected');
          useSquad.getState().loadSummaries();
        }
        useSquad.getState().apply(ev);
      } catch {
        /* ignore malformed */
      }
    };
    ws.onclose = () => {
      if (stopped) return;
      useSquad.getState().setConnection('reconnecting');
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
