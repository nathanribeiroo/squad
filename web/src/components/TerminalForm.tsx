import { useCallback, useEffect, useState } from 'react';
import { useReactFlow } from '@xyflow/react';
import { api, hub } from '../api/client';
import type { AgentInfo, ShellOption, WNode } from '../api/types';
import { flightLevel, useBranch, workdir } from '../lib/floors';
import { AGENTS, callsign } from '../lib/model';
import { ROLE_AGENTS } from '../lib/roles';
import { shortPath, trimDir } from '../lib/paths';
import { useNodes, useSquad } from '../state/store';
import { DirInput } from './DirInput';
import { Icon } from './Icon';
import './TerminalForm.css';

const DEFAULT_NAMES: Record<string, string> = { claude: 'Claude', codex: 'Codex', devin: 'Devin', gemini: 'Gemini', shell: 'Shell', custom: 'Comando' };
const SHORT_LABELS: Record<string, string> = { custom: 'Comando' };

/** Terminal names are unique per workspace: Claude, Claude 2, Claude 3… */
function uniqueName(base: string, taken: Set<string>): string {
  if (!taken.has(base)) return base;
  for (let i = 2; ; i++) if (!taken.has(`${base} ${i}`)) return `${base} ${i}`;
}

// The agents found on this computer (GET /api/agents), once per page.
let agentsReq: Promise<AgentInfo[]> | null = null;
function useAgents(): AgentInfo[] | null {
  const [agents, setAgents] = useState<AgentInfo[] | null>(null);
  useEffect(() => {
    agentsReq ??= hub.agents().then(
      (d) => d.agents,
      () => {
        agentsReq = null;
        return [];
      },
    );
    let live = true;
    void agentsReq.then((a) => live && setAgents(a));
    return () => {
      live = false;
    };
  }, []);
  return agents;
}

// The installed shells don't change while the page is open.
let shellsReq: Promise<ShellOption[]> | null = null;
function useShells(): ShellOption[] {
  const [shells, setShells] = useState<ShellOption[]>([]);
  useEffect(() => {
    shellsReq ??= api.shells().then(
      (r) => r.shells,
      () => {
        shellsReq = null;
        return [];
      },
    );
    let live = true;
    void shellsReq.then((s) => live && setShells(s));
    return () => {
      live = false;
    };
  }, []);
  return shells;
}

function baseName(p: string): string {
  return p.split(/[\\/]/).pop() ?? p;
}

/**
 * Novo voo (docs/torre/TELAS.md, quadro 16), or with `node` "Configurar…".
 * Changing what launches it (agent, command, folder, shell) restarts it.
 */
export function TerminalForm({ node, onClose }: { node?: WNode; onClose: () => void }) {
  const rf = useReactFlow();
  const taken = useNodes();
  // A new terminal goes on the floor on screen; its folder is relative to
  // that floor's copy of the workspace folder.
  const activeFloor = useSquad((s) => s.activeFloor);
  const floor = node ? (node.floor ?? '') : activeFloor;
  const floorDef = useSquad((s) => (floor ? s.workspace?.floors?.find((f) => f.id === floor) : undefined));
  const branch = useBranch(floor);
  const wsCwd = useSquad((s) => workdir(s.workspace, floor) ?? '');
  const suggestDirs = useCallback((p: string) => api.dirs(p, floor), [floor]);
  const shells = useShells();
  const found = useAgents();
  const t = node?.terminal;
  const [agent, setAgent] = useState(t?.agent ?? 'claude');
  const [name, setName] = useState(t?.name ?? '');
  const [cs, setCs] = useState(t?.callsign ?? '');
  const [command, setCommand] = useState(t?.command ?? '');
  const [role, setRole] = useState<string>(t?.role ?? '');
  const roles = useSquad((s) => s.roles);
  const hasRole = agent !== 'shell'; // shells have no role
  const [cwd, setCwd] = useState(t?.cwd ?? '');
  const [shell, setShell] = useState(t?.shell ?? '');
  const [manager, setManager] = useState<boolean | undefined>(t?.manager);
  const [advanced, setAdvanced] = useState(!!(t?.command || t?.shell || t?.manager !== undefined) || agent === 'custom');
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);

  const newRole = hasRole ? role : '';
  const roleDef = roles.find((r) => r.id === newRole);
  const relaunch =
    !!t &&
    (agent !== t.agent ||
      command.trim() !== (t.command ?? '') ||
      trimDir(cwd) !== (t.cwd ?? '') ||
      shell !== (t.shell ?? '') ||
      // the agent gets its role's instructions when it starts
      (ROLE_AGENTS.has(agent) && newRole !== (t.role ?? '')));
  const names = new Set(taken.flatMap((n) => (n.terminal && n.id !== node?.id ? [n.terminal.name] : [])));
  const shownName = name.trim() || t?.name || uniqueName(DEFAULT_NAMES[agent], names);
  // What the server gives when the field is left empty: the role's
  // abbreviation, else the name's consonants (numbered when taken).
  const autoCs = t?.callsign || roleDef?.abbr || callsign({ name: shownName });

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    setBusy(true);
    setError('');
    const fields = {
      agent,
      command: command.trim() || undefined,
      role: newRole || undefined,
      cwd: trimDir(cwd) || undefined,
      shell: shell || undefined,
      manager,
    };
    try {
      if (node && t) {
        await api.updateNode(node.id, { terminal: { ...t, ...fields, name: name.trim() || t.name, callsign: cs || t.callsign } });
        if (relaunch) await api.nodeAction(node.id, 'restart');
      } else {
        const center = rf.screenToFlowPosition({ x: window.innerWidth / 2, y: window.innerHeight / 2 });
        await api.addNode({
          kind: 'terminal',
          x: Math.round(center.x - 280),
          y: Math.round(center.y - 220),
          terminal: { ...fields, name: shownName, callsign: cs || undefined },
          floor: floor || undefined,
        });
      }
      onClose();
    } catch (err) {
      setError((err as Error).message);
    } finally {
      setBusy(false);
    }
  };

  const def = shells.find((s) => s.default);
  const others = shells.filter((s) => !s.default);
  const dupLabel = (s: ShellOption) => shells.filter((o) => o.label === s.label).length > 1;
  // Under each agent: installed or not (the CLIs the summaries look for).
  const status = (a: string): { text: string; cls: string } | null => {
    if (a === 'shell') return { text: def?.label ?? 'shell', cls: '' };
    if (a === 'custom') return { text: 'próprio', cls: '' };
    const e = found?.find((x) => x.agent === a);
    if (!e) return null;
    return e.found ? { text: '✓ instalado', cls: 'ok' } : { text: '✕ não achado', cls: 'err' };
  };

  return (
    <form className="popover new-term" onSubmit={submit} aria-label={t ? `Configurar ${t.name}` : 'Novo voo'}>
      <header className="nt-head">
        <Icon name={t ? 'tune' : 'flight_takeoff'} size={18} />
        <strong>{t ? 'Configurar' : 'Novo voo'}</strong>
        <span className="nt-where mono">
          {t ? callsign(t) : `${floor ? `em ${flightLevel(floorDef)}` : 'na pista'}${branch ? ` · ${branch}` : ''}`}
        </span>
        <span style={{ flex: 1 }} />
        <button type="button" className="icon-btn" onClick={onClose} aria-label="Fechar">
          <Icon name="close" size={18} />
        </button>
      </header>
      <div className="nt-body">
        <div className="field">
          Agente
          <div className="nt-agents" role="radiogroup" aria-label="Agente">
            {AGENTS.map((a) => {
              const st = status(a.value);
              return (
                <button
                  key={a.value}
                  type="button"
                  role="radio"
                  aria-checked={agent === a.value}
                  className={`nt-agent${agent === a.value ? ' on' : ''}`}
                  onClick={() => {
                    setAgent(a.value);
                    if (a.value === 'custom') setAdvanced(true);
                  }}
                >
                  <span className="nt-agent-name">{SHORT_LABELS[a.value] ?? a.label}</span>
                  {st && <span className={`nt-agent-st mono ${st.cls}`}>{st.text}</span>}
                </button>
              );
            })}
          </div>
        </div>
        <div className="nt-row">
          <label className="field">
            Nome
            <input autoFocus placeholder={t?.name ?? shownName} value={name} onChange={(e) => setName(e.target.value)} />
          </label>
          <label className="field" title="Como o voo aparece no Rádio, nos fios e para os outros voos. Vazio, vem da sigla do papel ou do nome.">
            Indicativo
            <input
              className="mono nt-cs"
              placeholder={autoCs}
              value={cs}
              maxLength={4}
              onChange={(e) => setCs(e.target.value.toUpperCase().replace(/[^A-Z0-9]/g, ''))}
            />
          </label>
        </div>
        {hasRole && (
          <label className="field">
            <span className="field-row">
              Papel
              <button type="button" className="link-btn" onClick={() => useSquad.getState().openRoles(role || null)}>
                editar papéis…
              </button>
            </span>
            <span className="nt-select">
              {roleDef && <i style={{ background: `var(--r-${roleDef.color})` }} />}
              <select value={role} onChange={(e) => setRole(e.target.value)}>
                <option value="">Nenhum</option>
                {roles.map((r) => (
                  <option key={r.id} value={r.id}>
                    {r.abbr ? `${r.abbr} · ${r.name}` : r.name}
                  </option>
                ))}
              </select>
            </span>
            {role && !ROLE_AGENTS.has(agent) && (
              <span className="field-hint">Comandos próprios não recebem as instruções do papel: ele fica só como etiqueta.</span>
            )}
          </label>
        )}
        <label className="field">
          Pasta (opcional)
          <DirInput value={cwd} onChange={setCwd} placeholder={shortPath(wsCwd) || 'pasta do workspace'} suggest={suggestDirs} />
        </label>
        <button type="button" className={`nt-adv${advanced ? ' open' : ''}`} onClick={() => setAdvanced(!advanced)} aria-expanded={advanced}>
          <Icon name="chevron_right" size={16} />
          Avançado
        </button>
        {advanced && (
          <div className="nt-adv-body">
            <label className="field">
              Comando {agent !== 'custom' && '(opcional)'}
              <input
                className="mono"
                placeholder={agent === 'shell' ? 'só o shell' : agent === 'custom' ? 'ex.: npm run dev' : agent}
                value={command}
                onChange={(e) => setCommand(e.target.value)}
              />
            </label>
            <label className="field">
              Shell
              <select value={shell} onChange={(e) => setShell(e.target.value)}>
                <option value="">Padrão{def ? ` (${def.label})` : ''}</option>
                {others.map((s) => (
                  <option key={s.path} value={s.path} title={s.path}>
                    {dupLabel(s) ? `${s.label} · ${s.path}` : s.label}
                  </option>
                ))}
                {shell && !shells.some((s) => s.path === shell) && <option value={shell}>{baseName(shell)}</option>}
              </select>
            </label>
            {ROLE_AGENTS.has(agent) && (
              <label className="nt-check">
                <input type="checkbox" checked={manager ?? !!roleDef?.manager} onChange={(e) => setManager(e.target.checked)} />
                <span>
                  <b>Líder</b>: cria voos e liga nós na frequência dele (<span className="mono">torre new</span>).
                  {manager === undefined && roleDef && ' Segue o papel.'}
                </span>
              </label>
            )}
          </div>
        )}
        {error && <div className="nt-error">{error}</div>}
        {relaunch && (
          <div className="nt-relaunch">
            <Icon name="warning" size={16} />O voo reinicia, e a conversa atual é encerrada.
          </div>
        )}
      </div>
      <footer className="nt-foot">
        <button type="button" className="btn" onClick={onClose}>
          Cancelar
        </button>
        <button className="btn filled" type="submit" disabled={busy || (agent === 'custom' && !command.trim())}>
          {!t && <Icon name="flight_takeoff" size={16} />}
          {t ? (relaunch ? 'Salvar e reiniciar' : 'Salvar') : 'Decolar'}
        </button>
      </footer>
    </form>
  );
}
