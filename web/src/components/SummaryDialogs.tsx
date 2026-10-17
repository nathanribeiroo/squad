import { useEffect, useState } from 'react';
import { api } from '../api/client';
import type { SummaryEngine, SummarySettings, WorkspaceOverview } from '../api/types';
import { callsign, clock } from '../lib/model';
import { useNodeRoleColor } from '../lib/roles';
import { toast } from '../lib/toast';
import { useSquad } from '../state/store';
import { Dialog } from './Dialog';
import { Icon } from './Icon';
import './SummaryDialogs.css';

/** The summary dialog open, if any: the workspace overview or the settings. */
export function SummaryDialogs() {
  const d = useSquad((s) => s.summaryDialog);
  const close = () => useSquad.getState().setSummaryDialog(null);
  if (d === 'overview') return <Overview onClose={close} />;
  if (d === 'settings') return <Settings onClose={close} />;
  return null;
}

const ENGINES: { id: SummaryEngine; label: string; hint?: string }[] = [
  { id: 'auto', label: 'Automático', hint: 'O agente do próprio voo resume (um Devin pelo devin, um Claude pelo claude); sem a CLI dele, a primeira instalada.' },
  { id: 'claude', label: 'Claude Code', hint: 'claude -p, com um modelo pequeno (Haiku)' },
  { id: 'devin', label: 'Devin', hint: 'devin -p' },
  { id: 'codex', label: 'Codex', hint: 'codex exec' },
  { id: 'gemini', label: 'Gemini', hint: 'gemini -p' },
  { id: 'off', label: 'Desligado', hint: 'Só os boletins, sem resumos' },
];

function Settings({ onClose }: { onClose: () => void }) {
  const [s, setS] = useState<SummarySettings | null>(useSquad.getState().summarySettings);
  useEffect(() => {
    api.summarySettings().then(setS, (e: Error) => toast(e.message, 'err'));
  }, []);
  const save = (patch: Partial<Pick<SummarySettings, 'cards' | 'engine'>>) => {
    if (!s) return;
    const next = { cards: s.cards, engine: s.engine, ...patch };
    setS({ ...s, ...next });
    api.setSummarySettings(next).then(
      (r) => {
        setS(r);
        useSquad.getState().setSummarySettings(r);
      },
      (e: Error) => toast(e.message, 'err'),
    );
  };
  const found = (id: string) => s?.engines.find((e) => e.name === id);
  const none = !!s && !s.engines.some((e) => e.available);
  return (
    <Dialog icon="psychology" title="Resumos e briefing" sub="Neste workspace" onClose={onClose}>
      {!s ? (
        <p className="field-hint">Lendo…</p>
      ) : (
        <div className="sm-settings">
          <label className="sm-check">
            <input type="checkbox" checked={s.cards} onChange={(e) => save({ cards: e.target.checked })} />
            <span>
              <b>Boletim ao fim de cada turno</b>
              <span>A mensagem final do agente, embaixo do voo. Não custa nada: vem dos hooks dos agentes.</span>
            </span>
          </label>
          <div className="sm-group">
            <div className="sm-title">Quem escreve os resumos (botão Resumir e briefing)</div>
            {ENGINES.map((e) => {
              const info = found(e.id);
              const missing = !!info && !info.available;
              return (
                <label key={e.id} className={`sm-engine${s.engine === e.id ? ' on' : ''}${missing ? ' missing' : ''}`}>
                  <input type="radio" checked={s.engine === e.id} onChange={() => save({ engine: e.id })} />
                  <span>
                    <b>{e.label}</b>
                    {info && <span className={`sm-tag${info.available ? ' ok' : ''}`}>{info.available ? 'instalado' : 'não encontrado'}</span>}
                    {e.hint && <span className="sm-hint">{e.hint}</span>}
                  </span>
                </label>
              );
            })}
          </div>
          {none && <p className="fl-warn">Nenhuma CLI de agente foi encontrada no PATH da Torre. Os boletins funcionam; o botão Resumir não aparece.</p>}
          <p className="field-hint">
            Cada resumo gasta um pouco do plano da ferramenta que o escreve, e só roda quando você clica. Ele roda numa pasta vazia, sem ferramentas, e não mexe no
            projeto.
          </p>
        </div>
      )}
    </Dialog>
  );
}

function Overview({ onClose }: { onClose: () => void }) {
  const [o, setO] = useState<WorkspaceOverview | null>(null);
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);
  const nodes = useSquad((s) => s.workspace?.nodes);
  const wsName = useSquad((s) => s.workspace?.name ?? '');
  const roleOf = useNodeRoleColor();
  const load = () => {
    setBusy(true);
    setError('');
    api.overview().then(
      (r) => {
        setO(r);
        setBusy(false);
      },
      (e: Error) => {
        setError(e.message);
        setBusy(false);
      },
    );
  };
  useEffect(load, []);
  const flight = (name: string) => nodes?.find((x) => x.terminal?.name === name);
  const go = (name: string) => {
    const n = flight(name);
    if (!n) return;
    onClose();
    useSquad.getState().setFloor(n.floor ?? '', n.id);
  };
  return (
    <Dialog
      icon="summarize"
      title="Briefing"
      sub={[wsName, o && `por ${o.by} · ${clock(o.at)}`].filter(Boolean).join(' · ')}
      onClose={onClose}
      wide
      foot={
        <>
          <span className="sm-cost">Cada briefing gasta um pouco do plano de quem o escreve.</span>
          <span className="tn-spacer" />
          <button className="btn" onClick={load} disabled={busy}>
            <Icon name="refresh" />
            Atualizar
          </button>
        </>
      }
    >
      {busy && (
        <p className="sm-busy">
          <Icon name="progress_activity" size={16} className="spin" />
          Lendo o workspace… leva alguns segundos.
        </p>
      )}
      {error && <p className="fl-error">{error}</p>}
      {o && !busy && (
        <>
          <p className="sm-text">{o.text}</p>
          {o.attention.length > 0 && (
            <div className="sm-attention">
              <div className="label-caps">Pede atenção</div>
              <ul>
                {o.attention.slice(0, 3).map((a) => (
                  <li key={a}>
                    <Icon name="front_hand" size={15} />
                    <span>{a}</span>
                  </li>
                ))}
              </ul>
            </div>
          )}
          {o.terminals.length > 0 && (
            <div className="sm-flights">
              <div className="label-caps">Por voo</div>
              <ul className="sm-terms">
                {o.terminals.map((t) => {
                  const n = flight(t.name);
                  return (
                    <li key={t.name}>
                      <i style={{ background: n ? `var(--r-${roleOf(n.id)})` : 'var(--text-3)' }} />
                      <button className="sm-cs mono" onClick={() => go(t.name)} title={`Ir até ${t.name}`} disabled={!n}>
                        {n?.terminal ? callsign(n.terminal) : t.name}
                      </button>
                      <span>{t.status}</span>
                    </li>
                  );
                })}
              </ul>
            </div>
          )}
        </>
      )}
    </Dialog>
  );
}
