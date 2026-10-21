import { useEffect, useRef, useState } from 'react';
import { useReactFlow, useStore } from '@xyflow/react';
import { Icon } from '../components/Icon';
import { PLACE, placeNode, type Box, type PlaceKind } from '../lib/place';
import { useSquad } from '../state/store';

type Pt = { x: number; y: number };

/** Positions and sizes land on this step of the canvas (its dots are 20 apart). */
const SNAP = 10;
/** A press that moves less than this (on screen) is a click. */
const CLICK_PX = 5;

const snap = (v: number) => Math.round(v / SNAP) * SNAP;

/** A click: the item at its usual size, centered on the pointer. */
function clickBox(p: Pt, kind: PlaceKind): Box {
  const s = PLACE[kind];
  return { x: snap(p.x - s.w / 2), y: snap(p.y - s.h / 2), w: s.w, h: s.h };
}

/** A drag: the box from where it started, at least the item's smallest size
 * (growing the way the drag went). */
function dragBox(from: Pt, to: Pt, kind: PlaceKind): { box: Box; least: boolean } {
  const s = PLACE[kind];
  const x0 = snap(from.x);
  const y0 = snap(from.y);
  const dw = snap(Math.abs(to.x - x0));
  const dh = snap(Math.abs(to.y - y0));
  const w = Math.max(dw, s.minW);
  const h = Math.max(dh, s.minH);
  return { box: { x: to.x < x0 ? x0 - w : x0, y: to.y < y0 ? y0 - h : y0, w, h }, least: dw < s.minW || dh < s.minH };
}

/**
 * The place tool (T, N, P, C or their buttons): over the canvas, the item
 * follows the pointer at its usual size; a click puts it there, a drag draws
 * it the size wanted. ⇧ keeps the tool for another; Esc puts it down. The
 * wheel still moves and zooms the canvas.
 */
export function PlaceLayer() {
  const tool = useSquad((s) => s.tool);
  const kind = useSquad((s) => s.placeKind);
  const rf = useReactFlow();
  const layer = useRef<HTMLDivElement>(null);
  const [hover, setHover] = useState<Pt | null>(null);
  const [drag, setDrag] = useState<{ from: Pt; to: Pt; sx: number; sy: number; moved: boolean } | null>(null);
  const on = tool === 'place' && !!kind;

  // The wheel pans (⌘/ctrl zooms around the pointer), as on the canvas below.
  useEffect(() => {
    const el = layer.current;
    if (!on || !el) return;
    const wheel = (e: WheelEvent) => {
      e.preventDefault();
      const vp = rf.getViewport();
      if (e.ctrlKey || e.metaKey) {
        const r = el.getBoundingClientRect();
        const z = Math.min(2, Math.max(0.15, vp.zoom * Math.pow(2, -e.deltaY * (e.deltaMode === 1 ? 0.05 : 0.002) * 10)));
        const px = e.clientX - r.left;
        const py = e.clientY - r.top;
        void rf.setViewport({ x: px - ((px - vp.x) * z) / vp.zoom, y: py - ((py - vp.y) * z) / vp.zoom, zoom: z });
      } else {
        void rf.setViewport({ x: vp.x - e.deltaX, y: vp.y - e.deltaY, zoom: vp.zoom });
      }
    };
    el.addEventListener('wheel', wheel, { passive: false });
    return () => el.removeEventListener('wheel', wheel);
  }, [on, rf]);

  if (!on) return null;
  const at = (e: React.PointerEvent): Pt => rf.screenToFlowPosition({ x: e.clientX, y: e.clientY });

  const down = (e: React.PointerEvent) => {
    if (e.button !== 0) return;
    // No text selection and no native drag: either would take the press over
    // (a pointercancel, or a release that never comes), leaving the item on
    // the pointer.
    e.preventDefault();
    window.getSelection()?.removeAllRanges();
    try {
      e.currentTarget.setPointerCapture(e.pointerId);
    } catch {
      // Without the capture, a release outside is caught by the next move.
    }
    const p = at(e);
    setDrag({ from: p, to: p, sx: e.clientX, sy: e.clientY, moved: false });
  };
  const move = (e: React.PointerEvent) => {
    const p = at(e);
    setHover(p);
    if (!drag) return;
    // The button is up but no release arrived: it was released.
    if (!(e.buttons & 1)) return up(e);
    setDrag({ ...drag, to: p, moved: drag.moved || Math.hypot(e.clientX - drag.sx, e.clientY - drag.sy) > CLICK_PX });
  };
  const up = (e: React.PointerEvent) => {
    if (!drag) return;
    setDrag(null);
    const box = drag.moved ? dragBox(drag.from, drag.to, kind).box : clickBox(drag.from, kind);
    // ⇧ keeps the tool for another one (a flight asks first, so it doesn't).
    if (!e.shiftKey || kind === 'terminal') useSquad.getState().setTool('select');
    placeNode(kind, box);
  };

  let ghost: { box: Box; least: boolean } | null = null;
  if (drag?.moved) ghost = dragBox(drag.from, drag.to, kind);
  else if (drag) ghost = { box: clickBox(drag.from, kind), least: false };
  else if (hover) ghost = { box: clickBox(hover, kind), least: false };

  return (
    <>
      <div
        ref={layer}
        className="place-layer"
        onPointerDown={down}
        onPointerMove={move}
        onPointerUp={up}
        onPointerLeave={() => !drag && setHover(null)}
        onPointerCancel={() => setDrag(null)}
      >
        {ghost && <Ghost kind={kind} box={ghost.box} least={ghost.least} sizing={!!drag?.moved} />}
      </div>
      <PlaceHint kind={kind} />
    </>
  );
}

/** The item about to be placed, on screen: dashed, with its name and size. */
function Ghost({ kind, box, least, sizing, note }: { kind: PlaceKind; box: Box; least: boolean; sizing: boolean; note?: string }) {
  const [tx, ty, zoom] = useStore((s) => s.transform);
  const spec = PLACE[kind];
  const w = box.w * zoom;
  const h = box.h * zoom;
  const tip = note ?? (sizing ? (least ? 'tamanho mínimo' : 'solte para criar') : 'clique para pôr aqui · ou arraste para escolher o tamanho');
  return (
    <div className={`pl-ghost${sizing ? ' sizing' : ''}${note ? ' pending' : ''}`} style={{ left: box.x * zoom + tx, top: box.y * zoom + ty, width: w, height: h }}>
      <div className="pl-ghost-head">
        <Icon name={spec.icon} size={14} />
        <span>{spec.label}</span>
        <span className="pl-ghost-size mono">
          {Math.round(box.w)} × {Math.round(box.h)}
        </span>
      </div>
      {w > 150 && h > 70 && <div className="pl-ghost-tip">{tip}</div>}
    </div>
  );
}

/** Beside the item's button in the tools: how to use it, and leaving (Esc). */
function PlaceHint({ kind }: { kind: PlaceKind }) {
  const at = (['terminal', 'note', 'portal', 'cargo'] as PlaceKind[]).indexOf(kind);
  const spec = PLACE[kind];
  return (
    <div className="draw-bar place-hint" role="status" style={{ top: 92 + at * 34 }}>
      <span className="draw-bar-tag">{spec.label}</span>
      <span className="draw-bar-hint">
        Clique no canvas para pôr, ou arraste para escolher o tamanho.
        {kind !== 'terminal' && <span className="place-hint-more"> ⇧ põe outro em seguida.</span>}
      </span>
      <button className="tbx-btn" onClick={() => useSquad.getState().setTool('select')} aria-label="Cancelar" title="Cancelar (Esc)">
        <Icon name="close" size={18} />
      </button>
    </div>
  );
}

/** Where the new flight takes off, while its form is open. */
export function PendingFlight() {
  const box = useSquad((s) => (s.newTermOpen ? s.newTermBox : null));
  if (!box) return null;
  return (
    <div className="place-pending">
      <Ghost kind="terminal" box={box} least={false} sizing={false} note="o voo decola aqui" />
    </div>
  );
}
