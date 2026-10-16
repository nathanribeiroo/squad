// REST client. The session cookie (set by /?token=…) authenticates requests.
// A workspace's API lives under /w/<id>/ (the one on screen: setWorkspace);
// the workspace list is at the root.
import type {
  Floor,
  FloorChanges,
  FloorConfig,
  FloorStatusResponse,
  LandResult,
  RoleDef,
  ShellOption,
  StateResponse,
  WEdge,
  WNode,
  StateExtra,
  WorkspaceSummary,
} from './types';

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
      msg = (data.error as string) ?? msg;
    } catch {
      /* not JSON */
    }
    throw new ApiError(msg, res.status, data);
  }
  if (res.status === 204) return undefined as T;
  return (await res.json()) as T;
}

export const api = {
  state: () => req<StateResponse>('GET', wsPath('/api/state')),
  addNode: (n: Partial<WNode>) => req<WNode>('POST', wsPath('/api/nodes'), n),
  updateNode: (id: string, patch: Partial<WNode>) => req<WNode>('PATCH', wsPath(`/api/nodes/${id}`), patch),
  removeNode: (id: string) => req<void>('DELETE', wsPath(`/api/nodes/${id}`)),
  nodeAction: (id: string, action: 'start' | 'stop' | 'restart') => req<unknown>('POST', wsPath(`/api/nodes/${id}/${action}`)),
  addEdge: (a: string, b: string) => req<WEdge>('POST', wsPath('/api/edges'), { a, b }),
  updateEdge: (id: string, ropePoints: [number, number][]) => req<WEdge>('PATCH', wsPath(`/api/edges/${id}`), { ropePoints }),
  removeEdge: (id: string) => req<void>('DELETE', wsPath(`/api/edges/${id}`)),
  send: (id: string, body: { text: string; mentions: string[]; images: { name: string; data: string }[] }) =>
    req<{ ok: boolean }>('POST', wsPath(`/api/nodes/${id}/send`), body),
  pause: () => req<StateExtra>('POST', wsPath('/api/conversations/pause')),
  resume: () => req<StateExtra>('POST', wsPath('/api/conversations/resume')),
  note: (id: string) => req<{ id: string; file: string; title: string; locked: boolean; content: string }>('GET', wsPath(`/api/notes/${id}`)),
  saveNote: (id: string, content: string) => req<{ ok: boolean }>('PUT', wsPath(`/api/notes/${id}`), { content }),
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
  floorLog: async (id: string) => {
    const res = await fetch(wsPath(`/api/floors/${encodeURIComponent(id)}/log`), { credentials: 'same-origin' });
    if (!res.ok) throw new ApiError(res.statusText, res.status, {});
    return res.text();
  },
};

/** The workspace list (not tied to the workspace on screen). */
export const hub = {
  list: () => req<WorkspaceSummary[]>('GET', '/api/workspaces'),
  create: (b: { name?: string; cwd: string; color?: string }) => req<WorkspaceSummary>('POST', '/api/workspaces', b),
  update: (id: string, p: { name?: string; cwd?: string; color?: string }) =>
    req<WorkspaceSummary>('PATCH', `/api/workspaces/${encodeURIComponent(id)}`, p),
  remove: (id: string) => req<void>('DELETE', `/api/workspaces/${encodeURIComponent(id)}`),
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
