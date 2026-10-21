import { useEffect, useRef, useState } from 'react';
import { NodeResizer, useReactFlow, type Node, type NodeProps, type ResizeParams } from '@xyflow/react';
import { api } from '../../api/client';
import type { TerminalData, TerminalRuntime, UIState, WNode } from '../../api/types';
import { AskOptions } from '../../components/AskOptions';
import { confirmDialog } from '../../components/ConfirmDialog';
import { HiddenScreen } from '../../components/HiddenScreen';
import { CHOICES, askPhrase, choiceTitle, respond } from '../../lib/attention';
import { Icon } from '../../components/Icon';
import { STATE_META, callsign, elapsed, stateLabel } from '../../lib/model';
import { ROLE_AGENTS, changeRole, isManager, roleColor, roleIcon, useCreator, useRole, useRoleLookup } from '../../lib/roles';
import { flightLevel, useBranch, workdir } from '../../lib/floors';
import { TurnCard, useCardShown } from './TurnCard';
import { shortPath, terminalCwd } from '../../lib/paths';
import { dockAt, toggleFocus, useScreenHidden, useScreenPlace, type ScreenPlace } from '../../lib/screens';
import { toast } from '../../lib/toast';
import { pasteIntoFlight, useCargoDrop } from '../../lib/cargoDrop';
import { useNow } from '../../lib/useNow';
import { useNodeName, useSquad } from '../../state/store';
import { termSession } from '../../term/session';
import { gridFor } from '../../term/grid';
import { ScreenSpace, XTermView } from '../../term/XTermView';
import { NodeHandles, connectClass, useConnectTarget } from './NodeHandles';
import { NodeMenu, type MenuEntry } from './NodeMenu';
import { StatePill } from './StatePill';

export type TerminalFlowNode = Node<{ node: WNode }, 'terminal'>;

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
  const place = useScreenPlace(id);
  const hidden = useScreenHidden(id);

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

  // A cargo's files dropped on the flight: their paths in its prompt.
  const ready = useSquad((s) => s.cargoDrag);
  const drop = useCargoDrop((items) => pasteIntoFlight(id, items));
  const cls = `tnode tnode-${level} st-${state}${focused ? ' focused' : ''}${selected ? ' selected' : ''}${connectClass(target)}${ready ? ' drop-ready' : ''}${drop.over ? ' drop-over' : ''}`;
  const dropHint = drop.over && <DropHint />;

  if (level === 'card') {
    // Design card is 240 px wide; shrink it when the node is smaller on screen.
    const fit = Math.min(1, ((width ?? data.node.w) * zoom) / 240);
    return (
      <div className={`${cls} node-drag`} {...drop.props}>
        <NodeHandles id={id} />
        <ScreenSpace zoom={zoom} fit={fit}>
          <TerminalCard id={id} t={t} rt={rt} state={state} />
        </ScreenSpace>
        {dropHint}
      </div>
    );
  }

  if (level === 'mini') {
    return (
      <div className={`${cls} node-drag`} {...drop.props}>
        <NodeHandles id={id} />
        <ScreenSpace zoom={zoom} className="tmini">
          <i className="tn-band" />
          <header className="tm-head">
            <span className="tn-sq" style={{ background: `var(--r-${roleColorKey})` }} />
            <span className="tm-callsign mono">{callsign(t)}</span>
            <span className="tm-name">{t.name}</span>
            <span className="tn-spacer" />
            <StatePill state={state} since={rt?.activitySince} exitCode={rt?.exitCode} yourTurn={rt?.yourTurn} compact />
          </header>
          {hidden ? (
            <div className="tm-body" style={{ position: 'relative' }}>
              <HiddenScreen id={id} compact />
            </div>
          ) : (
            <MiniSnapshot id={id} />
          )}
        </ScreenSpace>
        {dropHint}
      </div>
    );
  }

  const onResizeEnd = (_: unknown, p: ResizeParams) => {
    const td: TerminalData = { ...t, ...gridFor(id, p.width, p.height) };
    const w = Math.round(p.width);
    const h = Math.round(p.height);
    useSquad.getState().moveNodeLocal(id, Math.round(p.x), Math.round(p.y));
    api.updateNode(id, { x: Math.round(p.x), y: Math.round(p.y), w, h, terminal: td }).catch((e: Error) => toast(e.message, 'err'));
  };

  return (
    <div className={`${cls}${cardShown ? ' has-card' : ''}`} {...drop.props}>
      <NodeResizer isVisible={selected && !focused} minWidth={320} minHeight={200} onResizeEnd={onResizeEnd} />
      <NodeHandles id={id} />
      <i className="tn-band" />
      <LiveHeader id={id} t={t} rt={rt} state={state} focused={focused} />
      {state === 'ask' ? <AskStrip rt={rt} /> : <SenderStrip id={id} />}
      <TermBody id={id}>
        {place === 'node' ? <XTermView id={id} zoom={zoom} /> : <Elsewhere id={id} place={place} />}
        {place === 'node' && hidden && <HiddenScreen id={id} />}
        {(rt?.status === 'failed' || rt?.status === 'exited' || rt?.status === 'stopped') && <StoppedOverlay id={id} rt={rt} />}
      </TermBody>
      <LiveFooter id={id} t={t} createdBy={data.node.createdBy} floor={data.node.floor ?? ''} />
      <TurnCard id={id} working={state === 'work'} />
      {state === 'ask' && <RespondBar id={id} rt={rt} focused={focused} />}
      {dropHint}
    </div>
  );
}

/** Over a flight while a cargo's files are dragged above it. */
export function DropHint() {
  return (
    <div className="drop-hint">
      <Icon name="inventory_2" size={18} />
      Soltar para colar o caminho no prompt
      <span className="drop-sub">sem Enter: você termina a mensagem</span>
    </div>
  );
}

/** The node of a flight whose screen is in the Foco or docked at an edge. */
function Elsewhere({ id, place }: { id: string; place: ScreenPlace }) {
  const side = useSquad((s) => s.dock?.side);
  const back = () => (place === 'focus' ? toggleFocus() : useSquad.getState().setDock(null));
  return (
    <div className="tn-elsewhere nodrag" onMouseDown={(e) => e.stopPropagation()}>
      <b>{place === 'focus' ? 'Em foco' : side === 'left' ? 'Encaixado à esquerda' : 'Encaixado à direita'}</b>
      <span>A tela deste voo está {place === 'focus' ? 'no foco' : 'na borda'}.</span>
      <button className="btn" onClick={back} title={id}>
        <Icon name="close_fullscreen" size={15} />
        Trazer de volta
      </button>
    </div>
  );
}

/** PRIORIDADE: what the flight asks you, over its screen (26 px, solid amber). */
export function AskStrip({ rt }: { rt?: TerminalRuntime }) {
  const a = rt?.attention;
  return (
    <div className="tn-ask" title={a?.text}>
      <Icon name="front_hand" size={15} />
      <span className="tn-ask-what">{askPhrase(a)}</span>
      {(a?.detail || (a?.kind !== 'permission' && a?.text)) && <span className="tn-ask-detail">{a?.detail || a?.text}</span>}
    </div>
  );
}

/**
 * RESPONDER, under the flight (outside it): the answers its prompt offers,
 * with their keys (1, 2, 3 when the flight is selected), or on a question
 * or a plan the agent's own options; and ABRIR TELA. An agent whose keys
 * Torre hasn't verified only gets ABRIR TELA.
 */
export function RespondBar({ id, rt, focused }: { id: string; rt?: TerminalRuntime; focused: boolean }) {
  const t = useSquad((s) => s.workspace?.nodes.find((n) => n.id === id)?.terminal);
  const name = useNodeName(id);
  const a = rt?.attention;
  const choices = CHOICES.filter((c) => a?.choices?.includes(c.choice));
  const open = () => {
    useSquad.getState().setFocusedTerminal(id);
    termSession(id).focus();
  };
  return (
    <div className="tn-respond nodrag nopan" onPointerDown={(e) => e.stopPropagation()}>
      <span className="tn-respond-label">Responder</span>
      {choices.map((c) => (
        <button key={c.choice} className={`tn-respond-btn ${c.choice}`} title={choiceTitle(rt?.attention, c)} onClick={() => respond(id, c.choice)}>
          {c.label}
          <span className="kbd">{c.key}</span>
        </button>
      ))}
      {a?.options?.length ? <AskOptions id={id} a={a} who={t ? callsign(t) : name} /> : null}
      {choices.length === 0 && !a?.options?.length && <span className="tn-respond-note">Responda na tela do voo.</span>}
      <span className="tn-spacer" />
      {!focused && (
        <button className="tn-respond-open" onClick={open}>
          <Icon name="open_in_full" size={15} />
          Abrir tela
        </button>
      )}
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
  const hidden = useScreenHidden(id);
  const dock = useSquad((s) => (s.dock?.id === id ? s.dock.side : null));
  const running = rt?.status === 'running' || rt?.status === 'starting';
  const act = (a: 'start' | 'stop' | 'restart' | 'new-conversation') => api.nodeAction(id, a).catch((e: Error) => toast(e.message, 'err'));
  const cs = callsign(t);
  const newConversation = async () => {
    const ok = await confirmDialog({
      title: `Nova conversa em ${cs}?`,
      icon: 'edit_square',
      tone: 'info',
      lose: ['O contexto: o voo reinicia e começa do zero'],
      keep: ['A conversa atual, guardada no agente (Copiar comando de retomada a abre fora da Torre)'],
      action: 'Nova conversa',
    });
    if (ok) act('new-conversation');
  };
  const copyResume = () => {
    const line = resumeLine(t);
    if (!line) return;
    navigator.clipboard.writeText(line).then(
      () => toast(`Copiado: ${line}`),
      () => toast(line),
    );
  };
  const remove = async () => {
    const ok = await confirmDialog({
      title: `Remover o voo ${cs}?`,
      tone: 'danger',
      lose: [running ? `O processo de ${t.name} é encerrado, no meio do que estiver fazendo` : `${t.name} sai do canvas`, 'As frequências dele'],
      keep: [ROLE_AGENTS.has(t.agent) && 'A conversa, guardada no agente', 'Os arquivos que ele mudou'],
      action: 'Remover voo',
    });
    if (ok) api.removeNode(id).catch((e: Error) => toast(e.message, 'err'));
  };
  const items: MenuEntry[] = [
    running
      ? { icon: 'stop_circle', label: 'Parar', hint: 'vai ao hangar', action: () => act('stop') }
      : { icon: 'play_circle', label: 'Iniciar', action: () => act('start') },
    { icon: 'restart_alt', label: 'Reiniciar', action: () => act('restart') },
    ...(ROLE_AGENTS.has(t.agent)
      ? [
          { icon: 'edit_square' as const, label: 'Nova conversa…', action: newConversation },
          ...(resumeLine(t) ? [{ icon: 'content_copy' as const, label: 'Copiar comando de retomada', action: copyResume }] : []),
        ]
      : []),
    { icon: 'tune', label: 'Configurar…', action: () => useSquad.getState().setEditTerm(id) },
    'divider',
    { icon: 'open_in_full', label: 'Foco', hint: '⌥F', action: () => toggleFocus(id) },
    { icon: 'dock_to_left', label: 'Encaixar à esquerda', hint: '⌥⇧←', checked: dock === 'left', action: () => dockAt('left', id) },
    { icon: 'dock_to_right', label: 'Encaixar à direita', hint: '⌥⇧→', checked: dock === 'right', action: () => dockAt('right', id) },
    { icon: 'visibility_off', label: 'Ocultar a tela', checked: hidden, action: () => useSquad.getState().toggleHiddenScreen(id) },
    ...(ROLE_AGENTS.has(t.agent)
      ? [
          {
            icon: 'groups' as const,
            label: 'Líder',
            checked: manager,
            action: () => api.updateNode(id, { terminal: { ...t, manager: !manager } }).catch((e: Error) => toast(e.message, 'err')),
          },
        ]
      : []),
    'divider',
    // Shells have no role (docs/design_handoff_squad/brief.md).
    ...(t.agent === 'shell'
      ? []
      : [
          { section: 'Papel' },
          ...roles.map((r) => ({
            icon: roleIcon(r),
            label: r.name,
            checked: t.role === r.id,
            action: () => changeRole(id, t, t.role === r.id ? '' : r.id, running),
          })),
          { icon: 'badge' as const, label: 'Editar papéis…', action: () => useSquad.getState().openRoles(t.role || null) },
          'divider' as const,
        ]),
    { icon: 'delete', label: 'Remover…', action: remove, danger: true },
  ];
  const color = roleColor(role);
  return (
    <header className="tn-head node-drag">
      <span
        className="tn-sq"
        style={{ background: `var(--r-${color})` }}
        title={role ? (ROLE_AGENTS.has(t.agent) ? `Papel: ${role.name}` : `Papel: ${role.name} (só a etiqueta: ${t.agent} não recebe instruções)`) : 'Sem papel'}
      />
      <span className="tn-callsign mono" title={`Indicativo de ${t.name}`}>
        {callsign(t)}
      </span>
      <span className="tn-name">{t.name}</span>
      <span className="tn-agent mono">{t.agent}</span>
      {manager && (
        <span className="tn-chip" title="Modo líder: cria voos e os põe na frequência dele (torre new). Desligue no menu ⋯">
          <Icon name="groups" size={14} />
          líder
        </span>
      )}
      {focused && (
        <span className="tn-chip on" title="⌥Esc devolve o teclado ao canvas">
          <Icon name="keyboard" size={14} />
          teclado
        </span>
      )}
      <span className="tn-spacer" />
      {rt?.interrupted && (
        <span className="tn-chip wait" title="Interrompido no meio do turno: Continuar, na faixa do topo, manda seguir">
          <Icon name="u_turn_left" size={14} />
          arremetida
        </span>
      )}
      <StatePill state={state} since={rt?.activitySince} exitCode={rt?.exitCode} yourTurn={rt?.yourTurn} />
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
  const level = useSquad((s) => flightLevel(floor ? s.workspace?.floors?.find((f) => f.id === floor) : undefined));
  return (
    <footer className="tn-foot mono">
      <span title={branch ? `${level}, branch ${branch}` : level}>
        {level}
        {branch && ` · ${branch}`}
      </span>
      <span>
        {t.cols || 80}×{t.rows || 24}
      </span>
      {inbox > 0 && <span className="tn-inbox">caixa {inbox}</span>}
      {creator && (
        <span className="tn-creator" title={`${creator.name} criou este voo (torre new)`}>
          <i style={{ background: `var(--r-${creator.color})` }} />
          criado por {creator.callsign}
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
  // The request it is answering; those still in its line wait for the turn to end.
  const line = pending.filter((p) => p.to === id && p.queued);
  const incoming = pending.find((p) => p.to === id && !p.queued);
  const outgoing = pending.find((p) => p.from === id);
  const now = useNow(1000, !!outgoing || !!reply);
  const lookup = useRoleLookup();
  const roleOf = (nid: string) => roleColor(lookup(nodes?.find((n) => n.id === nid)?.terminal?.role));
  // Flights by callsign (PLN); someone not on the canvas by name.
  const cs = (nid: string | undefined, name: string) => {
    const t = nodes?.find((n) => n.id === nid)?.terminal;
    return t ? callsign(t) : name;
  };

  if (incoming) {
    const r = roleOf(incoming.from);
    return (
      <div className="tn-strip" style={{ background: `var(--rb-${r})`, color: `var(--rt-${r})` }}>
        <Icon name="call_received" size={15} />
        Pedido {incoming.id} de {cs(incoming.from, incoming.fromName)} · a resposta volta para ele
        {line.length > 0 && ` · ${line.length} na fila`}
      </div>
    );
  }
  if (line.length > 0) {
    const r = roleOf(line[0].from);
    return (
      <div className="tn-strip" style={{ background: `var(--rb-${r})`, color: `var(--rt-${r})` }}>
        <Icon name="call_received" size={15} />
        {line.length === 1
          ? `Pedido ${line[0].id} de ${cs(line[0].from, line[0].fromName)} na fila · entra quando este turno acabar`
          : `${line.length} pedidos na fila · entram um por vez quando este turno acabar`}
      </div>
    );
  }
  if (outgoing) {
    return (
      <div className="tn-strip" style={{ background: 'var(--stb-wait)', color: 'var(--stt-wait)' }}>
        <Icon name="hourglass_top" size={15} />
        {outgoing.queued ? 'Na fila de' : 'Aguardando'} {cs(outgoing.to, outgoing.toName)} · pedido {outgoing.id} · enviado há {secondsAgo(outgoing.sentAt, now)}
      </div>
    );
  }
  if (reply && now - reply.at < REPLY_STRIP_MS) {
    return (
      <div className="tn-strip" style={{ background: 'var(--stb-done)', color: 'var(--stt-done)' }}>
        <Icon name="reply" size={15} />
        Resposta de {cs(reply.from, reply.fromName)} · {reply.id} · {duration(reply.elapsedMs)}
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

/** PANE (striped red) or HANGAR over a terminal whose process stopped, with INICIAR. */
function StoppedOverlay({ id, rt }: { id: string; rt: TerminalRuntime }) {
  const failed = rt.status === 'failed' || !!rt.error || (rt.exitCode ?? 0) > 0;
  const start = () => api.nodeAction(id, 'start').catch((e: Error) => toast(e.message, 'err'));
  return (
    <div className={`tn-stopped${failed ? ' err' : ''}`}>
      <Icon name={failed ? 'error' : 'bedtime'} size={16} />
      <span className="tn-stopped-text">
        <b>{failed ? `Pane${rt.exitCode ? ` · saída ${rt.exitCode}` : ''}` : 'Hangar'}</b>
        {failed ? rt.error || 'O processo terminou com erro.' : 'O processo parou; a conversa está guardada.'}
      </span>
      <button className="btn" onClick={start}>
        <Icon name="play_arrow" size={16} />
        Iniciar
      </button>
    </div>
  );
}

function MiniSnapshot({ id }: { id: string }) {
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
        style={{ fontSize: 9, lineHeight: '12px' }}
        dangerouslySetInnerHTML={{ __html: snap.html }}
      />
      <footer className="tm-foot mono">
        <span>retrato · há {Math.max(0, Math.round((now - snap.at) / 1000))} s</span>
        <span className="tn-spacer" />
        {inbox > 0 && <span className="tn-inbox">caixa {inbox}</span>}
      </footer>
    </>
  );
}

/** The flight strip below 35% zoom: paper, the state band, the callsign big. */
function TerminalCard({ id, t, rt, state }: { id: string; t: TerminalData; rt?: TerminalRuntime; state: UIState }) {
  const role = useRole(t.role);
  const inbox = useInbox(id);
  const manager = isManager(t, role);
  const timed = state === 'work' || state === 'wait' || state === 'done';
  const now = useNow(1000, timed);
  const extra = inbox > 0 ? `caixa ${inbox}` : manager ? 'líder' : '';
  return (
    <div className="tcard">
      <i className="tn-band" />
      <div className="tc-main">
        <span className="tc-callsign mono">{callsign(t)}</span>
        <span className="tc-sub mono">
          {t.agent}
          {role && ` · ${role.name}`}
        </span>
        {extra && <span className={`tc-extra mono${inbox > 0 ? ' inbox' : ''}`}>{extra}</span>}
      </div>
      <div className="tc-state">
        <Icon name={STATE_META[state].icon} size={18} />
        <span className="tc-word">{stateLabel(state, rt?.exitCode)}</span>
        {timed && <span className="mono tc-time">{elapsed(rt?.activitySince, now)}</span>}
      </div>
    </div>
  );
}
