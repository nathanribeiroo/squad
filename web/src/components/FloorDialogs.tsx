import { useEffect, useMemo, useState } from 'react';
import { ApiError, api } from '../api/client';
import type { Floor, FloorChanges, LandResult, PermissionRule, WNode } from '../api/types';
import { CHECK_NAMES, GROUND, checkViews, flightLevel, plural, slugify, useBranch, type CheckView } from '../lib/floors';
import { callsign } from '../lib/model';
import { shortPath } from '../lib/paths';
import { toast } from '../lib/toast';
import { isVisual } from '../lib/visual';
import { useSquad } from '../state/store';
import { expectGone } from './FloorBar';
import { Dialog } from './Dialog';
import { Icon } from './Icon';
import './FloorDialogs.css';

/** The floor dialog open (FloorDialog in the store), if any. */
export function FloorDialogs() {
  const d = useSquad((s) => s.floorDialog);
  const floor = useSquad((s) => (d && 'id' in d ? s.workspace?.floors?.find((f) => f.id === d.id) : undefined));
  const copy = useSquad((s) => s.workspace?.copy);
  const close = () => useSquad.getState().setFloorDialog(null);
  if (!d) return null;
  if (d.kind === 'create') return <CreateFloor onClose={close} />;
  if (d.kind === 'merge-copy' || d.kind === 'copy-log') {
    if (!copy) return null; // another session opened meanwhile
    if (d.kind === 'merge-copy') return <MergeCopy copy={copy} onClose={close} />;
    return <SetupLog floor={copy} title={`Preparo da cópia ${copy.branch}`} load={api.copyLog} onClose={close} />;
  }
  if (!floor) return null; // landed meanwhile
  if (d.kind === 'land') return <LandFloor key={floor.id} floor={floor} onClose={close} />;
  if (d.kind === 'discard') return <DiscardFloor key={floor.id} floor={floor} onClose={close} />;
  if (d.kind === 'rename') return <RenameFloor key={floor.id} floor={floor} onClose={close} />;
  return <SetupLog key={floor.id} floor={floor} title={`Preparo · ${flightLevel(floor)} · ${floor.name}`} load={() => api.floorLog(floor.id)} onClose={close} />;
}

/** Patterns typed as "a, b" or one per line. */
function patterns(s: string): string[] {
  return s
    .split(/[,\n]/)
    .map((p) => p.trim())
    .filter(Boolean);
}

function CreateFloor({ onClose }: { onClose: () => void }) {
  const ground = useBranch('');
  const config = useSquad((s) => s.workspace?.floorConfig) ?? { copy: ['.env*'], setup: '' };
  const level = useSquad((s) => `FL${(s.workspace?.floorSeq ?? Math.max(0, ...(s.workspace?.floors ?? []).map((f) => f.number ?? 0))) + 1}`);
  const [name, setName] = useState('');
  const [branch, setBranch] = useState('');
  const [branchEdited, setBranchEdited] = useState(false);
  const [branches, setBranches] = useState<string[]>([]);
  const [prep, setPrep] = useState(false);
  const [copy, setCopy] = useState(config.copy.join(', '));
  const [setup, setSetup] = useState(config.setup ?? '');
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);
  useEffect(() => {
    api.branches().then(
      (r) => setBranches(r.branches),
      () => setBranches([]),
    );
  }, []);

  const target = branchEdited ? branch.trim() : slugify(name);
  const existing = !!target && branches.includes(target);
  const configChanged = patterns(copy).join('\n') !== config.copy.join('\n') || setup.trim() !== (config.setup ?? '');

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    setBusy(true);
    setError('');
    try {
      if (configChanged) await api.floorConfig({ copy: patterns(copy), setup: setup.trim() });
      const f = await api.createFloor({ name: name.trim(), branch: branchEdited ? target : undefined, existing });
      useSquad.getState().setFloor(f.id);
      toast(`Nível ${f.name} criado: branch ${f.branch}`);
      onClose();
    } catch (err) {
      setError((err as Error).message);
    } finally {
      setBusy(false);
    }
  };

  return (
    <Dialog icon="layers" title="Novo nível de voo" sub="Uma cópia isolada do repositório, num branch próprio, com um canvas próprio." onClose={onClose}>
      <form className="fl-form" onSubmit={submit}>
        <div className="fl-row">
        <div className="field fl-level">
          Nível
          <span className="fl-level-box mono">{level}</span>
        </div>
        <label className="field">
          Nome
          <input value={name} maxLength={40} autoFocus placeholder="Ex.: Página de login" onChange={(e) => setName(e.target.value)} />
        </label>
        <label className="field">
          Branch
          <input
            className="mono"
            value={branchEdited ? branch : target}
            placeholder="o nome, sem acentos"
            onChange={(e) => {
              setBranchEdited(true);
              setBranch(e.target.value);
            }}
          />
        </label>
        </div>
        <p className="field-hint">
          {existing ? (
            <>
              O branch <b className="mono">{target}</b> já existe: o nível abre esse branch como está.
            </>
          ) : (
            <>
              Parte do último commit de <b className="mono">{ground || GROUND}</b>. O que ainda não foi commitado fica na pista.
            </>
          )}
        </p>
        <button type="button" className="fl-toggle" onClick={() => setPrep(!prep)} aria-expanded={prep}>
          <Icon name={prep ? 'expand_more' : 'chevron_right'} size={18} />
          Preparação
          <span className="fl-toggle-sub mono">
            {config.copy.length ? config.copy.join(', ') : 'nada copiado'}
            {config.setup ? ` · ${config.setup}` : ''}
          </span>
        </button>
        {prep && (
          <div className="fl-prep">
            <label className="field">
              Copiar da pista (arquivos fora do git)
              <input className="mono" value={copy} placeholder=".env*, config/local.json" onChange={(e) => setCopy(e.target.value)} />
            </label>
            <label className="field">
              Comando de preparo
              <input className="mono" value={setup} placeholder="ex.: npm install" onChange={(e) => setSetup(e.target.value)} />
            </label>
            <p className="field-hint">
              Valem para os próximos níveis deste workspace. Os caminhos partem da pasta do workspace; o comando roda na cópia, no shell padrão, depois da cópia dos
              arquivos.
            </p>
          </div>
        )}
        {error && <p className="fl-error">{error}</p>}
        <div className="fl-actions">
          <button type="button" className="btn" onClick={onClose}>
            Cancelar
          </button>
          <button className="btn filled" disabled={busy || !name.trim() || !target}>
            {busy ? 'Criando…' : 'Criar nível'}
          </button>
        </div>
      </form>
    </Dialog>
  );
}

/** Another name for a level; the agents use the new one from then on. */
function RenameFloor({ floor, onClose }: { floor: Floor; onClose: () => void }) {
  const [name, setName] = useState(floor.name);
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);
  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (name.trim() === floor.name) return onClose();
    setBusy(true);
    setError('');
    try {
      const f = await api.renameFloor(floor.id, name.trim());
      toast(`${flightLevel(f)} agora se chama ${f.name}`);
      onClose();
    } catch (err) {
      setError((err as Error).message);
    } finally {
      setBusy(false);
    }
  };
  return (
    <Dialog icon="edit" title={`Renomear ${flightLevel(floor)}`} sub={floor.name} onClose={onClose}>
      <form className="fl-form" onSubmit={submit}>
        <label className="field">
          Nome
          <input value={name} maxLength={40} autoFocus onFocus={(e) => e.currentTarget.select()} onChange={(e) => setName(e.target.value)} />
        </label>
        <p className="field-hint">
          O branch <b className="mono">{floor.branch}</b> e a pasta continuam os mesmos. Os agentes passam a chamar o nível pelo nome novo.
        </p>
        {error && <p className="fl-error">{error}</p>}
        <div className="fl-actions">
          <button type="button" className="btn" onClick={onClose}>
            Cancelar
          </button>
          <button className="btn filled" disabled={busy || !name.trim()}>
            {busy ? 'Renomeando…' : 'Renomear'}
          </button>
        </div>
      </form>
    </Dialog>
  );
}

/** What landing or discarding closes: terminals and portals; notes go down to the ground floor. */
function useFloorNodes(id: string): { closes: WNode[]; notes: WNode[] } {
  const nodes = useSquad((s) => s.workspace?.nodes);
  return useMemo(() => {
    const on = (nodes ?? []).filter((n) => n.floor === id);
    // Groups and drawings go with the floor without a word.
    return { closes: on.filter((n) => n.kind !== 'note' && !isVisual(n)), notes: on.filter((n) => n.kind === 'note') };
  }, [nodes, id]);
}

function label(n: WNode): string {
  return n.terminal?.name ?? n.note?.title ?? (n.portal ? `o portal ${n.portal.name || 'Portal'}` : n.id);
}

function list(items: string[]): string {
  return items.length <= 1 ? (items[0] ?? '') : `${items.slice(0, -1).join(', ')} e ${items[items.length - 1]}`;
}

function Files({ files, max = 8 }: { files: { path: string; status?: string }[]; max?: number }) {
  return (
    <ul className="fl-files mono">
      {files.slice(0, max).map((f) => (
        <li key={f.path}>
          {f.status && <span className="fl-file-st">{f.status.trim() || '·'}</span>}
          {f.path}
        </li>
      ))}
      {files.length > max && <li className="fl-more">e mais {files.length - max}</li>}
    </ul>
  );
}

/** A floor's changes (id), or the session copy's (id null). */
function useChanges(id: string | null): [FloorChanges | null, string] {
  const [changes, setChanges] = useState<FloorChanges | null>(null);
  const [error, setError] = useState('');
  useEffect(() => {
    (id === null ? api.copyChanges() : api.floorChanges(id)).then(setChanges, (e: Error) => setError(e.message));
  }, [id]);
  return [changes, error];
}

/** The branches a worktree's branch can merge into. */
function useTargets(branch: string): string[] {
  const [branches, setBranches] = useState<string[]>([]);
  useEffect(() => {
    api.branches().then(
      (r) => setBranches(r.branches.filter((b) => b !== branch)),
      () => setBranches([]),
    );
  }, [branch]);
  return branches;
}

/** The commits a floor or a copy has, and those its base got meanwhile. */
function Commits({ changes, base, what }: { changes: FloorChanges; base?: string; what: [string, string, string] }) {
  return (
    <section className="fl-section">
      <div className="fl-section-title">
        <Icon name="commit" size={16} />
        {changes.ahead ? plural(changes.ahead, what[0], what[1]) : what[2]}
        {changes.behind > 0 && (
          <span className="fl-muted">
            · {base || 'a origem'} tem {plural(changes.behind, 'commit novo', 'commits novos')}
          </span>
        )}
      </div>
      {changes.commits.length > 0 && (
        <ul className="fl-commits mono">
          {changes.commits.map((c) => (
            <li key={c.hash}>
              <span className="fl-hash">{c.hash}</span> {c.subject}
            </li>
          ))}
        </ul>
      )}
    </section>
  );
}

/**
 * The checks before landing (docs/torre/TELAS.md, quadro 14): everything
 * committed, flights still in the air (a warning, not a block) and how far
 * the base moved, and the conflicts a merge into the base would have (git
 * merge-tree, before trying; checked again when landing).
 */
function LandChecks({ changes, base, flying, checks }: { changes: FloorChanges; base?: string; flying: string[]; checks?: CheckView[] }) {
  const clean = changes.files.length === 0;
  const conflicts = changes.conflicts;
  return (
    <div className={`fl-checks${checks?.length === 2 ? ' six' : ''}`}>
      {checks?.map((c) => (
        <div key={c.kind} className={`fl-check ${CHECK_LOOK[c.state]}`} title={c.detail}>
          <b>{c.state === 'none' ? CHECK_NAMES[c.kind] : c.label}</b>
          <span>{c.state === 'none' ? 'sem marca' : c.state === 'stale' ? 'commit antigo · marcar de novo' : c.detail.split(' · ')[0]}</span>
        </div>
      ))}
      {!conflicts ? (
        <div className="fl-check idle">
          <b>conflitos</b>
          <span>checados ao pousar</span>
        </div>
      ) : conflicts.length === 0 ? (
        <div className="fl-check ok">
          <b>✓ sem conflitos</b>
          <span>com {base || 'a origem'}</span>
        </div>
      ) : (
        <div className="fl-check err" title={conflicts.join('\n')}>
          <b>✕ {plural(conflicts.length, 'conflito', 'conflitos')}</b>
          <span className="mono">
            {conflicts.slice(0, 2).join(', ')}
            {conflicts.length > 2 ? ` +${conflicts.length - 2}` : ''}
          </span>
        </div>
      )}
      <div className={`fl-check ${clean ? 'ok' : 'err'}`}>
        <b>{clean ? '✓ tudo commitado' : `✕ ${plural(changes.files.length, 'alteração', 'alterações')}`}</b>
        <span>{clean ? '0 alterações' : 'commite antes de pousar'}</span>
      </div>
      <div className={`fl-check ${flying.length ? 'wait' : 'ok'}`}>
        <b>{flying.length ? `⧗ ${plural(flying.length, 'voo no ar', 'voos no ar')}` : '✓ nenhum voo no ar'}</b>
        <span>{flying.length ? `${flying.join(', ')} ${flying.length > 1 ? 'fecham' : 'fecha'}` : 'nada é interrompido'}</span>
      </div>
      <div className="fl-check idle">
        <b>
          {base || 'origem'} +{changes.behind}
        </b>
        <span>{changes.behind === 1 ? 'commit novo' : 'commits novos'}</span>
      </div>
    </div>
  );
}

const RULE_KINDS: Record<PermissionRule['kind'], string> = { allow: 'permitir', ask: 'perguntar', deny: 'negar' };

/** The agent whose permissions file it is. */
function ruleAgent(file: string): string {
  return file.startsWith('.claude/') ? 'Claude' : file.startsWith('.devin/') ? 'Devin' : file;
}

/**
 * The permission rules the agents saved on a floor (or in the session's
 * copy) that its ground floor lacks, and the box that takes them there. It
 * starts unchecked: a flight can write those files too, so the user reads
 * the rules before they apply to the whole project.
 */
function NewRules({ rules, take, onTake, where }: { rules: PermissionRule[]; take: boolean; onTake: (t: boolean) => void; where: string }) {
  if (rules.length === 0) return null;
  return (
    <section className="fl-section">
      <div className="fl-section-title">
        <Icon name="shield" size={16} />
        {plural(rules.length, 'permissão nova', 'permissões novas')}
        <span className="fl-muted">· liberadas aqui com “sempre”, fora do git</span>
      </div>
      <ul className="fl-rules mono">
        {rules.map((r) => (
          <li key={`${r.file} ${r.kind} ${r.rule}`} title={r.file}>
            <span className={`fl-rule-kind ${r.kind}`}>{RULE_KINDS[r.kind]}</span>
            <span className="fl-rule-agent">{ruleAgent(r.file)}</span>
            {r.rule}
          </li>
        ))}
      </ul>
      <label className={`fl-option${take ? ' on' : ''}`}>
        <input type="checkbox" checked={take} onChange={(e) => onTake(e.target.checked)} />
        <span>
          <b>Levar essas permissões para {where}</b>
          <span className="fl-option-sub">Os agentes de lá deixam de perguntar por elas. Confira antes: um voo também pode escrever esses arquivos.</span>
        </span>
      </label>
    </section>
  );
}

/** "· 2 permissões levadas", for a land's toast. */
function taken(r: LandResult): string {
  return r.permissions ? ` · ${plural(r.permissions, 'permissão levada', 'permissões levadas')}` : '';
}

// How a floor's marks (tests, review, security, design) look among the land checks.
const CHECK_LOOK = { pass: 'ok', fail: 'err', stale: 'wait', none: 'idle' } as const;

/** "Mesclar em [branch]": the target, among the branches it can merge into. */
function IntoSelect({ into, branches, onChange, onClick }: { into: string; branches: string[]; onChange: (b: string) => void; onClick?: () => void }) {
  return (
    <select className="mono" value={into} onChange={(e) => onChange(e.target.value)} onClick={onClick}>
      {!into && <option value="">escolha um branch</option>}
      {(branches.includes(into) || !into ? branches : [into, ...branches]).map((b) => (
        <option key={b} value={b}>
          {b}
        </option>
      ))}
    </select>
  );
}

type Refusal = { message: string; files?: string[]; conflicts?: string[]; folder?: string; branch?: string };

function refusalOf(e: unknown): Refusal {
  const body = e instanceof ApiError ? e.body : {};
  return {
    message: (e as Error).message,
    files: body.files as string[] | undefined,
    conflicts: body.conflicts as string[] | undefined,
    folder: body.folder as string | undefined,
    branch: body.branch as string | undefined,
  };
}

/** Why a land or a merge was refused; nothing changed. who is "do nível" or
 * "da sessão", again what to do after fixing it. */
function RefusalView({ r, into, branch, who, again }: { r: Refusal; into: string; branch: string; who: string; again: string }) {
  return (
    <div className="fl-error">
      {r.conflicts ? (
        <>
          <b>Conflito ao mesclar em {into}. Nada mudou.</b> Peça a um voo {who} para mesclar {into} no branch {branch}, resolver e commitar; depois {again}.
          <Files files={r.conflicts.map((path) => ({ path }))} />
        </>
      ) : r.folder ? (
        <>
          <b>
            A pasta <span className="mono">{shortPath(r.folder)}</span>, onde o branch {r.branch} está aberto, tem alterações sem commit. Nada mudou.
          </b>{' '}
          Commite ou descarte essas alterações lá (ou escolha outro branch); depois {again}.
          {r.files && <Files files={r.files.map((path) => ({ path }))} />}
        </>
      ) : (
        <>
          {r.message}
          {r.files && <Files files={r.files.map((path) => ({ path }))} />}
        </>
      )}
    </div>
  );
}

function LandFloor({ floor, onClose }: { floor: Floor; onClose: () => void }) {
  const [changes, loadError] = useChanges(floor.id);
  const branches = useTargets(floor.branch);
  const [mode, setMode] = useState<'merge' | 'keep'>('merge');
  const [into, setInto] = useState(floor.base ?? '');
  const [take, setTake] = useState(false);
  const [refusal, setRefusal] = useState<Refusal | null>(null);
  const [busy, setBusy] = useState(false);
  const { closes, notes } = useFloorNodes(floor.id);
  const runtime = useSquad((s) => s.runtime);
  const head = useSquad((s) => s.floorStatus?.floors[floor.id]?.head);
  const flying = closes.filter((n) => n.terminal && ['work', 'wait'].includes(runtime[n.id]?.activity ?? '')).map((n) => callsign(n.terminal!));
  // Nothing to merge: keeping the branch is the obvious choice.
  useEffect(() => {
    if (changes && changes.ahead === 0) setMode('keep');
  }, [changes]);

  const dirty = (changes?.files.length ?? 0) > 0;
  const canMerge = !!into;

  const land = async () => {
    setBusy(true);
    setRefusal(null);
    const undo = expectGone(floor.id);
    try {
      const permissions = take && (changes?.rules.length ?? 0) > 0;
      const r = await api.landFloor(floor.id, mode === 'merge' ? { into, permissions } : { permissions });
      toast(
        (r.merged
          ? `${floor.name} pousou: ${r.branch} em ${r.into} (${(r.commit ?? '').slice(0, 7)})`
          : r.upToDate
            ? `${floor.name} pousou: ${r.into} já tinha tudo de ${r.branch}`
            : `${floor.name} pousou; o trabalho continua no branch ${r.branch}`) + taken(r),
      );
      onClose();
    } catch (e) {
      undo();
      setRefusal(refusalOf(e));
    } finally {
      setBusy(false);
    }
  };

  const target = mode === 'merge' ? into : '';
  return (
    <Dialog
      icon="flight_land"
      title={`Autorizar pouso · ${flightLevel(floor)}`}
      sub={
        <span className="mono">
          {floor.name} · {floor.branch}
          {target && ` → ${target}`}
        </span>
      }
      onClose={onClose}
      wide
      foot={
        <>
          <span className="fl-who mono">quem pode: você, ou o líder que criou o nível</span>
          <span className="tn-spacer" />
          <button className="btn" onClick={onClose}>
            Cancelar
          </button>
          <button className="btn filled" disabled={busy || !changes || dirty || (mode === 'merge' && !canMerge)} onClick={land}>
            <Icon name={mode === 'merge' ? 'call_merge' : 'flight_land'} />
            {busy ? 'Pousando…' : mode === 'merge' ? 'Mesclar e pousar' : 'Pousar'}
          </button>
        </>
      }
    >
      {loadError && <p className="fl-error">{loadError}</p>}
      {!changes && !loadError && <p className="field-hint">Lendo o nível…</p>}
      {changes && (
        <>
          {changes.missing && <p className="fl-warn">A pasta do nível sumiu. Pousar só limpa o registro; os commits do branch continuam.</p>}
          <LandChecks changes={changes} base={floor.base} flying={flying} checks={checkViews(floor, head)} />
          <Commits changes={changes} base={floor.base} what={['commit do nível', 'commits do nível', 'Nenhum commit novo no nível']} />
          {dirty && (
            <div className="fl-warn">
              <b>{plural(changes.files.length, 'alteração não commitada', 'alterações não commitadas')}.</b> Commite antes de pousar (peça a um voo do nível), ou
              descarte o nível.
              <Files files={changes.files} />
            </div>
          )}
          <section className="fl-options" role="radiogroup">
            <label className={`fl-option${mode === 'merge' ? ' on' : ''}`}>
              <input type="radio" checked={mode === 'merge'} onChange={() => setMode('merge')} />
              <span>
                <b>Mesclar em</b> <IntoSelect into={into} branches={branches} onChange={setInto} onClick={() => setMode('merge')} />
                <span className="fl-option-sub">Commit de merge. Se o branch estiver aberto numa pasta, ela precisa estar sem alterações.</span>
              </span>
            </label>
            <label className={`fl-option${mode === 'keep' ? ' on' : ''}`}>
              <input type="radio" checked={mode === 'keep'} onChange={() => setMode('keep')} />
              <span>
                <b>Só guardar o branch</b>
                <span className="fl-option-sub">Nada é mesclado; o trabalho continua em {floor.branch}.</span>
              </span>
            </label>
          </section>
          <NewRules rules={changes.rules} take={take} onTake={setTake} where="a pista" />
          <p className="field-hint">
            Ao pousar: {closes.length ? `fecha ${list(closes.map(label))}; ` : ''}
            {notes.length ? `${list(notes.map(label))} ${notes.length > 1 ? 'descem' : 'desce'} para a pista; ` : ''}a pasta {shortPath(floor.cwd)} é apagada e o
            branch {floor.branch} continua.
          </p>
          {refusal && <RefusalView r={refusal} into={target} branch={floor.branch} who="do nível" again="peça o pouso de novo" />}
        </>
      )}
    </Dialog>
  );
}

/** Merges the session's branch, keeping its copy: the session goes on working
 * there and can merge again. */
function MergeCopy({ copy, onClose }: { copy: Floor; onClose: () => void }) {
  const [changes, loadError] = useChanges(null);
  const branches = useTargets(copy.branch);
  const [into, setInto] = useState(copy.base ?? '');
  const [take, setTake] = useState(false);
  const [refusal, setRefusal] = useState<Refusal | null>(null);
  const [busy, setBusy] = useState(false);
  const dirty = (changes?.files.length ?? 0) > 0;

  const merge = async () => {
    setBusy(true);
    setRefusal(null);
    try {
      const r = await api.mergeCopy(into, take && (changes?.rules.length ?? 0) > 0);
      toast(
        (r.upToDate ? `${r.into} já tinha tudo de ${r.branch}` : `A sessão pousou: ${r.branch} em ${r.into} (${(r.commit ?? '').slice(0, 7)}); ela continua na cópia`) +
          taken(r),
      );
      onClose();
    } catch (e) {
      setRefusal(refusalOf(e));
    } finally {
      setBusy(false);
    }
  };

  return (
    <Dialog
      icon="call_merge"
      title="Autorizar pouso · sessão"
      sub={
        <span className="mono">
          ⎇ {copy.branch}
          {into && ` → ${into}`}
        </span>
      }
      onClose={onClose}
      wide
      foot={
        <>
          <span className="tn-spacer" />
          <button className="btn" onClick={onClose}>
            Cancelar
          </button>
          <button className="btn filled" disabled={busy || !changes || dirty || !into || changes.missing} onClick={merge}>
            <Icon name="flight_land" />
            {busy ? 'Pousando…' : 'Mesclar e pousar'}
          </button>
        </>
      }
    >
      {loadError && <p className="fl-error">{loadError}</p>}
      {!changes && !loadError && <p className="field-hint">Lendo a cópia…</p>}
      {changes && (
        <>
          {changes.missing && <p className="fl-warn">A pasta da cópia sumiu. Os commits do branch {copy.branch} continuam no repositório.</p>}
          <Commits changes={changes} base={copy.base} what={['commit da sessão', 'commits da sessão', 'Nenhum commit novo na sessão']} />
          {dirty && (
            <div className="fl-warn">
              <b>{plural(changes.files.length, 'alteração não commitada', 'alterações não commitadas')}.</b> Commite antes de pousar (peça a um voo da sessão).
              <Files files={changes.files} />
            </div>
          )}
          <section className="fl-options">
            <div className="fl-option on">
              <span>
                <b>Mesclar em</b> <IntoSelect into={into} branches={branches} onChange={setInto} />
                <span className="fl-option-sub">Commit de merge. Se o branch estiver aberto numa pasta (a do projeto, por exemplo), ela precisa estar sem alterações.</span>
              </span>
            </div>
          </section>
          <NewRules rules={changes.rules} take={take} onTake={setTake} where="a pasta do projeto" />
          <p className="field-hint">
            A cópia <span className="mono">{shortPath(copy.cwd)}</span> continua: a sessão segue trabalhando no branch {copy.branch} e pode mesclar de novo.
          </p>
          {refusal && <RefusalView r={refusal} into={into} branch={copy.branch} who="da sessão" again="peça o pouso de novo" />}
        </>
      )}
    </Dialog>
  );
}

function DiscardFloor({ floor, onClose }: { floor: Floor; onClose: () => void }) {
  const [changes, loadError] = useChanges(floor.id);
  const [sure, setSure] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const { closes, notes } = useFloorNodes(floor.id);
  const loses = !!changes && (changes.files.length > 0 || (!!floor.newBranch && changes.ahead > 0));

  const discard = async () => {
    setBusy(true);
    setError('');
    const undo = expectGone(floor.id);
    try {
      await api.discardFloor(floor.id);
      toast(`Nível ${floor.name} descartado`);
      onClose();
    } catch (e) {
      undo();
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  };

  return (
    <Dialog
      icon="delete"
      title={`Descartar ${flightLevel(floor)}?`}
      sub="Apaga a cópia e o que há nela. Não dá para desfazer."
      tone="danger"
      onClose={onClose}
      foot={
        <>
          <span className="tn-spacer" />
          <button className="btn" onClick={onClose}>
            Cancelar
          </button>
          <button className="btn danger" disabled={busy || !changes || (loses && !sure)} onClick={discard}>
            <Icon name="delete" />
            {busy ? 'Descartando…' : 'Descartar nível'}
          </button>
        </>
      }
    >
      {loadError && <p className="fl-error">{loadError}</p>}
      {changes && (
        <ul className="fl-consequences">
          <li>
            Apaga a pasta <span className="mono">{shortPath(floor.cwd)}</span>
            {changes.files.length > 0 && <b>, com {plural(changes.files.length, 'alteração não commitada', 'alterações não commitadas')}</b>}.
          </li>
          {closes.length > 0 && <li>Fecha {list(closes.map(label))}.</li>}
          {notes.length > 0 && (
            <li>
              {list(notes.map(label))} {notes.length > 1 ? 'descem' : 'desce'} para a pista.
            </li>
          )}
          <li>
            {floor.newBranch ? (
              <>
                Apaga o branch <span className="mono">{floor.branch}</span>
                {changes.ahead > 0 && <b>, com {plural(changes.ahead, 'commit que não está', 'commits que não estão')} em {floor.base || 'outro branch'}</b>}.
              </>
            ) : (
              <>
                O branch <span className="mono">{floor.branch}</span> já existia e continua.
              </>
            )}
          </li>
        </ul>
      )}
      {changes && changes.files.length > 0 && <Files files={changes.files} />}
      {loses && (
        <label className="cf-ack">
          <input type="checkbox" checked={sure} onChange={(e) => setSure(e.target.checked)} />
          <span>Entendo que esse trabalho se perde.</span>
        </label>
      )}
      {error && <p className="fl-error">{error}</p>}
    </Dialog>
  );
}

/** The setup command's output, of a floor or of the session's copy. */
function SetupLog({ floor, title, load, onClose }: { floor: Floor; title: string; load: () => Promise<string>; onClose: () => void }) {
  const [log, setLog] = useState<string | null>(null);
  const preparing = floor.status === 'preparing';
  useEffect(() => {
    let alive = true;
    const read = () =>
      load().then(
        (t) => alive && setLog(t),
        () => alive && setLog(''),
      );
    void read();
    if (!preparing) return;
    const timer = window.setInterval(read, 2000);
    return () => {
      alive = false;
      window.clearInterval(timer);
    };
    // load is a new function on every render: the floor and its state say when to read again.
  }, [floor.id, floor.path, preparing]);
  const state = preparing ? 'rodando…' : floor.status === 'failed' ? 'falhou' : 'terminou';
  return (
    <Dialog icon="description" title={title} sub={log ? `O comando de preparo ${state}` : undefined} onClose={onClose} wide>
      {floor.status === 'failed' && floor.error && <p className="fl-error">{floor.error.split('\n')[0]}</p>}
      {log === null ? (
        <p className="field-hint">Lendo…</p>
      ) : log ? (
        <pre className="fl-log mono">{log}</pre>
      ) : (
        <p className="field-hint">Não rodou comando de preparo. Ele se configura em Novo nível de voo → Preparação.</p>
      )}
    </Dialog>
  );
}
