import { useMemo, useState } from 'react';
import { useBranch } from '../lib/floors';
import type { IconName } from '../lib/icons';
import { clock, elapsed } from '../lib/model';
import { useNodeRoleColor } from '../lib/roles';
import { useNow } from '../lib/useNow';
import { useSquad, type ActivityItem } from '../state/store';
import { Icon } from './Icon';
import './ActivityDrawer.css';

const OUTCOME: Record<ActivityItem['outcome'], { label: (a: ActivityItem, now: number) => string; icon: IconName; state: string }> = {
  pending: { label: (a, now) => `Pendente · ${elapsed(a.time, now)}`, icon: 'hourglass_top', state: 'wait' },
  replied: { label: (a) => `Respondido em ${elapsed(Date.now() - (a.elapsedMs ?? 0), Date.now())}`, icon: 'reply', state: 'done' },
  delivered: { label: () => 'Entregue', icon: 'check', state: 'done' },
  cancelled: { label: () => 'Cancelado', icon: 'block', state: 'err' },
  held: { label: () => 'Retido', icon: 'pause', state: 'wait' },
  note: { label: () => 'Nota editada', icon: 'description', state: 'idle' },
  turn: { label: () => 'Fim do turno', icon: 'check_circle', state: 'done' },
};

export function ActivityDrawer() {
  const open = useSquad((s) => s.drawerOpen);
  const setDrawer = useSquad((s) => s.setDrawer);
  const items = useSquad((s) => s.activity);
  const ws = useSquad((s) => s.workspace);
  const extra = useSquad((s) => s.extra);
  const roleOf = useNodeRoleColor();
  const [filter, setFilter] = useState<string | null>(null);
  const branch = useBranch(useSquad((s) => s.activeFloor));
  const canSummarize = useSquad((s) => !!s.summarySettings && s.summarySettings.engine !== 'off' && s.summarySettings.engines.some((e) => e.available));

  const people = useMemo(() => {
    const names = new Map<string, string>();
    for (const n of ws?.nodes ?? []) if (n.terminal) names.set(n.id, n.terminal.name);
    return [...names.entries()];
  }, [ws]);
  const shown = filter ? items.filter((a) => a.from === filter || a.to === filter) : items;
  const pending = extra.pending.length;
  const now = useNow(1000, open && pending > 0);

  if (!open) {
    return (
      <button className="ad-tab" onClick={() => setDrawer(true)} aria-label="Abrir atividade">
        <Icon name="chevron_left" size={18} />
        <span className="ad-tab-label">Atividade</span>
        {pending > 0 && <span className="tb-count mono">{pending}</span>}
      </button>
    );
  }

  return (
    <aside className="activity-drawer" aria-label="Atividade">
      <header className="ad-head">
        <span className="ad-title">Atividade</span>
        <span className="ad-sub">
          {ws?.name ?? ''}
          {branch && ` · ${branch}`}
        </span>
        <span style={{ flex: 1 }} />
        {canSummarize && (
          <button className="btn ad-overview" onClick={() => useSquad.getState().setSummaryDialog('overview')} title="Uma visão geral do que a equipe está fazendo">
            <Icon name="psychology" size={16} />
            Resumo
          </button>
        )}
        <button className="icon-btn" onClick={() => setDrawer(false)} aria-label="Fechar">
          <Icon name="close" size={18} />
        </button>
      </header>
      <div className="ad-filters">
        <button className={`ad-chip${filter === null ? ' on' : ''}`} onClick={() => setFilter(null)}>
          Todos
        </button>
        {people.map(([id, name]) => (
          <button key={id} className={`ad-chip${filter === id ? ' on' : ''}`} onClick={() => setFilter(filter === id ? null : id)}>
            <i style={{ background: `var(--r-${roleOf(id)})` }} />
            {name}
          </button>
        ))}
      </div>
      <div className="ad-list">
        {shown.length === 0 && <div className="ad-empty">Nenhuma conversa ainda. Ligue dois terminais e use squad ask.</div>}
        {shown.map((a) => {
          const o = OUTCOME[a.outcome];
          return (
            <div key={a.key} className="ad-item">
              <span className="ad-time mono">{clock(a.time)}</span>
              <div>
                <div className="ad-line">
                  <span style={{ color: a.from ? `var(--r-${roleOf(a.from)})` : 'var(--text)' }}>{a.fromName || 'Você'}</span>
                  {a.outcome === 'turn' ? (
                    <span className="ad-turn">terminou o turno</span>
                  ) : (
                    <>
                      <Icon name={a.outcome === 'note' ? 'edit' : 'arrow_forward'} size={15} style={{ color: 'var(--text-2)' }} />
                      <span style={{ color: a.to ? `var(--r-${roleOf(a.to)})` : 'var(--text)' }}>{a.toName}</span>
                    </>
                  )}
                  {a.id && <span className="mono ad-id">{a.id}</span>}
                </div>
                {a.text && <div className="ad-msg">{a.text}</div>}
                <span className="ad-pill" style={{ background: `var(--stb-${o.state})`, color: `var(--st-${o.state})` }}>
                  <Icon name={o.icon} size={14} />
                  {o.label(a, now)}
                </span>
              </div>
            </div>
          );
        })}
      </div>
      <footer className="ad-foot">
        <Icon name="tune" size={16} style={{ color: 'var(--text-3)' }} />
        <span className="mono">
          máx. {extra.limits.maxHops} saltos · {extra.limits.perMinute} pedidos/min
        </span>
        <span style={{ flex: 1 }} />
        <span className="ad-limits">Limites</span>
      </footer>
    </aside>
  );
}
