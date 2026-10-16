import { useMemo } from 'react';
import type { WNode } from '../../api/types';
import { floorName } from '../../lib/floors';
import { useSquad } from '../../state/store';

function label(n: WNode): string {
  return n.terminal?.name ?? n.note?.title ?? (n.portal ? n.portal.name || 'Portal' : n.id);
}

/**
 * Wires to nodes on other floors can't be drawn on this floor's canvas: they
 * show as tags under the node ("⇄ Plano · térreo"). A tag pulses while a
 * request travels on its wire; clicking it shows the other floor, centered on
 * that node.
 */
export function CrossFloorLinks({ id }: { id: string }) {
  const ws = useSquad((s) => s.workspace);
  const flights = useSquad((s) => s.flights);
  const zoom = useSquad((s) => s.renderZoom);
  const links = useMemo(() => {
    const me = ws?.nodes.find((n) => n.id === id);
    if (!ws || !me) return [];
    const floor = me.floor ?? '';
    const out: { id: string; name: string; floor: string; floorName: string }[] = [];
    for (const e of ws.edges) {
      if (e.a !== id && e.b !== id) continue;
      const other = ws.nodes.find((n) => n.id === (e.a === id ? e.b : e.a));
      if (!other || (other.floor ?? '') === floor) continue;
      out.push({ id: other.id, name: label(other), floor: other.floor ?? '', floorName: floorName(ws.floors, other.floor) });
    }
    return out;
  }, [ws, id]);
  if (links.length === 0) return null;

  const busy = (other: string) =>
    Object.values(flights).some(
      (f) => ((f.from === id && f.to === other) || (f.from === other && f.to === id)) && f.phase !== 'received' && f.phase !== 'cancelled',
    );
  // Zoomed out, the tags keep a readable size.
  const scale = zoom < 0.6 ? 1 / Math.max(zoom, 0.15) : 1;
  return (
    <div className="xfloor nodrag nopan" style={scale !== 1 ? { transform: `scale(${scale})` } : undefined}>
      {links.map((l) => (
        <button
          key={l.id}
          className={`xfloor-tag${busy(l.id) ? ' busy' : ''}`}
          title={`Ligado a ${l.name}, ${l.floor ? `no andar ${l.floorName}` : 'no térreo'}. Clique para ir até lá.`}
          onClick={(e) => {
            e.stopPropagation();
            useSquad.getState().setFloor(l.floor, l.id);
          }}
        >
          <span className="xfloor-icon">⇄</span>
          <span className="xfloor-name">{l.name}</span>
          <span className="xfloor-floor">· {l.floorName}</span>
        </button>
      ))}
    </div>
  );
}
