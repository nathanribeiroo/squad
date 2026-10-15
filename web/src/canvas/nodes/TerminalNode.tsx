import { useEffect, useRef, useState } from 'react';
import { NodeResizer, useReactFlow, type Node, type NodeProps, type ResizeParams } from '@xyflow/react';
import { api } from '../../api/client';
import type { Role, TerminalData, TerminalRuntime, UIState, WNode } from '../../api/types';
import { Icon } from '../../components/Icon';
import { ROLE_META, STATE_META, elapsed, roleKey, stateLabel } from '../../lib/model';
import { toast } from '../../lib/toast';
import { useNow } from '../../lib/useNow';
import { useSquad } from '../../state/store';
import { termSession } from '../../term/session';
import { ScreenSpace, XTermView } from '../../term/XTermView';
import { NodeHandles, connectClass, useConnectTarget } from './NodeHandles';
import { NodeMenu, type MenuItem } from './NodeMenu';
import { StatePill } from './StatePill';

export type TerminalFlowNode = Node<{ node: WNode }, 'terminal'>;

// Fixed chrome of the live node, in world px (handoff §2): borders, role
// stripe, header, footer and the body padding around the terminal grid.
const CHROME_W = 2 + 20;
const CHROME_H = 2 + 3 + 32 + 22 + 16;

function shortPath(p?: string): string {
  if (!p) return '';
  const m = p.match(/^\/(Users|home)\/[^/]+/);
  return m ? '~' + p.slice(m[0].length) : p;
}

/** Terminal grid that fits a node of w×h world px. */
function gridFor(id: string, w: number, h: number): { cols: number; rows: number } | null {
  const cell = termSession(id).cellSize();
  if (!cell) return null;
  return { cols: Math.max(20, Math.floor((w - CHROME_W) / cell.w)), rows: Math.max(5, Math.floor((h - CHROME_H) / cell.h)) };
}

function useInbox(id: string): number {
  return useSquad((s) => s.extra.pending.reduce((n, p) => (p.to === id ? n + 1 : n), 0));
}

export function TerminalNode({ id, data, selected, width }: NodeProps<TerminalFlowNode>) {
  const t = data.node.terminal!;
  const rt = useSquad((s) => s.runtime[id]);
  const level = useSquad((s) => s.zoomLevel);
  const zoom = useSquad((s) => s.renderZoom);
  const focused = useSquad((s) => s.focusedTerminal === id);
  const target = useConnectTarget(id);
  const state: UIState = rt?.activity ?? 'boot';

  // The session exists for every level, so miniatures have a live buffer.
  useEffect(() => {
    termSession(id);
  }, [id]);
  useEffect(() => {
    if (rt?.status === 'running') termSession(id).kick();
  }, [id, rt?.status]);
  useEffect(() => {
    if (t.cols && t.rows) termSession(id).resize(t.cols, t.rows);
  }, [id, t.cols, t.rows]);
  // A terminal without a saved size gets the grid that fits its node.
  const sized = !!(t.cols && t.rows);
  useEffect(() => {
    if (sized || level !== 'live') return;
    let live = true;
    void termSession(id).ready.then(() => {
      const n = useSquad.getState().workspace?.nodes.find((x) => x.id === id);
      if (!live || !n?.terminal || (n.terminal.cols && n.terminal.rows)) return;
      const g = gridFor(id, n.w, n.h);
      if (g) api.updateNode(id, { terminal: { ...n.terminal, ...g } }).catch(() => {});
    });
    return () => {
      live = false;
    };
  }, [id, sized, level]);

  const cls = `tnode tnode-${level}${focused ? ' focused' : ''}${selected ? ' selected' : ''}${connectClass(target)}`;

  if (level === 'card') {
    // Design card is 240 px wide; shrink it when the node is smaller on screen.
    const fit = Math.min(1, ((width ?? data.node.w) * zoom) / 240);
    return (
      <div className={`${cls} node-drag`}>
        <NodeHandles id={id} />
        <ScreenSpace zoom={zoom} fit={fit}>
          <TerminalCard id={id} t={t} rt={rt} state={state} />
        </ScreenSpace>
      </div>
    );
  }

  if (level === 'mini') {
    return (
      <div className={`${cls} node-drag`}>
        <NodeHandles id={id} />
        <ScreenSpace zoom={zoom} className="tmini">
          <div className="node-stripe" style={{ background: `var(--r-${roleKey(t.role)})` }} />
          <header className="tm-head">
            <span className="tm-name">{t.name}</span>
            <span className="tm-agent mono">{t.agent}</span>
            <span className="tn-spacer" />
            <StatePill state={state} since={rt?.activitySince} exitCode={rt?.exitCode} compact />
          </header>
          <MiniSnapshot id={id} zoom={zoom} />
        </ScreenSpace>
      </div>
    );
  }

  const onResizeEnd = (_: unknown, p: ResizeParams) => {
    const td: TerminalData = { ...t, ...gridFor(id, p.width, p.height) };
    const w = Math.round(p.width);
    const h = Math.round(p.height);
    useSquad.getState().moveNodeLocal(id, Math.round(p.x), Math.round(p.y));
    api.updateNode(id, { x: Math.round(p.x), y: Math.round(p.y), w, h, terminal: td }).catch((e: Error) => toast(e.message));
  };

  return (
    <div className={cls}>
      <NodeResizer isVisible={selected && !focused} minWidth={320} minHeight={200} onResizeEnd={onResizeEnd} />
      <NodeHandles id={id} />
      <div className="node-stripe" style={{ background: `var(--r-${roleKey(t.role)})` }} />
      <LiveHeader id={id} t={t} rt={rt} state={state} focused={focused} />
      <TermBody id={id}>
        <XTermView id={id} zoom={zoom} />
        {(rt?.status === 'failed' || rt?.status === 'exited' || rt?.status === 'stopped') && <StoppedOverlay rt={rt} />}
      </TermBody>
      <LiveFooter id={id} t={t} />
    </div>
  );
}

/**
 * Terminal body. Wheel: plain scrolling stays in the terminal (the canvas
 * does not pan); pinch / ⌘ or ctrl + wheel zooms the canvas around the
 * pointer instead of reaching the terminal or zooming the page.
 */
function TermBody({ id, children }: { id: string; children: React.ReactNode }) {
  const ref = useRef<HTMLDivElement>(null);
  const rf = useReactFlow();
  useEffect(() => {
    const el = ref.current!;
    const zoom = (e: WheelEvent) => {
      if (!e.ctrlKey && !e.metaKey) return;
      e.preventDefault();
      e.stopPropagation();
      const pane = el.closest('.react-flow')!.getBoundingClientRect();
      const vp = rf.getViewport();
      const step = e.deltaMode === 1 ? 0.05 : 0.002;
      const z = Math.min(2, Math.max(0.15, vp.zoom * Math.pow(2, -e.deltaY * step * 10)));
      const px = e.clientX - pane.left;
      const py = e.clientY - pane.top;
      void rf.setViewport({ x: px - ((px - vp.x) * z) / vp.zoom, y: py - ((py - vp.y) * z) / vp.zoom, zoom: z });
    };
    const keep = (e: WheelEvent) => {
      if (!e.ctrlKey && !e.metaKey) e.stopPropagation();
    };
    el.addEventListener('wheel', zoom, { capture: true, passive: false });
    el.addEventListener('wheel', keep);
    return () => {
      el.removeEventListener('wheel', zoom, { capture: true });
      el.removeEventListener('wheel', keep);
    };
  }, [rf]);
  return (
    <div ref={ref} className="tn-body nodrag" onMouseDown={() => termSession(id).focus()}>
      {children}
    </div>
  );
}

function LiveHeader({ id, t, rt, state, focused }: { id: string; t: TerminalData; rt?: TerminalRuntime; state: UIState; focused: boolean }) {
  const role = roleKey(t.role);
  const running = rt?.status === 'running' || rt?.status === 'starting';
  const act = (a: 'start' | 'stop' | 'restart') => api.nodeAction(id, a).catch((e: Error) => toast(e.message));
  const setRole = (r: Role) => api.updateNode(id, { terminal: { ...t, role: r } }).catch((e: Error) => toast(e.message));
  const remove = () => {
    if (window.confirm(`Remover "${t.name}"? O processo do terminal é encerrado.`)) api.removeNode(id).catch((e: Error) => toast(e.message));
  };
  const items: (MenuItem | 'divider')[] = [
    running
      ? { icon: 'stop_circle', label: 'Parar', action: () => act('stop') }
      : { icon: 'play_circle', label: 'Iniciar', action: () => act('start') },
    { icon: 'restart_alt', label: 'Reiniciar', action: () => act('restart') },
    'divider',
    ...(Object.keys(ROLE_META) as Exclude<Role, ''>[]).map((r) => ({
      icon: ROLE_META[r].icon,
      label: ROLE_META[r].label,
      checked: t.role === r,
      action: () => setRole(t.role === r ? '' : r),
    })),
    'divider',
    { icon: 'delete', label: 'Remover', action: remove, danger: true },
  ];
  return (
    <header className="tn-head node-drag">
      <span className="tn-name">{t.name}</span>
      <span className="tn-agent mono">{t.agent}</span>
      {role !== 'none' && (
        <span className="tn-role" style={{ background: `var(--rb-${role})`, color: `var(--r-${role})` }}>
          <Icon name={ROLE_META[role].icon} size={14} />
          {ROLE_META[role].label}
        </span>
      )}
      <span className="tn-spacer" />
      {focused && (
        <span className="tn-kbd-here" title="⌥Esc devolve o teclado ao canvas">
          <Icon name="keyboard" size={15} />
          teclado aqui
        </span>
      )}
      <StatePill state={state} since={rt?.activitySince} exitCode={rt?.exitCode} />
      <NodeMenu items={items} />
    </header>
  );
}

function LiveFooter({ id, t }: { id: string; t: TerminalData }) {
  const cwd = useSquad((s) => t.cwd || s.workspace?.cwd);
  const inbox = useInbox(id);
  return (
    <footer className="tn-foot mono">
      <span>⎇ main</span>
      <span>
        {t.cols || 80}×{t.rows || 24}
      </span>
      {inbox > 0 && <span style={{ color: 'var(--st-wait)' }}>caixa {inbox}</span>}
      <span className="tn-spacer" />
      <span className="tn-cwd">{shortPath(cwd)}</span>
    </footer>
  );
}

function StoppedOverlay({ rt }: { rt: TerminalRuntime }) {
  const failed = rt.status === 'failed' || !!rt.error;
  return (
    <div className={`tn-stopped${failed ? ' err' : ''}`}>
      <Icon name={failed ? 'error' : 'bedtime'} size={16} />
      {failed ? rt.error || 'O processo terminou com erro.' : `Processo encerrado${rt.exitCode ? ` (saída ${rt.exitCode})` : ''}.`} Use o menu ⋯ para iniciar.
    </div>
  );
}

function MiniSnapshot({ id, zoom }: { id: string; zoom: number }) {
  const [snap, setSnap] = useState({ html: '', at: Date.now() });
  const now = useNow(1000);
  const inbox = useInbox(id);
  useEffect(() => {
    const s = termSession(id);
    const tick = () => setSnap({ html: s.snapshotHTML(), at: Date.now() });
    tick();
    const timer = window.setInterval(tick, 3000);
    return () => window.clearInterval(timer);
  }, [id]);
  return (
    <>
      <div
        className="tm-body"
        style={{ fontSize: 11 * zoom, lineHeight: `${15 * zoom}px` }}
        dangerouslySetInnerHTML={{ __html: snap.html }}
      />
      <footer className="tm-foot">
        <Icon name="photo_camera" size={13} />
        <span className="mono">miniatura · há {Math.max(0, Math.round((now - snap.at) / 1000))} s</span>
        <span className="tn-spacer" />
        {inbox > 0 && <span className="mono" style={{ color: 'var(--st-wait)' }}>caixa {inbox}</span>}
      </footer>
    </>
  );
}

function TerminalCard({ id, t, rt, state }: { id: string; t: TerminalData; rt?: TerminalRuntime; state: UIState }) {
  const role = roleKey(t.role);
  const inbox = useInbox(id);
  const timed = state === 'work' || state === 'wait' || state === 'done';
  const now = useNow(1000, timed);
  return (
    <div className="tcard">
      <div className="tc-stripe" style={{ background: `var(--r-${role})` }} />
      <div className="tc-main">
        <div className="tc-title">
          <span className="tc-name">{t.name}</span>
          {inbox > 0 && <span className="tc-inbox mono">{inbox}</span>}
        </div>
        <div className="tc-sub">
          <span className="mono">{t.agent}</span>
          {role !== 'none' && (
            <>
              {' · '}
              <span style={{ color: `var(--r-${role})`, fontWeight: 600 }}>{ROLE_META[role].label}</span>
            </>
          )}
        </div>
      </div>
      <div className="tc-band" style={{ background: `var(--stb-${state})`, color: `var(--st-${state})` }}>
        <Icon name={STATE_META[state].icon} size={20} />
        <span className="tc-state">{stateLabel(state, rt?.exitCode)}</span>
        <span className="tn-spacer" />
        {timed && <span className="mono tc-time">{elapsed(rt?.activitySince, now)}</span>}
      </div>
    </div>
  );
}
