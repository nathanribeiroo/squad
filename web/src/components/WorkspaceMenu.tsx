import { useEffect, useRef, useState } from 'react';
import { hub } from '../api/client';
import type { SessionSummary, WorkspaceSummary } from '../api/types';
import { shortPath, trimDir } from '../lib/paths';
import { ago, busyOf, busyText, contents } from '../lib/sessions';
import { toast, toastError } from '../lib/toast';
import { useNow } from '../lib/useNow';
import { WS_COLORS, wsColor } from '../lib/workspaces';
import { useSquad } from '../state/store';
import { DirInput } from './DirInput';
import { Icon } from './Icon';
import { confirmDialog, confirmOpen } from './ConfirmDialog';
import { useLeave } from './SessionsMenu';
import './SessionsMenu.css';
import './WorkspaceMenu.css';

function plural(n: number, one: string, many: string): string {
  return `${n} ${n === 1 ? one : many}`;
}

/** "2 no ar · 1 pendente", or how many flights when idle; "inativo" or
 * "carregando 3/8" first when its agents are stopped or starting. */
export function wsActivity(w: WorkspaceSummary): string {
  const parts = [];
  if (w.attention) parts.push(`${w.attention} pede${w.attention > 1 ? 'm' : ''} você`);
  if (w.active) parts.push(`${w.active} no ar`);
  if (w.pending) parts.push(plural(w.pending, 'pendente', 'pendentes'));
  if (w.interrupted) parts.push(plural(w.interrupted, 'interrompido', 'interrompidos'));
  if (!parts.length) parts.push(w.terminals ? plural(w.terminals, 'voo', 'voos') : 'sem voos');
  if (w.state === 'loading' && w.loading) parts.unshift(`carregando ${w.loading.ready}/${w.loading.total}`);
  if (w.state === 'inactive') parts.unshift('inativo');
  return parts.join(' · ');
}

function activityClass(w: WorkspaceSummary): string {
  if (w.attention) return 'ws-ask';
  if (w.pending || w.interrupted) return 'ws-wait';
  if (w.active || w.state === 'loading') return 'ws-busy';
  return '';
}

/** Stops a workspace's agents now; with agents working, asks first. */
async function sleepWorkspace(ws: WorkspaceSummary, confirm = false): Promise<void> {
  try {
    await hub.sleep(ws.id, confirm);
    toast(`${ws.name} inativo: os voos voltam quando você entrar nele.`, 'ok');
  } catch (err) {
    const why = busyOf(err);
    if (!why || confirm) throw err;
    const ok = await confirmDialog({
      title: `Inativar ${ws.name} agora?`,
      icon: 'bedtime',
      tone: 'warn',
      text: busyText(why),
      lose: [
        why.working.length > 0 && `O turno de ${why.working.join(', ')} para no meio`,
        why.pending > 0 && (why.pending === 1 ? 'O pedido em andamento é cancelado' : `Os ${why.pending} pedidos em andamento são cancelados`),
      ],
      keep: ['As conversas e as telas dos voos', 'Ao voltar, os voos parados no meio aparecem em arremetida, com "Continuar"'],
      action: 'Inativar mesmo assim',
    });
    if (ok) await sleepWorkspace(ws, true);
  }
}

type Mode = { kind: 'list' } | { kind: 'new' } | { kind: 'edit'; ws: WorkspaceSummary };

/** Asks, then removes a workspace; the one on screen gives way to another.
 * False when the user changed their mind. */
async function removeWorkspace(ws: WorkspaceSummary): Promise<boolean> {
  if (useSquad.getState().workspaces.length < 2) {
    toast('O último workspace não pode ser removido: crie outro antes.', 'wait');
    return false;
  }
  const ok = await confirmDialog({
    title: `Remover o workspace ${ws.name}?`,
    tone: 'danger',
    lose: [
      ws.terminals ? `${ws.terminals === 1 ? 'O voo dele é encerrado' : `Os ${ws.terminals} voos dele são encerrados`}` : '',
      `${ws.sessions === 1 ? 'A sessão sai' : `As ${ws.sessions} sessões saem`} da lista: o canvas e as NOTAMs vão para a lixeira da Torre (~/.torre/trash)`,
    ],
    keep: [
      <>
        A pasta do projeto, <span className="mono">{shortPath(ws.cwd)}</span>, como está
      </>,
      'As conversas, guardadas nos agentes',
    ],
    action: 'Remover workspace',
  });
  if (!ok) return false;
  await hub.remove(ws.id);
  const st = useSquad.getState();
  if (st.activeWs === ws.id) {
    const next = st.workspaces.find((w) => w.id !== ws.id);
    if (next) st.switchWorkspace(next.id);
  }
  return true;
}

/** The workspace selector under the top bar button (⌥1–9 switch directly). */
export function WorkspaceMenu({ onClose }: { onClose: () => void }) {
  const list = useSquad((s) => s.workspaces);
  const active = useSquad((s) => s.activeWs);
  const switchTo = useSquad((s) => s.switchWorkspace);
  const [mode, setMode] = useState<Mode>({ kind: 'list' });
  // The workspace whose sessions show beside the list (hover or the arrow).
  const [peek, setPeek] = useState<{ id: string; top: number } | null>(null);
  const timer = useRef<number | undefined>(undefined);
  const ref = useRef<HTMLDivElement>(null);
  useEffect(() => () => window.clearTimeout(timer.current), []);
  const peekAt = (id: string, row: HTMLElement, delay: number) => {
    window.clearTimeout(timer.current);
    const top = row.getBoundingClientRect().top - (ref.current?.getBoundingClientRect().top ?? 0);
    if (delay) timer.current = window.setTimeout(() => setPeek({ id, top }), delay);
    else setPeek({ id, top });
  };
  const peeked = mode.kind === 'list' && peek ? list.find((w) => w.id === peek.id) : undefined;

  useEffect(() => {
    const away = (e: PointerEvent) => {
      // The top bar button toggles the menu itself.
      const t = e.target as Element;
      if (!ref.current?.contains(t) && !t.closest?.('.tb-workspace, .dlg-backdrop')) onClose();
    };
    const esc = (e: KeyboardEvent) => e.key === 'Escape' && !confirmOpen() && onClose();
    window.addEventListener('pointerdown', away, true);
    window.addEventListener('keydown', esc);
    return () => {
      window.removeEventListener('pointerdown', away, true);
      window.removeEventListener('keydown', esc);
    };
  }, [onClose]);

  return (
    <div className="popover ws-menu" ref={ref} role="dialog" aria-label="Workspaces">
      {mode.kind === 'list' ? (
        <>
          <div className="ws-menu-title">Workspaces</div>
          <div className="ws-list" role="listbox">
            {list.map((w, i) => (
              <div
                key={w.id}
                className={`ws-row${w.id === active ? ' on' : ''}${w.id === peeked?.id ? ' peek' : ''}${w.state === 'inactive' ? ' sleeping' : ''}`}
                role="option"
                aria-selected={w.id === active}
                onMouseEnter={(e) => peekAt(w.id, e.currentTarget, 250)}
                onMouseLeave={() => window.clearTimeout(timer.current)}
              >
                <button
                  className="ws-row-main"
                  onClick={() => {
                    switchTo(w.id);
                    onClose();
                  }}
                >
                  <span className="ws-dot" style={{ background: wsColor(w.color) }} />
                  <span className="ws-row-text">
                    <span className="ws-row-name">
                      <span className="ws-row-label">{w.name}</span>
                      {w.keepActive && (
                        <span className="ws-pin" title="Sempre ativo">
                          <Icon name="keep" size={13} fill />
                        </span>
                      )}
                      {i < 9 && <span className="kbd">⌥{i + 1}</span>}
                    </span>
                    <span className="ws-row-sub">
                      <span className="ws-row-path" title={w.cwd}>
                        <bdi>{shortPath(w.cwd)}</bdi>
                      </span>
                      <span className={activityClass(w)}>
                        {w.state === 'inactive' && <Icon name="bedtime" size={12} className="ws-state-icon" />}
                        {wsActivity(w)}
                      </span>
                    </span>
                  </span>
                </button>
                <button
                  className={`icon-btn ws-row-edit ws-row-keep${w.keepActive ? ' on' : ''}`}
                  onClick={() => hub.update(w.id, { keepActive: !w.keepActive }).catch(toastError)}
                  aria-label={`Manter ${w.name} sempre ativo`}
                  aria-pressed={!!w.keepActive}
                  title={w.keepActive ? 'Sempre ativo: os voos não param fora da tela. Clique para desligar' : 'Manter sempre ativo: os voos não param fora da tela'}
                >
                  <Icon name="keep" size={16} fill={w.keepActive} />
                </button>
                {w.id !== active && w.state !== 'inactive' && (
                  <button
                    className="icon-btn ws-row-edit"
                    onClick={() => sleepWorkspace(w).catch(toastError)}
                    aria-label={`Inativar ${w.name} agora`}
                    title="Inativar agora: os voos param, cada um com a sua conversa, e voltam quando você entrar"
                  >
                    <Icon name="bedtime" size={16} />
                  </button>
                )}
                <button className="icon-btn ws-row-edit" onClick={() => setMode({ kind: 'edit', ws: w })} aria-label={`Editar ${w.name}`} title="Editar">
                  <Icon name="edit" size={16} />
                </button>
                <button
                  className="icon-btn ws-row-edit ws-row-remove"
                  onClick={() => removeWorkspace(w).then((done) => done && onClose(), toastError)}
                  aria-label={`Remover ${w.name}`}
                  title={list.length < 2 ? 'O último workspace não pode ser removido' : 'Remover'}
                >
                  <Icon name="delete" size={16} />
                </button>
                <button
                  className="icon-btn ws-row-sessions"
                  onClick={(e) => peekAt(w.id, e.currentTarget.parentElement!, 0)}
                  aria-label={`Sessões de ${w.name}`}
                  aria-expanded={w.id === peeked?.id}
                  title={w.sessions === 1 ? '1 sessão' : `${w.sessions} sessões`}
                >
                  {w.sessions > 1 && <span className="mono">{w.sessions}</span>}
                  <Icon name="chevron_right" size={16} />
                </button>
              </div>
            ))}
          </div>
          <span className="ws-menu-div" />
          <button className="ws-new" onClick={() => setMode({ kind: 'new' })} onMouseEnter={() => window.clearTimeout(timer.current)}>
            <Icon name="add" size={18} />
            Novo workspace
          </button>
          {peeked && <SessionsPeek key={peeked.id} ws={peeked} top={peek!.top} onDone={onClose} />}
        </>
      ) : (
        <WorkspaceForm
          ws={mode.kind === 'edit' ? mode.ws : undefined}
          last={list.length <= 1}
          onBack={() => setMode({ kind: 'list' })}
          onDone={onClose}
        />
      )}
    </div>
  );
}

/** A workspace's sessions, beside its row: opening one there opens it and
 * shows that workspace. */
function SessionsPeek({ ws, top, onDone }: { ws: WorkspaceSummary; top: number; onDone: () => void }) {
  const list = useSquad((s) => s.sessionLists[ws.id]);
  const switchTo = useSquad((s) => s.switchWorkspace);
  const [error, setError] = useState('');
  const leave = useLeave();
  const now = useNow(30_000);

  useEffect(() => {
    hub.sessions(ws.id).then(useSquad.getState().setSessions, (err: Error) => setError(err.message));
  }, [ws.id]);

  const pick = (s: SessionSummary) => {
    if (s.current) {
      switchTo(ws.id);
      onDone();
      return;
    }
    leave.run(async (confirm) => {
      useSquad.getState().setSessions(await hub.openSession(ws.id, s.id, confirm));
      switchTo(ws.id);
      onDone();
    });
  };

  return (
    <div className="popover ses-peek" style={{ top }} role="listbox" aria-label={`Sessões de ${ws.name}`}>
      <div className="ws-menu-title">Sessões de {ws.name}</div>
      <div className="ses-list">
        {list?.sessions.map((s) => (
          <div key={s.id}>
            <button className={`ses-peek-row${s.current ? ' on' : ''}`} role="option" aria-selected={s.current} disabled={leave.running} onClick={() => pick(s)}>
              <span className="ses-row-head">
                <span className={`ses-row-name${s.named ? '' : ' provisional'}`}>{s.name}</span>
                {s.current && <span className="ses-badge">atual</span>}
              </span>
              <span className="ses-row-sub">
                {s.current ? 'aberta agora' : ago(s.openedAt, now)} · {contents(s)}
              </span>
            </button>
          </div>
        ))}
        {!list && !error && <div className="ses-empty">Carregando…</div>}
      </div>
      {(error || leave.error) && <div className="ws-error ses-error">{error || leave.error}</div>}
    </div>
  );
}

function WorkspaceForm({ ws, last, onBack, onDone }: { ws?: WorkspaceSummary; last: boolean; onBack: () => void; onDone: () => void }) {
  const switchTo = useSquad((s) => s.switchWorkspace);
  const [name, setName] = useState(ws?.name ?? '');
  const [cwd, setCwd] = useState(ws?.cwd ?? '');
  const [color, setColor] = useState(ws?.color ?? '');
  const [session, setSession] = useState('');
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);
  const moved = !!ws && trimDir(cwd) !== ws.cwd;

  const run = async (fn: () => Promise<void>) => {
    setBusy(true);
    setError('');
    try {
      await fn();
    } catch (err) {
      setError((err as Error).message);
    } finally {
      setBusy(false);
    }
  };

  const submit = (e: React.FormEvent) => {
    e.preventDefault();
    void run(async () => {
      if (ws) {
        await hub.update(ws.id, { name: name.trim() || ws.name, cwd: trimDir(cwd), ...(color ? { color } : {}) });
      } else {
        const created = await hub.create({ name: name.trim(), cwd: trimDir(cwd), session: session.trim(), ...(color ? { color } : {}) });
        switchTo(created.id);
      }
      onDone();
    });
  };

  const remove = () => {
    if (!ws) return;
    void run(async () => {
      if (await removeWorkspace(ws)) onDone();
    });
  };

  return (
    <form className="ws-form" onSubmit={submit}>
      <div className="ws-form-head">
        <button type="button" className="icon-btn" onClick={onBack} aria-label="Voltar">
          <Icon name="arrow_back" size={18} />
        </button>
        <strong>{ws ? 'Editar workspace' : 'Novo workspace'}</strong>
      </div>
      <label className="field">
        Pasta do projeto
        <DirInput value={cwd} onChange={setCwd} placeholder="~/projetos/meu-app" suggest={hub.dirs} />
      </label>
      <label className="field">
        Nome {!ws && '(opcional)'}
        <input value={name} placeholder={ws?.name ?? (cwd ? trimDir(cwd).split(/[\\/]/).pop() : 'nome da pasta')} onChange={(e) => setName(e.target.value)} />
      </label>
      {!ws && (
        <label className="field">
          Nome da sessão (opcional)
          <input value={session} maxLength={60} placeholder="sem nome, o voo líder sugere um" onChange={(e) => setSession(e.target.value)} />
        </label>
      )}
      <div className="field">
        Cor
        <div className="ws-colors" role="radiogroup" aria-label="Cor">
          {Object.keys(WS_COLORS).map((k) => (
            <button
              key={k}
              type="button"
              role="radio"
              aria-checked={color === k}
              aria-label={k}
              className={`ws-swatch${color === k ? ' on' : ''}`}
              style={{ background: WS_COLORS[k] }}
              onClick={() => setColor(k)}
            />
          ))}
        </div>
      </div>
      {moved && <div className="ws-note">Os voos abertos continuam na pasta antiga até serem reiniciados.</div>}
      {error && <div className="ws-error">{error}</div>}
      <button className="btn filled" type="submit" disabled={busy || !cwd.trim()} style={{ justifyContent: 'center' }}>
        {ws ? 'Salvar' : 'Criar e abrir'}
      </button>
      {ws && !last && (
        <button type="button" className="ws-remove" onClick={remove} disabled={busy}>
          <Icon name="delete" size={16} />
          Remover workspace
        </button>
      )}
    </form>
  );
}
