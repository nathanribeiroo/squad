import { useEffect, useMemo, useRef, useState } from 'react';
import { ApiError, api, hub } from '../api/client';
import type { SessionList, SessionSummary, WNode } from '../api/types';
import { slugify } from '../lib/floors';
import { shortPath } from '../lib/paths';
import { ago, busyOf, busyText, contents, newBranchName, pendingWork, when, type Busy } from '../lib/sessions';
import { useNow } from '../lib/useNow';
import { useSquad } from '../state/store';
import { Icon } from './Icon';
import './WorkspaceMenu.css';
import './SessionsMenu.css';

function setSessions(l: SessionList) {
  useSquad.getState().setSessions(l);
}

/** Leaving the open session (open another, or a new one): run(fn) calls
 * fn(false); when agents are working, the server refuses and busy asks
 * first, and confirming calls fn(true). */
export function useLeave() {
  const [busy, setBusy] = useState<{ why: Busy; target: string; confirm: () => void } | null>(null);
  const [error, setError] = useState('');
  const [running, setRunning] = useState(false);
  const run = (target: string, fn: (confirm: boolean) => Promise<void>, confirm = false) => {
    setRunning(true);
    setError('');
    setBusy(null);
    fn(confirm)
      .catch((err: Error) => {
        const why = busyOf(err);
        if (why && !confirm) setBusy({ why, target, confirm: () => run(target, fn, true) });
        else setError(err.message);
      })
      .finally(() => setRunning(false));
  };
  return { run, busy, cancel: () => setBusy(null), error, running };
}

/** The confirmation to leave a session whose agents are working. */
export function BusyConfirm({ why, onConfirm, onCancel }: { why: Busy; onConfirm: () => void; onCancel: () => void }) {
  return (
    <div className="ses-busy" role="alert">
      <Icon name="warning" size={18} />
      <div className="ses-busy-text">
        <span>{busyText(why)}</span>
        <span className="ses-busy-sub">
          {why.working.length === 0
            ? 'Trocando agora, os pedidos são cancelados.'
            : why.working.length === 1
              ? 'Trocando agora, o trabalho para no meio. Quando você voltar para esta sessão, o voo aparece em arremetida, com "Continuar".'
              : 'Trocando agora, o trabalho para no meio. Quando você voltar para esta sessão, os voos aparecem em arremetida, com "Continuar".'}
        </span>
        <span className="ses-busy-actions">
          <button type="button" className="btn" onClick={onCancel}>
            Cancelar
          </button>
          <button type="button" className="btn filled" onClick={onConfirm}>
            Trocar mesmo assim
          </button>
        </span>
      </div>
    </div>
  );
}

/** The sessions of the workspace on screen, under the session in the top bar. */
export function SessionsMenu({ onClose }: { onClose: () => void }) {
  const ws = useSquad((s) => s.activeWs) ?? '';
  const list = useSquad((s) => (s.activeWs ? s.sessionLists[s.activeWs] : undefined));
  const wsName = useSquad((s) => s.workspaces.find((w) => w.id === s.activeWs)?.name ?? '');
  const [creating, setCreating] = useState(false);
  const [error, setError] = useState('');
  const leave = useLeave();
  const now = useNow(30_000);
  const ref = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (ws) hub.sessions(ws).then(setSessions, (err: Error) => setError(err.message));
  }, [ws]);

  useEffect(() => {
    const away = (e: PointerEvent) => {
      // The top bar button toggles the menu itself.
      if (!ref.current?.contains(e.target as Node) && !(e.target as Element).closest?.('.tb-session')) onClose();
    };
    const esc = (e: KeyboardEvent) => e.key === 'Escape' && onClose();
    window.addEventListener('pointerdown', away, true);
    window.addEventListener('keydown', esc);
    return () => {
      window.removeEventListener('pointerdown', away, true);
      window.removeEventListener('keydown', esc);
    };
  }, [onClose]);

  const open = (s: SessionSummary) =>
    leave.run(s.id, async (confirm) => {
      setSessions(await hub.openSession(ws, s.id, confirm));
      onClose();
    });

  return (
    <div className="popover ses-menu" ref={ref} role="dialog" aria-label="Sessões">
      {creating ? (
        <NewSessionForm ws={ws} onBack={() => setCreating(false)} onDone={onClose} />
      ) : (
        <>
          <div className="ws-menu-title">Sessões de {wsName}</div>
          <div className="ses-list">
            {list?.sessions.map((s) => (
              <div key={s.id}>
                <SessionRow
                  ws={ws}
                  s={s}
                  now={now}
                  disabled={leave.running}
                  onOpen={() => open(s)}
                  onMerge={() => {
                    onClose();
                    useSquad.getState().setFloorDialog({ kind: 'merge-copy' });
                  }}
                  onError={setError}
                />
                {leave.busy?.target === s.id && <BusyConfirm why={leave.busy.why} onConfirm={leave.busy.confirm} onCancel={leave.cancel} />}
              </div>
            ))}
            {!list && !error && <div className="ses-empty">Carregando…</div>}
          </div>
          {(error || leave.error) && <div className="ws-error ses-error">{error || leave.error}</div>}
          <span className="ws-menu-div" />
          <button className="ws-new" onClick={() => setCreating(true)}>
            <Icon name="add" size={18} />
            Nova sessão
          </button>
        </>
      )}
    </div>
  );
}

function SessionRow({
  ws,
  s,
  now,
  disabled,
  onOpen,
  onMerge,
  onError,
}: {
  ws: string;
  s: SessionSummary;
  now: number;
  disabled: boolean;
  onOpen: () => void;
  onMerge: () => void;
  onError: (msg: string) => void;
}) {
  const [editing, setEditing] = useState(false);
  const [name, setName] = useState(s.name);
  // Uncommitted changes in its copy that removing it would lose.
  const [losing, setLosing] = useState<string[] | null>(null);
  const cancelled = useRef(false);
  useEffect(() => {
    if (!editing) setName(s.name); // named meanwhile (by the agent, or elsewhere)
  }, [s.name, editing]);

  const save = () => {
    setEditing(false);
    const n = name.trim();
    if (cancelled.current || !n || n === s.name) return setName(s.name);
    hub.renameSession(ws, s.id, n).then(setSessions, (err: Error) => {
      setName(s.name);
      onError(err.message);
    });
  };
  const remove = (confirm = false) => {
    setLosing(null);
    hub.removeSession(ws, s.id, confirm).then(setSessions, (err: Error) => {
      const files = err instanceof ApiError && err.status === 409 ? (err.body.files as string[] | undefined) : undefined;
      if (files) setLosing(files);
      else onError(err.message);
    });
  };
  const askRemove = () => {
    const copy = s.copy
      ? ` A cópia ${shortPath(s.copy)} é apagada; o branch ${s.branch} continua no repositório, com o que foi commitado.`
      : '';
    const msg = `Remover a sessão "${s.name}"?\n\nO canvas e as NOTAMs dela vão para a lixeira da Torre (~/.torre/trash).${copy} As conversas continuam guardadas nos agentes.`;
    if (window.confirm(msg)) remove();
  };

  return (
    <>
      <div className={`ses-row${s.current ? ' on' : ''}`}>
        <div className="ses-row-text">
          <span className="ses-row-head">
            {editing ? (
              <input
                className="ses-rename"
                autoFocus
                value={name}
                maxLength={60}
                aria-label="Nome da sessão"
                onChange={(e) => setName(e.target.value)}
                onFocus={(e) => {
                  cancelled.current = false;
                  e.target.select();
                }}
                onBlur={save}
                onKeyDown={(e) => {
                  e.stopPropagation();
                  if (e.key === 'Escape') cancelled.current = true;
                  if (e.key === 'Enter' || e.key === 'Escape') e.currentTarget.blur();
                }}
              />
            ) : (
              <span className={`ses-row-name${s.named ? '' : ' provisional'}`} title={s.named ? s.name : `${s.name} (o voo líder dá o nome no fim do primeiro turno)`}>
                {s.name}
              </span>
            )}
            {s.current && <span className="ses-badge">atual</span>}
          </span>
          <span className="ses-row-sub">
            criada {when(s.createdAt)} · {s.current ? 'aberta agora' : `última abertura ${ago(s.openedAt, now)}`}
          </span>
          <span className="ses-row-sub">
            {s.branch && (
              <span className="ses-branch mono" title={`Trabalha numa cópia do repositório: ${s.copy}`}>
                ⎇ {s.branch}
              </span>
            )}
            <span>{contents(s)}</span>
            {s.agents.map((a) => (
              <span key={a} className="ses-agent mono">
                {a}
              </span>
            ))}
          </span>
        </div>
        <span className="ses-row-actions">
          {!editing && (
            <button className="icon-btn ses-hover" onClick={() => setEditing(true)} aria-label={`Renomear ${s.name}`} title="Renomear">
              <Icon name="edit" size={16} />
            </button>
          )}
          {s.current && s.branch && (
            <button className="btn ses-open" onClick={onMerge} title={`Pousar o branch ${s.branch}; a sessão continua na cópia`}>
              <Icon name="call_merge" size={16} />
              Pedir pouso…
            </button>
          )}
          {!s.current && (
            <button
              className="icon-btn ses-hover ses-danger"
              onClick={askRemove}
              disabled={s.floors > 0}
              aria-label={`Remover ${s.name}`}
              title={s.floors > 0 ? 'Tem níveis de voo: abra a sessão e pouse ou descarte os níveis antes' : 'Remover'}
            >
              <Icon name="delete" size={16} />
            </button>
          )}
          {!s.current && (
            <button className="btn ses-open" onClick={onOpen} disabled={disabled}>
              Abrir
            </button>
          )}
        </span>
      </div>
      {losing && (
        <div className="ses-busy" role="alert">
          <Icon name="warning" size={18} />
          <div className="ses-busy-text">
            <span>
              A cópia tem {losing.length === 1 ? '1 alteração sem commit, que se perde' : `${losing.length} alterações sem commit, que se perdem`}:{' '}
              <span className="mono">{losing.slice(0, 5).join(', ')}{losing.length > 5 ? ` e mais ${losing.length - 5}` : ''}</span>.
            </span>
            <span className="ses-busy-sub">Para {losing.length === 1 ? 'guardá-la' : 'guardá-las'}, abra a sessão e peça a um voo para commitar.</span>
            <span className="ses-busy-actions">
              <button type="button" className="btn" onClick={() => setLosing(null)}>
                Cancelar
              </button>
              <button type="button" className="btn filled" onClick={() => remove(true)}>
                Remover mesmo assim
              </button>
            </span>
          </div>
        </div>
      )}
    </>
  );
}

const NO_NODES: WNode[] = [];

function NewSessionForm({ ws, onBack, onDone }: { ws: string; onBack: () => void; onDone: () => void }) {
  const nodes = useSquad((s) => s.workspace?.nodes ?? NO_NODES);
  const runtime = useSquad((s) => s.runtime);
  const status = useSquad((s) => s.floorStatus);
  const project = useSquad((s) => s.workspace?.cwd ?? '');
  const inCopy = useSquad((s) => !!s.workspace?.copy);
  const current = useSquad((s) => s.sessionLists[ws]?.sessions.find((x) => x.current)?.name ?? 'A sessão atual');
  // The terminals a new session can start with: the ground floor's.
  const terminals = nodes.filter((n) => n.terminal && !n.floor).length;
  // Work in progress in the project folder: a copy keeps the new session out of it.
  const working = useMemo(
    () =>
      nodes
        .filter((n) => n.terminal && !['shell', 'custom'].includes(n.terminal.agent) && ['work', 'wait'].includes(runtime[n.id]?.activity ?? ''))
        .map((n) => n.terminal!.name),
    [nodes, runtime],
  );
  const changes = !inCopy && status?.repo ? status.ground.changes : 0;
  const pending = !inCopy && (working.length > 0 || changes > 0);
  const repo = !!status?.repo;

  const [name, setName] = useState('');
  const [keep, setKeep] = useState(false);
  const [where, setWhere] = useState<'folder' | 'copy'>(() => (pending ? 'copy' : 'folder'));
  const [branches, setBranches] = useState<string[]>([]);
  const [from, setFrom] = useState(status?.ground.branch ?? '');
  const [branch, setBranch] = useState('');
  const [branchEdited, setBranchEdited] = useState(false);
  const leave = useLeave();
  useEffect(() => {
    if (!repo) return;
    api.branches().then(
      (r) => setBranches(r.branches),
      () => setBranches([]),
    );
  }, [repo]);

  const auto = useMemo(() => newBranchName(slugify(name), branches), [name, branches]);
  const target = branchEdited ? branch.trim() : auto;
  const taken = where === 'copy' && branches.includes(target);

  const submit = (e: React.FormEvent) => {
    e.preventDefault();
    const copy = where === 'copy' ? { branch: target, from } : undefined;
    leave.run('new', async (confirm) => {
      setSessions(await hub.newSession(ws, { name: name.trim() || undefined, keepTerminals: keep, confirm, copy }));
      onDone();
    });
  };

  return (
    <form className="ws-form" onSubmit={submit}>
      <div className="ws-form-head">
        <button type="button" className="icon-btn" onClick={onBack} aria-label="Voltar">
          <Icon name="arrow_back" size={18} />
        </button>
        <strong>Nova sessão</strong>
      </div>
      <label className="field">
        Nome (opcional)
        <input autoFocus value={name} maxLength={60} placeholder="sem nome, o voo líder sugere um" onChange={(e) => setName(e.target.value)} />
      </label>
      {terminals > 0 && (
        <label className="ses-check">
          <input type="checkbox" checked={keep} onChange={(e) => setKeep(e.target.checked)} />
          <span>
            <b>Começar com os mesmos voos, sem as conversas</b>: {terminals === 1 ? 'o voo' : `os ${terminals} voos`} da pista, com papéis e frequências.
            Cada voo começa uma conversa nova.
          </span>
        </label>
      )}
      {repo && (
        <div className="field">
          Onde a sessão trabalha
          <div className="ses-where" role="radiogroup" aria-label="Onde a sessão trabalha">
            <label className={`ses-where-opt${where === 'folder' ? ' on' : ''}`}>
              <input type="radio" name="where" checked={where === 'folder'} onChange={() => setWhere('folder')} />
              <span>
                <b>Na pasta do projeto</b>
                <span className="ses-where-sub">
                  <span className="mono">{shortPath(project)}</span>, a mesma das outras sessões.
                </span>
              </span>
            </label>
            <label className={`ses-where-opt${where === 'copy' ? ' on' : ''}`}>
              <input type="radio" name="where" checked={where === 'copy'} onChange={() => setWhere('copy')} />
              <span>
                <b>Numa cópia isolada</b>
                <span className="ses-where-sub">Um branch novo, numa cópia do repositório. O que está sem commit fica de fora.</span>
              </span>
            </label>
          </div>
        </div>
      )}
      {repo && where === 'copy' && (
        <div className="ses-copy-fields">
          <label className="field">
            A partir de
            <select className="mono" value={from} onChange={(e) => setFrom(e.target.value)}>
              {!branches.includes(from) && <option value={from}>{from || 'o commit atual'}</option>}
              {branches.map((b) => (
                <option key={b} value={b}>
                  {b}
                </option>
              ))}
            </select>
          </label>
          <label className="field">
            Branch novo
            <input
              className="mono"
              value={target}
              onChange={(e) => {
                setBranchEdited(true);
                setBranch(e.target.value);
              }}
            />
          </label>
        </div>
      )}
      {taken && <div className="ws-error">O branch {target} já existe: escolha outro nome.</div>}
      {pending && (
        <div className="ses-pending">
          <Icon name="warning" size={16} />
          <span>
            {pendingWork(current, working.length, changes)}{' '}
            {where === 'copy' ? 'Numa cópia, a nova sessão não mexe nisso.' : 'Na pasta do projeto, a nova sessão trabalha em cima disso.'}
          </span>
        </div>
      )}
      <div className="ws-note">A sessão atual fica guardada e volta como está quando você abri-la de novo.</div>
      {leave.busy ? (
        <BusyConfirm why={leave.busy.why} onConfirm={leave.busy.confirm} onCancel={leave.cancel} />
      ) : (
        <button className="btn filled" type="submit" disabled={leave.running || taken || (where === 'copy' && !target)} style={{ justifyContent: 'center' }}>
          {leave.running && where === 'copy' ? 'Criando a cópia…' : 'Criar e abrir'}
        </button>
      )}
      {leave.error && <div className="ws-error">{leave.error}</div>}
    </form>
  );
}
