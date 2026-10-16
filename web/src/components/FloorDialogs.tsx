import { useEffect, useMemo, useState } from 'react';
import { ApiError, api } from '../api/client';
import type { Floor, FloorChanges, WNode } from '../api/types';
import { GROUND, plural, slugify, useBranch } from '../lib/floors';
import { shortPath } from '../lib/paths';
import { toast } from '../lib/toast';
import { useSquad } from '../state/store';
import { expectGone } from './FloorBar';
import { Dialog } from './Dialog';
import { Icon } from './Icon';
import './FloorDialogs.css';

/** The floor dialog open (FloorDialog in the store), if any. */
export function FloorDialogs() {
  const d = useSquad((s) => s.floorDialog);
  const floor = useSquad((s) => (d && d.kind !== 'create' ? s.workspace?.floors?.find((f) => f.id === d.id) : undefined));
  const close = () => useSquad.getState().setFloorDialog(null);
  if (!d) return null;
  if (d.kind === 'create') return <CreateFloor onClose={close} />;
  if (!floor) return null; // landed meanwhile
  if (d.kind === 'land') return <LandFloor key={floor.id} floor={floor} onClose={close} />;
  if (d.kind === 'discard') return <DiscardFloor key={floor.id} floor={floor} onClose={close} />;
  return <FloorLog key={floor.id} floor={floor} onClose={close} />;
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
      toast(`Andar ${f.name} criado: branch ${f.branch}`);
      onClose();
    } catch (err) {
      setError((err as Error).message);
    } finally {
      setBusy(false);
    }
  };

  return (
    <Dialog icon="layers" title="Novo andar" sub="Uma cópia isolada do repositório, num branch próprio, com um canvas próprio." onClose={onClose}>
      <form className="fl-form" onSubmit={submit}>
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
        <p className="field-hint">
          {existing ? (
            <>
              O branch <b className="mono">{target}</b> já existe: o andar abre esse branch como está.
            </>
          ) : (
            <>
              Parte do último commit de <b className="mono">{ground || GROUND}</b>. O que ainda não foi commitado fica no térreo.
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
              Copiar do térreo (arquivos fora do git)
              <input className="mono" value={copy} placeholder=".env*, config/local.json" onChange={(e) => setCopy(e.target.value)} />
            </label>
            <label className="field">
              Comando de preparo
              <input className="mono" value={setup} placeholder="ex.: npm install" onChange={(e) => setSetup(e.target.value)} />
            </label>
            <p className="field-hint">
              Valem para os próximos andares deste workspace. Os caminhos partem da pasta do workspace; o comando roda na cópia, no shell padrão, depois da cópia dos
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
            {busy ? 'Criando…' : 'Criar andar'}
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
    return { closes: on.filter((n) => n.kind !== 'note'), notes: on.filter((n) => n.kind === 'note') };
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

function useChanges(id: string): [FloorChanges | null, string] {
  const [changes, setChanges] = useState<FloorChanges | null>(null);
  const [error, setError] = useState('');
  useEffect(() => {
    api.floorChanges(id).then(setChanges, (e: Error) => setError(e.message));
  }, [id]);
  return [changes, error];
}

type Refusal = { message: string; files?: string[]; conflicts?: string[] };

function LandFloor({ floor, onClose }: { floor: Floor; onClose: () => void }) {
  const [changes, loadError] = useChanges(floor.id);
  const [branches, setBranches] = useState<string[]>([]);
  const [mode, setMode] = useState<'merge' | 'keep'>('merge');
  const [into, setInto] = useState(floor.base ?? '');
  const [refusal, setRefusal] = useState<Refusal | null>(null);
  const [busy, setBusy] = useState(false);
  const { closes, notes } = useFloorNodes(floor.id);
  useEffect(() => {
    api.branches().then(
      (r) => setBranches(r.branches.filter((b) => b !== floor.branch)),
      () => setBranches([]),
    );
  }, [floor.branch]);
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
      const r = await api.landFloor(floor.id, mode === 'merge' ? { into } : {});
      toast(
        r.merged
          ? `${floor.name} aterrissado: ${r.branch} mesclado em ${r.into} (${(r.commit ?? '').slice(0, 7)})`
          : r.upToDate
            ? `${floor.name} aterrissado: ${r.into} já tinha tudo de ${r.branch}`
            : `${floor.name} aterrissado; o trabalho continua no branch ${r.branch}`,
      );
      onClose();
    } catch (e) {
      undo();
      const body = e instanceof ApiError ? e.body : {};
      setRefusal({ message: (e as Error).message, files: body.files as string[] | undefined, conflicts: body.conflicts as string[] | undefined });
    } finally {
      setBusy(false);
    }
  };

  const target = mode === 'merge' ? into : '';
  return (
    <Dialog
      icon="flight_land"
      title={`Aterrissar ${floor.name}`}
      sub={
        <span className="mono">
          ⎇ {floor.branch}
          {target && ` → ${target}`}
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
          <button className="btn filled" disabled={busy || !changes || dirty || (mode === 'merge' && !canMerge)} onClick={land}>
            <Icon name={mode === 'merge' ? 'call_merge' : 'flight_land'} />
            {busy ? 'Aterrissando…' : mode === 'merge' ? `Mesclar e aterrissar` : 'Aterrissar'}
          </button>
        </>
      }
    >
      {loadError && <p className="fl-error">{loadError}</p>}
      {!changes && !loadError && <p className="field-hint">Lendo o andar…</p>}
      {changes && (
        <>
          {changes.missing && <p className="fl-warn">A pasta do andar sumiu. Aterrissar só limpa o registro; os commits do branch continuam.</p>}
          <section className="fl-section">
            <div className="fl-section-title">
              <Icon name="commit" size={16} />
              {changes.ahead ? plural(changes.ahead, 'commit do andar', 'commits do andar') : 'Nenhum commit novo no andar'}
              {changes.behind > 0 && (
                <span className="fl-muted">
                  · {floor.base || 'a origem'} tem {plural(changes.behind, 'commit novo', 'commits novos')}
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
          {dirty && (
            <div className="fl-warn">
              <b>{plural(changes.files.length, 'alteração não commitada', 'alterações não commitadas')}.</b> Commite antes de aterrissar (peça a um agente do andar), ou
              descarte o andar.
              <Files files={changes.files} />
            </div>
          )}
          <section className="fl-options" role="radiogroup">
            <label className={`fl-option${mode === 'merge' ? ' on' : ''}`}>
              <input type="radio" checked={mode === 'merge'} onChange={() => setMode('merge')} />
              <span>
                <b>Mesclar em</b>{' '}
                <select className="mono" value={into} onChange={(e) => setInto(e.target.value)} onClick={() => setMode('merge')}>
                  {!into && <option value="">escolha um branch</option>}
                  {(branches.includes(into) || !into ? branches : [into, ...branches]).map((b) => (
                    <option key={b} value={b}>
                      {b}
                    </option>
                  ))}
                </select>
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
          <p className="field-hint">
            Ao aterrissar: {closes.length ? `fecha ${list(closes.map(label))}; ` : ''}
            {notes.length ? `${list(notes.map(label))} ${notes.length > 1 ? 'descem' : 'desce'} para o térreo; ` : ''}a pasta {shortPath(floor.cwd)} é apagada e o
            branch {floor.branch} continua.
          </p>
          {refusal && (
            <div className="fl-error">
              {refusal.conflicts ? (
                <>
                  <b>Conflito ao mesclar em {target}. Nada mudou.</b> Peça a um agente do andar para mesclar {target} no branch {floor.branch}, resolver e commitar; depois
                  aterrisse de novo.
                  <Files files={refusal.conflicts.map((path) => ({ path }))} />
                </>
              ) : (
                <>
                  {refusal.message}
                  {refusal.files && <Files files={refusal.files.map((path) => ({ path }))} />}
                </>
              )}
            </div>
          )}
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
      toast(`Andar ${floor.name} descartado`);
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
      title={`Descartar ${floor.name}`}
      sub="Apaga a cópia e o que há nela. Não dá para desfazer."
      onClose={onClose}
      foot={
        <>
          <span className="tn-spacer" />
          <button className="btn" onClick={onClose}>
            Cancelar
          </button>
          <button className="btn fl-btn-danger" disabled={busy || !changes || (loses && !sure)} onClick={discard}>
            <Icon name="delete" />
            {busy ? 'Descartando…' : 'Descartar andar'}
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
              {list(notes.map(label))} {notes.length > 1 ? 'descem' : 'desce'} para o térreo.
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
        <label className="fl-sure">
          <input type="checkbox" checked={sure} onChange={(e) => setSure(e.target.checked)} />
          Entendo que esse trabalho se perde.
        </label>
      )}
      {error && <p className="fl-error">{error}</p>}
    </Dialog>
  );
}

function FloorLog({ floor, onClose }: { floor: Floor; onClose: () => void }) {
  const [log, setLog] = useState<string | null>(null);
  const preparing = floor.status === 'preparing';
  useEffect(() => {
    let alive = true;
    const load = () =>
      api.floorLog(floor.id).then(
        (t) => alive && setLog(t),
        () => alive && setLog(''),
      );
    void load();
    if (!preparing) return;
    const timer = window.setInterval(load, 2000);
    return () => {
      alive = false;
      window.clearInterval(timer);
    };
  }, [floor.id, preparing]);
  const state = preparing ? 'rodando…' : floor.status === 'failed' ? 'falhou' : 'terminou';
  return (
    <Dialog icon="description" title={`Preparo de ${floor.name}`} sub={log ? `O comando de preparo ${state}` : undefined} onClose={onClose} wide>
      {floor.status === 'failed' && floor.error && <p className="fl-error">{floor.error.split('\n')[0]}</p>}
      {log === null ? (
        <p className="field-hint">Lendo…</p>
      ) : log ? (
        <pre className="fl-log mono">{log}</pre>
      ) : (
        <p className="field-hint">Este andar não rodou comando de preparo. Ele se configura em Novo andar → Preparação.</p>
      )}
    </Dialog>
  );
}
