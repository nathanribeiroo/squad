import { useRef, useState } from 'react';
import { useReactFlow, useStore } from '@xyflow/react';
import type { DrawingData, DrawingShape } from '../api/types';
import { Icon } from '../components/Icon';
import type { IconName } from '../lib/icons';
import { VISUAL_COLORS, addDrawing, addGroup, colorVar } from '../lib/visual';
import { useSquad } from '../state/store';
import { Shape, STROKE, TEXT_SIZE } from './nodes/DrawingNode';

type Pt = [number, number];

/** Most points a stroke keeps (the server takes 2000). */
const MAX_POINTS = 1500;

/**
 * Over the canvas while drawing (Desenho, D) or framing a new group (G with
 * nothing selected): a drag draws the shape picked in the bar, a click with
 * Texto starts a text. Each stroke becomes a drawing node; the tool stays on
 * until Esc or V.
 */
export function DrawLayer() {
  const tool = useSquad((s) => s.tool);
  const shape = useSquad((s) => s.drawShape);
  const color = useSquad((s) => s.drawColor);
  const zoom = useStore((s) => s.transform[2]);
  const rf = useReactFlow();
  const layer = useRef<HTMLDivElement>(null);
  // Points of the stroke being drawn: on screen (for the preview) and on the canvas.
  const [live, setLive] = useState<{ screen: Pt[]; flow: Pt[] } | null>(null);
  if (tool !== 'draw' && tool !== 'frame') return null;
  const framing = tool === 'frame';

  const at = (e: React.PointerEvent): { screen: Pt; flow: Pt } => {
    const r = layer.current!.getBoundingClientRect();
    const f = rf.screenToFlowPosition({ x: e.clientX, y: e.clientY });
    return { screen: [e.clientX - r.left, e.clientY - r.top], flow: [f.x, f.y] };
  };
  const down = (e: React.PointerEvent) => {
    if (e.button !== 0) return;
    e.currentTarget.setPointerCapture(e.pointerId);
    const p = at(e);
    setLive({ screen: [p.screen], flow: [p.flow] });
  };
  const move = (e: React.PointerEvent) => {
    if (!live) return;
    const p = at(e);
    if (!framing && shape === 'pen') {
      const [lx, ly] = live.screen[live.screen.length - 1];
      if (Math.hypot(p.screen[0] - lx, p.screen[1] - ly) < 2) return;
      setLive({ screen: [...live.screen, p.screen], flow: [...live.flow, p.flow] });
    } else {
      setLive({ screen: [live.screen[0], p.screen], flow: [live.flow[0], p.flow] });
    }
  };
  const up = () => {
    if (!live) return;
    setLive(null);
    finish(live.flow, live.screen);
  };

  const finish = (flow: Pt[], screen: Pt[]) => {
    const [a, b = a] = [flow[0], flow[flow.length - 1]];
    const dragged = screen.length > 1 && Math.hypot(screen[screen.length - 1][0] - screen[0][0], screen[screen.length - 1][1] - screen[0][1]) > 4;
    if (framing) {
      const box = boxOf([a, b], 0);
      if (box.w >= 80 && box.h >= 60) void addGroup(box);
      useSquad.getState().setTool('select');
      return;
    }
    const d: DrawingData = { shape, color };
    switch (shape) {
      case 'text': {
        void addDrawing({ x: a[0], y: a[1] - TEXT_SIZE, w: 260, h: 34 }, { ...d, text: '' }).then((n) => n && useSquad.getState().setEditText(n.id));
        return;
      }
      case 'pen': {
        if (!dragged) return;
        const pts = thin(flow);
        const box = boxOf(pts, STROKE * 2);
        void addDrawing(box, { ...d, points: pts.map((p) => norm(p, box)) });
        return;
      }
      case 'arrow': {
        if (!dragged) return;
        const box = boxOf([a, b], 18);
        void addDrawing(box, { ...d, points: [norm(a, box), norm(b, box)] });
        return;
      }
      default: {
        const box = boxOf([a, b], 0);
        if (box.w >= 6 && box.h >= 6) void addDrawing(box, d);
      }
    }
  };

  // The preview, on screen.
  let preview: React.ReactNode = null;
  if (live && live.screen.length > 1) {
    const box = boxOf(live.screen, 0);
    const d: DrawingData = { shape: framing ? 'rect' : shape, color };
    if (framing) {
      preview = <rect className="dl-frame" x={box.x} y={box.y} width={box.w} height={box.h} />;
    } else if (shape === 'pen' || shape === 'arrow') {
      const pb = boxOf(shape === 'pen' ? live.screen : [live.screen[0], live.screen[live.screen.length - 1]], 1);
      const pts = shape === 'pen' ? live.screen : [live.screen[0], live.screen[live.screen.length - 1]];
      preview = (
        <g transform={`translate(${pb.x} ${pb.y})`}>
          <Shape d={{ ...d, points: pts.map((p) => norm(p, pb)) }} w={pb.w} h={pb.h} scale={zoom} />
        </g>
      );
    } else if (shape !== 'text') {
      preview = (
        <g transform={`translate(${box.x} ${box.y})`}>
          <Shape d={d} w={box.w} h={box.h} scale={zoom} />
        </g>
      );
    }
  }

  return (
    <>
      <div
        ref={layer}
        className={`draw-layer${framing ? ' framing' : ''}${shape === 'text' && !framing ? ' texting' : ''}`}
        style={{ '--dc': colorVar(color) } as React.CSSProperties}
        onPointerDown={down}
        onPointerMove={move}
        onPointerUp={up}
        onPointerCancel={() => setLive(null)}
      >
        <svg className="dl-svg">{preview}</svg>
      </div>
      {framing ? <FrameHint /> : <DrawBar />}
    </>
  );
}

/** The box around points, with room around them. */
function boxOf(pts: Pt[], pad: number) {
  const xs = pts.map((p) => p[0]);
  const ys = pts.map((p) => p[1]);
  const x = Math.min(...xs) - pad;
  const y = Math.min(...ys) - pad;
  return { x, y, w: Math.max(...xs) + pad - x, h: Math.max(...ys) + pad - y };
}

/** A point 0–1 across a box. */
function norm([x, y]: Pt, b: { x: number; y: number; w: number; h: number }): Pt {
  const r = (v: number) => Math.round(v * 10000) / 10000;
  return [r(b.w ? (x - b.x) / b.w : 0.5), r(b.h ? (y - b.y) / b.h : 0.5)];
}

/** Fewer points for a long stroke (every k-th, the last kept). */
function thin(pts: Pt[]): Pt[] {
  if (pts.length <= MAX_POINTS) return pts;
  const k = Math.ceil(pts.length / MAX_POINTS);
  return pts.filter((_, i) => i % k === 0 || i === pts.length - 1);
}

const SHAPES: { shape: DrawingShape; icon: IconName; label: string }[] = [
  { shape: 'pen', icon: 'gesture', label: 'Caneta' },
  { shape: 'arrow', icon: 'north_east', label: 'Seta' },
  { shape: 'rect', icon: 'rectangle', label: 'Retângulo' },
  { shape: 'ellipse', icon: 'circle', label: 'Elipse' },
  { shape: 'text', icon: 'title', label: 'Texto' },
];

/** Desenho's bar: the shape, the color, and leaving (Esc). */
function DrawBar() {
  const shape = useSquad((s) => s.drawShape);
  const color = useSquad((s) => s.drawColor);
  const set = useSquad((s) => s.setDraw);
  return (
    <div className="draw-bar" role="toolbar" aria-label="Desenho">
      <span className="draw-bar-tag">Desenho</span>
      {SHAPES.map((s) => (
        <button key={s.shape} className={`tbx-btn${s.shape === shape ? ' active' : ''}`} onClick={() => set({ shape: s.shape })} aria-label={s.label} title={s.label}>
          <Icon name={s.icon} size={18} />
        </button>
      ))}
      <span className="draw-bar-div" />
      {VISUAL_COLORS.map((c) => (
        <button key={c} className={`draw-swatch${c === color ? ' on' : ''}`} style={{ background: colorVar(c) }} onClick={() => set({ color: c })} aria-label={`Cor ${c}`} />
      ))}
      <span className="draw-bar-div" />
      <button className="tbx-btn" onClick={() => useSquad.getState().setTool('select')} aria-label="Sair do desenho" title="Sair do desenho (Esc)">
        <Icon name="close" size={18} />
      </button>
    </div>
  );
}

function FrameHint() {
  return (
    <div className="draw-bar frame-hint" role="status">
      <span className="draw-bar-tag">Grupo</span>
      <span className="draw-bar-hint">Arraste para fazer a moldura. Ou selecione nós e aperte G.</span>
      <button className="tbx-btn" onClick={() => useSquad.getState().setTool('select')} aria-label="Cancelar" title="Cancelar (Esc)">
        <Icon name="close" size={18} />
      </button>
    </div>
  );
}
