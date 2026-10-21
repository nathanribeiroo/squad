// Putting a new item on the canvas: its tool is picked (T, N, P, C), then a
// click places it at its usual size, or a drag gives it the size dragged.
import { api } from '../api/client';
import type { WNode } from '../api/types';
import { useSquad } from '../state/store';
import type { IconName } from './icons';
import { toast } from './toast';

export type PlaceKind = 'terminal' | 'note' | 'portal' | 'cargo';

export interface Box {
  x: number;
  y: number;
  w: number;
  h: number;
}

/** Each item: its name, icon and key, the size a click gives it (the
 * server's default) and the least it can be (its node's resizer). */
export const PLACE: Record<PlaceKind, { label: string; icon: IconName; key: string; w: number; h: number; minW: number; minH: number }> = {
  terminal: { label: 'Voo', icon: 'flight_takeoff', key: 'T', w: 560, h: 440, minW: 320, minH: 200 },
  note: { label: 'NOTAM', icon: 'sticky_note_2', key: 'N', w: 400, h: 350, minW: 280, minH: 200 },
  portal: { label: 'Portal', icon: 'public', key: 'P', w: 560, h: 420, minW: 320, minH: 240 },
  cargo: { label: 'Carga', icon: 'inventory_2', key: 'C', w: 340, h: 440, minW: 260, minH: 220 },
};

/** Picks an item's tool, or puts the tool down when it is already picked. */
export function togglePlace(kind: PlaceKind) {
  const st = useSquad.getState();
  if (st.tool === 'place' && st.placeKind === kind) st.setTool('select');
  else st.startPlace(kind);
}

/**
 * Creates the item in the box. A flight first asks what it is (the Novo voo
 * form), and takes off in the box when the form is sent.
 */
export function placeNode(kind: PlaceKind, box: Box) {
  const st = useSquad.getState();
  const b = { x: Math.round(box.x), y: Math.round(box.y), w: Math.round(box.w), h: Math.round(box.h) };
  if (kind === 'terminal') {
    st.setNewTerm(true, b);
    return;
  }
  const body: Partial<WNode> = { kind, ...b, floor: st.activeFloor || undefined };
  // A draft: it opens with its name selected; the name the user gives names the file.
  if (kind === 'note') body.note = { title: '', file: '', draft: true };
  // A cargo opens on the folder the level works in, named after it.
  if (kind === 'cargo') body.cargo = { name: '', path: '' };
  api.addNode(body).then(
    (n) => useSquad.getState().requestSelect(n.id),
    (e: Error) => toast(e.message, 'err'),
  );
}
