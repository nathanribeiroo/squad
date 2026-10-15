import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import {
  Background,
  BackgroundVariant,
  ConnectionMode,
  ReactFlow,
  useReactFlow,
  applyEdgeChanges,
  applyNodeChanges,
  type Connection,
  type Edge,
  type EdgeChange,
  type Node,
  type NodeChange,
  type OnBeforeDelete,
  type Viewport,
} from '@xyflow/react';
import { api } from '../api/client';
import type { WNode } from '../api/types';
import { MiniMapAndZoom } from '../components/ZoomControls';
import { toast } from '../lib/toast';
import { useSquad } from '../state/store';
import { pruneSessions } from '../term/session';
import { WireEdge } from './edges/WireEdge';
import { insertRopePoint } from './geometry';
import { NoteNode } from './nodes/NoteNode';
import { PortalNode } from './nodes/PortalNode';
import { TerminalNode } from './nodes/TerminalNode';
import './canvas.css';
import './nodes/nodes.css';

const nodeTypes = { terminal: TerminalNode, note: NoteNode, portal: PortalNode };
const edgeTypes = { wire: WireEdge };

// Each workspace keeps its own pan and zoom.
const viewportKey = (ws: string | null) => `squad.viewport.${ws ?? ''}`;

function savedViewport(ws: string | null): Viewport {
  try {
    const v = JSON.parse(localStorage.getItem(viewportKey(ws)) ?? localStorage.getItem('squad.viewport') ?? '') as Viewport;
    if (Number.isFinite(v.x) && Number.isFinite(v.y) && Number.isFinite(v.zoom)) return v;
  } catch {
    /* default below */
  }
  return { x: 60, y: 40, zoom: 0.8 };
}

function toFlow(n: WNode, prev?: Node): Node {
  return {
    ...prev,
    id: n.id,
    type: n.kind,
    // Keep the local position while the user is dragging.
    position: prev?.dragging ? prev.position : { x: n.x, y: n.y },
    width: n.w,
    height: n.h,
    dragHandle: '.node-drag',
    data: { node: n },
  };
}

export function Canvas() {
  const ws = useSquad((s) => s.workspace);
  const tool = useSquad((s) => s.tool);
  const theme = useSquad((s) => s.theme);
  const setSelected = useSquad((s) => s.setSelectedNode);
  const setFocused = useSquad((s) => s.setFocusedTerminal);
  const [nodes, setNodes] = useState<Node[]>([]);
  const [edges, setEdges] = useState<Edge[]>([]);
  const activeWs = useSquad((s) => s.activeWs);
  const defaultViewport = useMemo(() => savedViewport(activeWs), [activeWs]);

  // The server is the source of truth; local state only adds React Flow's
  // own fields (selection, measured size, drag).
  useEffect(() => {
    setNodes((prev) => {
      const byId = new Map(prev.map((n) => [n.id, n]));
      return (ws?.nodes ?? []).map((n) => toFlow(n, byId.get(n.id)));
    });
    if (ws) pruneSessions(new Set(ws.nodes.filter((n) => n.kind === 'terminal').map((n) => n.id)));
  }, [ws?.nodes]);
  useEffect(() => useSquad.getState().setZoom(defaultViewport.zoom, true), [defaultViewport]);
  useEffect(() => {
    setEdges((prev) => {
      const byId = new Map(prev.map((e) => [e.id, e]));
      return (ws?.edges ?? []).map((e) => ({ ...byId.get(e.id), id: e.id, source: e.a, target: e.b, type: 'wire', data: { rope: e.ropePoints } }));
    });
  }, [ws?.edges]);

  const onNodesChange = useCallback((c: NodeChange[]) => setNodes((n) => applyNodeChanges(c, n)), []);
  const onEdgesChange = useCallback((c: EdgeChange[]) => setEdges((e) => applyEdgeChanges(c, e)), []);

  const name = (id: string) => {
    const n = useSquad.getState().workspace?.nodes.find((x) => x.id === id);
    return n?.terminal?.name ?? n?.note?.title ?? 'Portal';
  };

  const linked = (a: string, b: string) => edges.some((e) => (e.source === a && e.target === b) || (e.source === b && e.target === a));

  const onConnect = useCallback(
    (c: Connection) => {
      if (!c.source || !c.target || c.source === c.target) return;
      api
        .addEdge(c.source, c.target)
        .then(() => toast(`${name(c.source)} ↔ ${name(c.target)} ligados`))
        .catch((e: Error) => toast(e.message));
    },
    [],
  );

  // Keyboard deletion only removes wires; nodes are removed from their menu
  // (removing a terminal kills its process).
  const onBeforeDelete: OnBeforeDelete = useCallback(async ({ edges }) => ({ nodes: [], edges }), []);
  const onEdgesDelete = useCallback((removed: Edge[]) => {
    for (const e of removed) {
      api
        .removeEdge(e.id)
        .then(() => toast(`${name(e.source)} ↔ ${name(e.target)} desligados`))
        .catch((err: Error) => toast(err.message));
    }
  }, []);

  const onNodeDragStop = useCallback((_: unknown, _node: Node, moved: Node[]) => {
    const st = useSquad.getState();
    for (const n of moved) {
      const x = Math.round(n.position.x);
      const y = Math.round(n.position.y);
      st.moveNodeLocal(n.id, x, y);
      api.updateNode(n.id, { x, y }).catch((e: Error) => toast(e.message));
    }
  }, []);

  const onMove = useCallback((_: unknown, v: Viewport) => useSquad.getState().setZoom(v.zoom, false), []);
  // Content is re-laid out for the new zoom only when the gesture ends
  // (handoff: freeze during the gesture, redraw on move end).
  const settle = useRef<number | undefined>(undefined);
  const onMoveEnd = useCallback((_: unknown, v: Viewport) => {
    // Programmatic zoom (pinch over a terminal) ends on every tick; wait for
    // the gesture to stop before re-laying out the terminals.
    window.clearTimeout(settle.current);
    settle.current = window.setTimeout(() => useSquad.getState().setZoom(v.zoom, true), 120);
    try {
      localStorage.setItem(viewportKey(useSquad.getState().activeWs), JSON.stringify(v));
    } catch {
      /* storage may be unavailable */
    }
  }, []);

  const rf = useReactFlow();
  // Another workspace on screen: back to where its canvas was left
  // (defaultViewport only applies on the first render).
  const shownWs = useRef(activeWs);
  useEffect(() => {
    if (shownWs.current === activeWs) return;
    shownWs.current = activeWs;
    void rf.setViewport(defaultViewport);
  }, [activeWs, defaultViewport, rf]);
  // Double-clicking a wire adds a bend point there.
  const onEdgeDoubleClick = useCallback(
    (e: React.MouseEvent, edge: Edge) => {
      const a = rf.getInternalNode(edge.source);
      const b = rf.getInternalNode(edge.target);
      const we = useSquad.getState().workspace?.edges.find((x) => x.id === edge.id);
      if (!a || !b || !we) return;
      const rect = (n: typeof a) => ({ x: n.internals.positionAbsolute.x, y: n.internals.positionAbsolute.y, w: n.measured.width ?? 0, h: n.measured.height ?? 0 });
      const p = rf.screenToFlowPosition({ x: e.clientX, y: e.clientY });
      api.updateEdge(edge.id, insertRopePoint(rect(a), rect(b), we.ropePoints ?? [], p)).catch((err: Error) => toast(err.message));
    },
    [rf],
  );

  const pan = tool === 'pan';

  return (
    <ReactFlow
      nodes={nodes}
      edges={edges}
      nodeTypes={nodeTypes}
      edgeTypes={edgeTypes}
      onNodesChange={onNodesChange}
      onEdgesChange={onEdgesChange}
      onConnect={onConnect}
      isValidConnection={(c) => c.source !== c.target && !linked(c.source, c.target)}
      onBeforeDelete={onBeforeDelete}
      onEdgesDelete={onEdgesDelete}
      onEdgeDoubleClick={onEdgeDoubleClick}
      zoomOnDoubleClick={false}
      onNodeDragStop={onNodeDragStop}
      onSelectionChange={({ nodes: sel }) => setSelected(sel.length === 1 ? sel[0].id : null)}
      onPaneClick={() => setFocused(null)}
      onMove={onMove}
      onMoveEnd={onMoveEnd}
      connectionMode={ConnectionMode.Loose}
      connectionRadius={28}
      defaultViewport={defaultViewport}
      minZoom={0.15}
      maxZoom={2}
      colorMode={theme}
      panOnDrag={pan ? true : [1, 2]}
      selectionOnDrag={!pan}
      panOnScroll
      nodesDraggable={!pan}
      elementsSelectable={!pan}
      deleteKeyCode={['Backspace', 'Delete']}
      proOptions={{ hideAttribution: true }}
      className={pan ? 'tool-pan' : undefined}
    >
      <Background variant={BackgroundVariant.Dots} gap={20} size={1} color="var(--grid)" />
      <MiniMapAndZoom />
    </ReactFlow>
  );
}
