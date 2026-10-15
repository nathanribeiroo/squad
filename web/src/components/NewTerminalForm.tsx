import { useState } from 'react';
import { useReactFlow } from '@xyflow/react';
import { api } from '../api/client';
import { AGENTS, ROLE_META } from '../lib/model';
import { useNodes } from '../state/store';
import { Icon } from './Icon';

const DEFAULT_NAMES: Record<string, string> = { claude: 'Claude', codex: 'Codex', devin: 'Devin', gemini: 'Gemini', shell: 'Shell', custom: 'Terminal' };

/** Terminal names are unique per workspace: Claude, Claude 2, Claude 3… */
function uniqueName(base: string, taken: Set<string>): string {
  if (!taken.has(base)) return base;
  for (let i = 2; ; i++) if (!taken.has(`${base} ${i}`)) return `${base} ${i}`;
}

export function NewTerminalForm({ onClose }: { onClose: () => void }) {
  const rf = useReactFlow();
  const taken = useNodes();
  const [agent, setAgent] = useState('claude');
  const [name, setName] = useState('');
  const [command, setCommand] = useState('');
  const [role, setRole] = useState('');
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    setBusy(true);
    setError('');
    const names = new Set(taken.flatMap((n) => (n.terminal ? [n.terminal.name] : [])));
    const center = rf.screenToFlowPosition({ x: window.innerWidth / 2, y: window.innerHeight / 2 });
    try {
      await api.addNode({
        kind: 'terminal',
        x: Math.round(center.x - 280),
        y: Math.round(center.y - 220),
        terminal: { name: name.trim() || uniqueName(DEFAULT_NAMES[agent], names), agent, command: command.trim() || undefined, role: (role || undefined) as never },
      });
      onClose();
    } catch (err) {
      setError((err as Error).message);
    } finally {
      setBusy(false);
    }
  };

  return (
    <form className="popover new-term" style={{ left: 64, top: 64, width: 280, padding: 12 }} onSubmit={submit}>
      <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 10 }}>
        <Icon name="terminal" size={18} />
        <strong style={{ fontSize: 13 }}>Novo terminal</strong>
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
          <input autoFocus placeholder={DEFAULT_NAMES[agent]} value={name} onChange={(e) => setName(e.target.value)} />
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
        <button className="btn filled" type="submit" disabled={busy} style={{ justifyContent: 'center' }}>
          Criar
        </button>
      </div>
    </form>
  );
}
