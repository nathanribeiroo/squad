import { useEffect, useState } from 'react';
import { api } from '../api/client';
import { useActiveFloor, useBranch } from '../lib/floors';
import { shortPath } from '../lib/paths';
import { useNow } from '../lib/useNow';
import { wsColor } from '../lib/workspaces';
import { useSquad } from '../state/store';
import { FloorBar } from './FloorBar';
import { Icon } from './Icon';
import { WorkspaceMenu } from './WorkspaceMenu';
import './TopBar.css';

function Logo() {
  return (
    <div className="tb-logo" aria-label="squad">
      <span className="tb-logo-grid">
        <i />
        <i />
        <i />
        <i className="hollow" />
      </span>
      <span className="tb-wordmark">squad</span>
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
  const workspaces = useSquad((s) => s.workspaces);
  const activeWs = useSquad((s) => s.activeWs);
  const index = workspaces.findIndex((w) => w.id === activeWs);
  const elsewhere = workspaces.some((w) => w.id !== activeWs && (w.active > 0 || w.pending > 0));
  useEffect(() => setWsMenu(false), [activeWs]); // switched (⌥1–9 or the list)
  const floor = useActiveFloor();
  const branch = useBranch(floor?.id ?? '');
  const cwd = floor?.cwd ?? ws?.cwd ?? '';

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
            {active} {active === 1 ? 'agente ativo' : 'agentes ativos'}
          </span>
          {elsewhere && <span className="tb-ws-elsewhere" title="Atividade em outro workspace" />}
          {index >= 0 && index < 9 && <span className="kbd">⌥{index + 1}</span>}
          <Icon name="expand_more" size={18} />
        </button>
        {wsMenu && <WorkspaceMenu onClose={() => setWsMenu(false)} />}
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
        <span className="tb-rate-label">pedidos por min</span>
      </span>
      <button className="btn" onClick={togglePause} title={extra.paused ? 'Retomar conversas (⌥P)' : 'Pausar conversas (⌥P)'}>
        <Icon name={extra.paused ? 'play_arrow' : 'pause'} />
        {extra.paused ? 'Retomar conversas' : 'Pausar conversas'}
        <span className="kbd">⌥P</span>
      </button>
      <button className={`btn${drawerOpen ? ' filled' : ''}`} onClick={() => setDrawer(!drawerOpen)} title="Atividade (⌥A)">
        <Icon name="view_timeline" />
        Atividade
        {pending > 0 && <span className="tb-count mono">{pending}</span>}
      </button>
      <span className={`tb-server ${connection === 'connected' ? 'ok' : 'wait'}`}>
        <Icon name="lan" size={18} />
        {connection === 'connected' ? 'servidor' : 'reconectando…'}
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
        Resumos…
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
