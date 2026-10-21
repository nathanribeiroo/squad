import { useMemo, useRef, useState } from 'react';
import { Handle, NodeResizer, Position, type Node, type NodeProps, type ResizeParams } from '@xyflow/react';
import { api } from '../../api/client';
import type { TerminalRuntime, UIState, WNode } from '../../api/types';
import { Icon } from '../../components/Icon';
import { plural } from '../../lib/floors';
import { STATE_META } from '../../lib/model';
import { toast } from '../../lib/toast';
import { VISUAL_COLORS, collapse, colorVar, expand, insideGroup } from '../../lib/visual';
import { useSquad } from '../../state/store';
import { NodeMenu } from './NodeMenu';

export type GroupFlowNode = Node<{ node: WNode }, 'frame'>;

const NO_NODES: WNode[] = [];

/** States the summary names, in this order ("1 pede você · 2 no ar"). */
const SUMMARY: UIState[] = ['ask', 'err', 'wait', 'work', 'done'];

/**
 * A group (Grupo, G): a named frame behind the nodes inside it, which move
 * with it; collapsed, a card that says how its flights are doing. For the
 * user's eyes only: agents don't see it (internal/store/visual.go).
 */
export function GroupNode({ id, data, selected }: NodeProps<GroupFlowNode>) {
  const g = data.node;
  const gr = g.group!;
  const nodes = useSquad((s) => s.workspace?.nodes) ?? NO_NODES;
  const runtime = useSquad((s) => s.runtime);
  const zoom = useSquad((s) => s.renderZoom);
  const [renaming, setRenaming] = useState(false);
  const inside = useMemo(() => insideGroup(nodes, g), [nodes, g]);
  const sum = useMemo(() => summary(inside, runtime), [inside, runtime]);
  const style = { '--gc': colorVar(gr.color), '--gs': Math.min(3, Math.max(1, 0.8 / zoom)) } as React.CSSProperties;

  const patch = (next: Partial<NonNullable<WNode['group']>>) => api.updateNode(id, { group: { ...gr, ...next } }).catch((e: Error) => toast(e.message, 'err'));
  const onResizeEnd = (_: unknown, p: ResizeParams) => {
    useSquad.getState().moveNodeLocal(id, Math.round(p.x), Math.round(p.y));
    api.updateNode(id, { x: Math.round(p.x), y: Math.round(p.y), w: Math.round(p.width), h: Math.round(p.height) }).catch((e: Error) => toast(e.message, 'err'));
  };
  const menu = (
    <NodeMenu
      items={[
        { icon: 'edit', label: 'Renomear', action: () => setRenaming(true) },
        gr.collapsed
          ? { icon: 'unfold_more', label: 'Expandir', action: () => void expand(g) }
          : { icon: 'unfold_less', label: 'Recolher', action: () => collapse(g), hint: plural(inside.length, 'nó', 'nós') },
        { section: 'Cor' },
        { colors: VISUAL_COLORS, value: gr.color, css: colorVar, pick: (color) => void patch({ color }) },
        'divider',
        { icon: 'delete', label: 'Remover moldura', action: () => api.removeNode(id).catch((e: Error) => toast(e.message, 'err')), danger: true, hint: 'os nós ficam' },
      ]}
    />
  );
  const name = renaming ? (
    <GroupNameInput name={gr.name} onDone={(n) => (setRenaming(false), n !== null && n !== gr.name && void patch({ name: n }))} />
  ) : (
    <span className="gn-name" title="Duplo clique para renomear" onDoubleClick={() => setRenaming(true)}>
      {gr.name}
    </span>
  );

  if (gr.collapsed) {
    return (
      <div className={`gcard node-drag${selected ? ' selected' : ''}${sum.ask ? ' ask' : ''}`} style={style} onDoubleClick={() => !renaming && void expand(g)}>
        {/* The wires of the nodes it hides end here. */}
        <Handle type="source" id="body" position={Position.Top} isConnectable={false} className="gc-handle" />
        <header className="gc-head">
          <Icon name="select" size={16} />
          {name}
          <span className="tn-spacer" />
          <button className="icon-btn nodrag" onClick={() => void expand(g)} title="Expandir: os nós voltam para onde estavam" aria-label="Expandir">
            <Icon name="unfold_more" size={17} />
          </button>
          {menu}
        </header>
        <div className="gc-body">
          {sum.flights.length > 0 && (
            <span className="gc-dots" aria-hidden>
              {sum.flights.slice(0, 12).map((f) => (
                <i key={f.id} className={`gc-dot st-${f.state}`} />
              ))}
            </span>
          )}
          <span className="gc-sum">{sum.text}</span>
        </div>
      </div>
    );
  }

  return (
    <div className={`gnode${selected ? ' selected' : ''}`} style={style}>
      <NodeResizer isVisible={selected} minWidth={160} minHeight={100} onResizeEnd={onResizeEnd} />
      <header className="gn-head node-drag">
        <span className="gn-tag">Grupo</span>
        {name}
        {inside.length > 0 && <span className="gn-count mono">{sum.text}</span>}
        <button className="icon-btn nodrag" onClick={() => collapse(g)} title="Recolher num cartão: os voos continuam no ar" aria-label="Recolher">
          <Icon name="unfold_less" size={16} />
        </button>
        {menu}
      </header>
    </div>
  );
}

/** "4 voos · 1 pede você · 2 no ar · 1 NOTAM"; ask: someone inside asks you. */
function summary(inside: WNode[], runtime: Record<string, TerminalRuntime>) {
  const flights = inside.filter((n) => n.terminal).map((n) => ({ id: n.id, state: (runtime[n.id]?.activity ?? 'off') as UIState }));
  const parts: string[] = [];
  if (flights.length) parts.push(plural(flights.length, 'voo', 'voos'));
  for (const st of SUMMARY) {
    const k = flights.filter((f) => f.state === st).length;
    if (k) parts.push(`${k} ${STATE_META[st].label.toLowerCase()}`);
  }
  const notes = inside.filter((n) => n.note).length;
  const portals = inside.filter((n) => n.portal).length;
  const cargo = inside.filter((n) => n.cargo).length;
  if (notes) parts.push(plural(notes, 'NOTAM', 'NOTAMs'));
  if (portals) parts.push(plural(portals, 'portal', 'portais'));
  if (cargo) parts.push(plural(cargo, 'carga', 'cargas'));
  if (!parts.length) parts.push(inside.length ? plural(inside.length, 'desenho', 'desenhos') : 'vazio');
  return { flights, text: parts.join(' · '), ask: flights.some((f) => f.state === 'ask') };
}

/** The group's name, editable: Enter or a click elsewhere keeps it, Esc gives up. */
function GroupNameInput({ name, onDone }: { name: string; onDone: (name: string | null) => void }) {
  const done = useRef(false);
  const finish = (v: string | null) => {
    if (done.current) return;
    done.current = true;
    onDone(v === null ? null : v.trim() || name);
  };
  return (
    <input
      className="gn-input nodrag"
      defaultValue={name}
      autoFocus
      maxLength={60}
      spellCheck={false}
      aria-label="Nome do grupo"
      onFocus={(e) => e.target.select()}
      onBlur={(e) => finish(e.target.value)}
      onKeyDown={(e) => {
        e.stopPropagation(); // canvas shortcuts stay out of the name
        if (e.key === 'Enter') finish(e.currentTarget.value);
        if (e.key === 'Escape') finish(null);
      }}
    />
  );
}
