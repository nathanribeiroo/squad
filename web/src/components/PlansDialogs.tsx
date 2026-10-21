import { useEffect, useMemo, useRef, useState } from 'react';
import { api, hub } from '../api/client';
import type { AgentInfo, Floor, Plan } from '../api/types';
import { flightLevel, plural } from '../lib/floors';
import { useRoleLookup, roleColor } from '../lib/roles';
import { colorVar } from '../lib/visual';
import { toast } from '../lib/toast';
import { useSquad } from '../state/store';
import { confirmDialog } from './ConfirmDialog';
import { Dialog } from './Dialog';
import { Icon } from './Icon';
import './PlansDialogs.css';

// A selector must not return a new [] each time (zustand would loop).
const NO_FLOORS: Floor[] = [];

/** A plan with every list (a hand-written file may leave some out). */
function normal(p: Plan): Plan {
  return {
    ...p,
    flights: p.flights ?? [],
    notes: p.notes ?? [],
    portals: p.portals ?? [],
    cargo: p.cargo ?? [],
    links: p.links ?? [],
    levels: p.levels ?? [],
    groups: p.groups ?? [],
    drawings: p.drawings ?? [],
  };
}

/** The flight plan dialogs (TELAS.md, quadro 20): the gallery (⌥M) and saving one (⌥⇧M). */
export function PlansDialogs() {
  const d = useSquad((s) => s.plansDialog);
  const close = () => useSquad.getState().setPlansDialog(null);
  if (d === 'gallery') return <Gallery onClose={close} />;
  if (d === 'save') return <SavePlan onClose={close} />;
  return null;
}

/** "PLN · IMP · REV · 1 NOTAM · 3 frequências". */
function composition(p: Plan): string {
  const parts = p.flights.map((f) => f.callsign || f.name);
  if (p.notes.length) parts.push(plural(p.notes.length, 'NOTAM', 'NOTAMs'));
  if (p.portals.length === 1) parts.push(`portal ${p.portals[0].url?.replace(/^https?:\/\//, '') || p.portals[0].name}`);
  else if (p.portals.length) parts.push(plural(p.portals.length, 'portal', 'portais'));
  if (p.cargo?.length) parts.push(plural(p.cargo.length, 'carga', 'cargas'));
  if (p.levels.length) parts.push(plural(p.levels.length, 'nível', 'níveis'));
  if (p.links.length) parts.push(plural(p.links.length, 'frequência', 'frequências'));
  if (p.groups.length) parts.push(plural(p.groups.length, 'grupo', 'grupos'));
  if (p.drawings.length) parts.push(plural(p.drawings.length, 'desenho', 'desenhos'));
  return parts.join(' · ');
}

function Gallery({ onClose }: { onClose: () => void }) {
  const [plans, setPlans] = useState<Plan[] | null>(null);
  const [pick, setPick] = useState<string | null>(null);
  const [agents, setAgents] = useState<AgentInfo[]>([]);
  const [busy, setBusy] = useState(false);
  const file = useRef<HTMLInputElement>(null);
  const load = (select?: string) =>
    hub.plans().then(
      (got) => {
        const list = got.map(normal);
        setPlans(list);
        setPick((cur) => select ?? (cur && list.some((p) => p.id === cur) ? cur : (list[0]?.id ?? null)));
      },
      (e: Error) => {
        setPlans([]);
        toast(e.message, 'err');
      },
    );
  useEffect(() => {
    void load();
    hub.agents().then(
      (d) => setAgents(d.agents),
      () => {},
    );
  }, []);
  const plan = plans?.find((p) => p.id === pick) ?? null;

  const importFile = async (f: File) => {
    try {
      const p = await hub.importPlan(await f.text());
      toast(`Plano ${p.name} importado: revise os comandos antes de decolar`);
      await load(p.id);
    } catch (e) {
      toast((e as Error).message, 'err');
    }
  };
  const exportPlan = (p: Plan) => {
    const blob = new Blob([JSON.stringify(p, null, 2)], { type: 'application/json' });
    const a = document.createElement('a');
    a.href = URL.createObjectURL(blob);
    a.download = `${p.id}.torre-plano.json`;
    a.click();
    URL.revokeObjectURL(a.href);
  };
  const remove = async (p: Plan) => {
    const ok = await confirmDialog({
      title: `Remover o plano ${p.name}?`,
      tone: 'danger',
      lose: ['O plano, nesta Torre (exporte antes, se quiser guardar o arquivo)'],
      keep: ['Os voos, NOTAMs, portais, grupos e desenhos que ele já pôs no ar'],
      action: 'Remover plano',
    });
    if (!ok) return;
    hub
      .removePlan(p.id)
      .then(() => load())
      .catch((e: Error) => toast(e.message, 'err'));
  };
  const takeOff = async () => {
    if (!plan) return;
    setBusy(true);
    try {
      const st = useSquad.getState();
      const res = await api.applyPlan(plan.id, { floor: st.activeFloor, commands: commandsOf(plan) });
      const flights = plan.flights.length;
      toast(`${plan.name} no ar: ${plural(res.nodes.length, 'item', 'itens')}${flights ? `, ${plural(flights, 'voo novo', 'voos novos')}` : ''}`);
      for (const w of res.warnings) toast(w, 'wait');
      onClose();
    } catch (e) {
      toast((e as Error).message, 'err');
    } finally {
      setBusy(false);
    }
  };

  const foot = plan ? (
    <>
      <span className="pl-foot-note mono">
        {plan.flights.length > 0 ? `${plural(plan.flights.length, 'voo novo', 'voos novos')}, conversas novas` : 'sem voos'}
        {plan.levels.length > 0 && ` · ${plural(plan.levels.length, 'nível novo', 'níveis novos')}`}
      </span>
      <span style={{ flex: 1 }} />
      <button className="btn" onClick={onClose}>
        Cancelar
      </button>
      <button className="btn filled" onClick={takeOff} disabled={busy}>
        <Icon name="flight_takeoff" size={16} />
        {busy ? 'Decolando…' : 'Decolar a equipe'}
      </button>
    </>
  ) : undefined;

  return (
    <Dialog icon="flight_class" title="Planos de voo" sub="Equipes salvas, para decolar de novo com conversas novas." onClose={onClose} wide foot={foot}>
      <div className="pl-top">
        <span className="label-caps">{plans ? plural(plans.length, 'plano', 'planos') : 'carregando…'}</span>
        <span style={{ flex: 1 }} />
        <input ref={file} type="file" accept=".json,application/json" hidden onChange={(e) => e.target.files?.[0] && void importFile(e.target.files[0])} />
        <button className="btn" onClick={() => file.current?.click()}>
          <Icon name="upload" size={15} />
          Importar
        </button>
      </div>
      <div className="pl-grid">
        {plans?.map((p) => (
          <div
            key={p.id}
            role="button"
            tabIndex={0}
            className={`pl-card${p.id === pick ? ' on' : ''}`}
            onClick={() => setPick(p.id)}
            onKeyDown={(e) => e.key === 'Enter' && setPick(p.id)}
          >
            <Miniature plan={p} />
            <b className="pl-name">
              {p.name}
              {p.imported && <span className="pl-tag">importado</span>}
            </b>
            <span className="pl-comp mono">{composition(p)}</span>
            <span className="pl-actions">
              <button className="icon-btn" title="Exportar o arquivo do plano" onClick={(e) => (e.stopPropagation(), exportPlan(p))}>
                <Icon name="download" size={15} />
              </button>
              <button className="icon-btn" title="Remover o plano" onClick={(e) => (e.stopPropagation(), void remove(p))}>
                <Icon name="delete" size={15} />
              </button>
            </span>
          </div>
        ))}
        <button className="pl-card pl-save" onClick={() => useSquad.getState().setPlansDialog('save')}>
          <Icon name="library_add" size={22} />
          <b>Salvar a seleção como plano</b>
          <span>
            Selecione voos, NOTAMs, portais ou um grupo no Espaço e use <span className="kbd">⌥⇧M</span>, ou guarde o nível ou a sessão inteira.
          </span>
        </button>
      </div>
      {plan && <Review plan={plan} agents={agents} />}
    </Dialog>
  );
}

/** The plan's own command lines (what the server asks the review for). */
function commandsOf(p: Plan): string[] {
  return [...new Set(p.flights.map((f) => f.command?.trim() ?? '').filter(Boolean))];
}

/**
 * ANTES DE DECOLAR: what each flight will run. An agent runs the command set
 * in Configurações → Agentes; a flight with its own command line runs it as
 * written, which an imported plan could have put there.
 */
function Review({ plan, agents }: { plan: Plan; agents: AgentInfo[] }) {
  const roleOf = useRoleLookup();
  const levels = useSquad((s) => s.workspace?.floors) ?? NO_FLOORS;
  const onLevel = useSquad((s) => s.activeFloor);
  const here = onLevel ? flightLevel(levels.find((f) => f.id === onLevel)) : 'PISTA';
  return (
    <div className="pl-review">
      <span className="label-caps">Antes de decolar · {plan.name}</span>
      <ul className="pl-lines">
        {plan.flights.map((f) => {
          const role = roleOf(f.role);
          const own = f.command?.trim();
          const line = own || agents.find((a) => a.agent === f.agent)?.defaultCommand || f.agent;
          return (
            <li key={f.key} className={own ? 'own' : ''}>
              <i style={{ background: `var(--r-${roleColor(role)})` }} />
              <b className="mono">{f.callsign || f.name}</b>
              <code className="mono">{f.agent === 'shell' && !own ? 'shell' : line}</code>
              <span className="pl-line-sub">
                {f.name}
                {role ? ` · ${role.name}` : f.role ? ` · papel ${f.role} (não existe aqui)` : ''}
                {f.level ? ` · nível ${plan.levels.find((l) => l.key === f.level)?.name ?? ''}` : ''}
                {own ? ' · comando próprio do plano' : ''}
              </span>
            </li>
          );
        })}
      </ul>
      {plan.levels.length > 0 && (
        <p className="pl-hint">
          Cria {plural(plan.levels.length, 'nível', 'níveis')}: {plan.levels.map((l) => l.name).join(', ')} (cópias novas do repositório, com o nome do plano). O resto
          vai para {here}, ao lado do que já está lá.
        </p>
      )}
      {plan.levels.length === 0 && <p className="pl-hint">Vai para {here}, ao lado do que já está lá. Nomes que já existem ganham um número.</p>}
      {plan.imported && commandsOf(plan).length > 0 ? (
        <p className="pl-warn">
          <Icon name="warning" size={15} />
          Este plano veio de um arquivo e traz comandos próprios. Eles rodam como estão escritos acima: confira cada um antes de decolar.
        </p>
      ) : (
        <p className="pl-hint">Um plano importado pode trazer comandos próprios. Eles aparecem aqui antes de rodar.</p>
      )}
    </div>
  );
}

/** A drawing of the plan: its items where they sit, levels side by side. */
function Miniature({ plan }: { plan: Plan }) {
  const roleOf = useRoleLookup();
  const { items, lines, frames, view } = useMemo(() => {
    const levels = ['', ...plan.levels.map((l) => l.key)];
    type It = { key: string; x: number; y: number; w: number; h: number; kind: 'f' | 'n' | 'p' | 'c'; label: string; level: string; color?: string };
    const all: It[] = [
      ...plan.flights.map((f) => ({ ...f, kind: 'f' as const, label: f.callsign || f.name, level: f.level ?? '', color: roleColor(roleOf(f.role)) })),
      ...plan.notes.map((n) => ({ ...n, kind: 'n' as const, label: 'NOTAM', level: n.level ?? '' })),
      ...plan.portals.map((o) => ({ ...o, kind: 'p' as const, label: '⊕', level: o.level ?? '' })),
      ...(plan.cargo ?? []).map((o) => ({ ...o, kind: 'c' as const, label: 'CARGA', level: o.level ?? '' })),
    ].map((it) => ({ ...it, w: it.w || 400, h: it.h || 300 }));
    // Groups and drawings: the frames behind (a drawing as its box).
    type Fr = { key: string; x: number; y: number; w: number; h: number; kind: 'g' | 'd'; level: string; color?: string };
    const backs: Fr[] = [
      ...plan.groups.map((g) => ({ ...g, kind: 'g' as const, level: g.level ?? '' })),
      ...plan.drawings.map((d) => ({ key: d.key, x: d.x, y: d.y, w: d.w, h: d.h, kind: 'd' as const, level: d.level ?? '', color: d.color })),
    ];
    // Each level is a column after the previous one.
    let x0 = 0;
    const placed: It[] = [];
    const frames: Fr[] = [];
    for (const l of levels) {
      const mine = all.filter((it) => it.level === l);
      const back = backs.filter((f) => f.level === l);
      if (!mine.length && !back.length) continue;
      const right = Math.max(...mine.map((it) => it.x + it.w), ...back.map((f) => f.x + f.w));
      for (const it of mine) placed.push({ ...it, x: it.x + x0 });
      for (const f of back) frames.push({ ...f, x: f.x + x0 });
      x0 += right + 300;
    }
    const at = new Map(placed.map((it) => [it.key, it]));
    const lines = plan.links.flatMap(([a, b]) => {
      const p = at.get(a);
      const q = at.get(b);
      return p && q ? [{ k: `${a}-${b}`, x1: p.x + p.w / 2, y1: p.y + p.h / 2, x2: q.x + q.w / 2, y2: q.y + q.h / 2 }] : [];
    });
    const w = Math.max(1, ...placed.map((it) => it.x + it.w), ...frames.map((f) => f.x + f.w));
    const h = Math.max(1, ...placed.map((it) => it.y + it.h), ...frames.map((f) => f.y + f.h));
    const pad = 120;
    return { items: placed, lines, frames, view: `${-pad} ${-pad} ${w + 2 * pad} ${h + 2 * pad}` };
  }, [plan, roleOf]);
  return (
    <svg className="pl-mini" viewBox={view} preserveAspectRatio="xMidYMid meet" aria-hidden>
      {frames.map((f) => (
        <rect key={f.key} x={f.x} y={f.y} width={f.w} height={f.h} className={`pl-mini-${f.kind}`} style={{ stroke: colorVar(f.color) }} />
      ))}
      {lines.map((l) => (
        <line key={l.k} x1={l.x1} y1={l.y1} x2={l.x2} y2={l.y2} className="pl-mini-wire" />
      ))}
      {items.map((it) => (
        <g key={it.key}>
          <rect x={it.x} y={it.y} width={it.w} height={it.h} className={`pl-mini-${it.kind}`} />
          {it.kind === 'f' && <rect x={it.x} y={it.y} width={40} height={it.h} style={{ fill: `var(--r-${it.color})` }} />}
          <text x={it.x + (it.kind === 'f' ? 70 : 30)} y={it.y + 130} className={`pl-mini-t pl-mini-t-${it.kind}`}>
            {it.label}
          </text>
        </g>
      ))}
    </svg>
  );
}

/** Saves the selection, the level on screen or the whole session as a plan. */
function SavePlan({ onClose }: { onClose: () => void }) {
  const selected = useSquad((s) => s.selectedIds);
  const floor = useSquad((s) => s.activeFloor);
  const nodes = useSquad((s) => s.workspace?.nodes);
  const levels = useSquad((s) => s.workspace?.floors);
  const onLevel = (nodes ?? []).filter((n) => (n.floor ?? '') === floor).length;
  const all = nodes?.length ?? 0;
  const here = floor ? flightLevel(levels?.find((f) => f.id === floor)) : 'a pista';
  const [scope, setScope] = useState<'selection' | 'level' | 'session'>(selected.length > 0 ? 'selection' : 'level');
  const [name, setName] = useState('');
  const [content, setContent] = useState(true);
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);
  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    setBusy(true);
    setError('');
    try {
      const p = await api.savePlan({ name: name.trim(), scope, nodes: selected, floor, notesContent: content });
      toast(`Plano ${p.name} salvo: ${composition(p)}`);
      useSquad.getState().setPlansDialog('gallery');
    } catch (err) {
      setError((err as Error).message);
    } finally {
      setBusy(false);
    }
  };
  const opt = (value: typeof scope, label: string, n: number) => (
    <label className={`pl-opt${scope === value ? ' on' : ''}${n === 0 ? ' off' : ''}`}>
      <input type="radio" name="scope" checked={scope === value} disabled={n === 0} onChange={() => setScope(value)} />
      <span>{label}</span>
      <span className="mono pl-opt-n">{plural(n, 'item', 'itens')}</span>
    </label>
  );
  return (
    <Dialog
      icon="library_add"
      title="Salvar como plano de voo"
      sub="Guarda voos (agente, papel, nome, posição), NOTAMs, portais, frequências, níveis, grupos e desenhos. Um grupo selecionado leva o que está dentro dele."
      onClose={onClose}
    >
      <form className="pl-form" onSubmit={submit}>
        <label className="field">
          Nome
          <input value={name} maxLength={60} autoFocus placeholder="Ex.: Trio clássico" onChange={(e) => setName(e.target.value)} />
        </label>
        <div className="pl-opts" role="radiogroup" aria-label="O que guardar">
          {opt('selection', 'A seleção', selected.length)}
          {opt('level', `Tudo em ${here}`, onLevel)}
          {opt('session', 'A sessão inteira, com os níveis', all)}
        </div>
        <label className="pl-check">
          <input type="checkbox" checked={content} onChange={(e) => setContent(e.target.checked)} />
          Guardar o conteúdo das NOTAMs (senão, só o nome)
        </label>
        <p className="field-hint">As conversas não vão junto: o plano decola com conversas novas. Ele fica nesta Torre, para qualquer workspace, e pode ser exportado.</p>
        {error && <p className="fl-error">{error}</p>}
        <div className="fl-actions">
          <button type="button" className="btn" onClick={onClose}>
            Cancelar
          </button>
          <button className="btn filled" disabled={busy || !name.trim()}>
            {busy ? 'Salvando…' : 'Salvar plano'}
          </button>
        </div>
      </form>
    </Dialog>
  );
}
