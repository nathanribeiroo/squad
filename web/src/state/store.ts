// Global client state, fed by /ws/events.
import { create } from 'zustand';
import { setWorkspace } from '../api/client';
import type {
  Floor,
  FloorConfig,
  FloorStatusResponse,
  PendingRequest,
  RoleDef,
  ServerEvent,
  StateExtra,
  StateResponse,
  TerminalRuntime,
  WEdge,
  WNode,
  Workspace,
  WorkspaceSummary,
} from '../api/types';

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
  /** When the reply started travelling back (reply.sent, or reply.received when it came straight from a hook). */
  replyAt?: number;
  elapsedMs?: number;
}

/** An agent acting on a portal (portal.action): drawn as its cursor on the page and a pill on the wire. */
export interface PortalAction {
  portal: string;
  by: string;
  byName: string;
  kind: string; // click, fill, type, key, scroll, goto, back, forward, reload
  label: string;
  x: number;
  y: number;
  hasXY: boolean;
  at: number;
  seq: number;
}

/** How long an agent's portal action stays on screen. */
export const PORTAL_ACTION_MS = 2500;

/** Last reply an origin terminal got (its "reply" strip). */
export interface LastReply {
  id: string;
  fromName: string;
  elapsedMs: number;
  at: number;
}

export interface NoteEdit {
  author: string;
  at: number;
}

export type ZoomLevel = 'live' | 'mini' | 'card';

/** A floor dialog: create one, land or discard a floor, read its setup log. */
export type FloorDialog = { kind: 'create' } | { kind: 'land' | 'discard' | 'log'; id: string };

/** Zoom level with ±0.03 hysteresis around 0.6 and 0.35 (handoff §2). */
export function levelFor(z: number, prev: ZoomLevel): ZoomLevel {
  const h = 0.03;
  if (prev === 'live' && z >= 0.6 - h) return 'live';
  if (prev === 'mini' && z >= 0.35 - h && z < 0.6 + h) return 'mini';
  if (prev === 'card' && z < 0.35 + h) return 'card';
  return z >= 0.6 ? 'live' : z >= 0.35 ? 'mini' : 'card';
}

interface SquadState {
  /** Every workspace, for the selector (/ws/workspaces). */
  workspaces: WorkspaceSummary[];
  /** The role library, shared by every workspace (/ws/workspaces). */
  roles: RoleDef[];
  /** The role library panel: closed (false), or open on a role (null: none chosen). */
  rolesPanel: false | string | null;
  /** The workspace on screen. */
  activeWs: string | null;
  /** The floor on screen ("" is the ground floor). */
  activeFloor: string;
  /** Git state of the floors (GET /api/floors/status, polled by the selector). */
  floorStatus: FloorStatusResponse | null;
  floorDialog: FloorDialog | null;
  /** A node to center once its floor is on screen (a wire to another floor). */
  focusRequest: string | null;
  workspace: Workspace | null;
  runtime: Record<string, TerminalRuntime>;
  extra: StateExtra;
  connection: 'connecting' | 'connected' | 'reconnecting';
  activity: ActivityItem[];
  flights: Record<string, Flight>;
  noteEdits: Record<string, NoteEdit>;
  lastReply: Record<string, LastReply>;
  /** Latest agent action per portal. */
  portalActions: Record<string, PortalAction>;
  /** Times of recent requests, for the per-minute rate in the top bar. */
  sendTimes: number[];
  drawerOpen: boolean;
  composerOpen: boolean;
  composerTarget: string | null;
  newTermOpen: boolean;
  /** Terminal whose settings form ("Configurar…") is open. */
  editTermId: string | null;
  focusedTerminal: string | null;
  selectedNode: string | null;
  tool: 'select' | 'pan';
  /** Level from the live zoom (changes during a gesture). */
  zoomLevel: ZoomLevel;
  /** Zoom content is laid out for; updated when a gesture ends. */
  renderZoom: number;
  theme: 'dark' | 'light';
  termTheme: 'noite' | 'papel';

  setWorkspaces(list: WorkspaceSummary[]): void;
  setRoles(list: RoleDef[]): void;
  openRoles(id?: string | null): void;
  closeRoles(): void;
  switchWorkspace(id: string): void;
  /** Shows a floor ("" the ground floor), optionally centering a node of it. */
  setFloor(id: string, focus?: string): void;
  setFloorStatus(s: FloorStatusResponse | null): void;
  setFloorDialog(d: FloorDialog | null): void;
  clearFocusRequest(): void;
  load(s: StateResponse): void;
  apply(ev: ServerEvent): void;
  setConnection(c: SquadState['connection']): void;
  setDrawer(open: boolean): void;
  setComposer(open: boolean, target?: string | null): void;
  setNewTerm(open: boolean): void;
  setEditTerm(id: string | null): void;
  setFocusedTerminal(id: string | null): void;
  setSelectedNode(id: string | null): void;
  setTool(t: SquadState['tool']): void;
  setZoom(z: number, settled: boolean): void;
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
  return n?.terminal?.name ?? n?.note?.title ?? (n?.portal ? n.portal.name || 'Portal' : '');
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

/** State that belongs to the workspace on screen, cleared when switching. */
const perWorkspace = {
  workspace: null,
  runtime: {},
  extra: emptyExtra,
  connection: 'connecting',
  activity: [],
  flights: {},
  noteEdits: {},
  lastReply: {},
  portalActions: {},
  sendTimes: [],
  composerOpen: false,
  composerTarget: null,
  newTermOpen: false,
  editTermId: null,
  focusedTerminal: null,
  selectedNode: null,
  floorStatus: null,
  floorDialog: null,
  focusRequest: null,
} satisfies Partial<SquadState>;

const WS_KEY = 'squad.ws';
const floorKey = (ws: string) => `squad.floor.${ws}`;

/** The floor to show when a workspace opens: the URL's (when it is that
 * workspace's), then the last one shown here. */
function initialFloor(ws: string): string {
  const q = new URLSearchParams(location.search);
  if (q.get('ws') === ws && q.has('floor')) return q.get('floor') ?? '';
  return stored(floorKey(ws), '');
}

/** Keeps the floor on screen in the URL (and out of it on the ground floor). */
function floorToURL(floor: string) {
  const url = new URL(location.href);
  if (floor) url.searchParams.set('floor', floor);
  else url.searchParams.delete('floor');
  history.replaceState(null, '', url);
}

/** The floor on screen, unless the workspace no longer has it. */
function validFloor(floor: string, floors?: Floor[]): string {
  return floor && floors?.some((f) => f.id === floor) ? floor : '';
}

export const useSquad = create<SquadState>((set, get) => ({
  workspaces: [],
  roles: [],
  rolesPanel: false,
  activeWs: null,
  activeFloor: '',
  floorStatus: null,
  floorDialog: null,
  focusRequest: null,
  workspace: null,
  runtime: {},
  extra: emptyExtra,
  connection: 'connecting',
  activity: [],
  flights: {},
  noteEdits: {},
  lastReply: {},
  portalActions: {},
  sendTimes: [],
  drawerOpen: false,
  composerOpen: false,
  composerTarget: null,
  newTermOpen: false,
  editTermId: null,
  focusedTerminal: null,
  selectedNode: null,
  tool: 'select',
  zoomLevel: 'live',
  renderZoom: 1,
  theme: stored<'dark' | 'light'>('squad.theme', prefersDark() ? 'dark' : 'light'),
  termTheme: stored<'noite' | 'papel'>('squad.term', stored('squad.theme', prefersDark() ? 'dark' : 'light') === 'dark' ? 'noite' : 'papel'),

  setRoles: (roles) => set({ roles }),
  openRoles: (id = null) => set({ rolesPanel: id }),
  closeRoles: () => set({ rolesPanel: false }),

  setWorkspaces: (workspaces) => {
    set({ workspaces });
    const st = get();
    if (workspaces.length === 0 || workspaces.some((w) => w.id === st.activeWs)) return;
    // First list, or the workspace on screen was removed: the one in the
    // URL, then the last one used here, then the first.
    const want = [new URLSearchParams(location.search).get('ws'), stored(WS_KEY, '')];
    const pick = want.find((id) => id && workspaces.some((w) => w.id === id)) ?? workspaces[0].id;
    st.switchWorkspace(pick);
  },

  switchWorkspace: (id) => {
    if (id === get().activeWs) return;
    // term/session closes the sockets of the workspace left behind (the
    // server keeps their screens for when we come back).
    setWorkspace(id);
    persist(WS_KEY, id);
    const activeFloor = initialFloor(id);
    const url = new URL(location.href);
    url.searchParams.set('ws', id);
    history.replaceState(null, '', url);
    floorToURL(activeFloor);
    set({ ...perWorkspace, activeWs: id, activeFloor });
  },

  setFloor: (id, focus) => {
    const st = get();
    if (st.activeWs) persist(floorKey(st.activeWs), id);
    floorToURL(id);
    if (id === st.activeFloor) {
      if (focus) set({ focusRequest: focus });
      return;
    }
    set({ activeFloor: id, focusRequest: focus ?? null, selectedNode: null, focusedTerminal: null, newTermOpen: false, editTermId: null });
  },
  setFloorStatus: (floorStatus) => set({ floorStatus }),
  setFloorDialog: (floorDialog) => set({ floorDialog }),
  clearFocusRequest: () => set({ focusRequest: null }),

  load: (s) => {
    const pending: Record<string, Flight> = {};
    for (const p of s.extra?.pending ?? []) {
      pending[p.id] = { id: p.id, from: p.from, to: p.to, phase: 'pending', startedAt: Date.parse(p.sentAt), updatedAt: Date.now() };
    }
    // The server only reports a count; spread it as "now" so it decays.
    const sendTimes = Array.from({ length: s.extra?.rate ?? 0 }, () => Date.now());
    set({ workspace: s.workspace, runtime: s.runtime ?? {}, extra: s.extra ?? emptyExtra, flights: pending, sendTimes });
    // A floor landed while we were away (or a stale link): the ground floor.
    const st = get();
    const floor = validFloor(st.activeFloor, s.workspace.floors);
    if (floor !== st.activeFloor) st.setFloor(floor);
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
        const lastReply = { ...st.lastReply };
        delete lastReply[d.from];
        const p: PendingRequest = { id: d.id, from: d.from, to: d.to, fromName: d.fromName, toName: d.toName, preview: d.preview, hop: d.hop, sentAt: ev.time };
        set({
          lastReply,
          sendTimes: [...st.sendTimes.filter((t) => now - t < 60_000), now],
          extra: { ...st.extra, pending: [...st.extra.pending.filter((x) => x.id !== p.id), p] },
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
        const now = Date.now();
        if (f) set({ flights: { ...st.flights, [d.id]: { ...f, phase: 'replying', updatedAt: now, replyAt: now } } });
        return;
      }
      case 'reply.received':
      case 'request.cancelled': {
        const cancelled = ev.type === 'request.cancelled';
        const f = st.flights[d.id];
        const flights = { ...st.flights };
        const now = Date.now();
        if (f) flights[d.id] = { ...f, phase: cancelled ? 'cancelled' : 'received', updatedAt: now, replyAt: f.replyAt ?? now, elapsedMs: d.elapsedMs };
        const lastReply = cancelled ? st.lastReply : { ...st.lastReply, [d.from]: { id: d.id, fromName: d.toName, elapsedMs: d.elapsedMs ?? 0, at: now } };
        set({
          flights,
          lastReply,
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
      case 'conversations.paused': {
        const reason: string = d.reason ?? '';
        const loop = d.paused && reason.startsWith('hop limit');
        set({
          extra: { ...st.extra, paused: !!d.paused },
          activity: loop
            ? [
                { key: `loop-${ev.time}`, time: ev.time, fromName: 'Squad', toName: 'todos', text: `Laço: limite de ${st.extra.limits.maxHops} saltos atingido ${reason.replace(/^hop limit \d+ reached /, '')}. Conversas pausadas até você retomar.`, outcome: 'held' as const },
                ...st.activity,
              ].slice(0, 200)
            : st.activity,
        });
        return;
      }
      case 'message.sent':
        set({
          activity: [
            { key: `msg-${ev.time}-${d.to}`, time: ev.time, to: d.to, fromName: 'Você', toName: d.toName, text: d.preview + (d.images ? ` (+${d.images} imagem${d.images > 1 ? 'ns' : ''})` : ''), outcome: 'delivered' as const },
            ...st.activity,
          ].slice(0, 200),
        });
        return;
      case 'portal.action': {
        const prev = st.portalActions[d.portal];
        set({ portalActions: { ...st.portalActions, [d.portal]: { ...(d as PortalAction), at: Date.now(), seq: (prev?.seq ?? 0) + 1 } } });
        return;
      }
      case 'workspace.updated':
        if (ws) set({ workspace: { ...ws, name: d.name, cwd: d.cwd, color: d.color } });
        return;
      case 'floors.updated': {
        if (!ws) return;
        const floors = d.floors as Floor[];
        const activeFloor = validFloor(st.activeFloor, floors);
        if (activeFloor !== st.activeFloor) st.setFloor(activeFloor);
        set({ workspace: { ...ws, floors, floorConfig: d.config as FloorConfig } });
        return;
      }
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
  // One terminal form at a time: new or settings.
  setNewTerm: (newTermOpen) => set(newTermOpen ? { newTermOpen, editTermId: null } : { newTermOpen }),
  setEditTerm: (editTermId) => set(editTermId ? { editTermId, newTermOpen: false } : { editTermId }),
  setFocusedTerminal: (focusedTerminal) => set({ focusedTerminal }),
  setSelectedNode: (selectedNode) => set({ selectedNode }),
  setTool: (tool) => set({ tool }),
  setZoom: (z, settled) => {
    const st = get();
    const zoomLevel = levelFor(z, st.zoomLevel);
    if (zoomLevel !== st.zoomLevel) set({ zoomLevel });
    if (settled && z !== st.renderZoom) set({ renderZoom: z });
  },
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
