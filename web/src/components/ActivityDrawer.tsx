import { useMemo, useState } from 'react';
import { api } from '../api/client';
import { CHOICES, askPhrase, choiceTitle, respond } from '../lib/attention';
import type { IconName } from '../lib/icons';
import { CHECK_NAMES } from '../lib/floors';
import { callsign, clock, clockDay, elapsed } from '../lib/model';
import { useNodeRoleColor } from '../lib/roles';
import { toast } from '../lib/toast';
import { useNow } from '../lib/useNow';
import { useSquad, type ActivityItem } from '../state/store';
import { confirmDialog } from './ConfirmDialog';
import { Icon } from './Icon';
import './ActivityDrawer.css';

// Outcome pills (docs/torre/TELAS.md, quadro 07): style is the pill's look.
const OUTCOME: Record<ActivityItem['outcome'], { label: (a: ActivityItem, now: number) => string; icon: IconName; style: string }> = {
  pending: {
    label: (a, now) => (a.queued ? `Na fila · ${a.toName || 'o voo'} termina o turno · ${elapsed(a.time, now)}` : `Pendente · ${elapsed(a.time, now)}`),
    icon: 'hourglass_top',
    style: 'wait dashed',
  },
  replied: { label: (a) => `Respondido em ${elapsed(Date.now() - (a.elapsedMs ?? 0), Date.now())}`, icon: 'reply', style: 'done' },
  delivered: { label: () => 'Entregue', icon: 'check', style: 'done' },
  cancelled: { label: () => 'Cancelado', icon: 'block', style: 'err' },
  held: { label: (a) => (a.hold === 'rate' ? 'Retido · limite por minuto' : 'Retido · espera geral'), icon: 'all_inclusive', style: 'wait dashed' },
  note: { label: () => 'NOTAM editado', icon: 'description', style: 'idle' },
  turn: { label: () => 'Em solo', icon: 'flight_land', style: 'done' },
  check: {
    label: (a) => `${a.check ? CHECK_NAMES[a.check] : ''} · ${a.result === 'fail' ? 'falhou' : 'passou'}`,
    icon: 'check_circle',
    style: 'done',
  },
  answered: {
    label: (a) => `Pediu você · ${a.choice ? ANSWERED[a.choice] : 'respondido'} em ${elapsed(Date.now() - (a.elapsedMs ?? 0), Date.now())}`,
    icon: 'front_hand',
    style: 'done',
  },
};

// How an ask was answered from the interface (none: in the terminal).
const ANSWERED = { allow: 'autorizado', always: 'sempre', deny: 'negado' } as const;

/** Asks before clearing the session's Rádio. */
async function clearRadio(session?: string) {
  const ok = await confirmDialog({
    title: 'Limpar o Rádio?',
    icon: 'delete',
    tone: 'danger',
    text: `Some o histórico do Rádio ${session ? `da sessão ${session}` : 'desta sessão'}, também depois de recarregar.`,
    lose: ['Os pedidos, as respostas, as mensagens e os pedidos de permissão já registrados'],
    keep: ['Os pedidos ainda sem resposta e os voos que pedem você', 'Os boletins de fim de turno nos voos'],
    action: 'Limpar o Rádio',
  });
  if (ok) api.clearRadio().catch((e: Error) => toast(e.message, 'err'));
}

type Filter = { kind: 'all' } | { kind: 'pending' } | { kind: 'ask' } | { kind: 'node'; id: string };

/** The Rádio (Atividade): who asked what of whom, and how it ended. */
export function ActivityDrawer() {
  const open = useSquad((s) => s.drawerOpen);
  const setDrawer = useSquad((s) => s.setDrawer);
  const items = useSquad((s) => s.activity);
  const more = useSquad((s) => s.radioMore);
  const ws = useSquad((s) => s.workspace);
  const extra = useSquad((s) => s.extra);
  const sendTimes = useSquad((s) => s.sendTimes);
  const session = useSquad((s) => s.sessionLists[s.activeWs ?? '']?.sessions.find((x) => x.id === s.activeSession)?.name);
  const roleOf = useNodeRoleColor();
  const [filter, setFilter] = useState<Filter>({ kind: 'all' });
  const askingIds = useSquad((s) => s.askingIds);
  const runtime = useSquad((s) => s.runtime);
  const canSummarize = useSquad((s) => !!s.summarySettings && s.summarySettings.engine !== 'off' && s.summarySettings.engines.some((e) => e.available));

  // Flights by id: callsign and floor (the drawer speaks in callsigns).
  const flights = useMemo(() => {
    const m = new Map<string, { cs: string; floor: string }>();
    for (const n of ws?.nodes ?? []) if (n.terminal) m.set(n.id, { cs: callsign(n.terminal), floor: n.floor ?? '' });
    return m;
  }, [ws]);
  const pendingIds = new Set(extra.pending.map((p) => p.id));
  const isPending = (a: ActivityItem) => a.outcome === 'pending' && !!a.id && pendingIds.has(a.id);
  let shown = items;
  if (filter.kind === 'pending') shown = items.filter(isPending);
  if (filter.kind === 'ask') shown = items.filter((a) => a.outcome === 'answered');
  if (filter.kind === 'node') shown = items.filter((a) => a.from === filter.id || a.to === filter.id);
  // What is still open goes first.
  shown = [...shown.filter(isPending), ...shown.filter((a) => !isPending(a))];
  const pending = extra.pending.length;
  const now = useNow(1000, open);
  // Flights asking the user are pinned above everything (PRIORIDADE).
  const asks = filter.kind === 'pending' ? [] : askingIds.filter((id) => filter.kind !== 'node' || filter.id === id);
  const rate = sendTimes.filter((t) => now - t < 60_000).length;

  if (!open) {
    return (
      <button className="ad-tab" onClick={() => setDrawer(true)} aria-label="Abrir o Rádio">
        <Icon name="chevron_left" size={16} />
        <span className="ad-tab-label">Rádio</span>
        {askingIds.length > 0 ? <span className="tb-count ask mono">{askingIds.length}</span> : pending > 0 && <span className="tb-count mono">{pending}</span>}
      </button>
    );
  }

  const who = (id: string | undefined, name: string) => (id && flights.get(id)?.cs) || (name === 'Você' || !name ? 'VOCÊ' : name);
  const color = (id?: string) => (id && flights.has(id) ? `var(--rt-${roleOf(id)})` : 'var(--text)');
  const goTo = (id?: string) => {
    if (!id || !flights.has(id)) return;
    useSquad.getState().setFloor(flights.get(id)!.floor, id);
  };

  return (
    <aside className="activity-drawer" aria-label="Rádio">
      <header className="ad-head">
        <Icon name="cell_tower" size={18} className="ad-icon" />
        <span className="ad-title">Rádio</span>
        <span className="ad-sub mono">
          {ws?.name ?? ''}
          {session && ` · ${session}`}
        </span>
        <span style={{ flex: 1 }} />
        {canSummarize && (
          <button className="btn ad-overview" onClick={() => useSquad.getState().setSummaryDialog('overview')} title="Briefing: uma visão geral do que os voos estão fazendo">
            <Icon name="summarize" size={15} />
            Briefing
          </button>
        )}
        {items.length > 0 && (
          <button className="icon-btn" onClick={() => void clearRadio(session)} title="Limpar o Rádio" aria-label="Limpar o Rádio">
            <Icon name="delete" size={17} />
          </button>
        )}
        <button className="icon-btn" onClick={() => setDrawer(false)} aria-label="Fechar">
          <Icon name="close" size={18} />
        </button>
      </header>
      <div className="ad-filters">
        <button className={`ad-chip${filter.kind === 'all' ? ' on' : ''}`} onClick={() => setFilter({ kind: 'all' })}>
          Todos
        </button>
        <button className={`ad-chip ask${filter.kind === 'ask' ? ' on' : ''}`} onClick={() => setFilter(filter.kind === 'ask' ? { kind: 'all' } : { kind: 'ask' })}>
          Pedem você {askingIds.length}
        </button>
        <button className={`ad-chip wait${filter.kind === 'pending' ? ' on' : ''}`} onClick={() => setFilter(filter.kind === 'pending' ? { kind: 'all' } : { kind: 'pending' })}>
          Pendentes {pending}
        </button>
        {[...flights.entries()].map(([id, f]) => {
          const on = filter.kind === 'node' && filter.id === id;
          return (
            <button key={id} className={`ad-chip mono${on ? ' on' : ''}`} onClick={() => setFilter(on ? { kind: 'all' } : { kind: 'node', id })}>
              <i style={{ background: `var(--r-${roleOf(id)})` }} />
              {f.cs}
            </button>
          );
        })}
      </div>
      <div className="ad-list">
        {asks.map((id) => {
          const rt = runtime[id];
          const a = rt?.attention;
          return (
            <div key={`ask-${id}`} className="ad-item pinned ask">
              <div className="ad-line mono">
                <span className="ad-time">{clock(a?.since ?? rt?.activitySince ?? Date.now())}</span>
                <b style={{ color: color(id) }}>{who(id, '')}</b>
                <span className="ad-arrow">→</span>
                <b>VOCÊ</b>
                <span style={{ flex: 1 }} />
                <span className="ad-pill ask">
                  <Icon name="front_hand" size={13} />
                  Pede você · {elapsed(a?.since ?? rt?.activitySince, now)}
                </span>
              </div>
              <div className="ad-msg">
                {askPhrase(a)}
                {a?.detail && <code className="ad-detail mono">{a.detail}</code>}
              </div>
              <div className="ad-actions">
                {CHOICES.filter((c) => a?.choices?.includes(c.choice)).map((c) => (
                  <button key={c.choice} className={`btn${c.choice === 'allow' ? ' filled ask' : ''}`} title={choiceTitle(a, c)} onClick={() => respond(id, c.choice)}>
                    {c.label}
                  </button>
                ))}
                <button className="btn" onClick={() => goTo(id)}>
                  Ver no espaço
                </button>
              </div>
              {a?.choices?.includes('always') && a.labels?.always && <div className="ad-always mono">Sempre = {a.labels.always}</div>}
            </div>
          );
        })}
        {shown.length === 0 && asks.length === 0 && <div className="ad-empty">Rádio em silêncio. Ponha dois voos na mesma frequência e use torre ask.</div>}
        {shown.map((a) => {
          const o = OUTCOME[a.outcome];
          const open = isPending(a);
          return (
            <div key={a.key} className={`ad-item${open ? ' pinned' : ''}`}>
              <div className="ad-line mono">
                <span className="ad-time">{clockDay(a.time)}</span>
                <b style={{ color: color(a.from) }}>{who(a.from, a.fromName)}</b>
                {a.outcome === 'turn' ? (
                  <span className="ad-turn">fim do turno</span>
                ) : (
                  <>
                    <span className="ad-arrow">→</span>
                    <b style={{ color: color(a.to) }}>{a.outcome === 'note' || a.outcome === 'check' ? a.toName : who(a.to, a.toName)}</b>
                  </>
                )}
                {a.id && <span className="ad-id">{a.id}</span>}
                <span style={{ flex: 1 }} />
                <span className={`ad-pill ${a.outcome === 'check' && a.result === 'fail' ? 'err' : o.style}`}>
                  <Icon name={a.outcome === 'check' && a.result === 'fail' ? 'error' : o.icon} size={13} />
                  {o.label(a, now)}
                </span>
              </div>
              {a.text && a.outcome !== 'turn' && <Message item={a} />}
              {open && (
                <div className="ad-actions">
                  <button className="btn" onClick={() => goTo(a.to)}>
                    Ver no espaço
                  </button>
                  <button
                    className="btn"
                    onClick={() => api.cancelRequest(a.id!).catch((e: Error) => toast(e.message, 'err'))}
                    title={`${who(a.from, a.fromName)} recebe "cancelado" agora; ${who(a.to, a.toName)} não é interrompido`}
                  >
                    Cancelar pedido
                  </button>
                </div>
              )}
            </div>
          );
        })}
        {more && (
          <button className="btn ad-older" onClick={() => useSquad.getState().loadOlderRadio()}>
            <Icon name="history" size={15} />
            Mais antigos
          </button>
        )}
      </div>
      <footer className="ad-foot mono">
        <Icon name="swap_horiz" size={15} />
        <span>
          {rate}/{extra.limits.perMinute} por min · máx. {extra.limits.maxHops} saltos
        </span>
        <span style={{ flex: 1 }} />
        <button className="btn ad-limits" onClick={() => useSquad.getState().openSettings('limits')} title="Configurações → Limites">
          Limites
        </button>
      </footer>
    </aside>
  );
}

/** A message in the Rádio: four lines, and the whole request and its reply on demand. */
function Message({ item }: { item: ActivityItem }) {
  const [texts, setTexts] = useState<{ text: string; reply?: string } | null>(null);
  const [open, setOpen] = useState<'request' | 'reply' | null>(null);
  // Requests arrive as a preview cut at 140 characters ("…").
  const cut = !!item.id && item.key.startsWith('req-') && item.text.endsWith('…');
  const show = (what: 'request' | 'reply') => {
    if (open === what) return setOpen(null);
    if (texts) return setOpen(what);
    api
      .requestText(item.id!)
      .then((r) => {
        setTexts(r);
        setOpen(what);
      })
      .catch((e: Error) => toast(e.message, 'err'));
  };
  return (
    <>
      <div className={`ad-msg${open === 'request' ? ' full' : ''}`}>{open === 'request' && texts ? texts.text : item.text}</div>
      {open === 'reply' && texts?.reply && <div className="ad-msg full ad-reply">{texts.reply}</div>}
      {(cut || item.hasReply) && (
        <div className="ad-mores">
          {cut && (
            <button className="ad-more" onClick={() => show('request')}>
              {open === 'request' ? 'Mostrar menos' : 'Ver o pedido inteiro'}
            </button>
          )}
          {item.hasReply && (
            <button className="ad-more" onClick={() => show('reply')}>
              {open === 'reply' ? 'Esconder a resposta' : 'Ver a resposta'}
            </button>
          )}
        </div>
      )}
    </>
  );
}
