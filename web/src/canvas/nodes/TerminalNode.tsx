import { useEffect, useRef, useState } from 'react';
import { NodeResizer, useReactFlow, type Node, type NodeProps, type ResizeParams } from '@xyflow/react';
import { api } from '../../api/client';
import type { TerminalData, TerminalRuntime, UIState, WNode } from '../../api/types';
import { Icon } from '../../components/Icon';
import { STATE_META, elapsed, stateLabel } from '../../lib/model';
import { ROLE_AGENTS, changeRole, isManager, roleColor, roleIcon, useCreator, useRole, useRoleLookup } from '../../lib/roles';
import { useBranch, workdir } from '../../lib/floors';
import { TurnCard, useCardShown } from './TurnCard';
import { shortPath, terminalCwd } from '../../lib/paths';
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
  const roleColorKey = roleColor(useRole(t.role));
  const cardShown = useCardShown(id, state === 'work');

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
          <div className="node-stripe" style={{ background: `var(--r-${roleColorKey})` }} />
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
    <div className={`${cls}${cardShown ? ' has-card' : ''}`}>
      <NodeResizer isVisible={selected && !focused} minWidth={320} minHeight={200} onResizeEnd={onResizeEnd} />
      <NodeHandles id={id} />
      <div className="node-stripe" style={{ background: `var(--r-${roleColorKey})` }} />
      <LiveHeader id={id} t={t} rt={rt} state={state} focused={focused} />
      <SenderStrip id={id} />
      <TermBody id={id}>
        <XTermView id={id} zoom={zoom} />
        {(rt?.status === 'failed' || rt?.status === 'exited' || rt?.status === 'stopped') && <StoppedOverlay rt={rt} />}
      </TermBody>
      <LiveFooter id={id} t={t} createdBy={data.node.createdBy} floor={data.node.floor ?? ''} />
      <TurnCard id={id} working={state === 'work'} />
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

/** The command that opens a terminal's conversation outside Torre, in its folder. */
function resumeLine(t: TerminalData): string {
  if (!t.conversation || t.empty) return '';
  return t.agent === 'codex' ? `codex resume ${t.conversation}` : `${t.agent} --resume ${t.conversation}`;
}

function LiveHeader({ id, t, rt, state, focused }: { id: string; t: TerminalData; rt?: TerminalRuntime; state: UIState; focused: boolean }) {
  const role = useRole(t.role);
  const roles = useSquad((s) => s.roles);
  const manager = isManager(t, role);
  const running = rt?.status === 'running' || rt?.status === 'starting';
  const act = (a: 'start' | 'stop' | 'restart' | 'new-conversation') => api.nodeAction(id, a).catch((e: Error) => toast(e.message));
  const newConversation = () => {
    if (window.confirm(`Começar uma conversa nova em ${t.name}? O agente reinicia sem o contexto da conversa atual (ela continua guardada no agente).`))
      act('new-conversation');
  };
  const copyResume = () => {
    const line = resumeLine(t);
    if (!line) return;
    navigator.clipboard.writeText(line).then(
      () => toast(`Copiado: ${line}`),
      () => toast(line),
    );
  };
  const remove = () => {
    if (window.confirm(`Remover "${t.name}"? O processo do terminal é encerrado.`)) api.removeNode(id).catch((e: Error) => toast(e.message));
  };
  const items: (MenuItem | 'divider')[] = [
    running
      ? { icon: 'stop_circle', label: 'Parar', action: () => act('stop') }
      : { icon: 'play_circle', label: 'Iniciar', action: () => act('start') },
    { icon: 'restart_alt', label: 'Reiniciar', action: () => act('restart') },
    ...(ROLE_AGENTS.has(t.agent)
      ? [
          { icon: 'edit_square' as const, label: 'Nova conversa', action: newConversation },
          ...(resumeLine(t) ? [{ icon: 'content_copy' as const, label: 'Copiar comando de retomada', action: copyResume }] : []),
        ]
      : []),
    { icon: 'tune', label: 'Configurar…', action: () => useSquad.getState().setEditTerm(id) },
    ...(ROLE_AGENTS.has(t.agent)
      ? [
          {
            icon: 'groups' as const,
            label: 'Modo gerente',
            checked: manager,
            action: () => api.updateNode(id, { terminal: { ...t, manager: !manager } }).catch((e: Error) => toast(e.message)),
          },
        ]
      : []),
    'divider',
    // Shells have no role (docs/design_handoff_squad/brief.md).
    ...(t.agent === 'shell'
      ? []
      : [
          ...roles.map((r) => ({
            icon: roleIcon(r),
            label: r.name,
            checked: t.role === r.id,
            action: () => changeRole(id, t, t.role === r.id ? '' : r.id, running),
          })),
          { icon: 'badge' as const, label: 'Editar papéis…', action: () => useSquad.getState().openRoles(t.role || null) },
          'divider' as const,
        ]),
    { icon: 'delete', label: 'Remover', action: remove, danger: true },
  ];
  return (
    <header className="tn-head node-drag">
      <span className="tn-name">{t.name}</span>
      <span className="tn-agent mono">{t.agent}</span>
      {role && (
        <span
          className="tn-role"
          style={{ background: `var(--rb-${role.color})`, color: `var(--r-${role.color})` }}
          title={ROLE_AGENTS.has(t.agent) ? `Papel: ${role.name}` : `Papel: ${role.name} (só a etiqueta: ${t.agent} não recebe instruções)`}
        >
          <Icon name={roleIcon(role)} size={14} />
          {role.name}
        </span>
      )}
      {manager && (
        <span className="tn-manager" title="Modo gerente: cria e liga terminais (torre new). Desligue no menu ⋯">
          <Icon name="groups" size={15} />
          gerente
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

function LiveFooter({ id, t, createdBy, floor }: { id: string; t: TerminalData; createdBy?: string; floor: string }) {
  // Relative folders start from the workspace folder, or its floor's copy.
  const cwd = useSquad((s) => terminalCwd(t.cwd, workdir(s.workspace, floor)));
  const branch = useBranch(floor);
  const inbox = useInbox(id);
  const creator = useCreator(createdBy);
  return (
    <footer className="tn-foot mono">
      {branch && <span title="Branch do andar">⎇ {branch}</span>}
      <span>
        {t.cols || 80}×{t.rows || 24}
      </span>
      {inbox > 0 && <span style={{ color: 'var(--st-wait)' }}>caixa {inbox}</span>}
      {creator && (
        <span className="tn-creator" title={`${creator.name} criou este terminal (torre new)`}>
          <i style={{ background: `var(--r-${creator.color})` }} />
          criado por {creator.name}
        </span>
      )}
      <span className="tn-spacer" />
      <span className="tn-cwd" title={cwd}>
        <bdi>{shortPath(cwd)}</bdi>
      </span>
    </footer>
  );
}

const REPLY_STRIP_MS = 60_000;

/**
 * Strip above the terminal (never over it, handoff §2): an incoming request,
 * this terminal waiting on one, or the reply it just got.
 */
function SenderStrip({ id }: { id: string }) {
  const pending = useSquad((s) => s.extra.pending);
  const reply = useSquad((s) => s.lastReply[id]);
  const nodes = useSquad((s) => s.workspace?.nodes);
  const incoming = pending.find((p) => p.to === id);
  const outgoing = pending.find((p) => p.from === id);
  const now = useNow(1000, !!outgoing || !!reply);
  const lookup = useRoleLookup();
  const roleOf = (nid: string) => roleColor(lookup(nodes?.find((n) => n.id === nid)?.terminal?.role));

  if (incoming) {
    const r = roleOf(incoming.from);
    return (
      <div className="tn-strip" style={{ background: `var(--rb-${r})`, color: `var(--r-${r})` }}>
        <Icon name="call_received" size={15} />
        Pedido {incoming.id} de {incoming.fromName} · a resposta volta para ele
      </div>
    );
  }
  if (outgoing) {
    return (
      <div className="tn-strip" style={{ background: 'var(--stb-wait)', color: 'var(--st-wait)' }}>
        <Icon name="hourglass_top" size={15} />
        Aguardando {outgoing.toName} · pedido {outgoing.id} · enviado há {secondsAgo(outgoing.sentAt, now)}
      </div>
    );
  }
  if (reply && now - reply.at < REPLY_STRIP_MS) {
    return (
      <div className="tn-strip" style={{ background: 'var(--stb-done)', color: 'var(--st-done)' }}>
        <Icon name="reply" size={15} />
        Resposta de {reply.fromName} · {reply.id} · {duration(reply.elapsedMs)}
      </div>
    );
  }
  return null;
}

function secondsAgo(t: string, now: number): string {
  const s = Math.max(0, Math.round((now - Date.parse(t)) / 1000));
  return s < 60 ? `${s} s` : elapsed(t, now);
}

/** 2m14s, 42s */
function duration(ms: number): string {
  const s = Math.round(ms / 1000);
  return s < 60 ? `${s}s` : `${Math.floor(s / 60)}m${String(s % 60).padStart(2, '0')}s`;
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
  const role = useRole(t.role);
  const inbox = useInbox(id);
  const timed = state === 'work' || state === 'wait' || state === 'done';
  const now = useNow(1000, timed);
  return (
    <div className="tcard">
      <div className="tc-stripe" style={{ background: `var(--r-${roleColor(role)})` }} />
      <div className="tc-main">
        <div className="tc-title">
          <span className="tc-name">{t.name}</span>
          {inbox > 0 && <span className="tc-inbox mono">{inbox}</span>}
        </div>
        <div className="tc-sub">
          <span className="mono">{t.agent}</span>
          {role && (
            <>
              {' · '}
              <span style={{ color: `var(--r-${role.color})`, fontWeight: 600 }}>{role.name}</span>
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
