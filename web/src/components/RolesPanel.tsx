import { useEffect, useMemo, useState } from 'react';
import { api, hub } from '../api/client';
import type { RoleDef } from '../api/types';
import { callsign } from '../lib/model';
import { MAX_ROLE_NAME, MAX_ROLE_PROMPT, MAX_ROLE_WHEN, ROLE_AGENTS, ROLE_COLORS, ROLE_ICONS, roleIcon } from '../lib/roles';
import { toast } from '../lib/toast';
import { useNodes, useSquad } from '../state/store';
import { confirmDialog, confirmOpen } from './ConfirmDialog';
import { Icon } from './Icon';
import './RolesPanel.css';

type Draft = Omit<RoleDef, 'id'> & { id?: string };

const NEW: Draft = { name: '', abbr: '', prompt: '', when: '', color: 'teal', icon: 'psychology' };

function same(a: Draft, b: Draft): boolean {
  return (
    a.name === b.name &&
    (a.abbr ?? '') === (b.abbr ?? '') &&
    a.prompt === b.prompt &&
    (a.when ?? '') === (b.when ?? '') &&
    a.color === b.color &&
    a.icon === b.icon &&
    !!a.manager === !!b.manager
  );
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
  }, [selected, current?.name, current?.abbr, current?.prompt, current?.when, current?.color, current?.icon, current?.manager]);

  const leave = (then: () => void) => {
    if (!dirty) return then();
    void confirmDialog({
      title: 'Descartar as alterações?',
      icon: 'edit_note',
      tone: 'warn',
      lose: [`O que você mudou em ${base.name || 'papel novo'} e não salvou`],
      action: 'Descartar',
    }).then((ok) => ok && then());
  };

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape' && !confirmOpen()) {
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
      toast((e as Error).message, 'err');
    } finally {
      setBusy(false);
    }
  };
  const reset = async () => {
    if (!current) return;
    const ok = await confirmDialog({
      title: `Restaurar ${current.name}?`,
      icon: 'undo',
      tone: 'warn',
      lose: [`As suas mudanças em ${current.name}: nome, sigla, cor, ícone, quando chamar e instruções voltam aos da Torre`],
      keep: ['Os voos com o papel, que recebem as instruções ao reiniciar'],
      action: 'Restaurar padrão',
    });
    if (!ok) return;
    try {
      const r = await hub.resetRole(current.id);
      useSquad.getState().setRoles(upsert(useSquad.getState().roles, r));
      setSaved(true);
    } catch (e) {
      toast((e as Error).message, 'err');
    }
  };
  const remove = async () => {
    if (!current) return;
    const ok = await confirmDialog({
      title: `Remover o papel ${current.name}?`,
      tone: 'danger',
      lose: [`${current.name} (${current.abbr}) e as instruções dele, para todos os workspaces`],
      action: 'Remover papel',
    });
    if (!ok) return;
    try {
      await hub.removeRole(current.id);
      const rest = useSquad.getState().roles.filter((r) => r.id !== current.id);
      useSquad.getState().setRoles(rest);
      setSelected(rest[0]?.id ?? null);
    } catch (e) {
      toast((e as Error).message, 'err');
    }
  };

  return (
    <div className="roles-backdrop" onMouseDown={(e) => e.target === e.currentTarget && leave(close)}>
      <div className="roles-panel" role="dialog" aria-label="Papéis">
        <header className="roles-head">
          <Icon name="badge" size={20} />
          <strong>Papéis</strong>
          <span className="roles-head-sub">As instruções que o voo recebe ao decolar. Valem para todos os workspaces.</span>
          <span className="tn-spacer" />
          <button className="icon-btn" onClick={() => leave(close)} aria-label="Fechar">
            <Icon name="close" size={18} />
          </button>
        </header>
        <div className="roles-body">
          <nav className="roles-list">
            {roles.map((r) => (
              <button key={r.id} className={`roles-item${r.id === selected ? ' on' : ''}`} onClick={() => leave(() => setSelected(r.id))}>
                <span className="roles-glyph" style={{ background: `var(--rb-${r.color})`, color: `var(--rt-${r.color})` }}>
                  <Icon name={roleIcon(r)} size={16} />
                </span>
                <span className="roles-name">{r.name}</span>
                {r.edited && <span className="roles-tag">editado</span>}
                <span className="roles-abbr mono">{r.abbr}</span>
              </button>
            ))}
            <button className={`roles-item roles-new${selected === null ? ' on' : ''}`} onClick={() => leave(() => setSelected(null))}>
              <Icon name="add" size={18} />
              Novo papel
            </button>
          </nav>
          <div className="roles-main">
            <Editor draft={draft} setDraft={setDraft} current={current} />
            <Restart role={current} saved={saved && !dirty} />
          </div>
        </div>
        <footer className="roles-foot">
          {current?.edited && (
            <button className="btn" onClick={reset} title="Volta às instruções que a Torre traz">
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
          <Usage role={current} />
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
      <div className="roles-name-row">
        <label className="field">
          Nome
          <input value={draft.name} maxLength={MAX_ROLE_NAME} placeholder="Ex.: Designer" autoFocus={!current} onChange={(e) => set({ name: e.target.value })} />
        </label>
        <label className="field" title="Dá o indicativo dos voos deste papel (IMP, IMP2…). Vazia, sai das consoantes do nome.">
          Sigla
          <input
            className="mono roles-abbr-input"
            value={draft.abbr ?? ''}
            maxLength={3}
            placeholder={draft.name.trim() ? callsign({ name: draft.name }) : 'ABC'}
            onChange={(e) => set({ abbr: e.target.value.toUpperCase().replace(/[^A-Z0-9]/g, '') })}
          />
        </label>
      </div>
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
          <b>Líder</b>: o voo pode criar voos na frequência dele e pôr nós na mesma frequência (<span className="mono">torre new</span>,{' '}
          <span className="mono">torre connect</span>). Cada voo pode desligar isso no menu ⋯.
        </span>
      </label>
      <label className="field" title="Uma linha para o líder: o torre roles mostra, e o Planejador decide por ela quando chamar este papel.">
        <span className="field-row">
          Quando chamar
          <span className="mono">
            {(draft.when ?? '').length}/{MAX_ROLE_WHEN}
          </span>
        </span>
        <input
          value={draft.when ?? ''}
          maxLength={MAX_ROLE_WHEN}
          placeholder="Ex.: Tarefas de interface: telas, componentes, estado."
          onChange={(e) => set({ when: e.target.value })}
        />
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
          placeholder="O que este agente faz, como trabalha e com quem fala (torre ask, torre portal…)."
          onChange={(e) => set({ prompt: e.target.value })}
        />
      </label>
    </div>
  );
}

/** Who uses the role in this workspace, by callsign. */
function Usage({ role }: { role?: RoleDef }) {
  const users = useUsers(role);
  if (!role || users.length === 0) return null;
  return <span className="roles-usage mono">neste workspace: {users.map((n) => callsign(n.terminal!)).join(', ')}</span>;
}

function useUsers(role?: RoleDef) {
  const nodes = useNodes();
  return useMemo(() => (role ? nodes.filter((n) => n.terminal?.role === role.id) : []), [nodes, role]);
}

/**
 * After a save: agents read their role when they start, so the running ones
 * can be restarted from here.
 */
function Restart({ role, saved }: { role?: RoleDef; saved: boolean }) {
  const users = useUsers(role);
  const runtime = useSquad((s) => s.runtime);
  const running = users.filter((n) => ROLE_AGENTS.has(n.terminal!.agent) && runtime[n.id]?.status === 'running');
  if (!role || !saved || running.length === 0) return null;
  const names = running.map((n) => callsign(n.terminal!)).join(', ');
  const restart = async () => {
    const ok = await confirmDialog({
      title: `Reiniciar ${names}?`,
      tone: 'info',
      text: running.length > 1 ? 'Cada um para e decola de novo, já com as instruções novas.' : 'O voo para e decola de novo, já com as instruções novas.',
      keep: [running.length > 1 ? 'A conversa: cada voo volta para a dele' : 'A conversa: o voo volta para ela'],
      action: 'Reiniciar',
    });
    if (!ok) return;
    for (const n of running) api.nodeAction(n.id, 'restart').catch((e: Error) => toast(e.message, 'err'));
  };
  return (
    <div className="roles-saved">
      <Icon name="check" size={16} />
      <span>
        Salvo. <b className="mono">{names}</b> {running.length > 1 ? 'recebem' : 'recebe'} as instruções novas ao reiniciar.
      </span>
      <span className="tn-spacer" />
      <button className="btn" onClick={restart}>
        Reiniciar agora
      </button>
    </div>
  );
}
