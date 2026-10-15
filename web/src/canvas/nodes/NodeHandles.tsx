import { Handle, Position, useConnection } from '@xyflow/react';
import { useSquad } from '../../state/store';

/**
 * Connection handles shared by every node. Edges are undirected and drawn
 * between facing sides (see geometry.ts), so handles only start and receive
 * drags: four side dots start one, and while a drag is in progress the whole
 * body of another node becomes a drop target.
 */
export function NodeHandles({ id }: { id: string }) {
  const target = useConnectTarget(id);
  return (
    <>
      <Handle
        type="source"
        id="body"
        position={Position.Top}
        className="body-handle"
        isConnectableStart={false}
        style={{ pointerEvents: target === 'valid' ? 'all' : 'none' }}
      />
      <Handle type="source" id="l" position={Position.Left} className="side-handle" />
      <Handle type="source" id="r" position={Position.Right} className="side-handle" />
      <Handle type="source" id="t" position={Position.Top} className="side-handle" />
      <Handle type="source" id="b" position={Position.Bottom} className="side-handle" />
    </>
  );
}

/** While a connection is being dragged: is this node a valid target? */
export function useConnectTarget(id: string): 'none' | 'valid' | 'invalid' | 'origin' {
  const conn = useConnection();
  const linked = useSquad((s) => {
    const from = conn.inProgress ? conn.fromNode.id : null;
    return !!from && !!s.workspace?.edges.some((e) => (e.a === from && e.b === id) || (e.a === id && e.b === from));
  });
  if (!conn.inProgress) return 'none';
  if (conn.fromNode.id === id) return 'origin';
  return linked ? 'invalid' : 'valid';
}

export function connectClass(target: ReturnType<typeof useConnectTarget>): string {
  return target === 'valid' ? ' connect-valid' : target === 'invalid' ? ' connect-invalid' : '';
}
