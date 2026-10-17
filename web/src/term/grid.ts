import { termSession } from './session';

// Fixed chrome of the live node, in world px (docs/torre/TELAS.md, quadro
// 02): borders, the state band, header, footer and the body padding around
// the terminal grid.
const CHROME_W = 2 + 6 + 20;
const CHROME_H = 2 + 34 + 22 + 16;

/** Terminal grid that fits a node of w×h world px. */
export function gridFor(id: string, w: number, h: number): { cols: number; rows: number } | null {
  return gridIn(id, w - CHROME_W, h - CHROME_H);
}

/** Terminal grid that fits w×h px at the base type size. */
export function gridIn(id: string, w: number, h: number): { cols: number; rows: number } | null {
  const cell = termSession(id).cellSize();
  if (!cell) return null;
  return { cols: Math.max(20, Math.floor(w / cell.w)), rows: Math.max(5, Math.floor(h / cell.h)) };
}
