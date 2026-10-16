import { useEffect, useMemo, useState } from 'react';
import { api, hub } from '../api/client';
import type { RoleDef } from '../api/types';
import { MAX_ROLE_NAME, MAX_ROLE_PROMPT, ROLE_AGENTS, ROLE_COLORS, ROLE_ICONS, roleIcon } from '../lib/roles';
import { toast } from '../lib/toast';
import { useNodes, useSquad } from '../state/store';
import { Icon } from './Icon';
import './RolesPanel.css';

type Draft = Omit<RoleDef, 'id'> & { id?: string };

const NEW: Draft = { name: '', prompt: '', color: 'teal', icon: 'psychology' };

function same(a: Draft, b: Draft): boolean {
  return a.name === b.name && a.prompt === b.prompt && a.color === b.color && a.icon === b.icon && !!a.manager === !!b.manager;
}

/**
 * The role library: the instructions each agent receives when it starts
 * (internal/roles). Shared by every workspace; opened from the settings
 * menu, the terminal form and a terminal's menu.
 */
export function RolesPanel() {
  const open = useSquad((s) => s.rolesPanel);
  if (open === false) return null;
  return <Panel initial={open} />;
}

function Panel({ initial }: { initial: string | null }) {
  const roles = useSquad((s) => s.roles);
  const close = useSquad((s) => s.closeRoles);
  const [selected, setSelected] = useState<string | null>(initial ?? roles[0]?.id ?? null);
  const current = roles.find((r) => r.id === selected);
  const base: Draft = current ?? NEW;
  const [draft, setDraft] = useState<Draft>(base);
  const [busy, setBusy] = useState(false);
  const [saved, setSaved] = useState(false);
  const dirty = !same(draft, base);

  // Another role chosen (or the library changed under us): edit it.
  useEffect(() => {
    setDraft(current ?? NEW);
    setSaved(false);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [selected, current?.name, current?.prompt, current?.color, current?.icon, current?.manager]);

  const leave = (then: () => void) => {
    if (!dirty || window.confirm('Descartar as alterações deste papel?')) then();
  };

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        e.stopPropagation();
        leave(close);
      }
    };
    window.addEventListener('keydown', onKey, true);
    return () => window.removeEventListener('keydown', onKey, true);
  });

  const save = async () => {
    setBusy(true);
    try {
      const r = draft.id ? await hub.updateRole({ ...(draft as RoleDef) }) : await hub.createRole(draft);
      // The list arrives through /ws/workspaces; show the saved role now.
      useSquad.getState().setRoles(upsert(useSquad.getState().roles, r));
      setSelected(r.id);
      setSaved(true);
    } catch (e) {
      toast((e as Error).message);
    } finally {
      setBusy(false);
    }
  };
  const reset = async () => {
    if (!current || !window.confirm(`Restaurar o papel "${current.name}" como o Squad o traz?`)) return;
    try {
      const r = await hub.resetRole(current.id);
      useSquad.getState().setRoles(upsert(useSquad.getState().roles, r));
      setSaved(true);
    } catch (e) {
      toast((e as Error).message);
    }
  };
  const remove = async () => {
    if (!current || !window.confirm(`Remover o papel "${current.name}"?`)) return;
    try {
      await hub.removeRole(current.id);
      const rest = useSquad.getState().roles.filter((r) => r.id !== current.id);
      useSquad.getState().setRoles(rest);
      setSelected(rest[0]?.id ?? null);
    } catch (e) {
      toast((e as Error).message);
    }
  };

  return (
    <div className="roles-backdrop" onMouseDown={(e) => e.target === e.currentTarget && leave(close)}>
      <div className="roles-panel" role="dialog" aria-label="Papéis">
        <header className="roles-head">
          <Icon name="badge" size={20} />
          <div>
            <strong>Papéis</strong>
            <span>As instruções que o agente recebe ao abrir. Valem para todos os workspaces.</span>
          </div>
          <span className="tn-spacer" />
          <button className="icon-btn" onClick={() => leave(close)} aria-label="Fechar">
            <Icon name="close" size={18} />
          </button>
        </header>
        <div className="roles-body">
          <nav className="roles-list">
            {roles.map((r) => (
              <button key={r.id} className={`roles-item${r.id === selected ? ' on' : ''}`} onClick={() => leave(() => setSelected(r.id))}>
                <span className="roles-glyph" style={{ background: `var(--rb-${r.color})`, color: `var(--r-${r.color})` }}>
                  <Icon name={roleIcon(r)} size={15} />
                </span>
                <span className="roles-name">{r.name}</span>
                {r.edited && <span className="roles-tag">editado</span>}
              </button>
            ))}
            <button className={`roles-item roles-new${selected === null ? ' on' : ''}`} onClick={() => leave(() => setSelected(null))}>
              <Icon name="add" size={18} />
              Novo papel
            </button>
          </nav>
          <Editor draft={draft} setDraft={setDraft} current={current} />
        </div>
        <footer className="roles-foot">
          {current?.edited && (
            <button className="btn" onClick={reset} title="Volta às instruções que o Squad traz">
              <Icon name="undo" />
              Restaurar padrão
            </button>
          )}
          {current && !current.builtin && (
            <button className="btn roles-danger" onClick={remove}>
              <Icon name="delete" />
              Remover
            </button>
          )}
          <Usage role={current} saved={saved && !dirty} />
          <span className="tn-spacer" />
          <button className="btn filled" disabled={busy || !dirty || !draft.name.trim()} onClick={save}>
            {current ? 'Salvar' : 'Criar papel'}
          </button>
        </footer>
      </div>
    </div>
  );
}

function upsert(list: RoleDef[], r: RoleDef): RoleDef[] {
  return list.some((x) => x.id === r.id) ? list.map((x) => (x.id === r.id ? r : x)) : [...list, r];
}

function Editor({ draft, setDraft, current }: { draft: Draft; setDraft: (d: Draft) => void; current?: RoleDef }) {
  const set = (p: Partial<Draft>) => setDraft({ ...draft, ...p });
  return (
    <div className="roles-editor">
      <label className="field">
        Nome
        <input value={draft.name} maxLength={MAX_ROLE_NAME} placeholder="Ex.: Designer" autoFocus={!current} onChange={(e) => set({ name: e.target.value })} />
      </label>
      <div className="field">
        Cor
        <div className="roles-swatches">
          {ROLE_COLORS.map((c) => (
            <button
              key={c}
              className={`roles-swatch${draft.color === c ? ' on' : ''}`}
              style={{ background: `var(--r-${c})` }}
              onClick={() => set({ color: c })}
              aria-label={`Cor ${c}`}
            />
          ))}
        </div>
      </div>
      <div className="field">
        Ícone
        <div className="roles-icons">
          {ROLE_ICONS.map((i) => (
            <button
              key={i}
              className={`roles-icon${draft.icon === i ? ' on' : ''}`}
              style={draft.icon === i ? { background: `var(--rb-${draft.color})`, color: `var(--r-${draft.color})` } : undefined}
              onClick={() => set({ icon: i })}
              aria-label={i}
            >
              <Icon name={i} size={18} />
            </button>
          ))}
        </div>
      </div>
      <label className="roles-check">
        <input type="checkbox" checked={!!draft.manager} onChange={(e) => set({ manager: e.target.checked })} />
        <span>
          <b>Gerente</b>: o agente pode criar terminais ligados a ele e ligar nós (<span className="mono">squad new</span>,{' '}
          <span className="mono">squad connect</span>). Cada terminal pode desligar isso no menu ⋯.
        </span>
      </label>
      <label className="field roles-prompt">
        <span className="field-row">
          Instruções
          <span className="mono">
            {draft.prompt.length}/{MAX_ROLE_PROMPT}
          </span>
        </span>
        <textarea
          value={draft.prompt}
          maxLength={MAX_ROLE_PROMPT}
          spellCheck={false}
          placeholder="O que este agente faz, como trabalha e com quem fala (squad ask, squad portal…)."
          onChange={(e) => set({ prompt: e.target.value })}
        />
      </label>
    </div>
  );
}

/**
 * Who uses the role in this workspace. Agents read their role when they
 * start, so after a change the running ones can be restarted from here.
 */
function Usage({ role, saved }: { role?: RoleDef; saved: boolean }) {
  const nodes = useNodes();
  const runtime = useSquad((s) => s.runtime);
  const users = useMemo(() => (role ? nodes.filter((n) => n.terminal?.role === role.id) : []), [nodes, role]);
  if (!role || users.length === 0) return null;
  const names = users.map((n) => n.terminal!.name).join(', ');
  const running = users.filter((n) => ROLE_AGENTS.has(n.terminal!.agent) && runtime[n.id]?.status === 'running');
  if (saved && running.length > 0) {
    const restart = () => {
      if (!window.confirm(`Reiniciar ${running.map((n) => n.terminal!.name).join(', ')}? A sessão atual de cada um é encerrada.`)) return;
      for (const n of running) api.nodeAction(n.id, 'restart').catch((e: Error) => toast(e.message));
    };
    return (
      <span className="roles-usage">
        Salvo. {names} {running.length > 1 ? 'recebem' : 'recebe'} as instruções novas ao reiniciar.
        <button className="link-btn" onClick={restart}>
          Reiniciar agora
        </button>
      </span>
    );
  }
  return <span className="roles-usage">Neste workspace: {names}</span>;
}
