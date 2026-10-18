import { useRef } from 'react';
import { NodeResizer, type Node, type NodeProps, type ResizeParams } from '@xyflow/react';
import { api } from '../../api/client';
import type { DrawingData, WNode } from '../../api/types';
import { toast } from '../../lib/toast';
import { colorVar } from '../../lib/visual';
import { useSquad } from '../../state/store';

export type DrawingFlowNode = Node<{ node: WNode }, 'drawing'>;

/** Stroke width of drawings, on the canvas. */
export const STROKE = 3;
/** Font size of a text drawing, and its line height. */
export const TEXT_SIZE = 20;
const TEXT_LINE = 26;

/**
 * A drawing (Desenho, D): a stroke, an arrow, a rectangle, an ellipse or a
 * text, for the user's eyes only. It takes clicks on its own lines (and a
 * text on its words), so the nodes behind it stay usable.
 */
export function DrawingNode({ id, data, selected, width, height }: NodeProps<DrawingFlowNode>) {
  const n = data.node;
  const d = n.drawing!;
  const editing = useSquad((s) => s.editText === id);
  const w = width ?? n.w;
  const h = height ?? n.h;
  const onResizeEnd = (_: unknown, p: ResizeParams) => {
    useSquad.getState().moveNodeLocal(id, Math.round(p.x), Math.round(p.y));
    api.updateNode(id, { x: Math.round(p.x), y: Math.round(p.y), w: Math.round(p.width), h: Math.round(p.height) }).catch((e: Error) => toast(e.message, 'err'));
  };
  return (
    <div className={`dnode${selected ? ' selected' : ''}`} style={{ '--dc': colorVar(d.color) } as React.CSSProperties}>
      <NodeResizer isVisible={selected && !editing} minWidth={d.shape === 'text' ? 60 : 8} minHeight={d.shape === 'text' ? TEXT_LINE : 8} onResizeEnd={onResizeEnd} />
      {d.shape === 'text' ? (
        editing ? (
          <TextEditor id={id} node={n} />
        ) : (
          <div className="d-text" onDoubleClick={() => useSquad.getState().setEditText(id)} title="Duplo clique para editar">
            {d.text}
          </div>
        )
      ) : (
        <svg className="d-svg" width={w} height={h} viewBox={`0 0 ${w} ${h}`}>
          <Shape d={d} w={w} h={h} />
        </svg>
      )}
    </div>
  );
}

/** The drawing's lines, w × h, twice: a wide invisible one takes the clicks. */
export function Shape({ d, w, h, scale = 1 }: { d: DrawingData; w: number; h: number; scale?: number }) {
  const body = shapeOf(d, w, h, scale);
  if (!body) return null;
  return (
    <>
      <g className="d-hit">{body}</g>
      <g className="d-line" style={{ strokeWidth: STROKE * scale }}>
        {body}
        {d.shape === 'arrow' && arrowHead(d, w, h, scale)}
      </g>
    </>
  );
}

function shapeOf(d: DrawingData, w: number, h: number, scale: number) {
  const inset = (STROKE * scale) / 2;
  const pts = (d.points ?? []).map(([x, y]) => [x * w, y * h] as const);
  switch (d.shape) {
    case 'pen':
      return pts.length === 1 ? <circle cx={pts[0][0]} cy={pts[0][1]} r={STROKE * scale} /> : <path d={pts.map(([x, y], i) => `${i ? 'L' : 'M'}${x.toFixed(1)} ${y.toFixed(1)}`).join('')} />;
    case 'arrow':
      return pts.length === 2 ? <line x1={pts[0][0]} y1={pts[0][1]} x2={pts[1][0]} y2={pts[1][1]} /> : null;
    case 'rect':
      return <rect x={inset} y={inset} width={Math.max(0, w - 2 * inset)} height={Math.max(0, h - 2 * inset)} />;
    case 'ellipse':
      return <ellipse cx={w / 2} cy={h / 2} rx={Math.max(0, w / 2 - inset)} ry={Math.max(0, h / 2 - inset)} />;
  }
  return null;
}

/** The arrow's head at its second point. */
function arrowHead(d: DrawingData, w: number, h: number, scale: number) {
  const [[x0, y0], [x1, y1]] = (d.points ?? []).map(([x, y]) => [x * w, y * h]);
  if (x1 === undefined) return null;
  const a = Math.atan2(y1 - y0, x1 - x0);
  const len = 16 * scale;
  const wing = (s: number) => `${(x1 - len * Math.cos(a + s)).toFixed(1)},${(y1 - len * Math.sin(a + s)).toFixed(1)}`;
  return <polygon className="d-head" points={`${x1},${y1} ${wing(0.45)} ${wing(-0.45)}`} />;
}

/** A text being typed: Esc or a click elsewhere keeps it; left empty, it goes. */
function TextEditor({ id, node }: { id: string; node: WNode }) {
  const done = useRef(false);
  const d = node.drawing!;
  const finish = (el: HTMLTextAreaElement) => {
    if (done.current) return;
    done.current = true;
    const st = useSquad.getState();
    if (st.editText === id) st.setEditText(null);
    const text = el.value.replace(/\s+$/, '');
    if (!text.trim()) {
      api.removeNode(id).catch((e: Error) => toast(e.message, 'err'));
      return;
    }
    const h = Math.max(TEXT_LINE + 8, Math.ceil(el.scrollHeight));
    if (text !== d.text || h !== node.h) api.updateNode(id, { h, drawing: { ...d, text } }).catch((e: Error) => toast(e.message, 'err'));
  };
  return (
    <textarea
      className="d-text d-edit nodrag nowheel"
      defaultValue={d.text ?? ''}
      autoFocus
      spellCheck={false}
      maxLength={2000}
      placeholder="Texto…"
      onFocus={(e) => e.currentTarget.setSelectionRange(e.currentTarget.value.length, e.currentTarget.value.length)}
      onBlur={(e) => finish(e.currentTarget)}
      onInput={(e) => {
        // Grows with the words while typing.
        const el = e.currentTarget;
        el.style.height = 'auto';
        el.style.height = `${el.scrollHeight}px`;
      }}
      onKeyDown={(e) => {
        e.stopPropagation(); // canvas shortcuts stay out of the text
        if (e.key === 'Escape') e.currentTarget.blur();
      }}
    />
  );
}
