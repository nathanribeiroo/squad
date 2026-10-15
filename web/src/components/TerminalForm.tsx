import { useEffect, useState } from 'react';
import { useReactFlow } from '@xyflow/react';
import { api } from '../api/client';
import type { Role, ShellOption, WNode } from '../api/types';
import { AGENTS, ROLE_META } from '../lib/model';
import { shortPath, trimDir } from '../lib/paths';
import { useNodes, useSquad } from '../state/store';
import { DirInput } from './DirInput';
import { Icon } from './Icon';

const DEFAULT_NAMES: Record<string, string> = { claude: 'Claude', codex: 'Codex', devin: 'Devin', gemini: 'Gemini', shell: 'Shell', custom: 'Terminal' };

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
  const wsCwd = useSquad((s) => s.workspace?.cwd ?? '');
  const shells = useShells();
  const t = node?.terminal;
  const [agent, setAgent] = useState(t?.agent ?? 'claude');
  const [name, setName] = useState(t?.name ?? '');
  const [command, setCommand] = useState(t?.command ?? '');
  const [role, setRole] = useState<string>(t?.role ?? '');
  const [cwd, setCwd] = useState(t?.cwd ?? '');
  const [shell, setShell] = useState(t?.shell ?? '');
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);

  const relaunch =
    !!t && (agent !== t.agent || command.trim() !== (t.command ?? '') || trimDir(cwd) !== (t.cwd ?? '') || shell !== (t.shell ?? ''));

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    setBusy(true);
    setError('');
    const fields = {
      agent,
      command: command.trim() || undefined,
      role: (role || undefined) as Role | undefined,
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
          {t ? `Configurar ${t.name}` : 'Novo terminal'}
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
          <DirInput value={cwd} onChange={setCwd} placeholder={shortPath(wsCwd) || 'pasta do workspace'} />
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
        <label className="field">
          Papel
          <select value={role} onChange={(e) => setRole(e.target.value)}>
            <option value="">Nenhum</option>
            {Object.entries(ROLE_META).map(([k, v]) => (
              <option key={k} value={k}>
                {v.label}
              </option>
            ))}
          </select>
        </label>
        {error && <div style={{ color: 'var(--st-err)', fontSize: 12 }}>{error}</div>}
        {relaunch && <div style={{ color: 'var(--text-2)', fontSize: 11.5 }}>O terminal reinicia e a sessão atual é encerrada.</div>}
        <button className="btn filled" type="submit" disabled={busy} style={{ justifyContent: 'center' }}>
          {t ? (relaunch ? 'Salvar e reiniciar' : 'Salvar') : 'Criar'}
        </button>
      </div>
    </form>
  );
}
