import { api } from '../api/client';
import type { DrawingData, WNode } from '../api/types';
import { useSquad } from '../state/store';
import { ROLE_COLORS } from './roles';
import { toast } from './toast';

// Groups and drawings (internal/store/visual.go): for the user's eyes only.
// A group holds whatever lies whole inside its frame, at the moment: moving
// it moves them, collapsing it hides them behind a card. Agents never see
// either, and no wire reaches them.

/** A collapsed group's card. */
export const CARD_W = 300;
export const CARD_H = 104;
/** Room a new group leaves around what it frames, and its header. */
const PAD = 32;
const HEAD = 36;

/** Colors a group or a drawing may take: none (the text's), amber, the roles'. */
export const VISUAL_COLORS = ['none', 'amber', ...ROLE_COLORS] as const;

/** The CSS color of a group's or a drawing's color key. */
export function colorVar(key?: string): string {
  if (!key || key === 'none') return 'var(--rt-none)';
  if (key === 'amber') return 'var(--st-wait)';
  return `var(--r-${key})`;
}

export const isVisual = (n: WNode) => n.kind === 'group' || n.kind === 'drawing';

/** The node's frame: a collapsed group's open one. */
export function boxOf(n: WNode): { x: number; y: number; w: number; h: number } {
  if (n.group?.collapsed && n.group.openW && n.group.openH) return { x: n.x, y: n.y, w: n.group.openW, h: n.group.openH };
  return { x: n.x, y: n.y, w: n.w, h: n.h };
}

/** What lies whole inside a group's frame, on its level; a collapsed group's are its members (store.Inside). */
export function insideGroup(nodes: WNode[], g: WNode): WNode[] {
  const floor = g.floor ?? '';
  if (g.group?.collapsed) {
    const m = new Set(g.group.members ?? []);
    return nodes.filter((n) => m.has(n.id) && (n.floor ?? '') === floor);
  }
  const f = boxOf(g);
  return nodes.filter((n) => {
    if (n.id === g.id || (n.floor ?? '') !== floor) return false;
    const b = boxOf(n);
    return b.x >= f.x && b.y >= f.y && b.x + b.w <= f.x + f.w && b.y + b.h <= f.y + f.h;
  });
}

/** For each node a collapsed group hides (on its level), that group's id. */
export function hiddenBy(nodes: WNode[]): Map<string, string> {
  const floorOf = new Map(nodes.map((n) => [n.id, n.floor ?? '']));
  const out = new Map<string, string>();
  for (const g of nodes) {
    if (!g.group?.collapsed) continue;
    for (const m of g.group.members ?? []) if (m !== g.id && floorOf.get(m) === (g.floor ?? '')) out.set(m, g.id);
  }
  return out;
}

/** The node a hidden one shows through (the card of the outermost group hiding it); id itself when nothing hides it. */
export function shownAs(hidden: Map<string, string>, id: string): string {
  let at = id;
  for (let i = 0; i < 20 && hidden.has(at); i++) at = hidden.get(at)!;
  return at;
}

/** The collapsed groups hiding id, and the ones hiding those. */
function hidersOf(nodes: WNode[], id: string): WNode[] {
  const out: WNode[] = [];
  const seen = new Set([id]);
  for (let i = 0, todo = [id]; i < todo.length; i++) {
    for (const g of nodes) {
      if (g.group?.collapsed && !seen.has(g.id) && g.group.members?.includes(todo[i])) {
        seen.add(g.id);
        out.push(g);
        todo.push(g.id);
      }
    }
  }
  return out;
}

/** Collapses a group into a card: what is inside hides behind it. */
export function collapse(g: WNode) {
  const nodes = useSquad.getState().workspace?.nodes ?? [];
  const members = insideGroup(nodes, g).map((n) => n.id);
  void api
    .updateNode(g.id, { w: CARD_W, h: CARD_H, group: { ...g.group!, collapsed: true, members, openW: g.w, openH: g.h } })
    .catch((e: Error) => toast(e.message, 'err'));
}

/** Opens a collapsed group: its frame and its nodes come back. */
export function expand(g: WNode): Promise<unknown> {
  const gr = g.group!;
  return api
    .updateNode(g.id, { w: gr.openW || 720, h: gr.openH || 480, group: { name: gr.name, color: gr.color } })
    .catch((e: Error) => toast(e.message, 'err'));
}

/** Opens the groups hiding a node (to go to it); whether any was closed. */
export async function reveal(id: string): Promise<boolean> {
  const hiders = hidersOf(useSquad.getState().workspace?.nodes ?? [], id);
  await Promise.all(hiders.map(expand));
  return hiders.length > 0;
}

/** "Grupo N", the next number not taken on the canvas. */
function newGroupName(nodes: WNode[]): string {
  const taken = new Set(nodes.map((n) => n.group?.name));
  for (let i = 1; ; i++) if (!taken.has(`Grupo ${i}`)) return `Grupo ${i}`;
}

/** Makes a group at x, y, w × h on the level on screen. */
export function addGroup(box: { x: number; y: number; w: number; h: number }) {
  const st = useSquad.getState();
  const nodes = st.workspace?.nodes ?? [];
  return api
    .addNode({ kind: 'group', ...roundBox(box), floor: st.activeFloor || undefined, group: { name: newGroupName(nodes), color: 'none' } })
    .catch((e: Error) => {
      toast(e.message, 'err');
      return null;
    });
}

/**
 * G: a frame around the selected nodes; with nothing selected, the next
 * drag on the canvas draws one.
 */
export function groupSelection() {
  const st = useSquad.getState();
  const nodes = st.workspace?.nodes ?? [];
  const sel = nodes.filter((n) => st.selectedIds.includes(n.id) && (n.floor ?? '') === st.activeFloor);
  if (sel.length === 0) {
    st.setTool(st.tool === 'frame' ? 'select' : 'frame');
    return;
  }
  const boxes = sel.map(boxOf);
  const x = Math.min(...boxes.map((b) => b.x)) - PAD;
  const y = Math.min(...boxes.map((b) => b.y)) - PAD - HEAD;
  const right = Math.max(...boxes.map((b) => b.x + b.w)) + PAD;
  const bottom = Math.max(...boxes.map((b) => b.y + b.h)) + PAD;
  void addGroup({ x, y, w: right - x, h: bottom - y });
}

/** Makes a drawing on the level on screen. */
export function addDrawing(box: { x: number; y: number; w: number; h: number }, drawing: DrawingData) {
  const st = useSquad.getState();
  return api.addNode({ kind: 'drawing', ...roundBox(box), floor: st.activeFloor || undefined, drawing }).catch((e: Error) => {
    toast(e.message, 'err');
    return null;
  });
}

function roundBox(b: { x: number; y: number; w: number; h: number }) {
  return { x: Math.round(b.x), y: Math.round(b.y), w: Math.max(1, Math.round(b.w)), h: Math.max(1, Math.round(b.h)) };
}

/** Removes groups and drawings (Delete on the canvas); a group's nodes stay. */
export function removeVisuals(ids: string[]) {
  for (const id of ids) api.removeNode(id).catch((e: Error) => toast(e.message, 'err'));
}
