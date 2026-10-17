import type { WNode } from '../api/types';
import { useSquad } from '../state/store';

/**
 * Where a flight's screen is drawn: a terminal has one screen (one xterm), so
 * the Foco (⌥F) takes it first, then the edge it is docked at (⌥⇧←/→, in the
 * Espaço), and otherwise its node.
 */
export type ScreenPlace = 'focus' | 'dock' | 'node';

export function useScreenPlace(id: string): ScreenPlace {
  return useSquad((s) => (s.focusPane === id ? 'focus' : s.view === 'space' && s.dock?.id === id ? 'dock' : 'node'));
}

/** Whether a flight's screen is hidden (⋯ → Ocultar a tela). */
export function useScreenHidden(id: string): boolean {
  return useSquad((s) => s.hiddenScreens.includes(id));
}

/** The flight ⌥F and ⌥⇧←/→ act on: the one with the keyboard, the one selected, then the one on screen. */
function paneTarget(): string | null {
  const st = useSquad.getState();
  const isTerm = (id: string | null | undefined): id is string => !!id && !!st.workspace?.nodes.some((n) => n.id === id && n.terminal);
  return [st.focusedTerminal, st.selectedNode, st.focusPane, st.dock?.id].find(isTerm) ?? null;
}

/** ⌥F: the flight on the whole stage, and back. */
export function toggleFocus(id = paneTarget()) {
  const st = useSquad.getState();
  if (st.focusPane) return st.setFocusPane(null);
  if (id) st.setFocusPane(id);
}

/** ⌥⇧← / ⌥⇧→: docks the flight at an edge; the same keys again undock it. */
export function dockAt(side: 'left' | 'right', id = paneTarget()) {
  const st = useSquad.getState();
  if (!id) return;
  if (st.view !== 'space') st.setView('space');
  if (st.dock?.id === id && st.dock.side === side) return st.setDock(null);
  st.setDock({ id, side });
}

/**
 * Tab / ⇧Tab: the next node of the level on screen, in reading order (rows
 * from the top, then left to right).
 */
export function nextNode(back: boolean) {
  const st = useSquad.getState();
  const nodes = (st.workspace?.nodes ?? []).filter((n) => (n.floor ?? '') === st.activeFloor);
  if (nodes.length === 0) return;
  const order = [...nodes].sort(readingOrder);
  const i = order.findIndex((n) => n.id === st.selectedNode);
  const next = i < 0 ? (back ? order.length - 1 : 0) : (i + (back ? -1 : 1) + order.length) % order.length;
  st.requestSelect(order[next].id);
}

// Nodes whose tops are within a row of each other read left to right.
const ROW = 160;

function readingOrder(a: WNode, b: WNode): number {
  const ra = Math.round(a.y / ROW);
  const rb = Math.round(b.y / ROW);
  return ra !== rb ? ra - rb : a.x - b.x;
}
