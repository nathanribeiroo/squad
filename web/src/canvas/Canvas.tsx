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
import { confirmDialog } from '../components/ConfirmDialog';
import { MiniMapAndZoom } from '../components/ZoomControls';
import { cargoItems, hasCargo } from '../lib/cargo';
import { callsign } from '../lib/model';
import { hiddenInStack, stackGains, stackOf } from '../lib/stacks';
import { toast } from '../lib/toast';
import { hiddenBy, insideGroup, removeVisuals, reveal, shownAs } from '../lib/visual';
import { useSquad } from '../state/store';
import { pruneSessions } from '../term/session';
import { DrawLayer } from './DrawLayer';
import { WireEdge } from './edges/WireEdge';
import { insertRopePoint } from './geometry';
import { CargoNode } from './nodes/CargoNode';
import { DrawingNode } from './nodes/DrawingNode';
import { GroupNode } from './nodes/GroupNode';
import { NoteNode } from './nodes/NoteNode';
import { PortalNode } from './nodes/PortalNode';
import { TerminalNode } from './nodes/TerminalNode';
import './canvas.css';
import './nodes/nodes.css';

// A group is a "frame" to React Flow, whose "group" type has styles of its own.
const nodeTypes = { terminal: TerminalNode, note: NoteNode, portal: PortalNode, cargo: CargoNode, frame: GroupNode, drawing: DrawingNode };
const flowType = (n: WNode) => (n.kind === 'group' ? 'frame' : n.kind);
const isVisualType = (t?: string) => t === 'frame' || t === 'drawing';
/** Nodes no wire reaches: frames and drawings, and cargo (its files go by drag and drop). */
const unwired = (t?: string) => isVisualType(t) || t === 'cargo';
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
    type: flowType(n),
    // Keep the local position while the user is dragging.
    position: prev?.dragging ? prev.position : { x: n.x, y: n.y },
    width: n.w,
    height: n.h,
    // A drawing is taken by its lines anywhere (nodes.css).
    dragHandle: n.kind === 'drawing' ? undefined : '.node-drag',
    // Group frames at the back, drawings over them, then the rest. A
    // selected node rises by 1000: a selected frame comes to the front (its
    // tab and menu over the nodes inside, which still take the clicks).
    zIndex: n.kind === 'group' ? (n.group?.collapsed ? 0 : -2) : n.kind === 'drawing' ? -1 : undefined,
    data: { node: n },
  };
}

/** Group frames first, then drawings: what is drawn first is behind. */
const layer = (n: WNode) => (n.kind === 'group' && !n.group?.collapsed ? 0 : n.kind === 'drawing' ? 1 : 2);

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
  // A note stacked on another is drawn inside the stack (its base's node);
  // a collapsed group hides what it holds behind its card.
  const hidden = useMemo(() => hiddenBy(ws?.nodes ?? []), [ws?.nodes]);
  const hiddenKey = [...hidden].map(([id, g]) => `${id}:${g}`).join(' ');
  const onScreen = useCallback((n: WNode) => (n.floor ?? '') === floor && !hiddenInStack(n) && !hidden.has(n.id), [floor, hidden]);
  useEffect(() => {
    setNodes((prev) => {
      const byId = new Map(prev.map((n) => [n.id, n]));
      return (ws?.nodes ?? [])
        .filter(onScreen)
        .sort((a, b) => layer(a) - layer(b))
        .map((n) => toFlow(n, byId.get(n.id)));
    });
    if (ws) pruneSessions(new Set(ws.nodes.filter((n) => n.kind === 'terminal').map((n) => n.id)));
  }, [ws?.nodes, onScreen]);
  useEffect(() => useSquad.getState().setZoom(defaultViewport.zoom, true), [defaultViewport]);
  // Wires show when both ends are on the canvas (wires to other floors show
  // as a tag on the node); the key changes only when the set of shown nodes
  // does, not on every drag. A wire to a node a collapsed group hides ends
  // at the group's card (and one inside the group isn't drawn).
  const shownKey = useMemo(() => (ws?.nodes ?? []).filter(onScreen).map((n) => n.id).join(' '), [ws?.nodes, onScreen]);
  useEffect(() => {
    const hid = new Map(hiddenKey ? hiddenKey.split(' ').map((p) => p.split(':') as [string, string]) : []);
    setEdges((prev) => {
      const byId = new Map(prev.map((e) => [e.id, e]));
      const shown = new Set(shownKey.split(' '));
      return (ws?.edges ?? []).flatMap((e) => {
        const a = shownAs(hid, e.a);
        const b = shownAs(hid, e.b);
        if (a === b || !shown.has(a) || !shown.has(b)) return [];
        const via = a !== e.a || b !== e.b;
        return [{ ...byId.get(e.id), id: e.id, source: a, target: b, type: 'wire', data: via ? { a: e.a, b: e.b } : { rope: e.ropePoints } }];
      });
    });
  }, [ws?.edges, shownKey, hiddenKey]);

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

  // Keyboard deletion removes wires, drawings and group frames; the other
  // nodes are removed from their menu (removing a terminal kills its process).
  const onBeforeDelete: OnBeforeDelete = useCallback(async ({ nodes, edges }) => {
    const visuals = nodes.filter((n) => isVisualType(n.type));
    removeVisuals(visuals.map((n) => n.id));
    return { nodes: visuals, edges };
  }, []);
  const onEdgesDelete = useCallback((removed: Edge[]) => {
    for (const e of removed) {
      api
        .removeEdge(e.id)
        .then(() => toast(`${name(e.source)} ↔ ${name(e.target)} desligados`))
        .catch((err: Error) => toast(err.message, 'err'));
    }
  }, []);

  // Where the dragged nodes were, to go back if stacking is cancelled; what
  // a dragged group carries along (what lies inside it), from where.
  const dragFrom = useRef<Record<string, { x: number; y: number }>>({});
  const carried = useRef<Record<string, { x: number; y: number }>>({});
  const onNodeDragStart = useCallback((_: unknown, _node: Node, moved: Node[]) => {
    dragFrom.current = Object.fromEntries(moved.map((n) => [n.id, { ...n.position }]));
    const all = useSquad.getState().workspace?.nodes ?? [];
    const out: Record<string, { x: number; y: number }> = {};
    for (const m of moved) {
      const g = m.type === 'frame' ? all.find((n) => n.id === m.id) : undefined;
      if (g) for (const c of insideGroup(all, g)) if (!(c.id in dragFrom.current)) out[c.id] = { x: c.x, y: c.y };
    }
    carried.current = out;
  }, []);
  const delta = (moved: Node[]) => {
    const from = dragFrom.current[moved[0].id];
    return from ? { dx: moved[0].position.x - from.x, dy: moved[0].position.y - from.y } : { dx: 0, dy: 0 };
  };
  const onNodeDrag = useCallback((_: unknown, _node: Node, moved: Node[]) => {
    const c = carried.current;
    if (!Object.keys(c).length) return;
    const { dx, dy } = delta(moved);
    setNodes((ns) => ns.map((n) => (c[n.id] ? { ...n, dragging: true, position: { x: c[n.id].x + dx, y: c[n.id].y + dy } } : n)));
  }, []);
  const onNodeDragStop = useCallback((_: unknown, _node: Node, moved: Node[]) => {
    const st = useSquad.getState();
    const c = carried.current;
    carried.current = {};
    // A group moves with what is inside it, at once.
    if (Object.keys(c).length) {
      const { dx, dy } = delta(moved);
      const moves = [
        ...moved.map((n) => ({ id: n.id, x: Math.round(n.position.x), y: Math.round(n.position.y) })),
        ...Object.entries(c).map(([id, p]) => ({ id, x: Math.round(p.x + dx), y: Math.round(p.y + dy) })),
      ];
      st.moveNodesLocal(moves);
      setNodes((ns) => ns.map((n) => (c[n.id] ? { ...n, dragging: false } : n)));
      api.moveNodes(moves).catch((e: Error) => toast(e.message, 'err'));
      return;
    }
    // A note dropped on another note (its center inside it) stacks on it.
    const onto = moved.length === 1 && moved[0].type === 'note' ? noteUnder(moved[0]) : null;
    if (onto) {
      void stackOnto(moved[0].id, onto, dragFrom.current[moved[0].id]);
      return;
    }
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
  // it is on the canvas (opening the groups that hide it).
  const focusRequest = useSquad((s) => s.focusRequest);
  useEffect(() => {
    if (focusRequest && hiddenBy(useSquad.getState().workspace?.nodes ?? []).has(focusRequest)) void reveal(focusRequest);
  }, [focusRequest]);
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
      if (!a || !b || !we || edge.source !== we.a || edge.target !== we.b) return; // not one ending at a group's card
      const rect = (n: typeof a) => ({ x: n.internals.positionAbsolute.x, y: n.internals.positionAbsolute.y, w: n.measured.width ?? 0, h: n.measured.height ?? 0 });
      const p = rf.screenToFlowPosition({ x: e.clientX, y: e.clientY });
      api.updateEdge(edge.id, insertRopePoint(rect(a), rect(b), we.ropePoints ?? [], p)).catch((err: Error) => toast(err.message, 'err'));
    },
    [rf],
  );

  // A cargo's folder dropped on the empty canvas: a cargo of it there.
  const onPane = (e: React.DragEvent) => (e.target as HTMLElement).classList?.contains('react-flow__pane');
  const onDragOver = useCallback((e: React.DragEvent) => {
    if (!hasCargo(e.dataTransfer) || !onPane(e)) return;
    e.preventDefault();
    e.dataTransfer.dropEffect = 'copy';
  }, []);
  const onDrop = useCallback(
    (e: React.DragEvent) => {
      if (!hasCargo(e.dataTransfer) || !onPane(e)) return;
      e.preventDefault();
      useSquad.getState().setCargoDrag(false);
      const dirs = cargoItems(e.dataTransfer).filter((i) => i.dir);
      if (!dirs.length) {
        toast('Solte uma pasta para abrir uma carga dela; arquivos vão para um voo ou para Transmitir.', 'wait');
        return;
      }
      const p = rf.screenToFlowPosition({ x: e.clientX, y: e.clientY });
      const floor = useSquad.getState().activeFloor;
      dirs.slice(0, 6).forEach((d, i) =>
        api
          .addNode({ kind: 'cargo', x: Math.round(p.x + i * 30), y: Math.round(p.y + i * 30), floor: floor || undefined, cargo: { name: '', path: d.path } })
          .catch((err: Error) => toast(err.message, 'err')),
      );
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
      isValidConnection={(c) =>
        c.source !== c.target && !linked(c.source, c.target) && !nodes.some((n) => (n.id === c.source || n.id === c.target) && unwired(n.type))
      }
      onBeforeDelete={onBeforeDelete}
      onEdgesDelete={onEdgesDelete}
      onEdgeDoubleClick={onEdgeDoubleClick}
      onDragOver={onDragOver}
      onDrop={onDrop}
      zoomOnDoubleClick={false}
      onNodeDragStart={onNodeDragStart}
      onNodeDrag={onNodeDrag}
      onNodeDragStop={onNodeDragStop}
      onSelectionChange={({ nodes: sel }) => {
        // React Flow calls this on every frame of a drag (the selected nodes
        // move); the store changes only when the selection does.
        const st = useSquad.getState();
        const one = sel.length === 1 ? sel[0].id : null;
        if (one === st.selectedNode && sel.length === st.selectedIds.length && sel.every((n, i) => n.id === st.selectedIds[i])) return;
        setSelected(one);
        st.setSelectedIds(sel.map((n) => n.id));
      }}
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
      <DrawLayer />
    </ReactFlow>
  );
}

/** The note whose box holds the center of a dragged note, on its level. */
function noteUnder(dragged: Node): string | null {
  const ws = useSquad.getState().workspace;
  const me = ws?.nodes.find((n) => n.id === dragged.id);
  if (!ws || !me) return null;
  const cx = dragged.position.x + me.w / 2;
  const cy = dragged.position.y + me.h / 2;
  const hit = ws.nodes.find(
    (n) => n.id !== me.id && n.kind === 'note' && !hiddenInStack(n) && (n.floor ?? '') === (me.floor ?? '') && cx >= n.x && cx <= n.x + n.w && cy >= n.y && cy <= n.y + n.h,
  );
  return hit?.id ?? null;
}

/**
 * Stacks a note (or a stack) on another. When flights wired to one side
 * would see the notes of the other, the user confirms it first; cancelling
 * puts the note back where it was.
 */
async function stackOnto(id: string, onto: string, from?: { x: number; y: number }) {
  const st = useSquad.getState();
  const ws = st.workspace;
  if (!ws) return;
  const back = () => from && st.moveNodeLocal(id, Math.round(from.x), Math.round(from.y));
  const title = (x: string) => ws.nodes.find((n) => n.id === x)?.note?.title ?? '';
  const gains = stackGains(ws.nodes, ws.edges, id, onto);
  if (gains.length > 0) {
    const count = stackOf(ws.nodes, ws.nodes.find((n) => n.id === onto)!).length + stackOf(ws.nodes, ws.nodes.find((n) => n.id === id)!).length;
    const who = gains.map((g) => callsign(g.terminal!));
    const names = who.length > 1 ? `${who.slice(0, -1).join(', ')} e ${who[who.length - 1]}` : who[0];
    const ok = await confirmDialog({
      title: `Empilhar ${title(id)} sobre ${title(onto)}?`,
      icon: 'layers',
      tone: 'info',
      text: (
        <>
          {names} {who.length > 1 ? 'passam' : 'passa'} a ler e escrever as {count} NOTAMs da pilha: um fio ligado à pilha vale para todas.
        </>
      ),
      keep: ['Cada NOTAM continua com o seu arquivo e o seu cadeado', 'Tirar da pilha devolve a NOTAM com as frequências da pilha'],
      action: 'Empilhar',
    });
    if (!ok) return back();
  }
  api.stackNote(id, onto).catch((e: Error) => {
    back();
    toast(e.message, 'err');
  });
}
