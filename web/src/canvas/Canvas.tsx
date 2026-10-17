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

// Each session of a workspace (a canvas of its own), and each of its floors,
// keeps its own pan and zoom; before sessions, the workspace kept it.
const viewportKey = (ws: string | null, session: string | null, floor: string) =>
  `torre.viewport.${ws ?? ''}${session ? `.s.${session}` : ''}${floor ? `.${floor}` : ''}`;

function savedViewport(ws: string | null, session: string | null, floor: string): Viewport {
  try {
    const legacy = floor ? null : localStorage.getItem('torre.viewport');
    const saved = localStorage.getItem(viewportKey(ws, session, floor)) ?? localStorage.getItem(viewportKey(ws, null, floor));
    const v = JSON.parse(saved ?? legacy ?? '') as Viewport;
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
  const session = useSquad((s) => s.activeSession);
  const floor = useSquad((s) => s.activeFloor);
  const defaultViewport = useMemo(() => savedViewport(activeWs, session, floor), [activeWs, session, floor]);

  // The server is the source of truth; local state only adds React Flow's
  // own fields (selection, measured size, drag). The canvas shows the floor
  // on screen; each floor (a git worktree) has its own nodes.
  const onScreen = useCallback((n: WNode) => (n.floor ?? '') === floor, [floor]);
  useEffect(() => {
    setNodes((prev) => {
      const byId = new Map(prev.map((n) => [n.id, n]));
      return (ws?.nodes ?? []).filter(onScreen).map((n) => toFlow(n, byId.get(n.id)));
    });
    if (ws) pruneSessions(new Set(ws.nodes.filter((n) => n.kind === 'terminal').map((n) => n.id)));
  }, [ws?.nodes, onScreen]);
  useEffect(() => useSquad.getState().setZoom(defaultViewport.zoom, true), [defaultViewport]);
  // Wires show when both ends are on the canvas (wires to other floors show
  // as a tag on the node); the key changes only when the set of shown nodes
  // does, not on every drag.
  const shownKey = useMemo(() => (ws?.nodes ?? []).filter(onScreen).map((n) => n.id).join(' '), [ws?.nodes, onScreen]);
  useEffect(() => {
    setEdges((prev) => {
      const byId = new Map(prev.map((e) => [e.id, e]));
      const shown = new Set(shownKey.split(' '));
      return (ws?.edges ?? [])
        .filter((e) => shown.has(e.a) && shown.has(e.b))
        .map((e) => ({ ...byId.get(e.id), id: e.id, source: e.a, target: e.b, type: 'wire', data: { rope: e.ropePoints } }));
    });
  }, [ws?.edges, shownKey]);

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
        .then(() => toast(`${name(c.source)} ↔ ${name(c.target)} na mesma frequência`))
        .catch((e: Error) => toast(e.message, 'err'));
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
        .catch((err: Error) => toast(err.message, 'err'));
    }
  }, []);

  const onNodeDragStop = useCallback((_: unknown, _node: Node, moved: Node[]) => {
    const st = useSquad.getState();
    for (const n of moved) {
      const x = Math.round(n.position.x);
      const y = Math.round(n.position.y);
      st.moveNodeLocal(n.id, x, y);
      api.updateNode(n.id, { x, y }).catch((e: Error) => toast(e.message, 'err'));
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
      const st = useSquad.getState();
      localStorage.setItem(viewportKey(st.activeWs, st.activeSession, st.activeFloor), JSON.stringify(v));
    } catch {
      /* storage may be unavailable */
    }
  }, []);

  const rf = useReactFlow();
  // Another workspace, session or floor on screen: back to where its canvas
  // was left (defaultViewport only applies on the first render).
  const shown = useRef(`${activeWs}/${session}/${floor}`);
  useEffect(() => {
    if (shown.current === `${activeWs}/${session}/${floor}`) return;
    shown.current = `${activeWs}/${session}/${floor}`;
    void rf.setViewport(defaultViewport);
  }, [activeWs, session, floor, defaultViewport, rf]);
  // A wire's tag on another floor asked to show this node: center it once
  // it is on the canvas.
  const focusRequest = useSquad((s) => s.focusRequest);
  useEffect(() => {
    if (!focusRequest || !nodes.some((n) => n.id === focusRequest)) return;
    const t = window.setTimeout(() => {
      useSquad.getState().clearFocusRequest();
      void rf.fitView({ nodes: [{ id: focusRequest }], duration: 300, maxZoom: 1, padding: 0.3 });
      useSquad.getState().setSelectedNode(focusRequest);
    }, 60);
    return () => window.clearTimeout(t);
  }, [focusRequest, nodes, rf]);
  // Tab: select a node, and bring it into view when it is off screen (at the
  // same zoom).
  const selectRequest = useSquad((s) => s.selectRequest);
  useEffect(() => {
    if (!selectRequest) return;
    useSquad.getState().requestSelect(null);
    setNodes((ns) => ns.map((n) => (n.selected === (n.id === selectRequest) ? n : { ...n, selected: n.id === selectRequest })));
    useSquad.getState().setSelectedNode(selectRequest);
    const n = rf.getInternalNode(selectRequest);
    const pane = document.querySelector('.react-flow')?.getBoundingClientRect();
    if (!n || !pane) return;
    const a = rf.flowToScreenPosition(n.internals.positionAbsolute);
    const b = rf.flowToScreenPosition({ x: n.internals.positionAbsolute.x + (n.measured.width ?? 0), y: n.internals.positionAbsolute.y + (n.measured.height ?? 0) });
    if (a.x >= pane.left && a.y >= pane.top && b.x <= pane.right && b.y <= pane.bottom) return;
    const z = rf.getZoom();
    void rf.fitView({ nodes: [{ id: selectRequest }], duration: 250, minZoom: z, maxZoom: z });
  }, [selectRequest, rf]);
  // Double-clicking a wire adds a bend point there.
  const onEdgeDoubleClick = useCallback(
    (e: React.MouseEvent, edge: Edge) => {
      const a = rf.getInternalNode(edge.source);
      const b = rf.getInternalNode(edge.target);
      const we = useSquad.getState().workspace?.edges.find((x) => x.id === edge.id);
      if (!a || !b || !we) return;
      const rect = (n: typeof a) => ({ x: n.internals.positionAbsolute.x, y: n.internals.positionAbsolute.y, w: n.measured.width ?? 0, h: n.measured.height ?? 0 });
      const p = rf.screenToFlowPosition({ x: e.clientX, y: e.clientY });
      api.updateEdge(edge.id, insertRopePoint(rect(a), rect(b), we.ropePoints ?? [], p)).catch((err: Error) => toast(err.message, 'err'));
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
