// REST client. The session cookie (set by /?token=…) authenticates requests.
import type { ShellOption, StateResponse, WEdge, WNode, StateExtra } from './types';

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
  state: () => req<StateResponse>('GET', '/api/state'),
  addNode: (n: Partial<WNode>) => req<WNode>('POST', '/api/nodes', n),
  updateNode: (id: string, patch: Partial<WNode>) => req<WNode>('PATCH', `/api/nodes/${id}`, patch),
  removeNode: (id: string) => req<void>('DELETE', `/api/nodes/${id}`),
  nodeAction: (id: string, action: 'start' | 'stop' | 'restart') => req<unknown>('POST', `/api/nodes/${id}/${action}`),
  addEdge: (a: string, b: string) => req<WEdge>('POST', '/api/edges', { a, b }),
  updateEdge: (id: string, ropePoints: [number, number][]) => req<WEdge>('PATCH', `/api/edges/${id}`, { ropePoints }),
  removeEdge: (id: string) => req<void>('DELETE', `/api/edges/${id}`),
  send: (id: string, body: { text: string; mentions: string[]; images: { name: string; data: string }[] }) =>
    req<{ ok: boolean }>('POST', `/api/nodes/${id}/send`, body),
  pause: () => req<StateExtra>('POST', '/api/conversations/pause'),
  resume: () => req<StateExtra>('POST', '/api/conversations/resume'),
  note: (id: string) => req<{ id: string; file: string; title: string; locked: boolean; content: string }>('GET', `/api/notes/${id}`),
  saveNote: (id: string, content: string) => req<{ ok: boolean }>('PUT', `/api/notes/${id}`, { content }),
  dirs: (path: string) => req<{ suggestions: string[] }>('GET', `/api/dirs?path=${encodeURIComponent(path)}`),
  shells: () => req<{ shells: ShellOption[] }>('GET', '/api/shells'),
};

export function wsURL(path: string): string {
  const proto = location.protocol === 'https:' ? 'wss:' : 'ws:';
  return `${proto}//${location.host}${path}`;
}
