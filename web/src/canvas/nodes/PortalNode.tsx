import { useEffect, useRef, useState } from 'react';
import { NodeResizer, useReactFlow, type Node, type NodeProps, type ResizeParams } from '@xyflow/react';
import { api } from '../../api/client';
import type { WNode } from '../../api/types';
import { Icon } from '../../components/Icon';
import { toast } from '../../lib/toast';
import { useSquad } from '../../state/store';
import { NodeHandles, connectClass, useConnectTarget } from './NodeHandles';
import { NodeMenu } from './NodeMenu';
import { BUTTONS, mods, usePortalStream, type PortalStream } from './portalStream';

export type PortalFlowNode = Node<{ node: WNode }, 'portal'>;

/**
 * Browser portal (design handoff §4): the page, rendered by a headless
 * browser on the server, streamed as images. Click the page to use it (the
 * keyboard goes to the page until ⌥Esc); the address bar navigates.
 */
export function PortalNode({ id, data, selected }: NodeProps<PortalFlowNode>) {
  const url = data.node.portal?.url ?? '';
  const target = useConnectTarget(id);
  const level = useSquad((s) => s.zoomLevel);
  // Only stream while readable on screen; smaller zoom keeps the last frame.
  const stream = usePortalStream(id, !!url && level === 'live');
  const [focused, setFocused] = useState(false);
  const remove = () => api.removeNode(id).catch((e: Error) => toast(e.message));

  const onResizeEnd = (_: unknown, p: ResizeParams) => {
    useSquad.getState().moveNodeLocal(id, Math.round(p.x), Math.round(p.y));
    api
      .updateNode(id, { x: Math.round(p.x), y: Math.round(p.y), w: Math.round(p.width), h: Math.round(p.height) })
      .catch((e: Error) => toast(e.message));
  };

  return (
    <div className={`pnode${selected ? ' selected' : ''}${focused ? ' focused' : ''}${connectClass(target)}`}>
      <NodeResizer isVisible={selected && !focused} minWidth={320} minHeight={240} onResizeEnd={onResizeEnd} />
      <NodeHandles id={id} />
      <div className="node-stripe" style={{ background: 'var(--line-2)' }} />
      <header className="pn-head node-drag">
        <Icon name="public" size={18} style={{ color: 'var(--text-2)' }} />
        <span className="pn-title" title={stream.state?.title}>
          Portal
        </span>
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
        <AddressBar id={id} url={stream.state?.url || url} />
        {focused ? (
          <span className="tn-kbd-here" title="⌥Esc devolve o teclado ao canvas">
            <Icon name="keyboard" size={15} />
            teclado aqui
          </span>
        ) : (
          <LiveBadge stream={stream} hasUrl={!!url} paused={level !== 'live'} />
        )}
        {url && (
          <a className="icon-btn nodrag" href={stream.state?.url || url} target="_blank" rel="noreferrer noopener" title="Abrir no navegador">
            <Icon name="open_in_new" size={16} />
          </a>
        )}
        <NodeMenu items={[{ icon: 'delete', label: 'Remover', action: remove, danger: true }]} />
      </header>
      <PortalScreen stream={stream} url={url} onFocusChange={setFocused} />
    </div>
  );
}

function AddressBar({ id, url }: { id: string; url: string }) {
  const [editing, setEditing] = useState(false);
  const [draft, setDraft] = useState(url);
  const commit = () => {
    setEditing(false);
    let next = draft.trim();
    if (next && !/^[a-z]+:\/\//i.test(next)) next = `${/^(localhost|127\.|\[::1\])/.test(next) ? 'http' : 'https'}://${next}`;
    if (next && next !== url) api.updateNode(id, { portal: { url: next } }).catch((e: Error) => toast(e.message));
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
    text = 'pausado'; // zoomed out: the last frame, not streaming
  } else if (stream.noBrowser) {
    text = 'sem navegador';
    color = 'var(--st-err)';
  } else if (stream.state?.loading) {
    text = 'carregando…';
    color = 'var(--st-work)';
  } else if (stream.connected && stream.frame) {
    const slow = (stream.latency ?? 0) > 400;
    text = `ao vivo${stream.latency != null ? ` · ${stream.latency} ms` : ''}`;
    color = slow ? 'var(--st-wait)' : 'var(--st-done)';
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
  return e.code === 'KeyA' || e.code === 'KeyP' || e.code === 'KeyK' || e.key === 'Escape' || /^Digit[1-9]$/.test(e.code);
}

/**
 * The page. Pointer positions are mapped from the image on screen (any canvas
 * zoom) to page pixels; plain wheel scrolls the page, pinch or ⌘/ctrl + wheel
 * zooms the canvas.
 */
function PortalScreen({ stream, url, onFocusChange }: { stream: PortalStream; url: string; onFocusChange: (f: boolean) => void }) {
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
      <div className="pn-msg">
        <Icon name="public_off" size={28} />
        <span>Nenhum navegador encontrado para mostrar a página.</span>
        <span className="pn-hint">Instale o Chrome ou o Edge, ou defina SQUAD_BROWSER com o caminho do executável, e reinicie o Squad.</span>
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
    </div>
  );
}
