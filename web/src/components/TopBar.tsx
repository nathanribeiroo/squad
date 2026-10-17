import { useEffect, useState } from 'react';
import { api } from '../api/client';
import { useActiveFloor, useBranch } from '../lib/floors';
import { shortPath } from '../lib/paths';
import { useNow } from '../lib/useNow';
import { wsColor } from '../lib/workspaces';
import { useSquad } from '../state/store';
import { FloorBar } from './FloorBar';
import { Icon } from './Icon';
import { SessionsMenu } from './SessionsMenu';
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
  const ws = useSquad((s) => s.workspace);
  const runtime = useSquad((s) => s.runtime);
  const extra = useSquad((s) => s.extra);
  const connection = useSquad((s) => s.connection);
  const drawerOpen = useSquad((s) => s.drawerOpen);
  const setDrawer = useSquad((s) => s.setDrawer);
  const [menu, setMenu] = useState(false);
  const [wsMenu, setWsMenu] = useState(false);
  const [sesMenu, setSesMenu] = useState(false);
  const workspaces = useSquad((s) => s.workspaces);
  const activeWs = useSquad((s) => s.activeWs);
  const index = workspaces.findIndex((w) => w.id === activeWs);
  // The session list comes first: the workspace list follows a moment later.
  const session = useSquad(
    (s) => s.sessionLists[s.activeWs ?? '']?.sessions.find((x) => x.id === s.activeSession) ?? s.workspaces.find((w) => w.id === s.activeWs)?.session,
  );
  const elsewhere = workspaces.some((w) => w.id !== activeWs && (w.active > 0 || w.pending > 0));
  useEffect(() => {
    // switched (⌥1–9 or the list)
    setWsMenu(false);
    setSesMenu(false);
  }, [activeWs]);
  const floor = useActiveFloor();
  const branch = useBranch(floor?.id ?? '');
  const cwd = floor?.cwd ?? ws?.copy?.cwd ?? ws?.cwd ?? '';

  const sendTimes = useSquad((s) => s.sendTimes);
  const now = useNow(5000, sendTimes.length > 0);
  const rate = sendTimes.filter((t) => now - t < 60_000).length;
  const active = Object.values(runtime).filter((r) => r.activity === 'work' || r.activity === 'wait').length;
  const pending = extra.pending.length;

  const togglePause = () => {
    void (extra.paused ? api.resume() : api.pause());
  };

  return (
    <header className="topbar">
      <Logo />
      <span className="tb-divider" />
      <div style={{ position: 'relative' }}>
        <button className="tb-workspace" title="Workspaces" aria-expanded={wsMenu} onClick={() => setWsMenu(!wsMenu)}>
          <span className="tb-ws-color" style={{ background: wsColor(ws?.color) }} />
          <span className="tb-ws-name">{ws?.name ?? '…'}</span>
          <span className="tb-ws-sub">
            {active} {active === 1 ? 'voo no ar' : 'voos no ar'}
          </span>
          {elsewhere && <span className="tb-ws-elsewhere" title="Voos no ar em outro workspace" />}
          {index >= 0 && index < 9 && <span className="kbd">⌥{index + 1}</span>}
          <Icon name="expand_more" size={18} />
        </button>
        {wsMenu && <WorkspaceMenu onClose={() => setWsMenu(false)} />}
      </div>
      <Icon name="chevron_right" size={18} style={{ color: 'var(--text-3)' }} />
      <div style={{ position: 'relative', minWidth: 0 }}>
        <button
          className="tb-session"
          title={session ? `Sessão: ${session.name}` : 'Sessões'}
          aria-expanded={sesMenu}
          onClick={() => setSesMenu(!sesMenu)}
        >
          <Icon name="history" size={16} />
          <span className="tb-session-name">{session?.name ?? '…'}</span>
          <Icon name="expand_more" size={18} />
        </button>
        {sesMenu && <SessionsMenu onClose={() => setSesMenu(false)} />}
      </div>
      <Icon name="chevron_right" size={18} style={{ color: 'var(--text-3)' }} />
      <FloorBar />
      <span className="tb-path mono" title={cwd}>
        {branch && `⎇ ${branch} · `}
        {shortPath(cwd)}
      </span>
      <span className="tb-spacer" />
      <span className="tb-rate" title="Pedidos entre agentes no último minuto">
        <Icon name="swap_horiz" size={18} />
        <span className="mono">
          {rate}/{extra.limits.perMinute}
        </span>
        <span className="tb-rate-label">por min</span>
      </span>
      <button className="btn" onClick={togglePause} title={extra.paused ? 'Liberar todos (⌥P)' : 'Espera geral: segura os pedidos entre os voos (⌥P)'}>
        <Icon name={extra.paused ? 'play_arrow' : 'pause'} />
        {extra.paused ? 'Liberar todos' : 'Espera geral'}
        <span className="kbd">⌥P</span>
      </button>
      <button className={`btn${drawerOpen ? ' filled' : ''}`} onClick={() => setDrawer(!drawerOpen)} title="Rádio (⌥A)">
        <Icon name="cell_tower" />
        Rádio
        {pending > 0 && <span className="tb-count mono">{pending}</span>}
      </button>
      <span className={`tb-server ${connection === 'connected' ? 'ok' : 'wait'}`}>
        <Icon name="lan" size={18} />
        {connection === 'connected' ? 'Enlace' : 'Reconectando…'}
      </span>
      <div style={{ position: 'relative' }}>
        <button className="icon-btn" onClick={() => setMenu(!menu)} title="Configurações">
          <Icon name="settings" />
        </button>
        {menu && <SettingsMenu onClose={() => setMenu(false)} />}
      </div>
    </header>
  );
}

function SettingsMenu({ onClose }: { onClose: () => void }) {
  const theme = useSquad((s) => s.theme);
  const term = useSquad((s) => s.termTheme);
  const setTheme = useSquad((s) => s.setTheme);
  const setTerm = useSquad((s) => s.setTermTheme);
  return (
    <div className="popover tb-menu" onMouseLeave={onClose}>
      <div className="tb-menu-title">Tema</div>
      {(['dark', 'light'] as const).map((t) => (
        <button key={t} className={`tb-menu-item${theme === t ? ' on' : ''}`} onClick={() => setTheme(t)}>
          <Icon name={t === 'dark' ? 'dark_mode' : 'light_mode'} size={18} />
          {t === 'dark' ? 'Escuro' : 'Claro'}
        </button>
      ))}
      <div className="tb-menu-title">Agentes</div>
      <button
        className="tb-menu-item"
        onClick={() => {
          onClose();
          useSquad.getState().openRoles();
        }}
      >
        <Icon name="badge" size={18} />
        Papéis…
      </button>
      <button
        className="tb-menu-item"
        onClick={() => {
          onClose();
          useSquad.getState().setSummaryDialog('settings');
        }}
      >
        <Icon name="psychology" size={18} />
        Resumos e briefing…
      </button>
      <div className="tb-menu-title">Terminal</div>
      {(['noite', 'papel'] as const).map((t) => (
        <button key={t} className={`tb-menu-item${term === t ? ' on' : ''}`} onClick={() => setTerm(t)}>
          <Icon name="terminal" size={18} />
          {t === 'noite' ? 'Noite' : 'Papel'}
        </button>
      ))}
    </div>
  );
}
