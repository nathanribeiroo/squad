// /ws/portal/{id}: the live page of a portal. Text messages carry the state
// (or an error), binary ones a frame: 8-byte big-endian float64 paint time in
// Unix ms, then the JPEG. Input goes back as JSON.
import { useEffect, useRef, useState } from 'react';
import { wsPath, wsURL } from '../../api/client';

export interface PortalState {
  url: string;
  title: string;
  loading: boolean;
  error?: string;
  canBack: boolean;
  canForward: boolean;
  width: number;
  height: number;
}

/** A ref of the agents' last snapshot, where it is on the page now (page px). */
export interface PortalElement {
  ref: string;
  role: string;
  name: string;
  x: number;
  y: number;
  w: number;
  h: number;
}

export interface PortalStream {
  connected: boolean;
  state: PortalState | null;
  /** Object URL of the latest frame (kept across zoom levels). */
  frame: string | null;
  /** How old frames are when they arrive, smoothed (ms). */
  latency: number | null;
  /** Server-side failure, e.g. no browser installed. */
  error: string | null;
  noBrowser: boolean;
  /** The @ELEMENTOS layer while it is on: null until the first answer; snapshot false when no agent read this page yet. */
  elements: { snapshot: boolean; items: PortalElement[] } | null;
  send(msg: object): void;
}

// The last frame of each portal, so a node that remounts (zoom level change)
// or stops streaming still shows the page.
const lastFrames = new Map<string, string>();

/** Streams portal id while active; inactive keeps the last frame. With layer, the refs come too. */
export function usePortalStream(id: string, active: boolean, layer = false): PortalStream {
  const [connected, setConnected] = useState(false);
  const [state, setState] = useState<PortalState | null>(null);
  const [frame, setFrame] = useState<string | null>(() => lastFrames.get(id) ?? null);
  const [latency, setLatency] = useState<number | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [noBrowser, setNoBrowser] = useState(false);
  const [elements, setElements] = useState<PortalStream['elements']>(null);
  const ws = useRef<WebSocket | null>(null);
  const layerRef = useRef(layer);
  layerRef.current = layer;

  useEffect(() => {
    if (!active) return;
    let stopped = false;
    let retry = 0;
    let timer: number | undefined;
    let lag: number | null = null;
    let replay = true; // the first frame of a connection is the last one kept, maybe old
    const open = () => {
      if (stopped) return;
      const sock = new WebSocket(wsURL(wsPath(`/ws/portal/${id}`)));
      sock.binaryType = 'arraybuffer';
      ws.current = sock;
      sock.onopen = () => {
        replay = true;
        setConnected(true);
        if (layerRef.current) sock.send(JSON.stringify({ type: 'elements', on: true }));
      };
      sock.onmessage = (m) => {
        if (stopped) return;
        if (typeof m.data === 'string') {
          const msg = JSON.parse(m.data) as {
            type: string;
            state?: PortalState;
            error?: string;
            noBrowser?: boolean;
            elements?: PortalElement[];
            snapshot?: boolean;
          };
          if (msg.type === 'elements') {
            if (layerRef.current) setElements({ snapshot: !!msg.snapshot, items: msg.elements ?? [] });
          } else if (msg.type === 'state' && msg.state) {
            retry = 0;
            setError(null);
            setState(msg.state);
          } else if (msg.type === 'error') {
            setError(msg.error ?? 'erro');
            setNoBrowser(!!msg.noBrowser);
          }
          return;
        }
        const buf = m.data as ArrayBuffer;
        const painted = new DataView(buf, 0, 8).getFloat64(0);
        const url = URL.createObjectURL(new Blob([buf.slice(8)], { type: 'image/jpeg' }));
        const prev = lastFrames.get(id);
        lastFrames.set(id, url);
        setFrame(url);
        if (prev) URL.revokeObjectURL(prev);
        // The browser sends frames only when the page changes, so a frame's
        // age is the delay only for fresh ones, not for the replayed one.
        if (replay) {
          replay = false;
          return;
        }
        const age = Math.max(0, Date.now() - painted);
        lag = lag == null ? age : lag * 0.8 + age * 0.2;
        setLatency(Math.round(lag));
      };
      sock.onclose = () => {
        setConnected(false);
        if (ws.current === sock) ws.current = null;
        if (stopped) return;
        retry++;
        // A server-side error (no browser) closes the socket: retry slowly.
        timer = window.setTimeout(open, Math.min(10000, 500 * retry));
      };
    };
    open();
    return () => {
      stopped = true;
      window.clearTimeout(timer);
      ws.current?.close();
      ws.current = null;
      setConnected(false);
    };
  }, [id, active]);
  // Turning the layer on or off.
  useEffect(() => {
    setElements(null);
    if (ws.current?.readyState === WebSocket.OPEN) ws.current.send(JSON.stringify({ type: 'elements', on: layer }));
  }, [layer]);

  return {
    connected,
    state,
    frame,
    latency,
    error,
    noBrowser,
    elements: layer ? elements : null,
    send: (msg) => {
      if (ws.current?.readyState === WebSocket.OPEN) ws.current.send(JSON.stringify(msg));
    },
  };
}

/** Modifier bits of the DevTools protocol: Alt 1, Ctrl 2, Meta 4, Shift 8. */
export function mods(e: { altKey: boolean; ctrlKey: boolean; metaKey: boolean; shiftKey: boolean }): number {
  return (e.altKey ? 1 : 0) | (e.ctrlKey ? 2 : 0) | (e.metaKey ? 4 : 0) | (e.shiftKey ? 8 : 0);
}

export const BUTTONS = ['left', 'middle', 'right'] as const;
