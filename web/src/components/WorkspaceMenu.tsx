import { useEffect, useRef, useState } from 'react';
import { hub } from '../api/client';
import type { WorkspaceSummary } from '../api/types';
import { shortPath, trimDir } from '../lib/paths';
import { WS_COLORS, wsColor } from '../lib/workspaces';
import { useSquad } from '../state/store';
import { DirInput } from './DirInput';
import { Icon } from './Icon';
import './WorkspaceMenu.css';

function plural(n: number, one: string, many: string): string {
  return `${n} ${n === 1 ? one : many}`;
}

/** "2 ativos · 1 pendente", or how many terminals when idle. */
export function wsActivity(w: WorkspaceSummary): string {
  const parts = [];
  if (w.active) parts.push(plural(w.active, 'ativo', 'ativos'));
  if (w.pending) parts.push(plural(w.pending, 'pendente', 'pendentes'));
  if (!parts.length) parts.push(w.terminals ? plural(w.terminals, 'terminal', 'terminais') : 'sem terminais');
  return parts.join(' · ');
}

type Mode = { kind: 'list' } | { kind: 'new' } | { kind: 'edit'; ws: WorkspaceSummary };

/** The workspace selector under the top bar button (⌥1–9 switch directly). */
export function WorkspaceMenu({ onClose }: { onClose: () => void }) {
  const list = useSquad((s) => s.workspaces);
  const active = useSquad((s) => s.activeWs);
  const switchTo = useSquad((s) => s.switchWorkspace);
  const [mode, setMode] = useState<Mode>({ kind: 'list' });
  const ref = useRef<HTMLDivElement>(null);

  useEffect(() => {
    const away = (e: PointerEvent) => {
      // The top bar button toggles the menu itself.
      if (!ref.current?.contains(e.target as Node) && !(e.target as Element).closest?.('.tb-workspace')) onClose();
    };
    const esc = (e: KeyboardEvent) => e.key === 'Escape' && onClose();
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
              <div key={w.id} className={`ws-row${w.id === active ? ' on' : ''}`} role="option" aria-selected={w.id === active}>
                <button
                  className="ws-row-main"
                  onClick={() => {
                    switchTo(w.id);
                    onClose();
                  }}
                >
                  <span className="ws-dot" style={{ background: wsColor(w.color) }} />
                  <span className="ws-row-text">
                    <span className="ws-row-name">{w.name}</span>
                    <span className="ws-row-sub">
                      <span className="mono ws-row-path" title={w.cwd}>
                        <bdi>{shortPath(w.cwd)}</bdi>
                      </span>
                      <span className={w.active || w.pending ? 'ws-busy' : ''}>{wsActivity(w)}</span>
                    </span>
                  </span>
                  {i < 9 && <span className="kbd">⌥{i + 1}</span>}
                </button>
                <button className="icon-btn ws-row-edit" onClick={() => setMode({ kind: 'edit', ws: w })} aria-label={`Editar ${w.name}`} title="Editar">
                  <Icon name="edit" size={16} />
                </button>
              </div>
            ))}
          </div>
          <span className="ws-menu-div" />
          <button className="ws-new" onClick={() => setMode({ kind: 'new' })}>
            <Icon name="add" size={18} />
            Novo workspace
          </button>
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

function WorkspaceForm({ ws, last, onBack, onDone }: { ws?: WorkspaceSummary; last: boolean; onBack: () => void; onDone: () => void }) {
  const switchTo = useSquad((s) => s.switchWorkspace);
  const [name, setName] = useState(ws?.name ?? '');
  const [cwd, setCwd] = useState(ws?.cwd ?? '');
  const [color, setColor] = useState(ws?.color ?? '');
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
        const created = await hub.create({ name: name.trim(), cwd: trimDir(cwd), ...(color ? { color } : {}) });
        switchTo(created.id);
      }
      onDone();
    });
  };

  const remove = () => {
    if (!ws) return;
    const msg = `Remover o workspace "${ws.name}"?\n\nOs terminais dele são encerrados, e o canvas e as notas vão para a lixeira do Squad (~/.squad/trash).`;
    if (!window.confirm(msg)) return;
    void run(async () => {
      await hub.remove(ws.id);
      const st = useSquad.getState();
      if (st.activeWs === ws.id) {
        const next = st.workspaces.find((w) => w.id !== ws.id);
        if (next) st.switchWorkspace(next.id);
      }
      onDone();
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
      {moved && <div className="ws-note">Os terminais abertos continuam na pasta antiga até serem reiniciados.</div>}
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
