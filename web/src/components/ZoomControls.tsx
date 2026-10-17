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
      rx={6}
      style={{
        fill: isTerm ? `var(--stb-${state})` : 'var(--surface-3)',
        stroke: isTerm ? `var(--st-${state})` : 'none',
        strokeWidth: 12,
      }}
    />
  );
}

export function MiniMapAndZoom() {
  const rf = useReactFlow();
  const zoom = useStore((s) => s.transform[2]);
  const drawerOpen = useSquad((s) => s.drawerOpen);
  const right = drawerOpen ? 398 : 18;
  return (
    <>
      <MiniMap
        className="torre-minimap"
        style={{ right: right - 15, bottom: 56 - 15, width: 220, height: 132 }}
        nodeComponent={MiniNode}
        maskColor="transparent"
        maskStrokeColor="var(--text)"
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
