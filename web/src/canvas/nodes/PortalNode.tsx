import { useEffect, useRef, useState } from 'react';
import { NodeResizer, useReactFlow, type Node, type NodeProps, type ResizeParams } from '@xyflow/react';
import { api } from '../../api/client';
import type { PortalData, WNode } from '../../api/types';
import { Icon } from '../../components/Icon';
import { toast } from '../../lib/toast';
import { useSquad } from '../../state/store';
import { NodeHandles, connectClass, useConnectTarget } from './NodeHandles';
import { NodeMenu } from './NodeMenu';
import { actionVerb, usePortalAction } from './portalAction';
import { useCreator } from '../../lib/roles';
import { BUTTONS, mods, usePortalStream, type PortalStream } from './portalStream';

export type PortalFlowNode = Node<{ node: WNode }, 'portal'>;

/**
 * Browser portal (design handoff §4): the page, rendered by a headless
 * browser on the server, streamed as images. Click the page to use it (the
 * keyboard goes to the page until ⌥Esc); the address bar navigates. Agents
 * wired to it use it too (torre portal …): their cursor shows on the page.
 */
export function PortalNode({ id, data, selected }: NodeProps<PortalFlowNode>) {
  const portal: PortalData = data.node.portal ?? { url: '' };
  const url = portal.url;
  const target = useConnectTarget(id);
  const level = useSquad((s) => s.zoomLevel);
  // Only stream while readable on screen; smaller zoom keeps the last frame.
  const stream = usePortalStream(id, !!url && level === 'live');
  const [focused, setFocused] = useState(false);
  const [renaming, setRenaming] = useState(false);
  const creator = useCreator(data.node.createdBy);
  const remove = () => api.removeNode(id).catch((e: Error) => toast(e.message, 'err'));

  const onResizeEnd = (_: unknown, p: ResizeParams) => {
    useSquad.getState().moveNodeLocal(id, Math.round(p.x), Math.round(p.y));
    api
      .updateNode(id, { x: Math.round(p.x), y: Math.round(p.y), w: Math.round(p.width), h: Math.round(p.height) })
      .catch((e: Error) => toast(e.message, 'err'));
  };

  return (
    <div className={`pnode${selected ? ' selected' : ''}${focused ? ' focused' : ''}${connectClass(target)}`}>
      <NodeResizer isVisible={selected && !focused} minWidth={320} minHeight={240} onResizeEnd={onResizeEnd} />
      <NodeHandles id={id} />
      <header className="pn-head node-drag">
        <Icon name="public" size={18} style={{ color: 'var(--text-2)' }} />
        {renaming ? (
          <NameInput id={id} portal={portal} onDone={() => setRenaming(false)} />
        ) : (
          <span
            className="pn-title"
            title={`${stream.state?.title ? `${stream.state.title}\n` : ''}Os voos usam este nome: torre portal "${portal.name || 'Portal'}" … (duplo clique para renomear)`}
            onDoubleClick={() => setRenaming(true)}
          >
            {portal.name || 'Portal'}
          </span>
        )}
        {creator && (
          <span className="pn-creator" title={`${creator.name} abriu este portal (torre portal create)`}>
            <i style={{ background: `var(--r-${creator.color})` }} />
            por {creator.callsign}
          </span>
        )}
        <span className="pn-nav nodrag">
          <button className="icon-btn" disabled={!stream.state?.canBack} onClick={() => stream.send({ type: 'nav', action: 'back' })} aria-label="Voltar" title="Voltar">
            <Icon name="arrow_back" size={16} />
          </button>
          <button className="icon-btn" disabled={!stream.state?.canForward} onClick={() => stream.send({ type: 'nav', action: 'forward' })} aria-label="Avançar" title="Avançar">
            <Icon name="arrow_forward" size={16} />
          </button>
          <button className="icon-btn" disabled={!stream.connected} onClick={() => stream.send({ type: 'nav', action: 'reload' })} aria-label="Recarregar" title="Recarregar">
            <Icon name="refresh" size={16} />
          </button>
        </span>
        <AddressBar id={id} portal={portal} url={stream.state?.url || url} />
        {focused ? (
          <span className="tn-kbd-here" title="⌥Esc devolve o teclado ao canvas">
            <Icon name="keyboard" size={14} />
            teclado
          </span>
        ) : (
          <LiveBadge stream={stream} hasUrl={!!url} paused={level !== 'live'} />
        )}
        {url && (
          <a className="icon-btn nodrag" href={stream.state?.url || url} target="_blank" rel="noreferrer noopener" title="Abrir no navegador">
            <Icon name="open_in_new" size={16} />
          </a>
        )}
        <NodeMenu
          items={[
            { icon: 'edit', label: 'Renomear', action: () => setRenaming(true) },
            { icon: 'delete', label: 'Remover', action: remove, danger: true },
          ]}
        />
      </header>
      <PortalScreen id={id} stream={stream} url={url} onFocusChange={setFocused} />
    </div>
  );
}

/** Renames the portal; names are unique, agents use them to pick a portal. */
function NameInput({ id, portal, onDone }: { id: string; portal: PortalData; onDone: () => void }) {
  const cancelled = useRef(false);
  const commit = (value: string) => {
    onDone();
    const name = value.trim();
    if (cancelled.current || !name || name === portal.name) return;
    const taken = useSquad
      .getState()
      .workspace?.nodes.some((n) => n.id !== id && n.portal?.name?.toLowerCase() === name.toLowerCase());
    if (taken) {
      toast(`Já existe um portal chamado "${name}"`);
      return;
    }
    api.updateNode(id, { portal: { ...portal, name } }).catch((e: Error) => toast(e.message, 'err'));
  };
  return (
    <input
      className="pn-name nodrag"
      defaultValue={portal.name || 'Portal'}
      autoFocus
      spellCheck={false}
      maxLength={40}
      aria-label="Nome do portal"
      onFocus={(e) => e.target.select()}
      onBlur={(e) => commit(e.target.value)}
      onKeyDown={(e) => {
        if (e.key === 'Enter') e.currentTarget.blur();
        if (e.key === 'Escape') {
          e.stopPropagation();
          cancelled.current = true;
          e.currentTarget.blur();
        }
      }}
    />
  );
}

function AddressBar({ id, portal, url }: { id: string; portal: PortalData; url: string }) {
  const [editing, setEditing] = useState(false);
  const [draft, setDraft] = useState(url);
  const commit = () => {
    setEditing(false);
    let next = draft.trim();
    if (next && !/^[a-z]+:\/\//i.test(next)) next = `${/^(localhost|127\.|\[::1\])/.test(next) ? 'http' : 'https'}://${next}`;
    if (next && next !== url) api.updateNode(id, { portal: { ...portal, url: next } }).catch((e: Error) => toast(e.message, 'err'));
  };
  return editing ? (
    <input
      className="pn-url mono nodrag"
      value={draft}
      autoFocus
      spellCheck={false}
      placeholder="http://localhost:5173"
      onChange={(e) => setDraft(e.target.value)}
      onBlur={commit}
      onKeyDown={(e) => {
        if (e.key === 'Enter') commit();
        if (e.key === 'Escape') {
          e.stopPropagation();
          setDraft(url);
          setEditing(false);
        }
      }}
    />
  ) : (
    <button
      className="pn-url mono nodrag"
      onClick={() => {
        setDraft(url);
        setEditing(true);
      }}
      title="Editar endereço"
    >
      {url || 'definir endereço…'}
    </button>
  );
}

/** "● ao vivo · 120 ms" (amber above 400 ms), or why there is no picture. */
function LiveBadge({ stream, hasUrl, paused }: { stream: PortalStream; hasUrl: boolean; paused: boolean }) {
  let text = 'sem transmissão';
  let color = 'var(--text-3)';
  if (paused && stream.frame) {
    text = 'pausado · zoom'; // zoomed out: the last frame, not streaming
  } else if (stream.noBrowser) {
    text = 'sem navegador';
    color = 'var(--stt-err)';
  } else if (stream.state?.loading) {
    text = 'carregando…';
    color = 'var(--stt-work)';
  } else if (stream.connected && stream.frame) {
    const slow = (stream.latency ?? 0) > 400;
    text = `ao vivo${stream.latency != null ? ` · ${stream.latency} ms` : ''}`;
    color = slow ? 'var(--stt-wait)' : 'var(--stt-done)';
  } else if (hasUrl && stream.connected) {
    text = 'abrindo…';
  }
  return (
    <span className="pn-live" style={{ color }}>
      <i />
      {text}
    </span>
  );
}

/** Keys the canvas keeps while the page has the keyboard (see term/session). */
function canvasKey(e: React.KeyboardEvent): boolean {
  if (!e.altKey || e.ctrlKey || e.metaKey) return false;
  if (e.shiftKey && (e.code === 'ArrowLeft' || e.code === 'ArrowRight')) return true; // dock
  return e.code === 'KeyA' || e.code === 'KeyP' || e.code === 'KeyK' || e.code === 'KeyF' || e.key === 'Escape' || /^Digit[1-9]$/.test(e.code);
}

/**
 * The page. Pointer positions are mapped from the image on screen (any canvas
 * zoom) to page pixels; plain wheel scrolls the page, pinch or ⌘/ctrl + wheel
 * zooms the canvas.
 */
function PortalScreen({ id, stream, url, onFocusChange }: { id: string; stream: PortalStream; url: string; onFocusChange: (f: boolean) => void }) {
  const ref = useRef<HTMLDivElement>(null);
  const rf = useReactFlow();
  const moveQueued = useRef<MouseEvent | null>(null);
  const st = stream.state;

  const toPage = (e: { clientX: number; clientY: number }) => {
    const r = ref.current!.getBoundingClientRect();
    const w = st?.width || r.width;
    const h = st?.height || r.height;
    return { x: ((e.clientX - r.left) * w) / r.width, y: ((e.clientY - r.top) * h) / r.height };
  };

  // Wheel: native listeners (React's are passive) so preventDefault works.
  const sendRef = useRef(stream.send);
  sendRef.current = stream.send;
  const toPageRef = useRef(toPage);
  toPageRef.current = toPage;
  useEffect(() => {
    const el = ref.current!;
    const onWheel = (e: WheelEvent) => {
      e.preventDefault();
      e.stopPropagation();
      if (e.ctrlKey || e.metaKey) {
        const pane = el.closest('.react-flow')!.getBoundingClientRect();
        const vp = rf.getViewport();
        const step = e.deltaMode === 1 ? 0.05 : 0.002;
        const z = Math.min(2, Math.max(0.15, vp.zoom * Math.pow(2, -e.deltaY * step * 10)));
        const px = e.clientX - pane.left;
        const py = e.clientY - pane.top;
        void rf.setViewport({ x: px - ((px - vp.x) * z) / vp.zoom, y: py - ((py - vp.y) * z) / vp.zoom, zoom: z });
        return;
      }
      const line = e.deltaMode === 1 ? 40 : 1;
      sendRef.current({ type: 'mouse', event: 'wheel', ...toPageRef.current(e), dx: e.deltaX * line, dy: e.deltaY * line, mods: mods(e) });
    };
    el.addEventListener('wheel', onWheel, { passive: false });
    return () => el.removeEventListener('wheel', onWheel);
  }, [rf]);

  const pointer = (event: 'down' | 'up', e: React.MouseEvent) => {
    stream.send({ type: 'mouse', event, ...toPage(e), button: BUTTONS[e.button] ?? 'left', clicks: e.detail || 1, mods: mods(e) });
  };

  const onMouseMove = (e: React.MouseEvent) => {
    // One move per frame is enough.
    if (moveQueued.current) {
      moveQueued.current = e.nativeEvent;
      return;
    }
    moveQueued.current = e.nativeEvent;
    requestAnimationFrame(() => {
      const ev = moveQueued.current!;
      moveQueued.current = null;
      const button = ev.buttons & 1 ? 'left' : ev.buttons & 2 ? 'right' : ev.buttons & 4 ? 'middle' : 'none';
      stream.send({ type: 'mouse', event: 'move', ...toPage(ev), button, mods: mods(ev) });
    });
  };

  const onKey = (e: React.KeyboardEvent) => {
    if (canvasKey(e)) {
      if (e.key === 'Escape' && e.type === 'keydown') ref.current?.blur();
      return; // ⌥A, ⌥P, ⌥K, ⌥1–9 reach the app
    }
    e.preventDefault();
    e.stopPropagation();
    stream.send({ type: 'key', event: e.type === 'keydown' ? 'down' : 'up', key: e.key, code: e.code, keyCode: e.keyCode, mods: mods(e) });
  };

  let overlay: React.ReactNode = null;
  if (stream.noBrowser) {
    overlay = (
      <div className="pn-msg err">
        <Icon name="public_off" size={28} />
        <span>Nenhum Chrome, Edge ou Chromium no servidor.</span>
        <span className="pn-hint">Instale um deles ou defina TORRE_BROWSER com o caminho do executável, e reinicie a Torre.</span>
      </div>
    );
  } else if (stream.error) {
    overlay = (
      <div className="pn-msg">
        <Icon name="sync_problem" size={28} />
        <span>{stream.error}</span>
      </div>
    );
  } else if (!url) {
    overlay = (
      <div className="pn-msg">
        <Icon name="public" size={28} />
        <span>Defina um endereço na barra acima para abrir a página.</span>
      </div>
    );
  } else if (!stream.frame) {
    overlay = (
      <div className="pn-msg">
        <Icon name="progress_activity" size={24} className="spin" />
        <span>Abrindo a página…</span>
      </div>
    );
  }

  return (
    <div
      ref={ref}
      className="pn-screen nodrag nowheel"
      tabIndex={0}
      onFocus={() => onFocusChange(true)}
      onBlur={() => onFocusChange(false)}
      onMouseDown={(e) => {
        ref.current?.focus({ preventScroll: true });
        pointer('down', e);
      }}
      onMouseUp={(e) => pointer('up', e)}
      onMouseMove={onMouseMove}
      onContextMenu={(e) => e.preventDefault()}
      onKeyDown={onKey}
      onKeyUp={onKey}
      onPaste={(e) => {
        const text = e.clipboardData.getData('text');
        if (text) {
          e.preventDefault();
          stream.send({ type: 'text', text });
        }
      }}
    >
      {stream.frame && <img src={stream.frame} alt="" draggable={false} />}
      {st?.error && stream.frame && <div className="pn-error mono">Não foi possível abrir: {st.error}</div>}
      {overlay}
      {stream.frame && <AgentCursor id={id} width={st?.width} height={st?.height} />}
    </div>
  );
}

/**
 * An agent acting on the page: its cursor and name where it clicks (gliding
 * from its last spot), or a chip in the corner for keys, typing and navigation.
 */
function AgentCursor({ id, width, height }: { id: string; width?: number; height?: number }) {
  const cur = usePortalAction(id);
  if (!cur) return null;
  const { act, role } = cur;
  const who = act.byName || 'Agente';
  const color = `var(--r-${role})`;
  const doing = `${actionVerb(act.kind)}${act.label && !act.hasXY ? ` ${act.label}` : ''}`;
  if (!act.hasXY || !width || !height) {
    return (
      <div key={act.seq} className="pn-agent-chip" style={{ borderColor: color }}>
        <i style={{ background: color }} />
        <b style={{ color }}>{who}</b>
        <span className="mono">{doing}</span>
      </div>
    );
  }
  return (
    <div className="pn-cursor" style={{ left: `${(act.x / width) * 100}%`, top: `${(act.y / height) * 100}%`, color }}>
      <span key={act.seq} className="pn-cursor-body">
        <span className="pn-ripple" />
        <Icon name="arrow_selector_tool" size={22} fill />
        <span className="pn-cursor-name" style={{ background: color }}>
          {who} · {actionVerb(act.kind)}
        </span>
      </span>
    </div>
  );
}
