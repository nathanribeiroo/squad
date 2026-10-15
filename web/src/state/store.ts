// Global client state, fed by /ws/events.
import { create } from 'zustand';
import type { PendingRequest, ServerEvent, StateExtra, StateResponse, TerminalRuntime, WEdge, WNode, Workspace } from '../api/types';

export type ActivityOutcome = 'pending' | 'replied' | 'cancelled' | 'delivered' | 'held' | 'note';

export interface ActivityItem {
  key: string;
  time: string;
  id?: string; // request id
  from?: string;
  to?: string;
  fromName: string;
  toName: string;
  text: string;
  outcome: ActivityOutcome;
  elapsedMs?: number;
  source?: string;
}

/** A request in flight, for wire animations (W3). */
export interface Flight {
  id: string;
  from: string;
  to: string;
  phase: 'sending' | 'pending' | 'replying' | 'received' | 'cancelled';
  startedAt: number;
  updatedAt: number;
}

export interface NoteEdit {
  author: string;
  at: number;
}

interface SquadState {
  workspace: Workspace | null;
  runtime: Record<string, TerminalRuntime>;
  extra: StateExtra;
  connection: 'connecting' | 'connected' | 'reconnecting';
  activity: ActivityItem[];
  flights: Record<string, Flight>;
  noteEdits: Record<string, NoteEdit>;
  drawerOpen: boolean;
  composerOpen: boolean;
  composerTarget: string | null;
  newTermOpen: boolean;
  focusedTerminal: string | null;
  selectedNode: string | null;
  tool: 'select' | 'pan';
  theme: 'dark' | 'light';
  termTheme: 'noite' | 'papel';

  load(s: StateResponse): void;
  apply(ev: ServerEvent): void;
  setConnection(c: SquadState['connection']): void;
  setDrawer(open: boolean): void;
  setComposer(open: boolean, target?: string | null): void;
  setNewTerm(open: boolean): void;
  setFocusedTerminal(id: string | null): void;
  setSelectedNode(id: string | null): void;
  setTool(t: SquadState['tool']): void;
  setTheme(t: SquadState['theme']): void;
  setTermTheme(t: SquadState['termTheme']): void;
  moveNodeLocal(id: string, x: number, y: number): void;
}

const emptyExtra: StateExtra = { paused: false, limits: { maxHops: 6, perMinute: 30 }, rate: 0, pending: [] };

function upsert<T extends { id: string }>(list: T[], item: T): T[] {
  const i = list.findIndex((x) => x.id === item.id);
  if (i < 0) return [...list, item];
  const next = list.slice();
  next[i] = item;
  return next;
}

function nameOf(ws: Workspace | null, id?: string): string {
  const n = ws?.nodes.find((x) => x.id === id);
  return n?.terminal?.name ?? n?.note?.title ?? '';
}

function prefersDark(): boolean {
  return typeof matchMedia === 'function' ? matchMedia('(prefers-color-scheme: dark)').matches : true;
}

function stored<T extends string>(key: string, fallback: T): T {
  try {
    return (localStorage.getItem(key) as T) || fallback;
  } catch {
    return fallback;
  }
}

function persist(key: string, value: string) {
  try {
    localStorage.setItem(key, value);
  } catch {
    /* storage may be unavailable */
  }
}

export const useSquad = create<SquadState>((set, get) => ({
  workspace: null,
  runtime: {},
  extra: emptyExtra,
  connection: 'connecting',
  activity: [],
  flights: {},
  noteEdits: {},
  drawerOpen: false,
  composerOpen: false,
  composerTarget: null,
  newTermOpen: false,
  focusedTerminal: null,
  selectedNode: null,
  tool: 'select',
  theme: stored<'dark' | 'light'>('squad.theme', prefersDark() ? 'dark' : 'light'),
  termTheme: stored<'noite' | 'papel'>('squad.term', stored('squad.theme', prefersDark() ? 'dark' : 'light') === 'dark' ? 'noite' : 'papel'),

  load: (s) => {
    const pending: Record<string, Flight> = {};
    for (const p of s.extra?.pending ?? []) {
      pending[p.id] = { id: p.id, from: p.from, to: p.to, phase: 'pending', startedAt: Date.parse(p.sentAt), updatedAt: Date.now() };
    }
    set({ workspace: s.workspace, runtime: s.runtime ?? {}, extra: s.extra ?? emptyExtra, flights: pending });
  },

  apply: (ev) => {
    const st = get();
    const ws = st.workspace;
    const d = ev.data ?? {};
    switch (ev.type) {
      case 'hello':
        st.load(d as StateResponse);
        return;
      case 'node.added':
      case 'node.updated':
        if (ws) set({ workspace: { ...ws, nodes: upsert(ws.nodes, d as WNode) } });
        return;
      case 'node.removed':
        if (ws) set({ workspace: { ...ws, nodes: ws.nodes.filter((n) => n.id !== d.id) } });
        return;
      case 'edge.added':
      case 'edge.updated':
        if (ws) set({ workspace: { ...ws, edges: upsert(ws.edges, d as WEdge) } });
        return;
      case 'edge.removed':
        if (ws) set({ workspace: { ...ws, edges: ws.edges.filter((e) => e.id !== d.id) } });
        return;
      case 'terminal.state':
        set({ runtime: { ...st.runtime, [d.id]: { ...st.runtime[d.id], ...(d as TerminalRuntime) } } });
        return;
      case 'terminal.title':
        set({ runtime: { ...st.runtime, [d.id]: { ...st.runtime[d.id], title: d.title } } });
        return;
      case 'request.sent': {
        const now = Date.now();
        const p: PendingRequest = { id: d.id, from: d.from, to: d.to, fromName: d.fromName, toName: d.toName, preview: d.preview, hop: d.hop, sentAt: ev.time };
        set({
          extra: { ...st.extra, rate: st.extra.rate + 1, pending: [...st.extra.pending.filter((x) => x.id !== p.id), p] },
          flights: { ...st.flights, [d.id]: { id: d.id, from: d.from, to: d.to, phase: 'sending', startedAt: now, updatedAt: now } },
          activity: [
            { key: `req-${d.id}`, time: ev.time, id: d.id, from: d.from, to: d.to, fromName: d.fromName, toName: d.toName, text: d.preview ?? '', outcome: 'pending' as const },
            ...st.activity,
          ].slice(0, 200),
        });
        return;
      }
      case 'request.pending': {
        const f = st.flights[d.id];
        if (f) set({ flights: { ...st.flights, [d.id]: { ...f, phase: 'pending', updatedAt: Date.now() } } });
        return;
      }
      case 'reply.sent': {
        const f = st.flights[d.id];
        if (f) set({ flights: { ...st.flights, [d.id]: { ...f, phase: 'replying', updatedAt: Date.now() } } });
        return;
      }
      case 'reply.received':
      case 'request.cancelled': {
        const cancelled = ev.type === 'request.cancelled';
        const f = st.flights[d.id];
        const flights = { ...st.flights };
        if (f) flights[d.id] = { ...f, phase: cancelled ? 'cancelled' : 'received', updatedAt: Date.now() };
        set({
          flights,
          extra: { ...st.extra, pending: st.extra.pending.filter((x) => x.id !== d.id) },
          activity: st.activity.map((a) =>
            a.id === d.id ? { ...a, outcome: cancelled ? 'cancelled' : 'replied', elapsedMs: d.elapsedMs, source: d.source } : a,
          ),
        });
        // Forget finished flights after the "received" marker fades (~4 s).
        setTimeout(() => {
          const cur = get().flights;
          if (cur[d.id] && (cur[d.id].phase === 'received' || cur[d.id].phase === 'cancelled')) {
            const next = { ...cur };
            delete next[d.id];
            set({ flights: next });
          }
        }, 4500);
        return;
      }
      case 'request.held':
        set({
          activity: [
            { key: `held-${ev.time}`, time: ev.time, from: d.from, to: d.to, fromName: d.fromName, toName: d.toName, text: d.paused ? 'Retido: conversas pausadas' : 'Retido: limite de pedidos por minuto', outcome: 'held' as const },
            ...st.activity,
          ].slice(0, 200),
        });
        return;
      case 'conversations.paused':
        set({ extra: { ...st.extra, paused: !!d.paused } });
        return;
      case 'note.edited': {
        const author = d.author === 'external' ? 'Edição externa' : d.author === 'user' ? 'Você' : d.author;
        set({
          noteEdits: { ...st.noteEdits, [d.id]: { author, at: Date.now() } },
          activity: [
            { key: `note-${d.id}-${ev.time}`, time: ev.time, fromName: author, toName: nameOf(ws, d.id) || d.file, text: d.file, outcome: 'note' as const },
            ...st.activity,
          ].slice(0, 200),
        });
        return;
      }
    }
  },

  setConnection: (connection) => set({ connection }),
  setDrawer: (drawerOpen) => set({ drawerOpen }),
  setComposer: (composerOpen, target) => set({ composerOpen, composerTarget: target === undefined ? get().composerTarget : target }),
  setNewTerm: (newTermOpen) => set({ newTermOpen }),
  setFocusedTerminal: (focusedTerminal) => set({ focusedTerminal }),
  setSelectedNode: (selectedNode) => set({ selectedNode }),
  setTool: (tool) => set({ tool }),
  // The terminal palette follows the UI theme; it can still be overridden afterwards.
  setTheme: (theme) => {
    const termTheme = theme === 'dark' ? 'noite' : 'papel';
    persist('squad.theme', theme);
    persist('squad.term', termTheme);
    set({ theme, termTheme });
  },
  setTermTheme: (termTheme) => {
    persist('squad.term', termTheme);
    set({ termTheme });
  },
  moveNodeLocal: (id, x, y) => {
    const ws = get().workspace;
    if (!ws) return;
    set({ workspace: { ...ws, nodes: ws.nodes.map((n) => (n.id === id ? { ...n, x, y } : n)) } });
  },
}));

const NO_NODES: WNode[] = [];

/** Nodes of the workspace (a stable empty list before the first load: zustand
 * selectors must not return fresh objects). */
export function useNodes(): WNode[] {
  return useSquad((s) => s.workspace?.nodes ?? NO_NODES);
}

/** Name of a node by id. */
export function useNodeName(id?: string): string {
  return useSquad((s) => nameOf(s.workspace, id));
}
