// REST client. The session cookie (set by /?token=…) authenticates requests.
// A workspace's API lives under /w/<id>/ (the one on screen: setWorkspace);
// the workspace list is at the root.
import type {
  Floor,
  FloorChanges,
  FloorConfig,
  FloorStatusResponse,
  LandResult,
  SummarySettings,
  TurnCard,
  WorkspaceOverview,
  RoleDef,
  SessionList,
  ShellOption,
  StateResponse,
  WEdge,
  WNode,
  StateExtra,
  WorkspaceSummary,
  AttentionChoice,
  AgentsDetection,
  DirInfo,
  Limits,
  LimitsView,
} from './types';
import { errorMessage } from './errors';

let base = '';

/** Points the workspace API (api.*, workspace sockets) at workspace id. */
export function setWorkspace(id: string) {
  base = `/w/${encodeURIComponent(id)}`;
}

/** Path inside the workspace on screen. */
export function wsPath(p: string): string {
  return base + p;
}

/** A refused request: the server's message, its status and the whole body
 * (a refused land carries "files" or "conflicts"). */
export class ApiError extends Error {
  constructor(
    message: string,
    readonly status: number,
    readonly body: Record<string, unknown>,
  ) {
    super(message);
  }
}

async function req<T>(method: string, path: string, body?: unknown): Promise<T> {
  const res = await fetch(path, {
    method,
    headers: body !== undefined ? { 'Content-Type': 'application/json' } : undefined,
    body: body !== undefined ? JSON.stringify(body) : undefined,
    credentials: 'same-origin',
  });
  if (!res.ok) {
    let msg = res.statusText;
    let data: Record<string, unknown> = {};
    try {
      data = await res.json();
      msg = errorMessage(data, msg);
    } catch {
      /* not JSON */
    }
    throw new ApiError(msg, res.status, data);
  }
  if (res.status === 204) return undefined as T;
  return (await res.json()) as T;
}

async function text(path: string): Promise<string> {
  const res = await fetch(path, { credentials: 'same-origin' });
  if (!res.ok) throw new ApiError(res.statusText, res.status, {});
  return res.text();
}

export const api = {
  state: () => req<StateResponse>('GET', wsPath('/api/state')),
  addNode: (n: Partial<WNode>) => req<WNode>('POST', wsPath('/api/nodes'), n),
  updateNode: (id: string, patch: Partial<WNode>) => req<WNode>('PATCH', wsPath(`/api/nodes/${id}`), patch),
  removeNode: (id: string) => req<void>('DELETE', wsPath(`/api/nodes/${id}`)),
  nodeAction: (id: string, action: 'start' | 'stop' | 'restart' | 'new-conversation') => req<unknown>('POST', wsPath(`/api/nodes/${id}/${action}`)),
  /** Answers the prompt on a flight's screen (only the interface can). */
  limits: () => req<LimitsView>('GET', wsPath('/api/limits')),
  setLimits: (l: Partial<Limits>) => req<LimitsView>('PUT', wsPath('/api/limits'), l),
  respond: (id: string, choice: AttentionChoice) => req<void>('POST', wsPath(`/api/nodes/${id}/attention/respond`), { choice }),
  /** Agents interrupted mid-turn: send them "continue", or forget it (all, or ids). */
  interrupted: (action: 'continue' | 'dismiss', ids?: string[]) => req<{ ids: string[] }>('POST', wsPath(`/api/interrupted/${action}`), { ids }),
  addEdge: (a: string, b: string) => req<WEdge>('POST', wsPath('/api/edges'), { a, b }),
  updateEdge: (id: string, ropePoints: [number, number][]) => req<WEdge>('PATCH', wsPath(`/api/edges/${id}`), { ropePoints }),
  removeEdge: (id: string) => req<void>('DELETE', wsPath(`/api/edges/${id}`)),
  send: (id: string, body: { text: string; mentions: string[]; images: { name: string; data: string }[] }) =>
    req<{ ok: boolean }>('POST', wsPath(`/api/nodes/${id}/send`), body),
  /** A recent request's full message (the Rádio shows a preview). */
  requestText: (id: string) => req<{ id: string; text: string }>('GET', wsPath(`/api/requests/${id}`)),
  /** Ends a request for the user: the agent that asked gets its answer now. */
  cancelRequest: (id: string) => req<void>('POST', wsPath(`/api/requests/${id}/cancel`)),
  pause: () => req<StateExtra>('POST', wsPath('/api/conversations/pause')),
  resume: () => req<StateExtra>('POST', wsPath('/api/conversations/resume')),
  note: (id: string) => req<{ id: string; file: string; title: string; locked: boolean; content: string }>('GET', wsPath(`/api/notes/${id}`)),
  saveNote: (id: string, content: string) => req<{ ok: boolean }>('PUT', wsPath(`/api/notes/${id}`), { content }),
  /** Names a note the user made (a draft gets its file); the old name stays valid for agents. */
  renameNote: (id: string, title: string) => req<WNode>('PATCH', wsPath(`/api/notes/${id}`), { title }),
  /** Folder suggestions, relative to the workspace folder or to a floor's copy of it. */
  dirs: (path: string, floor?: string) =>
    req<{ suggestions: string[] }>('GET', wsPath(`/api/dirs?path=${encodeURIComponent(path)}${floor ? `&floor=${encodeURIComponent(floor)}` : ''}`)),
  shells: () => req<{ shells: ShellOption[] }>('GET', wsPath('/api/shells')),

  floorStatus: () => req<FloorStatusResponse>('GET', wsPath('/api/floors/status')),
  branches: () => req<{ branches: string[] }>('GET', wsPath('/api/branches')),
  createFloor: (b: { name: string; branch?: string; existing?: boolean }) => req<Floor>('POST', wsPath('/api/floors'), b),
  floorConfig: (c: FloorConfig) => req<FloorConfig>('PUT', wsPath('/api/floors/config'), c),
  floorChanges: (id: string) => req<FloorChanges>('GET', wsPath(`/api/floors/${encodeURIComponent(id)}/changes`)),
  landFloor: (id: string, b: { merge?: boolean; into?: string }) => req<LandResult>('POST', wsPath(`/api/floors/${encodeURIComponent(id)}/land`), b),
  discardFloor: (id: string) => req<void>('DELETE', wsPath(`/api/floors/${encodeURIComponent(id)}`)),
  /** Another name for a level; its branch and folder stay. */
  renameFloor: (id: string, name: string) => req<Floor>('PATCH', wsPath(`/api/floors/${encodeURIComponent(id)}`), { name }),
  cards: () => req<{ cards: TurnCard[] }>('GET', wsPath('/api/summary')),
  summarize: (id: string) => req<TurnCard>('POST', wsPath(`/api/nodes/${id}/summarize`)),
  overview: () => req<WorkspaceOverview>('POST', wsPath('/api/summary/workspace')),
  summarySettings: () => req<SummarySettings>('GET', wsPath('/api/summary/settings')),
  setSummarySettings: (s: Pick<SummarySettings, 'cards' | 'engine'>) => req<SummarySettings>('PUT', wsPath('/api/summary/settings'), s),
  floorLog: (id: string) => text(wsPath(`/api/floors/${encodeURIComponent(id)}/log`)),
  /** The session's copy: what merging would take, merge (the copy stays), the setup log. */
  copyChanges: () => req<FloorChanges>('GET', wsPath('/api/copy/changes')),
  mergeCopy: (into: string) => req<LandResult>('POST', wsPath('/api/copy/merge'), { into }),
  copyLog: () => text(wsPath('/api/copy/log')),
};

/** The workspace list (not tied to the workspace on screen). */
export const hub = {
  welcome: () => req<{ firstRun: boolean }>('GET', '/api/welcome'),
  dirInfo: (path: string) => req<DirInfo>('GET', `/api/dirs/info?path=${encodeURIComponent(path)}`),
  agents: (refresh = false) => req<AgentsDetection>('GET', `/api/agents${refresh ? '?refresh=1' : ''}`),
  setAgentCommand: (agent: string, defaultCommand: string) => req<AgentsDetection>('PUT', `/api/agents/${encodeURIComponent(agent)}`, { defaultCommand }),
  list: () => req<WorkspaceSummary[]>('GET', '/api/workspaces'),
  /** session names the workspace's first session. */
  create: (b: { name?: string; cwd: string; color?: string; session?: string }) => req<WorkspaceSummary>('POST', '/api/workspaces', b),
  update: (id: string, p: { name?: string; cwd?: string; color?: string }) =>
    req<WorkspaceSummary>('PATCH', `/api/workspaces/${encodeURIComponent(id)}`, p),
  remove: (id: string) => req<void>('DELETE', `/api/workspaces/${encodeURIComponent(id)}`),
  sessions: (ws: string) => req<SessionList>('GET', `/api/workspaces/${encodeURIComponent(ws)}/sessions`),
  /** Leaves the open session for a new one, in the workspace folder or in a
   * copy (a new branch from a branch). Refused with 409 ("working",
   * "pending") while agents work, unless confirm. */
  newSession: (ws: string, b: { name?: string; keepTerminals?: boolean; confirm?: boolean; copy?: { branch?: string; from?: string } }) =>
    req<SessionList>('POST', `/api/workspaces/${encodeURIComponent(ws)}/sessions`, b),
  /** Leaves the open session for session id (409 like newSession). */
  openSession: (ws: string, id: string, confirm?: boolean) =>
    req<SessionList>('POST', `/api/workspaces/${encodeURIComponent(ws)}/sessions/${encodeURIComponent(id)}/open`, { confirm: !!confirm }),
  renameSession: (ws: string, id: string, name: string) =>
    req<SessionList>('PATCH', `/api/workspaces/${encodeURIComponent(ws)}/sessions/${encodeURIComponent(id)}`, { name }),
  /** Refused with 409 ("files") when its copy has uncommitted changes, unless confirm. */
  removeSession: (ws: string, id: string, confirm?: boolean) =>
    req<SessionList>('DELETE', `/api/workspaces/${encodeURIComponent(ws)}/sessions/${encodeURIComponent(id)}${confirm ? '?confirm=true' : ''}`),
  /** Folder suggestions from the home folder (for a new workspace). */
  dirs: (path: string) => req<{ suggestions: string[] }>('GET', `/api/dirs?path=${encodeURIComponent(path)}`),
  createRole: (r: Omit<RoleDef, 'id'>) => req<RoleDef>('POST', '/api/roles', r),
  updateRole: (r: RoleDef) => req<RoleDef>('PUT', `/api/roles/${encodeURIComponent(r.id)}`, r),
  resetRole: (id: string) => req<RoleDef>('POST', `/api/roles/${encodeURIComponent(id)}/reset`),
  removeRole: (id: string) => req<void>('DELETE', `/api/roles/${encodeURIComponent(id)}`),
};

export function wsURL(path: string): string {
  const proto = location.protocol === 'https:' ? 'wss:' : 'ws:';
  return `${proto}//${location.host}${path}`;
}
