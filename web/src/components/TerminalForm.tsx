import { useCallback, useEffect, useState } from 'react';
import { useReactFlow } from '@xyflow/react';
import { api } from '../api/client';
import type { ShellOption, WNode } from '../api/types';
import { workdir } from '../lib/floors';
import { AGENTS } from '../lib/model';
import { ROLE_AGENTS } from '../lib/roles';
import { shortPath, trimDir } from '../lib/paths';
import { useNodes, useSquad } from '../state/store';
import { DirInput } from './DirInput';
import { Icon } from './Icon';

const DEFAULT_NAMES: Record<string, string> = { claude: 'Claude', codex: 'Codex', devin: 'Devin', gemini: 'Gemini', shell: 'Shell', custom: 'Comando' };

/** Terminal names are unique per workspace: Claude, Claude 2, Claude 3… */
function uniqueName(base: string, taken: Set<string>): string {
  if (!taken.has(base)) return base;
  for (let i = 2; ; i++) if (!taken.has(`${base} ${i}`)) return `${base} ${i}`;
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
 * Creates a terminal, or with `node` edits one ("Configurar…"). Changing what
 * launches it (agent, command, folder, shell) restarts the terminal.
 */
export function TerminalForm({ node, onClose }: { node?: WNode; onClose: () => void }) {
  const rf = useReactFlow();
  const taken = useNodes();
  // A new terminal goes on the floor on screen; its folder is relative to
  // that floor's copy of the workspace folder.
  const activeFloor = useSquad((s) => s.activeFloor);
  const floor = node ? (node.floor ?? '') : activeFloor;
  const wsCwd = useSquad((s) => workdir(s.workspace, floor) ?? '');
  const suggestDirs = useCallback((p: string) => api.dirs(p, floor), [floor]);
  const shells = useShells();
  const t = node?.terminal;
  const [agent, setAgent] = useState(t?.agent ?? 'claude');
  const [name, setName] = useState(t?.name ?? '');
  const [command, setCommand] = useState(t?.command ?? '');
  const [role, setRole] = useState<string>(t?.role ?? '');
  const roles = useSquad((s) => s.roles);
  const hasRole = agent !== 'shell'; // shells have no role
  const [cwd, setCwd] = useState(t?.cwd ?? '');
  const [shell, setShell] = useState(t?.shell ?? '');
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);

  const newRole = hasRole ? role : '';
  const relaunch =
    !!t &&
    (agent !== t.agent ||
      command.trim() !== (t.command ?? '') ||
      trimDir(cwd) !== (t.cwd ?? '') ||
      shell !== (t.shell ?? '') ||
      // the agent gets its role's instructions when it starts
      (ROLE_AGENTS.has(agent) && newRole !== (t.role ?? '')));

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
    };
    try {
      if (node && t) {
        await api.updateNode(node.id, { terminal: { ...t, ...fields, name: name.trim() || t.name } });
        if (relaunch) await api.nodeAction(node.id, 'restart');
      } else {
        const names = new Set(taken.flatMap((n) => (n.terminal ? [n.terminal.name] : [])));
        const center = rf.screenToFlowPosition({ x: window.innerWidth / 2, y: window.innerHeight / 2 });
        await api.addNode({
          kind: 'terminal',
          x: Math.round(center.x - 280),
          y: Math.round(center.y - 220),
          terminal: { ...fields, name: name.trim() || uniqueName(DEFAULT_NAMES[agent], names) },
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

  return (
    <form className="popover new-term" style={{ left: 64, top: 64, width: 300, padding: 12 }} onSubmit={submit}>
      <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 10 }}>
        <Icon name={t ? 'tune' : 'terminal'} size={18} />
        <strong style={{ fontSize: 13, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
          {t ? `Configurar ${t.name}` : 'Novo voo'}
        </strong>
        <span style={{ flex: 1 }} />
        <button type="button" className="icon-btn" onClick={onClose} aria-label="Fechar">
          <Icon name="close" size={18} />
        </button>
      </div>
      <div style={{ display: 'grid', gap: 8 }}>
        <label className="field">
          Agente
          <select value={agent} onChange={(e) => setAgent(e.target.value)}>
            {AGENTS.map((a) => (
              <option key={a.value} value={a.value}>
                {a.label}
              </option>
            ))}
          </select>
        </label>
        <label className="field">
          Nome
          <input autoFocus placeholder={t?.name ?? DEFAULT_NAMES[agent]} value={name} onChange={(e) => setName(e.target.value)} />
        </label>
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
          Pasta (opcional)
          <DirInput value={cwd} onChange={setCwd} placeholder={shortPath(wsCwd) || 'pasta do workspace'} suggest={suggestDirs} />
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
        {hasRole && (
          <label className="field">
            <span className="field-row">
              Papel
              <button type="button" className="link-btn" onClick={() => useSquad.getState().openRoles(role || null)}>
                Editar papéis…
              </button>
            </span>
            <select value={role} onChange={(e) => setRole(e.target.value)}>
              <option value="">Nenhum</option>
              {roles.map((r) => (
                <option key={r.id} value={r.id}>
                  {r.name}
                </option>
              ))}
            </select>
            {role && !ROLE_AGENTS.has(agent) && (
              <span className="field-hint">Comandos próprios não recebem as instruções do papel: ele fica só como etiqueta.</span>
            )}
          </label>
        )}
        {error && <div style={{ color: 'var(--stt-err)', fontSize: 12 }}>{error}</div>}
        {relaunch && <div style={{ color: 'var(--text-2)', fontSize: 11.5 }}>O voo reinicia, e a conversa atual é encerrada.</div>}
        <button className="btn filled" type="submit" disabled={busy} style={{ justifyContent: 'center' }}>
          {t ? (relaunch ? 'Salvar e reiniciar' : 'Salvar') : 'Criar'}
        </button>
      </div>
    </form>
  );
}
