import { BaseEdge, EdgeLabelRenderer, useInternalNode, type Edge, type EdgeProps, type InternalNode } from '@xyflow/react';
import { api } from '../../api/client';
import { Icon } from '../../components/Icon';
import { toast } from '../../lib/toast';
import { useNodeName } from '../../state/store';
import { wire, type Rect } from '../geometry';
import './wire.css';

export type WireData = { rope?: [number, number][] };
export type WireFlowEdge = Edge<WireData, 'wire'>;

function rectOf(n: InternalNode): Rect {
  const p = n.internals.positionAbsolute;
  return { x: p.x, y: p.y, w: n.measured.width ?? n.width ?? 0, h: n.measured.height ?? n.height ?? 0 };
}

export function WireEdge({ id, source, target, selected, data }: EdgeProps<WireFlowEdge>) {
  const a = useInternalNode(source);
  const b = useInternalNode(target);
  const aName = useNodeName(source);
  const bName = useNodeName(target);
  if (!a || !b) return null;
  const w = wire(rectOf(a), rectOf(b), data?.rope);

  const unlink = () => {
    api
      .removeEdge(id)
      .then(() => toast(`${aName} ↔ ${bName} desligados`))
      .catch((e: Error) => toast(e.message));
  };

  return (
    <>
      <BaseEdge id={id} path={w.path} className={`wire${selected ? ' selected' : ''}`} interactionWidth={24} />
      <circle className="wire-end" cx={w.s.x} cy={w.s.y} r={6} />
      <circle className="wire-end" cx={w.t.x} cy={w.t.y} r={6} />
      {selected && data?.rope?.map(([x, y], i) => <circle key={i} className="wire-rope" cx={x} cy={y} r={4} />)}
      {selected && (
        <EdgeLabelRenderer>
          <button
            className="wire-unlink nodrag nopan"
            style={{ transform: `translate(-50%, -50%) translate(${w.low.x}px, ${w.low.y}px)` }}
            onClick={unlink}
            title="Desligar (Delete)"
          >
            <Icon name="link_off" size={16} />
            Desligar
          </button>
        </EdgeLabelRenderer>
      )}
    </>
  );
}
