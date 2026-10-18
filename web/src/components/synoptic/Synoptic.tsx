import { useMemo, useState } from 'react';
import type { Floor, TerminalRuntime, UIState, WNode } from '../../api/types';
import { CHOICES, askPhrase, choiceTitle, respond } from '../../lib/attention';
import { checkViews, flightLevel, gitSummary } from '../../lib/floors';
import { loopOf, onLoop } from '../../lib/loop';
import { STATE_META, callsign, clock, elapsed } from '../../lib/model';
import { useNodeRoleColor } from '../../lib/roles';
import { useNow } from '../../lib/useNow';
import { useScreenHidden, useScreenPlace } from '../../lib/screens';
import { XTermView } from '../../term/XTermView';
import { AskOptions } from '../AskOptions';
import { HiddenScreen } from '../HiddenScreen';
import { LoopCard } from '../LoopCard';
import { useSquad } from '../../state/store';
import { Icon } from '../Icon';
import './Synoptic.css';

/**
 * Sinóptico (docs/torre/TELAS.md, quadro 01): the whole session at a glance,
 * to leave open all day. The radar, the strips, the landing queue, the
 * frequencies, the flight asking you and the selected flight's terminal.
 * Everything comes from the state the canvas already has (⌥S switches).
 */
export function Synoptic() {
  const nodes = useSquad((s) => s.workspace?.nodes);
  const floors = useSquad((s) => s.workspace?.floors);
  const runtime = useSquad((s) => s.runtime);
  const askingIds = useSquad((s) => s.askingIds);
  const pick = useSquad((s) => s.synPick);
  const pinned = useSquad((s) => s.synPinned);
  const flights = useMemo(() => (nodes ?? []).filter((n) => n.terminal), [nodes]);
  // The terminal on the right: the pinned one, else the selected, else the one asking you, else the first.
  const shown = flights.find((n) => n.id === pick) ?? flights.find((n) => n.id === askingIds[0]) ?? sortStrips(flights, runtime)[0];
  return (
    <main className="syn">
      <section className="syn-radar">
        <Radar flights={flights} floors={floors ?? []} runtime={runtime} selected={shown?.id} />
      </section>
      <section className="syn-mid">
        <Strips flights={flights} floors={floors ?? []} runtime={runtime} selected={shown?.id} />
        <LandingQueue floors={floors ?? []} flights={flights} runtime={runtime} />
        <Frequencies flights={flights} />
      </section>
      <section className="syn-right">
        <LoopCard inSynoptic />
        <PriorityCard flights={flights} floors={floors ?? []} runtime={runtime} />
        {shown && <SelectedTerminal n={shown} floors={floors ?? []} rt={runtime[shown.id]} pinned={pinned && shown.id === pick} />}
      </section>
      <footer className="syn-foot">
        <RadioTicker />
        <button className="syn-transmit" onClick={() => useSquad.getState().setComposer(true, shown?.id ?? null)}>
          <span className="syn-transmit-label">Transmitir →</span>
          {shown?.terminal && <span className="mono syn-transmit-cs">{callsign(shown.terminal)}</span>}
          <span className="syn-transmit-ph">escreva para o voo…</span>
          <span className="kbd">⌥K</span>
        </button>
      </footer>
    </main>
  );
}

// The order of the strips: who asks you, who waits, who flies, who landed.
const ORDER: UIState[] = ['ask', 'wait', 'work', 'boot', 'done', 'idle', 'err', 'off'];

function sortStrips(flights: WNode[], runtime: Record<string, TerminalRuntime>): WNode[] {
  // In solo after a turn of yours (SUA VEZ) right after who asks you.
  const rank = (n: WNode) => {
    const rt = runtime[n.id];
    return rt?.yourTurn && rt.activity === 'done' ? 0.5 : ORDER.indexOf(rt?.activity ?? 'off');
  };
  return [...flights].sort((a, b) => rank(a) - rank(b));
}

const levelOf = (n: WNode, floors: Floor[]) => (n.floor ? flightLevel(floors.find((f) => f.id === n.floor)) : 'PISTA');

/** Goes to a flight in the Espaço: its level, centered. */
function openInSpace(n: WNode) {
  const st = useSquad.getState();
  st.setView('space');
  st.setFloor(n.floor ?? '', n.id);
}

const SYMBOL: Record<UIState, string> = { work: '▲', ask: '●', done: '□', off: '○', err: '○', wait: '△', idle: '◇', boot: '◇' };

function Radar({ flights, floors, runtime, selected }: { flights: WNode[]; floors: Floor[]; runtime: Record<string, TerminalRuntime>; selected?: string }) {
  const pending = useSquad((s) => s.extra.pending);
  const lastReply = useSquad((s) => s.lastReply);
  const loop = useSquad((s) => loopOf(s.extra.pause));
  const now = useNow(2000);
  const levels = useMemo(() => [...floors].sort((a, b) => (a.number ?? 0) - (b.number ?? 0)), [floors]);
  const R = 290;
  const C = 300;
  const band = R / (levels.length + 1);
  // Real positions, scaled: the runway in the middle, a ring per level.
  const pos = useMemo(() => {
    const out = new Map<string, { x: number; y: number }>();
    const groups = [{ id: '', ring: 0 }, ...levels.map((f, i) => ({ id: f.id, ring: i + 1 }))];
    for (const g of groups) {
      const ns = flights.filter((n) => (n.floor ?? '') === g.id);
      if (!ns.length) continue;
      const xs = ns.map((n) => n.x + n.w / 2);
      const ys = ns.map((n) => n.y + n.h / 2);
      const [x0, x1, y0, y1] = [Math.min(...xs), Math.max(...xs), Math.min(...ys), Math.max(...ys)];
      ns.forEach((n, i) => {
        const nx = x1 > x0 ? (xs[i] - x0) / (x1 - x0) : 0.5;
        const ny = y1 > y0 ? (ys[i] - y0) / (y1 - y0) : 0.5;
        if (g.ring === 0) {
          out.set(n.id, { x: C + (nx - 0.5) * band * 1.2, y: C + (ny - 0.5) * band * 1.2 });
        } else {
          const a = (ns.length === 1 ? 0.15 : nx * 0.85) * Math.PI * 2 - Math.PI / 2;
          const r = g.ring * band + (0.3 + 0.4 * ny) * band;
          out.set(n.id, { x: C + Math.cos(a) * r, y: C + Math.sin(a) * r });
        }
      });
    }
    return out;
  }, [flights, levels, band]);
  const counts = (s: UIState[]) => flights.filter((n) => s.includes(runtime[n.id]?.activity ?? 'off')).length;
  const counters: [string, number, string][] = [
    ['No ar', counts(['work', 'boot']), 'work'],
    ['Prioridade', counts(['ask']), 'ask'],
    ['Aguarda', counts(['wait']), 'wait'],
    ['Em solo', counts(['done', 'idle']), 'done'],
    ['Hangar', counts(['off', 'err']), 'off'],
  ];
  return (
    <>
      <div className="syn-counters mono">
        {counters.map(([label, n, st]) => (
          <span key={label} style={{ color: `var(--stt-${st})` }}>
            {n} {label}
          </span>
        ))}
      </div>
      <svg className="syn-scope" viewBox="0 0 600 600" role="img" aria-label="Radar">
        <defs>
          <radialGradient id="syn-bg">
            <stop offset="0" stopColor="#0a1f12" />
            <stop offset="1" stopColor="#040b07" />
          </radialGradient>
          <linearGradient id="syn-sweep" x1="0" y1="0" x2="1" y2="0">
            <stop offset="0" stopColor="#3cff8a" stopOpacity="0" />
            <stop offset="1" stopColor="#3cff8a" stopOpacity="0.3" />
          </linearGradient>
        </defs>
        <circle cx={C} cy={C} r={R} fill="url(#syn-bg)" className="syn-bgc" />
        {[...levels, null].map((f, i) => (
          <circle key={f?.id ?? 'ground'} cx={C} cy={C} r={band * (i + 1)} className="syn-ring" />
        ))}
        <line x1={C - R} y1={C} x2={C + R} y2={C} className="syn-cross" />
        <line x1={C} y1={C - R} x2={C} y2={C + R} className="syn-cross" />
        <text x={C} y={C + band - 6} className="syn-ring-label">
          PISTA
        </text>
        {levels.map((f, i) => (
          <text key={f.id} x={C} y={C - band * (i + 2) + 14} className="syn-ring-label">
            {flightLevel(f)}
          </text>
        ))}
        <g className="syn-sweep">
          <path d={`M${C},${C} L${C + R},${C} A${R},${R} 0 0,0 ${C + R * Math.cos(-0.6)},${C + R * Math.sin(-0.6)} Z`} fill="url(#syn-sweep)" />
        </g>
        {loop?.chain.slice(1).map((s, i) => {
          const a = pos.get(loop.chain[i].id);
          const b = pos.get(s.id);
          return a && b ? <line key={`l-${i}`} x1={a.x} y1={a.y} x2={b.x} y2={b.y} className="syn-link loop" /> : null;
        })}
        {pending.map((p) => {
          const a = pos.get(p.from);
          const b = pos.get(p.to);
          return a && b ? <line key={p.id} x1={a.x} y1={a.y} x2={b.x} y2={b.y} className="syn-link pending" /> : null;
        })}
        {Object.entries(lastReply)
          .filter(([, r]) => now - r.at < 30_000 && r.from)
          .map(([to, r]) => {
            const a = pos.get(r.from!);
            const b = pos.get(to);
            return a && b ? <line key={`r-${to}`} x1={a.x} y1={a.y} x2={b.x} y2={b.y} className="syn-link reply" /> : null;
          })}
        {flights.map((n) => {
          const p = pos.get(n.id);
          if (!p) return null;
          const st = runtime[n.id]?.activity ?? 'off';
          return (
            <g
              key={n.id}
              className={`syn-flight st-${st}${selected === n.id ? ' sel' : ''}`}
              transform={`translate(${p.x},${p.y})`}
              onClick={() => useSquad.getState().setSynPick(n.id)}
              onDoubleClick={() => openInSpace(n)}
            >
              {st === 'ask' && <circle r="14" className="syn-ping" />}
              <text className="syn-sym" textAnchor="middle" dominantBaseline="central">
                {SYMBOL[st]}
              </text>
              <text x="14" y="-4" className="syn-tag">
                {callsign(n.terminal!)}
              </text>
              <text x="14" y="9" className="syn-tag-sub">
                {(n.floor ? levelOf(n, floors) : 'PISTA') + ' · ' + STATE_META[st].label.toUpperCase()}
              </text>
            </g>
          );
        })}
      </svg>
      <div className="syn-legend mono">
        <span style={{ color: 'var(--stt-work)' }}>▲ no ar</span>
        <span style={{ color: 'var(--stt-ask)' }}>● pede você</span>
        <span style={{ color: 'var(--stt-wait)' }}>△ aguarda</span>
        <span style={{ color: 'var(--stt-done)' }}>□ em solo</span>
        <span style={{ color: 'var(--stt-off)' }}>○ hangar</span>
        <span>— pedido · resposta</span>
      </div>
    </>
  );
}

/** What a flight is doing, in a few words, for its strip. */
function doing(n: WNode, rt: TerminalRuntime | undefined, nodes: WNode[], pending: { from: string; to: string; queued?: boolean }[], card?: { message: string }): string {
  const st = rt?.activity ?? 'off';
  if (st === 'ask') return askPhrase(rt?.attention);
  if (st === 'wait') {
    const p = pending.find((x) => x.from === n.id);
    const t = p && nodes.find((x) => x.id === p.to)?.terminal;
    if (!t) return 'Aguarda outro voo';
    return p.queued ? `Na fila de ${callsign(t)}` : `Aguarda ${callsign(t)}`;
  }
  if (st === 'work') return rt?.title?.replace(/^[⠀-⣿✳◐◓◑◒⏳*·\s]+/, '') || 'No ar';
  if (card?.message) return card.message.split('\n')[0];
  return STATE_META[st].label;
}

function Strips({ flights, floors, runtime, selected }: { flights: WNode[]; floors: Floor[]; runtime: Record<string, TerminalRuntime>; selected?: string }) {
  const pending = useSquad((s) => s.extra.pending);
  const cards = useSquad((s) => s.cards);
  const now = useNow(1000);
  const sorted = sortStrips(flights, runtime);
  return (
    <div className="syn-box">
      <div className="syn-box-head label-caps">
        Fichas <span className="mono">{flights.length}</span>
      </div>
      <div className="syn-strips">
        {sorted.length === 0 && <p className="syn-empty">Nenhum voo nesta sessão.</p>}
        {sorted.map((n) => {
          const rt = runtime[n.id];
          const st = rt?.activity ?? 'off';
          return (
            <button
              key={n.id}
              className={`syn-strip st-${st}${selected === n.id ? ' sel' : ''}`}
              onClick={() => useSquad.getState().setSynPick(n.id)}
              onDoubleClick={() => openInSpace(n)}
            >
              <i className="syn-strip-band" />
              <span className="syn-strip-cs mono">{callsign(n.terminal!)}</span>
              <span className="syn-strip-what">
                <span className="syn-strip-doing">{doing(n, rt, flights, pending, cards[n.id])}</span>
                <span className="syn-strip-sub mono">
                  {n.terminal!.agent} · {levelOf(n, floors)}
                </span>
              </span>
              <span className="syn-strip-state mono">
                {rt?.yourTurn && st === 'done' ? <b className="syn-your-turn">Sua vez</b> : STATE_META[st].label}
                {rt?.activitySince && ['work', 'wait', 'ask', 'done'].includes(st) && <span>{elapsed(rt.attention?.since ?? rt.activitySince, now)}</span>}
              </span>
            </button>
          );
        })}
      </div>
    </div>
  );
}

const CHECK_CLASS = { pass: 'ok', fail: 'bad', stale: 'warn', none: 'idle' } as const;

function LandingQueue({ floors, flights, runtime }: { floors: Floor[]; flights: WNode[]; runtime: Record<string, TerminalRuntime> }) {
  const status = useSquad((s) => s.floorStatus);
  const ready = floors.filter((f) => (status?.floors[f.id]?.ahead ?? 0) > 0);
  return (
    <div className="syn-box">
      <div className="syn-box-head label-caps">
        Fila de pouso <span className="mono">{ready.length}</span>
      </div>
      {ready.length === 0 && <p className="syn-empty">Nenhum nível com commits para pousar.</p>}
      {ready.map((f) => {
        const g = status?.floors[f.id];
        const flying = flights.filter((n) => n.floor === f.id && ['work', 'wait', 'ask'].includes(runtime[n.id]?.activity ?? '')).length;
        return (
          <div key={f.id} className="syn-land">
            <div className="mono syn-land-head">
              <b>{flightLevel(f)}</b> · {f.branch} → {f.base || 'main'} · {g?.ahead} {g?.ahead === 1 ? 'commit' : 'commits'}
            </div>
            <div className="syn-land-checks mono">
              {checkViews(f, g?.head).map((c) => (
                <span key={c.kind} className={CHECK_CLASS[c.state]} title={c.detail}>
                  {c.label}
                </span>
              ))}
              <span className={g?.changes ? 'bad' : 'ok'}>{g?.changes ? `✕ ${gitSummary(g)?.text}` : '✓ tudo commitado'}</span>
              <span className={flying ? 'bad' : 'ok'}>{flying ? `✕ ${flying} no ar` : '✓ ninguém no ar'}</span>
            </div>
            <button className="btn" onClick={() => useSquad.getState().setFloorDialog({ kind: 'land', id: f.id })}>
              Autorizar pouso…
            </button>
          </div>
        );
      })}
    </div>
  );
}

function Frequencies({ flights }: { flights: WNode[] }) {
  const edges = useSquad((s) => s.workspace?.edges);
  const pending = useSquad((s) => s.extra.pending);
  const lastReply = useSquad((s) => s.lastReply);
  const limits = useSquad((s) => s.extra.limits);
  const sendTimes = useSquad((s) => s.sendTimes);
  const loop = useSquad((s) => loopOf(s.extra.pause));
  const now = useNow(2000);
  const cs = (id: string) => {
    const t = flights.find((n) => n.id === id)?.terminal;
    return t ? callsign(t) : null;
  };
  const wires = (edges ?? [])
    .map((e) => ({ e, a: cs(e.a), b: cs(e.b) }))
    .filter((w) => w.a && w.b)
    .map((w) => {
      const open = pending.some((p) => (p.from === w.e.a && p.to === w.e.b) || (p.from === w.e.b && p.to === w.e.a));
      const replied = [w.e.a, w.e.b].some((id) => {
        const r = lastReply[id];
        return r && now - r.at < 60_000 && (r.from === w.e.a || r.from === w.e.b);
      });
      return { ...w, kind: onLoop(loop, w.e.a, w.e.b) ? 'loop' : open ? 'pending' : replied ? 'reply' : 'idle' };
    });
  const rate = sendTimes.filter((t) => now - t < 60_000).length;
  return (
    <div className="syn-box">
      <div className="syn-box-head label-caps">
        Frequências <span className="mono">{wires.length}</span>
      </div>
      <div className="syn-freqs">
        {wires.length === 0 && <p className="syn-empty">Nenhum voo na mesma frequência.</p>}
        {wires.map((w) => (
          <span key={w.e.id} className={`syn-freq mono ${w.kind}`}>
            {w.a} ⇄ {w.b}
            {w.kind === 'loop' && ' · laço'}
            {w.kind === 'pending' && ' · pedido'}
            {w.kind === 'reply' && ' · respondeu'}
          </span>
        ))}
      </div>
      <div className="syn-freq-foot mono">
        {rate}/{limits.perMinute} por min · máx. {limits.maxHops} saltos
      </div>
    </div>
  );
}

function PriorityCard({ flights, floors, runtime }: { flights: WNode[]; floors: Floor[]; runtime: Record<string, TerminalRuntime> }) {
  const ids = useSquad((s) => s.askingIds);
  const [index, setIndex] = useState(0);
  const now = useNow(1000, ids.length > 0);
  if (!ids.length) return null;
  const i = Math.min(index, ids.length - 1);
  const n = flights.find((x) => x.id === ids[i]);
  const rt = n && runtime[n.id];
  const a = rt?.attention;
  if (!n || !rt) return null;
  return (
    <div className="syn-prio" role="alert">
      <div className="syn-prio-head">
        <i className="syn-prio-dot" />
        <b>Prioridade · pede você</b>
        {ids.length > 1 && (
          <span className="syn-prio-nav mono">
            <button onClick={() => setIndex((i - 1 + ids.length) % ids.length)} aria-label="Anterior">
              ‹
            </button>
            {i + 1} de {ids.length}
            <button onClick={() => setIndex((i + 1) % ids.length)} aria-label="Próximo">
              ›
            </button>
          </span>
        )}
        <span style={{ flex: 1 }} />
        <span className="mono">{elapsed(a?.since ?? rt.activitySince, now)}</span>
      </div>
      <div className="syn-prio-who">
        <span className="syn-prio-cs mono">{callsign(n.terminal!)}</span>
        <span>
          <b>{n.terminal!.name}</b>
          <span className="mono syn-prio-sub">
            {n.terminal!.agent} · {levelOf(n, floors)}
          </span>
        </span>
        <span style={{ flex: 1 }} />
        <button className="btn" onClick={() => openInSpace(n)}>
          Ver no espaço
        </button>
      </div>
      <div className="syn-prio-q mono">
        <b>{askPhrase(a)}</b>
        {a?.detail && <span>{a.detail}</span>}
        {a?.text && <span className="syn-prio-text">{a.text}</span>}
      </div>
      <div className="syn-prio-actions">
        {CHOICES.filter((c) => a?.choices?.includes(c.choice)).map((c) => (
          <button key={c.choice} className={`tn-respond-btn ${c.choice}`} title={choiceTitle(a, c)} onClick={() => respond(n.id, c.choice)}>
            {c.label}
            <span className="kbd">{c.key}</span>
          </button>
        ))}
        {a?.options?.length ? <AskOptions id={n.id} a={a} who={callsign(n.terminal!)} /> : null}
        {!a?.choices?.length && !a?.options?.length && <span className="syn-prio-note">Responda na tela do voo, abaixo.</span>}
      </div>
      {a?.choices?.includes('always') && a.labels?.always && <div className="syn-prio-text mono">Sempre = {a.labels.always}</div>}
    </div>
  );
}

function SelectedTerminal({ n, floors, rt, pinned }: { n: WNode; floors: Floor[]; rt?: TerminalRuntime; pinned: boolean }) {
  const roleOf = useNodeRoleColor();
  const st = rt?.activity ?? 'off';
  const now = useNow(1000);
  const place = useScreenPlace(n.id);
  const hidden = useScreenHidden(n.id);
  return (
    <div className={`syn-term st-${st}`}>
      <header className="syn-term-head">
        <i style={{ background: `var(--r-${roleOf(n.id)})` }} />
        <b className="mono">{callsign(n.terminal!)}</b>
        <span>{n.terminal!.name}</span>
        <span className="mono syn-term-agent">{n.terminal!.agent}</span>
        <span style={{ flex: 1 }} />
        <span className="mono" style={{ color: `var(--stt-${st})` }}>
          {STATE_META[st].label.toUpperCase()} {rt?.activitySince && elapsed(rt.attention?.since ?? rt.activitySince, now)}
        </span>
        <button
          className={`icon-btn${pinned ? ' on' : ''}`}
          title={pinned ? 'Solto: volta a seguir a seleção' : 'Fixar este voo aqui'}
          aria-pressed={pinned}
          onClick={() => useSquad.getState().setSynPinned(!pinned, n.id)}
        >
          <Icon name="keep" size={16} />
        </button>
      </header>
      <div className="syn-term-body" onMouseDown={() => useSquad.getState().setFocusedTerminal(n.id)}>
        {place === 'focus' ? <div className="tn-elsewhere">Em foco · ⌥F volta</div> : <XTermView id={n.id} zoom={1} />}
        {hidden && <HiddenScreen id={n.id} />}
      </div>
      <footer className="syn-term-foot mono">
        {levelOf(n, floors)} · {pinned ? 'fixado' : 'segue a seleção'} · clique para digitar
      </footer>
    </div>
  );
}

/** The latest messages, scrolling by (40 s a lap; still with reduced motion). */
function RadioTicker() {
  const items = useSquad((s) => s.activity);
  const nodes = useSquad((s) => s.workspace?.nodes);
  const cs = (id: string | undefined, name: string) => {
    const t = id && nodes?.find((n) => n.id === id)?.terminal;
    return t ? callsign(t) : name || 'VOCÊ';
  };
  const text = items
    .slice(0, 12)
    .map((a) => `${clock(a.time)} ${cs(a.from, a.fromName)} → ${a.outcome === 'turn' ? 'fim do turno' : cs(a.to, a.toName)}${a.text ? ` · ${a.text.split('\n')[0].slice(0, 80)}` : ''}`)
    .join('   ◆   ');
  return (
    <button className="syn-radio" onClick={() => useSquad.getState().setDrawer(true)} title="Abrir o Rádio (⌥A)">
      <span className="syn-radio-tag">Rádio</span>
      <span className="syn-radio-track mono">
        <span className="syn-radio-run">{text || 'Rádio em silêncio.'}</span>
      </span>
    </button>
  );
}
