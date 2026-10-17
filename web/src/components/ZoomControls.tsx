import { MiniMap, useReactFlow, useStore, type MiniMapNodeProps } from '@xyflow/react';
import { useSquad } from '../state/store';
import { Icon } from './Icon';
import './ZoomControls.css';

function MiniNode({ id, x, y, width, height }: MiniMapNodeProps) {
  const node = useSquad((s) => s.workspace?.nodes.find((n) => n.id === id));
  const state = useSquad((s) => s.runtime[id]?.activity ?? 'off');
  const isTerm = node?.kind === 'terminal';
  return (
    <rect
      x={x}
      y={y}
      width={width}
      height={height}
      style={{
        fill: isTerm ? `var(--stb-${state})` : node?.kind === 'note' ? 'var(--paper-off)' : 'var(--surface-3)',
        stroke: isTerm ? `var(--st-${state})` : 'none',
        strokeWidth: 14,
        opacity: isTerm ? 1 : 0.7,
      }}
    />
  );
}

export function MiniMapAndZoom() {
  const rf = useReactFlow();
  const zoom = useStore((s) => s.transform[2]);
  const drawerOpen = useSquad((s) => s.drawerOpen);
  const right = drawerOpen ? 458 : 18; // beside the Rádio (--w-drawer 440)
  return (
    <>
      <div className="torre-scope-sweep" style={{ right, bottom: 56 }} aria-hidden="true" />
      <MiniMap
        className="torre-minimap"
        style={{ right: right - 15, bottom: 56 - 15, width: 230, height: 118 }}
        nodeComponent={MiniNode}
        maskColor="transparent"
        maskStrokeColor="var(--focus)"
        maskStrokeWidth={1.5}
        pannable
        zoomable
        ariaLabel="Minimapa"
      />
      <div className="zoom-ctl" style={{ right }}>
        <button onClick={() => rf.zoomOut({ duration: 150 })} aria-label="Diminuir zoom">
          <Icon name="remove" size={18} />
        </button>
        <span className="mono zoom-pct">{Math.round(zoom * 100)}%</span>
        <button onClick={() => rf.zoomIn({ duration: 150 })} aria-label="Aumentar zoom">
          <Icon name="add" size={18} />
        </button>
        <span className="zoom-div" />
        <button onClick={() => rf.fitView({ duration: 200, padding: 0.15 })} aria-label="Enquadrar">
          <Icon name="fit_screen" size={18} />
        </button>
      </div>
    </>
  );
}
