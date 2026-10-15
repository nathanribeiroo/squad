import { useMemo, useState } from 'react';
import { BaseEdge, EdgeLabelRenderer, useInternalNode, useReactFlow, type Edge, type EdgeProps, type InternalNode } from '@xyflow/react';
import { api } from '../../api/client';
import { Icon } from '../../components/Icon';
import { elapsed } from '../../lib/model';
import { toast } from '../../lib/toast';
import { useNow } from '../../lib/useNow';
import { useNodeName, useSquad, type Flight } from '../../state/store';
import { wire, type Rect, type Wire } from '../geometry';
import './wire.css';

export type WireData = { rope?: [number, number][] };
export type WireFlowEdge = Edge<WireData, 'wire'>;

/** Envelope travel time (handoff §6). */
const TRAVEL_MS = 600;
/** How long the "✓ id · 2:14" marker stays (the store drops the flight at 4.5 s). */
const MARKER_MS = 4000;

function rectOf(n: InternalNode): Rect {
  const p = n.internals.positionAbsolute;
  return { x: p.x, y: p.y, w: n.measured.width ?? n.width ?? 0, h: n.measured.height ?? n.height ?? 0 };
}

type Visual =
  | { kind: 'out'; f: Flight } // envelope travelling origin → destination
  | { kind: 'pending'; f: Flight }
  | { kind: 'back'; f: Flight } // reply travelling destination → origin
  | { kind: 'done'; f: Flight }
  | { kind: 'cancelled'; f: Flight };

function visualOf(f: Flight, now: number): Visual | null {
  if (f.replyAt !== undefined && now - f.replyAt < TRAVEL_MS) return { kind: 'back', f };
  switch (f.phase) {
    case 'sending':
      return now - f.startedAt < TRAVEL_MS ? { kind: 'out', f } : { kind: 'pending', f };
    case 'pending':
    case 'replying':
      return { kind: 'pending', f };
    case 'received':
      return now - (f.replyAt ?? f.updatedAt) < TRAVEL_MS + MARKER_MS ? { kind: 'done', f } : null;
    case 'cancelled':
      return now - f.updatedAt < MARKER_MS ? { kind: 'cancelled', f } : null;
  }
}

export function WireEdge({ id, source, target, selected, data }: EdgeProps<WireFlowEdge>) {
  const a = useInternalNode(source);
  const b = useInternalNode(target);
  const aName = useNodeName(source);
  const bName = useNodeName(target);
  const allFlights = useSquad((s) => s.flights);
  const paused = useSquad((s) => s.extra.paused);
  const flights = useMemo(
    () => Object.values(allFlights).filter((f) => (f.from === source && f.to === target) || (f.from === target && f.to === source)),
    [allFlights, source, target],
  );
  const now = useNow(200, flights.length > 0);
  const [dragRope, setDragRope] = useState<[number, number][] | null>(null);
  if (!a || !b) return null;

  const rope = dragRope ?? data?.rope ?? [];
  const ra = rectOf(a);
  const rb = rectOf(b);
  const w = wire(ra, rb, rope);
  // The same curve oriented from a flight's origin, for the travelling light.
  const oriented = (from: string): Wire => (from === source ? w : wire(rb, ra, [...rope].reverse()));

  const visuals = flights.map((f) => visualOf(f, now)).filter((v): v is Visual => v !== null);
  const pending = visuals.find((v) => v.kind === 'pending' || v.kind === 'out');
  const originIsSource = pending ? pending.f.from === source : true;

  const unlink = () => {
    api
      .removeEdge(id)
      .then(() => toast(`${aName} ↔ ${bName} desligados`))
      .catch((e: Error) => toast(e.message));
  };

  const wireClass = `wire${selected ? ' selected' : ''}${pending?.kind === 'pending' ? ' pending' : ''}`;
  const endClass = (isOrigin: boolean) => (pending?.kind === 'pending' ? (isOrigin ? ' origin' : ' dest') : '');

  return (
    <>
      <BaseEdge id={id} path={w.path} className={wireClass} interactionWidth={24} />
      {visuals.map((v) =>
        v.kind === 'out' || v.kind === 'back' ? (
          <path
            key={`${v.f.id}-${v.kind}`}
            d={oriented(v.kind === 'out' ? v.f.from : v.f.to).path}
            pathLength={100}
            className={`wire-light ${v.kind === 'out' ? 'wait' : 'done'}`}
          />
        ) : null,
      )}
      <circle className={`wire-end${endClass(originIsSource)}`} cx={w.s.x} cy={w.s.y} r={6} />
      <circle className={`wire-end${endClass(!originIsSource)}`} cx={w.t.x} cy={w.t.y} r={6} />
      {selected && <RopePoints edgeId={id} rope={rope} onDrag={setDragRope} />}
      <EdgeLabelRenderer>
        {visuals.map((v, i) => {
          const at = { transform: `translate(-50%, -50%) translate(${w.low.x}px, ${w.low.y + i * 36}px)` };
          switch (v.kind) {
            case 'out':
            case 'back':
              return (
                <div
                  key={`${v.f.id}-${v.kind}-env`}
                  className={`wire-envelope ${v.kind === 'out' ? 'wait' : 'done'}`}
                  style={{ offsetPath: `path("${oriented(v.kind === 'out' ? v.f.from : v.f.to).path}")` }}
                >
                  {v.kind === 'out' ? (
                    <>
                      <Icon name="mail" size={15} />
                      {v.f.id}
                      <Icon name="arrow_forward" size={15} />
                    </>
                  ) : (
                    <>
                      <Icon name="arrow_back" size={15} />
                      resposta {v.f.id}
                      <Icon name="reply" size={15} />
                    </>
                  )}
                </div>
              );
            case 'pending':
              return (
                <div key={`${v.f.id}-pill`} className="wire-pill wait" style={at} title={`Pedido ${v.f.id} aguardando resposta`}>
                  <Icon name="hourglass_top" size={17} />
                  <span className="mono wire-time">{elapsed(v.f.startedAt, now)}</span>
                  <span className="mono wire-id">{v.f.id}</span>
                </div>
              );
            case 'done':
              return (
                <div key={`${v.f.id}-done`} className="wire-pill done fade" style={at}>
                  <Icon name="check" size={17} />
                  <span className="mono wire-id">{v.f.id}</span>
                  {v.f.elapsedMs !== undefined && <span className="mono wire-time">· {elapsed(Date.now() - v.f.elapsedMs, Date.now())}</span>}
                </div>
              );
            case 'cancelled':
              return (
                <div key={`${v.f.id}-x`} className="wire-pill err fade" style={at}>
                  <Icon name="block" size={17} />
                  <span className="mono wire-id">{v.f.id}</span>
                  cancelado
                </div>
              );
          }
        })}
        {paused && visuals.length === 0 && !selected && (
          <div className="wire-paused" style={{ transform: `translate(-50%, -50%) translate(${w.low.x}px, ${w.low.y}px)` }} title="Conversas pausadas">
            <Icon name="pause" size={16} />
          </div>
        )}
        {selected && visuals.length === 0 && (
          <button
            className="wire-unlink nodrag nopan"
            style={{ transform: `translate(-50%, -50%) translate(${w.low.x}px, ${w.low.y + (rope.some(([x, y]) => Math.hypot(x - w.low.x, y - w.low.y) < 48) ? 40 : 0)}px)` }}
            onClick={unlink}
            title="Desligar (Delete)"
          >
            <Icon name="link_off" size={16} />
            Desligar
          </button>
        )}
      </EdgeLabelRenderer>
    </>
  );
}

/**
 * Bend points of the selected wire: drag to move, double-click to remove
 * (double-clicking the wire adds one, see Canvas).
 */
function RopePoints({ edgeId, rope, onDrag }: { edgeId: string; rope: [number, number][]; onDrag: (r: [number, number][] | null) => void }) {
  const rf = useReactFlow();
  const save = (r: [number, number][]) => api.updateEdge(edgeId, r).catch((e: Error) => toast(e.message));
  const startDrag = (e: React.PointerEvent, i: number) => {
    e.stopPropagation();
    const live = rope.map((p) => [...p] as [number, number]);
    let moved = false;
    const move = (ev: PointerEvent) => {
      const p = rf.screenToFlowPosition({ x: ev.clientX, y: ev.clientY });
      live[i] = [Math.round(p.x), Math.round(p.y)];
      moved = true;
      onDrag([...live]);
    };
    const up = () => {
      window.removeEventListener('pointermove', move);
      window.removeEventListener('pointerup', up);
      if (moved) void save(live).finally(() => onDrag(null));
    };
    window.addEventListener('pointermove', move);
    window.addEventListener('pointerup', up);
  };
  return (
    <>
      {rope.map(([x, y], i) => (
        <circle
          key={i}
          className="wire-rope nodrag nopan"
          cx={x}
          cy={y}
          r={5}
          onPointerDown={(e) => startDrag(e, i)}
          onDoubleClick={(e) => {
            e.stopPropagation();
            void save(rope.filter((_, j) => j !== i));
          }}
        />
      ))}
    </>
  );
}
