import { useEffect, useState } from 'react';
import { api } from '../api/client';
import { useNow } from '../lib/useNow';
import { wsColor } from '../lib/workspaces';
import { useSquad } from '../state/store';
import { FloorBar } from './FloorBar';
import { Icon } from './Icon';
import { SessionsMenu } from './SessionsMenu';
import { UsageChip } from './UsageMenu';
import { WorkspaceMenu } from './WorkspaceMenu';
import './TopBar.css';

/** The control tower (viewBox 56×64): shaft, cab, green glass, orange beacon. */
export function TowerLogo({ size = 16 }: { size?: number }) {
  return (
    <svg className="tower-logo" width={size} height={(size * 64) / 56} viewBox="0 0 56 64" aria-hidden="true">
      <path d="M20 62 L24 26 H32 L36 62 Z" fill="currentColor" />
      <path d="M10 26 H46 L40 12 H16 Z" fill="currentColor" />
      <path d="M18 14 H38 L35 22 H21 Z" fill="var(--brand-glass, #7fe0a0)" />
      <circle cx="28" cy="4" r="3" fill="var(--brand, #e05a2b)" />
    </svg>
  );
}

function Logo() {
  return (
    <div className="tb-logo" aria-label="Torre">
      <TowerLogo />
      <span className="tb-wordmark">TORRE</span>
    </div>
  );
}

export function TopBar() {
  const extra = useSquad((s) => s.extra);
  const connection = useSquad((s) => s.connection);
  const drawerOpen = useSquad((s) => s.drawerOpen);
  const setDrawer = useSquad((s) => s.setDrawer);
  const [wsMenu, setWsMenu] = useState(false);
  const [sesMenu, setSesMenu] = useState(false);
  const workspaces = useSquad((s) => s.workspaces);
  const view = useSquad((s) => s.view);
  const activeWs = useSquad((s) => s.activeWs);
  const ws = workspaces.find((w) => w.id === activeWs);
  const index = workspaces.findIndex((w) => w.id === activeWs);
  const wsName = useSquad((s) => s.workspace?.name) ?? ws?.name;
  // The session list comes first: the workspace list follows a moment later.
  const session = useSquad(
    (s) => s.sessionLists[s.activeWs ?? '']?.sessions.find((x) => x.id === s.activeSession) ?? s.workspaces.find((w) => w.id === s.activeWs)?.session,
  );
  const elsewhere = workspaces.some((w) => w.id !== activeWs && (w.active > 0 || w.pending > 0));
  const askElsewhere = workspaces.some((w) => w.id !== activeWs && (w.attention ?? 0) > 0);
  useEffect(() => {
    // switched (⌥1–9 or the list)
    setWsMenu(false);
    setSesMenu(false);
  }, [activeWs]);
  const pending = extra.pending.length;

  const togglePause = () => {
    void (extra.paused ? api.resume() : api.pause());
  };

  return (
    <header className="topbar">
      <Logo />
      <span className="tb-divider" />
      <div className="tb-anchor">
        <button className="tb-crumb tb-workspace" title="Workspaces" aria-expanded={wsMenu} onClick={() => setWsMenu(!wsMenu)}>
          <span className="tb-ws-color" style={{ background: wsColor(ws?.color) }} />
          <span className="tb-ws-name">{wsName ?? '…'}</span>
          {askElsewhere ? (
            <span className="tb-ws-elsewhere ask" title="Um voo de outro workspace pede você" />
          ) : (
            elsewhere && <span className="tb-ws-elsewhere" title="Voos no ar em outro workspace" />
          )}
          {index >= 0 && index < 9 && <span className="tb-kbd mono">⌥{index + 1}</span>}
          <Icon name="expand_more" size={16} className="tb-caret" />
        </button>
        {wsMenu && <WorkspaceMenu onClose={() => setWsMenu(false)} />}
      </div>
      <span className="tb-sep">›</span>
      <div className="tb-anchor tb-shrink">
        <button
          className="tb-crumb tb-session"
          title={session ? `Sessão: ${session.name}` : 'Sessões'}
          aria-expanded={sesMenu}
          onClick={() => setSesMenu(!sesMenu)}
        >
          <Icon name="history" size={16} className="tb-caret" />
          <span className="tb-session-name">{session?.name ?? '…'}</span>
          <Icon name="expand_more" size={16} className="tb-caret" />
        </button>
        {sesMenu && <SessionsMenu onClose={() => setSesMenu(false)} />}
      </div>
      <span className="tb-sep">›</span>
      <FloorBar />
      <span className="tb-spacer" />
      <div className="tb-views" role="tablist" aria-label="Vista">
        {(['space', 'synoptic'] as const).map((v) => (
          <button key={v} role="tab" aria-selected={view === v} className={`tb-view${view === v ? ' on' : ''}`} onClick={() => useSquad.getState().setView(v)} title="⌥S troca">
            {v === 'space' ? 'Espaço' : 'Sinóptico'}
          </button>
        ))}
      </div>
      <button
        className={`tb-btn${extra.paused ? ' hold' : ''}`}
        onClick={togglePause}
        title={extra.paused ? 'Liberar todos: os pedidos retidos seguem (⌥P)' : 'Espera geral: segura os pedidos entre os voos (⌥P)'}
      >
        <Icon name={extra.paused ? 'play_arrow' : 'all_inclusive'} size={16} />
        {extra.paused ? 'Liberar todos' : 'Espera geral'}
        <span className="tb-kbd mono">⌥P</span>
      </button>
      <button className={`tb-btn${drawerOpen ? ' on' : ''}`} onClick={() => setDrawer(!drawerOpen)} title="Rádio: os pedidos entre os voos (⌥A)">
        <Icon name="cell_tower" size={16} />
        Rádio
        {pending > 0 && <span className="tb-count mono">{pending}</span>}
      </button>
      <UsageChip />
      {connection === 'connected' ? (
        <span className="tb-link ok" title="Enlace com o servidor da Torre">
          <i />
          Enlace
        </span>
      ) : (
        <span className="tb-link wait" title="O torre serve parou ou está reiniciando">
          <Icon name="sync" size={15} className="spin" />
          Reconectando…
        </span>
      )}
      <button className="icon-btn tb-gear" onClick={() => useSquad.getState().openSettings()} title="Configurações">
        <Icon name="settings" size={18} />
      </button>
      <Clock />
    </header>
  );
}

/** The tower's clock: hh:mm:ss. */
function Clock() {
  const now = useNow(1000);
  const d = new Date(now);
  const p = (n: number) => String(n).padStart(2, '0');
  return <span className="tb-clock mono">{`${p(d.getHours())}:${p(d.getMinutes())}:${p(d.getSeconds())}`}</span>;
}

