// Global client state, fed by /ws/events.
import { create } from 'zustand';
import { api, setWorkspace } from '../api/client';
import { askPhrase } from '../lib/askText';
import type { CargoItem } from '../lib/cargo';
import type { Box, PlaceKind } from '../lib/place';
import { noticeAsk } from '../lib/notify';
import { mergeActivity, toActivity } from '../lib/radio';
import { toast } from '../lib/toast';
import type {
  CheckKind,
  AttentionChoice,
  DrawingShape,
  Floor,
  FloorConfig,
  FloorStatusResponse,
  PendingRequest,
  RadioItem,
  RoleDef,
  ServerEvent,
  SessionList,
  StateExtra,
  StateResponse,
  SummarySettings,
  TerminalRuntime,
  TurnCard,
  UsageAccount,
  UsageFlight,
  WEdge,
  WNode,
  Workspace,
  WorkspaceSummary,
} from '../api/types';

export type ActivityOutcome = 'pending' | 'replied' | 'cancelled' | 'delivered' | 'held' | 'note' | 'turn' | 'answered' | 'check';

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
  hold?: 'paused' | 'rate' | 'loop';
  /** An ask answered from the interface: the answer. */
  choice?: AttentionChoice | 'option';
  /** On a question or a plan: what was picked, or the user's own text. */
  answer?: string;
  /** The reply's text is kept. */
  hasReply?: boolean;
  /** A pending request waiting for its target's turn to end. */
  queued?: boolean;
  /** check: what a flight marked on a floor, and the result. */
  check?: CheckKind;
  result?: 'pass' | 'fail';
}

/** A request in flight, for wire animations (W3). */
export interface Flight {
  id: string;
  from: string;
  to: string;
  /** queued: waiting in the target's line for its turn to end. */
  phase: 'sending' | 'queued' | 'pending' | 'replying' | 'received' | 'cancelled';
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
  /** The flight that answered (its node id) and its name. */
  from?: string;
  fromName: string;
  elapsedMs: number;
  at: number;
}

export interface NoteEdit {
  author: string;
  at: number;
}

export type ZoomLevel = 'live' | 'mini' | 'card';

/** A floor dialog: create one, land or discard a floor, read its setup log;
 * merge the session's copy, read its setup log. */
export type FloorDialog = { kind: 'create' } | { kind: 'land' | 'discard' | 'log' | 'rename'; id: string } | { kind: 'merge-copy' | 'copy-log' };

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
  /** Its open session: when another one opens, the canvas is reloaded. */
  activeSession: string | null;
  /** Each workspace's sessions, as last read or announced ("sessions" event). */
  sessionLists: Record<string, SessionList>;
  /** The floor on screen ("" is the ground floor). */
  activeFloor: string;
  /** Git state of the floors (GET /api/floors/status, polled by the selector). */
  floorStatus: FloorStatusResponse | null;
  floorDialog: FloorDialog | null;
  /** A node to center once its floor is on screen (a wire to another floor). */
  focusRequest: string | null;
  /** Each agent terminal's last turn (internal/summary). */
  cards: Record<string, TurnCard>;
  /** Cards closed with ×: terminal → the turn's time. */
  hiddenCards: Record<string, string>;
  summarySettings: SummarySettings | null;
  summaryDialog: 'overview' | 'settings' | null;
  /** What each flight used (Consumo), by terminal. */
  usage: Record<string, UsageFlight>;
  /** The agents' plan limits, by agent: the same in every workspace. */
  accounts: Record<string, UsageAccount>;
  /** Text the Composer starts with (a suggested next step). */
  composerDraft: string | null;
  /** A cargo's files dropped on the closed Composer: it opens with them. */
  composerPaths: CargoItem[] | null;
  workspace: Workspace | null;
  runtime: Record<string, TerminalRuntime>;
  /** Flights asking the user (PRIORIDADE), the oldest first. */
  askingIds: string[];
  extra: StateExtra;
  connection: 'connecting' | 'connected' | 'reconnecting';
  /** Since when the link is down (0 while connected), and when it tries again. */
  linkLostAt: number;
  retryAt: number;
  /** The Rádio, newest first: kept by the server (internal/radio), a page at a time. */
  activity: ActivityItem[];
  /** Older lines of the Rádio wait on the server. */
  radioMore: boolean;
  flights: Record<string, Flight>;
  noteEdits: Record<string, NoteEdit>;
  lastReply: Record<string, LastReply>;
  /** Latest agent action per portal. */
  portalActions: Record<string, PortalAction>;
  /** Folders of a cargo's tree that changed on disk: cargo id → folder → a count. */
  cargoChanges: Record<string, Record<string, number>>;
  /** A cargo's files are being dragged: flights and Transmitir show where to drop. */
  cargoDrag: boolean;
  /** Times of recent requests, for the per-minute rate in the top bar. */
  sendTimes: number[];
  drawerOpen: boolean;
  composerOpen: boolean;
  composerTarget: string | null;
  newTermOpen: boolean;
  /** Where the new flight takes off: the box drawn with its tool. */
  newTermBox: Box | null;
  /** Terminal whose settings form ("Configurar…") is open. */
  editTermId: string | null;
  focusedTerminal: string | null;
  selectedNode: string | null;
  /** The canvas tool: select, pan, draw (Desenho, D), frame (drag a new
   * group, G) or place (click or drag where a new item goes: T, N, P, C). */
  tool: 'select' | 'pan' | 'draw' | 'frame' | 'place';
  /** The item the place tool puts on the canvas. */
  placeKind: PlaceKind | null;
  /** What the Desenho tool draws, and in which color. */
  drawShape: DrawingShape;
  drawColor: string;
  /** A text drawing open for typing (just made, or double-clicked). */
  editText: string | null;
  /** Level from the live zoom (changes during a gesture). */
  zoomLevel: ZoomLevel;
  /** Zoom content is laid out for; updated when a gesture ends. */
  renderZoom: number;
  /** The theme on screen; themePref may follow the system. */
  theme: 'dark' | 'light';
  themePref: 'dark' | 'light' | 'system';
  termTheme: 'noite' | 'papel';
  /** Configurações, open on a tab (null: closed). */
  settingsTab: SettingsTab | null;
  /** The view: the canvas (Espaço) or the Sinóptico (⌥S). */
  view: 'space' | 'synoptic';
  /** The flight the Sinóptico shows on the right, and whether it stays there. */
  synPick: string | null;
  synPinned: boolean;
  /** A flight docked at an edge of the Espaço (⌥⇧←/→), kept per workspace. */
  dock: Dock | null;
  /** The flight on the whole stage (Foco, ⌥F). */
  focusPane: string | null;
  /** Flights whose screen is hidden (⋯ → Ocultar a tela), kept per workspace. */
  hiddenScreens: string[];
  /** A node for the canvas to select (Tab), and to bring into view. */
  selectRequest: string | null;
  /** The nodes selected on the canvas (one or more). */
  selectedIds: string[];
  /** Flight plans: the gallery (⌥M) or saving one (⌥⇧M). */
  plansDialog: 'gallery' | 'save' | null;

  setWorkspaces(list: WorkspaceSummary[]): void;
  /** A workspace's sessions (from the "sessions" event, or a request). */
  setSessions(list: SessionList): void;
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
  setConnection(c: SquadState['connection'], retryAt?: number): void;
  setDrawer(open: boolean): void;
  /** Opens or closes the Composer; draft is text to start with. */
  setComposer(open: boolean, target?: string | null, draft?: string, paths?: CargoItem[]): void;
  takeComposerDraft(): string | null;
  takeComposerPaths(): CargoItem[] | null;
  /** Reads the cards and the summary settings (on connecting). */
  loadSummaries(): void;
  /** Reads the Rádio's last lines (on connecting). */
  loadRadio(): void;
  /** Reads what the flights used and the plan limits (on connecting). */
  loadUsage(): void;
  /** Reads the Rádio's next older page. */
  loadOlderRadio(): void;
  hideCard(terminal: string, at: string): void;
  setSummaryDialog(d: SquadState['summaryDialog']): void;
  setSummarySettings(s: SummarySettings): void;
  setNewTerm(open: boolean, box?: Box | null): void;
  /** Picks the tool that puts a new item on the canvas. */
  startPlace(kind: PlaceKind): void;
  setEditTerm(id: string | null): void;
  setFocusedTerminal(id: string | null): void;
  setSelectedNode(id: string | null): void;
  setTool(t: SquadState['tool']): void;
  setDraw(d: { shape?: DrawingShape; color?: string }): void;
  setEditText(id: string | null): void;
  setZoom(z: number, settled: boolean): void;
  setTheme(t: SquadState['themePref']): void;
  openSettings(tab?: SettingsTab | null): void;
  setView(v: SquadState['view']): void;
  setSynPick(id: string | null): void;
  setSynPinned(pinned: boolean, id: string): void;
  setTermTheme(t: SquadState['termTheme']): void;
  moveNodeLocal(id: string, x: number, y: number): void;
  moveNodesLocal(moves: { id: string; x: number; y: number }[]): void;
  setDock(d: Dock | null): void;
  setFocusPane(id: string | null): void;
  toggleHiddenScreen(id: string): void;
  requestSelect(id: string | null): void;
  setSelectedIds(ids: string[]): void;
  setCargoDrag(on: boolean): void;
  setPlansDialog(d: SquadState['plansDialog']): void;
}

export interface Dock {
  id: string;
  side: 'left' | 'right';
}

const dockKey = (ws: string) => `torre.dock.${ws}`;
const hiddenKey = (ws: string) => `torre.hidden.${ws}`;

function storedDock(ws: string | null): Dock | null {
  if (!ws) return null;
  try {
    const d = JSON.parse(localStorage.getItem(dockKey(ws)) ?? 'null') as Dock | null;
    return d && typeof d.id === 'string' && (d.side === 'left' || d.side === 'right') ? d : null;
  } catch {
    return null;
  }
}

function storedHidden(ws: string | null): string[] {
  if (!ws) return [];
  try {
    const v = JSON.parse(localStorage.getItem(hiddenKey(ws)) ?? '[]') as unknown;
    return Array.isArray(v) ? v.filter((x): x is string => typeof x === 'string') : [];
  } catch {
    return [];
  }
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
  return n?.terminal?.name ?? n?.note?.title ?? (n?.portal ? n.portal.name || 'Portal' : (n?.cargo?.name ?? n?.group?.name ?? ''));
}

function prefersDark(): boolean {
  return typeof matchMedia === 'function' ? matchMedia('(prefers-color-scheme: dark)').matches : true;
}

/** The theme the user chose; before Configurações had "Sistema", the one they picked. */
function initialPref(): 'dark' | 'light' | 'system' {
  const p = stored<'dark' | 'light' | 'system' | ''>('torre.themePref', '');
  if (p) return p;
  return stored<'dark' | 'light' | ''>('torre.theme', '') || 'system';
}

export type SettingsTab = 'appearance' | 'agents' | 'workspaces' | 'limits' | 'summaries' | 'shortcuts' | 'notices';

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
  askingIds: [],
  extra: emptyExtra,
  connection: 'connecting',
  linkLostAt: 0,
  retryAt: 0,
  activity: [],
  radioMore: false,
  flights: {},
  noteEdits: {},
  lastReply: {},
  portalActions: {},
  cargoChanges: {},
  sendTimes: [],
  composerOpen: false,
  composerTarget: null,
  newTermOpen: false,
  newTermBox: null,
  editTermId: null,
  focusedTerminal: null,
  selectedNode: null,
  floorStatus: null,
  floorDialog: null,
  focusRequest: null,
  cards: {},
  hiddenCards: {},
  summarySettings: null,
  summaryDialog: null,
  usage: {},
  composerDraft: null,
  composerPaths: null,
  dock: null,
  focusPane: null,
  hiddenScreens: [],
  selectRequest: null,
  selectedIds: [],
  plansDialog: null,
} satisfies Partial<SquadState>;

const WS_KEY = 'torre.ws';
const floorKey = (ws: string) => `torre.floor.${ws}`;

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

/** Lines of the Rádio that arrive while its page is read (loadRadio). */
let radioLive: ActivityItem[] | null = null;

export const useSquad = create<SquadState>((set, get) => ({
  workspaces: [],
  roles: [],
  rolesPanel: false,
  activeWs: null,
  activeSession: null,
  sessionLists: {},
  activeFloor: '',
  floorStatus: null,
  floorDialog: null,
  focusRequest: null,
  cards: {},
  hiddenCards: {},
  summarySettings: null,
  summaryDialog: null,
  usage: {},
  accounts: {},
  composerDraft: null,
  composerPaths: null,
  workspace: null,
  runtime: {},
  askingIds: [],
  extra: emptyExtra,
  connection: 'connecting',
  linkLostAt: 0,
  retryAt: 0,
  activity: [],
  radioMore: false,
  flights: {},
  noteEdits: {},
  lastReply: {},
  portalActions: {},
  cargoChanges: {},
  sendTimes: [],
  cargoDrag: false,
  drawerOpen: false,
  composerOpen: false,
  composerTarget: null,
  newTermOpen: false,
  newTermBox: null,
  editTermId: null,
  focusedTerminal: null,
  selectedNode: null,
  tool: 'select',
  placeKind: null,
  drawShape: 'pen',
  drawColor: 'none',
  editText: null,
  zoomLevel: 'live',
  renderZoom: 1,
  theme: initialPref() === 'system' ? (prefersDark() ? 'dark' : 'light') : stored<'dark' | 'light'>('torre.theme', prefersDark() ? 'dark' : 'light'),
  themePref: initialPref(),
  settingsTab: null,
  view: stored<'space' | 'synoptic'>('torre.view', 'space'),
  synPick: null,
  synPinned: false,
  dock: null,
  focusPane: null,
  hiddenScreens: [],
  selectRequest: null,
  selectedIds: [],
  plansDialog: null,
  termTheme: stored<'noite' | 'papel'>('torre.term', stored('torre.theme', prefersDark() ? 'dark' : 'light') === 'dark' ? 'noite' : 'papel'),

  setRoles: (roles) => set({ roles }),
  openSettings: (tab = 'appearance') => set({ settingsTab: tab }),
  setView: (view) => {
    persist('torre.view', view);
    set({ view, focusedTerminal: null });
  },
  // A pinned flight stays until unpinned; picking another only moves the selection.
  setSynPick: (id) => set((s) => (s.synPinned ? {} : { synPick: id })),
  setSynPinned: (synPinned, id) => set({ synPinned, synPick: id }),
  setDock: (dock) => {
    const ws = get().activeWs;
    if (ws) persist(dockKey(ws), dock ? JSON.stringify(dock) : '');
    // A flight docked leaves the Foco, and the other way around.
    set((s) => ({ dock, focusPane: dock && s.focusPane === dock.id ? null : s.focusPane }));
  },
  setFocusPane: (focusPane) => set({ focusPane }),
  toggleHiddenScreen: (id) => {
    const st = get();
    const hiddenScreens = st.hiddenScreens.includes(id) ? st.hiddenScreens.filter((x) => x !== id) : [...st.hiddenScreens, id];
    if (st.activeWs) persist(hiddenKey(st.activeWs), JSON.stringify(hiddenScreens));
    set({ hiddenScreens });
  },
  requestSelect: (selectRequest) => set({ selectRequest }),
  setSelectedIds: (selectedIds) => set({ selectedIds }),
  setCargoDrag: (cargoDrag) => set({ cargoDrag }),
  setPlansDialog: (plansDialog) => set({ plansDialog }),
  openRoles: (id = null) => set({ rolesPanel: id }),
  closeRoles: () => set({ rolesPanel: false }),

  setWorkspaces: (workspaces) => {
    const before = get().workspaces;
    set({ workspaces });
    const st = get();
    // A flight asking you in another workspace: the same notice as here (the
    // first list, when the page opens, isn't news).
    for (const w of workspaces) {
      const was = before.find((b) => b.id === w.id);
      if (was && w.id !== st.activeWs && (w.attention ?? 0) > (was.attention ?? 0)) {
        noticeAsk(w.name, w.attention === 1 ? 'Um voo pede você neste workspace' : `${w.attention} voos pedem você neste workspace`);
      }
    }
    if (workspaces.length === 0) return;
    const cur = workspaces.find((w) => w.id === st.activeWs);
    if (cur) {
      followSession(cur.id, cur.session?.id);
      return;
    }
    // First list, or the workspace on screen was removed: the one in the
    // URL, then the last one used here, then the first.
    const want = [new URLSearchParams(location.search).get('ws'), stored(WS_KEY, '')];
    const pick = want.find((id) => id && workspaces.some((w) => w.id === id)) ?? workspaces[0].id;
    st.switchWorkspace(pick);
  },

  setSessions: (list) => {
    set({ sessionLists: { ...get().sessionLists, [list.workspace]: list } });
    followSession(list.workspace, list.current);
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
    // A session just opened there is in its session list before the workspace list.
    const st = get();
    const activeSession = st.sessionLists[id]?.current || st.workspaces.find((w) => w.id === id)?.session?.id || null;
    set({ ...perWorkspace, activeWs: id, activeSession, activeFloor });
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
      pending[p.id] = { id: p.id, from: p.from, to: p.to, phase: p.queued ? 'queued' : 'pending', startedAt: Date.parse(p.sentAt), updatedAt: Date.now() };
    }
    // The server only reports a count; spread it as "now" so it decays.
    const sendTimes = Array.from({ length: s.extra?.rate ?? 0 }, () => Date.now());
    const runtime = s.runtime ?? {};
    // The docked flight comes back with the workspace; a hidden screen stays hidden.
    const isTerm = (id?: string | null) => !!id && s.workspace.nodes.some((n) => n.id === id && n.terminal);
    const cur = get();
    const dock = cur.dock && isTerm(cur.dock.id) ? cur.dock : storedDock(cur.activeWs);
    set({
      workspace: s.workspace,
      runtime,
      askingIds: askingOf(runtime, []),
      extra: s.extra ?? emptyExtra,
      flights: pending,
      sendTimes,
      dock: dock && isTerm(dock.id) ? dock : null,
      focusPane: isTerm(cur.focusPane) ? cur.focusPane : null,
      hiddenScreens: storedHidden(cur.activeWs),
    });
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
        if (st.usage[d.id]) {
          const usage = { ...st.usage };
          delete usage[d.id];
          set({ usage });
        }
        if (st.dock?.id === d.id) st.setDock(null);
        if (st.focusPane === d.id) set({ focusPane: null });
        return;
      case 'edge.added':
      case 'edge.updated':
        if (ws) set({ workspace: { ...ws, edges: upsert(ws.edges, d as WEdge) } });
        return;
      case 'edge.removed':
        if (ws) set({ workspace: { ...ws, edges: ws.edges.filter((e) => e.id !== d.id) } });
        return;
      case 'terminal.state': {
        const prev = st.runtime[d.id];
        const next: TerminalRuntime = { ...prev, ...(d as TerminalRuntime), attention: (d as TerminalRuntime).attention };
        const runtime = { ...st.runtime, [d.id]: next };
        const patch: Partial<SquadState> = { runtime, askingIds: askingOf(runtime, st.askingIds) };
        // A new ask: a notification and a sound, as chosen in Avisos.
        if (prev?.activity !== 'ask' && next.activity === 'ask') {
          const t = ws?.nodes.find((n) => n.id === d.id)?.terminal;
          if (t) noticeAsk(t.callsign || t.name, [askPhrase(next.attention), next.attention?.detail].filter(Boolean).join(': '));
        }
        set(patch);
        return;
      }
      case 'limits.changed':
        set({ extra: { ...st.extra, limits: { ...st.extra.limits, ...(d as Partial<StateExtra['limits']>) } } });
        return;
      case 'terminal.title':
        set({ runtime: { ...st.runtime, [d.id]: { ...st.runtime[d.id], title: d.title } } });
        return;
      case 'terminal.notice':
        if (d.kind === 'not-resumed') toast(`Não deu para retomar a conversa de ${d.name}: começou uma nova.`, 'wait');
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
        });
        return;
      }
      case 'request.queued':
      case 'request.pending': {
        // Waiting in the target's line, then delivered.
        const queued = ev.type === 'request.queued';
        const f = st.flights[d.id];
        set({
          flights: f ? { ...st.flights, [d.id]: { ...f, phase: queued ? 'queued' : 'pending', updatedAt: Date.now() } } : st.flights,
          extra: { ...st.extra, pending: st.extra.pending.map((p) => (p.id === d.id ? { ...p, queued } : p)) },
        });
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
        const lastReply = cancelled ? st.lastReply : { ...st.lastReply, [d.from]: { id: d.id, from: d.to, fromName: d.toName, elapsedMs: d.elapsedMs ?? 0, at: now } };
        set({
          flights,
          lastReply,
          extra: { ...st.extra, pending: st.extra.pending.filter((x) => x.id !== d.id) },
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
      case 'conversations.paused':
        set({ extra: { ...st.extra, paused: !!d.paused, pause: d.paused ? (d.pause ?? null) : null } });
        return;
      case 'radio.item': {
        const a = toActivity(d as RadioItem);
        radioLive?.push(a);
        set({ activity: mergeActivity(st.activity, [a]) });
        return;
      }
      case 'radio.cleared':
        set({ activity: [], radioMore: false });
        return;
      case 'portal.action': {
        const prev = st.portalActions[d.portal];
        set({ portalActions: { ...st.portalActions, [d.portal]: { ...(d as PortalAction), at: Date.now(), seq: (prev?.seq ?? 0) + 1 } } });
        return;
      }
      case 'cargo.changed': {
        const seen = st.cargoChanges[d.id] ?? {};
        set({ cargoChanges: { ...st.cargoChanges, [d.id]: { ...seen, [d.dir]: (seen[d.dir] ?? 0) + 1 } } });
        return;
      }
      case 'workspace.updated':
        if (ws) set({ workspace: { ...ws, name: d.name, cwd: d.cwd, color: d.color } });
        return;
      case 'turn.card': {
        const c = d as TurnCard;
        set({ cards: { ...st.cards, [c.terminal]: c } });
        return;
      }
      case 'summary.settings':
        set({ summarySettings: d as SummarySettings });
        return;
      case 'usage.flight': {
        const f = d as UsageFlight;
        set({ usage: { ...st.usage, [f.id]: f } });
        return;
      }
      case 'usage.account': {
        const a = d as UsageAccount;
        set({ accounts: { ...st.accounts, [a.agent]: a } });
        return;
      }
      case 'floors.updated': {
        if (!ws) return;
        const floors = d.floors as Floor[];
        const activeFloor = validFloor(st.activeFloor, floors);
        if (activeFloor !== st.activeFloor) st.setFloor(activeFloor);
        set({ workspace: { ...ws, floors, floorConfig: d.config as FloorConfig, copy: (d.copy as Floor | undefined) ?? undefined } });
        return;
      }
      case 'note.edited': {
        const author = d.author === 'external' ? 'Edição externa' : d.author === 'user' ? 'Você' : d.author;
        set({ noteEdits: { ...st.noteEdits, [d.id]: { author, at: Date.now() } } });
        return;
      }
    }
  },

  setConnection: (connection, retryAt) =>
    set((s) => ({
      connection,
      linkLostAt: connection === 'connected' ? 0 : s.linkLostAt || Date.now(),
      retryAt: retryAt ?? s.retryAt,
    })),
  setDrawer: (drawerOpen) => set({ drawerOpen }),
  setComposer: (composerOpen, target, draft, paths) =>
    set({
      composerOpen,
      composerTarget: target === undefined ? get().composerTarget : target,
      composerDraft: draft ?? null,
      composerPaths: paths ?? null,
    }),
  takeComposerDraft: () => {
    const d = get().composerDraft;
    if (d !== null) set({ composerDraft: null });
    return d;
  },
  takeComposerPaths: () => {
    const p = get().composerPaths;
    if (p !== null) set({ composerPaths: null });
    return p;
  },
  loadRadio: () => {
    const ws = get().activeWs;
    radioLive = [];
    api.radio().then(
      (r) => {
        const live = radioLive ?? [];
        radioLive = null;
        if (get().activeWs !== ws) return;
        // The page, and the lines that arrived while it was read.
        set({ activity: mergeActivity(r.items.map(toActivity), live), radioMore: r.more });
      },
      () => {
        radioLive = null;
      },
    );
  },
  loadOlderRadio: () => {
    const st = get();
    const last = st.activity[st.activity.length - 1];
    if (!st.radioMore || !last) return;
    const ws = st.activeWs;
    api.radio(last.time).then(
      (r) => {
        if (get().activeWs !== ws) return;
        set({ activity: mergeActivity(get().activity, r.items.map(toActivity)), radioMore: r.more });
      },
      () => {},
    );
  },
  loadSummaries: () => {
    const ws = get().activeWs;
    api.cards().then(
      (r) => {
        if (get().activeWs !== ws) return;
        set({ cards: Object.fromEntries(r.cards.map((c) => [c.terminal, c])) });
      },
      () => {},
    );
    api.summarySettings().then(
      (s) => get().activeWs === ws && set({ summarySettings: s }),
      () => {},
    );
  },
  loadUsage: () => {
    const ws = get().activeWs;
    api.usage().then(
      (r) => {
        if (get().activeWs !== ws) return;
        set({
          usage: Object.fromEntries(r.flights.map((f) => [f.id, f])),
          accounts: { ...get().accounts, ...Object.fromEntries(r.accounts.map((a) => [a.agent, a])) },
        });
      },
      () => {},
    );
  },
  hideCard: (terminal, at) => set({ hiddenCards: { ...get().hiddenCards, [terminal]: at } }),
  setSummaryDialog: (summaryDialog) => set({ summaryDialog }),
  setSummarySettings: (summarySettings) => set({ summarySettings }),
  // One terminal form at a time: new or settings.
  setNewTerm: (newTermOpen, box) => set(newTermOpen ? { newTermOpen, newTermBox: box ?? null, editTermId: null } : { newTermOpen, newTermBox: null }),
  startPlace: (placeKind) => set({ tool: 'place', placeKind, newTermOpen: false, newTermBox: null }),
  setEditTerm: (editTermId) => set(editTermId ? { editTermId, newTermOpen: false } : { editTermId }),
  setFocusedTerminal: (focusedTerminal) => set({ focusedTerminal }),
  setSelectedNode: (selectedNode) => set({ selectedNode }),
  setTool: (tool) => set(tool === 'place' ? { tool } : { tool, placeKind: null }),
  setDraw: (d) => set((st) => ({ drawShape: d.shape ?? st.drawShape, drawColor: d.color ?? st.drawColor })),
  setEditText: (editText) => set({ editText }),
  setZoom: (z, settled) => {
    const st = get();
    const zoomLevel = levelFor(z, st.zoomLevel);
    if (zoomLevel !== st.zoomLevel) set({ zoomLevel });
    if (settled && z !== st.renderZoom) set({ renderZoom: z });
  },
  // The terminal palette follows the UI theme; it can still be overridden afterwards.
  setTheme: (themePref) => {
    const theme = themePref === 'system' ? (prefersDark() ? 'dark' : 'light') : themePref;
    const termTheme = theme === 'dark' ? 'noite' : 'papel';
    persist('torre.themePref', themePref);
    persist('torre.theme', theme);
    persist('torre.term', termTheme);
    set({ theme, themePref, termTheme });
  },
  setTermTheme: (termTheme) => {
    persist('torre.term', termTheme);
    set({ termTheme });
  },
  moveNodeLocal: (id, x, y) => {
    const ws = get().workspace;
    if (!ws) return;
    set({ workspace: { ...ws, nodes: ws.nodes.map((n) => (n.id === id ? { ...n, x, y } : n)) } });
  },
  moveNodesLocal: (moves) => {
    const ws = get().workspace;
    if (!ws) return;
    const to = new Map(moves.map((m) => [m.id, m]));
    set({ workspace: { ...ws, nodes: ws.nodes.map((n) => (to.has(n.id) ? { ...n, x: to.get(n.id)!.x, y: to.get(n.id)!.y } : n)) } });
  },
}));

/** Follows the open session of the workspace on screen: when another one
 * opens (from here or from another window), its canvas replaces the old one
 * and the events reconnect (App). */
function followSession(ws: string, sid: string | undefined) {
  const st = useSquad.getState();
  if (ws !== st.activeWs || !sid || sid === st.activeSession) return;
  if (st.activeSession === null) {
    useSquad.setState({ activeSession: sid });
    return;
  }
  persist(floorKey(ws), '');
  floorToURL('');
  useSquad.setState({ ...perWorkspace, activeSession: sid, activeFloor: '' });
}

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

/** The ids of the flights asking the user, oldest first; prev when unchanged (a stable list for selectors). */
function askingOf(runtime: Record<string, TerminalRuntime>, prev: string[]): string[] {
  const ids = Object.values(runtime)
    .filter((r) => r.activity === 'ask')
    .sort((a, b) => Date.parse(a.attention?.since ?? a.activitySince) - Date.parse(b.attention?.since ?? b.activitySince))
    .map((r) => r.id);
  return ids.length === prev.length && ids.every((id, i) => id === prev[i]) ? prev : ids;
}
