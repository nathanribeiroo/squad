import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { NodeResizeControl, NodeResizer, type Node, type NodeProps, type ResizeParams } from '@xyflow/react';
import { api } from '../../api/client';
import type { CargoData, CargoEntry, CargoFolder, CargoSearch, WNode } from '../../api/types';
import { Icon } from '../../components/Icon';
import type { IconName } from '../../lib/icons';
import { baseName, formatSize, joinPath, quotePaths, setCargoDrag, tildePath, type CargoItem } from '../../lib/cargo';
import { toast } from '../../lib/toast';
import { useSquad } from '../../state/store';
import { NodeMenu } from './NodeMenu';

export type CargoFlowNode = Node<{ node: WNode }, 'cargo'>;

const NO_CHANGES: Record<string, number> = {};

/** A row of the tree: an entry, or a line about a folder (loading, unreadable, more). */
type Row =
  | { type: 'entry'; path: string; entry: CargoEntry; depth: number; parent: string; rel?: string }
  | { type: 'note'; key: string; depth: number; text: string; err?: boolean };

/**
 * CARGA: a file explorer of a folder of the computer, on the canvas. Browse
 * it (the tree opens folders in place; a double click goes into one), pick
 * files and folders (click, ⌘/Ctrl, ⇧) and drag them into a flight (the
 * paths are pasted into its prompt, without Enter) or into Transmitir. No
 * wires: the agent just gets the path. A double click on a file previews it.
 */
export function CargoNode({ id, data, selected }: NodeProps<CargoFlowNode>) {
  const cargo: CargoData = data.node.cargo ?? { name: 'Carga', path: '' };
  const floor = data.node.floor ?? '';
  const [renaming, setRenaming] = useState(false);
  const [hidden, setHidden] = useState(false);
  const [folders, setFolders] = useState<Record<string, CargoFolder>>({});
  const [fails, setFails] = useState<Record<string, string>>({});
  // The cargo's folder (absolute, from its first listing) and the one shown.
  const [home, setHome] = useState<string | null>(null);
  const [missing, setMissing] = useState<string | null>(null);
  const [cwd, setCwd] = useState<string | null>(null);
  const [back, setBack] = useState<string[]>([]);
  const [forward, setForward] = useState<string[]>([]);
  const [open, setOpen] = useState<string[]>([]);
  const [sel, setSel] = useState<string[]>([]);
  const anchor = useRef<string | null>(null);
  const [cursor, setCursor] = useState<string | null>(null);
  const [query, setQuery] = useState('');
  const [hits, setHits] = useState<CargoSearch | null>(null);
  const [preview, setPreview] = useState<{ path: string; entry: CargoEntry } | null>(null);
  const [editPath, setEditPath] = useState(false);
  const list = useRef<HTMLDivElement>(null);
  const changes = useSquad((s) => s.cargoChanges[id]) ?? NO_CHANGES;

  const shown = cwd ? folders[cwd] : undefined;
  const sep = shown?.sep ?? '/';

  /** Reads a folder into the tree (quietly: what is on screen stays until it arrives). */
  const load = useCallback(
    (dir: string, show = hidden) =>
      api.cargoList(dir, floor, show).then(
        (f) => {
          setFolders((prev) => ({ ...prev, [dir]: f }));
          setFails((prev) => {
            if (!(dir in prev)) return prev;
            const next = { ...prev };
            delete next[dir];
            return next;
          });
          return f;
        },
        (e: Error) => {
          setFails((prev) => ({ ...prev, [dir]: e.message }));
          throw e;
        },
      ),
    [floor, hidden],
  );

  // The cargo's folder: on mount, and when it is set to another one.
  useEffect(() => {
    let live = true;
    api.cargoList(cargo.path, floor, hidden).then(
      (f) => {
        if (!live) return;
        setMissing(null);
        setHome(f.path);
        setCwd(f.path);
        setFolders({ [f.path]: f });
        setFails({});
        setBack([]);
        setForward([]);
        setOpen([]);
        setSel([]);
      },
      (e: Error) => live && setMissing(e.message),
    );
    return () => {
      live = false;
    };
    // hidden is applied by its own effect below
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [cargo.path, floor]);

  /** The folders on screen: the one shown and those open in the tree. */
  const visible = useMemo(() => (cwd ? [cwd, ...open.filter((d) => d !== cwd)] : []), [cwd, open]);

  // The server tells when one of them changes (another program, an agent).
  useEffect(() => {
    if (!visible.length) return;
    const t = window.setTimeout(() => api.watchCargo(id, visible).catch(() => {}), 150);
    return () => window.clearTimeout(t);
  }, [id, visible]);
  useEffect(() => () => void api.watchCargo(id, []).catch(() => {}), [id]);

  const seen = useRef(changes);
  useEffect(() => {
    const prev = seen.current;
    seen.current = changes;
    for (const [dir, seq] of Object.entries(changes)) {
      if (prev[dir] !== seq && visible.includes(dir)) load(dir).catch(() => {});
    }
  }, [changes, visible, load]);

  // Dot files shown or not: read again what is on screen.
  const firstHidden = useRef(true);
  useEffect(() => {
    if (firstHidden.current) {
      firstHidden.current = false;
      return;
    }
    for (const d of visible) load(d, hidden).catch(() => {});
    // only when hidden changes
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [hidden]);

  const refresh = () => {
    if (missing !== null) {
      api.cargoList(cargo.path, floor, hidden).then(
        (f) => {
          setMissing(null);
          setHome(f.path);
          setCwd(f.path);
          setFolders({ [f.path]: f });
        },
        (e: Error) => setMissing(e.message),
      );
      return;
    }
    for (const d of visible) load(d).catch(() => {});
  };

  /** Goes into a folder (absolute, ~, or relative to the floor's folder). */
  const go = (dir: string, how: 'push' | 'back' | 'forward' = 'push') => {
    api.cargoList(dir, floor, hidden).then(
      (f) => {
        setFolders((prev) => ({ ...prev, [f.path]: f }));
        if (cwd && f.path !== cwd) {
          if (how === 'push') {
            setBack((b) => [...b, cwd]);
            setForward([]);
          } else if (how === 'back') {
            setBack((b) => b.slice(0, -1));
            setForward((fw) => [cwd, ...fw]);
          } else {
            setForward((fw) => fw.slice(1));
            setBack((b) => [...b, cwd]);
          }
        }
        setCwd(f.path);
        setOpen([]);
        setSel([]);
        setCursor(null);
        setQuery('');
        setPreview(null);
        list.current?.scrollTo({ top: 0 });
      },
      (e: Error) => toast(e.message, 'err'),
    );
  };

  const toggle = (dir: string) => {
    if (open.includes(dir)) {
      setOpen(open.filter((d) => d !== dir && !d.startsWith(dir + sep)));
      return;
    }
    setOpen([...open, dir]);
    if (!folders[dir]) load(dir).catch(() => {});
  };

  // The tree, flattened in screen order.
  const rows = useMemo(() => {
    const out: Row[] = [];
    if (hits) {
      for (const h of hits.hits) {
        out.push({ type: 'entry', path: joinPath(hits.path, h.path.split('/').join(sep), sep), entry: h, depth: 0, parent: hits.path, rel: h.path });
      }
      return out;
    }
    const walk = (dir: string, depth: number) => {
      const f = folders[dir];
      if (!f) {
        out.push({ type: 'note', key: `${dir}:wait`, depth, text: fails[dir] ? 'Sem acesso a esta pasta' : 'Carregando…', err: !!fails[dir] });
        return;
      }
      if (f.entries.length === 0 && depth > 0) out.push({ type: 'note', key: `${dir}:empty`, depth, text: 'Pasta vazia' });
      for (const e of f.entries) {
        const p = joinPath(f.path, e.name, sep);
        out.push({ type: 'entry', path: p, entry: e, depth, parent: dir });
        if (e.dir && open.includes(p)) walk(p, depth + 1);
      }
      if (f.more) out.push({ type: 'note', key: `${dir}:more`, depth, text: `+ ${f.more} itens não mostrados: use a busca` });
    };
    if (cwd && folders[cwd]) walk(cwd, 0);
    return out;
  }, [folders, fails, open, cwd, sep, hits]);
  const entries = useMemo(() => rows.filter((r): r is Extract<Row, { type: 'entry' }> => r.type === 'entry'), [rows]);
  const byPath = useMemo(() => new Map(entries.map((r) => [r.path, r])), [entries]);
  const items = (paths: string[]): CargoItem[] => paths.map((p) => ({ path: p, dir: !!byPath.get(p)?.entry.dir }));

  // Search: under the folder shown, as the user types.
  useEffect(() => {
    const q = query.trim();
    if (!q || !cwd) {
      setHits(null);
      return;
    }
    let live = true;
    const t = window.setTimeout(() => {
      api.cargoSearch(cwd, q, floor, hidden).then(
        (r) => live && setHits(r),
        (e: Error) => live && toast(e.message, 'err'),
      );
    }, 220);
    return () => {
      live = false;
      window.clearTimeout(t);
    };
  }, [query, cwd, floor, hidden]);

  const pick = (path: string, e: { shiftKey: boolean; metaKey: boolean; ctrlKey: boolean }) => {
    setCursor(path);
    if (e.shiftKey && anchor.current) {
      const a = entries.findIndex((r) => r.path === anchor.current);
      const b = entries.findIndex((r) => r.path === path);
      if (a >= 0 && b >= 0) {
        const [from, to] = a < b ? [a, b] : [b, a];
        setSel(entries.slice(from, to + 1).map((r) => r.path));
        return;
      }
    }
    anchor.current = path;
    if (e.metaKey || e.ctrlKey) setSel((cur) => (cur.includes(path) ? cur.filter((p) => p !== path) : [...cur, path]));
    else setSel([path]);
  };

  const activate = (r: Extract<Row, { type: 'entry' }>) => {
    if (r.entry.dir) go(r.path);
    else setPreview({ path: r.path, entry: r.entry });
  };

  const copyPaths = (paths: string[]) => {
    if (!paths.length) return;
    const text = quotePaths(paths);
    navigator.clipboard.writeText(text).then(
      () => toast(paths.length === 1 ? 'Caminho copiado' : `${paths.length} caminhos copiados`),
      () => toast('Não deu para copiar', 'err'),
    );
  };

  const up = () => shown?.parent && go(shown.parent);

  const onKeyDown = (e: React.KeyboardEvent) => {
    if (e.altKey && e.key !== 'ArrowUp') return; // ⌥ shortcuts are the app's
    e.stopPropagation(); // the canvas' letters, Tab and Delete stay out
    const at = entries.findIndex((r) => r.path === cursor);
    const cur = at >= 0 ? entries[at] : undefined;
    const mod = e.metaKey || e.ctrlKey;
    const move = (to: number) => {
      const r = entries[Math.max(0, Math.min(entries.length - 1, to))];
      if (!r) return;
      if (e.shiftKey) {
        pick(r.path, { shiftKey: true, metaKey: false, ctrlKey: false });
      } else {
        anchor.current = r.path;
        setCursor(r.path);
        setSel([r.path]);
      }
      list.current?.querySelector(`[data-path="${CSS.escape(r.path)}"]`)?.scrollIntoView({ block: 'nearest' });
    };
    if (e.key === 'ArrowDown') move(at + 1);
    else if (e.key === 'ArrowUp' && (e.altKey || mod)) up();
    else if (e.key === 'ArrowUp') move(at < 0 ? entries.length - 1 : at - 1);
    else if (e.key === 'Home') move(0);
    else if (e.key === 'End') move(entries.length - 1);
    else if (e.key === 'ArrowRight' && cur?.entry.dir && !hits) {
      if (!open.includes(cur.path)) toggle(cur.path);
      else move(at + 1);
    } else if (e.key === 'ArrowLeft' && cur && !hits) {
      if (cur.entry.dir && open.includes(cur.path)) toggle(cur.path);
      else if (cur.parent !== cwd) move(entries.findIndex((r) => r.path === cur.parent));
    } else if (e.key === 'Enter' && cur) activate(cur);
    else if (e.key === ' ' && cur && !cur.entry.dir) setPreview({ path: cur.path, entry: cur.entry });
    else if (e.key === 'Backspace') up();
    else if (mod && e.key.toLowerCase() === 'c') copyPaths(sel);
    else if (mod && e.key.toLowerCase() === 'a') setSel(entries.map((r) => r.path));
    else if (e.key === 'Escape') {
      if (query) setQuery('');
      else setSel([]);
    } else return;
    e.preventDefault();
  };

  const onDragStart = (e: React.DragEvent, path: string) => {
    const paths = sel.includes(path) ? entries.filter((r) => sel.includes(r.path)).map((r) => r.path) : [path];
    if (!sel.includes(path)) {
      setSel([path]);
      anchor.current = path;
      setCursor(path);
    }
    const its = items(paths);
    setCargoDrag(e.dataTransfer, its);
    e.dataTransfer.setDragImage(ghost(its), -10, -6);
    useSquad.getState().setCargoDrag(true);
  };
  const onDragEnd = () => useSquad.getState().setCargoDrag(false);

  const pin = () =>
    cwd &&
    api
      .updateCargo(id, { path: cwd })
      .then(() => toast(`A carga agora abre em ${baseName(cwd)}`))
      .catch((e: Error) => toast(e.message, 'err'));
  const remove = () => api.removeNode(id).catch((e: Error) => toast(e.message, 'err'));

  const onResizeEnd = (_: unknown, p: ResizeParams) => {
    useSquad.getState().moveNodeLocal(id, Math.round(p.x), Math.round(p.y));
    api
      .updateNode(id, { x: Math.round(p.x), y: Math.round(p.y), w: Math.round(p.width), h: Math.round(p.height) })
      .catch((e: Error) => toast(e.message, 'err'));
  };

  const dirs = shown?.entries.filter((e) => e.dir).length ?? 0;
  const files = (shown?.entries.length ?? 0) - dirs;

  return (
    <div className={`cnode${selected ? ' selected' : ''}`}>
      <NodeResizer isVisible={selected} minWidth={260} minHeight={220} onResizeEnd={onResizeEnd} />
      {!selected && (
        <NodeResizeControl position="bottom-right" minWidth={260} minHeight={220} onResizeEnd={onResizeEnd} className="cg-grip">
          <span title="Arraste para mudar o tamanho" />
        </NodeResizeControl>
      )}
      <header className="cg-head node-drag">
        <span className="cg-tag">
          <Icon name="inventory_2" size={13} />
          CARGA
        </span>
        {renaming ? (
          <NameInput id={id} cargo={cargo} onDone={() => setRenaming(false)} />
        ) : (
          <span className="cg-title" title={`${cargo.name}\nDuplo clique para renomear`} onDoubleClick={() => setRenaming(true)}>
            {cargo.name}
          </span>
        )}
        <span className="tn-spacer" />
        <span className="pn-nav nodrag">
          <button className="icon-btn" disabled={!back.length} onClick={() => go(back[back.length - 1], 'back')} aria-label="Voltar" title="Voltar">
            <Icon name="chevron_left" size={18} />
          </button>
          <button className="icon-btn" disabled={!forward.length} onClick={() => go(forward[0], 'forward')} aria-label="Avançar" title="Avançar">
            <Icon name="chevron_right" size={18} />
          </button>
          <button className="icon-btn" disabled={!shown?.parent} onClick={up} aria-label="Pasta de cima" title="Pasta de cima (⌫)">
            <Icon name="arrow_upward" size={16} />
          </button>
          <button className="icon-btn" disabled={!home || cwd === home} onClick={() => home && go(home)} aria-label="Pasta da carga" title="Voltar à pasta da carga">
            <Icon name="home" size={16} />
          </button>
        </span>
        <NodeMenu
          items={[
            { icon: 'edit', label: 'Renomear', action: () => setRenaming(true) },
            { icon: 'keep', label: 'Fixar esta pasta', hint: 'abre aqui', action: pin, disabled: !cwd || cwd === home },
            { icon: 'content_copy', label: sel.length ? 'Copiar os caminhos' : 'Copiar o caminho da pasta', action: () => copyPaths(sel.length ? sel : cwd ? [cwd] : []) },
            { icon: hidden ? 'visibility_off' : 'visibility', label: 'Mostrar ocultos', checked: hidden, action: () => setHidden(!hidden) },
            { icon: 'refresh', label: 'Atualizar', action: refresh },
            'divider',
            { icon: 'delete', label: 'Remover do canvas', hint: 'os arquivos ficam', action: remove, danger: true },
          ]}
        />
      </header>
      <div className="cg-bar">
        {editPath || missing !== null ? (
          <PathInput
            path={cwd ?? cargo.path}
            onGo={(p) => {
              setEditPath(false);
              if (missing !== null) {
                api.updateCargo(id, { path: p }).catch((e: Error) => toast(e.message, 'err'));
                return;
              }
              if (p !== cwd) go(p);
            }}
            onCancel={() => setEditPath(false)}
            autoFocus={editPath}
          />
        ) : (
          <Crumbs path={cwd ?? ''} home={shown?.home} sep={sep} onGo={(p) => p !== cwd && go(p)} onEdit={() => setEditPath(true)} />
        )}
        <label className={`cg-search nodrag${query ? ' on' : ''}`}>
          <Icon name="search" size={15} />
          <input
            value={query}
            spellCheck={false}
            placeholder="Buscar"
            aria-label="Buscar nesta pasta"
            disabled={missing !== null}
            onChange={(e) => setQuery(e.target.value)}
            onKeyDown={(e) => {
              e.stopPropagation();
              if (e.key === 'Escape') setQuery('');
              if (e.key === 'ArrowDown' || e.key === 'Enter') {
                e.preventDefault();
                list.current?.focus();
                if (entries[0]) {
                  setSel([entries[0].path]);
                  setCursor(entries[0].path);
                  anchor.current = entries[0].path;
                }
              }
            }}
          />
          {query && (
            <button className="cg-search-x" onClick={() => setQuery('')} aria-label="Limpar a busca">
              <Icon name="close" size={14} />
            </button>
          )}
        </label>
      </div>
      <div className="cg-main">
        {missing !== null ? (
          <div className="cg-missing">
            <Icon name="folder" size={28} />
            <b>A pasta da carga não está aqui</b>
            <span className="mono">{cargo.path}</span>
            <span>Escreva outra pasta acima e tecle Enter, ou remova a carga.</span>
            <button className="btn" onClick={refresh}>
              <Icon name="refresh" size={15} />
              Tentar de novo
            </button>
          </div>
        ) : (
          <div
            ref={list}
            className="cg-list nodrag nowheel nokey"
            tabIndex={0}
            role="tree"
            aria-multiselectable="true"
            aria-label={`Arquivos de ${cargo.name}`}
            onKeyDown={onKeyDown}
            onMouseDown={(e) => {
              if (e.target === e.currentTarget) setSel([]);
            }}
          >
            {hits && hits.hits.length === 0 && <div className="cg-empty">Nada com “{query.trim()}” aqui dentro.</div>}
            {!hits && shown && shown.entries.length === 0 && (
              <div className="cg-empty">Pasta vazia{shown.hidden ? ` · ${shown.hidden} ocultos` : ''}</div>
            )}
            {rows.map((r) =>
              r.type === 'note' ? (
                <div key={r.key} className={`cg-note${r.err ? ' err' : ''}`} style={{ paddingLeft: 30 + r.depth * 16 }}>
                  {r.text}
                </div>
              ) : (
                <EntryRow
                  key={r.path}
                  row={r}
                  open={open.includes(r.path)}
                  selected={sel.includes(r.path)}
                  cursor={cursor === r.path}
                  onToggle={() => toggle(r.path)}
                  onPick={(e) => pick(r.path, e)}
                  onActivate={() => activate(r)}
                  onDragStart={(e) => onDragStart(e, r.path)}
                  onDragEnd={onDragEnd}
                />
              ),
            )}
            {hits?.more && <div className="cg-note">Há mais resultados: refine a busca.</div>}
          </div>
        )}
        {preview && (
          <Preview
            path={preview.path}
            entry={preview.entry}
            floor={floor}
            onClose={() => {
              setPreview(null);
              list.current?.focus();
            }}
          />
        )}
      </div>
      <footer className="cg-foot mono">
        {missing !== null ? (
          <span>pasta não encontrada</span>
        ) : sel.length > 0 ? (
          <span className="cg-foot-sel">
            {sel.length === 1 ? baseName(sel[0]) : `${sel.length} selecionados`} · arraste para um voo
          </span>
        ) : hits ? (
          <span>
            {hits.hits.length}
            {hits.more ? '+' : ''} resultado{hits.hits.length === 1 ? '' : 's'}
          </span>
        ) : (
          <span>
            {dirs} pasta{dirs === 1 ? '' : 's'} · {files} arquivo{files === 1 ? '' : 's'}
            {shown?.hidden ? ` · ${shown.hidden} oculto${shown.hidden === 1 ? '' : 's'}` : ''}
          </span>
        )}
      </footer>
    </div>
  );
}

const KIND_ICON: Record<string, IconName> = { text: 'description', image: 'image', video: 'movie', audio: 'music_note', pdf: 'picture_as_pdf' };

function entryIcon(e: CargoEntry, open: boolean): IconName {
  if (e.dir) return open ? 'folder_open' : 'folder';
  return KIND_ICON[e.kind ?? ''] ?? 'note';
}

function EntryRow({
  row,
  open,
  selected,
  cursor,
  onToggle,
  onPick,
  onActivate,
  onDragStart,
  onDragEnd,
}: {
  row: Extract<Row, { type: 'entry' }>;
  open: boolean;
  selected: boolean;
  cursor: boolean;
  onToggle: () => void;
  onPick: (e: React.MouseEvent) => void;
  onActivate: () => void;
  onDragStart: (e: React.DragEvent) => void;
  onDragEnd: () => void;
}) {
  const e = row.entry;
  const meta = e.dir ? (e.items !== undefined && e.items >= 0 ? `${e.items} ${e.items === 1 ? 'item' : 'itens'}` : '') : formatSize(e.size ?? 0);
  const when = new Date(e.mod).toLocaleString('pt-BR', { dateStyle: 'short', timeStyle: 'short' });
  return (
    <div
      className={`cg-row${selected ? ' sel' : ''}${cursor ? ' cur' : ''}${e.dir ? ' dir' : ''}`}
      role="treeitem"
      aria-selected={selected}
      aria-expanded={e.dir && !row.rel ? open : undefined}
      data-path={row.path}
      draggable
      style={{ paddingLeft: 6 + row.depth * 16 }}
      title={`${row.rel ?? e.name}\n${e.dir ? 'Pasta' : formatSize(e.size ?? 0)} · ${when}\nArraste para um voo ou para Transmitir`}
      onMouseDown={(ev) => {
        if (ev.button !== 0) return;
        // A plain click on a picked row keeps the selection (to drag it all);
        // it narrows on mouseup instead.
        if (selected && !ev.shiftKey && !ev.metaKey && !ev.ctrlKey) return;
        onPick(ev);
      }}
      onClick={(ev) => {
        if (selected && !ev.shiftKey && !ev.metaKey && !ev.ctrlKey && ev.detail === 1) onPick(ev);
      }}
      onDoubleClick={onActivate}
      onDragStart={onDragStart}
      onDragEnd={onDragEnd}
    >
      {e.dir && !row.rel ? (
        <button
          className={`cg-twist${open ? ' open' : ''}`}
          tabIndex={-1}
          aria-label={open ? 'Fechar a pasta' : 'Abrir a pasta aqui'}
          onMouseDown={(ev) => ev.stopPropagation()}
          onClick={(ev) => {
            ev.stopPropagation();
            onToggle();
          }}
          onDoubleClick={(ev) => ev.stopPropagation()}
        >
          <Icon name="chevron_right" size={16} />
        </button>
      ) : (
        <span className="cg-twist-gap" />
      )}
      <Icon name={entryIcon(e, open)} size={16} fill={e.dir} className={`cg-icon k-${e.dir ? 'dir' : (e.kind ?? 'file')}`} />
      <span className="cg-name">
        {row.rel ? (
          <>
            {e.name}
            {row.rel.includes('/') && <span className="cg-rel"> {row.rel.slice(0, row.rel.lastIndexOf('/'))}</span>}
          </>
        ) : (
          e.name
        )}
      </span>
      <span className="cg-meta">{meta}</span>
    </div>
  );
}

/** The drag's picture: the name, or how many. */
function ghost(items: CargoItem[]): HTMLElement {
  const el = document.createElement('div');
  el.className = 'cg-ghost';
  const icon = document.createElement('span');
  icon.className = 'msi';
  icon.textContent = items.length === 1 ? (items[0].dir ? 'folder' : 'note') : 'inventory_2';
  el.append(icon, items.length === 1 ? baseName(items[0].path) : `${items.length} itens`);
  document.body.appendChild(el);
  window.setTimeout(() => el.remove(), 0);
  return el;
}

/** The folder shown as clickable steps (~ for the home folder); a click on
 * the empty end types a path. */
function Crumbs({ path, home, sep, onGo, onEdit }: { path: string; home?: string; sep: string; onGo: (p: string) => void; onEdit: () => void }) {
  const ref = useRef<HTMLDivElement>(null);
  const [cut, setCut] = useState(false);
  useEffect(() => {
    const el = ref.current;
    if (!el) return;
    el.scrollLeft = el.scrollWidth; // the end of the path in view
    setCut(el.scrollWidth > el.clientWidth + 1);
  }, [path]);
  const steps = useMemo(() => crumbs(path, home, sep), [path, home, sep]);
  return (
    <div
      ref={ref}
      className={`cg-crumbs nodrag${cut ? ' cut' : ''}`}
      title={`${path}\nClique no fim da linha para escrever um caminho`}
      onClick={(e) => e.target === e.currentTarget && onEdit()}
    >
      {steps.map((s, i) => (
        <span key={s.path} className="cg-crumb-wrap">
          {i > 0 && <span className="cg-crumb-sep">/</span>}
          <button className={`cg-crumb${i === steps.length - 1 ? ' last' : ''}`} onClick={() => onGo(s.path)}>
            {s.label}
          </button>
        </span>
      ))}
    </div>
  );
}

function crumbs(path: string, home: string | undefined, sep: string): { label: string; path: string }[] {
  if (!path) return [];
  const out: { label: string; path: string }[] = [];
  let rest = path;
  let at = '';
  if (home && tildePath(path, home) !== path) {
    out.push({ label: '~', path: home });
    at = home;
    rest = path.slice(home.length);
  } else if (/^[A-Za-z]:/.test(path)) {
    at = path.slice(0, 2) + sep;
    out.push({ label: path.slice(0, 2), path: at });
    rest = path.slice(2);
  } else if (path.startsWith(sep)) {
    at = sep;
    out.push({ label: sep, path: sep });
  }
  for (const part of rest.split(/[\\/]+/).filter(Boolean)) {
    at = at ? joinPath(at, part, sep) : part;
    out.push({ label: part, path: at });
  }
  return out;
}

function PathInput({ path, onGo, onCancel, autoFocus }: { path: string; onGo: (p: string) => void; onCancel: () => void; autoFocus: boolean }) {
  const [draft, setDraft] = useState(path);
  return (
    <input
      className="cg-path mono nodrag"
      value={draft}
      autoFocus={autoFocus}
      spellCheck={false}
      aria-label="Caminho da pasta"
      placeholder="/caminho/da/pasta ou ~/Documentos"
      onFocus={(e) => e.target.select()}
      onChange={(e) => setDraft(e.target.value)}
      onBlur={onCancel}
      onKeyDown={(e) => {
        e.stopPropagation();
        if (e.key === 'Enter' && draft.trim()) onGo(draft.trim());
        if (e.key === 'Escape') onCancel();
      }}
    />
  );
}

/** Renames the cargo ("Vídeos parte 2", "Resultados"). */
function NameInput({ id, cargo, onDone }: { id: string; cargo: CargoData; onDone: () => void }) {
  const cancelled = useRef(false);
  const commit = (value: string) => {
    onDone();
    const name = value.trim();
    if (cancelled.current || !name || name === cargo.name) return;
    api.updateCargo(id, { name }).catch((e: Error) => toast(e.message, 'err'));
  };
  return (
    <input
      className="pn-name nodrag"
      defaultValue={cargo.name}
      autoFocus
      spellCheck={false}
      maxLength={60}
      aria-label="Nome da carga"
      onFocus={(e) => e.target.select()}
      onBlur={(e) => commit(e.target.value)}
      onKeyDown={(e) => {
        e.stopPropagation();
        if (e.key === 'Enter') e.currentTarget.blur();
        if (e.key === 'Escape') {
          cancelled.current = true;
          e.currentTarget.blur();
        }
      }}
    />
  );
}

/** How much of a text file the preview reads. */
const TEXT_PREVIEW = 256 * 1024;

/** A file, over the tree: text, an image, a video, audio or a PDF. Esc closes it. */
function Preview({ path, entry, floor, onClose }: { path: string; entry: CargoEntry; floor: string; onClose: () => void }) {
  const url = api.cargoFileURL(path, floor);
  const [text, setText] = useState<string | null>(null);
  const ref = useRef<HTMLDivElement>(null);
  useEffect(() => ref.current?.focus(), []);
  useEffect(() => {
    if (entry.kind !== 'text') return;
    let live = true;
    fetch(url, { credentials: 'same-origin', headers: { Range: `bytes=0-${TEXT_PREVIEW - 1}` } })
      .then((r) => (r.ok ? r.text() : Promise.reject(new Error(r.statusText))))
      .then((t) => live && setText(t))
      .catch(() => live && setText('Não deu para ler o arquivo.'));
    return () => {
      live = false;
    };
  }, [url, entry.kind]);
  const cut = entry.kind === 'text' && (entry.size ?? 0) > TEXT_PREVIEW;
  return (
    <div
      ref={ref}
      className="cg-preview nodrag nowheel nokey"
      tabIndex={-1}
      role="dialog"
      aria-label={`Prévia de ${entry.name}`}
      onKeyDown={(e) => {
        if (e.altKey) return;
        e.stopPropagation();
        if (e.key === 'Escape' || e.key === ' ') {
          e.preventDefault();
          onClose();
        }
      }}
    >
      <div className="cg-preview-head">
        <button className="icon-btn" onClick={onClose} aria-label="Voltar à lista" title="Voltar à lista (Esc)">
          <Icon name="arrow_back" size={16} />
        </button>
        <Icon name={entryIcon(entry, false)} size={16} className={`cg-icon k-${entry.kind ?? 'file'}`} />
        <span className="cg-preview-name" title={path}>
          {entry.name}
        </span>
        <span className="cg-meta">{formatSize(entry.size ?? 0)}</span>
        <a className="icon-btn" href={url} target="_blank" rel="noreferrer noopener" title="Abrir numa aba">
          <Icon name="open_in_new" size={15} />
        </a>
      </div>
      <div className="cg-preview-body">
        {entry.kind === 'image' ? (
          <img src={url} alt={entry.name} draggable={false} />
        ) : entry.kind === 'video' ? (
          <video src={url} controls preload="metadata" />
        ) : entry.kind === 'audio' ? (
          <audio src={url} controls preload="metadata" />
        ) : entry.kind === 'pdf' ? (
          <iframe src={url} title={entry.name} />
        ) : entry.kind === 'text' ? (
          <pre className="cg-text">
            {text ?? ''}
            {cut && text !== null && <span className="cg-cut">{`\n… mostrando os primeiros ${formatSize(TEXT_PREVIEW)}`}</span>}
          </pre>
        ) : (
          <div className="cg-empty">
            Sem prévia para este tipo de arquivo.
            <br />
            Arraste-o para um voo: o agente recebe o caminho.
          </div>
        )}
      </div>
    </div>
  );
}
