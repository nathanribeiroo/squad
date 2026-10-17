import { useEffect, useRef, useState } from 'react';
import { api } from '../api/client';
import type { Floor, FloorGit } from '../api/types';
import { shortPath } from '../lib/paths';
import { flightLevel, gitSummary, plural, useFloors } from '../lib/floors';
import { toast } from '../lib/toast';
import { useSquad } from '../state/store';
import { Icon } from './Icon';

/** Floors the user is landing or discarding here: no "landed elsewhere"
 * notice when they go. expectGone returns the undo, for a refused request. */
const expectedGone = new Set<string>();
export function expectGone(id: string): () => void {
  expectedGone.add(id);
  return () => expectedGone.delete(id);
}

/** How often the selector refreshes the floors' git state while visible. */
const POLL_MS = 10_000;

/**
 * Keeps floorStatus (branches, uncommitted changes) fresh: on opening, every
 * POLL_MS while the page is visible, when the floors change and shortly after
 * an agent's turn ends (it may have changed or committed files).
 */
function useFloorStatus() {
  const activeWs = useSquad((s) => s.activeWs);
  const activeSession = useSquad((s) => s.activeSession);
  const floors = useSquad((s) => s.workspace?.floors);
  const working = useSquad((s) => Object.values(s.runtime).filter((r) => r.activity === 'work').length);
  const refresh = useRef<() => void>(() => {});

  useEffect(() => {
    let alive = true;
    let busy = false;
    let again = false;
    const run = async () => {
      if (busy) {
        again = true;
        return;
      }
      busy = true;
      try {
        const s = await api.floorStatus();
        if (alive && useSquad.getState().activeWs === activeWs) useSquad.getState().setFloorStatus(s);
      } catch {
        /* the next poll tries again */
      } finally {
        busy = false;
        if (again && alive) {
          again = false;
          void run();
        }
      }
    };
    refresh.current = () => void run();
    void run();
    const timer = window.setInterval(() => document.visibilityState === 'visible' && void run(), POLL_MS);
    const onVisible = () => document.visibilityState === 'visible' && void run();
    document.addEventListener('visibilitychange', onVisible);
    return () => {
      alive = false;
      window.clearInterval(timer);
      document.removeEventListener('visibilitychange', onVisible);
    };
  }, [activeWs, activeSession]);

  useEffect(() => refresh.current(), [floors]);
  useEffect(() => {
    const t = window.setTimeout(() => refresh.current(), 1500);
    return () => window.clearTimeout(t);
  }, [working]);
}

/** Tells when a floor disappears without the user landing it here (a manager did). */
function useGoneNotice(floors: Floor[]) {
  const prev = useRef<Floor[] | null>(null);
  const activeWs = useSquad((s) => s.activeWs);
  useEffect(() => {
    prev.current = null; // a new workspace: no notices for its first list
  }, [activeWs]);
  useEffect(() => {
    if (prev.current) {
      for (const f of prev.current) {
        if (floors.some((x) => x.id === f.id)) continue;
        if (expectedGone.delete(f.id)) continue;
        toast(`${f.name} pousou (branch ${f.branch})`);
      }
    }
    prev.current = floors;
  }, [floors]);
}

/** Agents working (or waiting) per floor, for the tabs' activity dot. */
function useBusyFloors(): Record<string, number> {
  const nodes = useSquad((s) => s.workspace?.nodes);
  const runtime = useSquad((s) => s.runtime);
  const out: Record<string, number> = {};
  for (const n of nodes ?? []) {
    const a = runtime[n.id]?.activity;
    if (n.terminal && (a === 'work' || a === 'wait')) out[n.floor ?? ''] = (out[n.floor ?? ''] ?? 0) + 1;
  }
  return out;
}

/**
 * The floor selector in the top bar (handoff §1, item 5): the ground floor
 * and each floor, with its branch and what it has that isn't landed; + makes
 * a floor, and the floor on screen opens its menu (land, discard, log). In a
 * session that works in a copy, the ground floor is the copy, and its menu
 * merges it.
 */
export function FloorBar() {
  const floors = useFloors();
  const active = useSquad((s) => s.activeFloor);
  const setFloor = useSquad((s) => s.setFloor);
  const setDialog = useSquad((s) => s.setFloorDialog);
  const status = useSquad((s) => s.floorStatus);
  const busy = useBusyFloors();
  // Levels where a flight asks you (PRIORIDADE): an amber dot on the tab.
  const askFloors = useSquad((s) => s.askingIds.map((id) => '|' + (s.workspace?.nodes.find((n) => n.id === id)?.floor ?? '') + '|').join(''));
  const [menu, setMenu] = useState<{ id: string; x: number; y: number } | null>(null);
  const tabsRef = useRef<HTMLDivElement>(null);
  useFloorStatus();
  useGoneNotice(floors);
  useEffect(() => setMenu(null), [active]);
  // The floor on screen stays in view when the tabs scroll (tabs grow when
  // their git state arrives).
  useEffect(() => {
    tabsRef.current?.querySelector('.tb-floor.active')?.scrollIntoView({ block: 'nearest', inline: 'nearest' });
  }, [active, floors, status]);

  const copy = useSquad((s) => s.workspace?.copy);
  let ground: { text: string; dirty?: boolean; err?: boolean; spin?: boolean } | null = gitSummary(status?.repo ? status.ground : undefined);
  if (copy && !status?.ground.missing) ground = prepSummary(copy) ?? ground;
  const notRepo = status !== null && !status.repo;

  const tab = (id: string, level: string, label: string, sub: { text: string; dirty?: boolean; err?: boolean; spin?: boolean } | null, title: string) => {
    const on = id === active;
    return (
      <button
        key={id || '-ground'}
        className={`tb-floor${on ? ' active' : ''}`}
        role="tab"
        aria-selected={on}
        title={title}
        onClick={(e) => {
          if (!on) return setFloor(id);
          if (!id && !copy) return; // the runway's menu is the session copy's
          const r = e.currentTarget.getBoundingClientRect();
          setMenu(menu ? null : { id, x: r.left, y: r.bottom + 6 });
        }}
      >
        <span className="tb-floor-level">{level}</span>
        <span className="tb-floor-name">{label}</span>
        {sub && (
          <span className={`tb-floor-sub${sub.dirty ? ' dirty' : ''}${sub.err ? ' err' : ''}`}>
            {sub.spin && <Icon name="progress_activity" size={13} className="spin" />}
            {sub.text}
          </span>
        )}
        {busy[id] > 0 && (
          <span className="tb-floor-air" title={`${busy[id]} no ar`} aria-label={`${busy[id]} no ar`}>
            ▲
          </span>
        )}
        {askFloors.includes(`|${id}|`) && <span className="tb-floor-ask" title="Um voo deste nível pede você" aria-label="pede você" />}
        {on && (id || copy) && <Icon name="expand_more" size={15} className="tb-caret" />}
      </button>
    );
  };

  return (
    <>
      <div className="tb-floors">
        <div className="tb-floor-tabs" role="tablist" aria-label="Níveis de voo" ref={tabsRef}>
          {tab(
            '',
            'PISTA',
            status?.repo ? status.ground.branch || 'HEAD' : '—',
            ground,
            copy ? `Pista da sessão: a cópia em ${copy.cwd}, branch ${copy.branch}` : 'Pista: a pasta do workspace',
          )}
          {floors.map((f) => {
            const g = status?.floors[f.id];
            const sub = prepSummary(f) ?? gitSummary(g);
            return tab(f.id, flightLevel(f), f.name, sub, `${flightLevel(f)} · ${f.name}: branch ${g?.branch || f.branch}, em ${f.cwd}`);
          })}
        </div>
        <button
          className="tb-floor-add"
          title={notRepo ? 'Níveis de voo são cópias de um repositório git, e a pasta deste workspace não é um' : 'Novo nível de voo'}
          aria-label="Novo nível de voo"
          disabled={notRepo}
          onClick={() => setDialog({ kind: 'create' })}
        >
          <Icon name="add" size={18} />
        </button>
      </div>
      {menu && (menu.id ? <FloorMenu id={menu.id} x={menu.x} y={menu.y} onClose={() => setMenu(null)} /> : <CopyMenu x={menu.x} y={menu.y} onClose={() => setMenu(null)} />)}
    </>
  );
}

/** A floor's (or the session copy's) setup state, while it isn't simply ready. */
function prepSummary(f: Floor): { text: string; err?: boolean; spin?: boolean } | null {
  if (f.status === 'preparing') return { text: 'preparando', spin: true };
  if (f.status === 'failed') return { text: 'preparo falhou', err: true };
  return null;
}

/** Closes a floor menu on a click elsewhere (but on its tab, which toggles it) or Escape. */
function useMenuClose(onClose: () => void) {
  const ref = useRef<HTMLDivElement>(null);
  useEffect(() => {
    const away = (e: PointerEvent) => {
      if (!ref.current?.contains(e.target as Node) && !(e.target as Element).closest?.('.tb-floor.active')) onClose();
    };
    const esc = (e: KeyboardEvent) => e.key === 'Escape' && onClose();
    window.addEventListener('pointerdown', away, true);
    window.addEventListener('keydown', esc);
    return () => {
      window.removeEventListener('pointerdown', away, true);
      window.removeEventListener('keydown', esc);
    };
  }, [onClose]);
  return ref;
}

function copyPath(path: string) {
  navigator.clipboard?.writeText(path).then(
    () => toast('Caminho copiado'),
    () => toast(path),
  );
}

/** The ground floor's menu in a session that works in a copy. */
function CopyMenu({ x, y, onClose }: { x: number; y: number; onClose: () => void }) {
  const copy = useSquad((s) => s.workspace?.copy);
  const git = useSquad((s) => s.floorStatus?.ground);
  const setDialog = useSquad((s) => s.setFloorDialog);
  const ref = useMenuClose(onClose);
  if (!copy) return null;
  const open = (kind: 'merge-copy' | 'copy-log') => {
    onClose();
    setDialog({ kind });
  };
  return (
    <div className="popover tb-menu fl-menu" ref={ref} style={{ position: 'fixed', left: x, top: y }} role="menu">
      <FloorMenuHead level="PISTA" name="cópia da sessão" branch={copy.branch} cwd={copy.cwd} git={git} />
      <button className="tb-menu-item" onClick={() => open('merge-copy')}>
        <Icon name="call_merge" size={18} />
        Pedir pouso…
      </button>
      <button className="tb-menu-item" onClick={() => open('copy-log')}>
        <Icon name="description" size={18} />
        Log do preparo
      </button>
      <button
        className="tb-menu-item"
        onClick={() => {
          onClose();
          copyPath(copy.cwd);
        }}
      >
        <Icon name="content_copy" size={18} />
        Copiar o caminho
      </button>
    </div>
  );
}

function FloorMenu({ id, x, y, onClose }: { id: string; x: number; y: number; onClose: () => void }) {
  const floor = useSquad((s) => s.workspace?.floors?.find((f) => f.id === id));
  const git = useSquad((s) => s.floorStatus?.floors[id]);
  const setDialog = useSquad((s) => s.setFloorDialog);
  const ref = useMenuClose(onClose);
  if (!floor) return null;
  const open = (kind: 'land' | 'discard' | 'log') => {
    onClose();
    setDialog({ kind, id });
  };
  return (
    <div className="popover tb-menu fl-menu" ref={ref} style={{ position: 'fixed', left: x, top: y }} role="menu">
      <FloorMenuHead level={flightLevel(floor)} name={floor.name} branch={floor.branch} cwd={floor.cwd} git={git} />
      <button className="tb-menu-item" onClick={() => open('land')}>
        <Icon name="flight_land" size={18} />
        Pedir pouso…
      </button>
      <button className="tb-menu-item" onClick={() => open('log')}>
        <Icon name="description" size={18} />
        Log do preparo
      </button>
      <button
        className="tb-menu-item"
        onClick={() => {
          onClose();
          copyPath(floor.cwd);
        }}
      >
        <Icon name="content_copy" size={18} />
        Copiar o caminho
      </button>
      <button className="tb-menu-item fl-danger" onClick={() => open('discard')}>
        <Icon name="delete" size={18} />
        Descartar…
      </button>
    </div>
  );
}

/** "FL1 · Login · ⎇ feat/login · 2 commits · 3 alt." and the folder, atop a level's menu. */
function FloorMenuHead({ level, name, branch, cwd, git }: { level: string; name: string; branch: string; cwd: string; git?: FloorGit }) {
  const parts = [`⎇ ${git?.branch || branch}`];
  if (git?.ahead) parts.push(plural(git.ahead, 'commit', 'commits'));
  if (git?.changes) parts.push(`${git.changes} alt.`);
  return (
    <div className="fl-menu-head">
      <b>
        {level} · {name}
      </b>
      <span>{parts.join(' · ')}</span>
      <span title={cwd}>{shortPath(cwd)}</span>
    </div>
  );
}
