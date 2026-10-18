import { useEffect, useMemo, useRef, useState } from 'react';
import type { UsageAccount, UsageFlight, WNode } from '../api/types';
import { AGENTS, callsign } from '../lib/model';
import { ago } from '../lib/sessions';
import { USAGE_AGENTS, acus, credits, level, percent, renewed, resets, tightest, tokens, usd, windowLabel, windowShort } from '../lib/usage';
import { useNow } from '../lib/useNow';
import { useSquad } from '../state/store';
import { Icon } from './Icon';
import './UsageMenu.css';

const agentName = (a: string) => AGENTS.find((x) => x.value === a)?.label ?? a;

/** The workspace's agent flights (every level), with what each used. */
function useRows(): { node: WNode; usage?: UsageFlight }[] {
  const nodes = useSquad((s) => s.workspace?.nodes);
  const usage = useSquad((s) => s.usage);
  return useMemo(
    () =>
      (nodes ?? [])
        .filter((n) => n.terminal && (USAGE_AGENTS.includes(n.terminal.agent) || usage[n.id]))
        .map((n) => ({ node: n, usage: usage[n.id] })),
    [nodes, usage],
  );
}

/** The accounts to show: those of the workspace's agents, else every one seen. */
function useAccounts(rows: { node: WNode }[]): { agent: string; account?: UsageAccount }[] {
  const accounts = useSquad((s) => s.accounts);
  return useMemo(() => {
    const here = USAGE_AGENTS.filter((a) => rows.some((r) => r.node.terminal?.agent === a));
    const agents = here.length ? here : USAGE_AGENTS.filter((a) => accounts[a]);
    return agents.map((agent) => ({ agent, account: accounts[agent] }));
  }, [rows, accounts]);
}

/** CONSUMO in the top bar: the fullest plan window (or the tokens used), and the box with the detail. */
export function UsageChip() {
  const [open, setOpen] = useState(false);
  const rows = useRows();
  const accounts = useAccounts(rows);
  const now = useNow(60_000);
  const activeWs = useSquad((s) => s.activeWs);
  useEffect(() => setOpen(false), [activeWs]);
  const top = tightest(accounts.flatMap((a) => (a.account ? [a.account] : [])), now);
  const total = rows.reduce((n, r) => n + (r.usage ? r.usage.tokens.input + r.usage.tokens.output + r.usage.tokens.cacheRead + r.usage.tokens.cacheWrite : 0), 0);
  const title = top ? `Consumo: ${windowLabel(top).toLowerCase()} em ${percent(top.used)}` : 'Consumo: tokens, cache, custo e limites dos agentes';

  return (
    <div className="tb-anchor">
      <button className={`tb-btn usage-chip${open ? ' on' : ''}`} aria-expanded={open} onClick={() => setOpen(!open)} title={title}>
        <Icon name="data_usage" size={16} />
        <span className="usage-chip-label">Consumo</span>
        {top ? (
          <span className={`usage-chip-val mono ${level(top.used)}`}>
            {windowShort(top)} {percent(top.used)}
          </span>
        ) : (
          total > 0 && <span className="usage-chip-val mono">{tokens(total)}</span>
        )}
      </button>
      {open && <UsageMenu rows={rows} accounts={accounts} onClose={() => setOpen(false)} />}
    </div>
  );
}

function UsageMenu({ rows, accounts, onClose }: { rows: { node: WNode; usage?: UsageFlight }[]; accounts: { agent: string; account?: UsageAccount }[]; onClose: () => void }) {
  const ref = useRef<HTMLDivElement>(null);
  const now = useNow(30_000);
  const floors = useSquad((s) => s.workspace?.floors);

  useEffect(() => {
    const away = (e: PointerEvent) => {
      const t = e.target as Element;
      if (!ref.current?.contains(t) && !t.closest?.('.usage-chip')) onClose();
    };
    const esc = (e: KeyboardEvent) => e.key === 'Escape' && onClose();
    window.addEventListener('pointerdown', away, true);
    window.addEventListener('keydown', esc);
    return () => {
      window.removeEventListener('pointerdown', away, true);
      window.removeEventListener('keydown', esc);
    };
  }, [onClose]);

  const goTo = (n: WNode) => {
    const st = useSquad.getState();
    if (st.view === 'synoptic') st.setSynPick(n.id);
    else st.setFloor(n.floor ?? '', n.id);
    onClose();
  };

  const sum = rows.reduce(
    (s, r) => {
      const u = r.usage;
      if (!u) return s;
      return {
        input: s.input + u.tokens.input,
        output: s.output + u.tokens.output,
        cacheRead: s.cacheRead + u.tokens.cacheRead,
        cacheWrite: s.cacheWrite + u.tokens.cacheWrite,
        cost: s.cost + (u.costUsd ?? 0),
        acus: s.acus + (u.acus ?? 0),
        credits: s.credits + (u.credits ?? 0),
      };
    },
    { input: 0, output: 0, cacheRead: 0, cacheWrite: 0, cost: 0, acus: 0, credits: 0 },
  );
  const floorOf = (n: WNode) => (n.floor ? floors?.find((f) => f.id === n.floor) : undefined);

  return (
    <div className="popover usage-menu" ref={ref} role="dialog" aria-label="Consumo">
      <div className="usage-title">Consumo</div>
      {accounts.map(({ agent, account }) => (
        <AccountSection key={agent} agent={agent} account={account} now={now} />
      ))}
      {rows.length > 0 ? (
        <table className="usage-table">
          <thead>
            <tr>
              <th>Voo</th>
              <th title="Tokens de entrada, fora do cache">Entrada</th>
              <th>Saída</th>
              <th title="Tokens lidos do cache (mais baratos)">Cache lido</th>
              <th title="Tokens gravados no cache">Cache criado</th>
              <th title="Quanto da janela de contexto a conversa ocupa agora">Contexto</th>
              <th title="Claude: estimativa a preços da API. Devin: ACUs ou créditos">Custo</th>
            </tr>
          </thead>
          <tbody>
            {rows.map(({ node, usage: u }) => {
              const t = node.terminal!;
              const fl = floorOf(node);
              return (
                <tr key={node.id} className={u ? '' : 'none'} onClick={() => goTo(node)} title={`Ir para ${t.name}${u?.models?.length ? ` · ${u.models.join(', ')}` : ''}`}>
                  <td className="usage-flight">
                    <b className="mono">{callsign(t)}</b>
                    <span className="usage-flight-name">{t.name}</span>
                    <span className="usage-flight-agent mono">
                      {fl ? `FL${fl.number ?? ''} · ` : ''}
                      {t.agent}
                    </span>
                  </td>
                  {u ? (
                    <>
                      <td className="mono">{tokens(u.tokens.input)}</td>
                      <td className="mono">{tokens(u.tokens.output)}</td>
                      <td className="mono">{tokens(u.tokens.cacheRead)}</td>
                      <td className="mono">{tokens(u.tokens.cacheWrite)}</td>
                      <td className="mono" title={u.context ? `${tokens(u.context.used)}${u.context.size ? ` de ${tokens(u.context.size)}` : ''} tokens` : undefined}>
                        {/* Without the window's size (Devin before its footer showed), the tokens. */}
                        {!u.context ? '—' : u.context.size ? <span className={level(u.context.percent)}>{percent(u.context.percent)}</span> : tokens(u.context.used)}
                      </td>
                      <td className="mono">{cost(u)}</td>
                    </>
                  ) : (
                    <td className="usage-none" colSpan={6}>
                      Aparece depois da primeira resposta
                    </td>
                  )}
                </tr>
              );
            })}
          </tbody>
          {rows.filter((r) => r.usage).length > 1 && (
            <tfoot>
              <tr>
                <td>Total</td>
                <td className="mono">{tokens(sum.input)}</td>
                <td className="mono">{tokens(sum.output)}</td>
                <td className="mono">{tokens(sum.cacheRead)}</td>
                <td className="mono">{tokens(sum.cacheWrite)}</td>
                <td />
                <td className="mono">
                  {sum.cost > 0 && <div>{usd(sum.cost)}</div>}
                  {sum.acus > 0 && <div>{acus(sum.acus)}</div>}
                  {sum.credits > 0 && <div>{credits(sum.credits)}</div>}
                  {!sum.cost && !sum.acus && !sum.credits && '—'}
                </td>
              </tr>
            </tfoot>
          )}
        </table>
      ) : (
        <div className="usage-empty">Nenhum voo de agente neste workspace.</div>
      )}
      <div className="usage-foot">
        Lido no seu computador, do que os agentes já gravam: nada vai para a rede. O custo do Claude é a estimativa dele a preços da API; nos planos Pro e Max ele
        não é cobrado.
      </div>
    </div>
  );
}

/** What a flight cost: Claude's estimate in dollars, Devin's ACUs or credits. */
function cost(u: UsageFlight): string {
  if (u.acus) return acus(u.acus);
  if (u.credits) return credits(u.credits);
  if (u.costUsd) return usd(u.costUsd);
  return '—';
}

/** An agent account: its plan windows, its ACUs, or why there are none. */
function AccountSection({ agent, account, now }: { agent: string; account?: UsageAccount; now: number }) {
  const windows = account?.windows ?? [];
  const a = account?.acus;
  let empty = '';
  if (!windows.length && !a) {
    if (agent === 'gemini') empty = 'O Gemini não informa limites de plano.';
    else if (!account && agent === 'devin') empty = 'A cota aparece quando você abre /usage num voo do Devin.';
    else if (!account) empty = `Os limites aparecem depois da primeira resposta${agent === 'claude' ? ' (planos Pro e Max)' : agent === 'codex' ? ' (conta do ChatGPT)' : ''}.`;
    else empty = 'A conta não informa limites (chave de API ou plano de empresa).';
  }
  return (
    <section className="usage-account">
      <div className="usage-account-head">
        <span className="usage-agent">{agentName(agent)}</span>
        {account && <span className="usage-seen">visto {ago(account.seen, now)}</span>}
      </div>
      {windows.map((w) => {
        const old = renewed(w, now);
        return (
          <div key={w.kind} className={`usage-window${old ? ' old' : ''}`}>
            <span className="usage-window-label">{windowLabel(w)}</span>
            <span className="usage-bar">
              <i className={old ? '' : level(w.used)} style={{ width: `${Math.min(100, Math.max(0, w.used))}%` }} />
            </span>
            <span className={`usage-window-val mono ${old ? '' : level(w.used)}`}>{old ? '—' : percent(w.used)}</span>
            <span className="usage-window-reset">{resets(w, now)}</span>
          </div>
        );
      })}
      {a && (
        <div className="usage-window">
          <span className="usage-window-label">ACUs</span>
          {a.limit ? (
            <>
              <span className="usage-bar">
                <i className={level((a.used * 100) / a.limit)} style={{ width: `${Math.min(100, (a.used * 100) / a.limit)}%` }} />
              </span>
              <span className="usage-window-val mono">{percent((a.used * 100) / a.limit)}</span>
              <span className="usage-window-reset mono">
                {acus(a.used).replace(' ACU', '')} de {acus(a.limit)}
              </span>
            </>
          ) : (
            <span className="usage-window-total mono">{acus(a.used)} na conta</span>
          )}
        </div>
      )}
      {empty && <div className="usage-account-empty">{empty}</div>}
    </section>
  );
}
