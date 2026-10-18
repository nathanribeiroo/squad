import { useEffect, useRef } from 'react';
import { api } from '../api/client';
import type { UIState } from '../api/types';
import { AskStrip, RespondBar } from '../canvas/nodes/TerminalNode';
import { StatePill } from '../canvas/nodes/StatePill';
import { callsign } from '../lib/model';
import { roleColor, useRole } from '../lib/roles';
import { dockAt, toggleFocus, useScreenHidden } from '../lib/screens';
import { useSquad } from '../state/store';
import { gridFor, gridIn } from '../term/grid';
import { termSession } from '../term/session';
import { XTermView } from '../term/XTermView';
import { HiddenScreen } from './HiddenScreen';
import { Icon } from './Icon';
import './ScreenPane.css';

const node = (id: string) => useSquad.getState().workspace?.nodes.find((n) => n.id === id);

/**
 * While a flight is in the Foco or docked, its terminal takes the size of
 * the pane (the agent redraws for it); back on the canvas, the size of its
 * node again.
 */
function useFitPty(id: string, body: React.RefObject<HTMLDivElement | null>) {
  useEffect(() => {
    const el = body.current;
    if (!el) return;
    let timer = 0;
    const fit = () => {
      window.clearTimeout(timer);
      timer = window.setTimeout(() => {
        const slot = el.querySelector<HTMLElement>('.xterm-slot');
        const t = node(id)?.terminal;
        const g = slot && gridIn(id, slot.clientWidth, slot.clientHeight);
        if (!t || !g || (t.cols === g.cols && t.rows === g.rows)) return;
        api.updateNode(id, { terminal: { ...t, ...g } }).catch(() => {});
      }, 150);
    };
    // Leaving the page with the pane open gives the node its size back too.
    const restore = (keepalive: boolean) => {
      const n = node(id);
      const g = n && gridFor(id, n.w, n.h);
      if (n?.terminal && g && (g.cols !== n.terminal.cols || g.rows !== n.terminal.rows)) {
        api.updateNode(id, { terminal: { ...n.terminal, ...g } }, keepalive).catch(() => {});
      }
    };
    const leave = () => restore(true);
    const ro = new ResizeObserver(fit);
    ro.observe(el);
    void termSession(id).ready.then(fit);
    window.addEventListener('pagehide', leave);
    return () => {
      ro.disconnect();
      window.clearTimeout(timer);
      window.removeEventListener('pagehide', leave);
      restore(false);
    };
  }, [id, body]);
}

/**
 * A flight's screen out of the canvas (docs/torre/TELAS.md, quadro 04): on
 * the whole stage (Foco, ⌥F) or docked at an edge with the canvas beside it
 * (⌥⇧←/→, a 2 px divider).
 */
export function ScreenPane({ id, mode }: { id: string; mode: 'focus' | 'left' | 'right' }) {
  const n = useSquad((s) => s.workspace?.nodes.find((x) => x.id === id));
  const rt = useSquad((s) => s.runtime[id]);
  const focused = useSquad((s) => s.focusedTerminal === id);
  const hidden = useScreenHidden(id);
  const role = useRole(n?.terminal?.role);
  const body = useRef<HTMLDivElement>(null);
  useFitPty(id, body);
  // The keyboard follows the flight into the pane.
  useEffect(() => {
    const t = window.setTimeout(() => termSession(id).focus(), 80);
    return () => window.clearTimeout(t);
  }, [id, mode]);
  if (!n?.terminal) return null;
  const t = n.terminal;
  const state: UIState = rt?.activity ?? 'boot';
  const where = mode === 'focus' ? 'foco · ⌥F volta' : mode === 'left' ? 'encaixado à esquerda' : 'encaixado à direita';
  const close = () => (mode === 'focus' ? toggleFocus() : useSquad.getState().setDock(null));
  return (
    <section className={`sp sp-${mode} st-${state}`} aria-label={`${callsign(t)} · ${where}`}>
      <i className="tn-band" />
      <header className="sp-head">
        <span className="tn-sq" style={{ background: `var(--r-${roleColor(role)})` }} />
        <b className="mono sp-cs">{callsign(t)}</b>
        <span className="sp-name">{t.name}</span>
        <span className="tn-agent mono">{t.agent}</span>
        <span className="tn-spacer" />
        <span className="sp-where mono">{where}</span>
        <StatePill state={state} since={rt?.activitySince} exitCode={rt?.exitCode} yourTurn={rt?.yourTurn} />
        {mode !== 'focus' && (
          <button className="icon-btn" onClick={() => dockAt(mode === 'left' ? 'right' : 'left', id)} title={mode === 'left' ? 'Encaixar à direita (⌥⇧→)' : 'Encaixar à esquerda (⌥⇧←)'}>
            <Icon name={mode === 'left' ? 'arrow_forward' : 'arrow_back'} size={16} />
          </button>
        )}
        <button className="icon-btn" onClick={close} title={mode === 'focus' ? 'Voltar ao canvas (⌥F)' : 'Desencaixar: volta ao canvas'}>
          <Icon name="close_fullscreen" size={16} />
        </button>
      </header>
      {state === 'ask' && <AskStrip rt={rt} />}
      <div ref={body} className="tn-body sp-body" onMouseDown={() => termSession(id).focus()}>
        <XTermView id={id} zoom={1} />
        {hidden && <HiddenScreen id={id} />}
      </div>
      {state === 'ask' && <RespondBar id={id} rt={rt} focused={focused} />}
    </section>
  );
}
