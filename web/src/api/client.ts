// REST client. The session cookie (set by /?token=…) authenticates requests.
// A workspace's API lives under /w/<id>/ (the one on screen: setWorkspace);
// the workspace list is at the root.
import type { ShellOption, StateResponse, WEdge, WNode, StateExtra, WorkspaceSummary } from './types';

let base = '';

/** Points the workspace API (api.*, workspace sockets) at workspace id. */
export function setWorkspace(id: string) {
  base = `/w/${encodeURIComponent(id)}`;
}

/** Path inside the workspace on screen. */
export function wsPath(p: string): string {
  return base + p;
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
    try {
      const j = await res.json();
      msg = j.error ?? msg;
    } catch {
      /* not JSON */
    }
    throw new Error(msg);
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
  dirs: (path: string) => req<{ suggestions: string[] }>('GET', wsPath(`/api/dirs?path=${encodeURIComponent(path)}`)),
  shells: () => req<{ shells: ShellOption[] }>('GET', wsPath('/api/shells')),
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
};

export function wsURL(path: string): string {
  const proto = location.protocol === 'https:' ? 'wss:' : 'ws:';
  return `${proto}//${location.host}${path}`;
}
