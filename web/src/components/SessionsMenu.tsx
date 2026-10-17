import { useEffect, useRef, useState } from 'react';
import { hub } from '../api/client';
import type { SessionList, SessionSummary } from '../api/types';
import { ago, busyOf, busyText, contents, when, type Busy } from '../lib/sessions';
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
              ? 'Trocando agora, o trabalho para no meio. Quando você voltar para esta sessão, o agente aparece como interrompido, com "Continuar".'
              : 'Trocando agora, o trabalho para no meio. Quando você voltar para esta sessão, os agentes aparecem como interrompidos, com "Continuar".'}
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
                <SessionRow ws={ws} s={s} now={now} disabled={leave.running} onOpen={() => open(s)} onError={setError} />
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
  onError,
}: {
  ws: string;
  s: SessionSummary;
  now: number;
  disabled: boolean;
  onOpen: () => void;
  onError: (msg: string) => void;
}) {
  const [editing, setEditing] = useState(false);
  const [name, setName] = useState(s.name);
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
  const remove = () => {
    const msg = `Remover a sessão "${s.name}"?\n\nO canvas e as notas dela vão para a lixeira do Squad (~/.squad/trash). As conversas continuam guardadas nos agentes.`;
    if (window.confirm(msg)) hub.removeSession(ws, s.id).then(setSessions, (err: Error) => onError(err.message));
  };

  return (
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
            <span className={`ses-row-name${s.named ? '' : ' provisional'}`} title={s.named ? s.name : `${s.name} (o agente dá o nome no fim do primeiro turno)`}>
              {s.name}
            </span>
          )}
          {s.current && <span className="ses-badge">atual</span>}
        </span>
        <span className="ses-row-sub">
          criada {when(s.createdAt)} · {s.current ? 'aberta agora' : `última abertura ${ago(s.openedAt, now)}`}
        </span>
        <span className="ses-row-sub">
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
        {!s.current && (
          <button
            className="icon-btn ses-hover ses-danger"
            onClick={remove}
            disabled={s.floors > 0}
            aria-label={`Remover ${s.name}`}
            title={s.floors > 0 ? 'Tem andares: abra a sessão e aterrisse ou descarte os andares antes' : 'Remover'}
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
  );
}

function NewSessionForm({ ws, onBack, onDone }: { ws: string; onBack: () => void; onDone: () => void }) {
  // The terminals a new session can start with: the ground floor's.
  const terminals = useSquad((s) => s.workspace?.nodes.filter((n) => n.terminal && !n.floor).length ?? 0);
  const [name, setName] = useState('');
  const [keep, setKeep] = useState(false);
  const leave = useLeave();

  const submit = (e: React.FormEvent) => {
    e.preventDefault();
    leave.run('new', async (confirm) => {
      setSessions(await hub.newSession(ws, { name: name.trim() || undefined, keepTerminals: keep, confirm }));
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
        <input autoFocus value={name} maxLength={60} placeholder="sem nome, o agente sugere um" onChange={(e) => setName(e.target.value)} />
      </label>
      {terminals > 0 && (
        <label className="ses-check">
          <input type="checkbox" checked={keep} onChange={(e) => setKeep(e.target.checked)} />
          <span>
            <b>Começar com os mesmos terminais, sem as conversas</b>: {terminals === 1 ? 'o terminal' : `os ${terminals} terminais`} do térreo, com papéis e fios.
            Cada agente começa uma conversa nova.
          </span>
        </label>
      )}
      <div className="ws-note">A sessão atual fica guardada e volta como está quando você abri-la de novo.</div>
      {leave.busy ? (
        <BusyConfirm why={leave.busy.why} onConfirm={leave.busy.confirm} onCancel={leave.cancel} />
      ) : (
        <button className="btn filled" type="submit" disabled={leave.running} style={{ justifyContent: 'center' }}>
          Criar e abrir
        </button>
      )}
      {leave.error && <div className="ws-error">{leave.error}</div>}
    </form>
  );
}
